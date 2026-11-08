package storyboard

// doc.go 分镜图片迁移与 Markdown 文档生成。
//
// 约定：
//   - 文档与图片落在同一目录，文档用相对文件名指向图片；
//   - 目标目录/文件写入前做同名检测，冲突时追加序号，绝不覆盖；
//   - 迁移失败的镜头改记为失败，不生成悬空链接。

import (
	"fmt"
	"io"
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
	"LunarSubsystem/LunarHarness/engine/resourcehub"
)

// docResult 分镜文档的落点。
type docResult struct {
	// Path 相对 LocalDir 的物理路径
	Path string
	// Handle 文档资源句柄
	Handle string
}

// writeStoryboardDoc 把成功镜头图片移动到文档目录，再写出 Markdown 文档。
func writeStoryboardDoc(plan storyboardPlan, shots []ShotView) (docResult, []ShotView, error) {
	dirName, dirRel := uniqueDocDir(plan.Title)
	for index := range shots {
		if shots[index].Status != shotPainted || shots[index].Handle == "" {
			continue
		}
		file, err := moveShotImage(shots[index].Handle, dirRel, fmt.Sprintf("shot-%02d", shots[index].Index))
		if err != nil {
			shots[index].Status = shotFailed
			shots[index].Error = "图片迁移失败：" + err.Error()
			shots[index].File = ""
			continue
		}
		shots[index].File = file
	}

	content := buildMarkdown(plan, shots)
	docRel := path.Join(docDisplayRoot, dirName, "分镜.md")
	result := FileEditor.CreateFile(docRel, content)
	if !result.OK {
		return docResult{}, shots, fmt.Errorf("分镜文档写入失败：%s", strings.TrimSpace(result.Summary))
	}
	handle := ""
	if len(result.Handles) > 0 {
		handle = result.Handles[0]
	}
	return docResult{
		Path:   path.Join(docRootRel, dirName, "分镜.md"),
		Handle: handle,
	}, shots, nil
}

// uniqueDocDir 生成不冲突的文档目录名。
func uniqueDocDir(title string) (dirName, dirRel string) {
	base := safeFileName(title)
	if base == "" {
		base = "故事"
	}
	stamp := time.Now().Format("20060102-150405")
	dirName = base + "-" + stamp
	dirRel = path.Join(docRootRel, dirName)
	for suffix := 2; pathExists(dirRel); suffix++ {
		dirName = fmt.Sprintf("%s-%s-%d", base, stamp, suffix)
		dirRel = path.Join(docRootRel, dirName)
	}
	return dirName, dirRel
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

// moveShotImage 把绘制产物迁移到文档目录，返回迁移后的文件名。
func moveShotImage(handle, dirRel, base string) (string, error) {
	entry, err := resource.Resolve(handle)
	if err != nil {
		return "", fmt.Errorf("解析资源 %s 失败：%w", handle, err)
	}
	srcRel := path.Clean(strings.ReplaceAll(strings.TrimSpace(entry.BlobPath), "\\", "/"))
	if srcRel == "" || srcRel == "." {
		return "", fmt.Errorf("资源 %s 没有本地文件", handle)
	}
	srcAbs := filepath.Join(config.LocalDir, filepath.FromSlash(srcRel))
	if _, statErr := os.Stat(srcAbs); statErr != nil {
		return "", fmt.Errorf("源图片不存在：%w", statErr)
	}

	ext := strings.ToLower(path.Ext(srcRel))
	if ext == "" {
		ext = ".png"
	}
	name := base + ext
	targetRel := path.Join(dirRel, name)
	for suffix := 2; pathExists(targetRel); suffix++ {
		name = fmt.Sprintf("%s-%d%s", base, suffix, ext)
		targetRel = path.Join(dirRel, name)
	}
	targetAbs := filepath.Join(config.LocalDir, filepath.FromSlash(targetRel))
	if err := os.MkdirAll(filepath.Dir(targetAbs), 0o755); err != nil {
		return "", fmt.Errorf("创建目录失败：%w", err)
	}
	if err := os.Rename(srcAbs, targetAbs); err != nil {
		if copyErr := copyFile(srcAbs, targetAbs); copyErr != nil {
			return "", fmt.Errorf("迁移图片失败：%w", copyErr)
		}
		if removeErr := os.Remove(srcAbs); removeErr != nil {
			return "", fmt.Errorf("清理源图片失败：%w", removeErr)
		}
	}
	if !pathExists(targetRel) {
		return "", fmt.Errorf("迁移后目标文件不存在：%s", targetRel)
	}
	resource.RewriteHandlesForMove(srcRel, targetRel)
	resourcehub.Invalidate()
	if ArtworkMoveHook != nil {
		ArtworkMoveHook(srcRel, targetRel)
	}
	return name, nil
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
	b.WriteString("# " + plan.Title + " · 分镜脚本\n\n")
	b.WriteString("- 生成时间：" + time.Now().Format("2006-01-02 15:04:05") + "\n")
	b.WriteString(fmt.Sprintf("- 分镜总数：%d（成功 %d / 失败 %d）\n", len(shots), success, failed))
	if plan.Style != "" {
		b.WriteString("- 画风约束：" + plan.Style + "\n")
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
			b.WriteString(" · " + shot.Act)
		}
		b.WriteString(" · 镜次 " + shot.Scene + "\n\n")
		b.WriteString("**叙事**：" + shot.Narration + "\n\n")
		b.WriteString("**提示词**：\n\n```text\n" + shot.Prompt + "\n```\n\n")
		if shot.Status == shotPainted && shot.File != "" {
			b.WriteString("![第 " + strconv.Itoa(shot.Index) + " 镜](./" + shot.File + ")\n\n")
			b.WriteString("> 文件：`./" + shot.File + "`")
			if shot.Handle != "" {
				b.WriteString("（句柄 `" + shot.Handle + "`）")
			}
			b.WriteString("\n")
		} else {
			reason := strings.TrimSpace(shot.Error)
			if reason == "" {
				reason = "未产出图片文件"
			}
			b.WriteString("> ⚠ 本镜生成失败：" + reason + "（没有可指向的图片文件）\n")
		}
	}
	return b.String()
}

// pathExists 判断相对 LocalDir 的路径是否存在。
func pathExists(rel string) bool {
	_, err := os.Stat(filepath.Join(config.LocalDir, filepath.FromSlash(rel)))
	return err == nil
}

// copyFile 跨卷迁移时的兜底复制。
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
