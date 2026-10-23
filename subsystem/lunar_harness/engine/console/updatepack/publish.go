package updatepack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/archive"
	"LunarSubsystem/LunarHarness/engine/console/channel"
)

// Publish 发布更新包：收集资源 → 打包 → 创建 Release → 上传附件。
// version 为版本标签（如 v2026.10.14-1530）；为空时自动按时间生成。
func Publish(cfg Config, version string) error {
	if err := validateConfig(cfg, true); err != nil {
		return err
	}
	if version == "" {
		version = "v" + time.Now().Format("2006.01.02-1504")
	}
	reportProgress(-1)
	logf("开始发布更新包 %s（仓库 %s/%s）", version, cfg.GiteeOwner, cfg.GiteeRepo)

	// ==== 1. 校验资源目录/程序文件存在 ====
	// 打包范围 = include（资源，相对 target_dir）+ program_files（主程序，相对 target_dir）
	targetDir := resolveTargetDir(cfg)
	candidates := make([]string, 0, len(cfg.Include)+len(cfg.ProgramFiles))
	candidates = append(candidates, cfg.Include...)
	candidates = append(candidates, cfg.ProgramFiles...)

	var include []string
	for _, rel := range candidates {
		if strings.TrimSpace(rel) == "" {
			continue
		}
		full := filepath.Join(targetDir, rel)
		if !fileExists(full) {
			logf("  [WARN] 资源/文件不存在，跳过: %s", full)
			continue
		}
		include = append(include, full)
	}
	if len(include) == 0 {
		return fmt.Errorf("没有可打包的资源目录（include/program_files 清单为空或全部缺失，target_dir=%s）", targetDir)
	}

	// ==== 2. 打包到临时目录（统一 7z 分卷：小资源产出单个 .7z.001，大资源自动分卷） ====
	outBase := filepath.Join(os.TempDir(), "lunar_updatepack_"+version)
	packFiles, err := packSevenZ(version, outBase, include, resolveExclude(cfg), resolveExcludeDirs(cfg))
	if err != nil {
		return err
	}
	defer func() {
		for _, f := range packFiles {
			os.Remove(f)
		}
	}()

	// ==== 3. 计算各包 SHA-256 并生成 Release 描述 ====
	// 描述采用 RELEASE.md 的精简发行说明（是什么 / 本次内容 / 怎么装 / 校验），
	// 首行带 [UPDATE] 通道标记——与 LTPX 应用市场共用仓库时靠它判别归属。
	shas := make(map[string]string, len(packFiles))
	for _, f := range packFiles {
		sum, sErr := fileSHA256(f)
		if sErr != nil {
			logf("  [WARN] 计算 SHA-256 失败 %s: %v", f, sErr)
			continue
		}
		shas[filepath.Base(f)] = sum
	}
	body := buildReleaseBody(cfg, version, packFiles, shas)

	// ==== 4. 创建 Release ====
	reportProgress(10)
	releaseID, err := createRelease(cfg, version, body)
	if err != nil {
		return fmt.Errorf("创建 Release 失败: %w", err)
	}

	// ==== 5. 上传附件 ====
	for i, f := range packFiles {
		logf("上传附件 (%d/%d): %s (%.1f MB)", i+1, len(packFiles), filepath.Base(f), float64(fileSize(f))/(1024*1024))
		if err := uploadAsset(cfg, releaseID, f); err != nil {
			return fmt.Errorf("上传附件失败: %w", err)
		}
		reportProgress(30 + 70*(i+1)/len(packFiles))
	}

	logf("✓ 更新包 %s 发布完成（共 %d 个附件）", version, len(packFiles))
	reportProgress(100)
	return nil
}

