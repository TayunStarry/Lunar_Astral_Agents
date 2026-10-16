package updatepack

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"LunarSubsystem/TuiSystem/channel"
)

// giteeRelease Gitee Release API 响应（仅解析用到的字段）
type giteeRelease struct {
	ID        int64        `json:"id"`
	TagName   string       `json:"tag_name"`
	Name      string       `json:"name"`
	Body      string       `json:"body"`
	CreatedAt string       `json:"created_at"`
	Assets    []giteeAsset `json:"assets"`
	Target    string       `json:"target_commitish"`
}

// giteeAsset Release 附件
type giteeAsset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// giteeTag 仓库标签（哈希匹配用）
type giteeTag struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
}

// tokenQuery 构造带令牌的查询参数：令牌为空时不带 access_token
// （Gitee 读取 API 支持匿名访问，公开仓库拉取无需令牌；写操作仍必须带）
func tokenQuery(cfg Config, extra map[string]string) url.Values {
	q := url.Values{}
	if cfg.GiteeToken != "" {
		q.Set("access_token", cfg.GiteeToken)
	}
	for k, v := range extra {
		q.Set(k, v)
	}
	return q
}

// apiRequest 执行 Gitee API GET 请求并解析 JSON
func apiRequest(path string, query url.Values, out any) error {
	requestURL := giteeAPIBase + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(requestURL)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// 返回可分类的接口错误：限流/鉴权/权限需要给出不同的人话提示（见 channel/api.go）
		return &channel.APIError{Status: resp.StatusCode, Path: path, Body: truncate(string(body), 300)}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}
	return nil
}

// defaultBranch 解析仓库默认分支（创建 Release 的 target_commitish 必填）。
// 优先读仓库信息；个别仓库信息端点不可达时回退到分支列表首项。
func defaultBranch(cfg Config) string {
	query := tokenQuery(cfg, nil)
	var info struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := apiRequest(fmt.Sprintf("/repos/%s/%s", cfg.GiteeOwner, cfg.GiteeRepo), query, &info); err == nil && info.DefaultBranch != "" {
		return info.DefaultBranch
	}
	var branches []struct {
		Name string `json:"name"`
	}
	if err := apiRequest(fmt.Sprintf("/repos/%s/%s/branches", cfg.GiteeOwner, cfg.GiteeRepo), query, &branches); err == nil && len(branches) > 0 {
		return branches[0].Name
	}
	return "master"
}

// releasesQuery 发布列表查询参数。
//
// 必须显式带 direction=desc：Gitee 的列表接口**默认 asc（最旧在前）**，而下面两处
// 「取最新」都依赖「响应最新在前」。只靠参数还不够（万一对端忽略），因此拿到的列表
// 一律再用 sortReleasesNewestFirst 自己排一遍。
func releasesQuery(cfg Config) url.Values {
	return tokenQuery(cfg, map[string]string{"per_page": "100", "direction": "desc"})
}

// sortReleasesNewestFirst 把发布列表排成「最新在前」。
//
// 这是「拉取最新却拉到旧包」的修复点：不能依赖接口返回顺序。
func sortReleasesNewestFirst(releases []giteeRelease) {
	sort.SliceStable(releases, func(i, j int) bool {
		return channel.Newer(releases[i].CreatedAt, releases[i].TagName,
			releases[j].CreatedAt, releases[j].TagName)
	})
}

// ListVersions 获取仓库中「更新包通道」的发布版本列表（最新在前）。
//
// 必须过滤通道：更新包与 LTPX 应用市场共用同一仓库，未过滤会把市场包
// 混进版本列表，选中后按 tag 拉取还会下载到工具包。
func ListVersions(cfg Config) ([]Version, error) {
	if err := validateConfig(cfg, false); err != nil {
		return nil, err
	}
	var releases []giteeRelease
	if err := apiRequest(fmt.Sprintf("/repos/%s/%s/releases", cfg.GiteeOwner, cfg.GiteeRepo), releasesQuery(cfg), &releases); err != nil {
		return nil, err
	}
	sortReleasesNewestFirst(releases)

	versions := make([]Version, 0, len(releases))
	skipped := 0
	for _, r := range releases {
		if !isUpdateRelease(r) {
			skipped++
			continue
		}
		v := Version{TagName: r.TagName, Name: r.Name, Body: r.Body}
		if assets := updateAssets(r); len(assets) > 0 {
			v.AssetURL = assets[0].BrowserDownloadURL
			v.AssetSHA = extractSHA(r.Body)
		}
		versions = append(versions, v)
	}
	if skipped > 0 {
		logf("  已跳过 %d 个非更新包发布（LTPX 应用市场等）", skipped)
	}
	return versions, nil
}

