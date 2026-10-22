package Descriptor

// watch.go 动图 / 视频的序列帧观看与摘要
//
// 与静态图共用同一条「有描述返回描述、没有描述现场生成」的语义，
// 区别只在输入形态：动图/视频先抽帧分组，静态图直接走视觉副本。

import (
	"fmt"
	"strconv"
	"strings"

	"CorePersonality/engine/multimodal"
	"CorePersonality/engine/persistence"
	"LunarSubsystem/LoggerGeneral"
)

// watchFrameGroups 观看一组序列帧并返回描述（通用观看结论会写回资源目录）
func watchFrameGroups(res persistence.ArchiveResource, groups []multimodal.MediaFrameGroup, mode, question string) (string, error) {
	if len(groups) == 0 {
		return "", fmt.Errorf("未从 %s 中抽取到可用画面", res.Handle)
	}
	watched := sampleFrameGroups(groups)
	if len(watched) < len(groups) {
		LoggerGeneral.Info("CrystalAstral", "描述者：%s 共 %d 组画面，超过单次上限，等距抽样观看 %d 组",
			res.Handle, len(groups), len(watched))
	}

	system := promptText("descriptorMediaRole.md")
	parts := make([]string, 0, len(watched))
	for index, group := range watched {
		LoggerGeneral.Info("CrystalAstral", "描述者：观看 %s 第 %d/%d 组（%d 帧）",
			res.Handle, index+1, len(watched), len(group.Frames))
		text, watchErr := describeFrameGroup(system, mode, group, index+1, len(watched), question)
		if watchErr != nil {
			LoggerGeneral.Warn("CrystalAstral", "描述者：%s 第 %d 组理解失败: %v", res.Handle, index+1, watchErr)
			continue
		}
		if trimmed := strings.TrimSpace(text); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("未能生成 %s 的画面描述", res.Handle)
	}

	result := strings.Join(parts, "\n\n")
	if question == "" && len([]rune(result)) > summaryThreshold {
		if summary, summaryErr := summarizeMedia(system, result); summaryErr == nil && strings.TrimSpace(summary) != "" {
			result = strings.TrimSpace(summary)
		} else {
			LoggerGeneral.Warn("CrystalAstral", "描述者：%s 客观摘要失败，退回拼接文本", res.Handle)
			result = hardCutRunes(result, summaryThreshold)
		}
	}
	if question == "" {
		storeResourceSummary(res, result)
	}
	return result, nil
}

// sampleFrameGroups 等距抽样：最多 maxSegments 组，恒包含首组与末组
func sampleFrameGroups(groups []multimodal.MediaFrameGroup) []multimodal.MediaFrameGroup {
	if len(groups) <= maxSegments {
		return groups
	}
	picked := make([]multimodal.MediaFrameGroup, 0, maxSegments)
	seen := map[int]bool{}
	for i := 0; i < maxSegments; i++ {
		index := (i * (len(groups) - 1)) / (maxSegments - 1)
		if seen[index] {
			continue
		}
		seen[index] = true
		picked = append(picked, groups[index])
	}
	return picked
}

// describeFrameGroup 观看一组采样帧（失败自动重试一次）
func describeFrameGroup(system, mode string, group multimodal.MediaFrameGroup, index, total int, question string) (string, error) {
	parts := make([]contentPart, 0, len(group.Frames)+1)
	parts = append(parts, contentPart{Type: "text", Text: frameGroupTask(mode, index, total, question)})
	for _, frame := range group.Frames {
		if strings.TrimSpace(frame.Image) == "" {
			continue
		}
		parts = append(parts, contentPart{Type: "image_url", ImageURL: &imageURL{URL: frame.Image}})
	}
	messages := []chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: parts},
	}
	return chatWithRetry(messages, nil, fmt.Sprintf("%s 第 %d/%d 组", mode, index, total))
}

// frameGroupTask 构建该组的观看任务提示词
func frameGroupTask(mode string, index, total int, question string) string {
	indexText := strconv.Itoa(index)
	totalText := strconv.Itoa(total)
	if question != "" {
		return renderPrompt(promptText("descriptorMediaQuestionTask.md"),
			"{question}", question,
			"{index}", indexText,
			"{total}", totalText)
	}
	name := "descriptorMediaVideoTask.md"
	if mode == "animation" {
		name = "descriptorMediaAnimationTask.md"
	}
	return renderPrompt(promptText(name),
		"{index}", indexText,
		"{total}", totalText)
}

// summarizeMedia 把多组理解整合为一份客观摘要
func summarizeMedia(system, concatenated string) (string, error) {
	task := renderPrompt(promptText("descriptorMediaSummaryTask.md"), "{content}", concatenated)
	return chatWithRetry([]chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: task},
	}, nil, "影像摘要")
}
