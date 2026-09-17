package module

// 图片入库标准化（感知者式预处理）：
// 静态图 → 缩放（宽或高 >640 等比缩放）+ 统一转码（JPEG/PNG data URI）；
// 动态图（GIF / APNG / 动态 WebP）→ 存储保留原动画 data URI（供前端 <img> 动画渲染），
// 理解走 AnimatedImageToMedia 慢放视频 + file:// 引用（llama-server 端 ffmpeg 抽帧），
// 与月华感知者的动态图链路同构；片段 >8 等距抽样（首末必含）。
// 任何一步失败均回退原始输入，不阻塞入库。

import (
	"encoding/base64"
	"fmt"
	"math"
	"strings"

	"LunarSubsystem/LoggerGeneral"
	multimodal "LunarSubsystem/MultimodalAnalysis/module"
)

// maxMemoryMediaSegments 动态图视频理解的最大片段引用数（感知者 MAX_SEGMENTS 同款）。
const maxMemoryMediaSegments = 8

// memoryStandardMaxDim 静态图入库最大边长（与感知者视频 640px 约束一致，规避 llama-server 视觉上限）。
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

// sniffImageMIME 按文件头魔数嗅探图片 mime（未知格式回退 image/png）。
func sniffImageMIME(raw []byte) string {
	switch {
	case len(raw) >= 3 && raw[0] == 0xFF && raw[1] == 0xD8 && raw[2] == 0xFF:
		return "image/jpeg"
	case len(raw) >= 8 && raw[0] == 0x89 && raw[1] == 0x50 && raw[2] == 0x4E && raw[3] == 0x47:
		return "image/png"
	case len(raw) >= 4 && raw[0] == 0x47 && raw[1] == 0x49 && raw[2] == 0x46 && raw[3] == 0x38:
		return "image/gif"
	case len(raw) >= 12 && string(raw[0:4]) == "RIFF" && string(raw[8:12]) == "WEBP":
		return "image/webp"
	default:
		return "image/png"
	}
}

// rebuildDataURI 重组为完整 data URI；原始串已带前缀且 mime 一致时直接复用，避免无谓的重编码。
func rebuildDataURI(mimeType string, raw []byte, original string) string {
	trimmed := strings.TrimSpace(original)
	if strings.HasPrefix(trimmed, "data:") {
		return trimmed
	}
	if mimeType == "" {
		mimeType = sniffImageMIME(raw)
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

// standardizeMemoryImage 记忆库图片入库前的标准化：
// 返回 storeURI（入库与去重用的完整 data URI）、mediaRefs（动态图视频理解的 file:// 引用，
// 静态图为 nil）、animated（是否动态图）。任一步失败回退原始输入（记 Warning，不返回错误）。
func standardizeMemoryImage(base64Image string) (storeURI string, mediaRefs []string, animated bool) {
	mimeType, raw, err := stripDataURI(base64Image)
	if err != nil {
		LoggerGeneral.Warn("FileManager", "图片标准化跳过（%v），按原始图片入库", err)
		return strings.TrimSpace(base64Image), nil, false
	}

	// 动态图：存储保留原动画 data URI；理解转慢放视频走 file:// 引用
	if multimodal.IsAnimatedImage(raw) {
		storeURI = rebuildDataURI(mimeType, raw, base64Image)
		segments, serr := multimodal.AnimatedImageToMedia(raw)
		if serr != nil {
			LoggerGeneral.Warn("FileManager", "动态图转视频失败（%v），回退单图理解", serr)
			return storeURI, nil, true
		}
		refs := make([]string, 0, len(segments))
		for _, seg := range segments {
			refs = append(refs, "file://"+seg.File)
		}
		refs = sampleMediaRefs(refs, maxMemoryMediaSegments)
		LoggerGeneral.Info("FileManager", "动态图标准化完成: 存储=%d bytes, 视频片段=%d 段", len(raw), len(refs))
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
