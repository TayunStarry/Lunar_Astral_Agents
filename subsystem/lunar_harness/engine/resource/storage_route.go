package resource

// storage_route.go 存储根归位：不可读二进制统一进 unknown/
//
// documents/ 只保留可解析的文本类实体；压缩包、可执行文件、模型权重与未知扩展名
// 一律归入 unknown/，既避免文档目录被二进制混杂，也让它们仍按 @D 句柄可回引。
// 前端 /file/write 与归档入库共用同一套判定。

import (
	"path"
	"path/filepath"
	"strings"
)

// isBinaryStorageExt 扩展名是否按「不可读二进制」归档。
//
// 媒体类走多媒体树、文本白名单与可识别文本类型留在 documents；PDF / Office 等
// 可解析的文档虽为二进制也留在 documents；其余（含未知扩展名）视为不可读二进制。
// 判定只看扩展名，不做文件头探测，保证落盘前即可决定目录。
func isBinaryStorageExt(ext string) bool {
	ext = strings.ToLower(strings.TrimSpace(ext))
	if ext == "" {
		return true
	}
	if IsMediaExtension(ext) || documentBinaryExtensions[ext] {
		return false
	}
	if binaryExtensions[ext] {
		return true
	}
	if workspaceTextExtensions[ext] {
		return false
	}
	return KindOfExt(ext) == ""
}

// RouteStorageRel 把写入路径归位到正确的存储根。
//
//   - documents/ 下的二进制 → unknown/同名文件
//   - 裸路径（相对 LocalDir）下的二进制 → unknown/<原相对路径>
//   - 其它存储根（multimedia / audios / unknown）与文本类路径原样返回
func RouteStorageRel(rel string) string {
	rel = normalizeRel(rel)
	if rel == "" {
		return rel
	}
	ext := strings.ToLower(filepath.Ext(rel))
	if !isBinaryStorageExt(ext) {
		return rel
	}
	root := rel
	if idx := strings.Index(rel, "/"); idx >= 0 {
		root = rel[:idx]
	}
	switch {
	case root == StorageRootUnknown:
		return rel
	case IsStoragePath(rel):
		if root != StorageRootDocuments {
			return rel
		}
		return path.Join(StorageRootUnknown, safeDocumentName(filepath.Base(rel), "", ext))
	default:
		return path.Join(StorageRootUnknown, filepath.ToSlash(rel))
	}
}
