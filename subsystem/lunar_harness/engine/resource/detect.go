package resource

// detect.go 存储层的类型归位（仅文件管理系统内部使用）
//
// 该函数用于扫描已有存储目录、给未声明类型的内部来源选择落盘目录，
// 不属于入站多模态信封协议：信封类型必须由发送侧声明并经 VerifyStored 校验，
// 入站链路禁止调用 DetectStorageKind 代偿识别。

import (
	"path/filepath"

	"LunarSubsystem/LunarHarness/engine/multimodal"
)

// DetectStorageKind 按字节魔数与扩展名给内部来源归位；仍保持「无远程探测、无主机名推断」。
func DetectStorageKind(in ClassifyInput) ClassifyResult {
	ext := classifyExt(in)

	// 1. 本地文件头魔数
	if head, ok := classifiableHead(in); ok {
		if info, sniffed := sniffMagic(head); sniffed {
			kind := info.Kind
			if kind == KindImage {
				if localAbs, hasLocal := classifiableLocalPath(in); hasLocal && isAnimatedFile(localAbs, info.Ext) {
					kind = KindAnimated
				} else if info.Ext == ".gif" || info.Ext == ".apng" || multimodal.IsAnimatedImage(head) {
					kind = KindAnimated
				}
			}
			return ClassifyResult{Kind: kind, Ext: info.Ext, MIME: info.MIME, Basis: ClassifyBasisMagic}
		}
	}

	// 2. 扩展名（仅用于内部归位）
	if kind := kindFromExtension(ext); kind != "" {
		return ClassifyResult{Kind: kind, Ext: ext, MIME: mimeForKind(kind, ext), Basis: ClassifyBasisExtension}
	}

	// 3. 兜底：远程链接按网页，本地实体按文件
	if IsRemoteURL(in.Source) {
		return ClassifyResult{Kind: KindURL, Ext: ext, MIME: URLMIME, Basis: ClassifyBasisFallback}
	}
	return ClassifyResult{Kind: KindDocument, Ext: ext, MIME: mimeForKind(KindDocument, ext), Basis: ClassifyBasisFallback}
}

// classifiableHead 取用于归位的文件头：调用方预读优先，其次本地实体
func classifiableHead(in ClassifyInput) ([]byte, bool) {
	if len(in.Head) > 0 {
		return in.Head, true
	}
	localAbs, ok := classifiableLocalPath(in)
	if !ok {
		return nil, false
	}
	head, err := readHead(localAbs)
	if err != nil {
		return nil, false
	}
	return head, true
}

// classifiableLocalPath 求可读的本地实体（来源已归一或本身就在本机）
func classifiableLocalPath(in ClassifyInput) (string, bool) {
	if in.LocalPath != "" {
		return in.LocalPath, fileExists(in.LocalPath)
	}
	switch in.SourceKind {
	case SourceKindPath, SourceKindWorkspace, "":
		if abs, err := resolveLocalSource(in.Source); err == nil {
			return abs, true
		}
	}
	return "", false
}

// kindFromExtension 扩展名到类型码的映射（仅内部归位使用）
func kindFromExtension(ext string) string {
	if ext == "" {
		return ""
	}
	kind := KindOfExt(ext)
	if kind == KindImage {
		switch ext {
		case ".gif", ".apng", ".webp":
			return KindAnimated
		}
	}
	return kind
}

// storageKindFromRoot 从存储目录反推声明类型（扫描已有文件时的「目录即声明」）
func storageKindFromRoot(rel string) string {
	rel = normalizeRel(rel)
	switch {
	case hasRelPrefix(rel, StorageRootAudios):
		return KindAudio
	case hasRelPrefix(rel, StorageRootMultimedia):
		return KindImage
	case hasRelPrefix(rel, StorageRootDocuments), hasRelPrefix(rel, StorageRootUnknown):
		return KindDocument
	}
	return ""
}

func hasRelPrefix(rel, root string) bool {
	return rel == root || len(rel) > len(root) && rel[:len(root)+1] == root+"/"
}

// extOfStoragePath 取存储相对路径的扩展名（仅用于命名与展示）
func extOfStoragePath(rel string) string {
	return normalizeExt(filepath.Ext(rel))
}
