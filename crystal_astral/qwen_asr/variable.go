package qwen_asr

import "sync"

// ==== 引擎懒加载 ====

// asrInitOnce 引擎懒加载一次性保护：模型只在首个请求到达时加载
var asrInitOnce sync.Once

// asrEngine 引擎单例（由 asrInitOnce 保证只初始化一次）
var asrEngine *QwenASR

// asrInitErr 引擎初始化错误（供后续请求快速返回）
var asrInitErr error

// ==== 常量 ====

// maxUploadMemory 音频表单解析内存上限（32MB）
const maxUploadMemory = 32 << 20

// defaultBf16CacheMB BF16 权重缓存大小（MB），与 subsystem/qwen_asr 保持一致
const defaultBf16CacheMB = "1024"
