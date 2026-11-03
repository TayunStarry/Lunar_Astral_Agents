package config

// ==== 绘图配置（独立文件 local_data/config/painter.json） ====
//
// 绘图相关配置（模型方案 + 扩散生成策略）唯一来源是本文件，general.json 不再承载
// 这些字段。painter.json 采用扁平结构：default_scheme、扩散参数与各模型方案全部在
// 顶层平铺，方案名本身就是顶层键（不硬编码方案名），便于单独维护与打包。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// PainterConfigFile 独立绘图配置文件名（相对 LocalDir/config）
const PainterConfigFile = "painter.json"

// ModelScheme 一套绘图模型方案的原始配置（与 DiffusionScheme 一一对应）
//
// 扩散模型、VAE、提示词编码模型必须成套匹配，因此按方案整组声明，
// 避免调用方把不同方案的模型混用成非法组合。
type ModelScheme struct {
	Label               string `json:"label"`                 // 界面显示名（空则用方案键名）
	DiffusionModel      string `json:"diffusion_model"`       // 扩散模型路径
	VariationalModel    string `json:"variational_model"`     // 变分模型路径
	PromptAnalysisModel string `json:"prompt_analysis_model"` // 提示分析模型路径
	PromptMmprojModel   string `json:"prompt_mmproj_model"`   // 多模态提示分析模型路径（可空）
	RealESRGANModel     string `json:"real_esrgan_model"`     // 4x超分辨率模型路径（可空）
	Steps               int    `json:"steps"`                 // 方案推荐默认步数（0=沿用 sd_steps）
}

// PainterDiffusionConfig 图像生成（sd.cpp）行为与资源策略（painter.json 顶层平铺的扩散参数）
type PainterDiffusionConfig struct {
	AllowDiffusion bool `json:"allow_diffusion"` // 是否允许运行扩散生成
	// 画图前显存守卫：使用指针以便区分"未配置"与"显式关闭"，未配置时保持默认值（开启，阈值 8192 MiB）
	ImageVRAMGuard    *bool `json:"image_vram_guard,omitempty"`
	ImageVRAMGuardMiB *int  `json:"image_vram_guard_mib,omitempty"`
	// sd.cpp 权重内存卸载模式（auto/always/off）与提示词编码器 CPU 运行开关
	SDOffloadToCPU     *string `json:"sd_offload_to_cpu,omitempty"`
	SDTextEncoderOnCPU *bool   `json:"sd_te_on_cpu,omitempty"`
	// VAE 分片解码策略与几何（off/always/auto；tile 以 latent 单元计，overlap 为比例）
	SDVaeTiling      *string  `json:"sd_vae_tiling,omitempty"`
	SDVaeTileSize    *string  `json:"sd_vae_tile_size,omitempty"`
	SDVaeTileOverlap *float64 `json:"sd_vae_tile_overlap,omitempty"`
	// VAE 解码后端（auto=放不进显存就整段改 CPU / gpu / cpu）与 GPU direct conv 开关
	SDVaeBackend    *string `json:"sd_vae_backend,omitempty"`
	SDVaeConvDirect *bool   `json:"sd_vae_conv_direct,omitempty"`
	// 显存预算（GiB，空=自动）与采样策略（auto=交给 sd.cpp 按模型选择）
	SDMaxVRAM        *string  `json:"sd_max_vram,omitempty"`
	SDSampler        *string  `json:"sd_sampler,omitempty"`
	SDScheduler      *string  `json:"sd_scheduler,omitempty"`
	SDFlowShift      *float64 `json:"sd_flow_shift,omitempty"`
	SDFlashAttention *bool    `json:"sd_diffusion_fa,omitempty"`
	// 调用方未指定时的默认步数与边长
	SDSteps     *int `json:"sd_steps,omitempty"`
	SDImageSize *int `json:"sd_image_size,omitempty"`
	// 图生图/合照按参考图比例推导尺寸时的总像素预算上限（百万像素，默认 1.6）
	PixelBudgetMP *float64 `json:"pixel_budget_mp,omitempty"`
	// 图生图的提示词引导系数覆盖值（<=0 表示不覆盖，按 1.0）
	Img2ImgCFG *float64 `json:"img2img_cfg,omitempty"`
}

