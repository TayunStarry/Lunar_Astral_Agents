package resource

// admit.go 统一入库入口 —— 全模态资源进入系统的唯一通道
//
// 设计依据：docs/adr/0001「统一入库入口」。
// 三类来源归一：
//   1. path  已是本地文件（前端上传已落盘）→ 只登记不复制，零拷贝
//   2. url   远程地址（QQ 等渠道仅给 URL）→ 下载后按命名规则落盘
//   3. data  字节流 / data URI（工具产出、转发内联）→ 落盘后登记
//
// 来源归一与落点选择分别见 admit_source.go 与 admit_blob.go。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// admitResultOf 把资源目录行转换为入库结果
func admitResultOf(res Resource, deduped bool) AdmitResult {
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
		Deduped:     deduped,
		State:       res.State,
	}
}

// Admit 统一入库入口：归一来源 → 规范化落盘 → 登记资源目录 → 返回句柄
func Admit(in AdmitInput) (AdmitResult, error) {
	if err := EnsureReady(); err != nil {
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
	// 3.1 图片扩展名以内容为准：QQ 等渠道经常把 GIF / 动态 WebP 命名为 .png 或 .jpg，
	//     声明 MIME 也随之失真。若沿用名称里的扩展名落盘，动图会被存成静态扩展名，
	//     下游任何按扩展名判定的环节都会把它当静态图。嗅探到真实格式时同步纠正 MIME。
	if kind == KindImage {
		if realExt, realMIME := sniffFileImageFormat(localPath); realExt != "" {
			if realExt != ext {
				logger.Info("FileManager", "图片扩展名与内容不符，按实际格式纠正: %q -> %q", ext, realExt)
				ext = realExt
			}
			in.MIME = realMIME
		}
	}
	// 文本类不记录内容哈希：文本以原文件名 / 路径为业务身份，哈希会随每次编辑变化
	if !IsMediaExtension(ext) {
		hash = ""
	}

	// 3.5 媒体内容哈希唯一：命中既有实体时，只有「新落点优先级更高」才继续写入
	//     （生成目录 > audios/multimedia 根目录；vision/footage 派生缓存不入索引）；
	//     否则直接复用既有句柄与路径。MustStore 不能制造同哈希的第二份实体。
	//     文本类不参与内容去重 —— 它们以原文件名为业务身份，由步骤 5.5 按路径复用。
	if IsMediaExtension(ext) {
		if existing, ok := FindActiveByHash(kind, hash); ok {
			targetRel := snapshotRel(hash, ext, in.Name)
			if !preferNewPath(targetRel, existing.BlobPath) {
				return admitResultOf(existing, true), nil
			}
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
		if existing, ok := archiveDB.FindResourceByBlobPath(blobRel); ok {
			blobOwned = existing.BlobOwned
		}
	}

	// 5. 构造目录元数据
	res := &Resource{
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
	blobAbs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(blobRel))

	// 5.4 落盘后再做一次哈希冲突裁决：placeBlob 可能把实体放到与 snapshotRel
	//     不同的位置（源文件已在系统存储内时原地引用）。仍按路径优先级决定保留哪一份。
	if IsMediaExtension(ext) {
		if existing, ok := FindActiveByHash(kind, hash); ok && existing.BlobPath != blobRel {
			if preferNewPath(blobRel, existing.BlobPath) {
				oldAbs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(existing.BlobPath))
				if oldAbs != blobAbs {
					_ = os.Remove(oldAbs)
				}
				if err := archiveDB.UpdateResourceBlobPath(existing.ID, blobRel); err != nil {
					return AdmitResult{}, err
				}
				if err := archiveDB.UpdateResourceScan(existing.ID, res.Name, ext, res.MIME, hash, size); err != nil {
					return AdmitResult{}, err
				}
				existing.BlobPath = blobRel
				existing.Name = res.Name
				existing.Ext = ext
				existing.MIME = res.MIME
				existing.ContentHash = hash
				existing.SizeBytes = size
				existing.State = StateActive
				return admitResultOf(existing, true), nil
			}
			// 既有实体优先：删除刚落盘的重复文件
			_ = os.Remove(blobAbs)
			return admitResultOf(existing, true), nil
		}
	}

	// 5.5 文本类按「原文件名」稳定复用：同一 blob_path 已有目录行时只刷新内容元数据，
	//     不新增句柄。媒体仍按内容寻址（相同内容复用 blob，但每次入库各占一行）。
	if !IsMediaExtension(ext) {
		if existing, ok := archiveDB.FindResourceByBlobPath(blobRel); ok && existing.Kind == kind {
			if err := archiveDB.RefreshResourceContent(existing.ID, res.Name, res.Ext, res.MIME, res.ContentHash, res.SizeBytes); err != nil {
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
	if err := archiveDB.InsertResource(res); err != nil {
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

// AdmitDocument 以「documents/原文件名」为业务身份入库或刷新文档
//
// 通用 Admit 已对文本类执行「同名路径复用句柄」；本函数只额外约束
// 来源必须位于 documents/，供前端保存与阅读者导入使用。
func AdmitDocument(relPath, name string) (AdmitResult, error) {
	if err := EnsureReady(); err != nil {
		return AdmitResult{}, err
	}
	relPath = strings.TrimPrefix(strings.ReplaceAll(strings.TrimSpace(relPath), "\\", "/"), "/")
	if !strings.HasPrefix(relPath, StorageRootDocuments+"/") {
		return AdmitResult{}, fmt.Errorf("仅支持 documents 目录下的文件: %s", relPath)
	}
	return Admit(AdmitInput{
		SourceKind: SourceKindPath,
		Source:     relPath,
		Name:       name,
		Origin:     OriginClientUpload,
		Retain:     RetainNormal,
	})
}

// refreshDocumentSections 重新解析文档段落并写回段落表（失败不阻断保存）
func refreshDocumentSections(res Resource, abs string) {
	data, err := os.ReadFile(abs)
	if err != nil {
		return
	}
	drafts := ParseMarkdownSections(stripBOM(string(data)))
	if len(drafts) == 0 {
		return
	}
	if err := archiveDB.ReplaceSections(res.ID, drafts); err != nil {
		logger.Warn("FileManager", "刷新段落表失败(%s): %v", res.Handle, err)
	}
}

// probeResourceMeta 探测并回写资源元数据（失败仅告警，不影响入库）
func probeResourceMeta(res *Resource, blobRel string) {
	abs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(blobRel))
	switch res.Kind {
	case KindImage:
		width, height, err := imageDimensions(abs)
		if err != nil {
			logger.Warn("FileManager", "图片尺寸探测失败(%s): %v", res.Handle, err)
			return
		}
		_ = archiveDB.UpdateResourceMeta(res.ID, width, height, 0)
		res.Width, res.Height = width, height
	case KindVideo, KindAudio:
		// 时长探测交给多模态子系统（ffprobe），此处不重复实现
	}
}
