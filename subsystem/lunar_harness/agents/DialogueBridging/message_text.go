package DialogueBridging

// 消息解析与单线程收发流程（按职责拆分）

import (
	"LunarSubsystem/LunarHarness/clients/napcat"
	"encoding/json"
	"strconv"
	"strings"
)

func rawIDString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var n int64
	if json.Unmarshal(raw, &n) == nil {
		return strconv.FormatInt(n, 10)
	}
	return ""
}

// extractSegmentText 从消息段列表中提取文本摘要（回复引用与合并转发子消息共用）。
// 图片、商城表情、视频、语音与文件都会先尝试收进资源归档层并写成规范句柄
// （[@I.00x] / [@U.00x] / [@D.00x]），入库失败才退化为文件名，
// 保证引用里的媒体仍可被后续回引，也不会出现无信息的占位文本。
// 语音只带文件名与地址：句柄化与转写由对话者取未读上下文时统一完成。
func extractSegmentText(groupID int64, depth int, segments []napcat.MessageSegment) string {
	var sb strings.Builder
	for _, segment := range segments {
		switch segment.Type {
		case "text":
			var textData napcat.TextData
			if json.Unmarshal(segment.Data, &textData) == nil {
				sb.WriteString(textData.Text)
			}
		case "at":
			var atData napcat.AtData
			if json.Unmarshal(segment.Data, &atData) == nil {
				sb.WriteString("[对 ")
				sb.WriteString(napcat.ResolveAtDisplay(groupID, atData.QQ))
				sb.WriteString(" 说] ")
			}
		case "image":
			var imageData napcat.ImageData
			if json.Unmarshal(segment.Data, &imageData) == nil {
				if handle, ok := admitInboundImage(imageData); ok {
					sb.WriteString("[")
					sb.WriteString(handle)
					sb.WriteString("]")
				} else {
					sb.WriteString(mediaDisplayName("图片", imageData.File, imageData.URL, imageData.Path))
				}
			}
		case "video":
			var videoData napcat.VideoData
			if json.Unmarshal(segment.Data, &videoData) == nil {
				source := resolveVideoSource(videoData)
				name := mediaDisplayName("视频", videoData.File, source)
				sb.WriteString(mediaLinkText(source, name))
			}
		case "record":
			var recordData napcat.RecordData
			if json.Unmarshal(segment.Data, &recordData) == nil {
				source := resolveAudioSource(recordData)
				name := mediaDisplayName("语音", recordData.File, source)
				sb.WriteString(name)
				sb.WriteString(" ")
			}
		case "file":
			var fileData napcat.FileData
			if json.Unmarshal(segment.Data, &fileData) == nil {
				// 引用摘要只取文本描述：媒体文件按声明名展示，文档仍是句柄文本
				text, _, _ := processFileSegment(groupID, fileData)
				sb.WriteString(text)
			}
		case "face":
			if text := faceDisplayText(segment.Data); text != "" {
				sb.WriteString("[表情: ")
				sb.WriteString(text)
				sb.WriteString("]")
			} else {
				sb.WriteString("[表情]")
			}
		case "mface":
			var mfaceData napcat.MFaceData
			if json.Unmarshal(segment.Data, &mfaceData) == nil {
				registered := false
				if handle, ok := admitInboundImage(napcat.ImageData{URL: mfaceData.URL, Path: mfaceData.Path}); ok {
					sb.WriteString("[")
					sb.WriteString(handle)
					sb.WriteString("]")
					registered = true
				}
				if !registered {
					sb.WriteString("[表情: ")
					sb.WriteString(mfaceDisplayText(mfaceData))
					sb.WriteString("]")
				}
			} else {
				sb.WriteString("[表情]")
			}
		case "forward":
			var forwardData napcat.ForwardData
			if json.Unmarshal(segment.Data, &forwardData) == nil {
				sb.WriteString(processForwardSegment(groupID, rawIDString(forwardData.ID), depth+1))
			}
		case "json":
			sb.WriteString("[卡片]")
		case "share":
			sb.WriteString("[链接]")
		}
	}
	return sb.String()
}

// appendContent 根据当前内容格式追加文本
// 如果已经是数组格式（有图片），追加为 text 类型元素；否则追加到纯字符串
func appendContent(contentArray *[]map[string]any, contentStr *string, text string) {
	if len(*contentArray) > 0 {
		*contentArray = append(*contentArray, map[string]any{
			"type": "text",
			"text": text,
		})
	} else {
		*contentStr += text
	}
}

// markMultimedia 首次遇到多媒体时将已累积的纯文本迁移到数组格式
func markMultimedia(contentArray *[]map[string]any, contentStr *string) {
	if len(*contentArray) == 0 && *contentStr != "" {
		*contentArray = append(*contentArray, map[string]any{
			"type": "text",
			"text": *contentStr,
		})
		*contentStr = ""
	}
}
