package memory

// 记忆库媒体入库与清理：
// 静态图 → 缩放（宽或高 >640 等比缩放）+ 统一转码（JPEG/PNG data URI）；
// 动态图（GIF / APNG / 动态 WebP）→ 存储保留原动画文件，
// 理解引用由多模态模块的统一观看管线按等距规则抽帧（长边 ≤640）。
// 任何一步失败均回退原始输入，不阻塞入库。

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	multimodal "LunarSubsystem/LunarHarness/engine/multimodal"
	"LunarSubsystem/LunarHarness/engine/persistence"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// memoryStandardMaxDim 静态图入库最大边长。
const memoryStandardMaxDim = 640

// animatedPerceptionRefs 生成动态图的采样帧引用
//
// 动态图按「一个很短的影像片段」处理：由多模态模块的统一观看管线
// 在整段动画上等距抽取 WatchFrameMax 帧（长边 ≤640），与视频切片同一套规则。
func animatedPerceptionRefs(raw []byte) ([]string, error) {
	slice, err := multimodal.AnimatedToWatchSlice(raw, multimodal.WatchFrameMax)
	if err != nil {
		return nil, err
	}
	return slice.Images, nil
}

// saveMemoryImageFile 把图片实体写入 multimedia/memory/<集合名>/，返回前端可读取的 URL
//
// 记忆库图片统一收进 memory 子目录，避免与生成图、背景图、截图等混在一起；
// 命名沿用内容哈希（同内容只存一份），由 SaveFileEx 顺带登记句柄映射。
func saveMemoryImageFile(collectionName, dataURI string) (string, error) {
	// 必须在创建临时文件之前初始化资源目录：否则初始化时的存储扫描会把
	// SaveFileEx 刚创建的临时文件登记成正式实体，导致返回临时文件名。
	if err := resource.EnsureReady(); err != nil {
		return "", err
	}
	mimeType, raw, err := persistence.StripDataURI(dataURI)
	if err != nil {
		return "", err
	}
	ext := ".png"
	switch strings.ToLower(mimeType) {
	case "image/jpeg", "image/jpg":
		ext = ".jpg"
	case "image/webp":
		ext = ".webp"
	case "image/gif":
		ext = ".gif"
	case "image/bmp":
		ext = ".bmp"
	}
	// 声明 MIME 可能失真（QQ 常把 GIF 命名成 .png）：扩展名以文件头嗅探结果为准
	if sniffExt, _ := persistence.SniffImageFormat(raw); sniffExt != "" {
		ext = sniffExt
	}
	sum := sha256.Sum256(raw)
	rel := path.Join(resource.StorageRootMultimedia, MediaSubdir, collectionName, hex.EncodeToString(sum[:])[:16]+ext)
	result, saveErr := persistence.SaveFileEx(rel, persistence.SaveOptions{Overwrite: true, MustStore: true}, bytes.NewReader(raw))
	if saveErr != nil {
		return "", saveErr
	}
	// 统一以 /file/read/ 形式返回（前端与 QQ 适配器都按该约定解析）
	return "/file/read/" + result.RelPath, nil
}

// saveMemoryAudioFile 把音频实体写入 audios/<集合名>/，返回可读取的 URL
func saveMemoryAudioFile(collectionName, dataURI string) (string, error) {
	// 必须在创建临时文件之前初始化资源目录：否则初始化时的存储扫描会把
	// SaveFileEx 刚创建的临时文件登记成正式实体，导致返回临时文件名。
	if err := resource.EnsureReady(); err != nil {
		return "", err
	}
	mimeType, raw, err := persistence.StripDataURI(dataURI)
	if err != nil {
		return "", err
	}
	ext := audioFileExt(mimeType)
	sum := sha256.Sum256(raw)
	rel := path.Join(resource.StorageRootAudios, collectionName, hex.EncodeToString(sum[:])[:16]+ext)
	result, saveErr := persistence.SaveFileEx(rel, persistence.SaveOptions{Overwrite: true, MustStore: true}, bytes.NewReader(raw))
	if saveErr != nil {
		return "", saveErr
	}
	return "/file/read/" + result.RelPath, nil
}

// audioFileExt 音频 MIME → 扩展名
func audioFileExt(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "audio/ogg":
		return ".ogg"
	case "audio/mp4", "audio/m4a", "audio/x-m4a":
		return ".m4a"
	case "audio/flac", "audio/x-flac":
		return ".flac"
	case "audio/aac":
		return ".aac"
	case "audio/opus":
		return ".opus"
	case "audio/webm":
		return ".webm"
	default:
		return ".audio"
	}
}

