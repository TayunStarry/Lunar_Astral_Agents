package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// giteeAPIBase Gitee OpenAPI 基地址（变量形式便于本地 HTTP 服务复现真实请求路径）。
var giteeAPIBase = "https://gitee.com/api/v5"

// giteeHTTP 轻量 HTTP 客户端（列表/详情）。
var giteeHTTP = &http.Client{Timeout: 60 * time.Second}

// fetchRepoInfo 读取仓库元信息：存在性、默认分支与推送权限。
// 发布前先查一次，避免「打包半小时才在上传阶段发现令牌没权限」。
func fetchRepoInfo(cfg *ReleaseConfig) (*GiteeRepoInfo, error) {
	query := url.Values{}
	if cfg.GiteeToken != "" {
		query.Set("access_token", cfg.GiteeToken)
	}
	var info GiteeRepoInfo
	path := fmt.Sprintf("/repos/%s/%s", cfg.GiteeOwner, cfg.GiteeRepo)
	if err := giteeGet(path, query, &info); err != nil {
		return nil, fmt.Errorf("无法访问 Gitee 仓库 %s：%w（检查 gitee_owner / gitee_repo 与 gitee_token 权限）", repoLabel(cfg), err)
	}
	return &info, nil
}

// listUpdateReleases 拉取更新通道的发布列表（按创建时间倒序）。
func listUpdateReleases(cfg *ReleaseConfig) ([]UpdateRelease, error) {
	if err := validateRepoConfig(cfg, false); err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Set("per_page", "100")
	query.Set("sort", "created")
	query.Set("direction", "desc")
	if cfg.GiteeToken != "" {
		query.Set("access_token", cfg.GiteeToken)
	}

	path := fmt.Sprintf("/repos/%s/%s/releases", cfg.GiteeOwner, cfg.GiteeRepo)
	var raw []GiteeRelease
	if err := giteeGet(path, query, &raw); err != nil {
		return nil, err
	}

	out := make([]UpdateRelease, 0, len(raw))
	for _, r := range raw {
		if !isUpdateRelease(r) {
			continue
		}
		out = append(out, parseUpdateRelease(r))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt != out[j].CreatedAt {
			return out[i].CreatedAt > out[j].CreatedAt
		}
		return CompareTagNatural(out[i].Tag, out[j].Tag) > 0
	})
	return out, nil
}

// fetchUpdateRelease 按 tag 获取单个发布。
func fetchUpdateRelease(cfg *ReleaseConfig, tag string) (*UpdateRelease, error) {
	query := url.Values{}
	if cfg.GiteeToken != "" {
		query.Set("access_token", cfg.GiteeToken)
	}
	path := fmt.Sprintf("/repos/%s/%s/releases/tags/%s", cfg.GiteeOwner, cfg.GiteeRepo, url.PathEscape(tag))
	var raw GiteeRelease
	if err := giteeGet(path, query, &raw); err != nil {
		return nil, err
	}
	r := parseUpdateRelease(raw)
	return &r, nil
}

// parseUpdateRelease 解析发布的分卷、校验值与可用性。
func parseUpdateRelease(r GiteeRelease) UpdateRelease {
	shas := parseDeclaredSHAs(r.Body)
	rel := UpdateRelease{
		ID:        r.ID,
		Tag:       r.TagName,
		Name:      r.Name,
		Body:      r.Body,
		CreatedAt: r.CreatedAt,
	}
	for _, a := range r.Assets {
		if !isUpdateAsset(a.Name) {
			continue
		}
		rel.Assets = append(rel.Assets, ReleaseAsset{
			Name:   a.Name,
			Size:   a.Size,
			URL:    a.BrowserDownloadURL,
			SHA256: shas[a.Name],
		})
	}
	vols, err := orderedVolumes(r.Assets)
	if err != nil {
		rel.Problem = err.Error()
		return rel
	}
	rel.Volumes = vols
	byName := make(map[string]ReleaseAsset, len(rel.Assets))
	for _, a := range rel.Assets {
		byName[a.Name] = a
	}
	var missing []string
	ordered := make([]ReleaseAsset, 0, len(vols))
	for _, name := range vols {
		a, ok := byName[name]
		if !ok || a.SHA256 == "" {
			missing = append(missing, name)
		}
		if ok {
			ordered = append(ordered, a)
		}
	}
	rel.Assets = ordered
	if len(missing) > 0 {
		rel.Problem = "发布未声明分卷校验值：" + strings.Join(missing, "、")
		return rel
	}
	rel.Valid = true
	return rel
}

// giteeGet 执行 GET 并解析 JSON。
func giteeGet(path string, query url.Values, out any) error {
	u := giteeAPIBase + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := giteeHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("请求 Gitee 失败（GET %s）：%w", path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取 Gitee 响应失败（GET %s）：%w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Gitee HTTP %d（GET %s）：%s", resp.StatusCode, path, truncateText(string(data), 512))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("解析 Gitee 响应失败（GET %s）：%w", path, err)
	}
	return nil
}

// truncateText 截断过长的错误文本。
func truncateText(s string, limit int) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}

// withAccessToken 给下载地址附加令牌（Gitee 私有仓库下载需要）。
func withAccessToken(rawURL, token string) string {
	if token == "" || strings.Contains(rawURL, "access_token=") {
		return rawURL
	}
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + "access_token=" + url.QueryEscape(token)
}
