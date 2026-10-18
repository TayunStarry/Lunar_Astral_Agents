package module

// archive_admit.go 统一入库入口 —— 全模态资源进入系统的唯一通道
//
// 设计依据：docs/adr/0001「统一入库入口」。
// 三类来源归一：
//   1. path  已是本地文件（前端上传已落盘）→ 只登记不复制，零拷贝
//   2. url   远程地址（QQ 等渠道仅给 URL）→ 下载后按下方命名规则落盘
//   3. data  字节流 / data URI（工具产出、转发内联）→ 落盘后登记
//
// 哈希定义：SHA-256 前 16 位十六进制（与前端 calculateFileHash 既有约定一致）。
// 落盘命名分两类：
//   - 媒体（图片/视频/音频）：multimedia/<hash><ext>，内容寻址，同内容只存一份
//   - 文本类：documents/<原文件名>，同名即同一份文档，便于人机两端按名称传递与讨论

import (
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// 来源类型
const (
	SourceKindPath      = "path"      // 本地文件路径（相对 LocalDir 或绝对路径或 file:// 或本机 http(s):// 服务地址）
	SourceKindURL       = "url"       // 远程地址
	SourceKindData      = "data"      // data URI
	SourceKindBytes     = "bytes"     // 原始字节
	SourceKindWorkspace = "workspace" // 工作区文件（用户直接引用的项目内文件，按只读方式收录）
)

// workspaceTextExtensions 允许按需收录的工作区文本类扩展名（白名单，防止把可执行文件等读入归档）
var workspaceTextExtensions = map[string]bool{
	".md": true, ".markdown": true, ".txt": true, ".json": true, ".xml": true,
	".yaml": true, ".yml": true, ".toml": true, ".ini": true, ".cfg": true, ".conf": true,
	".log": true, ".csv": true, ".tsv": true,
	".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true, ".cjs": true,
	".py": true, ".rs": true, ".java": true, ".kt": true, ".c": true, ".h": true, ".cpp": true,
	".cxx": true, ".hpp": true, ".cs": true, ".rb": true, ".php": true, ".swift": true,
	".sh": true, ".ps1": true, ".bat": true, ".cmd": true, ".sql": true,
	".html": true, ".htm": true, ".css": true, ".vue": true, ".lua": true, ".r": true,
	".dockerfile": true, ".env": true, ".gitignore": true, ".editorconfig": true,
}

// workspaceMaxBytes 单个工作区文件收录的体积上限（100MB，与工作区大文件留出余量）
const workspaceMaxBytes = 100 * 1024 * 1024

// AdmitInput 入库请求
type AdmitInput struct {
	// SourceKind 来源类型：path / url / data / bytes
	SourceKind string
	// Source 来源值（路径 / 地址 / data URI）
	Source string
	// Data 原始字节（SourceKind 为 bytes 时使用）
	Data []byte
	// Name 建议文件名（可空：path 来源自动取基名）
	Name string
	// KindHint 类型码建议值（可空：按扩展名与 MIME 推断）
	KindHint string
	// Origin 来源语义（client-upload / channel / tool-output / engine-temp）
	Origin string
	// Retain 保留等级建议值（可空：按 origin 推断）
	Retain string
	// MIME 建议 MIME（可空：按扩展名推断）
	MIME string
	// Dedup 按内容去重：为 true 时，若已存在相同类型码 + 相同内容哈希的资源，
	// 直接复用该资源句柄，**不再新增目录行**（blob 本就内容寻址，天然同一份）。
	//
	// 为什么需要它：本层默认「每次入库一行、序号永不复用」，同一张图被反复发送
	// 就会一路占出 @I.002 / @I.003 / @I.004…。图片没有文件名这类业务身份，
	// 内容哈希就是它唯一的身份，因此由调用方显式要求按内容去重。
	Dedup bool
}

// AdmitResult 入库结果
type AdmitResult struct {
	// Handle 规范句柄（形如 @I.007）
	Handle string `json:"handle"`
	// Kind 类型码
	Kind string `json:"kind"`
	// Seq 资源序号
	Seq int64 `json:"seq"`
	// ID 资源目录自增主键
	ID int64 `json:"id"`
	// ContentHash sha16
	ContentHash string `json:"content_hash"`
	// BlobPath 相对 LocalDir 的物理路径（内容仓库位置，斜杠分隔）
	BlobPath string `json:"blob_path"`
	// MIME 规范化 MIME
	MIME string `json:"mime"`
	// SizeBytes 字节数
	SizeBytes int64 `json:"size_bytes"`
	// Name 原始文件名
	Name string `json:"name"`
	// Deduped 是否复用了既有 blob（内容已存在）
	Deduped bool `json:"deduped"`
	// State blob 状态
	State string `json:"state"`
}

// ResourceAdmit 统一入库入口：归一来源 → 规范化落盘 → 登记资源目录 → 返回句柄
func ResourceAdmit(in AdmitInput) (AdmitResult, error) {
	if err := EnsureArchiveInitialized(); err != nil {
		return AdmitResult{}, err
	}
	if in.Origin == "" {
		in.Origin = OriginClientUpload
	}
	if in.Name == "" && in.SourceKind == SourceKindPath {
		in.Name = filepath.Base(in.Source)
	}

	// 1. 把来源归一为「本地文件 + 建议文件名 + 扩展名」
	localPath, cleanup, err := normalizeAdmitSource(in)
	if err != nil {
		return AdmitResult{}, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	// 2. 内容哈希（流式，避免大文件整体入内存）
	hash, size, err := fileSHA16(localPath)
	if err != nil {
		return AdmitResult{}, fmt.Errorf("计算内容哈希失败: %v", err)
	}

	// 3. 类型码推断：显式建议 > 扩展名 > 空
	ext := strings.ToLower(filepath.Ext(in.Name))
	if ext == "" {
		ext = strings.ToLower(filepath.Ext(localPath))
	}
	kind := in.KindHint
	if kind == "" {
		kind = KindOfExt(ext)
	}
	if kind == "" {
		// 未知类型：按文档处理（纯文本兜底），保留扩展名供调用方判断
		kind = KindDocument
	}

	// 3.5 媒体按内容去重（调用方显式要求时）：命中既有资源则直接复用句柄，不落盘、不新增目录行。
	//     文本类不参与内容去重 —— 它们以原文件名为业务身份，由步骤 5.5 按路径复用。
	if in.Dedup && IsMediaExtension(ext) {
		if existing, ok := ArchiveDatabase.FindResourceByHash(kind, hash); ok {
			return AdmitResult{
				Handle:      existing.Handle,
				Kind:        existing.Kind,
				Seq:         existing.Seq,
				ID:          existing.ID,
				ContentHash: existing.ContentHash,
				BlobPath:    existing.BlobPath,
				MIME:        existing.MIME,
				SizeBytes:   existing.SizeBytes,
				Name:        existing.Name,
				Deduped:     true,
				State:       existing.State,
			}, nil
		}
	}

	// 4. 落盘：媒体按内容哈希进 multimedia/；文本类按原文件名进 documents/
	//    源文件一律原地保留，只有系统自建的临时文件会被清理（见 placeBlob）
	blobRel, moved, blobOwned, err := placeBlob(localPath, hash, ext, in.Name, in.SourceKind)
	if err != nil {
		return AdmitResult{}, err
	}
	// 未新写实体时，沿用**既有目录行**的归属：同一份内容若早已由前端上传
	// 到两棵树内，它就是受保护实体，不能因为这次是归档写入就把它标成可回收
	if !moved {
		if existing, ok := ArchiveDatabase.FindResourceByBlobPath(blobRel); ok {
			blobOwned = existing.BlobOwned
		}
	}

	// 5. 构造目录元数据
	res := &ArchiveResource{
		Kind:        kind,
		Name:        in.Name,
		Ext:         ext,
		MIME:        resolveMIME(in.MIME, ext),
		ContentHash: hash,
		BlobPath:    blobRel,
		SizeBytes:   size,
		Title:       strings.TrimSuffix(filepath.Base(in.Name), ext),
		Origin:      in.Origin,
		Retain:      firstNonEmpty(in.Retain, DefaultRetain(in.Origin)),
		State:       StateActive,
		BlobOwned:   blobOwned,
	}
	if res.Name == "" {
		res.Name = hash + ext
	}
	if res.Title == "" {
		res.Title = res.Name
	}
	blobAbs := filepath.Join(filepath.Clean(*GeneralConfig.LocalDir), filepath.FromSlash(blobRel))

	// 5.5 文本类按「原文件名」稳定复用：同一 blob_path 已有目录行时只刷新内容元数据，
	//     不新增句柄。媒体仍按内容寻址（相同内容复用 blob，但每次入库各占一行）。
	if !IsMediaExtension(ext) {
		if existing, ok := ArchiveDatabase.FindResourceByBlobPath(blobRel); ok && existing.Kind == kind {
			if err := ArchiveDatabase.RefreshResourceContent(existing.ID, res.Name, res.Ext, res.MIME, res.ContentHash, res.SizeBytes); err != nil {
				return AdmitResult{}, err
			}
			existing.Name = res.Name
			existing.Ext = res.Ext
			existing.MIME = res.MIME
			existing.ContentHash = res.ContentHash
			existing.SizeBytes = res.SizeBytes
			existing.State = StateActive
			if kind == KindDocument {
				refreshDocumentSections(existing, blobAbs)
			}
			return AdmitResult{
				Handle:      existing.Handle,
				Kind:        existing.Kind,
				Seq:         existing.Seq,
				ID:          existing.ID,
				ContentHash: res.ContentHash,
				BlobPath:    existing.BlobPath,
				MIME:        res.MIME,
				SizeBytes:   res.SizeBytes,
				Name:        res.Name,
				Deduped:     true,
				State:       existing.State,
			}, nil
		}
	}

	// 6. 登记资源目录（每次入库一行，句柄序号永不复用；blob 已去重）
	if err := ArchiveDatabase.InsertResource(res); err != nil {
		return AdmitResult{}, err
	}
	if kind == KindDocument {
		refreshDocumentSections(*res, blobAbs)
	}

	// 7. 探测元数据（图片尺寸 / 视频时长），失败不阻断入库
	probeResourceMeta(res, blobRel)

	return AdmitResult{
		Handle:      res.Handle,
		Kind:        res.Kind,
		Seq:         res.Seq,
		ID:          res.ID,
		ContentHash: res.ContentHash,
		BlobPath:    res.BlobPath,
		MIME:        res.MIME,
		SizeBytes:   res.SizeBytes,
		Name:        res.Name,
		Deduped:     !moved,
		State:       res.State,
	}, nil
}

// ResourceAdmitDocument 以「documents/原文件名」为业务身份入库或刷新文档
//
// 通用 ResourceAdmit 已对文本类执行「同名路径复用句柄」；本函数只额外约束
// 来源必须位于 documents/，供前端保存与阅读者导入使用。
func ResourceAdmitDocument(relPath, name string) (AdmitResult, error) {
	if err := EnsureArchiveInitialized(); err != nil {
		return AdmitResult{}, err
	}
	relPath = strings.TrimPrefix(strings.ReplaceAll(strings.TrimSpace(relPath), "\\", "/"), "/")
	if !strings.HasPrefix(relPath, "documents/") {
		return AdmitResult{}, fmt.Errorf("仅支持 documents 目录下的文件: %s", relPath)
	}
	return ResourceAdmit(AdmitInput{
		SourceKind: SourceKindPath,
		Source:     relPath,
		Name:       name,
		Origin:     OriginClientUpload,
		Retain:     RetainNormal,
	})
}

// refreshDocumentSections 重新解析文档段落并写回段落表（失败不阻断保存）
func refreshDocumentSections(res ArchiveResource, abs string) {
	data, err := os.ReadFile(abs)
	if err != nil {
		return
	}
	drafts := ParseMarkdownSections(stripBOM(string(data)))
	if len(drafts) == 0 {
		return
	}
	if err := ResourceReplaceSections(res.ID, drafts); err != nil {
		LoggerGeneral.Warn("FileManager", "刷新段落表失败(%s): %v", res.Handle, err)
	}
}

// normalizeAdmitSource 将来源归一为本地文件路径；cleanup 非空时由调用方负责清理
func normalizeAdmitSource(in AdmitInput) (string, func(), error) {
	switch in.SourceKind {
	case SourceKindPath, "":
		local, err := resolveLocalSource(in.Source)
		if err != nil {
			return "", nil, err
		}
		if in.Name == "" {
			in.Name = filepath.Base(local)
		}
		return local, nil, nil
	case SourceKindURL:
		return downloadToTemp(in.Source, in.Name)
	case SourceKindData:
		data, err := decodeDataURI(in.Source)
		if err != nil {
			return "", nil, err
		}
		return writeTemp(data, in.Name)
	case SourceKindBytes:
		if len(in.Data) == 0 {
			return "", nil, fmt.Errorf("字节来源为空")
		}
		return writeTemp(in.Data, in.Name)
	case SourceKindWorkspace:
		return resolveWorkspaceSource(in.Source)
	default:
		return "", nil, fmt.Errorf("不支持的来源类型: %q", in.SourceKind)
	}
}

// resolveWorkspaceSource 解析用户直接引用的工作区文件
//
// 与 LocalDir 来源的区别：LocalDir 存的是系统自己的数据（上传件、媒体），
// 而工作区是用户的项目目录（源码、文档）。两类都要能在对话里被引用，
// 但工作区文件按只读方式收录，且限制在文本类扩展名内。
//
// 安全约束：
//   - 相对路径一律以进程工作目录为根（服务端与工作区同根启动）
//   - 解析后的绝对路径必须仍位于工作目录之下（防 ../ 穿越）
//   - 扩展名必须在 workspaceTextExtensions 白名单内
//   - 体积不得超过 workspaceMaxBytes
func resolveWorkspaceSource(source string) (string, func(), error) {
	rel := strings.ReplaceAll(strings.TrimSpace(source), "\\", "/")
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return "", nil, fmt.Errorf("工作区来源为空")
	}
	if after, ok := strings.CutPrefix(rel, "file://"); ok {
		rel = after
	}
	root, err := os.Getwd()
	if err != nil {
		return "", nil, fmt.Errorf("获取工作目录失败: %v", err)
	}
	root = filepath.Clean(root)

	abs := filepath.Clean(filepath.Join(root, filepath.FromSlash(rel)))
	// 穿越校验：解析结果必须仍在工作目录之下
	if abs != root && !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
		return "", nil, fmt.Errorf("工作区路径越界: %s", source)
	}
	// 扩展名白名单（.gitignore / .env 这类无基名的点文件按整体名匹配）
	lowerBase := strings.ToLower(filepath.Base(abs))
	ext := strings.ToLower(filepath.Ext(lowerBase))
	if !workspaceTextExtensions[ext] && !workspaceTextExtensions[lowerBase] {
		return "", nil, fmt.Errorf("工作区引用仅支持文本类文件，不支持: %s", lowerBase)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", nil, fmt.Errorf("工作区文件不存在: %s", source)
	}
	if info.IsDir() {
		return "", nil, fmt.Errorf("工作区引用不支持目录: %s", source)
	}
	if info.Size() > workspaceMaxBytes {
		return "", nil, fmt.Errorf("工作区文件过大（%d 字节，上限 %d）: %s", info.Size(), int64(workspaceMaxBytes), source)
	}
	return abs, nil, nil
}

