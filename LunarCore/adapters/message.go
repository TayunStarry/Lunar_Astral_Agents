package adapters

import (
	"encoding/json"

	"github.com/dop251/goja"
)

// 未处理的上下文消息
var UnreadContext = make([]PostMessage, 0)

// 未处理的视频URL
var UnreadVideoUrl = make([]string, 0)

// pullContext 拉取上下文消息
func (class *Adapters) pullContext() goja.Value {
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
func (class *Adapters) pullVideoUrl() goja.Value {
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
