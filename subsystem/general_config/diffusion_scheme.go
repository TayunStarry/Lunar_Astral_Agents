package GeneralConfig

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
)

// DiffusionScheme 一套可切换的绘图模型方案：扩散模型 + VAE + 提示词编码模型 + 超分模型。
//
// 为什么需要方案而不是一组扁平路径：turbo（速度优先）与 quality（质量优先）往往
// 使用完全不同的三类模型（扩散/VAE/文本编码器必须成套匹配），把它们拆成独立字段
// 由调用方自由组合只会产生「扩散是 A、VAE 是 B」的非法组合。方案把「成套」这件事
// 固化在配置结构里，切换时整组生效。
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

// DiffusionSchemeInfo 方案摘要（HTTP 与智能体查询用，不含本地路径）
type DiffusionSchemeInfo struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	Default bool   `json:"default"`
	Current bool   `json:"current"`
	Steps   int    `json:"steps,omitempty"`
}

var (
	// diffusionSchemeMu 保护方案表与当前方案；生成任务在创建时取快照，避免切换期间串味
	diffusionSchemeMu sync.RWMutex
	// diffusionSchemes 已注册的方案（键名统一小写）
	diffusionSchemes = map[string]DiffusionScheme{}
	// diffusionSchemeOrder 稳定的方案展示顺序（按键名排序）
	diffusionSchemeOrder []string
	// defaultDiffusionScheme 配置声明的默认方案
	defaultDiffusionScheme string
	// activeDiffusionScheme 当前生效方案（由 Go 层统一记录，智能体与前端共享）
	activeDiffusionScheme string
)

// normalizeSchemeName 归一化方案键名（大小写与空白不敏感）
func normalizeSchemeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// SetDiffusionSchemes 注册配置中的绘图模型方案，并立刻启用默认方案。
//
// 未配置任何方案时清空状态，调用方（ResolveDiffusionScheme）会回退到 models 下的
// 扁平模型路径，保持旧配置文件的行为不变。
func SetDiffusionSchemes(defaultName string, schemes map[string]DiffusionScheme) {
	normalized := make(map[string]DiffusionScheme, len(schemes))
	order := make([]string, 0, len(schemes))
	for name, scheme := range schemes {
		key := normalizeSchemeName(name)
		if key == "" || strings.TrimSpace(scheme.DiffusionModel) == "" {
			log.Printf("[Config][WARN] -> models.schemes.%s 缺少 diffusion_model，已忽略该方案", name)
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
		activeDiffusionScheme = ""
		return
	}
	key := normalizeSchemeName(defaultName)
	if _, ok := normalized[key]; !ok {
		key = order[0]
		log.Printf("[Config][WARN] -> models.default_scheme=%q 不存在，回退到 %s", defaultName, key)
	}
	defaultDiffusionScheme = key
	activeDiffusionScheme = key
	applySchemeLocked(normalized[key])
}

// applySchemeLocked 把方案写入全局模型路径（调用方必须持有写锁）。
//
// 扩散任务本身按任务快照取路径（见 ResolveDiffusionScheme），这里同步全局值是为了
// 让仍按全局配置读取的旧代码路径与日志显示保持一致。
func applySchemeLocked(scheme DiffusionScheme) {
	// 整组覆盖而不是「非空才覆盖」：方案是成套声明，若允许空值继承，
	// 从 A 切到未配置超分模型的 B 时会继续沿用 A 的超分模型，形成隐性串味。
	*DiffusionModel = scheme.DiffusionModel
	*VariationalModel = scheme.VariationalModel
	*PromptAnalysisModel = scheme.PromptAnalysisModel
	*PromptMmprojModel = scheme.PromptMmprojModel
	*RealESRGANModel = scheme.RealESRGANModel
}

// DefaultDiffusionSchemeName 返回配置声明的默认方案（未配置方案时为空）
func DefaultDiffusionSchemeName() string {
	diffusionSchemeMu.RLock()
	defer diffusionSchemeMu.RUnlock()
	return defaultDiffusionScheme
}

// CurrentDiffusionSchemeName 返回当前生效方案（未配置方案时为空）
func CurrentDiffusionSchemeName() string {
	diffusionSchemeMu.RLock()
	defer diffusionSchemeMu.RUnlock()
	return activeDiffusionScheme
}

// DiffusionSchemeInfos 返回方案摘要列表（供 HTTP 接口与智能体查询）
func DiffusionSchemeInfos() []DiffusionSchemeInfo {
	diffusionSchemeMu.RLock()
	defer diffusionSchemeMu.RUnlock()
	infos := make([]DiffusionSchemeInfo, 0, len(diffusionSchemeOrder))
	for _, name := range diffusionSchemeOrder {
		scheme := diffusionSchemes[name]
		infos = append(infos, DiffusionSchemeInfo{
			Name:    name,
			Label:   scheme.Label,
			Default: name == defaultDiffusionScheme,
			Current: name == activeDiffusionScheme,
			Steps:   scheme.Steps,
		})
	}
	return infos
}

// SetActiveDiffusionScheme 切换当前方案（状态只保存在 Go 层，不写回配置文件）。
func SetActiveDiffusionScheme(name string) error {
	key := normalizeSchemeName(name)
	diffusionSchemeMu.Lock()
	defer diffusionSchemeMu.Unlock()
	if len(diffusionSchemes) == 0 {
		return fmt.Errorf("配置中没有可切换的绘图模型方案（models.schemes 为空）")
	}
	scheme, ok := diffusionSchemes[key]
	if !ok {
		return fmt.Errorf("未知的绘图模型方案 %q，可用方案: %s", name, strings.Join(diffusionSchemeOrder, " / "))
	}
	activeDiffusionScheme = key
	applySchemeLocked(scheme)
	return nil
}

// NormalizeDiffusionSchemeName 把外部传入的方案名归一化为已注册键名；无法识别返回空串。
func NormalizeDiffusionSchemeName(name string) string {
	key := normalizeSchemeName(name)
	if key == "" {
		return ""
	}
	diffusionSchemeMu.RLock()
	defer diffusionSchemeMu.RUnlock()
	if _, ok := diffusionSchemes[key]; ok {
		return key
	}
	return ""
}

// ResolveDiffusionScheme 取指定方案的模型路径快照；name 为空或未知时回退当前方案。
//
// 返回的是值拷贝：生成任务在创建时取一份，之后即使前端或智能体切换方案，
// 已入队的任务仍按入队时的方案执行（避免同一批任务一半用 turbo、一半用 quality）。
func ResolveDiffusionScheme(name string) DiffusionScheme {
	diffusionSchemeMu.RLock()
	defer diffusionSchemeMu.RUnlock()
	if len(diffusionSchemes) == 0 {
		return DiffusionScheme{
			DiffusionModel:      *DiffusionModel,
			VariationalModel:    *VariationalModel,
			PromptAnalysisModel: *PromptAnalysisModel,
			PromptMmprojModel:   *PromptMmprojModel,
			RealESRGANModel:     *RealESRGANModel,
			Steps:               *SDSteps,
		}
	}
	key := normalizeSchemeName(name)
	scheme, ok := diffusionSchemes[key]
	if !ok {
		scheme, ok = diffusionSchemes[activeDiffusionScheme]
	}
	if !ok {
		scheme = diffusionSchemes[diffusionSchemeOrder[0]]
	}
	if scheme.Steps <= 0 {
		scheme.Steps = *SDSteps
	}
	return scheme
}
