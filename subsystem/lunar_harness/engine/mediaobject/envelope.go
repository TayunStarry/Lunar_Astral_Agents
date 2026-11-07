package mediaobject

// envelope.go 入站多模态信封：发送侧声明类型，后端只做强制校验。
//
// 六类媒体信封各有一份专用载荷字段，禁止共用一个字段承载多种类型，
// 也禁止后端根据 URL、主机名或扩展名代偿识别。

import (
	"fmt"
	"strings"
)

// Ref 信封的公共引用字段：来源、URL 或本地路径、声明 MIME、名字。
//
// 各信封载荷独立持有本结构，传输时由各自专用字段承载。
type Ref struct {
	// Source 来源（见 Source* 常量）
	Source string `json:"source"`
	// URL 远程引用（http/https/file/data）
	URL string `json:"url,omitempty"`
	// Path 本地路径引用
	Path string `json:"path,omitempty"`
	// MIME 来源方声明的媒体类型
	MIME string `json:"mime,omitempty"`
	// Name 来源方声明的名字
	Name string `json:"name,omitempty"`
}

// ImageStaticPayload 静态图信封载荷
type ImageStaticPayload struct {
	Ref
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
}

// ImageAnimatedPayload 动图信封载荷
type ImageAnimatedPayload struct {
	Ref
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
}

// VideoPayload 视频信封载荷
type VideoPayload struct {
	Ref
	DurationMS int64 `json:"duration_ms,omitempty"`
}

// AudioPayload 音频信封载荷
type AudioPayload struct {
	Ref
	DurationMS int64 `json:"duration_ms,omitempty"`
}

// FilePayload 文件信封载荷
type FilePayload struct {
	Ref
	Bytes int64 `json:"bytes,omitempty"`
}

// WebpagePayload 网页信封载荷
type WebpagePayload struct {
	Ref
	// Platform 平台声明：web_page（缺省）或 web_video
	Platform string `json:"platform,omitempty"`
}