// resolveLocalSource 把各种本地/同源表示解析为可读的本地文件路径
//
// 支持：绝对路径、相对 LocalDir 的路径、file:// 前缀、本机 http(s) 服务地址（/file/read/...）
func resolveLocalSource(source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", fmt.Errorf("来源为空")
	}
	// file:// 前缀
	if rest, ok := strings.CutPrefix(source, "file://"); ok {
		source = rest
	}
	// 本机服务地址：前端上传后的图片以 http(s)://127.0.0.1/... 形式进入
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		if rel, ok := localPathFromServiceURL(source); ok {
			source = rel
		} else {
			return "", fmt.Errorf("非本机地址请使用 url 来源: %s", source)
		}
	}
	abs, err := resolveUnderLocalDir(source)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("本地来源不存在: %s", source)
	}
	return abs, nil
}

// localPathFromServiceURL 从本机服务地址中提取相对 LocalDir 的路径
//
// 前端上传后返回 `${origin}/file/read/<relative>`，其中 relative 即 LocalDir 下的路径。
func localPathFromServiceURL(raw string) (string, bool) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	host := parsed.Hostname()
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return "", false
	}
	p := strings.TrimPrefix(parsed.Path, "/file/read/")
	if p == parsed.Path || p == "" {
		return "", false
	}
	decoded, err := url.PathUnescape(p)
	if err != nil {
		return "", false
	}
	return decoded, true
}

