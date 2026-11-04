package LifeMemorizer

// agent.go 生活记忆整理者：把一批旧记忆整理成一条条「事实条目」。
//
// 整理目标是「条目化」而不是「压缩成梗概」：模型输出少于保留下限时先补一次纠正提示，
// 仍然过少则退回按原条目做轻量整理（去重 + 截短），宁可条目多、每条短，也不把事实丢掉。
//
// 分片（chain/life 的 buildMemoryBatches）、批次计划与事务日志仍留在链路层；
// 本智能体只负责模型调用、提示词、输出解析与失败兜底。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Consolidate 把一批旧记忆正文整理为事实条目；模型始终不达标时返回原条目兜底。
func Consolidate(ctx context.Context, contents []string) ([]string, error) {
	if len(contents) == 0 {
		return nil, nil
	}
	minKeep := minKeepFor(len(contents))
	user := joinContents(contents)
	var lastErr error
	for attempt := 0; attempt <= ModelRetry; attempt++ {
		system := TaskPrompt(len(contents), minKeep)
		if attempt > 0 {
			system += fmt.Sprintf("\n\n上一次把整批压得太少了，丢了事实。这次必须按事实拆开，至少输出 %d 条。", minKeep)
		}
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
			logger.Warn("Life", "[记忆整理] 模型输出解析失败（%d/%d）: %v", attempt+1, ModelRetry+1, err)
			continue
		}
		if len(texts) < minKeep {
			lastErr = fmt.Errorf("输出条数 %d 少于下限 %d", len(texts), minKeep)
			recordLastRun(len(contents), attempt+1, lastErr)
			logger.Warn("Life", "[记忆整理] 模型输出过少（%d/%d）：%d 条 < 下限 %d",
				attempt+1, ModelRetry+1, len(texts), minKeep)
			continue
		}
		recordLastRun(len(contents), attempt+1, nil)
		return texts, nil
	}
	logger.Warn("Life", "[记忆整理] 模型始终过度压缩（%v），改为保留原条目做轻量整理", lastErr)
	recordLastRun(len(contents), ModelRetry+1, lastErr)
	agentbase.ReportRun(AgentID, lastErr)
	return fallbackTexts(contents), nil
}

// messagesOf 组装 system + user 两条消息（与迁移前的纯文本调用一致）。
func messagesOf(system, user string) []agentbase.Message {
	return []agentbase.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
}

// recordLastRun 记录最近一次整理任务，供数据导出与修复复位。
func recordLastRun(count, attempts int, err error) {
	lastRunMu.Lock()
	defer lastRunMu.Unlock()
	lastRun = map[string]any{
		"role":       RoleName,
		"items":      count,
		"attempts":   attempts,
		"error":      errorText(err),
		"updated_at": time.Now().Format(time.RFC3339),
	}
}

// snapshotLastRun 返回最近一次整理记录副本。
func snapshotLastRun() map[string]any {
	lastRunMu.Lock()
	defer lastRunMu.Unlock()
	out := make(map[string]any, len(lastRun))
	for key, value := range lastRun {
		out[key] = value
	}
	return out
}

// resetLastRun 清空最近一次整理记录。
func resetLastRun() string {
	lastRunMu.Lock()
	defer lastRunMu.Unlock()
	lastRun = map[string]any{}
	return "已清空生活记忆整理者的最近整理记录"
}

// errorText 把错误转成记录用文本（nil 输出空串）。
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
