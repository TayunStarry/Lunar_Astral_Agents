package resource

// audio_transcript.go 音频转写悬空句柄：有描述、无实体。
//
// 新音频接入规范：
//   - 音频字节不再写入 audios/ 或 multimedia/，接入时直接调用 audio_transcriber
//     完成文本转写；
//   - 资源目录登记一个「悬空句柄」：State=active、BlobPath 为空、ParseText 为转写文本，
//     句柄可作为描述信息长期保留；
//   - 只有用户引用包含音频的消息时，才按句柄读取其转写文本（不再解码音频本体）。

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// danglingAudioURLPrefix 悬空音频句柄的合成 URL 前缀（不指向任何实体文件）。
// 同类音频（内容哈希一致）复用同一句柄，天然完成去重。
const danglingAudioURLPrefix = "lunar-audio://"

// AdmitAudioTranscript 登记音频转写悬空句柄；同一内容哈希复用既有句柄。
func AdmitAudioTranscript(in AudioTranscriptInput) (AdmitResult, error) {
	if err := EnsureReady(); err != nil {
		return AdmitResult{}, err
	}
	transcript := strings.TrimSpace(in.Transcript)
	if transcript == "" {
		transcript = "（这段音频没有转写出可用文本）"
	}
	hash := normalizeTranscriptHash(in.ContentHash, transcript)
	rawURL := NormalizeURL(danglingAudioURLPrefix + hash)
	if rawURL == "" {
		return AdmitResult{}, fmt.Errorf("音频内容哈希为空")
	}
	origin := strings.TrimSpace(in.Origin)
	if origin == "" {
		origin = OriginClientUpload
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = "语音.wav"
	}

	// 同一内容哈希：复用既有句柄，仅刷新描述文本与使用时间
	if existing, found := archiveDB.FindResourceByURL(rawURL); found {
		archiveDB.TouchResource(existing.ID)
		if existing.Handle != "" {
			if strings.TrimSpace(existing.ParseText) != transcript {
				_ = archiveDB.setURLParseText(rawURL, transcript)
			}
			return admitResultOf(existing, true), nil
		}
	}

	res := &Resource{
		Kind:        KindAudio,
		URL:         rawURL,
		Name:        name,
		Ext:         strings.ToLower(filepath.Ext(name)),
		MIME:        "audio/x-wav",
		ContentHash: hash,
		Title:       strings.TrimSuffix(name, filepath.Ext(name)),
		Tags:        []string{"voice", "transcript"},
		Origin:      origin,
		Retain:      DefaultRetain(origin),
		State:       StateActive,
		ParseText:   transcript,
		ParseState:  ParseReady,
		CreatedAt:   time.Now().Unix(),
		UsedAt:      time.Now().Unix(),
	}
	if res.Ext == "" {
		res.Ext = ".wav"
	}
	if res.Title == "" {
		res.Title = name
	}
	if err := archiveDB.InsertResource(res); err != nil {
		logger.Warn("FileManager", "音频转写句柄登记失败: %v", err)
		return AdmitResult{}, err
	}
	return admitResultOf(*res, false), nil
}

// normalizeTranscriptHash 内容哈希规整：缺失时按转写文本计算，统一截断到 sha16。
func normalizeTranscriptHash(hash, transcript string) string {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		sum := sha256.Sum256([]byte(transcript))
		hash = fmt.Sprintf("%x", sum)
	}
	hash = strings.ToLower(hash)
	if len(hash) > 16 {
		hash = hash[:16]
	}
	return hash
}
