package adapters

import (
	"encoding/json"
	"logger"

	"github.com/dop251/goja"
)

// MusicRenderFunc 音乐渲染回调函数，由 server/handlers 包初始化时注册
// 参数：abcNotation ABC乐谱, title 标题
// 返回：audioURL 音频URL, fileName 文件名, err 错误
var MusicRenderFunc func(abcNotation string, title string) (audioURL string, fileName string, err error)

func init() {
	MusicRenderFunc = func(abcNotation string, title string) (string, string, error) {
		return "", "", nil // 默认空实现，渲染功能由 handlers 包注册
	}
}

func (class *Runtime) pushContext(msgType string, content string, audio string) goja.Value {
	data := PushContextData{
		Type:    msgType,
		Content: content,
		Audio:   audio,
	}
	PushMessageFunc("context", data)

	// 拦截音乐类型消息：当 ABC 乐谱推送到前端时，
	// 自动触发后端 FluidSynth + SoundFont 音频渲染管线
	if msgType == "music" {
		go renderMusicAudio(content)
	}

	return class.runtime.ToValue(true)
}

// renderMusicAudio 异步渲染音乐音频
func renderMusicAudio(abcNotation string) {
	if abcNotation == "" {
		return
	}

	// 提取标题
	title := extractABCTitle(abcNotation)

	// 调用注册的渲染函数
	audioURL, fileName, err := MusicRenderFunc(abcNotation, title)
	if err != nil {
		logger.Error("LunarCore", "音乐音频渲染失败: %v", err)
		return
	}
	if audioURL == "" {
		// 渲染函数未注册或依赖不满足，静默跳过
		return
	}

	// 将音频 URL 推送到前端
	logger.Info("LunarCore", "音乐音频渲染完成: %s", audioURL)
	audioData, _ := json.Marshal(map[string]string{
		"type":      "audio_ready",
		"audio_url": audioURL,
		"file_name": fileName,
	})
	PushMessageFunc("context", PushContextData{
		Type:    "music_audio",
		Content: string(audioData),
	})
}

// extractABCTitle 从 ABC 乐谱中提取标题
func extractABCTitle(abcNotation string) string {
	for _, line := range splitLines(abcNotation) {
		if len(line) > 2 && line[:2] == "T:" {
			return trimSpace(line[2:])
		}
	}
	return "music"
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func (class *Runtime) pushImage(images []string) goja.Value {
	data := PushImageData{
		Type:   "image",
		Images: images,
	}
	PushMessageFunc("image", data)
	return class.runtime.ToValue(true)
}

// pullContext 拉取上下文消息
func (class *Runtime) pullContext() goja.Value {
	// 如果未处理的上下文消息为空，返回空数组
	if len(UnreadContext) == 0 {
		return class.runtime.ToValue([]PostMessage{})
	}
	ctxJson, _ := json.Marshal(UnreadContext)
	// 清空未处理的上下文消息
	UnreadContext = make([]PostMessage, 0)
	// 定义响应格式
	var response []any
	// 将JSON字符串解析为响应格式
	json.Unmarshal(ctxJson, &response)
	// 返回拉取到的上下文消息
	return class.runtime.ToValue(response)
}

// pullVideoUrl 拉取视频URL
func (class *Runtime) pullVideoUrl() goja.Value {
	// 如果未处理的视频URL为空，返回空数组
	if len(UnreadVideoUrl) == 0 {
		return class.runtime.ToValue([]string{})
	}
	// 拷贝未处理的视频URL
	url := append([]string{}, UnreadVideoUrl...)
	// 清空未处理的视频URL
	UnreadVideoUrl = make([]string, 0)
	// 返回拉取到的视频URL
	return class.runtime.ToValue(url)
}

// getAgentPosition 获取缓存的智能体3D位置（由前端遥测数据更新）
func (class *Runtime) getAgentPosition() goja.Value {
	agentPositionMutex.RLock()
	defer agentPositionMutex.RUnlock()
	return class.runtime.ToValue(map[string]interface{}{
		"x": agentPosition.X,
		"y": agentPosition.Y,
		"z": agentPosition.Z,
	})
}

// pushAgentEvent 将 3D 引擎事件推送到 AI 上下文
// eventType: 事件类型（如 "movement_complete", "action_started"）
// data: 事件数据的 JSON 字符串
func (class *Runtime) pushAgentEvent(eventType string, data string) goja.Value {
	message := PostMessage{
		Role: "system",
		Content: map[string]string{
			"type":    "agent_event",
			"event":   eventType,
			"data":    data,
		},
	}
	UnreadContext = append(UnreadContext, message)
	logger.Info("LunarCore", "[智能体事件] %s: %s", eventType, data)
	return class.runtime.ToValue(true)
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

// PushAgentEventToContext 供 HTTP handler 调用，将引擎事件推送到 AI 上下文
// 该函数不在 JS 运行时中导出，仅由 Go 端调用
func PushAgentEventToContext(eventType string, data string) {
	message := PostMessage{
		Role: "system",
		Content: map[string]string{
			"type":  "agent_event",
			"event": eventType,
			"data":  data,
		},
	}
	UnreadContext = append(UnreadContext, message)
	logger.Info("LunarCore", "[智能体事件] %s: %s", eventType, data)
}