// createRelease 创建 Release（幂等：已存在同名 tag 且属于更新包通道时复用）。
//
// 复用前必须校验通道：更新包与 LTPX 应用市场共用同一仓库，若某市场包的 id
// 恰好等于本次版本号，直接复用会把更新包附件传到市场发布上（两侧同时损坏）。
func createRelease(cfg Config, version, body string) (int64, error) {
	// 先探测是否已存在
	query := urlValues("access_token", cfg.GiteeToken)
	var existing giteeRelease
	existPath := fmt.Sprintf("/repos/%s/%s/releases/tags/%s", cfg.GiteeOwner, cfg.GiteeRepo, version)
	if err := apiRequest(existPath, query, &existing); err == nil && existing.ID > 0 {
		if kind := releaseKind(existing); kind != channel.Update {
			return 0, fmt.Errorf("tag %s 已被「%s」占用（Release ID=%d）：两条通道共用同一仓库，"+
				"请改用其它版本号，或先处理该发布", version, kind, existing.ID)
		}
		logf("  Release %s 已存在（更新包通道），复用（ID=%d）", version, existing.ID)
		return existing.ID, nil
	}

	payload, _ := json.Marshal(map[string]string{
		"access_token":     cfg.GiteeToken,
		"tag_name":         version,
		"name":             "更新包 " + version,
		"body":             body,
		"target_commitish": defaultBranch(cfg),
	})
	resp, err := postJSON(fmt.Sprintf("/repos/%s/%s/releases", cfg.GiteeOwner, cfg.GiteeRepo), payload)
	if err != nil {
		return 0, err
	}
	var release giteeRelease
	if err := json.Unmarshal(resp, &release); err != nil {
		return 0, fmt.Errorf("解析 Release 响应失败: %w", err)
	}
	if release.ID == 0 {
		return 0, fmt.Errorf("Release 创建异常: %s", truncate(string(resp), 300))
	}
	logf("  Release 已创建: %s（ID=%d）", version, release.ID)
	return release.ID, nil
}

// uploadAsset 上传 zip/7z 附件到 Release
func uploadAsset(cfg Config, releaseID int64, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	uploadURL := fmt.Sprintf("%s/repos/%s/%s/releases/%d/attach_files?access_token=%s",
		giteeAPIBase, cfg.GiteeOwner, cfg.GiteeRepo, releaseID, cfg.GiteeToken)

	pr := newProgressReader(f, fileSize(filePath))
	body := &multipartBody{field: "file", filename: filepath.Base(filePath), reader: pr}
	req, err := http.NewRequest(http.MethodPost, uploadURL, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", body.ContentType())

	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(data), 300))
	}
	return nil
}

// progressReader 边读边上报告进度
type progressReader struct {
	r     io.Reader
	total int64
	sent  int64
}

func newProgressReader(r io.Reader, total int64) *progressReader {
	return &progressReader{r: r, total: total}
}

func (p *progressReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	if n > 0 && p.total > 0 {
		p.sent += int64(n)
		reportProgress(int(p.sent * 100 / p.total))
	}
	return n, err
}

// multipartBody 简化 multipart 上传体（单文件字段，无额外表单项）
type multipartBody struct {
	field    string
	filename string
	reader   io.Reader
	written  bool
	boundary string
}

// ContentType 返回 multipart 内容类型
func (m *multipartBody) ContentType() string {
	if m.boundary == "" {
		m.boundary = "lunarUpdatePackBoundary7d1a2c"
	}
	return "multipart/form-data; boundary=" + m.boundary
}

// Read 按 multipart 编码流式产出：起始边界 → 文件头 → 文件内容 → 结束边界
func (m *multipartBody) Read(p []byte) (int, error) {
	if !m.written {
		m.written = true
		header := "--" + m.boundary + "\r\n" +
			"Content-Disposition: form-data; name=\"" + m.field + "\"; filename=\"" + m.filename + "\"\r\n" +
			"Content-Type: application/octet-stream\r\n\r\n"
		m.reader = io.MultiReader(strings.NewReader(header), m.reader,
			strings.NewReader("\r\n--"+m.boundary+"--\r\n"))
	}
	return m.reader.Read(p)
}

// ==== 辅助 ====

// validateConfig 校验配置完备性。needToken 为 true 时（发布/写操作）要求令牌非空；
// 拉取公开仓库属只读操作，可不带令牌（Gitee 读取 API 支持匿名访问）。
func validateConfig(cfg Config, needToken bool) error {
	if cfg.GiteeOwner == "" || cfg.GiteeRepo == "" {
		return fmt.Errorf("未配置 Gitee 仓库（update_pack.gitee_owner / gitee_repo）")
	}
	if needToken && cfg.GiteeToken == "" {
		return fmt.Errorf("未提供 Gitee 令牌（发布需 Release 读写权限的私人令牌）")
	}
	return nil
}

