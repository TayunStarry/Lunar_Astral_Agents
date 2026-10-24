package BilibiliLife

// diary.go 刷B站日记：句柄文档与 B 站动态共用同一份文案。
//
// 一轮只写一段「标题 + 本轮全部视频（标题与 URL 同行） + 简述」：
//   - 每个视频一行《标题》 URL，模型能直接看出哪一行 URL 对应哪个标题；
//   - 链接逐条列举这一轮看过的每个视频（不只是发 @ 的那一条），可按任一 URL 回溯摘要库；
//   - 简述 2-4 行，10 行是硬上限而不是目标（简述就该短，不用满）；
//   - 同一份文案两处落地：documents/月华刷B站日记.md（新日记置顶）与 B 站动态；
//     呼叫用户不写进正文（纯文本 @ 构建不出真实呼叫关系），改由视频评论区完成；
//   - 文档经 ResourceAdmitDocument 登记：文本类按「同名即同一份文档」复用既有句柄，
//     @D.xxx 不随内容变化，回执行号按本段实际行数计算。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

// diaryDocRel 日记文档在 local_data 下的相对路径（句柄体系按该路径复用同一句柄）
const diaryDocRel = "documents/月华刷B站日记.md"

// normalizeBrief 规整简述：去空行、最多 10 行，**不补齐**。
//
// 10 行是硬上限而不是目标：简述该多短就多短，补空行会让它看起来像 10 行的长文。
func normalizeBrief(text string) []string {
	lines := make([]string, 0, dynamicSummaryMaxLines)
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lines = append(lines, trimmed)
		if len(lines) >= dynamicSummaryMaxLines {
			break
		}
	}
	return lines
}

// diaryVideo 日记里的一条视频：标题与 URL 写在同一行。
type diaryVideo struct {
	Title string
	URL   string
}

// noteLines 一段日记的全部行：标题 + 本轮看过的全部视频（标题与 URL 同行） + 简述。
//
// 每条视频一行《标题》 URL，月华之后能直接看出哪一行 URL 对应哪个标题，
// 也能按任意一条 URL 查视频摘要库回溯。
// 文档与动态都从这一份行列表派生（文档只在标题前加 ##），保证两处是同一份文案。
func noteLines(title string, videos []diaryVideo, brief string) []string {
	lines := make([]string, 0, len(videos)+dynamicSummaryMaxLines+1)
	lines = append(lines, title)
	seen := make(map[string]bool, len(videos))
	for _, video := range videos {
		videoTitle := strings.TrimSpace(video.Title)
		link := strings.TrimSpace(video.URL)
		key := link
		if key == "" {
			key = videoTitle
		}
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		switch {
		case videoTitle != "" && link != "":
			lines = append(lines, fmt.Sprintf("《%s》 %s", videoTitle, link))
		case link != "":
			lines = append(lines, link)
		default:
			lines = append(lines, "《"+videoTitle+"》")
		}
	}
	return append(lines, normalizeBrief(brief)...)
}

// buildDynamicText 拼 B 站动态正文（行式格式，无 markdown、无空行）。
func buildDynamicText(title string, videos []diaryVideo, brief string) string {
	return strings.Join(noteLines(title, videos, brief), "\n")
}

// appendDiaryDocument 把一段新日记插入句柄文档顶部，返回文档句柄与本段总行数。
//
// 新日记写在旧日记前面：最新一段永远从文档第 1 行开始，回执行号因此可直接指向它。
func appendDiaryDocument(title string, videos []diaryVideo, brief string) (string, int, error) {
	rel := filepath.FromSlash(diaryDocRel)
	abs := filepath.Join(config.LocalDir, rel)

	// 读取既有日记（文件不存在视为空文档，从本段起建）
	var old string
	if raw, err := os.ReadFile(abs); err == nil {
		old = strings.TrimPrefix(string(raw), "\uFEFF")
	}

	lines := noteLines(title, videos, brief)
	var builder strings.Builder
	fmt.Fprintf(&builder, "## %s\n", lines[0])
	for _, line := range lines[1:] {
		builder.WriteString(line)
		builder.WriteString("\n")
	}
	builder.WriteString(old)

	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", 0, err
	}
	if err := os.WriteFile(abs, []byte(builder.String()), 0o644); err != nil {
		return "", 0, err
	}

	result, err := persistence.ResourceAdmitDocument(diaryDocRel, filepath.Base(diaryDocRel))
	if err != nil {
		return "", 0, fmt.Errorf("日记文档登记句柄失败: %w", err)
	}
	total := len(lines)
	logger.Info("BilibiliLife", "日记已写入句柄文档 %s|L1-%d", result.Handle, total)
	return result.Handle, total, nil
}