// fetchRelease 按拉取方式获取目标 Release（PullHash 走 zipball，不返回 Release）。
// 取到的发布必须是更新包通道，否则报通道不符——这是「拉更新包拉到 LTPX」的拦截点。
func fetchRelease(cfg Config, spec PullSpec) (*giteeRelease, error) {
	switch spec.Kind {
	case PullLatest:
		var releases []giteeRelease
		if err := apiRequest(fmt.Sprintf("/repos/%s/%s/releases", cfg.GiteeOwner, cfg.GiteeRepo), releasesQuery(cfg), &releases); err != nil {
			return nil, err
		}
		if len(releases) == 0 {
			return nil, fmt.Errorf("仓库暂无发布版本")
		}
		// 自己排序后再取「最新且确有更新包附件」的一条：接口顺序不可信
		//（Gitee 默认 asc，实测会把更旧的包当成最新），其间还可能夹着更新的
		// 市场发布，或只有源码归档的空发布
		sortReleasesNewestFirst(releases)
		market, empty := 0, 0
		for i := range releases {
			r := releases[i]
			switch {
			case !isUpdateRelease(r):
				market++
			case len(updateAssets(r)) == 0:
				empty++
			default:
				return &r, nil
			}
		}
		return nil, fmt.Errorf("仓库中共 %d 个发布，其中 %d 个属于 LTPX 应用市场、%d 个更新包缺少附件，"+
			"没有可用的更新包发布", len(releases), market, empty)

	case PullTag:
		query := tokenQuery(cfg, nil)
		var release giteeRelease
		if err := apiRequest(fmt.Sprintf("/repos/%s/%s/releases/tags/%s", cfg.GiteeOwner, cfg.GiteeRepo, url.PathEscape(spec.Value)), query, &release); err != nil {
			return nil, fmt.Errorf("未找到 tag %s 对应的发布: %w", spec.Value, err)
		}
		if kind := releaseKind(release); kind != channel.Update {
			return nil, channelMismatchError(spec.Value, kind)
		}
		return &release, nil
	default:
		return nil, fmt.Errorf("哈希拉取走 zipball 通道，不使用 Release")
	}
}

// matchUpdateTagByHash 在仓库 tags 中查找「更新包通道」且 commit 哈希前缀匹配的 tag 名。
//
// tags 列表不带描述与附件，必须逐条回查 Release 才能判定通道；
// 未命中更新包通道的 tag（如市场包 tag）一律跳过，返回空串表示需走 zipball 兜底。
func matchUpdateTagByHash(cfg Config, hash string) (string, error) {
	query := tokenQuery(cfg, map[string]string{"per_page": "100"})
	var tags []giteeTag
	if err := apiRequest(fmt.Sprintf("/repos/%s/%s/tags", cfg.GiteeOwner, cfg.GiteeRepo), query, &tags); err != nil {
		return "", err
	}
	lower := strings.ToLower(hash)
	for _, tag := range tags {
		if !strings.HasPrefix(strings.ToLower(tag.Commit.SHA), lower) {
			continue
		}
		release, err := fetchRelease(cfg, PullSpec{Kind: PullTag, Value: tag.Name})
		if err != nil {
			continue // 该 tag 无 Release 或属于其它通道
		}
		if isUpdateRelease(*release) {
			return tag.Name, nil
		}
		logf("  跳过 tag %s（属于 LTPX 应用市场通道）", tag.Name)
	}
	return "", nil
}

// truncate 截断文本用于错误信息
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

// extractSHA 从 Release 描述中提取 SHA-256 声明（格式约定：SHA-256: <hex>）
func extractSHA(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.Index(strings.ToLower(line), "sha-256:"); idx >= 0 {
			parts := strings.Fields(line[idx:])
			if len(parts) >= 2 {
				return parts[1]
			}
		}
	}
	return ""
}
