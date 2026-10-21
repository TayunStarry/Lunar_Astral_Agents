package lunarloop

import (
	persistence "CorePersonality/engine/persistence"
	"CorePersonality/transport/napcat"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/dop251/goja"
)

func (class *Runtime) pushContext(call goja.FunctionCall) goja.Value {
	msgType := call.Argument(0).String()
	content := call.Argument(1).String()
	audio := ""
	if arg := call.Argument(2); arg != nil && !goja.IsUndefined(arg) && !goja.IsNull(arg) {
		audio = arg.String()
	}
	// 第四个参数（可选）：是否为快速应答。QQ 桥接据此转发但不占用应答份额。
	auto := false
	if arg := call.Argument(3); arg != nil && !goja.IsUndefined(arg) && !goja.IsNull(arg) {
		auto = arg.ToBoolean()
	}
	data := PushContextData{
		Type:    msgType,
		Content: content,
		Audio:   audio,
		Auto:    auto,
	}
	PushMessageFunc("context", data)

	return class.runtime.ToValue(true)
}

// pushImage 推送图片（供 JS 运行时调用）
// 用法：pushImage(images)；表情包可选：pushImage(images, true)
// images 为图片 URL 列表（/file/read/... 或 http(s)://），前端与 QQ 适配器都按 URL 消费
// 第二个参数 isSticker 标记图片是否为表情包，前端据此决定角标显示
func (class *Runtime) pushImage(call goja.FunctionCall) goja.Value {
	// 解析图片数组（兼容 []string / []any）
	var images []string
	if arg := call.Argument(0); !goja.IsUndefined(arg) && !goja.IsNull(arg) {
		if exported := arg.Export(); exported != nil {
			for _, v := range exported.([]any) {
				if s, ok := v.(string); ok && s != "" {
					images = append(images, s)
				}
			}
		}
	}
	// 解析表情包标记（可选参数，缺失时按普通图片处理，保持旧版兼容）
	isSticker := false
	if arg := call.Argument(1); !goja.IsUndefined(arg) && !goja.IsNull(arg) {
		isSticker = arg.ToBoolean()
	}
	// 图片推送即登记：把生成目录中的作品纳入句柄体系，并按内容哈希唯一性
	// 归一到最终保留路径（根目录重复件会被舍弃，URL 同步改写）
	for i, img := range images {
		images[i] = registerPushedImage(img)
	}

	data := PushImageData{
		Type:    "image",
		Images:  images,
		Sticker: isSticker,
	}
	PushMessageFunc("image", data)
	// 把规范化后的 URL 返回给 JS，近期记忆必须与广播使用同一份地址
	return class.runtime.ToValue(images)
}

// registerPushedImage 把待推送图片的服务路径登记进句柄体系，返回规范化后的 URL
//
// 只处理本机内容仓库路径（/file/read/... 或 multimedia/...）；http(s) 与 data URI 原样返回。
// 若同哈希已存在于优先级更高的路径，RegisterPath 会返回该路径（新副本已被清理），
// 因此这里改写 URL 后，前端与 QQ 适配器拿到的始终是有效实体。
func registerPushedImage(raw string) string {
	const readPrefix = "/file/read/"
	rel := ""
	switch {
	case strings.HasPrefix(raw, readPrefix):
		rel = strings.TrimPrefix(raw, readPrefix)
	case strings.HasPrefix(raw, "multimedia/"), strings.HasPrefix(raw, "audios/"):
		rel = raw
	default:
		return raw
	}
	if decoded, err := url.PathUnescape(rel); err == nil {
		rel = decoded
	}
	canonical, _, err := persistence.RegisterPath(rel)
	// 兜底：canonical 必须是内容仓库路径。句柄（@I.00x）一旦混进 /file/read/，
	// 前端与 QQ 适配器都会拿到 404（QQ 侧表现为「下载文件失败: Not Found」）。
	// 这种情况下退回原始 URL —— 实体在我们登记前已 stat 存在，原始路径通常仍可读。
	if err != nil || canonical == "" || strings.HasPrefix(canonical, "@") {
		return raw
	}
	return readPrefix + canonical
}

