package updatepack

import (
	"archive/zip"
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

	"LunarSubsystem/TuiSystem/archive"
)

// Publish 发布更新包：收集资源 → 打包 → 创建 Release → 上传附件。
// version 为版本标签（如 v2026.10.14-1530）；为空时自动按时间生成。
func Publish(cfg Config, version string) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}
	if version == "" {
		version = "v" + time.Now().Format("2006.01.02-1504")
	}
	reportProgress(-1)
	logf("开始发布更新包 %s（仓库 %s/%s）", version, cfg.GiteeOwner, cfg.GiteeRepo)

	// ==== 1. 校验资源目录存在 ====
	targetDir := resolveTargetDir(cfg)
	var include []string
	for _, dir := range cfg.Include {
		full := filepath.Join(targetDir, dir)
		if !fileExists(full) {
			logf("  [WARN] 资源目录不存在，跳过: %s", full)
			continue
		}
		include = append(include, full)
	}
	if len(include) == 0 {
		return fmt.Errorf("没有可打包的资源目录（include 清单为空或全部缺失，target_dir=%s）", targetDir)
	}

	// ==== 2. 打包到临时目录（小包 zip；超过 90MB 走 7z 分卷） ====
	outBase := filepath.Join(os.TempDir(), "lunar_updatepack_"+version)
	totalSize := dirTotalSize(include)
	var packFiles []string
	var err error
	if totalSize > splitThreshold {
		packFiles, err = packSevenZ(cfg, version, outBase, include)
	} else {
		packFiles, err = packZip(version, outBase+".zip", include, targetDir)
	}
	if err != nil {
		return err
	}
	defer func() {
		for _, f := range packFiles {
			os.Remove(f)
		}
	}()

	// ==== 3. 计算各包 SHA-256 并生成 Release 描述 ====
	var body strings.Builder
	body.WriteString("星月智能更新包 " + version + "\n\n资源目录: " + strings.Join(cfg.Include, ", ") + "\n")
	for _, f := range packFiles {
		sum, sErr := fileSHA256(f)
		if sErr != nil {
			logf("  [WARN] 计算 SHA-256 失败 %s: %v", f, sErr)
			continue
		}
		body.WriteString(fmt.Sprintf("\n%s (SHA-256: %s)", filepath.Base(f), sum))
	}

	// ==== 4. 创建 Release ====
	reportProgress(10)
	releaseID, err := createRelease(cfg, version, body.String())
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

// createRelease 创建 Release（幂等：已存在同名 tag 时复用）
func createRelease(cfg Config, version, body string) (int64, error) {
	// 先探测是否已存在
	query := urlValues("access_token", cfg.GiteeToken)
	var existing giteeRelease
	existPath := fmt.Sprintf("/repos/%s/%s/releases/tags/%s", cfg.GiteeOwner, cfg.GiteeRepo, version)
	if err := apiRequest(existPath, query, &existing); err == nil && existing.ID > 0 {
		logf("  Release %s 已存在，复用（ID=%d）", version, existing.ID)
		return existing.ID, nil
	}

	payload, _ := json.Marshal(map[string]string{
		"access_token":     cfg.GiteeToken,
		"tag_name":         version,
		"name":             "更新包 " + version,
		"body":             body,
		"target_commitish": "master",
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

// validateConfig 校验配置完备性
func validateConfig(cfg Config) error {
	if cfg.GiteeOwner == "" || cfg.GiteeRepo == "" {
		return fmt.Errorf("未配置 Gitee 仓库（update_pack.gitee_owner / gitee_repo）")
	}
	if cfg.GiteeToken == "" {
		return fmt.Errorf("未配置 Gitee 令牌（update_pack.gitee_token，需 Release 写权限）")
	}
	return nil
}

// resolveTargetDir 解析资源根目录（空时默认 local_data）
func resolveTargetDir(cfg Config) string {
	if cfg.TargetDir == "" {
		return "local_data"
	}
	return cfg.TargetDir
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

// packZip 打包为 zip（记录相对 targetDir 的路径，解压时直接映射）
func packZip(version, outPath string, include []string, targetDir string) ([]string, error) {
	logf("打包为 zip: %s（源 %.1f MB）", outPath, float64(dirTotalSize(include))/(1024*1024))
	out, err := os.Create(outPath)
	if err != nil {
		return nil, fmt.Errorf("创建输出文件失败: %w", err)
	}
	defer out.Close()

	w := zip.NewWriter(out)
	for _, src := range include {
		relRoot, relErr := filepath.Rel(targetDir, src)
		if relErr != nil {
			relRoot = filepath.Base(src)
		}
		if wErr := walkInto(w, src, relRoot); wErr != nil {
			return nil, wErr
		}
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("收尾 zip 失败: %w", err)
	}
	logf("  zip 打包完成: %.1f MB", float64(fileSize(outPath))/(1024*1024))
	return []string{outPath}, nil
}

// walkInto 递归写入目录/文件到 zip
func walkInto(w *zip.Writer, src, relRoot string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return writeFileToZip(w, src, relRoot)
	}
	return filepath.Walk(src, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return err
		}
		rel, rErr := filepath.Rel(src, path)
		if rErr != nil {
			return rErr
		}
		return writeFileToZip(w, path, filepath.Join(relRoot, rel))
	})
}

// writeFileToZip 写入单个文件
func writeFileToZip(w *zip.Writer, path, name string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		logf("  [WARN] 读取失败跳过 %s: %v", path, err)
		return nil
	}
	entry, err := w.Create(strings.ReplaceAll(name, "\\", "/"))
	if err != nil {
		return err
	}
	_, err = entry.Write(data)
	return err
}

// splitThreshold 超过该大小改用 7z 分卷（对齐 Gitee 单附件 100MB 限制）
const splitThreshold = 90 * 1024 * 1024

// packSevenZ 大资源包走 7z 分卷（复用 archive 包的分卷压缩能力，单卷 95MB 对齐附件限制）
func packSevenZ(cfg Config, version, outBase string, include []string) ([]string, error) {
	logf("资源总量 %.1f MB 超过单附件阈值，改用 7z 分卷打包", float64(dirTotalSize(include))/(1024*1024))

	acfg := archive.Config{
		Import: include,
		Output: outBase,
		Size:   95,
		Level:  5,
		Archive: []string{
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

	// 收集产物：单卷 outBase.7z 或分卷 outBase.7z.001..N
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
