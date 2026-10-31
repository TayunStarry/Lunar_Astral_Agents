package multimodal

// watch_prompt.go 统一观看任务文本：视觉理解与哔哩观影共用同一份措辞。
//
// 任务只要第一人称的「发生了什么 + 感受」，明确禁止分时段罗列与完整记录；
// 旧的分时段结构化格式会让模型输出长文模板，反复触发防死循环拦截。

import (
	"strconv"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/prompts"
)

// WatchTask 组装一个切片的观看任务；context 为调用方补充的背景（可为空）。
func WatchTask(index, total int, slice WatchSlice, context string) string {
	return renderWatch(watchPrompt("mediaWatchTask.md"),
		"{index}", strconv.Itoa(index),
		"{total}", strconv.Itoa(total),
		"{start}", FormatSeconds(slice.Start),
		"{end}", FormatSeconds(slice.End),
		"{count}", strconv.Itoa(len(slice.Images)),
		"{context}", context)
}

// WatchMergeTask 把多片观看感受合成一段整体感受的任务。
func WatchMergeTask() string {
	return watchPrompt("mediaWatchMergeTask.md")
}

// WatchQuestionTask 针对整段影像的定向提问任务。
func WatchQuestionTask(question string) string {
	return renderWatch(watchPrompt("mediaWatchQuestionTask.md"), "{question}", question)
}

// watchPrompt 读取内嵌提示词（失败返回空串并记录错误）
func watchPrompt(name string) string {
	data, err := prompts.Read(name)
	if err != nil {
		logger.Error(LogModuleMedia, "读取观看提示词 %s 失败: %v", name, err)
		return ""
	}
	return string(data)
}

// renderWatch 单次替换占位符（插入正文里的花括号不会被二次展开）
func renderWatch(text string, pairs ...string) string {
	return strings.NewReplacer(pairs...).Replace(text)
}
