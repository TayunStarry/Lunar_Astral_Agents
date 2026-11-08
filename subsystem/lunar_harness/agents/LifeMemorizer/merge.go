package LifeMemorizer

// merge.go 相似记忆簇的合并改写。
//
// 与 Consolidate 的「整批条目化」不同：Merge 只接收语义相近的少量条目，
// 目标是去掉重复表述、合并同一事实，同时保留所有独立事实。
// 模型不可用或输出不可信时返回错误，由链路层保留原条目，绝不自行改写兜底。

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// mergeSystemPrompt 合并相近记忆的任务提示。
//
// 「原样返回」是合法终止态：链路层会把它识别为「没有可合并内容」并解除待处理标记，
// 不会当成失败重试。因此这里明确允许原样返回，不要求模型输出空数组或强行改写。
const mergeSystemPrompt = "你是记忆合并助手。下面几条记忆在语义上高度相近，请只合并其中重复或冗余的表述，" +
	"保留全部独立事实（人名、时间、地点、数字、偏好、结论都要留下），不要添加原文没有的信息。" +
	"如果这些条目其实各自独立、没有可合并的内容，请原样返回，不要为了整理而改写措辞。" +
	"输出 JSON 字符串数组，每条一个事实条目；条数不能多于输入条数；不要解释、不要标题。"

// Merge 对一组相似记忆做合并改写，返回合并后的事实条目。
// 返回与输入实质相同的结果表示「没有可合并内容」，由调用方按合法终止处理；
// 返回错误表示模型结果不可用，调用方应保留原条目不动。
func Merge(ctx context.Context, contents []string) ([]string, error) {
	return mergeInternal(ctx, contents, true)
}

// mergeInternal 执行合并；report=false 用于修复后的复跑验证，不重复上报故障。
//
// 正文判定固定按相似簇合并口径（MergeMinRetainPercent + MergeMinRunes），
// 不受整批条目化的保留阈值影响。
func mergeInternal(ctx context.Context, contents []string, report bool) ([]string, error) {
	if len(contents) == 0 {
		return nil, nil
	}
	recordInputs(contents)
	inputRunes := totalRunes(contents)
	minOutputRunes := mergeRetainFloor(contents)
	user := joinContents(contents)
	lastOutputRunes := 0
	var lastErr error

	// 合并是一次独立的模型调用：自己开一段实况，成功后由 defer 收尾。
	// 重试留在同一轮里，多组合并则是同一智能体的多轮记录，不与调用方（对话者等）合流。
	agentbase.BeginLive(RoleName)
	defer agentbase.EndLive(RoleName)

	for attempt := 0; attempt <= ModelRetry; attempt++ {
		system := mergeSystemPrompt
		if persona := agentbase.PromptText(promptRole); persona != "" {
			system = persona + "\n\n## 本次任务\n\n" + system
		}
		reply, err := agentbase.Stream(ctx, agentbase.StreamRequest{
			Agent:      AgentID,
			Role:       RoleName,
			Messages:   messagesOf(system, user),
			Model:      config.AgentMultimodalModel,
			NonStream:  true,
			ManageLive: true,
		})
		if err != nil {
			lastErr = err
		} else {
			texts, outputRunes, checkErr := checkMerge(contents, reply.Text)
			lastOutputRunes = outputRunes
			if checkErr == nil {
				if report {
					recordLastRunWith(len(contents), attempt+1, nil, map[string]any{
						"input_items":              len(contents),
						"input_runes":              inputRunes,
						"output_runes":             outputRunes,
						"merge_min_retain_percent": MergeMinRetainPercent,
						"merge_min_output_runes":   minOutputRunes,
					})
				}
				return texts, nil
			}
			lastErr = checkErr
			logger.Warn("Life", "[记忆整理] 合并结果不可用（%d/%d）：%v", attempt+1, ModelRetry+1, checkErr)
		}
		if report {
			recordLastRun(len(contents), attempt+1, lastErr)
		}
	}
	logger.Warn("Life", "[记忆整理] 合并模型结果不可用（%v），保留原条目", lastErr)
	if report {
		recordLastRunWith(len(contents), ModelRetry+1, lastErr, map[string]any{
			"input_items":              len(contents),
			"input_runes":              inputRunes,
			"output_runes":             lastOutputRunes,
			"merge_min_retain_percent": MergeMinRetainPercent,
			"merge_min_output_runes":   minOutputRunes,
		})
		agentbase.ReportDetail(agentbase.FailureDetail{
			Agent:           AgentID,
			Kind:            agentbase.FailureKindOutput,
			Criterion:       fmt.Sprintf("合并结果非空、有实质正文，且正文总量 ≥ max(输入的 %d%%, %d 字)", MergeMinRetainPercent, MergeMinRunes),
			Entry:           "merge",
			Err:             lastErr,
			Tool:            "agentbase.Stream",
			Retries:         ModelRetry,
			ContextMessages: len(contents),
			ContextRunes:    inputRunes,
			Metrics: map[string]int64{
				"input_items":              int64(len(contents)),
				"input_runes":              int64(inputRunes),
				"output_runes":             int64(lastOutputRunes),
				"merge_min_retain_percent": int64(MergeMinRetainPercent),
				"merge_min_output_runes":   int64(minOutputRunes),
			},
		})
	}
	return nil, lastErr
}