// memoryMediaRelFromURL 把 /file/read/ URL 还原为内容仓库相对路径。
// 只接受 multimedia/ 与 audios/ 两类媒体树，其它路径不参与记忆媒体清理。
func memoryMediaRelFromURL(rawURL string) (string, bool) {
	raw := strings.TrimSpace(rawURL)
	if raw == "" {
		return "", false
	}
	// 兼容完整 http(s) 地址与带查询串/锚点的地址
	if idx := strings.Index(raw, "/file/read/"); idx >= 0 {
		raw = raw[idx:]
	}
	if cut := strings.IndexAny(raw, "?#"); cut >= 0 {
		raw = raw[:cut]
	}
	if !strings.HasPrefix(raw, "/file/read/") {
		return "", false
	}
	rel := strings.TrimPrefix(raw, "/file/read/")
	rel = strings.ReplaceAll(rel, "\\", "/")
	rel = strings.TrimPrefix(rel, "/")
	if decoded, err := url.PathUnescape(rel); err == nil {
		rel = decoded
	}
	if rel == "" || strings.Contains(rel, "..") {
		return "", false
	}
	if !strings.HasPrefix(rel, resource.StorageRootMultimedia+"/") && !strings.HasPrefix(rel, resource.StorageRootAudios+"/") {
		return "", false
	}
	return rel, true
}

// normalizeMemoryMediaURL 归一化媒体 URL 用于“是否仍被引用”的比较
func normalizeMemoryMediaURL(rawURL string) string {
	if rel, ok := memoryMediaRelFromURL(rawURL); ok {
		return rel
	}
	raw := strings.TrimSpace(rawURL)
	if cut := strings.IndexAny(raw, "?#"); cut >= 0 {
		raw = raw[:cut]
	}
	return raw
}

// memoryMediaRelForDoc 返回该文档可以安全清理的媒体相对路径。
//
// 只认本分组专属的媒体目录：
//   - 新版图片：multimedia/memory/<分组>/
//   - 旧版图片：multimedia/<分组>/（兼容迁移前数据）
//   - 音频：audios/<分组>/
//
// 其它路径（生成图、截图、公共图片等）即使被记忆条目引用也不删除，避免误删共享资源。
func memoryMediaRelForDoc(collection string, doc Document) (string, bool) {
	rel, ok := memoryMediaRelFromURL(doc.URL)
	if !ok {
		return "", false
	}
	switch documentType(doc) {
	case TypeImage:
		newPrefix := path.Join(resource.StorageRootMultimedia, MediaSubdir, collection) + "/"
		if strings.HasPrefix(rel, newPrefix) {
			return rel, true
		}
		if collection == MediaSubdir {
			// 分组名恰好叫 memory：旧版文件只可能直接位于 multimedia/memory/ 下，
			// 不能再把 multimedia/memory/<其它分组>/ 误判成本分组。
			if path.Dir(rel) == path.Join(resource.StorageRootMultimedia, MediaSubdir) {
				return rel, true
			}
			return "", false
		}
		legacyPrefix := path.Join(resource.StorageRootMultimedia, collection) + "/"
		if strings.HasPrefix(rel, legacyPrefix) {
			return rel, true
		}
	case TypeAudio:
		prefix := path.Join(resource.StorageRootAudios, collection) + "/"
		if strings.HasPrefix(rel, prefix) {
			return rel, true
		}
	}
	return "", false
}

// mediaURLReferenced 判断某个媒体 URL 是否仍被任意集合的记忆条目引用
func (d *Store) mediaURLReferenced(rawURL string) bool {
	target := normalizeMemoryMediaURL(rawURL)
	if target == "" {
		return false
	}
	for _, other := range d.snapshotCollections() {
		other.mu.RLock()
		for i := range other.Documents {
			if normalizeMemoryMediaURL(other.Documents[i].URL) == target {
				other.mu.RUnlock()
				return true
			}
		}
		other.mu.RUnlock()
	}
	return false
}

// removeMemoryMediaForDoc 删除文档对应的媒体实体，并清理资源目录中的句柄行。
//
// 只有在没有其它记忆条目引用同一 URL 时才删除；无法确定归属的公共资源一律跳过。
func (d *Store) removeMemoryMediaForDoc(collection string, doc Document) {
	rel, ok := memoryMediaRelForDoc(collection, doc)
	if !ok {
		return
	}
	if d.mediaURLReferenced(doc.URL) {
		return
	}
	base := filepath.Clean(config.LocalDir)
	abs := filepath.Join(base, filepath.FromSlash(rel))
	if abs == base || !strings.HasPrefix(abs, base+string(os.PathSeparator)) {
		logger.Warn("FileManager", "记忆媒体路径越界，跳过删除: %s", rel)
		return
	}
	if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
		logger.Warn("FileManager", "删除记忆媒体文件失败 [%s]: %v", rel, err)
		return
	}
	// 文件（或已缺失的实体）不再被引用：同步移除资源目录行，避免残留失效句柄
	if err := resource.EnsureReady(); err == nil {
		resource.ForgetPath(rel)
	}
	// 分组媒体目录空了就一并移除；非空时失败可忽略
	_ = os.Remove(filepath.Dir(abs))
	logger.Info("FileManager", "已删除记忆媒体: %s", rel)
}

// removeMemoryMediaForDocs 批量清理一组文档的媒体实体（按 URL 去重）
func (d *Store) removeMemoryMediaForDocs(collection string, docs []Document) {
	seen := make(map[string]bool, len(docs))
	for _, doc := range docs {
		if doc.URL == "" {
			continue
		}
		key := normalizeMemoryMediaURL(doc.URL)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		d.removeMemoryMediaForDoc(collection, doc)
	}
}

