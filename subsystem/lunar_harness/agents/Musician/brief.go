package Musician

// brief.go 音乐描述的解析、兜底、规整与介绍文案。

import (
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/music"
)

// parseBrief 从模型输出中提取 JSON 描述。
func parseBrief(text string) (music.Brief, error) {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return music.Brief{}, fmt.Errorf("描述中没有 JSON 对象")
	}
	var brief music.Brief
	if err := json.Unmarshal([]byte(text[start:end+1]), &brief); err != nil {
		return music.Brief{}, fmt.Errorf("描述 JSON 解析失败: %w", err)
	}
	return brief, nil
}

// fallbackBrief 主模型失败时的兜底描述（音乐本体仍由 MIDI-LLM 生成）。
func fallbackBrief(instruction string) music.Brief {
	title := strings.TrimSpace(instruction)
	if len([]rune(title)) > 512 {
		title = string([]rune(title)[:512])
	}
	if title == "" {
		title = "即兴小曲"
	}
	return music.Brief{
		Title:       title,
		Prompt:      "A gentle lyrical piano piece with soft strings and a warm cello, about 16 bars, 100 BPM, 4/4 meter.",
		Style:       "抒情",
		Mood:        "温柔",
		Tempo:       defaultTempo,
		Key:         "C",
		Meter:       "4/4",
		Bars:        minMelodyBars,
		Instruments: []string{"钢琴", "弦乐", "大提琴"},
		Structure:   "起承转合",
		Summary:     "一段安静的钢琴小曲",
	}
}

// normalizeBrief 补齐篇幅、速度与英文描述等关键字段。
func normalizeBrief(brief music.Brief) music.Brief {
	if brief.Bars <= 0 {
		brief.Bars = minMelodyBars
	}
	if brief.Tempo <= 0 {
		brief.Tempo = defaultTempo
	}
	if strings.TrimSpace(brief.Title) == "" {
		brief.Title = "即兴小曲"
	}
	if strings.TrimSpace(brief.Prompt) == "" {
		brief.Prompt = music.BriefDescription(brief)
	}
	return brief
}

// describeBrief 生成推送卡片上的一行音乐描述。
func describeBrief(brief music.Brief) string {
	if summary := strings.TrimSpace(brief.Summary); summary != "" {
		return summary
	}
	parts := make([]string, 0, 4)
	if len(brief.Instruments) > 0 {
		parts = append(parts, strings.Join(brief.Instruments, " · "))
	}
	if brief.Tempo > 0 {
		parts = append(parts, fmt.Sprintf("%d BPM", brief.Tempo))
	}
	if key := strings.TrimSpace(brief.Key); key != "" {
		parts = append(parts, key)
	}
	if meter := strings.TrimSpace(brief.Meter); meter != "" {
		parts = append(parts, meter)
	}
	return strings.Join(parts, " · ")
}

// buildSummary 生成回传给对话者的作品介绍。
func buildSummary(brief music.Brief) string {
	desc := "月华演奏了《" + brief.Title + "》"
	details := make([]string, 0, 4)
	if instruments := strings.Join(brief.Instruments, ","); instruments != "" {
		details = append(details, "使用"+instruments)
	}
	if brief.Key != "" {
		mode := "大调"
		if brief.Key == strings.ToLower(brief.Key) {
			mode = "小调"
		}
		details = append(details, brief.Key+mode)
	}
	if brief.Tempo > 0 {
		details = append(details, fmt.Sprintf("%dBPM", brief.Tempo))
	}
	if brief.Structure != "" {
		details = append(details, "结构为"+brief.Structure)
	}
	if len(details) > 0 {
		desc += "（" + strings.Join(details, "，") + "）"
	}
	return desc + "。\n乐谱与演奏音频已推送给用户，可以查看和播放。"
}