// resolveUnderLocalDir 解析相对 LocalDir 的路径并做穿越校验
func resolveUnderLocalDir(rel string) (string, error) {
	rel = strings.ReplaceAll(rel, "\\", "/")
	rel = strings.TrimPrefix(rel, "/")
	localDir := filepath.Clean(*GeneralConfig.LocalDir)
	if filepath.IsAbs(rel) {
		abs := filepath.Clean(rel)
		if !strings.HasPrefix(abs, localDir+string(os.PathSeparator)) && abs != localDir {
			return "", fmt.Errorf("路径超出本地目录范围: %s", rel)
		}
		return abs, nil
	}
	abs := filepath.Clean(filepath.Join(localDir, filepath.FromSlash(rel)))
	if !strings.HasPrefix(abs, localDir+string(os.PathSeparator)) {
		return "", fmt.Errorf("路径穿越: %s", rel)
	}
	return abs, nil
}

// fileSHA16 流式计算文件 SHA-256 并取前 16 位十六进制，同时返回文件大小
func fileSHA16(localPath string) (string, int64, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", 0, err
	}
	return sha16Reader(f), info.Size(), nil
}

// 归档私有副本的落点：**直接进两棵树的根目录**，与前端写入件同目录、同命名方案（sha16）。
//
// 归属不靠路径区分，而是资源目录里的 blob_owned 列：
//   blob_owned = 1 → 归档自己写入的副本，配额回收可清理
//   blob_owned = 0 → 前端/引擎写入的实体，回收器不得删除
//
// 为什么不再用 archive/ 子目录：路径前缀只是表达归属的一种手段，却给目录结构加了一层
// 无谓嵌套、并让「文档的家」看起来又被分成两半。放进根目录后结构更直白，代价是删除前
// 必须查一次归属与引用计数（见 ArchiveCollect / ResourceDelete）——这个代价很小，
// 而且第 2 项守卫（引用计数）本来就必须查。