// migrateLegacyMemoryImages 把旧版 multimedia/<分组>/ 的图片迁移到
// multimedia/memory/<分组>/，并同步改写文档 URL 与资源目录句柄。
//
// 返回是否有文档被改写；迁移失败的条目保持旧路径，仍可正常读取/删除。
func (c *Collection) migrateLegacyMemoryImages() bool {
	if c.Name == MediaSubdir {
		return false
	}
	c.mu.RLock()
	docs := append([]Document(nil), c.Documents...)
	c.mu.RUnlock()
	if len(docs) == 0 {
		return false
	}

	oldPrefix := path.Join(resource.StorageRootMultimedia, c.Name) + "/"
	newPrefix := path.Join(resource.StorageRootMultimedia, MediaSubdir, c.Name) + "/"
	base := filepath.Clean(config.LocalDir)
	changed := false
	for i := range docs {
		doc := &docs[i]
		if documentType(*doc) != TypeImage {
			continue
		}
		rel, ok := memoryMediaRelFromURL(doc.URL)
		if !ok || !strings.HasPrefix(rel, oldPrefix) || strings.HasPrefix(rel, newPrefix) {
			continue
		}
		newRel := path.Join(resource.StorageRootMultimedia, MediaSubdir, c.Name, strings.TrimPrefix(rel, oldPrefix))
		oldAbs := filepath.Join(base, filepath.FromSlash(rel))
		newAbs := filepath.Join(base, filepath.FromSlash(newRel))
		if err := os.MkdirAll(filepath.Dir(newAbs), 0755); err != nil {
			logger.Warn("FileManager", "迁移记忆图片目录失败 [%s]: %v", rel, err)
			continue
		}
		if err := os.Rename(oldAbs, newAbs); err != nil {
			if _, statErr := os.Stat(newAbs); statErr == nil {
				// 新路径已有同内容实体（内容寻址），删掉旧副本即可
				_ = os.Remove(oldAbs)
			} else {
				logger.Warn("FileManager", "迁移记忆图片失败 [%s] -> [%s]: %v", rel, newRel, err)
				continue
			}
		}
		resource.RewriteHandlesForMove(rel, newRel)
		doc.URL = "/file/read/" + newRel
		changed = true
	}
	if !changed {
		return false
	}

	c.mu.Lock()
	for i := range c.Documents {
		for j := range docs {
			if c.Documents[i].ID == docs[j].ID && c.Documents[i].URL != docs[j].URL {
				c.Documents[i].URL = docs[j].URL
				break
			}
		}
	}
	c.mu.Unlock()
	// 旧目录已空则移除；仍有未迁移/非记忆文件时保留
	_ = os.Remove(filepath.Join(base, filepath.FromSlash(strings.TrimSuffix(oldPrefix, "/"))))
	logger.Info("FileManager", "分组 [%s] 旧版记忆图片已迁移到 %s", c.Name, newPrefix)
	return true
}

// standardizeMemoryImage 记忆库图片入库前的标准化：
// 返回 storeURI（入库与去重用的完整 data URI）、mediaRefs（动态图的感知者式采样帧引用，
// 静态图为 nil）、animated（是否动态图）。任一步失败回退原始输入（记 Warning，不返回错误）。
func standardizeMemoryImage(base64Image string) (storeURI string, mediaRefs []string, animated bool) {
	mimeType, raw, err := persistence.StripDataURI(base64Image)
	if err != nil {
		logger.Warn("FileManager", "图片标准化跳过（%v），按原始图片入库", err)
		return strings.TrimSpace(base64Image), nil, false
	}

	// 动态图：存储保留原动画 data URI；理解引用按配置的输入模式生成
	if multimodal.IsAnimatedImage(raw) {
		// 声明 MIME 与内容不符时（QQ 常把 GIF 标成 image/png）以嗅探结果为准
		if _, realMIME := persistence.SniffImageFormat(raw); realMIME != "" {
			mimeType = realMIME
		}
		storeURI = persistence.RebuildDataURI(mimeType, raw, base64Image)
		refs, rerr := animatedPerceptionRefs(raw)
		if rerr != nil {
			logger.Warn("FileManager", "动态图感知预处理失败（%v），回退单图理解", rerr)
			return storeURI, nil, true
		}
		logger.Info("FileManager", "动态图标准化完成: 存储=%d bytes, 采样帧引用=%d 条", len(raw), len(refs))
		return storeURI, refs, true
	}

	// 静态图：缩放 + 统一转码
	uri, serr := multimodal.StandardizeImageForPerception(raw, memoryStandardMaxDim)
	if serr != nil {
		logger.Warn("FileManager", "静态图标准化失败（%v），按原始图片入库", serr)
		return persistence.RebuildDataURI(mimeType, raw, base64Image), nil, false
	}
	logger.Info("FileManager", "静态图标准化完成: %d bytes -> %d bytes", len(raw), len(uri))
	return uri, nil, false
}