// PainterConfig 绘图配置的内存表示（painter.json 为扁平结构）
type PainterConfig struct {
	DefaultScheme string                 // 默认启用的方案键名（缺省取排序后的第一个）
	Schemes       map[string]ModelScheme // 顶层方案键 → 成套模型路径（方案名不固定）
	Diffusion     PainterDiffusionConfig // 顶层平铺的扩散参数
}

// UnmarshalJSON 按扁平结构解析 painter.json：
//
//   - default_scheme 与扩散参数键从顶层读取；
//   - 其余以对象给出的顶层键一律视为方案定义（方案名不硬编码）；
//   - 未知标量键忽略并记 WARN，避免拼写错误悄悄污染配置；
//   - 旧版嵌套节（models / diffusion / schemes）不再解析，仅提示迁移。
func (c *PainterConfig) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	// 扩散参数整体解码：结构体只会挑走已知键，未知键留给下面的方案识别。
	var diffusion PainterDiffusionConfig
	if err := json.Unmarshal(data, &diffusion); err != nil {
		return err
	}
	c.Diffusion = diffusion
	c.Schemes = make(map[string]ModelScheme)
	if raw, ok := fields["default_scheme"]; ok {
		if err := json.Unmarshal(raw, &c.DefaultScheme); err != nil {
			return fmt.Errorf("default_scheme 解析失败: %w", err)
		}
	}
	for key, raw := range fields {
		if key == "default_scheme" || isPainterDiffusionKey(key) {
			continue
		}
		if isLegacyPainterSection(key) {
			log.Printf("[Config][WARN] -> painter.json 检测到旧嵌套节 %q，已忽略；请改用扁平格式（方案名直接作为顶层键）", key)
			continue
		}
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 || trimmed[0] != '{' {
			log.Printf("[Config][WARN] -> painter.json 忽略未知标量键 %q（既不是方案对象也不是已知扩散参数）", key)
			continue
		}
		var scheme ModelScheme
		if err := json.Unmarshal(raw, &scheme); err != nil {
			return fmt.Errorf("方案 %q 解析失败: %w", key, err)
		}
		c.Schemes[key] = scheme
	}
	return nil
}

// isPainterDiffusionKey 判断顶层键是否为已知的扩散参数键。
func isPainterDiffusionKey(key string) bool {
	switch key {
	case "allow_diffusion",
		"image_vram_guard", "image_vram_guard_mib",
		"sd_offload_to_cpu", "sd_te_on_cpu",
		"sd_vae_tiling", "sd_vae_tile_size", "sd_vae_tile_overlap",
		"sd_vae_backend", "sd_vae_conv_direct",
		"sd_max_vram", "sd_sampler", "sd_scheduler", "sd_flow_shift", "sd_diffusion_fa",
		"sd_steps", "sd_image_size",
		"pixel_budget_mp", "img2img_cfg":
		return true
	}
	return false
}

// isLegacyPainterSection 判断是否为旧版嵌套结构的分组键（仅用于提示迁移，不再解析）。
func isLegacyPainterSection(key string) bool {
	switch key {
	case "models", "schemes", "diffusion":
		return true
	}
	return false
}

// LoadPainterConfigFile 读取 dir 下的 painter.json；文件不存在时返回 (nil,false,nil)。
func LoadPainterConfigFile(dir string) (*PainterConfig, bool, error) {
	path := filepath.Join(dir, PainterConfigFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var cfg PainterConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, false, err
	}
	return &cfg, true, nil
}

