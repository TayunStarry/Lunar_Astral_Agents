package GeneralConfig

import "flag"

// ==== 月华客户端显示配置 ====
//
// 控制月华客户端（桌面端 / 移动端）角色的渲染方式：基岩版模型引擎（bedrock，默认）
// 或 Live2D 引擎层（live2d）。该配置由前端直接读取（fetch /file/read/lunar_config.json），
// 此处声明 flag 以集中呈现配置契约，默认值保持既有行为（bedrock）。

var (
	// CharacterRenderer 客户端角色渲染器枚举（bedrock / live2d）
	CharacterRenderer = flag.String("client-character-renderer", "bedrock", "客户端角色渲染器（bedrock/live2d）")
	// Live2DModel Live2D 模型基址（空则用引擎层默认 /file/read/models/live2d/）
	Live2DModel = flag.String("client-live2d-model", "", "Live2D 模型基址（空则用引擎层默认）")
)
