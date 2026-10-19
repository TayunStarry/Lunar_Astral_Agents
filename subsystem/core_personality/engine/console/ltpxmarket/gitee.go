package ltpxmarket

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"CorePersonality/engine/console/channel"
)

// giteeRelease Gitee Release API 响应（仅解析用到的字段）
type giteeRelease struct {
	ID        int64        `json:"id"`
	TagName   string       `json:"tag_name"`
	Name      string       `json:"name"`
	Body      string       `json:"body"`
	CreatedAt string       `json:"created_at"`
	Assets    []giteeAsset `json:"assets"`
}

// giteeAsset Release 附件
type giteeAsset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// apiRequest 执行 Gitee API GET 请求并解析 JSON（令牌经 query 携带，可匿名访问公开仓库）
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
		// 返回可分类的接口错误（限流/鉴权/权限提示不同，见 channel/api.go）
		return &channel.APIError{Status: resp.StatusCode, Path: path, Body: truncate(string(body), 300)}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}
	return nil
}

// postJSON 执行 POST 请求并返回响应体（令牌置于 payload 内）
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

// deleteJSON 执行 DELETE 请求（删除 Release / tag 等写操作，令牌经 query 携带）
func deleteJSON(path string) error {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodDelete, giteeAPIBase+path, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(data), 300))
	}
	return nil
}

// uploadAsset 上传附件到 Release（multipart 流式上传；filename 为附件呈现名，
// 可与磁盘文件名不同——上传已打包 .ltpx 时以规范名 ltpx_<id>.ltpx 上传原文件；
// base/span 为进度映射区间，如 30/70 表示附件上传占整体进度的 30-100）
func uploadAsset(cfg Config, releaseID int64, filePath, filename string, base, span int) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	uploadURL := fmt.Sprintf("%s/repos/%s/%s/releases/%d/attach_files?access_token=%s",
		giteeAPIBase, cfg.GiteeOwner, cfg.GiteeRepo, releaseID, url.QueryEscape(cfg.GiteeToken))

	pr := &progressReader{r: f, total: fileSize(filePath), base: base, span: span}
	body := &multipartBody{field: "file", filename: filename, reader: pr}
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

// downloadFile 下载远程文件（token 可空；total>0 时上报进度，映射至 0-55 区间）
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
				reportProgress(int(sent * 55 / total)) // 下载占整体进度 0-55
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

// progressReader 边读边上报进度（base+span：把 0-100 的读取进度映射到 base 至 base+span 区间）
type progressReader struct {
	r     io.Reader
	total int64
	sent  int64
	base  int
	span  int
}

func (p *progressReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	if n > 0 && p.total > 0 {
		p.sent += int64(n)
		reportProgress(p.base + int(p.sent*100/p.total)*p.span/100)
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
		m.boundary = "lunarLtpxMarketBoundary9f3e1c"
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

// truncate 截断文本用于错误信息
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
