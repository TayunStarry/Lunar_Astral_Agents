package Vision

// watch.go 动图 / 视频的观看
//
// 与哔哩观影共用同一条管线（multimodal/watch.go）：视频按 60 秒切片、动图整体一片、
// 每片等距抽帧，逐片写第一人称观看感受，多片再合成为一段整体感受。
// 本文件只负责「模型调用 + 结果收敛 + 写回资源目录」，不再自行实现抽帧与采样。

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/model/agentchat"
	"LunarSubsystem/LunarHarness/engine/multimodal"
	persistence "LunarSubsystem/LunarHarness/engine/persistence"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// watchSlices 观看切片序列，返回整体感受（无定向问题时写回资源目录）
func watchSlices(res resource.Resource, slices []multimodal.WatchSlice, label, question string) (string, error) {
	if len(slices) == 0 {
		return "", fmt.Errorf("未从 %s 中抽取到可用画面", res.Handle)
	}
	// 抽帧结果写入统一缓存表，同一视频再次解读时可直接复用
	cacheSliceFrames(res, slices)
	system := agentchat.PromptText("lifeRole.md")
	if strings.TrimSpace(question) != "" {
		return watchQuestion(system, slices, question)
	}

	parts := make([]string, 0, len(slices))
	for index, slice := range slices {
		text, err := watchSlice(system, label, slice, index+1, len(slices))
		if err != nil {
			logger.Warn("CrystalAstral", "视觉理解：%s 第 %d/%d 片观看失败: %v", res.Handle, index+1, len(slices), err)
			continue
		}
		if trimmed := strings.TrimSpace(text); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("未能生成 %s 的观看感受", res.Handle)
	}

	result := parts[0]
	if len(parts) > 1 {
		if merged, mergeErr := mergeImpressions(system, len(slices), strings.Join(parts, "\n\n")); mergeErr == nil {
			result = merged
		} else {
			logger.Warn("CrystalAstral", "视觉理解：%s 整体感受合成失败，退回逐片拼接: %v", res.Handle, mergeErr)
			result = strings.Join(parts, "\n\n")
		}
	}
	storeResourceSummary(res, result)
	return result, nil
}

// watchSlice 观看一个切片（失败自动重试一次）
func watchSlice(system, label string, slice multimodal.WatchSlice, index, total int) (string, error) {
	task := multimodal.WatchTask(index, total, slice, "这是"+label+"画面。")
	parts := make([]agentchat.ContentPart, 0, len(slice.Images)+1)
	parts = append(parts, agentchat.ContentPart{Type: "text", Text: task})
	for _, image := range slice.Images {
		parts = append(parts, agentchat.ContentPart{Type: "image_url", ImageURL: &agentchat.ImageURL{URL: image}})
	}
	messages := []agentchat.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: parts},
	}
	return agentchat.ChatWithRetry(roleLabel, messages, nil, fmt.Sprintf("%s 第 %d/%d 片", label, index, total))
}

// watchQuestion 针对整段影像的定向提问：各片采样帧合并后再取一次等距子集，单次调用回答
func watchQuestion(system string, slices []multimodal.WatchSlice, question string) (string, error) {
	var images []string
	for _, slice := range slices {
		images = append(images, slice.Images...)
	}
	images = multimodal.SampleImages(images, multimodal.WatchFramesMax)
	if len(images) == 0 {
		return "", fmt.Errorf("定向提问缺少可用画面")
	}
	parts := make([]agentchat.ContentPart, 0, len(images)+1)
	parts = append(parts, agentchat.ContentPart{Type: "text", Text: multimodal.WatchQuestionTask(question)})
	for _, image := range images {
		parts = append(parts, agentchat.ContentPart{Type: "image_url", ImageURL: &agentchat.ImageURL{URL: image}})
	}
	messages := []agentchat.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: parts},
	}
	return agentchat.ChatWithRetry(roleLabel, messages, nil, "影像定向提问")
}

// cacheSliceFrames 把切片抽帧图落到 multimedia/footage 并登记统一缓存表（best-effort）
func cacheSliceFrames(res resource.Resource, slices []multimodal.WatchSlice) {
	sourceURL := strings.TrimSpace(res.URL)
	if sourceURL == "" {
		sourceURL = resource.FileReadURL(res.BlobPath)
	}
	if sourceURL == "" {
		return
	}
	// 已有缓存则跳过，避免重复落盘
	if _, ok := resource.SliceCacheGet(sourceURL, 1); ok {
		return
	}
	hash := strings.TrimSpace(res.ContentHash)
	if hash == "" {
		sum := sha256.Sum256([]byte(sourceURL))
		hash = hex.EncodeToString(sum[:8])
	}
	for _, slice := range slices {
		dir := "multimedia/footage/" + hash + "/seg" + strconv.Itoa(slice.Index)
		total := int64(0)
		for index, dataURI := range slice.Images {
			marker := strings.Index(dataURI, "base64,")
			if marker < 0 {
				continue
			}
			data, err := base64.StdEncoding.DecodeString(dataURI[marker+len("base64,"):])
			if err != nil || len(data) == 0 {
				continue
			}
			rel := dir + "/f" + strconv.Itoa(index+1) + ".jpg"
			if _, err := persistence.SaveFileEx(rel, persistence.SaveOptions{Overwrite: true}, bytes.NewReader(data)); err != nil {
				logger.Warn("CrystalAstral", "视觉理解：切片缓存落盘失败(%s): %v", rel, err)
				continue
			}
			total += int64(len(data))
		}
		if total > 0 {
			_ = resource.SliceCachePut(sourceURL, slice.Index, dir, total)
		}
	}
}

// mergeImpressions 把多片观看感受合成为一段整体感受
func mergeImpressions(system string, total int, concatenated string) (string, error) {
	task := fmt.Sprintf("%s\n\n以上是同一段影像按时间顺序的 %d 段观看记录：\n%s",
		multimodal.WatchMergeTask(), total, concatenated)
	merged, err := agentchat.ChatWithRetry(roleLabel, []agentchat.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: task},
	}, nil, "整体感受")
	if err != nil {
		return "", err
	}
	merged = strings.TrimSpace(merged)
	if merged == "" {
		return "", fmt.Errorf("整体感受为空")
	}
	return merged, nil
}
