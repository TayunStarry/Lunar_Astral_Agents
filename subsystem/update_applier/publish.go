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
		logf("[ERROR] 发布预检失败（阶段=config）：%v", err)
		return nil, fmt.Errorf("发布配置无效：%w", err)
	}

	// 预检放在打包之前：令牌无效 / 仓库不存在 / 无推送权限要在几秒内报出来，
	// 而不是等几十分钟打包完才在上传阶段失败。
	repo, err := fetchRepoInfo(cfg)
	if err != nil {
		logf("[ERROR] 发布预检失败（阶段=preflight）：%v", err)
		return nil, fmt.Errorf("发布预检失败：%w", err)
	}
	branch := strings.TrimSpace(repo.DefaultBranch)
	if repo.Permission != nil && !repo.Permission.Push {
		err := fmt.Errorf("令牌对仓库 %s 没有推送权限（Gitee 令牌需勾选 projects 范围）", repoLabel(cfg))
		logf("[ERROR] 发布预检失败（阶段=preflight）：%v", err)
		return nil, err
	}
	logf("[publish] 预检通过：%s · 默认分支 %s · 推送权限 %s", repoLabel(cfg), dashIfEmpty(branch), permText(repo.Permission))

	packDir := filepath.Join(updateRoot(root), "pack", newNonce())
	result, err := BuildUpdatePackage(root, cfg, version, packDir, report)
	if err != nil {
		logf("[ERROR] 打包失败（阶段=pack · 版本=%s）：%v", version, err)
		return nil, fmt.Errorf("打包失败：%w", err)
	}
	defer os.RemoveAll(packDir)

	body := buildReleaseBody(cfg, result)
	report(Progress{Phase: "publish", Message: "创建 Release " + version})
	releaseID, err := ensureUpdateRelease(cfg, version, body, branch)
	if err != nil {
		logf("[ERROR] 创建 Release 失败（阶段=publish · tag=%s）：%v", version, err)
		return result, fmt.Errorf("创建 Release 失败：%w", err)
	}

	total := len(result.Volumes)
	var totalBytesAll, sentAll int64
	for _, vol := range result.Volumes {
		totalBytesAll += fileSize(vol)
	}
	for i, vol := range result.Volumes {
		name := filepath.Base(vol)
		size := fileSize(vol)
		report(Progress{
			Phase: "upload", Item: i, Total: total, Percent: percent64(sentAll, totalBytesAll),
			State: "uploading", Message: name, ItemSize: size, Detail: humanBytes(size),
		})
		err := uploadReleaseAsset(ctx, cfg, releaseID, vol, func(done, _ int64) {
			report(Progress{
				Phase: "upload", Item: i, Total: total, Percent: percent64(sentAll+done, totalBytesAll),
				State: "uploading", Message: name, ItemSize: size,
				Detail: fmt.Sprintf("%s / %s", humanBytes(done), humanBytes(size)),
			})
		})
		if err != nil {
			report(Progress{
				Phase: "upload", Item: i, Total: total, Percent: percent64(sentAll, totalBytesAll),
				State: "failed", Message: name, ItemSize: size, Detail: err.Error(),
			})
			logf("[ERROR] 上传失败（阶段=upload · 第 %d/%d 卷 · %s · %s）：%v",
				i+1, total, name, humanBytes(size), err)
			return result, fmt.Errorf("上传第 %d/%d 卷 %s 失败：%w", i+1, total, name, err)
		}
		sentAll += size
		report(Progress{
			Phase: "upload", Item: i, Total: total, Percent: percent64(sentAll, totalBytesAll),
			State: "done", Message: name, ItemSize: size, Detail: humanBytes(size),
		})
	}
	report(Progress{Phase: "upload", Done: total, Total: total, Percent: 100, Message: "上传完成"})
	logf("[publish] 上传完成：%d 个分卷 · 原始大小 %s", total, humanBytes(result.Bytes))

	pruneOldUpdateReleases(cfg, version, 10)
	return result, nil
}

