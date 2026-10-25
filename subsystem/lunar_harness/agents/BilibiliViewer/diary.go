package BilibiliViewer

// diary.go 观影日记：本地一份三段式文档，每轮结束都按模板重写。
//
//	# 月华的哔哩哔哩观影日记
//	# 刚刚观看的视频   —— 最近一轮看过的视频（标题 + 链接），没看视频时保留上一轮内容
//	# 动态日记        —— 最近一次发到 B 站动态的正文，没发动态时保留上一轮内容
//	# 历史记忆        —— 跨轮累计的自我记忆，每轮重写，参与后续选片/编排/互动文案
//
// 返回给对话者的句柄指向「刚刚观看的视频 → 动态日记」这一段，
// 月华用 @D.xxx|L起-止 就能一次读到本轮看了什么、发了什么。

import (
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

const (
	// diaryDocRel 日记文档在 local_data 下的相对路径（句柄体系按该路径复用同一句柄）
	diaryDocRel = "documents/月华刷B站日记.md"
	// diaryFileTitle 文档总标题
	diaryFileTitle = "# 月华的哔哩哔哩观影日记"
	// 三段的小标题
	diaryVideosHeading  = "# 刚刚观看的视频"
	diaryDynamicHeading = "# 动态日记"
	diaryMemoryHeading  = "# 历史记忆"
	// diaryVideoSeparator 视频条目之间的分隔线
	diaryVideoSeparator = "---------"
	// memoryMaxRunes 历史记忆的字数上限（超出由模型提炼，这里再兜底截断）
	memoryMaxRunes = 4096
	// diaryEmptyPlaceholder 段落为空时的占位（避免句柄区间落到空段落）
	diaryEmptyPlaceholder = "（暂无）"
)

// diaryVideo 日记里的一条视频：标题与链接分开两行。
type diaryVideo struct {
	Title string
	URL   string
}

// diarySections 已落盘日记的三段内容
type diarySections struct {
	Videos  []string // 原样行（- 标题 / * 链接 / 分隔线），没看视频时直接复用
	Dynamic string   // 最近一次动态正文
	Memory  string   // 历史记忆
	Legacy  string   // 旧模板全文（首次迁移时用于提炼记忆）
}

// normalizeDynamic 规整动态正文：去首个空行、压缩连续空行、最多 10 行、去掉 @。
//
// B 站动态支持换行排版，保留一个空行分段即可；行数上限防止模型写出长文。
func normalizeDynamic(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "@", "")
	lines := make([]string, 0, dynamicSummaryMaxLines)
	blank := true
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if blank {
				continue
			}
			blank = true
			lines = append(lines, "")
		} else {
			blank = false
			lines = append(lines, line)
		}
		if len(lines) >= dynamicSummaryMaxLines {
			break
		}
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// clampMemory 历史记忆兜底截断到上限
func clampMemory(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if text == "" {
		return ""
	}
	return truncate(text, memoryMaxRunes)
}

// buildVideoLines 生成「刚刚观看的视频」段落：每条视频 - 标题 / * 链接，条目之间加分隔线。
func buildVideoLines(videos []diaryVideo) []string {
	lines := make([]string, 0, len(videos)*3)
	seen := make(map[string]bool, len(videos))
	for _, video := range videos {
		title := strings.TrimSpace(video.Title)
		link := strings.TrimSpace(video.URL)
		key := link
		if key == "" {
			key = title
		}
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if len(lines) > 0 {
			lines = append(lines, diaryVideoSeparator)
		}
		switch {
		case title != "" && link != "":
			lines = append(lines, "- 《"+title+"》", "* "+link)
		case link != "":
			lines = append(lines, "* "+link)
		default:
			lines = append(lines, "- 《"+title+"》")
		}
	}
	return lines
}

// escapeDiaryHeadings 防止正文里出现与段落标题相同的行，破坏下次解析
func escapeDiaryHeadings(lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		switch strings.TrimSpace(line) {
		case diaryFileTitle, diaryVideosHeading, diaryDynamicHeading, diaryMemoryHeading:
			out[i] = " " + line
		default:
			out[i] = line
		}
	}
	return out
}

