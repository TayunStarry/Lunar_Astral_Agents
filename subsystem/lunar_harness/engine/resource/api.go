package resource

// api.go 句柄系统的对外门面：大纲 / 解析 / 列表 / 搜索 / 摘要 / 删除。
//
// 这些接口只做「初始化检查 + 目录操作编排」，具体 SQL 在 query.go / search.go。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
)

// FindByURL 按链接地址查找已登记的 @U 资源。
func FindByURL(rawURL string) (Resource, bool) {
	if err := EnsureReady(); err != nil {
		return Resource{}, false
	}
	return archiveDB.FindResourceByURL(strings.TrimSpace(rawURL))
}

// LocalPathURL 把本机 /file/read/ 地址（裸路径或完整 http 地址）解析为本地绝对路径。
func LocalPathURL(rawURL string) (string, bool) {
	abs, err := resolveLocalSource(rawURL)
	if err != nil {
		return "", false
	}
	return abs, true
}

// IsLocalVideoURL 判断 @U 地址是否指向本机视频文件。
func IsLocalVideoURL(rawURL string) bool {
	abs, ok := LocalPathURL(rawURL)
	if !ok {
		return false
	}
	return KindOfExt(filepath.Ext(abs)) == KindVideo
}

// IsVideoURL 判断链接是否为本机或远程视频地址（按路径扩展名）。
func IsVideoURL(rawURL string) bool {
	if IsLocalVideoURL(rawURL) {
		return true
	}
	raw := strings.TrimSpace(rawURL)
	if !strings.HasPrefix(strings.ToLower(raw), "http://") && !strings.HasPrefix(strings.ToLower(raw), "https://") {
		return false
	}
	path := raw
	if cut := strings.IndexAny(path, "?#"); cut >= 0 {
		path = path[:cut]
	}
	return KindOfExt(filepath.Ext(path)) == KindVideo
}

// Outline 返回资源的大纲（段落表 + 每节的规范锚点句柄）
func Outline(rawHandle string) ([]Section, string, error) {
	if err := EnsureReady(); err != nil {
		return nil, "", err
	}
	handle, ok := ParseHandle(rawHandle)
	if !ok || handle.Seq <= 0 {
		return nil, "", fmt.Errorf("%s", ErrHandleInvalid)
	}
	res, found := archiveDB.GetResourceByHandle(handle.Kind, handle.Seq)
	if !found {
		return nil, "", fmt.Errorf("%s", ErrHandleInvalid)
	}
	// 句柄存在但实体不可读：移除失效句柄并返回统一提示
	if abs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(res.BlobPath)); res.BlobPath != "" && !fileExists(abs) {
		_ = archiveDB.DeleteResource(res.ID)
		return nil, "", fmt.Errorf("%s", ErrFileLost)
	}
	sections, err := archiveDB.ListSections(res.ID)
	if err != nil {
		return nil, "", err
	}
	// 段落表为空但 blob 在位：现场按标题解析并回填（迁移/历史资源首次访问时补建）
	if len(sections) == 0 && res.Kind == KindDocument && res.State == StateActive {
		abs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(res.BlobPath))
		if data, err := os.ReadFile(abs); err == nil {
			drafts := ParseMarkdownSections(stripBOM(string(data)))
			if len(drafts) > 0 {
				if err := archiveDB.ReplaceSections(res.ID, drafts); err == nil {
					sections, _ = archiveDB.ListSections(res.ID)
				}
			}
		}
	}
	return sections, res.Handle, nil
}

// Resolve 按句柄解析资源目录条目（前端按句柄定位文件时使用）
//
// 只解析目录行，不读取正文；调用方可用返回的 BlobPath/Name 定位实际文件。
func Resolve(rawHandle string) (Resource, error) {
	if err := EnsureReady(); err != nil {
		return Resource{}, err
	}
	handle, ok := ParseHandle(rawHandle)
	if !ok || handle.Seq <= 0 {
		return Resource{}, fmt.Errorf("%s", ErrHandleInvalid)
	}
	res, found := archiveDB.GetResourceByHandle(handle.Kind, handle.Seq)
	if !found {
		return Resource{}, fmt.Errorf("%s", ErrHandleInvalid)
	}
	if abs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(res.BlobPath)); res.BlobPath != "" && !fileExists(abs) {
		_ = archiveDB.DeleteResource(res.ID)
		return Resource{}, fmt.Errorf("%s", ErrFileLost)
	}
	return res, nil
}

