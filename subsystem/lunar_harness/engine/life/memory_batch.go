package life

// memory_batch.go 记忆整理的分批、模型调用与输出解析。

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/memory"
	"LunarSubsystem/LunarHarness/engine/model/textcall"
	"LunarSubsystem/LunarHarness/prompts"
)

// listAllTextMemories 枚举全部文本记忆，排除哔哩观影记忆。
func listAllTextMemories() []memoryItem {
	const pageSize = 200
	out := make([]memoryItem, 0)
	for offset := 0; ; offset += pageSize {
		entries, total := memory.List("text", offset, pageSize)
		for _, entry := range entries {
			if entry.Source == "bilibili_viewer" {
				continue
			}
			content := strings.TrimSpace(entry.Content)
			if content == "" || strings.HasPrefix(content, "[B站视频]") {
				continue
			}
			out = append(out, memoryItem{
				ID:        entry.ID,
				Content:   content,
				Role:      entry.Role,
				Timestamp: entry.Timestamp,
			})
		}
		if len(entries) == 0 || offset+pageSize >= total {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Timestamp < out[j].Timestamp })
	return out
}

// buildMemoryBatches 按 1 万字符分批，单条记忆不跨批、不截断。
func buildMemoryBatches(items []memoryItem) []memoryBatch {
	batches := make([]memoryBatch, 0, len(items)/10+1)
	current := memoryBatch{}
	runes := 0
	for _, item := range items {
		n := utf8.RuneCountInString(item.Content)
		if runes > 0 && runes+n > batchMaxRunes {
			batches = append(batches, current)
			current = memoryBatch{}
			runes = 0
		}
		current.Items = append(current.Items, item)
		runes += n
		if n > batchMaxRunes {
			logger.Warn(Module, "[记忆整理] 单条记忆 %s 超过 %d 字符，独占一批且不截断", item.ID, batchMaxRunes)
			batches = append(batches, current)
			current = memoryBatch{}
			runes = 0
		}
	}
	if len(current.Items) > 0 {
		batches = append(batches, current)
	}
	return batches
}

// consolidateBatch 调用 primary_reasoning 处理一个批次。
//
// 整理目标是「条目化」而不是「压缩成梗概」：模型输出少于 memoryMinKeep 时先补一次纠正提示，
// 仍然过少则退回按原条目做轻量整理（去重 + 截短），宁可条目多、每条短，也不把事实丢掉。
func consolidateBatch(ctx context.Context, batch memoryBatch) ([]string, error) {
	if len(batch.Items) == 0 {
		return nil, nil
	}
	minKeep := memoryMinKeep(len(batch.Items))
	persona := ""
	if content, err := prompts.Read("lifeRole.md"); err == nil {
		persona = string(content)
	}
	var builder strings.Builder
	for _, item := range batch.Items {
		builder.WriteString("- ")
		builder.WriteString(item.Content)
		builder.WriteString("\n")
	}
	user := builder.String()
	var lastErr error
	for attempt := 0; attempt <= modelRetry; attempt++ {
		system := memoryTaskPrompt(len(batch.Items), minKeep)
		if attempt > 0 {
			system += fmt.Sprintf("\n\n上一次把整批压得太少了，丢了事实。这次必须按事实拆开，至少输出 %d 条。", minKeep)
		}
		if persona != "" {
			system = persona + "\n\n## 本次任务\n\n" + system
		}
		text, err := textcall.Complete(ctx, config.AgentMultimodalModel, system, user)
		if err != nil {
			lastErr = err
			continue
		}
		texts, err := parseMemoryArray(text)
		if err != nil {
			lastErr = err
			logger.Warn(Module, "[记忆整理] 模型输出解析失败（%d/%d）: %v", attempt+1, modelRetry+1, err)
			continue
		}
		if len(texts) < minKeep {
			lastErr = fmt.Errorf("输出条数 %d 少于下限 %d", len(texts), minKeep)
			logger.Warn(Module, "[记忆整理] 模型输出过少（%d/%d）：%d 条 < 下限 %d",
				attempt+1, modelRetry+1, len(texts), minKeep)
			continue
		}
		return texts, nil
	}
	logger.Warn(Module, "[记忆整理] 模型始终过度压缩（%v），改为保留原条目做轻量整理", lastErr)
	return fallbackMemoryTexts(batch.Items), nil
}

// memoryMinKeep 整理后至少保留的条数：按输入条数比例取下限，并夹在 [floor, ceiling]。
func memoryMinKeep(count int) int {
	if count <= 0 {
		return 0
	}
	keep := count * memoryKeepPercent / 100
	if keep < memoryKeepFloor {
		keep = memoryKeepFloor
	}
	if keep > memoryKeepCeiling {
		keep = memoryKeepCeiling
	}
	if keep > count {
		keep = count
	}
	return keep
}

// fallbackMemoryTexts 模型过度压缩时的保底：原条目逐条去重、折叠空白并截短，
// 保留全部事实，只是把长段落压成短条目。
func fallbackMemoryTexts(items []memoryItem) []string {
	out := make([]string, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		text := strings.TrimSpace(strings.Join(strings.Fields(item.Content), " "))
		if text == "" {
			continue
		}
		text = truncateAtSentence(text, memoryMaxTextRunes)
		if seen[text] {
			continue
		}
		seen[text] = true
		out = append(out, text)
	}
	return out
}

// memoryTaskPrompt 条目化整理提示词：保留事实覆盖，只做去重与拆分，不做整体压缩。
func memoryTaskPrompt(itemCount, minKeep int) string {
	return fmt.Sprintf("把下面这批旧记忆整理成一条条「事实条目」，不是压成一段梗概，也不是做摘要。\n"+
		"整理要求：\n"+
		"1. 一个独立事实、一次事件、一个偏好或一条约定算一条；同一件事的不同侧面可以拆成多条。\n"+
		"2. 只合并完全重复的表述；意思相近但事实不同的必须分开保留，不要合并成大段落。\n"+
		"3. 只丢弃寒暄、无信息量的闲聊、机械统计与工具执行过程。\n"+
		"4. 人物、称呼、时间、地点、事件经过、数字、偏好、承诺与明确观点必须保留，不得省略或模糊化。\n"+
		"5. 每条 15-100 字，绝不超过 200 字；不编号、不加标签、不解释、不写总述。\n"+
		"6. 本批共 %d 条旧记忆，目标不少于 %d 条；条数不足通常说明合并过度，请按事实拆开。"+
		"不得编造原文没有的信息。\n"+
		"只输出 JSON 字符串数组。", itemCount, minKeep)
}

// parseMemoryArray 从模型输出里取出 JSON 字符串数组并清洗。
func parseMemoryArray(raw string) ([]string, error) {
	text := strings.TrimSpace(raw)
	if strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		kept := make([]string, 0, len(lines))
		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				continue
			}
			kept = append(kept, line)
		}
		text = strings.TrimSpace(strings.Join(kept, "\n"))
	}
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("输出不是 JSON 数组")
	}
	var values []string
	if err := json.Unmarshal([]byte(text[start:end+1]), &values); err != nil {
		return nil, fmt.Errorf("解析 JSON 数组失败: %w", err)
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		// 先按句读截短再去重：超长条目截断后可能变得完全相同，按截断后的正文判重才准
		value = truncateAtSentence(strings.TrimSpace(value), memoryMaxTextRunes)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out, nil
}
