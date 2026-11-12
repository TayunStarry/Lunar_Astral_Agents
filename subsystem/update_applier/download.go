package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// downloadHTTP 下载客户端：不设总超时，由 ctx 控制取消；依赖默认传输层的连接超时。
var downloadHTTP = &http.Client{}

// downloadReleaseAssets 下载并校验发布的全部分卷。
// 每个分卷下载到 .part，校验声明 SHA-256 后改名；校验失败整卷重下一次。
func downloadReleaseAssets(ctx context.Context, cfg *ReleaseConfig, rel *UpdateRelease, dir string, report func(TransferProgress)) error {
	if report == nil {
		report = func(TransferProgress) {}
	}
	if !rel.Valid {
		return fmt.Errorf("发布不可安装: %s", rel.Problem)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	for i, asset := range rel.Assets {
		dest := filepath.Join(dir, asset.Name)
		part := dest + ".part"
		var lastErr error
		for attempt := 0; attempt < 2; attempt++ {
			if attempt > 0 {
				_ = os.Remove(part)
				logf("分卷 %s 校验失败，重新下载", asset.Name)
			}
			report(TransferProgress{Asset: asset.Name, Total: asset.Size, Message: fmt.Sprintf("下载 %d/%d", i+1, len(rel.Assets))})
			if err := downloadOne(ctx, cfg, asset, part, report); err != nil {
				lastErr = err
				if ctx.Err() != nil {
					return ctx.Err()
				}
				continue
			}
			sum, err := hashFileSHA256(part)
			if err != nil {
				lastErr = err
				continue
			}
			if !strings.EqualFold(sum, asset.SHA256) {
				lastErr = fmt.Errorf("分卷 %s 校验失败：期望 %s，实际 %s", asset.Name, asset.SHA256, sum)
				continue
			}
			if err := os.Rename(part, dest); err != nil {
				return fmt.Errorf("保存分卷 %s 失败: %w", asset.Name, err)
			}
			lastErr = nil
			break
		}
		if lastErr != nil {
			return lastErr
		}
	}
	report(TransferProgress{Done: 1, Total: 1, Percent: 100, Message: "全部分卷下载并校验完成"})
	return nil
}

// downloadOne 下载单个分卷，支持 .part 断点续传。
func downloadOne(ctx context.Context, cfg *ReleaseConfig, asset ReleaseAsset, part string, report func(TransferProgress)) error {
	var offset int64
	if info, err := os.Stat(part); err == nil {
		offset = info.Size()
	}
	if asset.Size > 0 && offset >= asset.Size {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, withAccessToken(asset.URL, cfg.GiteeToken), nil)
	if err != nil {
		return err
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}

	resp, err := downloadHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("下载 %s 失败: %w", asset.Name, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		offset = 0
	case http.StatusPartialContent:
	default:
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("下载 %s 失败: HTTP %d %s", asset.Name, resp.StatusCode, truncateText(string(data), 256))
	}

	flags := os.O_CREATE | os.O_WRONLY
	if offset > 0 {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(part, flags, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	total := asset.Size
	if total <= 0 {
		total = offset + resp.ContentLength
	}

	buf := make([]byte, 128*1024)
	done := offset
	lastReport := time.Now()
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return err
			}
			done += int64(n)
			if time.Since(lastReport) > 200*time.Millisecond {
				lastReport = time.Now()
				report(TransferProgress{
					Asset:   asset.Name,
					Done:    done,
					Total:   total,
					Percent: percent64(done, total),
					Message: fmt.Sprintf("%s %s / %s", asset.Name, humanBytes(done), humanBytes(total)),
				})
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("下载 %s 中断: %w", asset.Name, readErr)
		}
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if total > 0 && done < total {
		return fmt.Errorf("分卷 %s 不完整：%d/%d 字节", asset.Name, done, total)
	}
	report(TransferProgress{Asset: asset.Name, Done: done, Total: total, Percent: 100, Message: asset.Name + " 下载完成"})
	return nil
}

// percent64 计算 64 位百分比。
func percent64(done, total int64) int {
	if total <= 0 {
		return 0
	}
	return int(done * 100 / total)
}

// humanBytes 人类可读的字节数。
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
