package music

// review.go 生成端自检：把 AMT token 流解析成音符并给出可枚举的问题码。
//
// 体检只读取 token，不渲染、不落盘，供演奏者在多候选之间排序与触发带诊断重写。

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// amtNote 一个已解析的音符事件（时间为 10ms 单位的绝对起点）。
type amtNote struct {
	// Time 绝对起始时间（10ms）
	Time int
	// Duration 时值（10ms）
	Duration int
	// Instrument GM 乐器编号（128 为鼓组）
	Instrument int
	// Pitch MIDI 音高（0-127）
	Pitch int
}

// scanNotes 解析 AMT 三连音；返回合法音符与损坏事件数（含尾部残缺三元组）。
func scanNotes(ids []int) ([]amtNote, int) {
	notes := make([]amtNote, 0, len(ids)/amtNotesPerStep+1)
	malformed := 0
	for index := 0; index+amtNotesPerStep <= len(ids); index += amtNotesPerStep {
		timeID, durID, noteID := ids[index], ids[index+1], ids[index+2]
		duration := durID - amtDurOffset
		if timeID < amtTimeOffset || timeID >= amtMaxTime ||
			duration < 0 || duration >= amtMaxDuration ||
			noteID < amtNoteOffset || noteID >= amtRestToken {
			malformed++
			continue
		}
		note := noteID - amtNoteOffset
		instrument := note / 128
		if instrument > amtDrumInstrument {
			malformed++
			continue
		}
		notes = append(notes, amtNote{
			Time:       timeID,
			Duration:   duration,
			Instrument: instrument,
			Pitch:      note % 128,
		})
	}
	if remainder := len(ids) % amtNotesPerStep; remainder > 0 {
		malformed++
	}
	return notes, malformed
}

// ReviewTokens 对一次模型输出做客观体检并计算候选排序分（越低越好）。
func ReviewTokens(ids []int, brief Brief) Review {
	normalized := normalizeBrief(brief)
	notes, malformed := scanNotes(ids)
	review := Review{
		Tokens:    len(ids),
		Notes:     len(notes),
		Malformed: malformed,
	}
	if len(notes) == 0 {
		review.Score = math.Inf(1)
		review.Issues = append(review.Issues, issueNoNotes)
		return review
	}

	maxEnd := 0
	lastTime := -1
	unordered := 0
	instruments := make(map[int]struct{})
	pitches := make(map[int]struct{})
	events := make([]timeEvent, 0, len(notes)*2)
	for _, note := range notes {
		if end := note.Time + note.Duration; end > maxEnd {
			maxEnd = end
		}
		if note.Time < lastTime {
			unordered++
		}
		lastTime = note.Time
		instruments[note.Instrument] = struct{}{}
		pitches[note.Pitch] = struct{}{}
		events = append(events, timeEvent{At: note.Time, Delta: 1})
		events = append(events, timeEvent{At: note.Time + note.Duration, Delta: -1})
	}
	review.MaxOverlap = peakOverlap(events)
	review.SpanSeconds = float64(maxEnd) / 100
	review.DistinctPitches = len(pitches)
	if review.SpanSeconds > 0 {
		review.NotesPerSecond = float64(len(notes)) / review.SpanSeconds
	}

	malformedRatio := float64(malformed) / float64(len(notes)+malformed)
	unorderedRatio := float64(unordered) / float64(len(notes))
	expected := expectedSpanSeconds(normalized)

	if malformedRatio > reviewMaxMalformedRatio {
		review.Issues = append(review.Issues, issueMalformed)
	}
	if unorderedRatio > reviewMaxUnorderedRatio {
		review.Issues = append(review.Issues, issueUnordered)
	}
	if review.SpanSeconds < expected*reviewSpanShortRatio {
		review.Issues = append(review.Issues, issueTooShort)
	}
	if review.SpanSeconds > expected*reviewSpanLongRatio {
		review.Issues = append(review.Issues, issueTooLong)
	}
	if review.MaxOverlap > reviewMaxOverlap {
		review.Issues = append(review.Issues, issueDense)
	}
	if review.DistinctPitches < reviewMinDistinctPitches {
		review.Issues = append(review.Issues, issueNarrowPitch)
	}
	if review.NotesPerSecond > reviewMaxNotesPerSecond {
		review.Issues = append(review.Issues, issueTooDense)
	}
	if review.NotesPerSecond < reviewMinNotesPerSecond {
		review.Issues = append(review.Issues, issueSparse)
	}
	if len(notes) < reviewMinNotes && !containsIssue(review.Issues, issueTooShort) {
		// 音符数量不足时统一按篇幅过短处理，避免同一原因重复报告
		review.Issues = append(review.Issues, issueTooShort)
	}
	review.Score = reviewScore(review, expected)
	review.Instruments = sortedKeys(instruments)
	return review
}

