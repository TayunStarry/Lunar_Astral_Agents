package updatepack

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// Pull 拉取更新包并应用到本地（最新 / 指定 tag / 指定 commit 哈希）。
// 更新包含主程序文件时不直接合并，而是返回 NeedsRestart 结果，由宿主拉起更新应用器，
// 在进程退出、文件占用释放后完成覆盖与重启。
func Pull(cfg Config, spec PullSpec) (PullResult, error) {
	if err := validateConfig(cfg, false); err != nil {
		return PullResult{}, err
	}
	targetDir := resolveTargetDir(cfg)
	reportProgress(-1)

	switch spec.Kind {
	case PullLatest, PullTag:
		label := "最新版本"
		if spec.Kind == PullTag {
			label = "tag " + spec.Value
		}
		logf("开始拉取更新包（%s，仓库 %s/%s）", label, cfg.GiteeOwner, cfg.GiteeRepo)
		release, err := fetchRelease(cfg, spec)
		if err != nil {
			return PullResult{}, err
		}
		// 仅识别更新包附件（发布端统一命名前缀 lunar_updatepack_）。
		// Gitee 会为 Release 自动附加源码归档（<tag>.zip / <tag>.tar.gz），
		// 共用仓库时还可能混入 LTPX 市场附件，都必须跳过——否则会被当作损坏的 zip 报
		// "not a valid zip file"。
		assets := updateAssets(*release)
		if len(assets) == 0 {
			return PullResult{}, fmt.Errorf("Release %s 没有更新包附件（仅源码归档或市场附件）", release.TagName)
		}
		logf("  目标版本: %s（%d 个附件）", release.TagName, len(assets))

		// 独立临时目录：分卷文件名在多个 Release 间同名，且并发拉取时
		// 共享 %TEMP% 平铺路径会互相覆盖/清理对方正在使用的分卷
		tmpDir, err := os.MkdirTemp("", "lunar_updatepack_pull_")
		if err != nil {
			return PullResult{}, fmt.Errorf("创建临时目录失败: %w", err)
		}
		// 含主程序文件时临时目录移交更新器（解压覆盖后由其清理），此时不再就地删除
		keepTmpDir := false
		defer func() {
			if !keepTmpDir {
				os.RemoveAll(tmpDir)
			}
		}()

		// 先下载全部附件再解压：7z 分卷须齐备后从起始卷一次性解压
		var locals []string
		for i, asset := range assets {
			local := filepath.Join(tmpDir, asset.Name)
			logf("下载附件 (%d/%d): %s", i+1, len(assets), asset.Name)
			if err := downloadFile(asset.BrowserDownloadURL, cfg.GiteeToken, local, asset.Size); err != nil {
				return PullResult{}, fmt.Errorf("下载附件失败: %w", err)
			}
			reportProgress(20 + 60*(i+1)/len(assets))
			locals = append(locals, local)
		}

		// 定位 7z 起始卷（发布端统一 7z.001 分卷格式；兼容旧的单卷 .7z）
		var sevenZStart string
		for _, local := range locals {
			if strings.HasSuffix(local, ".7z.001") {
				sevenZStart = local
				break
			}
			if strings.HasSuffix(local, ".7z") {
				sevenZStart = local
			}
		}
		reportProgress(85)

		if sevenZStart == "" {
			cleanupLocals(locals)
			return PullResult{}, fmt.Errorf("附件中没有 7z 分卷（.7z.001）")
		}

		keep := resolveBackupKeep(cfg)

		// 程序文件命中检测：不解压，直接用 7z l 列出包内条目比对（命中即需走重启流程）。
		// 命中时不在此解压合并（运行中无法覆盖自身），而是把已下载的分卷整体移交更新器：
		// 主程序以 .7z.001 路径为参数拉起更新器后退出，更新器等文件占用释放后自行解压、
		// 备份覆盖并重启。
		progs, err := detectProgramFiles(sevenZStart, cfg)
		if err != nil {
			return PullResult{}, fmt.Errorf("检查更新包内容失败: %w", err)
		}
		if len(progs) > 0 {
			keepTmpDir = true
			logf("  更新包含主程序文件（%s），需关闭主程序后由更新器覆盖并重启", strings.Join(progs, ", "))
			reportProgress(100)
			return PullResult{
				NeedsRestart: true,
				PackPath:     sevenZStart,
				ProgramFiles: progs,
				BackupKeep:   keep,
			}, nil
		}

		// 纯资源模式：解压到暂存目录而非直接写入目标——更新包内含解压工具自身
		// （package/archive/7z.exe 与 7z.dll），直接解压到程序根目录时
		// 7z 需覆盖正在运行的自己 → 拒绝访问 → 致命失败。暂存目录建在目标
		// 同卷（程序根目录下）：合并时用改名落位（零拷贝），跨卷则逐文件复制+删除。
		staging := filepath.Join(targetDir, ".update_staging_"+time.Now().Format("20060102-150405"))
		logf("解压 %s（7z 分卷）", filepath.Base(sevenZStart))
		if err := extractSevenZ(sevenZStart, staging); err != nil {
			cleanupLocals(locals)
			os.RemoveAll(staging)
			return PullResult{}, fmt.Errorf("解压失败: %w", err)
		}
		cleanupLocals(locals)

		// 就地合并（暂存目录随后删除）
		defer os.RemoveAll(staging)
		updated, skipped, err := mergeTree(staging, targetDir, keep > 0)
		if err != nil {
			return PullResult{}, fmt.Errorf("合并更新文件失败: %w", err)
		}
		pruneBackups(targetDir, keep) // 只保留最近 keep 次备份
		logf("  已更新 %d 个文件 → %s", updated, targetDir)
		if len(skipped) > 0 {
			// 未能更新的文件（多为正在运行的 7z 工具自身，其内容与包内一致）：
			// 跳过该文件但保留原有文件，不中断其余更新；首个失败原因已单独记录
			logf("  [WARN] %d 个文件未能更新（保留原文件；被占用的可关闭程序后重新拉取）: %s",
				len(skipped), strings.Join(truncateList(skipped, 5), ", "))
		}
		logf("✓ 更新包拉取完成 → %s", targetDir)
		reportProgress(100)
		return PullResult{Updated: updated, Skipped: skipped, BackupKeep: keep}, nil

	case PullHash:
		hash := strings.TrimSpace(spec.Value)
		if hash == "" {
			return PullResult{}, fmt.Errorf("commit 哈希为空")
		}
		logf("开始拉取更新包（commit 哈希 %s，仓库 %s/%s）", hash, cfg.GiteeOwner, cfg.GiteeRepo)

		// 优先：哈希 → tag → Release 附件（带 SHA 校验信息）
		tagName, err := matchUpdateTagByHash(cfg, hash)
		if err != nil {
			logf("  [WARN] 标签匹配失败，改走 zipball 通道: %v", err)
		}
		if tagName != "" {
			logf("  哈希 %s 对应标签 %s，按 Release 附件拉取", shortHash(hash), tagName)
			return Pull(cfg, PullSpec{Kind: PullTag, Value: tagName})
		}

		// 兜底：zipball 通道（Gitee 支持 ref=commit SHA 直接下载仓库 zip）
		logf("  未匹配到标签，走 zipball 通道（ref=%s）", shortHash(hash))
		local := filepath.Join(os.TempDir(), "lunar_zipball_"+shortHash(hash)+".zip")
		zipballURL := fmt.Sprintf("%s/repos/%s/%s/zipball", giteeAPIBase, cfg.GiteeOwner, cfg.GiteeRepo)
		query := "?ref=" + url.QueryEscape(hash)
		if cfg.GiteeToken != "" { // 公开仓库可匿名下载，不附加空令牌参数
			query += "&access_token=" + url.QueryEscape(cfg.GiteeToken)
		}
		if err := downloadFile(zipballURL+query, "", local, 0); err != nil {
			return PullResult{}, fmt.Errorf("zipball 下载失败: %w", err)
		}
		reportProgress(70)
		logf("解压 zipball → %s", targetDir)
		if err := extractPack(local, targetDir); err != nil {
			return PullResult{}, fmt.Errorf("zipball 解压失败: %w", err)
		}
		os.Remove(local)
	}

	logf("✓ 更新包拉取完成 → %s", targetDir)
	reportProgress(100)
	return PullResult{}, nil
}

