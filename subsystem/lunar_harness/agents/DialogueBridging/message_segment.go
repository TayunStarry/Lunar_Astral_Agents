package DialogueBridging

// 消息解析与单线程收发流程（按职责拆分）

import (
	"LunarSubsystem/LunarHarness/clients/napcat"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/mediaobject"
	"encoding/json"
	"strconv"
	"unicode/utf8"
)

// parseMessageSegments 解析消息段列表，返回 (内容, 是否含多模态内容项, 音频对象列表)。
// 群聊与私聊共用本实现，媒体类型只取 NapCat 消息段声明，按新信封逐项打包。
func parseMessageSegments(groupID int64, segments []napcat.MessageSegment) (any, bool, []mediaobject.Object) {
	var contentArray []map[string]any
	var contentStr string
	var hasMedia bool
	var audioObjects []mediaobject.Object

	for _, segment := range segments {
		switch segment.Type {
		case "text":
			var textData napcat.TextData
			if json.Unmarshal(segment.Data, &textData) == nil {
				// 文本保持纯文本，其中的链接抽成 webpage 信封（视频平台声明 web_video）
				if appendTextWithWebpages(&contentArray, &contentStr, groupID, textData.Text) {
					hasMedia = true
				}
			}
		case "at":
			var atData napcat.AtData
			if json.Unmarshal(segment.Data, &atData) == nil {
				appendContent(&contentArray, &contentStr, "[对 "+napcat.ResolveAtDisplay(groupID, atData.QQ)+" 说] ")
			}
		case "reply":
			var replyData napcat.ReplyData
			if json.Unmarshal(segment.Data, &replyData) == nil {
				appendContent(&contentArray, &contentStr, renderReplyQuote(groupID, rawIDString(replyData.ID)))
			}
		case "image":
			var imageData napcat.ImageData
			if json.Unmarshal(segment.Data, &imageData) == nil {
				imgURL := resolveImageURL(imageData)
				if imgURL != "" {
					hasMedia = true
					envelope, payload := buildImageSegmentEnvelope(groupID, imageData, imgURL)
					appendEnvelopeItem(&contentArray, &contentStr, envelope, payload)
				}
			}
		case "mface":
			// 商城表情：新版 NapCat 会转成 image 段，旧版仍以 mface 下发。
			// 有直链/本地路径时按动图处理；否则退回摘要文案，避免整条消息被当成空消息丢弃。
			var mfaceData napcat.MFaceData
			if json.Unmarshal(segment.Data, &mfaceData) == nil {
				if imgURL := resolveMFaceImage(mfaceData); imgURL != "" {
					hasMedia = true
					envelope, payload := buildMFaceEnvelope(groupID, mfaceData, imgURL)
					appendEnvelopeItem(&contentArray, &contentStr, envelope, payload)
				} else {
					appendContent(&contentArray, &contentStr, "[表情: "+mfaceDisplayText(mfaceData)+"] ")
				}
			}
		case "video":
			var videoData napcat.VideoData
			if json.Unmarshal(segment.Data, &videoData) == nil {
				source := resolveVideoSource(videoData)
				if source != "" {
					hasMedia = true
					envelope, payload := buildVideoEnvelope(groupID, videoData, source)
					appendEnvelopeItem(&contentArray, &contentStr, envelope, payload)
				} else {
					name := mediaDisplayName("视频", videoData.File, source)
					appendContent(&contentArray, &contentStr, mediaLinkText(source, name))
				}
			}
		case "record":
			var recordData napcat.RecordData
			if json.Unmarshal(segment.Data, &recordData) == nil {
				source := resolveAudioSource(recordData)
				// 语音在智能体侧统一句柄化并转写（ResourcesConversionHandle），这里只声明对象
				if source != "" {
					audioObjects = append(audioObjects, buildAudioObject(groupID, recordData, source))
				}
				name := mediaDisplayName("语音", recordData.File, source)
				appendContent(&contentArray, &contentStr, name+" ")
			}
		case "face":
			if text := faceDisplayText(segment.Data); text != "" {
				appendContent(&contentArray, &contentStr, "[表情: "+text+"] ")
			} else {
				appendContent(&contentArray, &contentStr, "[表情] ")
			}
		case "json":
			var jsonData napcat.JsonData
			if json.Unmarshal(segment.Data, &jsonData) == nil {
				// 摘要文本始终保留；拿到链接时再补一个 webpage 信封（一卡一信封）
				title := extractJsonCardTitle(jsonData.Data)
				if title != "" {
					appendContent(&contentArray, &contentStr, "[卡片: "+title+"] ")
				} else {
					appendContent(&contentArray, &contentStr, "[卡片] ")
				}
				if envelope, payload := buildCardWebpageEnvelope(groupID, extractJsonCardLink(jsonData.Data), title); envelope != "" {
					hasMedia = true
					appendEnvelopeItem(&contentArray, &contentStr, envelope, payload)
				}
			}
		case "share":
			var shareData napcat.ShareData
			if json.Unmarshal(segment.Data, &shareData) == nil {
				// 摘要文本始终保留；链接只进网页信封，不重复留在文本里
				if shareData.Title != "" {
					appendContent(&contentArray, &contentStr, "[链接: "+shareData.Title+"] ")
				} else {
					appendContent(&contentArray, &contentStr, "[链接] ")
				}
				if envelope, payload := buildCardWebpageEnvelope(groupID, shareData.URL, shareData.Title); envelope != "" {
					hasMedia = true
					appendEnvelopeItem(&contentArray, &contentStr, envelope, payload)
				}
			}
		case "file":
			var fileData napcat.FileData
			if json.Unmarshal(segment.Data, &fileData) == nil {
				fileText, envelope, payload := processFileSegment(groupID, fileData)
				if envelope != "" {
					hasMedia = true
					appendEnvelopeItem(&contentArray, &contentStr, envelope, payload)
				} else {
					appendContent(&contentArray, &contentStr, fileText)
				}
			}
		case "forward":
			var forwardData napcat.ForwardData
			if json.Unmarshal(segment.Data, &forwardData) == nil {
				// 转发内图片已注册进共享图片缓存，占位标签内嵌在文本中（不注入多模态项，避免大转发包撑爆上下文）
				forwardText := processForwardSegment(groupID, rawIDString(forwardData.ID), 0)
				appendContent(&contentArray, &contentStr, forwardText)
			}
		default:
			// 忽略其余消息段类型
		}
	}

	if hasMedia {
		return contentArray, true, audioObjects
	}
	return contentStr, false, audioObjects
}

