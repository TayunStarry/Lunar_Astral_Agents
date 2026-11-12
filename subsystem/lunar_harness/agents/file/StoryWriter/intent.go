package StoryWriter

// intent.go 上级需求判定：把“新建 / 延续”从显式参数、句柄与自然语言里确定下来。
//
// 故事管线不能把新建还是延续完全交给模型自由判断：同一句“写个故事”在不同上下文
// 里可能被理解成新开一篇或接着写。这里先做确定性判定，再把结论写进指令，模型只负责执行。

import (
	"fmt"
	"regexp"
	"strings"

	"LunarSubsystem/LunarHarness/engine/resource"
)

const (
	// ModeAuto 未显式指定，按指令、句柄与已有故事推断。
	ModeAuto = "auto"
	// ModeNew 新建一个故事。
	ModeNew = "new"
	// ModeContinue 延续已有故事。
	ModeContinue = "continue"
)

// Intent 一次故事任务的判定结果。
type Intent struct {
	Mode   string
	Story  string
	Reason string
}

var (
	// storyNamePattern 指令里点名的故事标题：《雨夜旧书店》。
	storyNamePattern = regexp.MustCompile(`《([^》]{1,40})》`)
	// chapterRefPattern 指向已有作品后续章节的表述。
	chapterRefPattern = regexp.MustCompile(`第[0-9一二三四五六七八九十百零两]+章|下一章|新的一章|再写一章|加一章|增加一章`)
	// modeMarkerPattern 上级系统写入的显式模式标记。
	modeMarkerPattern = regexp.MustCompile(`【故事模式】\s*(新建|延续)`)
	// chapterSeqPattern 章节文件编号前缀。
	chapterSeqPattern = regexp.MustCompile(`^(\d{3})-`)
)

var (
	// newStoryCues 明确要求新开一篇的表述。
	newStoryCues = []string{
		"新故事", "新的故事", "新作品", "全新的故事", "新建故事",
		"另起", "新开", "换一个故事", "换个故事", "重开一个",
	}
	// continueCues 明确要求接着写的表述。
	continueCues = []string{
		"续写", "继续写", "接着写", "继续创作", "接着上一",
		"延续上一次", "延续上次", "延续上一", "往下写", "往下续",
		"继续这个故事", "继续这个故事吧",
	}
)

// ResolveIntent 判定本次任务是新建还是延续。
//
// 优先级：显式 mode → 指令里的【故事模式】标记 → 新建语义 → 点名故事 →
// 目标句柄所属故事 → 延续语义 → 默认新建。
// 没有可延续的故事时，延续一律回落为新建，避免任务直接失败。
// 新建任务再经过 settleTarget 落到具体标题：指令点名《标题》就强制使用；
// 同名故事已存在或本次任务刚创建过故事时改为延续，避免重复建目录。
func ResolveIntent(mode, story, instruction string, handles []string, stories []StoryInfo, startedUnix int64) Intent {
	intent := resolveMode(mode, story, instruction, handles, stories)
	return settleTarget(intent, instruction, stories, startedUnix)
}

// resolveMode 只判定模式，不处理新建目标标题。
func resolveMode(mode, story, instruction string, handles []string, stories []StoryInfo) Intent {
	mode = normalizeMode(mode)
	instruction = strings.TrimSpace(instruction)

	switch mode {
	case ModeNew:
		return Intent{Mode: ModeNew, Reason: "上级明确要求新建"}
	case ModeContinue:
		return continueIntent(story, instruction, handles, stories, "上级明确要求延续")
	}

	if marker := modeMarker(instruction); marker != "" {
		if marker == ModeNew {
			return Intent{Mode: ModeNew, Reason: "指令标记为新建"}
		}
		return continueIntent(story, instruction, handles, stories, "指令标记为延续")
	}

	// 新建语义先于点名判断：「别续写《X》了，写个新故事」应当新建。
	if hasAny(instruction, newStoryCues) {
		return Intent{Mode: ModeNew, Reason: "指令含新建语义"}
	}
	if name := namedStory(instruction); name != "" {
		if matched := matchStory(name, stories); matched != "" {
			return Intent{Mode: ModeContinue, Story: matched, Reason: "指令点名已有故事《" + matched + "》"}
		}
	}
	if name := storyFromHandles(handles, stories); name != "" {
		return Intent{Mode: ModeContinue, Story: name, Reason: "目标句柄属于该故事"}
	}
	if hasAny(instruction, continueCues) || hasChapterRef(instruction) {
		return continueIntent("", instruction, handles, stories, "指令含延续语义")
	}
	if len(stories) == 0 {
		return Intent{Mode: ModeNew, Reason: "还没有任何故事"}
	}
	return Intent{Mode: ModeNew, Reason: "没有延续信号，按新建处理"}
}

