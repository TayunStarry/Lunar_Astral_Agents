package multimodal

// watch_prompt.go 统一观看任务文本：视觉理解与哔哩观影共用同一份措辞。
//
// 任务要求「可复述的具体内容 + 固定字段」，并限制字段数与总字数：
// 下游对话者与观感生成需要能直接转述的细节，而旧式无边界长文模板会反复触发防死循环拦截。
// 字段名避开机械记忆标记（如「内容概述」「画面理解」），避免清洗时被整段丢弃。
//
// 图片描述（agents/Vision/image.go）也经 WithVisualRules 前置同一份判定标准，
// 两条路径只在输出档位与字数上不同，不各自维护标准。

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

// WatchMergeTask 把逐片观看记录合成一份可转述的整体记录。
func WatchMergeTask() string {
	return watchPrompt("mediaWatchMergeTask.md")
}

// WatchQuestionTask 针对整段影像的定向提问任务。
func WatchQuestionTask(question string) string {
	return renderWatch(watchPrompt("mediaWatchQuestionTask.md"), "{question}", question)
}

// WithVisualRules 把共用视觉判定标准前置到任务文本。
//
// 图片描述（资源缩影）与影像观看（切片 / 合成 / 定向提问）都从这里取标准，
// 各任务模板只保留自己的字段、字数与格式，不再重复写「禁止空话」这类要求。
func WithVisualRules(task string) string {
	rules, err := prompts.Read(visualFactRulesPrompt)
	if err != nil {
		logger.Error(LogModuleMedia, "读取视觉判定标准 %s 失败: %v", visualFactRulesPrompt, err)
		return task
	}
	return strings.TrimSpace(string(rules)) + "\n\n" + strings.TrimSpace(task)
}

// watchPrompt 读取内嵌提示词并前置共用判定标准（失败返回空串并记录错误）
func watchPrompt(name string) string {
	data, err := prompts.Read(name)
	if err != nil {
		logger.Error(LogModuleMedia, "读取观看提示词 %s 失败: %v", name, err)
		return ""
	}
	return WithVisualRules(string(data))
}

// renderWatch 单次替换占位符（插入正文里的花括号不会被二次展开）
func renderWatch(text string, pairs ...string) string {
	return strings.NewReplacer(pairs...).Replace(text)
}