// renderReplyQuote 通过 get_msg 还原被回复消息，渲染为 [回复 <发送者>: <内容摘要>]
func renderReplyQuote(groupID int64, messageID string) string {
	if messageID == "" {
		return "[回复] "
	}
	detail, err := napcat.GetMessageDetail(messageID)
	if err != nil || detail == nil {
		logger.SubError("LunarCore", "Napcat", "获取回复引用消息失败: %v", err)
		return "[回复] "
	}

	sender := detail.Sender.Card
	if sender == "" {
		sender = detail.Sender.Nickname
	}
	if sender == "" {
		sender = strconv.FormatInt(detail.Sender.UserID, 10)
	}
	segments := detail.Message
	if len(segments) == 0 {
		segments = detail.Content
	}
	summary := extractSegmentText(groupID, 0, segments)
	if summary == "" {
		return "[回复] "
	}
	if utf8.RuneCountInString(summary) > maxReplyQuoteLen {
		summary = string([]rune(summary)[:maxReplyQuoteLen]) + "…"
	}
	return "[回复 " + sender + ": " + summary + "] "
}

// resolveImageURL 获取图片的可访问地址（data URI 或 http 直链）。
//
// 图片通道的字段组合因 NapCat 版本而异，取值顺序按可靠性排列：
//  1. path：NapCat 本地缓存路径，直接读盘，不受 CDN 时效影响；
//  2. url：CDN 直链（会过期），或本身已是 base64 data URI；
//  3. file_id / file：走 get_image 重新拉取，失败再退化 get_file；
//  4. file 恰好是本地路径时直接读盘。
