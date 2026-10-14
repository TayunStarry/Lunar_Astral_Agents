package updatepack

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Pull 拉取更新包并解压到本地（最新 / 指定 tag / 指定 commit 哈希）
func Pull(cfg Config, spec PullSpec) error {
	if err := validateConfig(cfg); err != nil {
		return err
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
			return err
		}
		if len(release.Assets) == 0 {
			return fmt.Errorf("Release %s 没有附件", release.TagName)
		}
		logf("  目标版本: %s（%d 个附件）", release.TagName, len(release.Assets))

		for i, asset := range release.Assets {
			local := filepath.Join(os.TempDir(), asset.Name)
			logf("下载附件 (%d/%d): %s", i+1, len(release.Assets), asset.Name)
			if err := downloadFile(asset.BrowserDownloadURL, cfg.GiteeToken, local, asset.Size); err != nil {
				return fmt.Errorf("下载附件失败: %w", err)
			}
			reportProgress(20 + 60*(i+1)/len(release.Assets))
			logf("解压 %s → %s", asset.Name, targetDir)
			if err := extractPack(local, targetDir); err != nil {
				return fmt.Errorf("解压失败: %w", err)
			}
			os.Remove(local)
		}

	case PullHash:
		hash := strings.TrimSpace(spec.Value)
		if hash == "" {
			return fmt.Errorf("commit 哈希为空")
		}
		logf("开始拉取更新包（commit 哈希 %s，仓库 %s/%s）", hash, cfg.GiteeOwner, cfg.GiteeRepo)

		// 优先：哈希 → tag → Release 附件（带 SHA 校验信息）
		tagName, err := matchTagByHash(cfg, hash)
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
		if err := downloadFile(zipballURL+"?access_token="+url.QueryEscape(cfg.GiteeToken)+"&ref="+url.QueryEscape(hash), "", local, 0); err != nil {
			return fmt.Errorf("zipball 下载失败: %w", err)
		}
		reportProgress(70)
		logf("解压 zipball → %s", targetDir)
		if err := extractPack(local, targetDir); err != nil {
			return fmt.Errorf("zipball 解压失败: %w", err)
		}
		os.Remove(local)
	}

	logf("✓ 更新包拉取完成 → %s", targetDir)
	reportProgress(100)
	return nil
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
