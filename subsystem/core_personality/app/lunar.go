package app

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"CorePersonality/personality/lunar/bridging/napcat"
	"CorePersonality/personality/lunar/engine"
	"CorePersonality/personality/lunar/engine/container"
	"CorePersonality/personality/lunar/hierarchy"
	"CorePersonality/personality/lunar/model/llama"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	image "LunarSubsystem/MultimodalAnalysis/server"
)

// assembleLunar 装配月华人格局的生产者部分：模型代理、绘图协处理器与 QQ 桥接。
// TTS 引擎不再于装配期 eager 初始化：三引擎均由 engine/tts_router 惰性加载
// （首次合成请求时加载，5 分钟无调用自动卸载）。
// JavaScript 智能体运行时（思考循环）属消费层，由 startLunarAgent 在装配末段启动。
func assembleLunar() error {
	// ==== MIME 类型映射 ====
	for ext, mimeType := range GeneralConfig.MimeMap {
		mime.AddExtensionType(ext, mimeType)
	}

	// ==== 本地数据目录 ====
	if err := os.MkdirAll(*GeneralConfig.LocalDir, 0755); err != nil {
		return err
	}

	// ==== llama.cpp 本地模型代理 ====
	llama.Init()

	// ==== 扩散生成任务协处理器 ====
	image.StartTaskProcessor()

	// ==== QQ 桥接适配器 ====
	initNapcatBridge()

	return nil
}

// startLunarAgent 启动月华的 JavaScript 智能体运行时（goja + agentSystem.js）。
// 该运行时一经启动即进入每秒思考循环，可能立即消费计划表等到期任务，故必须排在
// 全部服务层（HTTP / llama 代理 / 记忆库）与琉璃消费层（LTP9 / LTPX broker）之后启动；
// 否则到期任务会在依赖尚未就绪时执行，导致应答链报错或事件循环长时间阻塞。
func startLunarAgent() error {
	return engine.RunAgentContext()
}

// shutdownLunar 关闭月华人格局持有的资源
func shutdownLunar() {
	container.Close()
	napcat.StopBridge()
	llama.Close()
}

// lunarStaticRoot 月华消息终端的静态资源根（assets/client 子树）
func lunarStaticRoot() http.FileSystem {
	return hierarchy.Gethierarchy()
}

// initNapcatBridge 初始化 QQ 桥接适配器并注册消息回调
func initNapcatBridge() {
	exePath, err := os.Executable()
	if err != nil {
		LoggerGeneral.Error("CorePersonality", "获取可执行文件路径失败: %v", err)
		return
	}
	exeDir := filepath.Dir(exePath)
	configPath := filepath.Join(exeDir, *GeneralConfig.LocalDir, "lunar_config.json")

	if err := napcat.LoadBridgingConfig(configPath); err != nil {
		LoggerGeneral.Error("CorePersonality", "加载桥接配置失败: %v", err)
		return
	}

	// QQ 消息（OpenAI 格式）按到达顺序推送到智能体统一未读队列
	napcat.SendMessageToAgent = func(messages []map[string]any) {
		for _, msg := range messages {
			engine.EnqueueMessage(engine.PostMessage{
				Role:    "user",
				Content: msg["content"],
			})
		}
	}

	// 视频/音频地址包装为媒体URL内容项，按到达顺序写入统一未读队列
	napcat.SendVideoToAgent = func(urls []string) {
		for _, url := range urls {
			engine.EnqueueMediaURL("video_url", url)
		}
	}
	napcat.SendAudioToAgent = func(urls []string) {
		for _, url := range urls {
			engine.EnqueueMediaURL("audio_url", url)
		}
	}

	// 启动桥接连接（内部阻塞读取 WebSocket 消息，必须放到独立 goroutine）
	go napcat.StartBridgeScanner()
}
