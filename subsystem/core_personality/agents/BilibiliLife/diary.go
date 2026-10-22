package BilibiliLife

// diary.go 刷B站日记：句柄文档与 B 站动态共用同一份文案。
//
// 一轮只写一段「标题 + 主角视频链接 + 简述」：
//   - 简述 2-4 行，10 行是硬上限而不是目标（简述就该短，不用满）；
//   - 同一份文案两处落地：documents/月华刷B站日记.md（新日记置顶）与 B 站动态；
//   - 文档经 ResourceAdmitDocument 登记：文本类按「同名即同一份文档」复用既有句柄，
//     @D.xxx 不随内容变化，回执行号按本段实际行数计算。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"CorePersonality/engine/persistence"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
)

// diaryDocRel 日记文档在 local_data 下的相对路径（句柄体系按该路径复用同一句柄）
const diaryDocRel = "documents/月华刷B站日记.md"

// diaryNotifier 生活回执回调：由 lunar_loop 注册（把回执送进月华未读队列）。
// BilibiliLife 反向 import lunar_loop 会成环，经回调注入。
var diaryNotifier func(message string)

// SetDiaryNotifier 注册日记回执回调。
func SetDiaryNotifier(fn func(message string)) { diaryNotifier = fn }

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

// noteLines 一段日记的全部行：标题 + 主角视频链接 + 简述。
//
// 文档与动态都从这一份行列表派生（文档只在标题前加 ##），保证两处是同一份文案。
func noteLines(title, videoURL, brief string) []string {
	lines := make([]string, 0, dynamicSummaryMaxLines+2)
	lines = append(lines, title)
	if link := strings.TrimSpace(videoURL); link != "" {
		lines = append(lines, link)
	}
	return append(lines, normalizeBrief(brief)...)
}

// buildDynamicText 拼 B 站动态正文（行式格式，无 markdown、无空行）。
func buildDynamicText(title, videoURL, brief string) string {
	return strings.Join(noteLines(title, videoURL, brief), "\n")
}

// appendDiaryDocument 把一段新日记插入句柄文档顶部，返回文档句柄与本段总行数。
//
// 新日记写在旧日记前面：最新一段永远从文档第 1 行开始，回执行号因此可直接指向它。
func appendDiaryDocument(title, videoURL, brief string) (string, int, error) {
	rel := filepath.FromSlash(diaryDocRel)
	abs := filepath.Join(*GeneralConfig.LocalDir, rel)

	// 读取既有日记（文件不存在视为空文档，从本段起建）
	var old string
	if raw, err := os.ReadFile(abs); err == nil {
		old = strings.TrimPrefix(string(raw), "\uFEFF")
	}

	lines := noteLines(title, videoURL, brief)
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
	LoggerGeneral.Info("BilibiliLife", "日记已写入句柄文档 %s|L1-%d", result.Handle, total)
	return result.Handle, total, nil
}

// diaryReceiptText 生成写给月华的固定回执：日记落在哪个句柄的哪些行。
//
// 结尾用条件式（想聊就说说）而不是疑问句：回执以 role=user 进未读队列，
// 疑问句会逼她每轮都作答，变成没头没脑的虚空应答；条件式让她自己决定要不要开口。
func diaryReceiptText(handle string, lines int) string {
	return fmt.Sprintf("月华刚刚刷了一会B站，日记写在 %s|L1-%d 了，可用于回溯刷了什么视频；想聊就说说刚看了什么", handle, lines)
}