// mediaExtensions 媒体类扩展名（决定归档副本落在媒体树还是文档树）
var mediaExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true, ".bmp": true,
	".tiff": true, ".tif": true, ".svg": true, ".ico": true, ".apng": true,
	".mp4": true, ".webm": true, ".avi": true, ".mov": true, ".mkv": true, ".wmv": true,
	".flv": true, ".m4v": true, ".mpg": true, ".mpeg": true,
	".wav": true, ".mp3": true, ".flac": true, ".ogg": true, ".m4a": true, ".aac": true,
	".opus": true, ".wma": true,
}

// IsMediaExtension 判断扩展名是否属媒体类（图片 / 视频 / 音频）
func IsMediaExtension(ext string) bool {
	return mediaExtensions[strings.ToLower(ext)]
}

// snapshotRel 生成内容仓库的相对路径
//
// 媒体类：图片/视频/音频必须在 llama-server 的 media-path（multimedia 根）之内才能被
// file:// 取用；命名沿用 sha16，保持内容寻址。
// 文本类：落在 documents 根，**直接使用原文件名**，便于人机两端按名称传递与讨论；
// 同名文件视为同一份文档并被覆盖更新。
func snapshotRel(hash, ext, name string) string {
	if IsMediaExtension(ext) {
		return path.Join("multimedia", hash+ext)
	}
	return path.Join("documents", safeDocumentName(name, hash, ext))
}

