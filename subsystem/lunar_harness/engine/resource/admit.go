package resource

// admit.go 统一入库入口 —— 全模态资源进入系统的唯一通道
//
// 强制流程（规范二）：
//   1. 归一来源并下载到本地存储目录（path 来源已在存储内则原地引用）
//   2. 校验完整性（哈希 / 大小 / 图片头部 / 视频非空）
//   3. 创建对应类型句柄
//   4. 绑定 /file/read/<rel> 目标 URL（URL 主表唯一绑定）
//
// 只有四步全部完成才返回成功；任何一步失败都不会留下句柄。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// admitResultOf 把 URL 主表记录转换为入库结果
func admitResultOf(res Resource, deduped bool) AdmitResult {
	return AdmitResult{
		Handle:      res.Handle,
		Kind:        res.Kind,
		Seq:         res.Seq,
		ID:          res.ID,
		ContentHash: res.ContentHash,
		BlobPath:    res.BlobPath,
		URL:         res.URL,
		MIME:        res.MIME,
		SizeBytes:   res.SizeBytes,
		Name:        res.Name,
		Deduped:     deduped,
		State:       res.State,
	}
}

// Admit 统一入库入口：归一来源 → 校验 → 落盘 → 建句柄 → 绑定本地 URL
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

	// 1. 归一为「本地文件 + 建议文件名」
	localPath, cleanup, err := normalizeAdmitSource(in)
	if err != nil {
		return AdmitResult{}, err
	}
	if cleanup != nil {
		defer cleanup()
	}
	// 2. 校验完整性
	hash, size, err := fileSHA16(localPath)
	if err != nil {
		return AdmitResult{}, fmt.Errorf("校验资源完整性失败: %v", err)
	}
	if size <= 0 {
		return AdmitResult{}, fmt.Errorf("资源内容为空: %s", in.Name)
	}

	ext := strings.ToLower(filepath.Ext(in.Name))
	if ext == "" {
		ext = strings.ToLower(filepath.Ext(localPath))
	}
	kind := trimKind(in.KindHint)
	if kind == "" {
		kind = KindOfExt(ext)
	}
	if kind == "" {
		kind = KindDocument
	}
	// 2.1 图片以内容为准修正扩展名与 MIME；动图单独归 G
	if kind == KindImage || kind == KindAnimated {
		if realExt, realMIME := sniffFileImageFormat(localPath); realExt != "" {
			if realExt != ext {
				logger.Info("FileManager", "图片扩展名与内容不符，按实际格式纠正: %q -> %q", ext, realExt)
				ext = realExt
			}
			in.MIME = realMIME
			if isAnimatedFile(localPath, ext) {
				kind = KindAnimated
			} else {
				kind = KindImage
			}
		}
	}
	if !IsMediaExtension(ext) {
		hash = ""
	}

	// 3. 落盘（源文件一律原地保留；只有系统自建临时来源会被清理）
	blobRel, moved, blobOwned, err := placeBlob(localPath, hash, ext, in.Name, in.SourceKind)
	if err != nil {
		return AdmitResult{}, err
	}
	if !moved {
		if existing, ok := archiveDB.FindResourceByBlobPath(blobRel); ok {
			blobOwned = existing.BlobOwned
		}
	}

	// 4. 构造记录并建句柄 + 绑定 URL（同一 URL 复用句柄）
	res := &Resource{
		Kind:        kind,
		Name:        in.Name,
		Ext:         ext,
		MIME:        resolveMIME(in.MIME, ext),
		ContentHash: hash,
		BlobPath:    blobRel,
		URL:         FileReadURL(blobRel),
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
	existing, existed := archiveDB.FindResourceByURL(res.URL)
	if existed && existing.Handle != "" {
		// 既有记录：保留原有解析文本 / 标题 / 归属
		_ = archiveDB.UpdateResourceScan(existing.ID, res.Name, res.Ext, res.MIME, res.ContentHash, res.SizeBytes)
		archiveDB.TouchResource(existing.ID)
		if existing.Kind != kind {
			return admitFlexibleKind(existing, res)
		}
		existing.Name, existing.Ext, existing.MIME = res.Name, res.Ext, res.MIME
		existing.ContentHash, existing.SizeBytes = res.ContentHash, res.SizeBytes
		return admitResultOf(existing, true), nil
	}
	if err := archiveDB.InsertResource(res); err != nil {
		return AdmitResult{}, err
	}
	blobAbs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(blobRel))
	if kind == KindDocument {
		refreshDocumentSections(*res, blobAbs)
	}
	probeResourceMeta(res, blobAbs)
	return admitResultOf(*res, !moved), nil
}

// admitFlexibleKind 类型码变化时按新类型重建句柄（保留原解析文本）
func admitFlexibleKind(existing Resource, res *Resource) (AdmitResult, error) {
	res.ParseText = existing.ParseText
	res.ParseState = existing.ParseState
	if err := archiveDB.InsertResource(res); err != nil {
		return AdmitResult{}, err
	}
	return admitResultOf(*res, false), nil
}

// AdmitDocument 以「documents/原文件名」为业务身份入库或刷新文档
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

// probeResourceMeta 探测并回写资源元数据（失败仅告警）
func probeResourceMeta(res *Resource, abs string) {
	switch res.Kind {
	case KindImage, KindAnimated:
		width, height, err := imageDimensions(abs)
		if err != nil {
			logger.Warn("FileManager", "图片尺寸探测失败(%s): %v", res.Handle, err)
			return
		}
		_ = archiveDB.UpdateResourceMeta(res.ID, width, height, 0)
		res.Width, res.Height = width, height
	}
}
