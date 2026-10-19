package persistence

// 图片入库标准化（感知者式预处理）：
// 静态图 → 缩放（宽或高 >640 等比缩放）+ 统一转码（JPEG/PNG data URI）；
// 动态图（GIF / APNG / 动态 WebP）→ 存储保留原动画文件，
// 理解引用由多模态模块按「开头稠密、结尾稀疏」的规则选帧（长边 ≤640、最多 64 帧）。
// 任何一步失败均回退原始输入，不阻塞入库。

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math"
	"path"
	"strings"

	multimodal "CorePersonality/engine/multimodal"
	"LunarSubsystem/LoggerGeneral"
)

// maxMemoryMediaSegments 动态图理解的最大帧数（与多模态模块的 AnimatedMaxUnderstandFrames 对齐）。
const maxMemoryMediaSegments = multimodal.AnimatedMaxUnderstandFrames

// memoryStandardMaxDim 静态图入库最大边长。
const memoryStandardMaxDim = 640

// stripDataURI 剥离图片的 data URI 前缀，返回 mime 与原始字节。
// 兼容「data:image/...;base64,<payload>」与裸 base64（按魔数嗅探 mime）。
func stripDataURI(base64Image string) (mimeType string, raw []byte, err error) {
	s := strings.TrimSpace(base64Image)
	if s == "" {
		return "", nil, fmt.Errorf("图片数据为空")
	}
	if strings.HasPrefix(s, "data:") {
		comma := strings.Index(s, ",")
		if comma < 0 {
			return "", nil, fmt.Errorf("data URI 格式非法（缺少逗号分隔）")
		}
		header := s[:comma]
		mimeType = strings.TrimPrefix(strings.Split(header, ";")[0], "data:")
		raw, err = base64.StdEncoding.DecodeString(s[comma+1:])
		if err != nil {
			return "", nil, fmt.Errorf("base64 解码失败: %v", err)
		}
		return mimeType, raw, nil
	}
	// 裸 base64：解码后按魔数嗅探
	raw, err = base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", nil, fmt.Errorf("base64 解码失败: %v", err)
	}
	return sniffImageMIME(raw), raw, nil
}

// sniffImageFormat 按文件头魔数识别图片真实格式，返回规范扩展名与 MIME。
// 无法识别时返回空串，由调用方决定兜底策略。
//
// 为什么必须嗅探内容：QQ 等渠道经常把 GIF / 动态 WebP 命名为 .png、.jpg，
// data URI 里的 MIME 声明也随之失真。扩展名与声明都不可信，只有字节可信。
func sniffImageFormat(raw []byte) (ext string, mimeType string) {
	switch {
	case len(raw) >= 3 && raw[0] == 0xFF && raw[1] == 0xD8 && raw[2] == 0xFF:
		return ".jpg", "image/jpeg"
	case len(raw) >= 8 && raw[0] == 0x89 && raw[1] == 0x50 && raw[2] == 0x4E && raw[3] == 0x47:
		return ".png", "image/png"
	case len(raw) >= 4 && raw[0] == 0x47 && raw[1] == 0x49 && raw[2] == 0x46 && raw[3] == 0x38:
		return ".gif", "image/gif"
	case len(raw) >= 12 && string(raw[0:4]) == "RIFF" && string(raw[8:12]) == "WEBP":
		return ".webp", "image/webp"
	case len(raw) >= 2 && raw[0] == 0x42 && raw[1] == 0x4D:
		return ".bmp", "image/bmp"
	default:
		return "", ""
	}
}

// sniffImageMIME 按文件头魔数嗅探图片 mime（未知格式回退 image/png）。
func sniffImageMIME(raw []byte) string {
	if _, mimeType := sniffImageFormat(raw); mimeType != "" {
		return mimeType
	}
	return "image/png"
}