// Envelope 一个入站内容项。
//
// Type 决定唯一合法的专用载荷字段；其余载荷字段必须为空。
type Envelope struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`

	ImageStatic   *ImageStaticPayload   `json:"image_static,omitempty"`
	ImageAnimated *ImageAnimatedPayload `json:"image_animated,omitempty"`
	Video         *VideoPayload         `json:"video,omitempty"`
	Audio         *AudioPayload         `json:"audio,omitempty"`
	File          *FilePayload          `json:"file,omitempty"`
	Webpage       *WebpagePayload       `json:"webpage,omitempty"`

	// Quote 引用快照（非媒体控制项，服务端内部使用）
	Quote *QuotePayload `json:"quote,omitempty"`
}

// QuotePayload 引用快照的载荷
type QuotePayload struct {
	MessageID string `json:"message_id,omitempty"`
	Sender    string `json:"sender,omitempty"`
	Text      string `json:"text,omitempty"`
}

// Declared 返回信封声明的类型与专用引用载荷。
//
// 只允许恰好一个专用载荷，且必须与 Type 同名；否则判定信封无效。
func (e *Envelope) Declared() (string, *Ref, error) {
	if e == nil {
		return "", nil, fmt.Errorf("信封为空")
	}
	type found struct {
		kind string
		ref  *Ref
	}
	var hits []found
	add := func(kind string, ref *Ref) {
		if ref != nil {
			hits = append(hits, found{kind: kind, ref: ref})
		}
	}
	if e.ImageStatic != nil {
		add(EnvelopeImageStatic, &e.ImageStatic.Ref)
	}
	if e.ImageAnimated != nil {
		add(EnvelopeImageAnimated, &e.ImageAnimated.Ref)
	}
	if e.Video != nil {
		add(EnvelopeVideo, &e.Video.Ref)
	}
	if e.Audio != nil {
		add(EnvelopeAudio, &e.Audio.Ref)
	}
	if e.File != nil {
		add(EnvelopeFile, &e.File.Ref)
	}
	if e.Webpage != nil {
		add(EnvelopeWebpage, &e.Webpage.Ref)
	}

	declared := strings.TrimSpace(e.Type)
	if len(hits) == 0 {
		return "", nil, fmt.Errorf("信封 %q 缺少专用载荷", declared)
	}
	if len(hits) > 1 {
		return "", nil, fmt.Errorf("信封 %q 同时携带多个专用载荷", declared)
	}
	if declared != hits[0].kind {
		return "", nil, fmt.Errorf("信封类型 %q 与载荷 %q 不一致", declared, hits[0].kind)
	}
	return hits[0].kind, hits[0].ref, nil
}

// ValidateEnvelope 校验信封结构与声明自洽性。
//
// 不做任何类型推断：类型、来源、MIME、网页平台全部由发送侧声明，
// 后端只判断声明之间是否互相矛盾。
func ValidateEnvelope(e *Envelope) (string, *Ref, error) {
	kind, ref, err := e.Declared()
	if err != nil {
		return "", nil, err
	}
	if err := validateRef(kind, ref); err != nil {
		return "", nil, err
	}
	if err := validateMeta(e); err != nil {
		return "", nil, err
	}
	if kind == EnvelopeWebpage {
		platform := strings.TrimSpace(e.Webpage.Platform)
		switch platform {
		case "":
			platform = WebPlatformPage
		case WebPlatformPage, WebPlatformVideo:
		default:
			return "", nil, fmt.Errorf("未知网页平台声明 %q", e.Webpage.Platform)
		}
		videoPlatform := IsVideoPlatformURL(ref.URL)
		if platform == WebPlatformVideo && !videoPlatform {
			return "", nil, fmt.Errorf("声明为 web_video，但主机不在共享视频平台域名表内")
		}
		if platform == WebPlatformPage && videoPlatform {
			return "", nil, fmt.Errorf("声明为 web_page，但主机属于视频平台；请改声明为 web_video")
		}
	}
	return kind, ref, nil
}

// Reference 返回信封实际引用（URL 优先，其次本地路径）
func (r *Ref) Reference() string {
	if r == nil {
		return ""
	}
	if strings.TrimSpace(r.URL) != "" {
		return strings.TrimSpace(r.URL)
	}
	return strings.TrimSpace(r.Path)
}

func validateRef(kind string, ref *Ref) error {
	if ref == nil {
		return fmt.Errorf("缺少引用载荷")
	}
	if !isKnownSource(ref.Source) {
		return fmt.Errorf("未知来源 %q", ref.Source)
	}
	urlRef := strings.TrimSpace(ref.URL)
	pathRef := strings.TrimSpace(ref.Path)
	if urlRef != "" && pathRef != "" {
		return fmt.Errorf("url 与 path 只能二选一")
	}
	if urlRef == "" && pathRef == "" {
		return fmt.Errorf("缺少媒体引用 url 或 path")
	}
	if kind == EnvelopeWebpage {
		if pathRef != "" {
			return fmt.Errorf("网页信封只接受 url")
		}
		lower := strings.ToLower(urlRef)
		if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
			return fmt.Errorf("网页信封只接受 http/https 链接")
		}
	}
	if urlRef != "" {
		if idx := strings.Index(urlRef, "://"); idx >= 0 {
			switch strings.ToLower(urlRef[:idx]) {
			case "http", "https", "file":
			default:
				return fmt.Errorf("不支持的引用协议 %q", urlRef[:idx])
			}
		} else if !strings.HasPrefix(strings.ToLower(urlRef), "data:") {
			return fmt.Errorf("无法识别的引用 %q", urlRef)
		}
	}
	return validateMIMEFamily(kind, ref.MIME)
}

func validateMIMEFamily(kind, raw string) error {
	mimeType := strings.ToLower(strings.TrimSpace(strings.SplitN(raw, ";", 2)[0]))
	if mimeType == "" {
		return nil
	}
	switch kind {
	case EnvelopeImageStatic, EnvelopeImageAnimated:
		if !strings.HasPrefix(mimeType, "image/") {
			return fmt.Errorf("信封 %q 的 MIME %q 不是图片", kind, raw)
		}
	case EnvelopeVideo:
		if !strings.HasPrefix(mimeType, "video/") {
			return fmt.Errorf("信封 %q 的 MIME %q 不是视频", kind, raw)
		}
	case EnvelopeAudio:
		if !strings.HasPrefix(mimeType, "audio/") {
			return fmt.Errorf("信封 %q 的 MIME %q 不是音频", kind, raw)
		}
	case EnvelopeWebpage:
		if mimeType != "text/html" && mimeType != "application/xhtml+xml" {
			return fmt.Errorf("网页信封的 MIME %q 不是 html", raw)
		}
	}
	return nil
}

func validateMeta(e *Envelope) error {
	if e.ImageStatic != nil && (e.ImageStatic.Width < 0 || e.ImageStatic.Height < 0) {
		return fmt.Errorf("图片尺寸不能为负数")
	}
	if e.ImageAnimated != nil && (e.ImageAnimated.Width < 0 || e.ImageAnimated.Height < 0) {
		return fmt.Errorf("图片尺寸不能为负数")
	}
	if e.Video != nil && e.Video.DurationMS < 0 {
		return fmt.Errorf("视频时长不能为负数")
	}
	if e.Audio != nil && e.Audio.DurationMS < 0 {
		return fmt.Errorf("音频时长不能为负数")
	}
	if e.File != nil && e.File.Bytes < 0 {
		return fmt.Errorf("文件大小不能为负数")
	}
	return nil
}

func isKnownSource(source string) bool {
	switch source {
	case SourceQQGroup, SourceQQPrivate, SourceClientUpload:
		return true
	default:
		return false
	}
}
