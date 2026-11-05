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

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// mergeSystemPrompt 合并相近记忆的任务提示。
const mergeSystemPrompt = "你是记忆合并助手。下面几条记忆在语义上高度相近，请只合并其中重复或冗余的表述，" +
	"保留全部独立事实（人名、时间、地点、数字、偏好、结论都要留下），不要添加原文没有的信息。" +
	"如果这些条目其实各自独立、没有可合并的内容，请原样返回，不要为了整理而改写措辞。" +
	"输出 JSON 字符串数组，每条一个事实条目；条数不能多于输入条数；不要解释、不要标题。"

// Merge 对一组相似记忆做合并改写，返回合并后的事实条目。
// 返回错误表示模型结果不可用；调用方应保留原条目不动。
func Merge(ctx context.Context, contents []string) ([]string, error) {
	if len(contents) == 0 {
		return nil, nil
	}
	user := joinContents(contents)
	var lastErr error
	for attempt := 0; attempt <= ModelRetry; attempt++ {
		system := mergeSystemPrompt
		if persona := agentbase.PromptText(promptRole); persona != "" {
			system = persona + "\n\n## 本次任务\n\n" + system
		}
		reply, err := agentbase.Stream(ctx, agentbase.StreamRequest{
			Agent:     AgentID,
			Role:      RoleName,
			Messages:  messagesOf(system, user),
			Model:     config.AgentMultimodalModel,
			NonStream: true,
			Silent:    true,
		})
		if err != nil {
			lastErr = err
			recordLastRun(len(contents), attempt+1, err)
			continue
		}
		text := strings.TrimSpace(reply.Text)
		if text == "" {
			lastErr = fmt.Errorf("模型返回空内容")
			recordLastRun(len(contents), attempt+1, lastErr)
			continue
		}
		texts, err := parseArray(text)
		if err != nil {
			lastErr = err
			recordLastRun(len(contents), attempt+1, err)
			logger.Warn("Life", "[记忆整理] 合并输出解析失败（%d/%d）: %v", attempt+1, ModelRetry+1, err)
			continue
		}
		if len(texts) == 0 {
			lastErr = fmt.Errorf("合并结果为空")
			recordLastRun(len(contents), attempt+1, lastErr)
			continue
		}
		// 合并不允许扩张条目：输出多于输入说明模型在拆分/发挥，结果不可信。
		if len(texts) > len(contents) {
			lastErr = fmt.Errorf("合并结果条数 %d 多于输入 %d", len(texts), len(contents))
			recordLastRun(len(contents), attempt+1, lastErr)
			logger.Warn("Life", "[记忆整理] 合并结果条数异常（%d/%d）：%d > %d",
				attempt+1, ModelRetry+1, len(texts), len(contents))
			continue
		}
		// 保留事实粒度：输出正文总量不得低于输入的下限比例，
		// 防止把多条不同事实压成一句笼统概括。
		if !retainEnough(contents, texts) {
			lastErr = fmt.Errorf("合并结果过度压缩（正文不足输入的 %d%%）", MinRetainPercent)
			recordLastRun(len(contents), attempt+1, lastErr)
			logger.Warn("Life", "[记忆整理] 合并结果过度压缩（%d/%d），保留原条目", attempt+1, ModelRetry+1)
			continue
		}
		recordLastRun(len(contents), attempt+1, nil)
		return texts, nil
	}
	logger.Warn("Life", "[记忆整理] 合并模型结果不可用（%v），保留原条目", lastErr)
	recordLastRun(len(contents), ModelRetry+1, lastErr)
	agentbase.ReportRun(AgentID, lastErr)
	return nil, lastErr
}