// detectProgramFiles 检测更新包内是否存在主程序文件（相对程序根目录的路径命中），
// 返回命中的相对路径清单；非空即表示本次更新需走「关程序 → 更新器覆盖 → 重启」流程。
// 基于 7z l 列出的包内条目比对，不落盘解压。
func detectProgramFiles(volPath string, cfg Config) ([]string, error) {
	entries, err := listSevenZEntries(volPath)
	if err != nil {
		return nil, err
	}
	index := make(map[string]bool, len(entries))
	for _, e := range entries {
		index[e] = true
	}
	var hit []string
	for _, rel := range cfg.ProgramFiles {
		norm := filepath.ToSlash(strings.TrimSpace(rel))
		if norm == "" {
			continue
		}
		if index[norm] {
			hit = append(hit, rel)
		}
	}
	return hit, nil
}

// listSevenZEntries 用 7z l -ba -slt 列出更新包内全部条目路径（不解压）。
// -slt 输出为结构化的 "Path = ..." 行，可完整保留含空格路径；分卷指定起始卷即可。
// 7z 在中文 Windows 上输出为 GBK 编码，先转 UTF-8 再解析。
func listSevenZEntries(volPath string) ([]string, error) {
	sevenZip, err := findSevenZip()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(sevenZip, "l", "-ba", "-slt", volPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("7z 列表失败: %w（7z: %s）", err, sevenZErrorDetail(out))
	}
	if decoded, decErr := simplifiedchinese.GBK.NewDecoder().Bytes(out); decErr == nil {
		out = decoded
	}
	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		if s, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "Path = "); ok {
			if s = strings.TrimSpace(s); s != "" {
				paths = append(paths, s)
			}
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("7z 未能列出包内条目（空包或格式异常）")
	}
	return paths, nil
}