// permText 推送权限的可读描述
func permText(p *GiteePermission) string {
	if p == nil {
		return "未知"
	}
	if p.Push || p.Admin {
		return "可推送"
	}
	return "只读"
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
// branch 为空时不下发 target_commitish，让 Gitee 使用仓库默认分支——
// 不能硬编码 master：仓库默认分支可能是 main，分支不存在会导致创建失败。
func ensureUpdateRelease(cfg *ReleaseConfig, version, body, branch string) (int64, error) {
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
	} else if err != nil {
		// 404 属正常路径（首次发布），但把原始响应记入日志，便于区分权限/网络问题
		logf("[publish] 未查到 Release %s，将新建（查询结果：%v）", version, err)
	}

	payload := map[string]any{
		"access_token": cfg.GiteeToken,
		"tag_name":     version,
		"name":         "更新包 " + version,
		"body":         body,
	}
	if branch != "" {
		payload["target_commitish"] = branch
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("构造 Release 请求失败：%w", err)
	}
	logf("[publish] 创建 Release：tag=%s · 目标分支=%s", version, dashIfEmpty(branch))
	resp, err := giteePostJSON(fmt.Sprintf("/repos/%s/%s/releases", cfg.GiteeOwner, cfg.GiteeRepo), raw)
	if err != nil {
		return 0, err
	}
	var release GiteeRelease
	if err := json.Unmarshal(resp, &release); err != nil {
		return 0, fmt.Errorf("解析 Release 响应失败：%w（原始响应：%s）", err, truncateText(string(resp), 256))
	}
	if release.ID == 0 {
		return 0, fmt.Errorf("Release 创建异常：%s", truncateText(string(resp), 512))
	}
	logf("[publish] Release 已创建：ID=%d · tag=%s", release.ID, release.TagName)
	return release.ID, nil
}

// uploadReleaseAsset 以流式 multipart 上传单个分卷。
// 失败时把 HTTP 状态与 Gitee 返回体原样带回，由调用方写入日志与界面。
func uploadReleaseAsset(ctx context.Context, cfg *ReleaseConfig, releaseID int64, filePath string, onProgress func(done, total int64)) error {
	name := filepath.Base(filePath)
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("打开分卷失败：%w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("读取分卷信息失败：%w", err)
	}

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	// 服务端提前拒绝（401/413）时取消读取端，避免写协程永久阻塞
	defer pr.Close()
	go func() {
		part, err := mw.CreateFormFile("file", name)
		if err == nil {
			_, err = io.Copy(part, &progressReader{r: f, total: info.Size(), onProgress: onProgress})
		}
		if cerr := mw.Close(); err == nil {
			err = cerr
		}
		pw.CloseWithError(err)
	}()

	logf("[publish] 上传 %s（%s）→ /repos/%s/%s/releases/%d/attach_files",
		name, humanBytes(info.Size()), cfg.GiteeOwner, cfg.GiteeRepo, releaseID)

	u := uploadAssetURL(cfg, releaseID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, pr)
	if err != nil {
		return fmt.Errorf("构造上传请求失败：%w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("上传请求未完成：%w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	_ = pr.Close()
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		logf("[publish] 上传成功：%s · HTTP %d", name, resp.StatusCode)
		return nil
	}
	body := truncateText(string(data), 512)
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("HTTP %d：令牌无效或缺少仓库写权限（检查 gitee_token 的 projects 范围）· Gitee 返回：%s", resp.StatusCode, body)
	case http.StatusNotFound:
		return fmt.Errorf("HTTP %d：Release %d 不存在或已被删除 · Gitee 返回：%s", resp.StatusCode, releaseID, body)
	case http.StatusRequestEntityTooLarge:
		return fmt.Errorf("HTTP %d：分卷过大（%s）超过 Gitee 单文件上限，请调小 pack_size_mb 后重试 · Gitee 返回：%s", resp.StatusCode, humanBytes(info.Size()), body)
	default:
		return fmt.Errorf("HTTP %d：%s", resp.StatusCode, body)
	}
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
	req, err := http.NewRequest(http.MethodPost, giteeAPIBase+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("构造请求失败（POST %s）：%w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败（POST %s）：%w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败（POST %s）：%w", path, err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Gitee HTTP %d（POST %s）：%s", resp.StatusCode, path, truncateText(string(body), 512))
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
