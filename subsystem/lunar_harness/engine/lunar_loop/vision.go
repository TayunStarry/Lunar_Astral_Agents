package lunarloop

// vision.go 视觉读取管线：思维链中静态图的统一消费形态
//
// 无论来源是 URL 还是 data URI，静态图入库后统一以「约束尺寸 + 句柄叠印」的副本流转：
//   - url 形态：/file/miniature/<blob路径>?tag=<句柄>（服务端按需变换并缓存副本）
//   - base64 形态：visionRead 经同一 VisionCopy 管线直出 data URI（供云端模型使用）
//
// 原图 blob 永远不被修改，也不进入模型上下文：扩散引擎的 initImg、超分等 Go 侧管线
// 直接使用原图路径，思维链侧只消费视觉读取副本（见 docs/adr/0008）。

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	mmmodule "LunarSubsystem/LunarHarness/engine/multimodal"
	filemodule "LunarSubsystem/LunarHarness/engine/persistence"

	"github.com/dop251/goja"
)

// visionTagPattern 合法句柄标签形态（叠印文本白名单，防止向副本烧写任意文字）
var visionTagPattern = regexp.MustCompile(`^@[A-Z]\.\d{3,}$`)

// visionImageExts 视觉读取端点允许的图片扩展名（仅图片类文件可读）
var visionImageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".bmp": true,
}

// VisionReadBlob 取思维链消费副本：缩放到约束尺寸、右上角叠印句柄、按内容寻址盘上缓存
//
// @param relPath 源图相对 LocalDir 路径（multimedia/ 下）
// @param tag     叠印句柄（空串时按 blob 路径从资源目录反查）
// @param side    副本最大边长（0 = 取配置 vision_max_side）
// @return 副本字节、contentType
func VisionReadBlob(relPath, tag string, side int) ([]byte, string, error) {
	if side <= 0 {
		side = *config.VisionMaxSide
	}
	// 路径规整与越界防护（与 ReadFile 同一套约定：相对 LocalDir 解析）
	clean := filepath.ToSlash(filepath.Clean(relPath))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(clean) {
		return nil, "", fmt.Errorf("非法路径: %s", relPath)
	}
	if !strings.HasPrefix(clean, "multimedia/") {
		return nil, "", fmt.Errorf("视觉读取端点仅允许访问 multimedia/ 下的文件: %s", relPath)
	}
	ext := strings.ToLower(filepath.Ext(clean))
	if !visionImageExts[ext] {
		return nil, "", fmt.Errorf("视觉读取端点仅允许图片类文件: %s", relPath)
	}
	// 叠印文本白名单校验：非法形态一律丢弃，改为按资源目录反查
	if !visionTagPattern.MatchString(tag) {
		tag = ""
	}
	// 确保资源目录就绪（幂等），随后反查句柄与内容哈希
	if err := filemodule.EnsureArchiveInitialized(); err != nil {
		return nil, "", fmt.Errorf("资源目录初始化失败: %w", err)
	}
	contentHash := ""
	if entry, ok := filemodule.ArchiveDatabase.FindResourceByBlobPath(clean); ok {
		if tag == "" {
			tag = entry.Handle
		}
		contentHash = entry.ContentHash
	}
	if contentHash == "" {
		// 目录未登记（历史别名等）：以路径哈希为缓存键，语义从「内容寻址」退化为「路径寻址」
		sum := sha256.Sum256([]byte(clean))
		contentHash = hex.EncodeToString(sum[:8])
	}
	cacheRel := "multimedia/vision/" + contentHash + "_s" + strconv.Itoa(side) + ".jpg"
	data, mime, cached, err := mmmodule.VisionTransform(
		filepath.Join(*config.LocalDir, clean),
		filepath.Join(*config.LocalDir, cacheRel),
		side, tag,
	)
	if err != nil {
		return nil, "", err
	}
	if cached {
		// 缓存命中：续期 used_at（LRU 淘汰依据，最近读取的最后淘汰）
		filemodule.VisionCacheTouch(contentHash, side)
		return data, mime, nil
	}
	// 新构建副本：簿记入表，随后按容量上限做 LRU 淘汰（副本为派生数据，淘汰可随时重建）
	_ = filemodule.VisionCachePut(filemodule.VisionCacheEntry{
		ContentHash: contentHash,
		Side:        side,
		Tag:         tag,
		CachePath:   cacheRel,
		Bytes:       int64(len(data)),
	})
	if removed, freed := filemodule.VisionCacheEvict(int64(*config.VisionCacheCapMB) << 20); removed > 0 {
		fmt.Printf("[视觉缓存] 容量超限，已淘汰 %d 个副本，释放 %d 字节\n", removed, freed)
	}
	return data, mime, nil
}

// VisionCopy 取思维链消费副本并附带像素尺寸
//
// 与 VisionReadBlob 同一变换（缩放 + 叠印句柄），额外返回副本宽高，供需要比例基准的
// 调用方（绘制者参考图、资源取回）不必再自己解码图片——更不必调用任何缩放入口。
//
// @param relPath 源图相对 LocalDir 路径（multimedia/ 下）
// @param tag     叠印句柄（空串时按 blob 路径从资源目录反查）
// @param side    副本最大边长（0 = 取配置 vision_max_side）
// @return 副本字节、contentType、宽、高、错误
func VisionCopy(relPath, tag string, side int) (data []byte, mime string, width, height int, err error) {
	data, mime, err = VisionReadBlob(relPath, tag, side)
	if err != nil {
		return nil, "", 0, 0, err
	}
	// 只读文件头取宽高（不整图解码）：副本恒为 JPEG，成本可忽略
	if cfg, _, decodeErr := image.DecodeConfig(bytes.NewReader(data)); decodeErr == nil {
		width, height = cfg.Width, cfg.Height
	}
	return data, mime, width, height, nil
}

// visionRead 适配TypeScript调用：按视觉读取管线生成副本并直接返回 data URI（base64 形态专用）
//
// 思维链里图片字节**唯一**的合法来源（url 形态走 /file/miniature/ 端点，
// base64 形态走这里）。resizeImage 已从 goja 卸载：模型侧不提供任何"自己缩放"的能力。
//
// 入参: (relPath, tag, side?)  返回值: [dataURI, error, width, height]
func (class *Runtime) visionRead(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("参数不足"), 0, 0})
	}
	relPath := call.Argument(0).String()
	tag := ""
	if arg := call.Argument(1); arg != nil && !goja.IsUndefined(arg) && !goja.IsNull(arg) {
		tag = arg.String()
	}
	side := asInt(call.Argument(2).Export())
	data, mime, width, height, err := VisionCopy(relPath, tag, side)
	if err != nil {
		return class.runtime.ToValue([]any{nil, err, 0, 0})
	}
	dataURI := fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(data))
	return class.runtime.ToValue([]any{dataURI, nil, width, height})
}
