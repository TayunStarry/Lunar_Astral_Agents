package OmniVoiceTTS

// ==== HTTP 请求/响应 ====

// TTSRequest 语音合成请求
type TTSRequest struct {
	// Text 要合成的文本（UTF-8）
	Text string `json:"text"`
	// Lang 语言提示：空/"auto" 自动，或 ISO id（"zh"）或语言名（"chinese"）
	Lang string `json:"lang,omitempty"`
	// Instruct 属性关键词设计音色（如 "female young adult moderate"），与 RefAudio 互斥使用
	Instruct string `json:"instruct,omitempty"`
	// RefAudio 参考音频路径（24kHz 单声道推荐，其他采样率自动线性重采样），用于声音克隆
	RefAudio string `json:"ref_audio,omitempty"`
	// RefText 参考音频的转写文本（声音克隆时必填）
	RefText string `json:"ref_text,omitempty"`
	// Seed 随机种子（0 使用库默认 42；固定种子可复现输出）
	Seed uint64 `json:"seed,omitempty"`
	// DisablePostproc 关闭输出后处理（静音裁剪/峰值归一/边缘淡出），用于固定时长的配音场景
	DisablePostproc bool `json:"disable_postproc,omitempty"`
}

// TTSResponse 语音合成响应
type TTSResponse struct {
	// Success 是否成功
	Success bool `json:"success"`
	// Audio base64 编码的 WAV 音频
	Audio string `json:"audio,omitempty"`
	// Error 错误信息
	Error string `json:"error,omitempty"`
	// Voice 实际使用的音色（回显）
	Voice string `json:"voice,omitempty"`
	// SampleRate 输出采样率（24000）
	SampleRate int `json:"sample_rate,omitempty"`
}

// LanguageEntry 支持的语言条目
type LanguageEntry struct {
	// Id 语言 id（传给 TTSRequest.Lang）
	Id string `json:"id"`
	// Name 语言显示名
	Name string `json:"name"`
}

// HealthResponse 健康检查响应
type HealthResponse struct {
	// Status 服务状态
	Status string `json:"status"`
	// Service 服务名
	Service string `json:"service"`
	// Loaded 引擎是否已加载
	Loaded bool `json:"loaded"`
	// Version 引擎构建版本（git hash + 日期）
	Version string `json:"version,omitempty"`
	// Languages 支持的语言数
	Languages int `json:"languages"`
}
