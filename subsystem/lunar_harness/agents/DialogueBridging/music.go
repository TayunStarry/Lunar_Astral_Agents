package DialogueBridging

// music.go 演奏音频投递策略：语音条 → 文字 + 文件 → 文字 + 链接。
//
// 这里是「一级失败后降级到哪一级」的业务判定；clients/napcat 只提供
// record / 文本 / 文件三种传输原语与音频地址归一，不再回答降级顺序。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/clients/napcat"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// deliverMusic 把演奏音频投递到 QQ：语音条 → 文字 + 文件 → 文字 + 链接。
func deliverMusic(isGroup bool, targetID int64, audio, title, summary string, duration float64) error {
	source, ok := napcat.AudioSource(audio)
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
		recordErr = napcat.SendGroupRecordMessage(targetID, audio)
	} else {
		recordErr = napcat.SendPrivateRecordMessage(targetID, audio)
	}
	if recordErr == nil {
		logger.SubInfo("LunarCore", "Napcat",
			"QQ 音乐以语音条发送成功: 《%s》 源格式=%s 源大小=%.2fMB 时长=%.1fs 发送耗时=%.2fs",
			title, format, float64(size)/(1<<20), duration, time.Since(start).Seconds())
		return nil
	}
	logger.SubWarn("LunarCore", "Napcat", "QQ 语音条发送失败，降级为音频文件: 《%s》 %v", title, recordErr)

	link := napcat.AudioLink(audio)
	caption := musicCaption(title, summary, duration) + "\n（QQ 语音条发送失败，改为发送音频文件）"
	if link != "" {
		caption += "\n音频地址: " + link
	}
	if err := sendMusicText(isGroup, targetID, caption); err != nil {
		logger.SubWarn("LunarCore", "Napcat", "QQ 音乐降级文字发送失败: %v", err)
	}

	if path, ok := napcat.LocalAudioFile(audio); ok {
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
		return napcat.SendGroupPlainTextMessage(targetID, text)
	}
	return napcat.SendPrivateTextMessage(targetID, text)
}

// sendMusicFile 上传本机音频文件（群聊落在群文件根目录，私聊走文件消息）
func sendMusicFile(isGroup bool, targetID int64, path string) error {
	name := filepath.Base(path)
	if isGroup {
		return napcat.SendGroupFile(targetID, path, name)
	}
	return napcat.SendPrivateFile(targetID, path, name)
}

// fileSizeOf 返回本机文件大小，失败时返回 0
func fileSizeOf(path string) int64 {
	if info, err := os.Stat(path); err == nil {
		return info.Size()
	}
	return 0
}