// Diagnosis 把问题码翻译成追加到音乐描述后的英文重写提示；无问题时返回空串。
func Diagnosis(review Review) string {
	hints := make([]string, 0, len(review.Issues))
	seen := make(map[string]struct{}, len(review.Issues))
	for _, issue := range review.Issues {
		hint, ok := diagnosisHints[issue]
		if !ok {
			continue
		}
		if _, exists := seen[hint]; exists {
			continue
		}
		seen[hint] = struct{}{}
		hints = append(hints, hint)
	}
	result := strings.TrimSpace(strings.Join(hints, " "))
	if len(result) > reviewDiagnosisLimit {
		result = result[:reviewDiagnosisLimit]
	}
	return result
}

// timeEvent 发声区间扫描事件：Delta=+1 起始，-1 结束。
type timeEvent struct {
	// At 事件时间（10ms）
	At int
	// Delta 并发数增量
	Delta int
}

// peakOverlap 扫描区间求任一时刻的最大并发音符数。
func peakOverlap(events []timeEvent) int {
	sort.Slice(events, func(i, j int) bool {
		if events[i].At != events[j].At {
			return events[i].At < events[j].At
		}
		// 同一时刻先结束后开始，避免把首尾相接算成重叠
		return events[i].Delta < events[j].Delta
	})
	current, peak := 0, 0
	for _, event := range events {
		current += event.Delta
		if current > peak {
			peak = current
		}
	}
	return peak
}

// expectedSpanSeconds 按拍号与篇幅估算期望时长。
func expectedSpanSeconds(brief Brief) float64 {
	num, den := meterParts(brief.Meter)
	beatsPerBar := float64(num) * 4 / float64(den)
	return float64(brief.Bars) * beatsPerBar * 60 / float64(brief.Tempo)
}

// reviewScore 候选排序分：违规越重分越高，供同批候选比较。
func reviewScore(review Review, expected float64) float64 {
	score := 0.0
	if review.Notes+review.Malformed > 0 {
		score += float64(review.Malformed) / float64(review.Notes+review.Malformed) * 50
	}
	if review.MaxOverlap > reviewMaxOverlap {
		score += float64(review.MaxOverlap-reviewMaxOverlap) * 2
	}
	if expected > 0 && review.SpanSeconds > 0 {
		score += math.Abs(review.SpanSeconds-expected) / expected * 10
	}
	if review.DistinctPitches < reviewMinDistinctPitches {
		score += float64(reviewMinDistinctPitches-review.DistinctPitches) * 2
	}
	if review.NotesPerSecond > reviewMaxNotesPerSecond {
		score += review.NotesPerSecond - reviewMaxNotesPerSecond
	}
	if review.NotesPerSecond < reviewMinNotesPerSecond {
		score += (reviewMinNotesPerSecond - review.NotesPerSecond) * 3
	}
	return score
}

// containsIssue 判断问题码是否已记录。
func containsIssue(issues []string, target string) bool {
	for _, issue := range issues {
		if issue == target {
			return true
		}
	}
	return false
}

// sortedKeys 返回升序乐器编号。
func sortedKeys(set map[int]struct{}) []int {
	keys := make([]int, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	return keys
}

// diagnosisHints 问题码到英文重写提示的映射。
var diagnosisHints = map[string]string{
	issueNoNotes:     "Output only valid music event tokens with a clear melody.",
	issueMalformed:   "Emit only well-formed note events; do not output text.",
	issueUnordered:   "Keep note onset times in increasing order.",
	issueTooShort:    "Continue to the full requested number of bars and end with a complete cadence.",
	issueTooLong:     "Keep the piece within the requested number of bars.",
	issueDense:       "Use a sparse texture: one clear melody line with light accompaniment instead of thick chord clusters.",
	issueNarrowPitch: "Use a wider melodic range with varied pitches and stepwise motion.",
	issueTooDense:    "Slow the harmonic rhythm and leave space between phrases.",
	issueSparse:      "Add a simple accompaniment so the texture is not empty.",
}

// String 输出便于日志阅读的一行摘要。
func (r Review) String() string {
	return fmt.Sprintf("tokens=%d notes=%d malformed=%d span=%.1fs density=%.1f/s overlap=%d pitches=%d issues=%v score=%.1f",
		r.Tokens, r.Notes, r.Malformed, r.SpanSeconds, r.NotesPerSecond,
		r.MaxOverlap, r.DistinctPitches, r.Issues, r.Score)
}