// settleTarget 把新建任务落到具体标题。
//
//   - 指令里点名了标题（如“写个新故事《雾港灯塔》”）→ 强制使用该标题；
//   - 该标题已存在 → 改为延续，避免同名目录重复创建；
//   - 没点名但本次任务已经建过故事（重试场景）→ 延续该故事，不再新建。
func settleTarget(intent Intent, instruction string, stories []StoryInfo, startedUnix int64) Intent {
	if intent.Mode != ModeNew {
		return intent
	}
	if title := explicitTitle(instruction); title != "" {
		if matched := matchStory(title, stories); matched != "" {
			return Intent{Mode: ModeContinue, Story: matched, Reason: "标题《" + matched + "》已存在，改为延续以避免重复目录"}
		}
		intent.Story = title
		intent.Reason += "；标题取自指令《" + title + "》"
		return intent
	}
	if recent := recentStory(stories, startedUnix); recent != "" {
		return Intent{Mode: ModeContinue, Story: recent, Reason: "本次任务已创建《" + recent + "》，继续在该故事目录内完成"}
	}
	return intent
}

// recentStory 取出本次任务开始后创建或更新过的故事（列表已按更新时间倒序）。
func recentStory(stories []StoryInfo, startedUnix int64) string {
	if startedUnix <= 0 {
		return ""
	}
	for _, info := range stories {
		if info.UpdatedUnix >= startedUnix {
			return info.Name
		}
	}
	return ""
}

// continueIntent 解析延续目标：显式故事名 → 指令点名 → 句柄 → 最近更新的故事。
func continueIntent(storyName, instruction string, handles []string, stories []StoryInfo, reason string) Intent {
	if len(stories) == 0 {
		return Intent{Mode: ModeNew, Reason: reason + "；但没有可延续的故事，改为新建"}
	}
	if matched := matchStory(storyName, stories); matched != "" {
		return Intent{Mode: ModeContinue, Story: matched, Reason: reason}
	}
	if name := namedStory(instruction); name != "" {
		if matched := matchStory(name, stories); matched != "" {
			return Intent{Mode: ModeContinue, Story: matched, Reason: reason}
		}
	}
	if name := storyFromHandles(handles, stories); name != "" {
		return Intent{Mode: ModeContinue, Story: name, Reason: reason}
	}
	return Intent{Mode: ModeContinue, Story: stories[0].Name, Reason: reason + "；取最近更新的故事"}
}

// taskBlock 把判定结果写成模型必须遵守的任务说明。
func taskBlock(intent Intent, stories []StoryInfo) string {
	var b strings.Builder
	if intent.Mode == ModeNew {
		b.WriteString("【任务判定】新建故事。\n")
		b.WriteString("本次必须新开一个故事目录，不得续写已有故事。\n")
		if intent.Story != "" {
			b.WriteString("故事标题已由系统确定为《")
			b.WriteString(intent.Story)
			b.WriteString("》，必须原样使用，不得改名或换成其他标题。\n")
		} else {
			b.WriteString("请自行拟定一个不超过 16 个汉字的新标题，且不得与已有故事重名。\n")
		}
		if len(stories) > 0 {
			names := make([]string, 0, len(stories))
			for _, info := range stories {
				names = append(names, info.Name)
			}
			b.WriteString("已有故事（仅用于避免重名）：")
			b.WriteString(strings.Join(names, "、"))
			b.WriteString("。\n")
		}
	} else {
		b.WriteString("【任务判定】延续故事《")
		b.WriteString(intent.Story)
		b.WriteString("》。\n")
		b.WriteString("故事目录：" + storyRoot + "/")
		b.WriteString(intent.Story)
		b.WriteString("\n")
		if info := findStory(stories, intent.Story); info != nil {
			b.WriteString(fmt.Sprintf("已有 %d 章，下一章编号至少 %03d。\n", info.Chapters, info.LastChapter+1))
		}
		b.WriteString("不要新建故事目录，不要整篇重写大纲；先读大纲、进度和最后一章，再写下一章。\n")
	}
	b.WriteString("【判定依据】")
	b.WriteString(intent.Reason)
	b.WriteString("\n")
	b.WriteString("以上判定已由系统确定，不要自行改成另一种模式；执行中发现信息不一致时，在总结里说明并沿用本判定。\n")
	return b.String()
}

