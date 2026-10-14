package updatepack

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// giteeRelease Gitee Release API 响应（仅解析用到的字段）
type giteeRelease struct {
	ID      int64          `json:"id"`
	TagName string         `json:"tag_name"`
	Name    string         `json:"name"`
	Body    string         `json:"body"`
	Assets  []giteeAsset   `json:"assets"`
	Target  string         `json:"target_commitish"`
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
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 300))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}
	return nil
}

// ListVersions 获取仓库的发布版本列表（按 API 默认顺序，通常最新在前）
func ListVersions(cfg Config) ([]Version, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	query := url.Values{"access_token": []string{cfg.GiteeToken}, "per_page": []string{"20"}}
	var releases []giteeRelease
	if err := apiRequest(fmt.Sprintf("/repos/%s/%s/releases", cfg.GiteeOwner, cfg.GiteeRepo), query, &releases); err != nil {
		return nil, err
	}

	versions := make([]Version, 0, len(releases))
	for _, r := range releases {
		v := Version{TagName: r.TagName, Name: r.Name, Body: r.Body}
		if len(r.Assets) > 0 {
			v.AssetURL = r.Assets[0].BrowserDownloadURL
			v.AssetSHA = extractSHA(r.Body)
		}
		versions = append(versions, v)
	}
	return versions, nil
}

// fetchRelease 按拉取方式获取目标 Release（PullHash 走 zipball，不返回 Release）
func fetchRelease(cfg Config, spec PullSpec) (*giteeRelease, error) {
	switch spec.Kind {
	case PullLatest:
		query := url.Values{"access_token": []string{cfg.GiteeToken}, "per_page": []string{"1"}}
		var releases []giteeRelease
		if err := apiRequest(fmt.Sprintf("/repos/%s/%s/releases", cfg.GiteeOwner, cfg.GiteeRepo), query, &releases); err != nil {
			return nil, err
		}
		if len(releases) == 0 {
			return nil, fmt.Errorf("仓库暂无发布版本")
		}
		return &releases[0], nil
	case PullTag:
		query := url.Values{"access_token": []string{cfg.GiteeToken}}
		var release giteeRelease
		if err := apiRequest(fmt.Sprintf("/repos/%s/%s/releases/tags/%s", cfg.GiteeOwner, cfg.GiteeRepo, url.PathEscape(spec.Value)), query, &release); err != nil {
			return nil, fmt.Errorf("未找到 tag %s 对应的发布: %w", spec.Value, err)
		}
		return &release, nil
	default:
		return nil, fmt.Errorf("哈希拉取走 zipball 通道，不使用 Release")
	}
}

// matchTagByHash 在仓库 tags 中查找 commit 哈希前缀匹配的 tag 名（未命中返回空串）
func matchTagByHash(cfg Config, hash string) (string, error) {
	query := url.Values{"access_token": []string{cfg.GiteeToken}, "per_page": []string{"100"}}
	var tags []giteeTag
	if err := apiRequest(fmt.Sprintf("/repos/%s/%s/tags", cfg.GiteeOwner, cfg.GiteeRepo), query, &tags); err != nil {
		return "", err
	}
	lower := strings.ToLower(hash)
	for _, tag := range tags {
		if strings.HasPrefix(strings.ToLower(tag.Commit.SHA), lower) {
			return tag.Name, nil
		}
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