// loadDiarySections 读取并拆分日记文件；旧模板文件整体收进 Legacy 供迁移提炼。
func loadDiarySections() diarySections {
	abs := filepath.Join(config.LocalDir, filepath.FromSlash(diaryDocRel))
	raw, err := os.ReadFile(abs)
	if err != nil {
		return diarySections{}
	}
	text := strings.TrimPrefix(string(raw), "\uFEFF")
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	index := make(map[string]int, 3)
	for i, line := range lines {
		switch strings.TrimSpace(line) {
		case diaryVideosHeading:
			index[diaryVideosHeading] = i
		case diaryDynamicHeading:
			index[diaryDynamicHeading] = i
		case diaryMemoryHeading:
			index[diaryMemoryHeading] = i
		}
	}
	if len(index) == 0 {
		return diarySections{Legacy: strings.TrimSpace(text)}
	}
	section := func(heading string) []string {
		start, ok := index[heading]
		if !ok {
			return nil
		}
		end := len(lines)
		for other, pos := range index {
			if other != heading && pos > start && pos < end {
				end = pos
			}
		}
		body := lines[start+1 : end]
		for len(body) > 0 && strings.TrimSpace(body[0]) == "" {
			body = body[1:]
		}
		for len(body) > 0 && strings.TrimSpace(body[len(body)-1]) == "" {
			body = body[:len(body)-1]
		}
		out := make([]string, 0, len(body))
		for _, line := range body {
			out = append(out, strings.TrimRight(line, " \t"))
		}
		return out
	}
	return diarySections{
		Videos:  section(diaryVideosHeading),
		Dynamic: strings.Join(section(diaryDynamicHeading), "\n"),
		Memory:  strings.Join(section(diaryMemoryHeading), "\n"),
	}
}

// loadDiaryMemory 只取历史记忆（供提示词注入）；没有记忆时返回空串。
func loadDiaryMemory() string {
	return strings.TrimSpace(loadDiarySections().Memory)
}

// writeDiaryFile 按模板重写整份日记，返回文档句柄与「刚刚观看的视频 → 动态日记」的行区间。
//
// videos 非空时用它覆盖视频段；为空时沿用 keptVideos（没看视频不更新 URL）。
// dynamic / memory 为空时分别写占位行，保证句柄区间稳定可读。
func writeDiaryFile(videos []diaryVideo, keptVideos []string, dynamic, memory string) (string, int, int, error) {
	rel := filepath.FromSlash(diaryDocRel)
	abs := filepath.Join(config.LocalDir, rel)

	lines := []string{diaryFileTitle, ""}
	start := len(lines) + 1 // 1 基：指向「# 刚刚观看的视频」
	lines = append(lines, diaryVideosHeading)
	videoLines := buildVideoLines(videos)
	if len(videoLines) == 0 {
		videoLines = keptVideos
	}
	if len(videoLines) == 0 {
		videoLines = []string{diaryEmptyPlaceholder}
	}
	lines = append(lines, videoLines...)

	lines = append(lines, "", diaryDynamicHeading)
	dynamicLines := strings.Split(normalizeDynamic(dynamic), "\n")
	if len(dynamicLines) == 1 && dynamicLines[0] == "" {
		dynamicLines = []string{diaryEmptyPlaceholder}
	}
	lines = append(lines, escapeDiaryHeadings(dynamicLines)...)
	end := len(lines) // 1 基：「动态日记」最后一行

	lines = append(lines, "", diaryMemoryHeading)
	memoryLines := strings.Split(clampMemory(memory), "\n")
	if len(memoryLines) == 1 && memoryLines[0] == "" {
		memoryLines = []string{diaryEmptyPlaceholder}
	}
	lines = append(lines, escapeDiaryHeadings(memoryLines)...)

	content := strings.Join(lines, "\n") + "\n"
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", 0, 0, err
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return "", 0, 0, err
	}
	result, err := persistence.ResourceAdmitDocument(diaryDocRel, filepath.Base(diaryDocRel))
	if err != nil {
		return "", 0, 0, err
	}
	logger.Info("BilibiliViewer", "观影日记已更新 %s|L%d-%d（视频 %d 条，记忆 %d 字）",
		result.Handle, start, end, len(buildVideoLines(videos)), len([]rune(clampMemory(memory))))
	return result.Handle, start, end, nil
}