// rebuildDataURI 重组为完整 data URI；原始串已带前缀且 mime 与内容一致时直接复用，避免无谓的重编码。
func rebuildDataURI(mimeType string, raw []byte, original string) string {
	trimmed := strings.TrimSpace(original)
	if mimeType == "" {
		mimeType = sniffImageMIME(raw)
	}
	if strings.HasPrefix(trimmed, "data:") {
		// 声明 MIME 可能与内容不符（QQ 常把 GIF 动图标成 image/png）：
		// 只有声明与嗅探结果一致时才原样复用，否则按真实格式重建。
		if comma := strings.Index(trimmed, ","); comma > 0 {
			declared := strings.TrimPrefix(strings.Split(trimmed[:comma], ";")[0], "data:")
			if strings.EqualFold(declared, mimeType) {
				return trimmed
			}
		}
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(raw)
}

// sampleMediaRefs 片段引用等距抽样：最多 max 段，恒包含首段与末段（感知者 sampleSegments 同款）。
func sampleMediaRefs(refs []string, max int) []string {
	if len(refs) <= max {
		return refs
	}
	picked := make([]string, 0, max)
	for i := 0; i < max; i++ {
		idx := int(math.Round(float64(i) * float64(len(refs)-1) / float64(max-1)))
		picked = append(picked, refs[idx])
	}
	// 取整可能产生重复索引，去重后保持顺序
	seen := make(map[string]bool, len(picked))
	out := picked[:0]
	for _, r := range picked {
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	return out
}

// animatedPerceptionRefs 生成动态图的帧序列引用（最多 64 帧 data URI）
//
// 动态图不再视频化：多模态模块会先探测总帧数，再按「开头稠密、结尾稀疏」的
// 分组规则选出最多 64 帧（长边 ≤640），这里只做引用转换与最终上限兜底。
func animatedPerceptionRefs(raw []byte) ([]string, error) {
	groups, err := multimodal.AnimatedImageToFrames(raw)
	if err != nil {
		return nil, err
	}
	refs := make([]string, 0)
	for _, group := range groups {
		for _, frame := range group.Frames {
			refs = append(refs, frame.Image)
		}
	}
	return sampleMediaRefs(refs, maxMemoryMediaSegments), nil
}

// saveMemoryImageFile 把图片实体写入 multimedia/<集合名>/，返回前端可读取的 URL
//
// 命名沿用内容哈希（同内容只存一份），由 SaveFileEx 顺带登记句柄映射。
func saveMemoryImageFile(collectionName, dataURI string) (string, error) {
	mimeType, raw, err := stripDataURI(dataURI)
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
	if sniffExt, _ := sniffImageFormat(raw); sniffExt != "" {
		ext = sniffExt
	}
	sum := sha256.Sum256(raw)
	rel := path.Join(StorageRootMultimedia, collectionName, hex.EncodeToString(sum[:])[:16]+ext)
	result, saveErr := SaveFileEx(rel, SaveOptions{Overwrite: true, MustStore: true}, bytes.NewReader(raw))
	if saveErr != nil {
		return "", saveErr
	}
	// 统一以 /file/read/ 形式返回（前端与 QQ 适配器都按该约定解析）
	return "/file/read/" + result.RelPath, nil
}

// standardizeMemoryImage 记忆库图片入库前的标准化：
// 返回 storeURI（入库与去重用的完整 data URI）、mediaRefs（动态图的感知者式采样帧引用，
// 静态图为 nil）、animated（是否动态图）。任一步失败回退原始输入（记 Warning，不返回错误）。
func standardizeMemoryImage(base64Image string) (storeURI string, mediaRefs []string, animated bool) {
	mimeType, raw, err := stripDataURI(base64Image)
	if err != nil {
		LoggerGeneral.Warn("FileManager", "图片标准化跳过（%v），按原始图片入库", err)
		return strings.TrimSpace(base64Image), nil, false
	}

	// 动态图：存储保留原动画 data URI；理解引用按配置的输入模式生成
	if multimodal.IsAnimatedImage(raw) {
		// 声明 MIME 与内容不符时（QQ 常把 GIF 标成 image/png）以嗅探结果为准
		if _, realMIME := sniffImageFormat(raw); realMIME != "" {
			mimeType = realMIME
		}
		storeURI = rebuildDataURI(mimeType, raw, base64Image)
		refs, rerr := animatedPerceptionRefs(raw)
		if rerr != nil {
			LoggerGeneral.Warn("FileManager", "动态图感知预处理失败（%v），回退单图理解", rerr)
			return storeURI, nil, true
		}
		LoggerGeneral.Info("FileManager", "动态图标准化完成: 存储=%d bytes, 采样帧引用=%d 条", len(raw), len(refs))
		return storeURI, refs, true
	}

	// 静态图：缩放 + 统一转码
	uri, serr := multimodal.StandardizeImageForPerception(raw, memoryStandardMaxDim)
	if serr != nil {
		LoggerGeneral.Warn("FileManager", "静态图标准化失败（%v），按原始图片入库", serr)
		return rebuildDataURI(mimeType, raw, base64Image), nil, false
	}
	LoggerGeneral.Info("FileManager", "静态图标准化完成: %d bytes -> %d bytes", len(raw), len(uri))
	return uri, nil, false
}
