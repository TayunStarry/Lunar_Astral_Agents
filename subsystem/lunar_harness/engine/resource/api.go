package resource

// api.go 句柄系统的对外门面：解析 / 大纲 / 列表 / 搜索 / 摘要 / 删除
//
// 这些接口只做「初始化检查 + 编排」，具体 SQL 在 url_master.go / handles.go / query.go。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
)

// FindByURL 按链接地址查找已登记资源（不要求已绑定句柄）
func FindByURL(rawURL string) (Resource, bool) {
	if err := EnsureReady(); err != nil {
		return Resource{}, false
	}
	return archiveDB.FindResourceByURL(strings.TrimSpace(rawURL))
}

// LocalPathURL 把 /file/read/ 地址（相对路由或完整 http 地址）解析为本地绝对路径
func LocalPathURL(rawURL string) (string, bool) {
	abs, err := resolveLocalSource(rawURL)
	if err != nil {
		return "", false
	}
	return abs, true
}

// IsLocalVideoURL 判断地址是否指向本机视频文件（按本地字节魔数校验）
func IsLocalVideoURL(rawURL string) bool {
	abs, ok := LocalPathURL(rawURL)
	if !ok {
		return false
	}
	return DetectStorageKind(ClassifyInput{
		SourceKind: SourceKindPath,
		Source:     abs,
		LocalPath:  abs,
	}).Kind == KindVideo
}

// IsVideoURL 只对本地实体做字节校验；远程链接的类型必须由发送侧声明，不在此推断。
func IsVideoURL(rawURL string) bool {
	return IsLocalVideoURL(rawURL)
}

// Outline 返回资源大纲（段落表 + 规范句柄）
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
	if res.BlobPath != "" {
		if !fileExists(localAbs(res.BlobPath)) {
			// 有文本描述：句柄保留（只降级 lost）；无描述：进入 GC 表 1
			_ = archiveDB.preserveOrArchive(res, gcOrphanReason(res))
			if !hasTextDescription(res) {
				return nil, "", fmt.Errorf("%s", ErrFileLost)
			}
		}
	}
	archiveDB.markGCCurrentCall(res.Handle)
	sections, err := archiveDB.ListSectionsByURL(res.URL)
	if err != nil {
		return nil, "", err
	}
	// 段落表为空但实体在位：现场解析并回填
	if len(sections) == 0 && res.Kind == KindDocument && res.State == StateActive && res.BlobPath != "" {
		abs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(res.BlobPath))
		if data, readErr := os.ReadFile(abs); readErr == nil {
			drafts := ParseMarkdownSections(stripBOM(string(data)))
			if len(drafts) > 0 {
				if err := archiveDB.ReplaceSections(res.ID, drafts); err == nil {
					sections, _ = archiveDB.ListSectionsByURL(res.URL)
				}
			}
		}
	}
	return sections, res.Handle, nil
}

// Resolve 按句柄解析资源条目（不读取正文）
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
	if res.BlobPath != "" {
		if !fileExists(localAbs(res.BlobPath)) {
			_ = archiveDB.preserveOrArchive(res, gcOrphanReason(res))
			if !hasTextDescription(res) {
				return Resource{}, fmt.Errorf("%s", ErrFileLost)
			}
			res.State = StateLost
		}
	}
	archiveDB.markGCCurrentCall(res.Handle)
	return res, nil
}

// Lookup 只读取资源记录，不校验实体、不删除失效句柄
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

// List 列出资源（kind 为空表示全部）
func List(kind string, limit int) ([]Resource, error) {
	if err := EnsureReady(); err != nil {
		return nil, err
	}
	return archiveDB.ListResources(trimKind(kind), limit)
}

// ListMatching 按文件名 / 路径 / 地址 / 标题子串过滤资源（kind 为空表示全部）
//
// 与 Search 的区别：只做目录级过滤（不检索解析文本与段落），返回完整资源记录，
// 供「按路径或文件夹名列出句柄」使用。
func ListMatching(kind string, keywords []string, limit int) ([]Resource, error) {
	if err := EnsureReady(); err != nil {
		return nil, err
	}
	return archiveDB.ListResourcesMatching(trimKind(kind), keywords, limit)
}

// Search 跨资源搜索
func Search(query, kind string, limit int) ([]SearchHit, error) {
	if err := EnsureReady(); err != nil {
		return nil, err
	}
	return archiveDB.SearchResources(query, trimKind(kind), limit)
}

// HandleOf 由类型码与序号构造规范句柄
func HandleOf(kind string, seq int64) string {
	return FormatHandle(kind, seq)
}

// PointsOf 返回句柄在 GC 表中记录的调用积分（无记录视为 0）
func PointsOf(handle string) int {
	if handle == "" || EnsureReady() != nil {
		return 0
	}
	return archiveDB.pointsOf(handle)
}

// GCTrackedPath 判断路径是否在积分回收范围（multimedia 根媒体 / audios 根哈希音频）
func GCTrackedPath(rel, kind string) bool {
	rel = normalizeRel(rel)
	return gcTrackablePath(rel, kind)
}

// GCCyclesLeft 持续未使用时，距积分回收还剩几次结算
func GCCyclesLeft(points int) int {
	left := points - GCPointsMin
	if left < 0 {
		return 0
	}
	return left
}

// ByHash 按类型码与内容哈希解析资源
func ByHash(kind, hash string) (Resource, bool) {
	if err := EnsureReady(); err != nil {
		return Resource{}, false
	}
	return archiveDB.FindResourceByHash(kind, hash)
}

// SetSummary 写入资源解析文本 / 标题标签
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

// MarkLost 标记资源实体已丢失（GC 回收后调用）
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

// ReplaceSections 全量替换资源段落表
func ReplaceSections(id int64, drafts []SectionDraft) error {
	if err := EnsureReady(); err != nil {
		return err
	}
	return archiveDB.ReplaceSections(id, drafts)
}

// Delete 显式删除资源（解除句柄绑定；归档私有副本一并移除）
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
	// 只有归档自己写入的副本可以删；前端/用户文件必须保留
	if res.BlobOwned && res.BlobPath != "" && requiresBlobRemoval(res) {
		abs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(res.BlobPath))
		_ = os.Remove(abs)
	}
	return archiveDB.DeleteResource(res.ID)
}

// requiresBlobRemoval 是否允许删除实体（派生缓存永不由句柄层删除）
func requiresBlobRemoval(res Resource) bool {
	return IsStoragePath(res.BlobPath) && !IsDerivedStoragePath(res.BlobPath)
}

// ForgetPath 移除某实体路径对应的全部句柄映射
func ForgetPath(rel string) {
	RemoveHandlesForPath(rel)
}

// ParseTextOfResource 读取资源记录上的解析文本（未解析返回空串）
func ParseTextOfResource(rawHandle string) string {
	res, ok := Lookup(rawHandle)
	if !ok || res.ParseState != ParseReady {
		return ""
	}
	return strings.TrimSpace(res.ParseText)
}

// RefreshDocumentSections 重新解析文档段落（阅读者导入后调用）
func RefreshDocumentSections(handle string) {
	res, err := Resolve(handle)
	if err != nil || res.Kind != KindDocument || res.BlobPath == "" {
		return
	}
	abs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(res.BlobPath))
	refreshDocumentSections(res, abs)
}
