package config

// 月华对话链（Go 原生实现）需要的补充配置。
//
// 这些字段原本由 TypeScript 侧直接读取 lunar_config.json 的原始 JSON；
// 迁移为 Go 原生链后统一在配置层解析并导出，避免运行期再各读一遍配置文件。

var (
	// AgentAppendSettings 追加到「月华 · 对话者」系统提示词末尾的逐行设定
	AgentAppendSettings []string
	// DiffusionPixelBudgetMP 参考图比例推导尺寸时的总像素预算上限（百万像素，默认 1.6）
	DiffusionPixelBudgetMP float64 = 1.6
	// DiffusionImg2ImgCFG 图生图提示词引导系数覆盖值（<=0 表示不覆盖，按 1.0）
	DiffusionImg2ImgCFG float64
)
