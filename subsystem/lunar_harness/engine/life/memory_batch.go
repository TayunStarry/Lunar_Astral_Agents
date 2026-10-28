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
func consolidateBatch(ctx context.Context, batch memoryBatch) ([]string, error) {
	if len(batch.Items) == 0 {
		return nil, nil
	}
	system := memoryTaskPrompt()
	if persona, err := prompts.Read("lifeRole.md"); err == nil {
		system = string(persona) + "\n\n## 本次任务\n\n" + system
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
		return texts, nil
	}
	return nil, fmt.Errorf("记忆整理模型调用失败: %w", lastErr)
}

func memoryTaskPrompt() string {
	return "把下面这批旧记忆做去重、相似合并、重要性筛选和润色压缩。" +
		"只保留值得长期记住的内容：人物、关系、事件、偏好、承诺、明确观点；" +
		"丢弃寒暄、重复、临时过程、机械统计与无信息量内容。" +
		"用第一人称口语化中文复述，每条 20-160 字；不编号、不加标签、不解释。" +
		"只输出 JSON 字符串数组；没有值得保留的内容输出 []。"
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
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		value = truncateRunes(value, memoryMaxTextRunes)
		seen[value] = true
		out = append(out, value)
	}
	return out, nil
}