// downloadFile 下载远程文件（token 可空；total>0 时上报进度）
func downloadFile(rawURL, token, dest string, total int64) error {
	client := &http.Client{Timeout: 30 * time.Minute}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	if token != "" && req.URL.Query().Get("access_token") == "" {
		q := req.URL.Query()
		q.Set("access_token", token)
		req.URL.RawQuery = q.Encode()
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	// 未知大小时以 Content-Length 兜底
	if total == 0 {
		total = resp.ContentLength
	}
	var sent int64
	buf := make([]byte, 256*1024)
	for {
		n, rErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := out.Write(buf[:n]); wErr != nil {
				return wErr
			}
			sent += int64(n)
			if total > 0 {
				reportProgress(int(sent * 80 / total))
			}
		}
		if rErr == io.EOF {
			break
		}
		if rErr != nil {
			return rErr
		}
	}
	return nil
}

// cleanupLocals 清理下载到临时目录的附件
func cleanupLocals(locals []string) {
	for _, local := range locals {
		os.Remove(local)
	}
}

// defaultBackupKeep 未配置时的备份保留次数
const defaultBackupKeep = 3

// resolveBackupKeep 解析备份保留次数：未配置→默认 3；0/负数→不备份
func resolveBackupKeep(cfg Config) int {
	if cfg.BackupKeep == nil {
		return defaultBackupKeep
	}
	if *cfg.BackupKeep <= 0 {
		return 0
	}
	return *cfg.BackupKeep
}