// safeDocumentName 把建议文件名规整为 documents/ 下可用的裸文件名
//
// 规则：去路径、去 Windows 非法字符、保留中文与空格；无扩展名时补来源扩展名；
// 规整后为空则退回 sha16 命名，保证始终有可落盘的名字。
func safeDocumentName(name, hash, ext string) string {
	base := filepath.Base(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	base = strings.TrimSpace(base)
	if base == "" || base == "." || base == ".." {
		return hash + ext
	}
	cleaned := make([]rune, 0, len(base))
	for _, r := range base {
		switch r {
		case '<', '>', ':', '"', '/', '\\', '|', '?', '*':
			cleaned = append(cleaned, '_')
		default:
			if r < 32 {
				cleaned = append(cleaned, '_')
			} else {
				cleaned = append(cleaned, r)
			}
		}
	}
	base = strings.TrimSpace(string(cleaned))
	// Windows 不允许文件名以空格或点结尾
	base = strings.TrimRight(base, " .")
	if base == "" || base == "." || base == ".." {
		return hash + ext
	}
	if filepath.Ext(base) == "" && ext != "" {
		base += ext
	}
	return base
}

// placeBlob 确定资源的实体位置，返回相对 LocalDir 的路径与「实体是否归归档所有」
//
// **源文件一律原地保留**（硬约束）：库里出现过「导入一份配置文件后原文件消失」的问题，
// 根因就是早期实现用 moveFile 把用户/前端的文件搬走甚至删掉了。
//
// 两条分支：
//  1. 实体已在系统自有的两棵树内（前端写入件、引擎产出）→ **直接引用，不复制不搬移**，
//     并标记 blob_owned=false：这类实体归前端/引擎所有，回收器永远不能删。
//  2. 实体在系统存储之外（工作区文件、下载件、字节流中转件）→ 复制/硬链接一份到
//     snapshotRel 指定的位置，标记 blob_owned=true（可回收；仅系统自建的临时来源会被清理）。
//
// 文本类目标已存在时按「同名即同一份文档」覆盖写入，并保守标记为非归档所有，
// 避免回收器删掉用户可见的原文件名文件。
//
// 返回值 moved 表示是否新写入了实体；owned 表示该实体是否归归档所有（可回收）。
func placeBlob(localPath, hash, ext, name, sourceKind string) (rel string, moved bool, owned bool, err error) {
	localAbs := filepath.Clean(localPath)
	archiveRel := snapshotRel(hash, ext, name)
	archiveAbs := filepath.Join(filepath.Clean(*GeneralConfig.LocalDir), filepath.FromSlash(archiveRel))

	// 分支 1：已在系统自有的存储内 → 原地引用（受保护，不归归档所有）
	if rel, ok := relativeToLocalDir(localAbs); ok && isSystemOwnedStore(rel) && fileExists(localAbs) {
		return rel, false, false, nil
	}
	if IsMediaExtension(ext) {
		// 媒体：内容寻址，目标存在即同内容复用
		if fileExists(archiveAbs) {
			removeTemporarySource(localAbs, sourceKind)
			return archiveRel, false, false, nil
		}
	} else if fileExists(archiveAbs) {
		// 文本类：同名即同一份文档，内容变化时覆盖原文件
		if err := copyIntoPlace(localAbs, archiveAbs); err != nil {
			return "", false, false, fmt.Errorf("覆盖文档实体失败: %v", err)
		}
		removeTemporarySource(localAbs, sourceKind)
		return archiveRel, false, false, nil
	}
	if err := os.MkdirAll(filepath.Dir(archiveAbs), 0755); err != nil {
		return "", false, false, fmt.Errorf("创建内容仓库目录失败: %v", err)
	}
	// 优先硬链接：零拷贝且不额外占空间；跨卷或权限不足时退回复制
	if err := os.Link(localAbs, archiveAbs); err == nil {
		removeTemporarySource(localAbs, sourceKind)
		return archiveRel, true, true, nil
	}
	if err := copyIntoPlace(localAbs, archiveAbs); err != nil {
		return "", false, false, fmt.Errorf("写入内容仓库失败: %v", err)
	}
	removeTemporarySource(localAbs, sourceKind)
	return archiveRel, true, true, nil
}

// isSystemOwnedStore 判断相对路径是否位于系统自有的存储树内（前端写入件与引擎产出）
//
// 判据是「路径位于这两棵树内」：这样即便前端改了命名方案（历史遗留的 base64 名、
// 引擎自命名等）也一律按「已有实体、直接引用」处理，不额外产生副本。
func isSystemOwnedStore(rel string) bool {
	rel = filepath.ToSlash(rel)
	return strings.HasPrefix(rel, "documents/") || strings.HasPrefix(rel, "multimedia/")
}

// 归属说明：实体是否可回收记录在资源目录的 blob_owned 列上（见 ArchiveResource.BlobOwned），
// 不再用路径前缀表达。删除前仍须复核引用计数（见 ArchiveCollect / ResourceDelete），
// 两道守卫合起来保证「回收器不会删掉前端可见的文件」。

// removeTemporarySource 仅清理系统自建的临时来源
//
// 判据是来源类型而非路径：下载件（url）与字节流落地的中转文件（data / bytes）由本进程
// 创建，可以清理；path 与 workspace 指向用户或前端的真实文件，**必须保留**。
func removeTemporarySource(abs, sourceKind string) {
	switch sourceKind {
	case SourceKindURL, SourceKindData, SourceKindBytes:
		_ = os.Remove(abs)
	}
}

// copyIntoPlace 把源文件复制到目标路径（不触碰源文件）
func copyIntoPlace(src, dst string) error {
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
		os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(dst)
		return err
	}
	return nil
}

