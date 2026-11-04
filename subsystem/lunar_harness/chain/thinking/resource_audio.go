package thinking

// resource_audio.go 音频句柄在对话上下文中的呈现。
//
// 转码 / ASR 转写 / 悬空句柄登记 / 转写文本读取已上移到 engine/resource：
//   - resource.IngestAudio：接入阶段转码、转写并登记 @A 句柄；
//   - resource.UnloadASRModel：批量转写结束后卸载 ASR；
//   - resource.ParseTextOfResource：按句柄读取转写文本。
//
// 本文件只保留对话协议形状：ContentAudioHandle 内容项与「@A.nnn（文本）」内联格式。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/resource"
)

// audioHandleContextText 音频句柄进入模型上下文时的统一文本形态。
// 解析文本优先；句柄只作为可追溯的描述标识，不读取任何音频字节。
func audioHandleContextText(handle, fallback string) string {
	handle = strings.TrimSpace(handle)
	text := strings.TrimSpace(fallback)
	if text == "" {
		text = resource.ParseTextOfResource(handle)
	}
	if text == "" {
		text = resource.ASRFallbackText
	}
	if handle == "" {
		return text
	}
	return fmt.Sprintf("%s（%s）", handle, text)
}

// audioHandleContextItem 构造音频句柄内容项（对话消息协议形状）
func audioHandleContextItem(handle, text, name string) ContentItem {
	return ContentItem{Type: ContentAudioHandle, AudioHandle: &AudioHandlePayload{
		Handle: handle,
		Text:   text,
		Name:   name,
	}}
}