// pruneBackups 只保留最近 n 次备份目录，其余删除（n<=0 时清空全部）。
// 备份目录名形如 20060102-150405，按字典序即时间序排列。
func pruneBackups(targetDir string, n int) {
	root := filepath.Join(targetDir, ".backup")
	entries, err := os.ReadDir(root)
	if err != nil {
		return // 无备份目录
	}
	var stamps []string
	for _, e := range entries {
		if e.IsDir() {
			stamps = append(stamps, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(stamps)))

	removed := 0
	for i := n; i < len(stamps); i++ {
		if err := os.RemoveAll(filepath.Join(root, stamps[i])); err == nil {
			removed++
		}
	}
	if removed > 0 {
		logf("  已清理 %d 个过期备份（保留最近 %d 次）", removed, n)
	}
}

// mergeTree 将暂存目录内容合并到目标目录（逐文件搬迁）：
//   - withBackup 为真时，覆盖前先把已有文件备份到 {targetDir}/.backup/<时间戳>/（保留回滚能力）；
//   - 目标文件被占用（如正在运行的 7z 工具自身）时跳过该文件、保留原文件，
//     并计入 skipped —— 不中断其余文件的更新。
func mergeTree(staging, targetDir string, withBackup bool) (updated int, skipped []string, err error) {
	backupDir := filepath.Join(targetDir, ".backup", time.Now().Format("20060102-150405"))
	backedUp := 0

	walkErr := filepath.Walk(staging, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if fi.IsDir() {
			return nil
		}
		rel, rErr := filepath.Rel(staging, path)
		if rErr != nil {
			return rErr
		}
		dest := filepath.Join(targetDir, rel)
		if mkErr := os.MkdirAll(filepath.Dir(dest), 0755); mkErr != nil {
			return mkErr
		}

		var backup string
		if withBackup && fileExists(dest) {
			backup = filepath.Join(backupDir, rel)
			if mkErr := os.MkdirAll(filepath.Dir(backup), 0755); mkErr != nil {
				return mkErr
			}
			// 改名失败即该文件被占用：原文件未动，跳过而不丢失数据
			if mvErr := os.Rename(dest, backup); mvErr != nil {
				skipped = append(skipped, rel)
				return nil
			}
			backedUp++
		}

		if mvErr := moveFile(path, dest); mvErr != nil {
			if backup != "" {
				_ = os.Rename(backup, dest) // 尽力回滚
			}
			if len(skipped) == 0 {
				// 首个失败原因必须浮出：否则会被笼统归因为"文件被占用"而误导排查
				logf("  [WARN] 合并文件失败示例（%s）: %v", rel, mvErr)
			}
			skipped = append(skipped, rel)
			return nil
		}
		updated++
		return nil
	})
	if walkErr != nil {
		return updated, skipped, walkErr
	}
	if backedUp > 0 {
		logf("  覆盖前已备份 %d 个文件 → %s", backedUp, backupDir)
	}
	return updated, skipped, nil
}

// moveFile 移动文件：优先改名（同卷零拷贝），跨卷时退回复制后删除源文件
func moveFile(src, dest string) error {
	if err := os.Rename(src, dest); err == nil {
		return nil
	}
	if err := copyFile(src, dest); err != nil {
		return err
	}
	return os.Remove(src)
}

// copyFile 复制文件并显式关闭两端句柄。
// 注意：源句柄必须在返回前关闭——Windows 下句柄未释放时无法删除该文件
// （曾因 defer 关闭过晚导致 os.Remove 恒失败、所有文件被误判为"被占用跳过"）。
func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		_ = in.Close()
		return err
	}
	_, copyErr := io.Copy(out, in)
	outErr := out.Close()
	inErr := in.Close()
	if copyErr != nil {
		return copyErr
	}
	if outErr != nil {
		return outErr
	}
	return inErr
}

// truncateList 列表摘要（最多展示 n 项，超出以 "等 N 项" 收尾）
func truncateList(items []string, n int) []string {
	if len(items) <= n {
		return items
	}
	out := append([]string{}, items[:n]...)
	out = append(out, fmt.Sprintf("等 %d 项", len(items)))
	return out
}

// extractSevenZ 用 7z 解压更新包（单卷 .7z 或分卷起始卷 .7z.001）。
// 其余分卷须与起始卷同目录——7z 会自动顺序查找，下载流程已保证齐备。
// 条目路径相对 target_dir，解压到 targetDir（调用方通常传暂存目录）。
func extractSevenZ(volPath, targetDir string) error {
	sevenZip, err := findSevenZip()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return err
	}

	cmd := exec.Command(sevenZip, "x", "-y", "-o"+targetDir, volPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("7z 解压失败: %w（7z: %s）", err, sevenZErrorDetail(out))
	}
	logf("  7z 解压完成")
	return nil
}