// normalizeMode 兼容英文与中文模式值，未知值按自动处理。
func normalizeMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ModeNew, "新建", "新写":
		return ModeNew
	case ModeContinue, "延续", "续写":
		return ModeContinue
	default:
		return ModeAuto
	}
}

// modeMarker 读取指令里的【故事模式】标记。
func modeMarker(instruction string) string {
	match := modeMarkerPattern.FindStringSubmatch(instruction)
	if len(match) != 2 {
		return ""
	}
	if match[1] == "延续" {
		return ModeContinue
	}
	return ModeNew
}

// namedStory 提取指令里《》包裹的故事标题。
func namedStory(instruction string) string {
	match := storyNamePattern.FindStringSubmatch(instruction)
	if len(match) != 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

// explicitTitle 提取本次新建要用的标题：跳过被“续写 / 继续写”等引出的旧故事名。
func explicitTitle(instruction string) string {
	for _, loc := range storyNamePattern.FindAllStringSubmatchIndex(instruction, -1) {
		prefix := strings.TrimSpace(instruction[:loc[0]])
		if refersPrevious(prefix) {
			continue
		}
		if title := sanitizeTitle(instruction[loc[2]:loc[3]]); title != "" {
			return title
		}
	}
	return ""
}

// refersPrevious 判断书名前的语境是否在指代已有故事（续写、接着写、别续写），
// 或其实是在说章节名（“第一章叫《无人小船》”）。
func refersPrevious(prefix string) bool {
	for _, cue := range []string{"续写", "继续写", "接着写", "继续创作", "别写", "不要写", "别再写"} {
		if strings.HasSuffix(prefix, cue) {
			return true
		}
	}
	runes := []rune(prefix)
	if len(runes) > 6 {
		runes = runes[len(runes)-6:]
	}
	return strings.Contains(string(runes), "章")
}

// sanitizeTitle 清理标题中的路径分隔符与非法字符，去掉首尾空白和点。
func sanitizeTitle(title string) string {
	title = strings.TrimSpace(title)
	title = strings.Map(func(r rune) rune {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|', '\r', '\n', '\t':
			return -1
		}
		return r
	}, title)
	title = strings.Trim(title, " .。，,、;；")
	return strings.TrimSpace(title)
}

// matchStory 在已有故事里按标题精确匹配（忽略大小写）。
func matchStory(name string, stories []StoryInfo) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	for _, info := range stories {
		if strings.EqualFold(strings.TrimSpace(info.Name), name) {
			return info.Name
		}
	}
	return ""
}

// storyFromHandles 从句柄指向的路径反推故事名。
func storyFromHandles(handles []string, stories []StoryInfo) string {
	for _, handle := range handles {
		entry, err := resource.Resolve(handle)
		if err != nil {
			continue
		}
		rel := strings.Trim(strings.ReplaceAll(entry.BlobPath, "\\", "/"), "/")
		rest, ok := strings.CutPrefix(rel, storyRoot+"/")
		if !ok {
			continue
		}
		name, _, ok := strings.Cut(rest, "/")
		if ok && matchStory(name, stories) != "" {
			return matchStory(name, stories)
		}
	}
	return ""
}

// findStory 按标题取故事摘要。
func findStory(stories []StoryInfo, name string) *StoryInfo {
	for index := range stories {
		if strings.EqualFold(strings.TrimSpace(stories[index].Name), strings.TrimSpace(name)) {
			return &stories[index]
		}
	}
	return nil
}

// hasAny 指令是否包含任一关键词；被「不要 / 别 / 无需」否定的关键词不算。
func hasAny(instruction string, cues []string) bool {
	for _, cue := range cues {
		offset := 0
		for {
			index := strings.Index(instruction[offset:], cue)
			if index < 0 {
				break
			}
			index += offset
			if !negatedAt(instruction, index) {
				return true
			}
			offset = index + len(cue)
		}
	}
	return false
}

// hasChapterRef 是否出现指向后续章节的表述（同样排除被否定的情况）。
func hasChapterRef(instruction string) bool {
	for _, loc := range chapterRefPattern.FindAllStringIndex(instruction, -1) {
		if !negatedAt(instruction, loc[0]) {
			return true
		}
	}
	return false
}

// negatedAt 判断关键词前是否紧邻否定词。
func negatedAt(text string, index int) bool {
	runes := []rune(text[:index])
	if len(runes) == 0 {
		return false
	}
	last := string(runes[len(runes)-1])
	if last == "不" || last == "别" || last == "勿" {
		return true
	}
	if len(runes) >= 2 {
		switch string(runes[len(runes)-2:]) {
		case "不要", "不用", "无需", "不必", "别去":
			return true
		}
	}
	return false
}
