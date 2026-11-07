package resource

// storage_route.go 落盘前的目录路由：先由 Classify 得到真实类型，再选存储根
//
// 类型判定只有 Classify 一处；本文件只做「类型 → 目录」映射：
//   图片 / 动图 / 视频 → multimedia/，音频 → audios/
//   文本与可解析文档 → documents/，不可读二进制 → unknown/
// 前端 /file/write 与归档入库（snapshotRel）共用同一套类型码判定。

import (
	"bufio"
	"io"
	"path"
	"path/filepath"
	"strings"
)

// isBinaryStorageExt 扩展名（分类器归一后）是否属于不可读二进制。
//
// 只判可读性，不判类型：类型码由 Classify 给出。PDF / Office 等可解析文档
// 虽为二进制仍留在 documents/；压缩包 / 可执行 / 模型权重 / 未知扩展名进 unknown/。
func isBinaryStorageExt(ext string) bool {
	ext = normalizeExt(ext)
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
// head 为调用方预读的文件头（可为 nil）：类型判定一律交给 Classify，扩展名缺失时
// 由它按魔数识别真实类型，避免无扩展名媒体先落进 documents/ 或 unknown/。
func RouteStorageRel(rel string, head []byte) string {
	rel = normalizeRel(rel)
	if rel == "" {
		return rel
	}
	cls := classifyStorageRel(rel, head)
	ext := cls.Ext
	if ext == "" {
		ext = normalizeExt(filepath.Ext(rel))
	}
	if isMediaKind(cls.Kind) {
		return routeMediaRel(rel, cls.Kind, ext)
	}
	if !isBinaryStorageExt(ext) {
		return rel
	}
	root := storageRootOf(rel)
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

// RouteStorageReader 在写入前决定落点，并把可能预读过的字节原样交回调用方。
//
// IO 权衡：扩展名已足以确定类型时（媒体 / 已知文档 / 已知二进制 / 文本白名单）
// 直接按扩展名路由，零额外读取，流式上传可以边收边写；只有扩展名缺失或未知时
// 才用 bufio 预读一个文件头的字节做魔数嗅探。预读字节留在缓冲区里，下游 io.Copy
// 直接复用，不存在第二次读取；代价是这类无扩展名上传要等文件头（或 EOF）到达后
// 才开始落盘。
func RouteStorageReader(rel string, body io.Reader) (string, io.Reader) {
	rel = normalizeRel(rel)
	if rel == "" || body == nil {
		return rel, body
	}
	if extDecidesStorage(filepath.Ext(rel)) {
		return RouteStorageRel(rel, nil), body
	}
	buffered := bufio.NewReaderSize(body, classifyHeadBytes)
	head, _ := buffered.Peek(classifyHeadBytes)
	return RouteStorageRel(rel, head), buffered
}

// classifyStorageRel 用存储层归位判定存储树内的相对路径（不联网、不读盘）
func classifyStorageRel(rel string, head []byte) ClassifyResult {
	return DetectStorageKind(ClassifyInput{
		SourceKind: SourceKindBytes,
		Source:     rel,
		Name:       filepath.Base(rel),
		Head:       head,
	})
}

// routeMediaRel 媒体按真实类型归入两条内容树；已在媒体树内的路径原样保留
func routeMediaRel(rel, kind, ext string) string {
	root := storageRootOf(rel)
	if root != StorageRootDocuments && root != StorageRootUnknown {
		return rel
	}
	base := safeDocumentName(filepath.Base(rel), "", ext)
	if kind == KindAudio {
		return path.Join(StorageRootAudios, base)
	}
	return path.Join(StorageRootMultimedia, base)
}

// extDecidesStorage 扩展名是否已足以确定存储根（无需再嗅探文件头）
func extDecidesStorage(rawExt string) bool {
	ext := normalizeExt(rawExt)
	if ext == "" {
		return false
	}
	return IsMediaExtension(ext) || binaryExtensions[ext] || documentBinaryExtensions[ext] ||
		workspaceTextExtensions[ext] || KindOfExt(ext) != ""
}

// storageRootOf 取路径首段作为存储根
func storageRootOf(rel string) string {
	if before, _, ok := strings.Cut(rel, "/"); ok {
		return before
	}
	return rel
}
