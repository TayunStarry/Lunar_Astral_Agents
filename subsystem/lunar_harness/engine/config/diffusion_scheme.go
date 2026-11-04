package config

import (
	"log"
	"sort"
	"strings"
	"sync"
)

// DiffusionScheme 一套绘图模型方案：扩散模型 + VAE + 提示词编码模型 + 超分模型。
//
// 为什么需要方案而不是一组扁平路径：不同方案往往使用完全不同的三类模型
// （扩散/VAE/文本编码器必须成套匹配），把它们拆成独立字段由调用方自由组合
// 只会产生「扩散是 A、VAE 是 B」的非法组合。方案把「成套」这件事固化在配置
// 结构里，启动时整组应用。
type DiffusionScheme struct {
	// Name 方案键名（配置中的对象键，如 turbo / quality）
	Name string
	// Label 供界面显示的方案名（缺省回退 Name）
	Label string
	// DiffusionModel 扩散模型路径
	DiffusionModel string
	// VariationalModel VAE 模型路径
	VariationalModel string
	// PromptAnalysisModel 提示词编码/精炼模型路径
	PromptAnalysisModel string
	// PromptMmprojModel 多模态提示词投影模型路径（可空）
	PromptMmprojModel string
	// RealESRGANModel 4x 超分模型路径（可空）
	RealESRGANModel string
	// Steps 该方案的推荐默认步数（调用方未指定时使用；0 表示沿用 sd_steps）
	Steps int
}

var (
	// diffusionSchemeMu 保护方案表与默认方案
	diffusionSchemeMu sync.RWMutex
	// diffusionSchemes 已注册的方案（键名统一小写）
	diffusionSchemes = map[string]DiffusionScheme{}
	// diffusionSchemeOrder 稳定的方案顺序（按键名排序，默认方案无效时回退首个）
	diffusionSchemeOrder []string
	// defaultDiffusionScheme 配置声明的默认方案（启动时应用）
	defaultDiffusionScheme string
)

// normalizeSchemeName 归一化方案键名（大小写与空白不敏感）
func normalizeSchemeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// SetDiffusionSchemes 注册配置中的绘图模型方案，并在启动时应用一次默认方案。
//
// 未配置任何方案时清空状态，调用方（ResolveDiffusionScheme）会回退到 models 下的
// 扁平模型路径，保持旧配置文件的行为不变。
func SetDiffusionSchemes(defaultName string, schemes map[string]DiffusionScheme) {
	normalized := make(map[string]DiffusionScheme, len(schemes))
	order := make([]string, 0, len(schemes))
	for name, scheme := range schemes {
		key := normalizeSchemeName(name)
		if key == "" || strings.TrimSpace(scheme.DiffusionModel) == "" {
			log.Printf("[Config][WARN] -> painter.json 方案 %q 缺少 diffusion_model，已忽略该方案", name)
			continue
		}
		scheme.Name = key
		if strings.TrimSpace(scheme.Label) == "" {
			scheme.Label = key
		}
		normalized[key] = scheme
		order = append(order, key)
	}
	sort.Strings(order)

	diffusionSchemeMu.Lock()
	defer diffusionSchemeMu.Unlock()
	diffusionSchemes = normalized
	diffusionSchemeOrder = order
	if len(normalized) == 0 {
		defaultDiffusionScheme = ""
		return
	}
	key := normalizeSchemeName(defaultName)
	if _, ok := normalized[key]; !ok {
		key = order[0]
		log.Printf("[Config][WARN] -> painter.json default_scheme=%q 不存在，回退到 %s", defaultName, key)
	}
	defaultDiffusionScheme = key
	applySchemeLocked(normalized[key])
}

// applySchemeLocked 把方案写入全局模型路径（调用方必须持有写锁）。
//
// 这里只在启动时按 default_scheme 调用一次，同步全局值与扁平配置读取路径一致。
func applySchemeLocked(scheme DiffusionScheme) {
	// 整组覆盖而不是「非空才覆盖」：方案是成套声明，若允许空值继承，
	// 未配置超分模型时会继续沿用上一组的超分模型，形成隐性串味。
	DiffusionModel = scheme.DiffusionModel
	VariationalModel = scheme.VariationalModel
	PromptAnalysisModel = scheme.PromptAnalysisModel
	PromptMmprojModel = scheme.PromptMmprojModel
	RealESRGANModel = scheme.RealESRGANModel
}

// ResolveDiffusionScheme 取启动时应用的默认方案模型路径；未注册方案时回退扁平配置。
//
// 返回的是值拷贝，调用方可安全持有；运行期不再切换方案。
func ResolveDiffusionScheme() DiffusionScheme {
	diffusionSchemeMu.RLock()
	defer diffusionSchemeMu.RUnlock()
	if len(diffusionSchemes) == 0 {
		return DiffusionScheme{
			DiffusionModel:      DiffusionModel,
			VariationalModel:    VariationalModel,
			PromptAnalysisModel: PromptAnalysisModel,
			PromptMmprojModel:   PromptMmprojModel,
			RealESRGANModel:     RealESRGANModel,
			Steps:               SDSteps,
		}
	}
	scheme, ok := diffusionSchemes[defaultDiffusionScheme]
	if !ok {
		scheme = diffusionSchemes[diffusionSchemeOrder[0]]
	}
	if scheme.Steps <= 0 {
		scheme.Steps = SDSteps
	}
	return scheme
}