// checkMerge 校验一次合并输出，返回可用的条目、输出正文长度与不可用原因。
//
// 合并的输入本身就是高度重复的记忆（典型场景：8 条几乎相同的记忆合成 1~2 条），
// 结果正文必然远短于输入总长，因此不沿用整批条目化的 20% 下限，只在下列情况判不可用：
//  1. 结果为空：模型返回空内容、非 JSON 数组或空数组；
//  2. 没有实质正文：去掉空白与标点后什么都不剩；
//  3. 正文过短：低于 max(输入的 5%, 12 字) 且不超过最长单条输入，说明几乎没留下信息；
//  4. 条数多于输入：模型在拆分或发挥，结果不可信。
//
// 返回的错误文本即命中的规则名，会原样出现在运行日志与失败上报里。
func checkMerge(contents []string, raw string) ([]string, int, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil, 0, fmt.Errorf("规则[结果为空]：模型返回空内容")
	}
	texts, err := parseArray(text)
	if err != nil {
		return nil, 0, fmt.Errorf("规则[结果为空]：%w", err)
	}
	if len(texts) == 0 {
		return nil, 0, fmt.Errorf("规则[结果为空]：合并结果为空数组")
	}
	outputRunes := totalRunes(texts)
	// 合并不允许扩张条目：输出多于输入说明模型在拆分/发挥，结果不可信。
	if len(texts) > len(contents) {
		return nil, outputRunes, fmt.Errorf("规则[条数扩张]：合并结果条数 %d 多于输入 %d", len(texts), len(contents))
	}
	if !hasSubstantiveText(texts) {
		return nil, outputRunes, fmt.Errorf("规则[无实质正文]：去掉空白与标点后没有可读内容")
	}
	floor := mergeRetainFloor(contents)
	if outputRunes < floor {
		inputRunes := totalRunes(contents)
		percent := 0.0
		if inputRunes > 0 {
			percent = float64(outputRunes) * 100 / float64(inputRunes)
		}
		return nil, outputRunes, fmt.Errorf(
			"规则[正文过短]：输出 %d 字 < 下限 %d 字（占输入 %.1f%%，要求 ≥%d%% 且 ≥%d 字，绝对下限不超过最长单条）",
			outputRunes, floor, percent, MergeMinRetainPercent, MergeMinRunes)
	}
	return texts, outputRunes, nil
}

// mergeRetainFloor 相似簇合并结果的正文下限（非空白字符数）。
//
// 取「输入总量的 5%」与「最小绝对字数」的较大者，但绝对下限不超过最长单条输入：
// 8 条完全相同的短记忆合成 1 条属于正常结果，固定下限不能把它误判成不可用。
func mergeRetainFloor(contents []string) int {
	inputRunes := totalRunes(contents)
	percentFloor := inputRunes * MergeMinRetainPercent / 100
	longest := 0
	for _, content := range contents {
		if runes := totalRunes([]string{content}); runes > longest {
			longest = runes
		}
	}
	return max(percentFloor, min(MergeMinRunes, longest))
}

// hasSubstantiveText 判断合并结果里是否还有实质正文（去掉空白、标点与符号后不为空）。
func hasSubstantiveText(texts []string) bool {
	for _, text := range texts {
		for _, r := range text {
			if !unicode.IsSpace(r) && !unicode.IsPunct(r) && !unicode.IsSymbol(r) {
				return true
			}
		}
	}
	return false
}