// NewVideoUrlContent 创建视频URL内容项（入统一未读队列）
func NewVideoUrlContent(url string) MediaUrlContent {
	return MediaUrlContent{Type: "video_url", VideoUrl: &MediaUrlPayload{URL: url}}
}

// NewAudioUrlContent 创建音频URL内容项（入统一未读队列）
func NewAudioUrlContent(url string) MediaUrlContent {
	return MediaUrlContent{Type: "audio_url", AudioUrl: &MediaUrlPayload{URL: url}}
}

// EnqueueMessage 将消息按到达顺序追加到统一未读队列
// napcat 回调与 HTTP handler 跨 goroutine 入队，互斥保护
func EnqueueMessage(messages ...PostMessage) {
	unreadMutex.Lock()
	defer unreadMutex.Unlock()
	UnreadContext = append(UnreadContext, messages...)
}

// EnqueueMediaURL 将视频/音频URL包装为媒体内容项，按到达顺序追加到统一未读队列
// kind 为 "video_url"（视频）或 "audio_url"（音频）；source 标记消息来源（如 qq）
func EnqueueMediaURL(kind string, url string, source string) {
	if kind == "video_url" {
		EnqueueMessage(PostMessage{Role: string(RoleUser), Content: NewVideoUrlContent(url), Source: source})
		return
	}
	EnqueueMessage(PostMessage{Role: string(RoleUser), Content: NewAudioUrlContent(url), Source: source})
}

// TakeUnreadContext 拉取并清空统一未读队列（互斥保护；队列为空时返回 nil）
func TakeUnreadContext() []PostMessage {
	unreadMutex.Lock()
	defer unreadMutex.Unlock()
	if len(UnreadContext) == 0 {
		return nil
	}
	messages := append([]PostMessage{}, UnreadContext...)
	UnreadContext = make([]PostMessage, 0)
	return messages
}

// UnreadCount 返回统一未读队列当前长度（互斥保护，供 HTTP 响应使用）
func UnreadCount() int {
	unreadMutex.Lock()
	defer unreadMutex.Unlock()
	return len(UnreadContext)
}

// unreadCount 绑定：unreadCount() → 宿主统一未读队列长度（只读，不消费）
//
// 生活链调度用：必须查队列长度而不是调 pullContext()——后者会把消息取走，
// 等于替对话链消费了本轮消息。
func (class *Runtime) unreadCount(goja.FunctionCall) goja.Value {
	return class.runtime.ToValue(UnreadCount())
}

// pullContext 拉取上下文消息（统一时序队列：文本与视频/音频URL混排，拉取即清空）
func (class *Runtime) pullContext() goja.Value {
	// 拉取全部未读消息
	messages := TakeUnreadContext()
	// 如果未处理的上下文消息为空，返回空数组
	if len(messages) == 0 {
		// 通知桥接器：智能体本轮无待处理消息（用于判断上一轮QQ对话是否已回应完）
		napcat.NotifyAgentIdle()
		return class.runtime.ToValue([]PostMessage{})
	}
	ctxJson, _ := json.Marshal(messages)
	// 定义响应格式
	var response []any
	// 将JSON字符串解析为响应格式
	json.Unmarshal(ctxJson, &response)
	// 返回拉取到的上下文消息
	return class.runtime.ToValue(response)
}

// getAgentPosition 获取缓存的智能体3D位置（由前端遥测数据更新）
func (class *Runtime) getAgentPosition() goja.Value {
	agentPositionMutex.RLock()
	defer agentPositionMutex.RUnlock()
	return class.runtime.ToValue(map[string]any{
		"x": agentPosition.X,
		"y": agentPosition.Y,
		"z": agentPosition.Z,
	})
}

// UpdateAgentPosition 供 HTTP handler 调用，更新缓存的智能体位置
// 该函数不在 JS 运行时中导出，仅由 Go 端调用
func UpdateAgentPosition(x, y, z float64) {
	agentPositionMutex.Lock()
	defer agentPositionMutex.Unlock()
	agentPosition.X = x
	agentPosition.Y = y
	agentPosition.Z = z
}
