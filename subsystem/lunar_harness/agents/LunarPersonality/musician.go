package LunarPersonality

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	musicHeaderRegex = regexp.MustCompile(`(?m)^[A-Za-z]:.*$`)
	musicVoiceRegex  = regexp.MustCompile(`\[[Vv]:\d+\]`)
	musicDynamicsRe  = regexp.MustCompile(`![^!|]*!`)
	musicEndRegex    = regexp.MustCompile(`\|\]|:\||\|\|`)
	musicNoteRegex   = regexp.MustCompile(`[A-Ga-gz]`)
)

// newMusicianRole 创建演奏者角色
func newMusicianRole() *MusicianRole {
	builder := newModelBuilder(readPrompt("prompts/musicianRole.md"), "演奏者", 40)
	return &MusicianRole{ModelBuilder: builder, MaxIterations: 4, MinMelodyBars: 12, DefaultTempo: 100}
}

func (role *MusicianRole) createCreativeWork(taskDescription string) string {
	role.Draft = nil
	var detail *MusicPieceDetail
	result := runCreativeLoop("演奏者", role.ModelBuilder, taskDescription, musicToolDefinitions(), role.MaxIterations,
		func(call ToolCallItem) string { return role.executeTool(call) },
		func(call ToolCallItem) {
			if call.Function.Name == "submit_chords" && role.AssembledDetail != nil {
				copyValue := *role.AssembledDetail
				detail = &copyValue
				role.AssembledDetail = nil
			}
		},
		func() string {
			if detail == nil {
				return ""
			}
			return buildMusicSummary([]MusicPieceDetail{*detail})
		},
	)
	return result
}

func (role *MusicianRole) executeTool(call ToolCallItem) string {
	args := parseToolArgs(call.Function.Arguments)
	if role.Draft != nil && role.Draft.Chords != "" {
		return "作品已创作完成，无需重复提交任何声部"
	}
	switch call.Function.Name {
	case "submit_melody":
		return role.submitMelody(args)
	case "submit_accompaniment":
		return role.submitAccompaniment(args)
	case "submit_chords":
		return role.submitChords(args)
	default:
		return "未知工具: " + call.Function.Name + "，请按流程依次调用 submit_melody → submit_accompaniment → submit_chords"
	}
}

func (role *MusicianRole) submitMelody(args map[string]any) string {
	title := strings.TrimSpace(stringArg(args, "title"))
	melody := sanitizeFragment(stringArg(args, "melody"))
	if title == "" {
		return "主旋律提交失败：作品标题不能为空"
	}
	if melody == "" {
		return "主旋律提交失败：旋律内容不能为空"
	}
	bars := countBars(melody)
	if bars < role.MinMelodyBars {
		return fmt.Sprintf("主旋律提交失败：当前仅 %d 小节，音乐展开不足。请扩展至至少 %d 小节后重新提交", bars, role.MinMelodyBars)
	}
	tempo := int(numberArg(args, "tempo"))
	if tempo <= 0 {
		tempo = role.DefaultTempo
	}
	role.Draft = &MusicDraft{
		Title: title, Structure: stringArg(args, "structure"), Key: defaultString(stringArg(args, "key"), "C"),
		Meter: defaultString(stringArg(args, "meter"), "4/4"), Tempo: tempo, Melody: melody,
		MelodyInstrument: defaultString(stringArg(args, "instrument"), "钢琴"), MelodyBars: bars,
	}
	return fmt.Sprintf("主旋律提交成功：共 %d 小节。下一步请调用 submit_accompaniment 生成伴奏声部，小节数必须恰好为 %d 小节", bars, bars)
}