// sevenZErrorDetail 提取 7z 输出中的关键错误行供上层展示。
// 7z 输出开头固定是版本横幅与归档信息，若只取前 N 字符会把真正的错误截掉
// （曾导致 "exit status 2" 这类报错看不出原因），故优先取 ERROR/System ERROR 行。
// 输出为 GBK 编码，先转 UTF-8。
func sevenZErrorDetail(out []byte) string {
	if decoded, decErr := simplifiedchinese.GBK.NewDecoder().Bytes(out); decErr == nil {
		out = decoded
	}
	lines := strings.Split(string(out), "\n")

	var errLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ERROR:") || strings.HasPrefix(line, "System ERROR") {
			errLines = append(errLines, line)
		}
	}
	if len(errLines) > 0 {
		return truncate(strings.Join(errLines, " | "), 300)
	}

	// 无显式 ERROR 行时退回尾部非空行（如 7z 的警告摘要）
	var tail []string
	for i := len(lines) - 1; i >= 0 && len(tail) < 5; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			tail = append([]string{s}, tail...)
		}
	}
	return truncate(strings.Join(tail, " | "), 300)
}

// findSevenZip 查找 7z 可执行文件：发行版自带（按可执行文件目录与工作目录解析）优先，
// 再退回系统安装路径。返回绝对路径（子进程 cwd 可能不同）。
func findSevenZip() (string, error) {
	rel := filepath.Join("local_data", "package", "archive", "7z.exe")
	candidates := []string{rel}
	// go test 等场景工作目录并非 exe 目录，补充按可执行文件目录解析
	if exe, err := os.Executable(); err == nil {
		candidates = append([]string{filepath.Join(filepath.Dir(exe), rel)}, candidates...)
	}
	candidates = append(candidates,
		`C:\Program Files\7-Zip\7z.exe`,
		`C:\Program Files (x86)\7-Zip\7z.exe`,
	)
	for _, p := range candidates {
		if fileExists(p) {
			if abs, err := filepath.Abs(p); err == nil {
				return abs, nil
			}
			return p, nil
		}
	}
	return "", fmt.Errorf("未找到 7z 工具（解压 7z 分卷更新包需要），请安装 7-Zip")
}

// extractPack 解压更新包到目标目录：
//   - Release 附件 zip：条目路径相对 target_dir，直接映射；
//   - zipball：首层为 {repo}-{sha}/ 目录，剥离后再映射；
//   - 被覆盖的已有文件先备份到 {targetDir}/.backup/<时间戳>/。
func extractPack(archivePath, targetDir string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()

	// 检测 zipball 前缀：首个条目形如 owner-repo-sha/
	stripPrefix := detectZipballPrefix(reader.File)

	backupDir := filepath.Join(targetDir, ".backup", time.Now().Format("20060102-150405"))
	restored := 0

	for _, f := range reader.File {
		name := f.Name
		if stripPrefix != "" {
			if !strings.HasPrefix(name, stripPrefix) {
				continue
			}
			name = strings.TrimPrefix(name, stripPrefix)
		}
		if name == "" {
			continue
		}

		// 防路径穿越
		cleaned := filepath.Clean(name)
		if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
			logf("  [WARN] 跳过可疑路径: %s", name)
			continue
		}
		dest := filepath.Join(targetDir, cleaned)

		if f.FileInfo().IsDir() {
			os.MkdirAll(dest, 0755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}

		// 已有文件先备份
		if fileExists(dest) {
			backup := filepath.Join(backupDir, cleaned)
			if err := os.MkdirAll(filepath.Dir(backup), 0755); err == nil {
				if err := os.Rename(dest, backup); err == nil {
					restored++
				}
			}
		}

		if err := extractEntry(f, dest); err != nil {
			return fmt.Errorf("解压条目 %s 失败: %w", name, err)
		}
		restored++
	}

	if backupDir != "" && restored > 0 {
		logf("  已写入 %d 个文件（被覆盖文件备份于 %s）", restored, backupDir)
	} else {
		logf("  已写入 %d 个文件", restored)
	}
	return nil
}

// detectZipballPrefix 检测 zipball 归档的顶层目录前缀
func detectZipballPrefix(files []*zip.File) string {
	if len(files) == 0 {
		return ""
	}
	first := files[0].Name
	if idx := strings.IndexByte(first, '/'); idx > 0 {
		return first[:idx+1]
	}
	return ""
}

// extractEntry 解压单个 zip 条目（流式拷贝）
func extractEntry(f *zip.File, dest string) error {
	src, err := f.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, src)
	return err
}

// shortHash 短哈希展示
func shortHash(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}
