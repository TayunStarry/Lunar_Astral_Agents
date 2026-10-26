package config

import "flag"

// RegisterFlags 注册全部命令行参数。
//
// 配置值统一以普通变量保存：flag 仅负责把命令行覆盖写回变量，
// 运行期读取不再需要解引用指针。必须在 flag.Parse 之前调用。
func RegisterFlags() {
	// ==== 运行开关 ====
	flag.BoolVar(&AllowDiffusion, "allow-diffusion", AllowDiffusion, "是否允许运行扩散生成机制, 默认允许")
	flag.BoolVar(&AllowBrowser, "allow-browser", AllowBrowser, "是否允许使用浏览器, 默认允许")

	// ==== 客户端显示 ====
	flag.StringVar(&CharacterRenderer, "client-character-renderer", CharacterRenderer, "客户端角色渲染器（bedrock/live2d）")
	flag.StringVar(&Live2DModel, "client-live2d-model", Live2DModel, "Live2D 模型基址（空则用引擎层默认）")

	// ==== 引擎路径 ====
	flag.StringVar(&InferEngine, "infer-engine", InferEngine, "推理引擎路径，用于图文推理")
	flag.StringVar(&VisualEngine, "visual-engine", VisualEngine, "绘图引擎路径，用于图像生成")

	// ==== 台词链自愈 ====
	flag.BoolVar(&EngineAutoReload, "engine-auto-reload", EngineAutoReload, "月华台词链连续失败时自动重建状态")
	flag.IntVar(&EngineAutoReloadFailures, "engine-auto-reload-failures", EngineAutoReloadFailures, "触发自动重建的连续应答失败次数")

	// ==== 目录与数据库 ====
	flag.StringVar(&LocalDir, "local-dir", LocalDir, "本地目录路径，用于存储资源文件")
	flag.StringVar(&CertFile, "cert-file", CertFile, "证书文件路径, 用于HTTPS加密通信")
	flag.StringVar(&KeyFile, "key-file", KeyFile, "私钥文件路径, 用于HTTPS加密通信")
	flag.StringVar(&KnowledgeDBPath, "knowledge-db", KnowledgeDBPath, "知识库文件路径")
	flag.StringVar(&ArchiveDBPath, "archive-db", ArchiveDBPath, "统一资源目录数据库路径")
	flag.StringVar(&MediaDBPath, "media-db", MediaDBPath, "媒体派生数据库路径")
	flag.StringVar(&WebDBPath, "web-db", WebDBPath, "网络检索缓存数据库路径")
	flag.StringVar(&TextDBPath, "text-db", TextDBPath, "文本内容索引数据库路径")
	flag.StringVar(&CharacterDBPath, "character-db", CharacterDBPath, "自定义角色库数据库路径")
	flag.IntVar(&ArchiveQuotaMB, "archive-quota-mb", ArchiveQuotaMB, "统一资源内容仓库容量上限(MB)")
	flag.IntVar(&ArchiveMaxBlobMB, "archive-max-blob-mb", ArchiveMaxBlobMB, "单个资源入仓体积上限(MB)")
	flag.IntVar(&ArchiveGCIntervalMin, "archive-gc-interval-min", ArchiveGCIntervalMin, "资源仓库回收周期(分钟)")
	flag.StringVar(&MemoryDBDir, "memory-db", MemoryDBDir, "记忆库文件夹路径")

	// ==== 端口 ====
	flag.IntVar(&BasicPort, "basic-port", BasicPort, "系统Web服务的监听端口, 用户可通过此端口访问客户端界面")
	flag.IntVar(&ModelPort, "model-port", ModelPort, "模型服务的基础端口号，用于分配模型运行端口")
	flag.IntVar(&ProxyPort, "proxy-port", ProxyPort, "系统Web服务的代理监听口, 界定了系统Web服务的端口范围")

	// ==== 系统运行 ====
	flag.BoolVar(&Developer, "developer", Developer, "启用调试模式, 显示详细日志")
	flag.IntVar(&SyncFetchTimeout, "sync-fetch-timeout", SyncFetchTimeout, "内部模型请求超时时间(秒)")
	flag.BoolVar(&AllowLAN, "allow-lan", AllowLAN, "允许局域网访问引擎服务（默认仅本机回环可访问）")
	flag.BoolVar(&InsecureTLS, "insecure-tls", InsecureTLS, "插件网络请求跳过 TLS 证书校验（不推荐）")
	flag.BoolVar(&ConsoleLogEnabled, "console-log", ConsoleLogEnabled, "将控制台输出镜像到 local_data/logs/console.log（默认关闭）")

	// ==== 图像与视觉 ====
	flag.IntVar(&MaxWidth, "max-width", MaxWidth, "最大宽度")
	flag.IntVar(&MaxHeight, "max-height", MaxHeight, "最大高度")
	flag.IntVar(&JPEGQuality, "jpeg-quality", JPEGQuality, "JPEG 压缩质量 (1-100)")
	flag.StringVar(&Format, "format", Format, "图片格式 (png, jpg, jpeg)")
	flag.StringVar(&FfmpegPath, "ffmpeg-path", FfmpegPath, "ffmpeg 可执行文件路径，若为空则使用系统 PATH 中的 ffmpeg")
	flag.BoolVar(&ImageVRAMGuard, "image-vram-guard", ImageVRAMGuard, "画图前检查可用显存, 低于阈值时先卸载月华模型释放显存")
	flag.IntVar(&ImageVRAMGuardMiB, "image-vram-guard-mib", ImageVRAMGuardMiB, "画图前可用显存阈值(MiB)")
	flag.StringVar(&SDOffloadToCPU, "sd-offload-to-cpu", SDOffloadToCPU, "sd.cpp 权重驻留内存按需载入显存: auto/always/off")
	flag.BoolVar(&SDTextEncoderOnCPU, "sd-te-on-cpu", SDTextEncoderOnCPU, "sd.cpp 提示词编码器在 CPU 运行(不占显存)")
	flag.StringVar(&SDVaeTiling, "sd-vae-tiling", SDVaeTiling, "VAE 分片解码: off/always/auto")
	flag.StringVar(&SDVaeTileSize, "sd-vae-tile-size", SDVaeTileSize, "VAE 分片尺寸(如 64x64), 空=引擎默认 32x32")
	flag.Float64Var(&SDVaeTileOverlap, "sd-vae-tile-overlap", SDVaeTileOverlap, "VAE 分片重叠比例(0=引擎默认 0.5)")
	flag.StringVar(&SDVaeBackend, "sd-vae-backend", SDVaeBackend, "VAE 解码后端: auto/gpu/cpu")
	flag.BoolVar(&SDVaeConvDirect, "sd-vae-conv-direct", SDVaeConvDirect, "GPU 整帧放不下时改用直接卷积(省显存)")
	flag.StringVar(&SDMaxVRAM, "sd-max-vram", SDMaxVRAM, "sd.cpp 显存预算(GiB); 空=自动")
	flag.StringVar(&SDSampler, "sd-sampler", SDSampler, "采样器: auto/euler/euler_a/...")
	flag.StringVar(&SDScheduler, "sd-scheduler", SDScheduler, "调度器: auto/discrete/karras/simple/flux/...")
	flag.Float64Var(&SDFlowShift, "sd-flow-shift", SDFlowShift, "Flow 模型 shift(0=auto)")
	flag.BoolVar(&SDFlashAttention, "sd-diffusion-fa", SDFlashAttention, "扩散模型使用 flash attention")
	flag.IntVar(&SDSteps, "sd-steps", SDSteps, "默认采样步数(调用方未指定时)")
	flag.IntVar(&SDImageSize, "sd-image-size", SDImageSize, "默认图片边长(调用方未指定时)")
	flag.StringVar(&VideoInputMode, "video-input", VideoInputMode, "视频解读输入模式: file(file://) / frames(图片序列帧)")
	flag.StringVar(&ImageInputMode, "image-input", ImageInputMode, "静态图思维链形态: url/file-read 端点 / base64")
	flag.IntVar(&VisionMaxSide, "vision-max-side", VisionMaxSide, "视觉读取副本最大边长(像素)")
	flag.IntVar(&VisionCacheCapMB, "vision-cache-cap-mb", VisionCacheCapMB, "视觉副本缓存池容量上限(MB), 0=不限制")

	// ==== 模型路由 ====
	flag.StringVar(&AgentEmbeddingModel, "agent-embedding-model", AgentEmbeddingModel, "核心智能体嵌入模型名称")
	flag.StringVar(&AgentEmbeddingURL, "agent-embedding-url", AgentEmbeddingURL, "核心智能体嵌入服务 API 地址")
	flag.StringVar(&AgentEmbeddingKey, "agent-embedding-key", AgentEmbeddingKey, "核心智能体嵌入服务 API 密钥")
	flag.StringVar(&AgentMultimodalModel, "agent-multimodal-model", AgentMultimodalModel, "核心智能体多模态模型名称")
	flag.StringVar(&AgentMultimodalURL, "agent-multimodal-url", AgentMultimodalURL, "核心智能体多模态服务 API 地址")
	flag.StringVar(&AgentMultimodalKey, "agent-multimodal-key", AgentMultimodalKey, "核心智能体多模态服务 API 密钥")
	flag.StringVar(&AgentASRModel, "agent-asr-model", AgentASRModel, "核心智能体语音识别模型名称")
	flag.StringVar(&MmprojModel, "mmproj-model", MmprojModel, "多模态投影模型路径")
	flag.StringVar(&AsrModel, "asr-model", AsrModel, "ASR模型路径")
	flag.StringVar(&DiffusionModel, "diffusion-model", DiffusionModel, "扩散模型路径")
	flag.StringVar(&VariationalModel, "variational-model", VariationalModel, "VAE模型路径")
	flag.StringVar(&PromptAnalysisModel, "prompt-analysis-model", PromptAnalysisModel, "提示分析模型路径")
	flag.StringVar(&PromptMmprojModel, "prompt-mmproj-model", PromptMmprojModel, "多模态投影模型路径")
	flag.StringVar(&RealESRGANModel, "real-esrgan-model", RealESRGANModel, "RealESRGAN模型路径")

	// ==== TTS ====
	flag.StringVar(&TTSEngine, "tts-engine", TTSEngine, "月华语音合成引擎（Qwen/Kokoro/OmniVoice）")
	flag.StringVar(&TTSRefAudio, "tts-ref-audio", TTSRefAudio, "TTS 参考音频路径（Qwen/OmniVoice）")
	flag.StringVar(&TTSRefText, "tts-ref-text", TTSRefText, "TTS 参考音频转写文本（OmniVoice）")
	flag.StringVar(&TTSVoice, "tts-voice", TTSVoice, "TTS 音色名称（Kokoro）")
	flag.Float64Var(&TTSSpeed, "tts-speed", TTSSpeed, "TTS 语速倍率（Kokoro）")
	flag.StringVar(&TTSLang, "tts-lang", TTSLang, "TTS 语言提示")
	flag.StringVar(&TTSInstruct, "tts-instruct", TTSInstruct, "TTS 音色属性关键词（OmniVoice）")
	flag.Uint64Var(&TTSSeed, "tts-seed", TTSSeed, "TTS 随机种子（OmniVoice）")
	flag.BoolVar(&TTSDisablePostproc, "tts-disable-postproc", TTSDisablePostproc, "关闭 TTS 输出后处理（OmniVoice）")
	flag.IntVar(&TTSIdleUnloadMinutes, "tts-idle-unload-minutes", TTSIdleUnloadMinutes, "TTS 引擎闲置卸载阈值（分钟）")

	// ==== WebView ====
	flag.StringVar(&WebViewTitle, "webview-title", WebViewTitle, "webview 窗口标题")
	flag.IntVar(&WebViewWidth, "webview-width", WebViewWidth, "webview 窗口宽度")
	flag.IntVar(&WebViewHeight, "webview-height", WebViewHeight, "webview 窗口高度")
	flag.IntVar(&WebViewMinWidth, "webview-min-width", WebViewMinWidth, "webview 窗口最小宽度")
	flag.IntVar(&WebViewMinHeight, "webview-min-height", WebViewMinHeight, "webview 窗口最小高度")
	flag.BoolVar(&WebViewResizable, "webview-resizable", WebViewResizable, "webview 窗口是否可调整大小")
	flag.StringVar(&WebViewIconPath, "webview-icon", WebViewIconPath, "webview 窗口图标 .ico 文件路径（空则使用默认 icon.ico）")
	flag.StringVar(&WebViewCaptionColor, "webview-caption-color", WebViewCaptionColor, "webview 标题栏背景色")
	flag.StringVar(&WebViewBorderColor, "webview-border-color", WebViewBorderColor, "webview 窗口边框色")
	flag.BoolVar(&WebViewDarkTitleBar, "webview-dark-titlebar", WebViewDarkTitleBar, "webview 标题栏使用深色模式")
}
