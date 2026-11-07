package mediaobject

// 入站多模态信封类型（发送侧声明，后端只做强制校验）
//
// 每类信封只有一份专用载荷字段，禁止共用、禁止后端代偿识别。
const (
	EnvelopeImageStatic   = "image_static"
	EnvelopeImageAnimated = "image_animated"
	EnvelopeVideo         = "video"
	EnvelopeAudio         = "audio"
	EnvelopeFile          = "file"
	EnvelopeWebpage       = "webpage"
)

// 旧内容项信封（仅服务端内部清洗产物 / 旧发送侧兼容，不接受入站）
const (
	EnvelopeText        = "text"
	EnvelopeImageURL    = "image_url"
	EnvelopeVideoURL    = "video_url"
	EnvelopeAudioURL    = "audio_url"
	EnvelopeInputAudio  = "input_audio"
	EnvelopeAudioHandle = "audio_handle"
	EnvelopeQuote       = "quote"
)

// 网页信封的平台声明
const (
	// WebPlatformPage 普通网页，转写为句柄 U
	WebPlatformPage = "web_page"
	// WebPlatformVideo 视频平台页面，转写为句柄 V
	WebPlatformVideo = "web_video"
)

// VideoPlatformHosts 共享视频平台域名表。
//
// 发送侧声明 web_video 与后端校验使用同一份数据，不允许在别处再维护第二份。
var VideoPlatformHosts = []string{
	"bilibili.com", "b23.tv", "youtube.com", "youtu.be", "vimeo.com",
	"nicovideo.jp", "v.qq.com", "youku.com", "iqiyi.com", "mgtv.com",
}

// 多模态对象的语义类型
const (
	// TypeImageStatic 静态图片（jpg / png / 单帧 webp 等）
	TypeImageStatic = "image_static"
	// TypeImageAnimated 动图（QQ 表情、gif / apng 等）
	TypeImageAnimated = "image_animated"
	// TypeVideo 视频
	TypeVideo = "video"
	// TypeAudio 音频（内联或链接）
	TypeAudio = "audio"
)

// 多模态对象的来源
const (
	// SourceQQGroup QQ 群聊消息段
	SourceQQGroup = "qq_group"
	// SourceQQPrivate QQ 私聊消息段
	SourceQQPrivate = "qq_private"
	// SourceClientUpload HTTP/HTTPS 前端声明并上传的媒体
	SourceClientUpload = "client_upload"
)

// 常见扩展名对应的 MIME（来源方声明扩展名时用于补全 MIME）
var mimeByExtension = map[string]string{
	"jpg": "image/jpeg", "jpeg": "image/jpeg", "png": "image/png",
	"gif": "image/gif", "webp": "image/webp", "bmp": "image/bmp",
	"svg": "image/svg+xml", "avif": "image/avif", "heic": "image/heic", "heif": "image/heif",
	"mp4": "video/mp4", "mov": "video/quicktime", "avi": "video/x-msvideo",
	"mkv": "video/x-matroska", "flv": "video/x-flv", "wmv": "video/x-ms-wmv",
	"m4v": "video/x-m4v", "mpeg": "video/mpeg", "mpg": "video/mpeg", "3gp": "video/3gpp",
	"mp3": "audio/mpeg", "wav": "audio/wav", "ogg": "audio/ogg", "oga": "audio/ogg",
	"m4a": "audio/mp4", "aac": "audio/aac", "flac": "audio/flac", "opus": "audio/opus",
	"amr": "audio/amr", "wma": "audio/x-ms-wma", "aiff": "audio/aiff", "webm": "audio/webm",
	"silk": "audio/silk",
}

// 具备动画能力的图片 MIME
var animatedImageMIMEs = map[string]bool{
	"image/gif":  true,
	"image/apng": true,
}
