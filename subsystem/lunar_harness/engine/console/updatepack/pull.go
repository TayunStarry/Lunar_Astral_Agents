package updatepack

// pull.go 拉取更新包并交给更新器覆盖。
//
// 更新包恒含主程序与配置指定的文件夹，覆盖只能在进程退出后进行：
// 本文件只负责下载与交接，解压、备份、覆盖与重启全部由更新器执行。

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Pull 拉取更新包（最新 / 指定 tag / 指定 commit 哈希）。
//
// 下载完成后返回分卷路径，宿主拉起更新器并退出进程；
// 运行中的主程序无法覆盖自身，因此不再保留「就地合并」的资源模式。
func Pull(cfg Config, spec PullSpec) (PullResult, error) {
	if err := validateConfig(cfg, false); err != nil {
		return PullResult{}, err
	}
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
		// 共用仓库时还可能混入 LTPX 市场附件，都必须跳过。
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
		// 下载成功后临时目录移交更新器（解压覆盖后由其清理），此时不再就地删除
		keepTmpDir := false
		defer func() {
			if !keepTmpDir {
				os.RemoveAll(tmpDir)
			}
		}()

		// 先下载全部附件：7z 分卷须齐备后从起始卷一次性解压
		var sevenZStart string
		for i, asset := range assets {
			local := filepath.Join(tmpDir, asset.Name)
			logf("下载附件 (%d/%d): %s", i+1, len(assets), asset.Name)
			if err := downloadFile(asset.BrowserDownloadURL, cfg.GiteeToken, local, asset.Size); err != nil {
				return PullResult{}, fmt.Errorf("下载附件失败: %w", err)
			}
			reportProgress(20 + 60*(i+1)/len(assets))
			if strings.HasSuffix(local, ".7z.001") {
				sevenZStart = local
			}
		}
		if sevenZStart == "" {
			return PullResult{}, fmt.Errorf("附件中没有 7z 分卷（.7z.001）")
		}

		keepTmpDir = true
		reportProgress(100)
		logf("  更新包已下载，后续由更新器覆盖并重启")
		return PullResult{
			PackPath:     sevenZStart,
			ProgramFiles: cfg.ProgramFiles,
			BackupKeep:   resolveBackupKeep(cfg),
		}, nil

	case PullHash:
		hash := strings.TrimSpace(spec.Value)
		if hash == "" {
			return PullResult{}, fmt.Errorf("commit 哈希为空")
		}
		logf("开始拉取更新包（commit 哈希 %s，仓库 %s/%s）", hash, cfg.GiteeOwner, cfg.GiteeRepo)
		tagName, err := matchUpdateTagByHash(cfg, hash)
		if err != nil {
			return PullResult{}, fmt.Errorf("匹配更新包标签失败: %w", err)
		}
		if tagName == "" {
			return PullResult{}, fmt.Errorf("提交 %s 没有对应的更新包发布；请改用版本标签或版本列表", shortHash(hash))
		}
		logf("  哈希 %s 对应标签 %s，按 Release 附件拉取", shortHash(hash), tagName)
		return Pull(cfg, PullSpec{Kind: PullTag, Value: tagName})
	}

	return PullResult{}, fmt.Errorf("未知拉取方式")
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

// shortHash 短哈希展示
func shortHash(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}