// applyPainterConfig 把绘图配置写入全局变量。
//
// 规则与原 general.json 的 models / diffusion 完全一致：非空才覆盖、指针字段区分
// 「未配置」与「显式关闭」，缺省一律保持代码内建默认值。
func applyPainterConfig(parameter *PainterConfig) {
	// ==== 绘图模型方案（顶层方案键）：注册后立即启用默认方案 ====
	if len(parameter.Schemes) > 0 {
		schemes := make(map[string]DiffusionScheme, len(parameter.Schemes))
		for name, raw := range parameter.Schemes {
			schemes[name] = DiffusionScheme{
				Label:               raw.Label,
				DiffusionModel:      raw.DiffusionModel,
				VariationalModel:    raw.VariationalModel,
				PromptAnalysisModel: raw.PromptAnalysisModel,
				PromptMmprojModel:   raw.PromptMmprojModel,
				RealESRGANModel:     raw.RealESRGANModel,
				Steps:               raw.Steps,
			}
		}
		SetDiffusionSchemes(parameter.DefaultScheme, schemes)
	}

	// ==== 图像生成行为与资源策略（顶层扩散参数） ====
	if parameter.Diffusion.AllowDiffusion == true {
		AllowDiffusion = true
	} else {
		AllowDiffusion = false
	}
	// 画图前显存守卫配置：仅在配置文件中显式给出时才覆盖默认值
	if parameter.Diffusion.ImageVRAMGuard != nil {
		ImageVRAMGuard = *parameter.Diffusion.ImageVRAMGuard
	}
	if parameter.Diffusion.ImageVRAMGuardMiB != nil {
		ImageVRAMGuardMiB = *parameter.Diffusion.ImageVRAMGuardMiB
	}
	// sd.cpp 权重内存卸载与提示词编码器 CPU 运行配置：仅在配置文件中显式给出时才覆盖默认值
	if parameter.Diffusion.SDOffloadToCPU != nil && *parameter.Diffusion.SDOffloadToCPU != "" {
		SDOffloadToCPU = *parameter.Diffusion.SDOffloadToCPU
	}
	if parameter.Diffusion.SDTextEncoderOnCPU != nil {
		SDTextEncoderOnCPU = *parameter.Diffusion.SDTextEncoderOnCPU
	}
	// VAE 分片解码策略：仅在配置文件显式给出非空值时才覆盖默认值（默认 off）
	if parameter.Diffusion.SDVaeTiling != nil && *parameter.Diffusion.SDVaeTiling != "" {
		SDVaeTiling = *parameter.Diffusion.SDVaeTiling
	}
	if parameter.Diffusion.SDVaeTileSize != nil && *parameter.Diffusion.SDVaeTileSize != "" {
		SDVaeTileSize = *parameter.Diffusion.SDVaeTileSize
	}
	if parameter.Diffusion.SDVaeTileOverlap != nil {
		SDVaeTileOverlap = *parameter.Diffusion.SDVaeTileOverlap
	}
	// VAE 解码后端与 direct conv：仅显式给出时覆盖（默认 auto / 关闭）
	if parameter.Diffusion.SDVaeBackend != nil && *parameter.Diffusion.SDVaeBackend != "" {
		SDVaeBackend = *parameter.Diffusion.SDVaeBackend
	}
	if parameter.Diffusion.SDVaeConvDirect != nil {
		SDVaeConvDirect = *parameter.Diffusion.SDVaeConvDirect
	}
	// 显存预算与采样策略：空字符串表示"使用引擎自动决策"，因此只在非空时覆盖
	if parameter.Diffusion.SDMaxVRAM != nil {
		SDMaxVRAM = *parameter.Diffusion.SDMaxVRAM
	}
	if parameter.Diffusion.SDSampler != nil && *parameter.Diffusion.SDSampler != "" {
		SDSampler = *parameter.Diffusion.SDSampler
	}
	if parameter.Diffusion.SDScheduler != nil && *parameter.Diffusion.SDScheduler != "" {
		SDScheduler = *parameter.Diffusion.SDScheduler
	}
	if parameter.Diffusion.SDFlowShift != nil {
		SDFlowShift = *parameter.Diffusion.SDFlowShift
	}
	if parameter.Diffusion.SDFlashAttention != nil {
		SDFlashAttention = *parameter.Diffusion.SDFlashAttention
	}
	// 缺省步数与边长：仅接受正数，避免非法配置把生成参数打成 0
	if parameter.Diffusion.SDSteps != nil && *parameter.Diffusion.SDSteps > 0 {
		SDSteps = *parameter.Diffusion.SDSteps
	}
	if parameter.Diffusion.SDImageSize != nil && *parameter.Diffusion.SDImageSize > 0 {
		SDImageSize = *parameter.Diffusion.SDImageSize
	}
	// 绘图尺寸预算与图生图 CFG 覆盖
	if parameter.Diffusion.PixelBudgetMP != nil && *parameter.Diffusion.PixelBudgetMP > 0 {
		DiffusionPixelBudgetMP = *parameter.Diffusion.PixelBudgetMP
	}
	if parameter.Diffusion.Img2ImgCFG != nil && *parameter.Diffusion.Img2ImgCFG > 0 {
		DiffusionImg2ImgCFG = *parameter.Diffusion.Img2ImgCFG
	}
}