// Lookup 只读取资源目录行，不校验本体、不删除失效句柄。
//
// 供记忆检索使用：记忆查询只应读取已登记的资料与摘要，不应因为本体不在位而改动目录。
func Lookup(rawHandle string) (Resource, bool) {
	if err := EnsureReady(); err != nil {
		return Resource{}, false
	}
	handle, ok := ParseHandle(rawHandle)
	if !ok || handle.Seq <= 0 {
		return Resource{}, false
	}
	return archiveDB.GetResourceByHandle(handle.Kind, handle.Seq)
}

// List 列出资源目录（kind 为空表示全部）
func List(kind string, limit int) ([]Resource, error) {
	if err := EnsureReady(); err != nil {
		return nil, err
	}
	return archiveDB.ListResources(kind, limit)
}

// Search 跨资源搜索
func Search(query, kind string, limit int) ([]SearchHit, error) {
	if err := EnsureReady(); err != nil {
		return nil, err
	}
	return archiveDB.SearchResources(query, kind, limit)
}

// HandleOf 由类型码与序号构造规范句柄（供 Go 侧调用方使用）
func HandleOf(kind string, seq int64) string {
	return FormatHandle(kind, seq)
}

// ByHash 按类型码与内容哈希解析资源
func ByHash(kind, hash string) (Resource, bool) {
	if err := EnsureReady(); err != nil {
		return Resource{}, false
	}
	return archiveDB.FindResourceByHash(kind, hash)
}

// SetSummary 写入资源摘要/描述（图片视觉描述、文档概要）
func SetSummary(id int64, summary string, tags []string) error {
	if err := EnsureReady(); err != nil {
		return err
	}
	res, found := archiveDB.GetResourceByID(id)
	if !found {
		return fmt.Errorf("资源不存在: %d", id)
	}
	if tags == nil {
		tags = res.Tags
	}
	return archiveDB.UpdateResourceExtras(id, res.Title, summary, tags)
}

// MarkLost 标记资源 blob 已丢失（GC 回收后调用）
func MarkLost(id int64) error {
	if err := EnsureReady(); err != nil {
		return err
	}
	return archiveDB.MarkState(id, StateLost)
}

// Stats 目录统计
func Stats() (StatsReport, error) {
	if err := EnsureReady(); err != nil {
		return StatsReport{}, err
	}
	return archiveDB.Stats()
}

// ReplaceSections 全量替换资源段落表（阅读者入库与迁移重建使用）
func ReplaceSections(id int64, drafts []SectionDraft) error {
	if err := EnsureReady(); err != nil {
		return err
	}
	return archiveDB.ReplaceSections(id, drafts)
}

// Delete 显式删除资源（目录行 + blob 一并移除）
func Delete(rawHandle string) error {
	if err := EnsureReady(); err != nil {
		return err
	}
	handle, ok := ParseHandle(rawHandle)
	if !ok || handle.Seq <= 0 {
		return fmt.Errorf("%s", ErrHandleInvalid)
	}
	res, found := archiveDB.GetResourceByHandle(handle.Kind, handle.Seq)
	if !found {
		return fmt.Errorf("%s", ErrHandleInvalid)
	}
	if res.BlobPath != "" {
		abs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(res.BlobPath))
		// 显式删除：仅当没有其它目录行引用同一实体时才移除物理文件（共享实体保留）
		referenced, refErr := archiveDB.blobReferencedByOthers(res.BlobPath, res.ID)
		if refErr == nil && !referenced {
			if !fileExists(abs) {
				_ = archiveDB.DeleteResource(res.ID)
				return fmt.Errorf("%s", ErrFileLost)
			}
			_ = os.Remove(abs)
		}
	}
	return archiveDB.DeleteResource(res.ID)
}

// ForgetPath 移除某实体路径对应的全部句柄映射（实体已被外部删除时使用）
func ForgetPath(rel string) {
	RemoveHandlesForPath(rel)
}
