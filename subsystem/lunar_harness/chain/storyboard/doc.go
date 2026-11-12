package storyboard

// doc.go 演绎产物目录与路径整理。
//
// 约定：
//   - 图片在合成阶段直接写入 multimedia/generated/<项目标题>/，配音写入 audios/generated/<项目标题>/；
//   - 最终产物是模板拼接的幻灯片 HTML（documents/lens/<项目标题>.html），
//     用相对路径指向图片与配音；
//   - 标题相同的工作复用同一个标题名，冲突时追加序号，绝不覆盖已有作品。

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// docResult 演绎产物的落点。
type docResult struct {
	// Path 相对 LocalDir 的物理路径
	Path string
	// Handle 文档资源句柄
	Handle string
}

// prepareOutputFolder 生成本次演绎的标题名：优先使用标题，同名产物已存在时追加序号。
// 图片目录、配音目录与幻灯片文件共用该标题名，保证三者始终配套。
func prepareOutputFolder(title string) string {
	base := safeFileName(title)
	if base == "" {
		base = "未命名演绎"
	}
	folder := base
	for suffix := 2; suffix < 1000; suffix++ {
		imagesExist := pathExists(path.Join(imageOutputRoot, folder))
		audiosExist := pathExists(path.Join(voiceOutputRoot, folder))
		docsExist := pathExists(path.Join(docRootRel, folder+slideFileExt))
		if !imagesExist && !audiosExist && !docsExist {
			return folder
		}
		folder = fmt.Sprintf("%s-%d", base, suffix)
	}
	return fmt.Sprintf("%s-%d", base, time.Now().Unix())
}

// shotImageLink 把图片句柄解析为相对幻灯片目录的链接。
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

// pathExists 判断相对 LocalDir 的路径是否存在。
func pathExists(rel string) bool {
	_, err := os.Stat(filepath.Join(config.LocalDir, filepath.FromSlash(rel)))
	return err == nil
}
