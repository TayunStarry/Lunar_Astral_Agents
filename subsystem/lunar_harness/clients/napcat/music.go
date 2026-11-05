package napcat

// music.go QQ 演奏音频出站
//
// 演奏音频由 fluidsynth 渲染为 44.1kHz 双声道 WAV（约 7MB / 40 秒）。
// NapCat 的 record 段（OneBot voice）在发送前会自行把非 silk 音频转成 NTSilk
// 并读取时长，因此这里不额外引入 ffmpeg / silk 编码器：直接把本机路径交给
// NapCat，优先发语音条；语音条失败时降级为「文字说明 + 音频文件」，若文件也
// 发不出去则保留文字里的音频地址并明确报错，绝不静默丢弃。

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// SendGroupRecordMessage 发送群语音条（音频转换与时长读取由 NapCat 完成）
func SendGroupRecordMessage(groupID int64, audio string) error {
	return sendRecordMessage("/send_group_msg", map[string]any{"group_id": groupID}, audio)
}

// SendPrivateRecordMessage 发送私聊语音条
func SendPrivateRecordMessage(userID int64, audio string) error {
	return sendRecordMessage("/send_private_msg", map[string]any{"user_id": userID}, audio)
}

// sendRecordMessage 组装 record 段并调用现有发送接口。
// 语音条需要 NapCat 先把 WAV 转成 silk，7MB / 40 秒的演奏可能超过普通接口的
// 10 秒超时，因此复用文件上传的长超时客户端。
func sendRecordMessage(action string, target map[string]any, audio string) error {
	source, ok := audioMediaSource(audio)
	if !ok {
		return fmt.Errorf("演奏音频不可读: %s", strings.TrimSpace(audio))
	}
	params := map[string]any{
		"message": []map[string]any{
			{"type": "record", "data": map[string]string{"file": source}},
		},
	}
	for key, value := range target {
		params[key] = value
	}
	client := Default()
	_, err := client.callAPIWithClient(action, params, client.uploadHTTPClient)
	return err
}

// SendMusic 把演奏音频投递到 QQ：语音条 → 文字 + 文件 → 文字 + 链接。
func SendMusic(isGroup bool, targetID int64, audio, title, summary string, duration float64) error {
	source, ok := audioMediaSource(audio)
	if !ok {
		text := musicCaption(title, summary, duration) + "\n演奏音频不可用，请在前端查看。"
		if err := sendMusicText(isGroup, targetID, text); err != nil {
			return fmt.Errorf("音频不可读且文字通知失败: %w", err)
		}
		return fmt.Errorf("演奏音频不可读: %s", strings.TrimSpace(audio))
	}

	size := fileSizeOf(source)
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(source)), ".")
	if format == "" {
		format = "未知"
	}

	start := time.Now()
	var recordErr error
	if isGroup {
		recordErr = SendGroupRecordMessage(targetID, audio)
	} else {
		recordErr = SendPrivateRecordMessage(targetID, audio)
	}
	if recordErr == nil {
		logger.SubInfo("LunarCore", "Napcat",
			"QQ 音乐以语音条发送成功: 《%s》 源格式=%s 源大小=%.2fMB 时长=%.1fs 发送耗时=%.2fs",
			title, format, float64(size)/(1<<20), duration, time.Since(start).Seconds())
		return nil
	}
	logger.SubWarn("LunarCore", "Napcat", "QQ 语音条发送失败，降级为音频文件: 《%s》 %v", title, recordErr)

	link := audioLink(audio)
	caption := musicCaption(title, summary, duration) + "\n（QQ 语音条发送失败，改为发送音频文件）"
	if link != "" {
		caption += "\n音频地址: " + link
	}
	if err := sendMusicText(isGroup, targetID, caption); err != nil {
		logger.SubWarn("LunarCore", "Napcat", "QQ 音乐降级文字发送失败: %v", err)
	}

	if path, ok := localAudioFile(audio); ok {
		fileStart := time.Now()
		if fileErr := sendMusicFile(isGroup, targetID, path); fileErr == nil {
			logger.SubInfo("LunarCore", "Napcat",
				"QQ 音乐以音频文件发送成功: %s 大小=%.2fMB 上传耗时=%.2fs",
				filepath.Base(path), float64(size)/(1<<20), time.Since(fileStart).Seconds())
			return nil
		} else {
			logger.SubWarn("LunarCore", "Napcat", "QQ 音乐音频文件发送失败: %v", fileErr)
			return fmt.Errorf("语音条失败（%v），音频文件也失败（%v），已发送文字与音频地址", recordErr, fileErr)
		}
	}
	// 非本机音频（如远程 URL）无法走文件上传，保留文字中的音频地址
	return fmt.Errorf("语音条失败（%v），音频非本机文件，已发送文字与音频地址: %s", recordErr, link)
}

// musicCaption 音乐消息的文字说明（标题 / 描述 / 时长）
func musicCaption(title, summary string, duration float64) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "即兴小曲"
	}
	parts := []string{"《" + title + "》演奏音频"}
	if text := strings.TrimSpace(summary); text != "" {
		parts = append(parts, text)
	}
	if duration > 0 {
		parts = append(parts, fmt.Sprintf("时长 %.1f 秒", duration))
	}
	return strings.Join(parts, "\n")
}

// sendMusicText 向群 / 私聊发送纯文本
func sendMusicText(isGroup bool, targetID int64, text string) error {
	if isGroup {
		return SendGroupPlainTextMessage(targetID, text)
	}
	return SendPrivateTextMessage(targetID, text)
}

// sendMusicFile 上传本机音频文件（群聊落在群文件根目录，私聊走文件消息）
func sendMusicFile(isGroup bool, targetID int64, path string) error {
	name := filepath.Base(path)
	if isGroup {
		return SendGroupFile(targetID, path, name)
	}
	return SendPrivateFile(targetID, path, name)
}