// relativeToLocalDir 求绝对路径相对 LocalDir 的斜杠形式路径（不在其下时返回 false）
func relativeToLocalDir(abs string) (string, bool) {
	localDir := filepath.Clean(*GeneralConfig.LocalDir)
	rel, err := filepath.Rel(localDir, abs)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}

// 说明：工作区来源不再需要单独的落盘实现 —— placeBlob 已统一为「源文件一律原地保留」，
// 与工作区文件所需语义一致，此处曾有的 placeWorkspaceBlob / moveFile 已随之删除。

// writeTemp 将字节写入临时文件，返回路径与清理函数
func writeTemp(data []byte, name string) (string, func(), error) {
	ext := filepath.Ext(name)
	tmp, err := os.CreateTemp("", "lunar-admit-*"+ext)
	if err != nil {
		return "", nil, fmt.Errorf("创建临时文件失败: %v", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", nil, fmt.Errorf("写入临时文件失败: %v", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", nil, fmt.Errorf("关闭临时文件失败: %v", err)
	}
	return tmp.Name(), func() { os.Remove(tmp.Name()) }, nil
}

// downloadToTemp 下载远程地址到临时文件，返回路径与清理函数
func downloadToTemp(raw string, name string) (string, func(), error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(raw)
	if err != nil {
		return "", nil, fmt.Errorf("下载资源失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("下载资源失败: HTTP %d", resp.StatusCode)
	}
	ext := filepath.Ext(name)
	if ext == "" {
		ext = filepath.Ext(parseURLPath(raw))
	}
	tmp, err := os.CreateTemp("", "lunar-admit-*"+ext)
	if err != nil {
		return "", nil, fmt.Errorf("创建临时文件失败: %v", err)
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", nil, fmt.Errorf("下载写入失败: %v", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", nil, fmt.Errorf("关闭临时文件失败: %v", err)
	}
	return tmp.Name(), func() { os.Remove(tmp.Name()) }, nil
}

// parseURLPath 提取 URL 的路径部分（失败时返回空串）
func parseURLPath(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Path
}

// decodeDataURI 解码 data:<mime>;base64,<payload> 形态的内联数据
func decodeDataURI(raw string) ([]byte, error) {
	if !strings.HasPrefix(raw, "data:") {
		return nil, fmt.Errorf("非法的 data URI")
	}
	comma := strings.Index(raw, ",")
	if comma < 0 {
		return nil, fmt.Errorf("非法的 data URI：缺少数据分隔符")
	}
	meta, payload := raw[:comma], raw[comma+1:]
	if !strings.Contains(meta, "base64") {
		decoded, err := url.PathUnescape(payload)
		if err != nil {
			return nil, fmt.Errorf("解码 data URI 失败: %v", err)
		}
		return []byte(decoded), nil
	}
	data, err := base64Decode(payload)
	if err != nil {
		return nil, fmt.Errorf("解码 data URI 失败: %v", err)
	}
	return data, nil
}

// resolveMIME 解析 MIME：优先调用方建议值，其次扩展名白名单，最后按类型码兜底
func resolveMIME(hint, ext string) string {
	if hint != "" {
		return hint
	}
	if entry, ok := previewAllowlist[strings.ToLower(ext)]; ok {
		return entry.MIME
	}
	switch KindOfExt(ext) {
	case KindImage:
		return "image/" + strings.TrimPrefix(strings.ToLower(ext), ".")
	case KindVideo:
		return "video/" + strings.TrimPrefix(strings.ToLower(ext), ".")
	case KindAudio:
		return "audio/" + strings.TrimPrefix(strings.ToLower(ext), ".")
	case KindDocument:
		return "text/plain"
	}
	return "application/octet-stream"
}

// firstNonEmpty 返回首个非空字符串
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// probeResourceMeta 探测并回写资源元数据（失败仅告警，不影响入库）
func probeResourceMeta(res *ArchiveResource, blobRel string) {
	abs := filepath.Join(filepath.Clean(*GeneralConfig.LocalDir), filepath.FromSlash(blobRel))
	switch res.Kind {
	case KindImage:
		width, height, err := imageDimensions(abs)
		if err != nil {
			LoggerGeneral.Warn("FileManager", "图片尺寸探测失败(%s): %v", res.Handle, err)
			return
		}
		_ = ArchiveDatabase.UpdateResourceMeta(res.ID, width, height, 0)
		res.Width, res.Height = width, height
	case KindVideo, KindAudio:
		// 时长探测交给多模态子系统（ffprobe），此处不重复实现
	}
}
