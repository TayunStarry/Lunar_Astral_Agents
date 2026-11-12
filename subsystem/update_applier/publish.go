package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// PublishUpdate 打包并上传更新包到 Gitee Release。
func PublishUpdate(ctx context.Context, root string, cfg *ReleaseConfig, version string, report func(Progress)) (*PackResult, error) {
	if report == nil {
		report = func(Progress) {}
	}
	if err := validateRepoConfig(cfg, true); err != nil {
		return nil, err
	}

	packDir := filepath.Join(updateRoot(root), "pack", newNonce())
	result, err := BuildUpdatePackage(root, cfg, version, packDir, report)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(packDir)

	body := buildReleaseBody(cfg, result)
	report(Progress{Phase: "publish", Message: "创建 Release"})
	releaseID, err := ensureUpdateRelease(cfg, version, body)
	if err != nil {
		return nil, err
	}

	for i, vol := range result.Volumes {
		name := filepath.Base(vol)
		report(Progress{Phase: "upload", Done: i, Total: len(result.Volumes), Percent: percent(i, len(result.Volumes)), Message: name})
		if err := uploadReleaseAsset(ctx, cfg, releaseID, vol, func(done, total int64) {
			report(Progress{
				Phase:   "upload",
				Done:    i,
				Total:   len(result.Volumes),
				Percent: percent(i, len(result.Volumes)),
				Message: fmt.Sprintf("%s %s / %s", name, humanBytes(done), humanBytes(total)),
			})
		}); err != nil {
			return nil, fmt.Errorf("上传 %s 失败: %w", name, err)
		}
	}
	report(Progress{Phase: "upload", Done: len(result.Volumes), Total: len(result.Volumes), Percent: 100, Message: "上传完成"})

	pruneOldUpdateReleases(cfg, version, 10)
	return result, nil
}

// buildReleaseBody 组装发行说明；「- 名称（大小） / SHA-256」格式与解析端约定一致。
func buildReleaseBody(cfg *ReleaseConfig, result *PackResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s 星月智能 · 更新包 %s\n\n", updateMarker, result.Version)
	b.WriteString("**这是什么**\n")
	b.WriteString("星月智能（Lunar Astral Agents）纯本地化 AI 智能体平台的增量更新包。\n")
	b.WriteString("内含主程序与指定文件夹，覆盖到程序根目录即可生效。\n\n")
	b.WriteString("**安装方式**\n")
	b.WriteString("1. 推荐：运行 `Lunar_Update_Applier.exe`，在「版本列表」中选择本版本并安装。\n")
	b.WriteString("2. 手动：下载全部分卷后从 `.7z.001` 起解压，将内容覆盖到程序根目录。\n\n")
	b.WriteString("**注意**：全部分卷需齐备后再解压；安装器会校验每个分卷的 SHA-256 并保留备份。\n\n")
	b.WriteString("**本次发布**\n")
	fmt.Fprintf(&b, "- 版本：%s\n", result.Version)
	fmt.Fprintf(&b, "- 时间：%s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "- 文件数：%d\n", result.FileCount)
	fmt.Fprintf(&b, "- 原始大小：%s\n", humanBytes(result.Bytes))
	if len(cfg.Include) > 0 {
		fmt.Fprintf(&b, "- 覆盖文件夹：%s\n", strings.Join(cfg.Include, "、"))
	}
	if len(cfg.ProgramFiles) > 0 {
		fmt.Fprintf(&b, "- 主程序文件：%s\n", strings.Join(cfg.ProgramFiles, "、"))
	}
	b.WriteString("\n**包内文件与校验**\n")
	for _, vol := range result.Volumes {
		name := filepath.Base(vol)
		fmt.Fprintf(&b, "- %s（%.1f MB）\n", name, float64(fileSize(vol))/(1024*1024))
		if sum := result.SHAs[name]; sum != "" {
			fmt.Fprintf(&b, "  SHA-256: %s\n", sum)
		}
	}
	return b.String()
}

// ensureUpdateRelease 创建 Release；同名 tag 已存在且属于更新通道时复用。
func ensureUpdateRelease(cfg *ReleaseConfig, version, body string) (int64, error) {
	query := url.Values{}
	if cfg.GiteeToken != "" {
		query.Set("access_token", cfg.GiteeToken)
	}
	var existing GiteeRelease
	path := fmt.Sprintf("/repos/%s/%s/releases/tags/%s", cfg.GiteeOwner, cfg.GiteeRepo, url.PathEscape(version))
	if err := giteeGet(path, query, &existing); err == nil && existing.ID > 0 {
		if !isUpdateRelease(existing) {
			return 0, fmt.Errorf("tag %s 已被非更新包发布占用（Release ID=%d）", version, existing.ID)
		}
		logf("Release %s 已存在，复用 ID=%d", version, existing.ID)
		return existing.ID, nil
	}

	payload, _ := json.Marshal(map[string]string{
		"access_token":     cfg.GiteeToken,
		"tag_name":         version,
		"name":             "更新包 " + version,
		"body":             body,
		"target_commitish": defaultBranchName(cfg),
	})
	resp, err := giteePostJSON(fmt.Sprintf("/repos/%s/%s/releases", cfg.GiteeOwner, cfg.GiteeRepo), payload)
	if err != nil {
		return 0, err
	}
	var release GiteeRelease
	if err := json.Unmarshal(resp, &release); err != nil {
		return 0, fmt.Errorf("解析 Release 响应失败: %w", err)
	}
	if release.ID == 0 {
		return 0, fmt.Errorf("Release 创建异常: %s", truncateText(string(resp), 512))
	}
	return release.ID, nil
}

// defaultBranchName 返回创建 Release 使用的目标分支。
func defaultBranchName(*ReleaseConfig) string {
	return "master"
}

// uploadReleaseAsset 以流式 multipart 上传单个分卷。
func uploadReleaseAsset(ctx context.Context, cfg *ReleaseConfig, releaseID int64, filePath string, onProgress func(done, total int64)) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		part, err := mw.CreateFormFile("file", filepath.Base(filePath))
		if err == nil {
			_, err = io.Copy(part, &progressReader{r: f, total: info.Size(), onProgress: onProgress})
		}
		if cerr := mw.Close(); err == nil {
			err = cerr
		}
		pw.CloseWithError(err)
	}()

	u := uploadAssetURL(cfg, releaseID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, pr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateText(string(data), 512))
	}
	return nil
}

