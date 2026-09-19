package engine

import (
	"LunarAstral/bridging/napcat"
	"encoding/json"

	"github.com/dop251/goja"
)

func (class *Runtime) pushContext(msgType string, content string, audio string) goja.Value {
	data := PushContextData{
		Type:    msgType,
		Content: content,
		Audio:   audio,
	}
	PushMessageFunc("context", data)

	return class.runtime.ToValue(true)
}

// pushImage 推送图片（供 JS 运行时调用）
// 用法：pushImage(images)；表情包可选：pushImage(images, true)
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

	data := PushImageData{
		Type:    "image",
		Images:  images,
		Sticker: isSticker,
	}
	PushMessageFunc("image", data)
	return class.runtime.ToValue(true)
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
// kind 为 "video_url"（视频）或 "audio_url"（音频）
func EnqueueMediaURL(kind string, url string) {
	if kind == "video_url" {
		EnqueueMessage(PostMessage{Role: string(RoleUser), Content: NewVideoUrlContent(url)})
		return
	}
	EnqueueMessage(PostMessage{Role: string(RoleUser), Content: NewAudioUrlContent(url)})
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
