package creation

// path_snapshot.go 调度者的路径树快照：只列 documents / unknown 的目录与文件名，
// 用提示词里的工具坐标（documents → /document）逐行输出完整路径，
// 用于减少规划阶段「不知道有哪些文件、反复 list_dir 试探」的无效调用。
//
// 快照只注入调度者的规划提示，不进入执行体上下文；执行体仍然按需读取具体文件。
// 体积控制：正文字符上限 pathSnapshotMaxRunes，超限截断并显式标注。
// 刷新策略：缓存 TTL 兜底外部改动；创作链每次成功写入后立即失效，保证下一轮看到自己的产物。

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"LunarSubsystem/LunarHarness/engine/config"
)

// pathSnapshotText 返回可注入规划提示的路径树快照（命中缓存时直接复用）。
func pathSnapshotText() string {
	key := strings.TrimSpace(config.LocalDir)
	if key == "" {
		return ""
	}
	key = filepath.Clean(key)

	pathSnapshotMu.Lock()
	defer pathSnapshotMu.Unlock()
	if pathSnapshotMemo.Key == key && time.Since(pathSnapshotMemo.BuiltAt) < pathSnapshotTTL {
		return pathSnapshotMemo.Text
	}
	text := buildPathSnapshot(key)
	pathSnapshotMemo = pathSnapshotCache{Key: key, Text: text, BuiltAt: time.Now()}
	return text
}

// invalidatePathSnapshot 立即失效缓存；文件增删改后调用，避免下一轮拿到旧树。
func invalidatePathSnapshot() {
	pathSnapshotMu.Lock()
	pathSnapshotMemo.BuiltAt = time.Time{}
	pathSnapshotMu.Unlock()
}

// buildPathSnapshot 扫描两个根并渲染成受限长度的树形文本。
func buildPathSnapshot(localRoot string) string {
	entries := make([]pathSnapshotNode, 0, 128)
	exceeded := false
	counts := make([]pathRootCount, len(pathSnapshotRoots))
	for index, root := range pathSnapshotRoots {
		counts[index] = scanPathRoot(localRoot, root, &entries, &exceeded)
	}
	return renderPathSnapshot(counts, entries, exceeded)
}

// scanPathRoot 递归扫描单个根目录；只记录相对路径与是否为目录，不读取文件内容或元数据。
func scanPathRoot(localRoot, root string, entries *[]pathSnapshotNode, exceeded *bool) pathRootCount {
	count := pathRootCount{}
	start := filepath.Join(localRoot, filepath.FromSlash(root))
	info, err := os.Stat(start)
	if err != nil || !info.IsDir() {
		return count
	}
	_ = filepath.WalkDir(start, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry == nil {
			return nil
		}
		if current == start {
			*entries = append(*entries, pathSnapshotNode{Rel: root, Dir: true})
			return nil
		}
		if len(*entries) >= pathSnapshotMaxEntries {
			*exceeded = true
			return filepath.SkipAll
		}
		rel, relErr := filepath.Rel(localRoot, current)
		if relErr != nil {
			return nil
		}
		if entry.IsDir() {
			count.Dirs++
		} else {
			count.Files++
		}
		*entries = append(*entries, pathSnapshotNode{
			Rel: filepath.ToSlash(rel),
			Dir: entry.IsDir(),
		})
		return nil
	})
	return count
}

// renderPathSnapshot 渲染统计头与路径列表；超出字符上限时截断并标注。
//
// 路径统一用提示词里的工具坐标（documents → /document），逐行完整可复制，
// 避免模型在 documents/… 与 /document/… 两套写法之间反复试探。
func renderPathSnapshot(counts []pathRootCount, entries []pathSnapshotNode, exceeded bool) string {
	var header strings.Builder
	for index, root := range pathSnapshotRoots {
		fmt.Fprintf(&header, "%s：%d 个文件 / %d 个目录\n", displayRoot(root), counts[index].Files, counts[index].Dirs)
	}
	if exceeded {
		fmt.Fprintf(&header, "（扫描达到 %d 条上限，统计与列表均不完整）\n", pathSnapshotMaxEntries)
	}
	header.WriteString("unknown 是独立存储根，不在 /document 下；以下路径可直接用作工具参数\n")
	header.WriteString("--- 已有路径 ---\n")

	var body strings.Builder
	body.WriteString(header.String())
	used := utf8.RuneCountInString(header.String())
	truncated := exceeded
	for _, entry := range entries {
		line := displayPath(entry)
		need := utf8.RuneCountInString(line) + 1
		if used+need+utf8.RuneCountInString(pathSnapshotTruncated) > pathSnapshotMaxRunes {
			truncated = true
			break
		}
		body.WriteString(line)
		body.WriteByte('\n')
		used += need
	}
	if truncated {
		body.WriteString(pathSnapshotTruncated)
	}
	return body.String()
}

// displayRoot 把内部存储根换成提示词使用的根路径写法。
func displayRoot(root string) string {
	if root == documentsRootRel {
		return "/document"
	}
	return root
}

// displayPath 把相对 LocalDir 的路径渲染成提示词坐标（documents/… → /document/…）。
func displayPath(entry pathSnapshotNode) string {
	rel := entry.Rel
	if rel == documentsRootRel || strings.HasPrefix(rel, documentsRootRel+"/") {
		rel = "/document" + strings.TrimPrefix(rel, documentsRootRel)
	}
	if entry.Dir && !strings.HasSuffix(rel, "/") {
		rel += "/"
	}
	return rel
}