// uploadAssetURL 构造附件上传地址（令牌放查询参数，Gitee v5 约定）。
// 单独成函数便于在不联网的情况下校验请求构造。
func uploadAssetURL(cfg *ReleaseConfig, releaseID int64) string {
	return fmt.Sprintf("%s/repos/%s/%s/releases/%d/attach_files?access_token=%s",
		giteeAPIBase, cfg.GiteeOwner, cfg.GiteeRepo, releaseID, url.QueryEscape(cfg.GiteeToken))
}

// progressReader 边读边回报字节进度。
type progressReader struct {
	r          io.Reader
	total      int64
	sent       int64
	last       time.Time
	onProgress func(done, total int64)
}

func (p *progressReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	if n > 0 {
		p.sent += int64(n)
		if p.onProgress != nil && time.Since(p.last) > 200*time.Millisecond {
			p.last = time.Now()
			p.onProgress(p.sent, p.total)
		}
	}
	return n, err
}

// giteePostJSON 执行 JSON POST。
func giteePostJSON(path string, payload []byte) ([]byte, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Post(giteeAPIBase+path, "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateText(string(body), 512))
	}
	return body, nil
}

// pruneOldUpdateReleases 删除超出保留数量的旧更新包发布与标签（best-effort）。
func pruneOldUpdateReleases(cfg *ReleaseConfig, currentTag string, keep int) {
	rels, err := listUpdateReleases(cfg)
	if err != nil || len(rels) <= keep {
		return
	}
	sort.SliceStable(rels, func(i, j int) bool { return rels[i].CreatedAt > rels[j].CreatedAt })
	for _, r := range rels[keep:] {
		if r.Tag == currentTag {
			continue
		}
		if err := giteeDelete(fmt.Sprintf("/repos/%s/%s/releases/%d", cfg.GiteeOwner, cfg.GiteeRepo, r.ID), cfg); err != nil {
			logf("删除旧发布 %s 失败: %v", r.Tag, err)
			continue
		}
		if err := giteeDelete(fmt.Sprintf("/repos/%s/%s/tags/%s", cfg.GiteeOwner, cfg.GiteeRepo, url.PathEscape(r.Tag)), cfg); err != nil {
			logf("删除旧标签 %s 失败: %v", r.Tag, err)
		}
	}
}

// giteeDelete 执行 DELETE（令牌放在查询参数）。
func giteeDelete(path string, cfg *ReleaseConfig) error {
	u := giteeAPIBase + path + "?access_token=" + url.QueryEscape(cfg.GiteeToken)
	req, err := http.NewRequest(http.MethodDelete, u, nil)
	if err != nil {
		return err
	}
	resp, err := giteeHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateText(string(data), 256))
	}
	return nil
}