func (role *MusicianRole) submitAccompaniment(args map[string]any) string {
	if role.Draft == nil {
		return "伴奏提交失败：尚未提交主旋律，请先调用 submit_melody"
	}
	accompaniment := sanitizeFragment(stringArg(args, "accompaniment"))
	if accompaniment == "" {
		return "伴奏提交失败：伴奏内容不能为空"
	}
	bars := countBars(accompaniment)
	if bars != role.Draft.MelodyBars {
		return fmt.Sprintf("伴奏提交失败：伴奏为 %d 小节，与主旋律 %d 小节不一致。请逐小节对齐后重新提交", bars, role.Draft.MelodyBars)
	}
	role.Draft.Accompaniment = accompaniment
	role.Draft.AccompanimentInstrument = defaultString(stringArg(args, "instrument"), role.Draft.MelodyInstrument)
	return fmt.Sprintf("伴奏提交成功：%d 小节，与主旋律对齐。最后一步请调用 submit_chords 生成和弦进行声部", bars)
}

func (role *MusicianRole) submitChords(args map[string]any) string {
	if role.Draft == nil {
		return "和弦提交失败：尚未提交主旋律，请先调用 submit_melody"
	}
	if role.Draft.Accompaniment == "" {
		return "和弦提交失败：尚未提交伴奏声部，请先调用 submit_accompaniment"
	}
	chords := sanitizeFragment(stringArg(args, "chords"))
	if chords == "" {
		return "和弦提交失败：和弦进行内容不能为空"
	}
	bars := countBars(chords)
	if bars != role.Draft.MelodyBars {
		return fmt.Sprintf("和弦提交失败：和弦进行为 %d 小节，与主旋律 %d 小节不一致", bars, role.Draft.MelodyBars)
	}
	role.Draft.Chords = chords
	role.Draft.ChordInstrument = defaultString(stringArg(args, "instrument"), "贝斯")
	abc, instruments := assembleScore(*role.Draft)
	pushAssistantContext("music", abc, "", false)
	detail := &MusicPieceDetail{
		Title: role.Draft.Title, Instruments: instruments, Tempo: role.Draft.Tempo,
		Structure: role.Draft.Structure, Key: role.Draft.Key, Meter: role.Draft.Meter, ABCLength: role.Draft.MelodyBars,
	}
	role.AssembledDetail = detail
	role.Draft = nil
	return fmt.Sprintf("和弦进行提交成功，作品《%s》创作完成：三声部已对齐拼装（共 %d 小节）。乐谱已推送到前端。", detail.Title, detail.ABCLength)
}

func sanitizeFragment(fragment string) string {
	value := musicHeaderRegex.ReplaceAllString(fragment, "")
	value = musicVoiceRegex.ReplaceAllString(value, "")
	value = strings.Join(strings.Fields(value), " ")
	return strings.TrimSpace(value)
}

func countBars(fragment string) int {
	body := musicHeaderRegex.ReplaceAllString(fragment, "")
	body = musicVoiceRegex.ReplaceAllString(body, "")
	body = musicDynamicsRe.ReplaceAllString(body, "")
	body = musicEndRegex.ReplaceAllString(body, "|")
	count := 0
	for _, segment := range strings.Split(body, "|") {
		if musicNoteRegex.MatchString(segment) {
			count++
		}
	}
	return count
}

func assembleScore(draft MusicDraft) (string, string) {
	instruments := make([]string, 0, 3)
	for _, item := range []string{draft.MelodyInstrument, draft.AccompanimentInstrument, draft.ChordInstrument} {
		if value := strings.TrimSpace(item); value != "" {
			instruments = append(instruments, value)
		}
	}
	instrumentText := strings.Join(instruments, ",")
	abc := strings.Join([]string{
		"X:1",
		"T:" + draft.Title,
		"M:" + draft.Meter,
		"L:1/8",
		fmt.Sprintf("Q:1/4=%d", draft.Tempo),
		"K:" + draft.Key,
		"[V:1] " + draft.Melody,
		"[V:2] " + draft.Accompaniment,
		"[V:3] " + draft.Chords,
		"",
	}, "\n")
	return injectInstrumentDirective(abc, instrumentText), instrumentText
}

