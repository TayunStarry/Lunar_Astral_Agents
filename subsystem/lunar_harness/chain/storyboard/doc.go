package storyboard

// doc.go 演绎文档生成与图片路径整理。
//
// 约定：
//   - 图片在绘制阶段直接写入 multimedia/generated/<项目标题>/，不再统一迁移；
//   - 分镜文档写入 documents/lens/<项目标题>/分镜.md，用相对路径指向图片；
//   - 标题相同的任务复用同一目录，冲突时追加序号，绝不覆盖已有作品。

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"LunarSubsystem/LunarHarness/agents/FileEditor"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// docResult 分镜文档的落点。
type docResult struct {
	// Path 相对 LocalDir 的物理路径
	Path string
	// Handle 文档资源句柄
	Handle string
}

// prepareOutputFolder 生成本次演绎的目录名：优先使用标题，标题目录已存在时追加序号。
// 图片与文档共用该目录名，保证 multimedia/generated/<标题> 与 documents/lens/<标题> 对齐。
func prepareOutputFolder(title string) string {
	base := safeFileName(title)
	if base == "" {
		base = "未命名演绎"
	}
	folder := base
	for suffix := 2; suffix < 1000; suffix++ {
		imagesExist := pathExists(path.Join("multimedia/generated", folder))
		docsExist := pathExists(path.Join(docRootRel, folder))
		if !imagesExist && !docsExist {
			return folder
		}
		folder = fmt.Sprintf("%s-%d", base, suffix)
	}
	return fmt.Sprintf("%s-%d", base, time.Now().Unix())
}

// writeStoryboardDoc 生成分镜 Markdown 文档，并把成功镜头映射为相对图片路径。
func writeStoryboardDoc(plan storyboardPlan, shots []ShotView, folder string) (docResult, []ShotView, error) {
	if folder == "" {
		folder = prepareOutputFolder(plan.Title)
	}
	docDirRel := path.Join(docRootRel, folder)
	for index := range shots {
		if shots[index].Status != shotPainted || shots[index].Handle == "" {
			continue
		}
		link, err := shotImageLink(docDirRel, shots[index].Handle)
		if err != nil {
			shots[index].Status = shotFailed
			shots[index].Error = "图片路径解析失败：" + err.Error()
			shots[index].File = ""
			continue
		}
		shots[index].File = link
	}

	content := buildMarkdown(plan, shots)
	docRel := path.Join(docDisplayRoot, folder, "分镜.md")
	result := FileEditor.CreateFile(docRel, content)
	if !result.OK {
		return docResult{}, shots, fmt.Errorf("分镜文档写入失败：%s", strings.TrimSpace(result.Summary))
	}
	handle := ""
	if len(result.Handles) > 0 {
		handle = result.Handles[0]
	}
	return docResult{
		Path:   path.Join(docRootRel, folder, "分镜.md"),
		Handle: handle,
	}, shots, nil
}

// shotImageLink 把图片句柄解析为相对分镜文档目录的链接。
func shotImageLink(docDirRel, handle string) (string, error) {
	entry, err := resource.Resolve(handle)
	if err != nil {
		return "", fmt.Errorf("解析资源 %s 失败：%w", handle, err)
	}
	srcRel := path.Clean(strings.ReplaceAll(strings.TrimSpace(entry.BlobPath), "\\", "/"))
	if srcRel == "" || srcRel == "." {
		return "", fmt.Errorf("资源 %s 没有本地文件", handle)
	}
	if _, statErr := os.Stat(filepath.Join(config.LocalDir, filepath.FromSlash(srcRel))); statErr != nil {
		return "", fmt.Errorf("图片不存在：%w", statErr)
	}
	link, err := filepath.Rel(filepath.FromSlash(docDirRel), filepath.FromSlash(srcRel))
	if err != nil {
		return "", fmt.Errorf("计算相对路径失败：%w", err)
	}
	return filepath.ToSlash(link), nil
}

// safeFileName 去掉 Windows 文件名非法字符并限制长度。
func safeFileName(title string) string {
	title = strings.TrimSpace(title)
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '<', '>', ':', '"', '/', '\\', '|', '?', '*':
			return -1
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, title)
	cleaned = strings.Trim(cleaned, " .")
	runes := []rune(cleaned)
	if len(runes) > 40 {
		cleaned = string(runes[:40])
	}
	return strings.TrimSpace(cleaned)
}

// buildMarkdown 生成分镜文档正文。
func buildMarkdown(plan storyboardPlan, shots []ShotView) string {
	success := 0
	failed := 0
	for _, shot := range shots {
		if shot.Status == shotPainted && shot.File != "" {
			success++
		} else {
			failed++
		}
	}
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(plan.Title)
	b.WriteString(" · 分镜脚本\n\n")
	b.WriteString("- 生成时间：")
	b.WriteString(time.Now().Format("2006-01-02 15:04:05"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("- 分镜总数：%d（成功 %d / 失败 %d）\n", len(shots), success, failed))
	if plan.Style != "" {
		b.WriteString("- 画风约束：")
		b.WriteString(plan.Style)
		b.WriteString("\n")
	}
	b.WriteString("\n")

	if failed > 0 {
		b.WriteString("## 失败清单\n\n")
		for _, shot := range shots {
			if shot.Status == shotPainted && shot.File != "" {
				continue
			}
			reason := strings.TrimSpace(shot.Error)
			if reason == "" {
				reason = "未产出图片文件"
			}
			b.WriteString(fmt.Sprintf("- 第 %d 镜（%s）：%s\n", shot.Index, shot.Scene, reason))
		}
		b.WriteString("\n")
	}

	b.WriteString("## 分镜规划\n")
	for _, shot := range shots {
		b.WriteString(fmt.Sprintf("\n### 第 %d 镜", shot.Index))
		if shot.Act != "" {
			b.WriteString(" · ")
			b.WriteString(shot.Act)
		}
		b.WriteString(" · 镜次 ")
		b.WriteString(shot.Scene)
		b.WriteString("\n\n")
		b.WriteString("**叙事**：")
		b.WriteString(shot.Narration)
		b.WriteString("\n\n")
		b.WriteString("**提示词**：\n\n```text\n")
		b.WriteString(shot.Prompt)
		b.WriteString("\n```\n\n")
		if shot.Status == shotPainted && shot.File != "" {
			b.WriteString("![第 ")
			b.WriteString(strconv.Itoa(shot.Index))
			b.WriteString(" 镜](<")
			b.WriteString(shot.File)
			b.WriteString(">)\n\n")
			b.WriteString("> 文件：`")
			b.WriteString(shot.File)
			b.WriteString("`")
			if shot.Handle != "" {
				b.WriteString("（句柄 `")
				b.WriteString(shot.Handle)
				b.WriteString("`）")
			}
			b.WriteString("\n")
		} else {
			reason := strings.TrimSpace(shot.Error)
			if reason == "" {
				reason = "未产出图片文件"
			}
			b.WriteString("> ⚠ 本镜生成失败：")
			b.WriteString(reason)
			b.WriteString("（没有可指向的图片文件）\n")
		}
	}
	return b.String()
}

// pathExists 判断相对 LocalDir 的路径是否存在。
func pathExists(rel string) bool {
	_, err := os.Stat(filepath.Join(config.LocalDir, filepath.FromSlash(rel)))
	return err == nil
}
