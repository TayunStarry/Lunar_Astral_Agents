package server

// GenerateRequest 图像生成请求结构体，用于接收客户端提交的扩散生成任务参数。
type GenerateRequest struct {
	Prompt               string  `json:"prompt"`                 // 正向提示词，必填，描述期望生成的图像内容
	NegativePrompt       string  `json:"negative_prompt"`        // 负向提示词，可选，描述需要排除的内容特征
	BatchSize            int     `json:"batch_size"`             // 批量生成数量，默认为 1
	Width                int     `json:"width"`                  // 图像宽度（像素），需为 64 的倍数
	Height               int     `json:"height"`                 // 图像高度（像素），需为 64 的倍数
	Strength             float64 `json:"strength"`               // 去噪强度（0.0~1.0），用于 img2img，控制初始图像保留程度
	Steps                int     `json:"steps"`                  // 采样步数，影响生成质量与耗时
	Seed                 int64   `json:"seed"`                   // 随机种子，负数表示使用随机种子
	CfgScale             float64 `json:"cfg_scale"`              // CFG 引导系数，控制对提示词的遵循程度
	InitImg              string  `json:"init_img"`               // Base64 编码的初始图像（img2img），空则表示文生图
	AllowSuperResolution bool    `json:"allow_super_resolution"` // 是否启用超分
}