// resolveTargetDir 解析资源/程序根目录（空时默认 local_data）。
// 相对路径一律以可执行文件目录（程序根）为基准：程序可能自任意工作目录启动，
// 若按工作目录解析，相对路径会落到用户当前目录——在「程序根目录」语义下后果严重
// （更新会写错位置，甚至把主程序覆盖到别处）。
func resolveTargetDir(cfg Config) string {
	root := cfg.TargetDir
	if root == "" {
		root = "local_data"
	}
	if filepath.IsAbs(root) {
		return root
	}
	if exe, err := os.Executable(); err == nil {
		if joined, err := filepath.Abs(filepath.Join(filepath.Dir(exe), root)); err == nil {
			return joined
		}
	}
	if abs, err := filepath.Abs(root); err == nil {
		return abs
	}
	return root
}

// fileExists 检查文件/目录存在
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// fileSize 取文件大小
func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// dirTotalSize 递归统计目录总大小
func dirTotalSize(dirs []string) int64 {
	var total int64
	for _, dir := range dirs {
		filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
			if err == nil && info != nil && !info.IsDir() {
				total += info.Size()
			}
			return nil
		})
	}
	return total
}

// fileSHA256 计算文件 SHA-256
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// defaultExcludePatterns 更新包默认排除清单（同步 environment_repair/archive 的
// 判定流程：日志、临时文件、版本库与构建产物不进更新包）。可用 update_pack.exclude 覆盖。
var defaultExcludePatterns = []string{
	"*.log", "*.tmp", "*.bak",
	".git/", "node_modules/", "__pycache__/", "*.pyc", "*.pyo", "*.egg-info/",
	"dist/", "build/",
	".DS_Store", "*.ts", "*.go",
}

// resolveExclude 解析打包排除规则（未配置时用默认清单）
func resolveExclude(cfg Config) []string {
	if len(cfg.Exclude) > 0 {
		return cfg.Exclude
	}
	return defaultExcludePatterns
}

// resolveExcludeDirs 解析「目录级排除」标记（未配置时默认 metadata.json）
func resolveExcludeDirs(cfg Config) []string {
	if len(cfg.ExcludeDirsWith) > 0 {
		return cfg.ExcludeDirsWith
	}
	return []string{"metadata.json"}
}

// packSevenZ 以 7z 分卷打包（统一格式：小资源产出单个 .7z.001，大资源自动分卷，
// 单卷 95MB 对齐 Gitee 单附件 100MB 限制）。复用 archive 包的扫描/排除/分卷能力。
func packSevenZ(_, outBase string, include, exclude, excludeDirsWith []string) ([]string, error) {
	logf("资源总量 %.1f MB，7z 分卷打包", float64(dirTotalSize(include))/(1024*1024))

	acfg := archive.Config{
		Import:          include,
		Exclude:         exclude,
		ExcludeDirsWith: excludeDirsWith,
		Output:          outBase,
		Size:            95,
		Level:           5,
		Archive: []string{
			// 发行版自带的 7z 优先（宿主工作目录），再退回系统安装路径
			filepath.Join("local_data", "package", "archive", "7z.exe"),
			`C:\Program Files\7-Zip\7z.exe`,
			`C:\Program Files (x86)\7-Zip\7z.exe`,
		},
	}
	tmpCfg := outBase + "_cfg.json"
	payload, _ := json.Marshal(acfg)
	if err := os.WriteFile(tmpCfg, payload, 0644); err != nil {
		return nil, fmt.Errorf("写入临时打包配置失败: %w", err)
	}
	defer os.Remove(tmpCfg)

	if err := archive.Execute(tmpCfg, &acfg); err != nil {
		return nil, fmt.Errorf("7z 分卷打包失败: %w", err)
	}

	// 收集产物：单卷 outBase.7z.001 或分卷 outBase.7z.001..N
	matches, err := filepath.Glob(outBase + ".7z*")
	if err != nil || len(matches) == 0 {
		return nil, fmt.Errorf("未找到打包产物")
	}
	return matches, nil
}

// postJSON 执行 POST 请求并返回响应体
func postJSON(path string, payload []byte) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(giteeAPIBase+path, "application/json", strings.NewReader(string(payload)))
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 300))
	}
	return body, nil
}

// urlValues 快速构造单键查询
func urlValues(key, value string) (v map[string][]string) { return map[string][]string{key: {value}} }