func injectInstrumentDirective(abc, instruments string) string {
	if instruments == "" {
		return abc
	}
	instruments = strings.ReplaceAll(instruments, "，", ",")
	list := make([]string, 0)
	for _, item := range strings.Split(instruments, ",") {
		if value := strings.TrimSpace(item); value != "" {
			list = append(list, value)
		}
	}
	if len(list) == 0 {
		return abc
	}
	if regexp.MustCompile(`(?m)^%%voice\s+`).MatchString(abc) {
		return abc
	}
	directives := make([]string, 0, len(list))
	for i, item := range list {
		directives = append(directives, fmt.Sprintf("%%voice %d %s", i+1, item))
	}
	directive := strings.Join(directives, "\n") + "\n"
	if match := regexp.MustCompile(`(?m)^X:\s*\d+`).FindStringIndex(abc); match != nil {
		return abc[:match[0]] + directive + abc[match[0]:]
	}
	return directive + abc
}

func buildMusicSummary(pieces []MusicPieceDetail) string {
	if len(pieces) == 0 {
		return ""
	}
	parts := make([]string, 0, len(pieces)+1)
	for _, piece := range pieces {
		desc := "月华演奏了《" + piece.Title + "》"
		details := make([]string, 0)
		if piece.Instruments != "" {
			details = append(details, "使用"+piece.Instruments)
		}
		if piece.Key != "" {
			mode := "大调"
			if piece.Key == strings.ToLower(piece.Key) {
				mode = "小调"
			}
			details = append(details, piece.Key+mode)
		}
		if piece.Tempo > 0 {
			details = append(details, fmt.Sprintf("%dBPM", piece.Tempo))
		}
		if piece.Structure != "" {
			details = append(details, "结构为"+piece.Structure)
		}
		if len(details) > 0 {
			desc += "（" + strings.Join(details, "，") + "）"
		}
		parts = append(parts, desc+"。")
	}
	parts = append(parts, "乐谱已通过音乐播放器推送给用户，可以查看和播放。")
	return strings.Join(parts, "\n")
}

// musicToolDefinitions 演奏者工具定义
func musicToolDefinitions() []ToolCall {
	const lineRule = "仅一行音符，用 | 分隔小节，以 |] 结尾"
	const noHeader = "禁止包含 X:/T:/M:/L:/Q:/K: 等头部字段与 [V:N] 声部标记"
	const sameBars = "小节数必须与主旋律完全一致、逐小节对齐"
	return []ToolCall{
		functionTool("submit_melody", "创作流程第一步：提交完整的主旋律声部。",
			map[string]any{
				"title":      map[string]any{"type": "string", "description": "音乐作品标题"},
				"instrument": map[string]any{"type": "string", "description": "主旋律乐器"},
				"tempo":      map[string]any{"type": "number", "description": "演奏速度（BPM），默认100"},
				"key":        map[string]any{"type": "string", "description": "调式，如 C、G、D、F、a"},
				"meter":      map[string]any{"type": "string", "description": "拍号，如 4/4、3/4、6/8"},
				"structure":  map[string]any{"type": "string", "description": "段落结构"},
				"melody":     map[string]any{"type": "string", "description": "主旋律ABC片段：" + lineRule + "。" + noHeader},
			}, []string{"title", "melody"}),
		functionTool("submit_accompaniment", "创作流程第二步：在已提交的主旋律基础上生成伴奏声部。",
			map[string]any{
				"instrument":    map[string]any{"type": "string", "description": "伴奏乐器"},
				"accompaniment": map[string]any{"type": "string", "description": "伴奏ABC片段：" + lineRule + "，" + sameBars + "。" + noHeader},
			}, []string{"accompaniment"}),
		functionTool("submit_chords", "创作流程第三步（最后一步）：生成和弦进行声部，完成整部作品。",
			map[string]any{
				"instrument": map[string]any{"type": "string", "description": "和弦声部乐器"},
				"chords":     map[string]any{"type": "string", "description": "和弦进行ABC片段：" + lineRule + "，" + sameBars + "。" + noHeader},
			}, []string{"chords"}),
	}
}
