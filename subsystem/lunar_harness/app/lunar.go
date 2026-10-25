package app

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"LunarSubsystem/LunarHarness/clients"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/model/llama"
	image "LunarSubsystem/LunarHarness/engine/multimodal/httpapi"
	"LunarSubsystem/LunarHarness/lunar"
	"LunarSubsystem/LunarHarness/transport/napcat"
)

// assembleLunar 装配月华人格局的生产者部分：模型代理、绘图协处理器与 QQ 桥接。
// TTS 引擎不再于装配期 eager 初始化：三引擎均由统一播报链路 engine/broadcaster
// 单载槽位管理（首次合成请求时加载，5 分钟无调用自动卸载）。
// JavaScript 智能体运行时（思考循环）属消费层，由 startLunarAgent 在装配末段启动。
func assembleLunar() error {
	// ==== MIME 类型映射 ====
	for ext, mimeType := range config.MimeMap {
		mime.AddExtensionType(ext, mimeType)
	}

	// ==== 本地数据目录 ====
	if err := os.MkdirAll(config.LocalDir, 0755); err != nil {
		return err
	}

	// ==== llama.cpp 本地模型代理 ====
	llama.Init()

	// 模型服务保活：重建对话链状态前先确认模型服务还活着。
	// 实测事故：llama.cpp 内部断言失败会让进程直接退出，此后所有请求都是
	// "Could not establish connection"；此时只重建链状态毫无意义，必须把模型服务一并拉起来。
	lunar.SetModelEnsureHook(func() error {
		if llama.Alive() {
			return nil
		}
		logger.Warn("LunarHarness", "模型服务不可达，重载引擎前先重启 llama-server")
		return llama.Restart()
	})

	// ==== 扩散生成任务协处理器 ====
	image.StartTaskProcessor()

	// ==== QQ 桥接适配器 ====
	initNapcatBridge()

	return nil
}

// startLunarAgent 启动月华的 Go 原生智能体（每秒思考循环）。
// 该循环一经启动即可能消费计划表等到期任务，故必须排在全部服务层与琉璃消费层之后启动。
func startLunarAgent() error {
	return lunar.Start()
}

// shutdownLunar 关闭月华人格局持有的资源
func shutdownLunar() {
	lunar.Stop()
	napcat.StopBridge()
	llama.Close()
}

// lunarStaticRoot 月华消息终端的静态资源根（clients/lunar 子树，mobile/ 即月华移动端）
func lunarStaticRoot() http.FileSystem {
	return clients.LunarFS()
}

// initNapcatBridge 初始化 QQ 桥接适配器并注册消息回调
func initNapcatBridge() {
	exePath, err := os.Executable()
	if err != nil {
		logger.Error("LunarHarness", "获取可执行文件路径失败: %v", err)
		return
	}
	exeDir := filepath.Dir(exePath)
	// QQ 桥接配置已迁到专用 napcat_config.json（新默认）；专用文件不存在时
	// 回退旧版 lunar_config.json 的 bridging 组，保证升级用户的桥接不失效
	configPath := filepath.Join(exeDir, config.LocalDir, "napcat_config.json")
	if _, err := os.Stat(configPath); err != nil {
		configPath = filepath.Join(exeDir, config.LocalDir, "lunar_config.json")
	}

	if err := napcat.LoadBridgingConfig(configPath); err != nil {
		logger.Error("LunarHarness", "加载桥接配置失败: %v", err)
		return
	}

	// QQ 消息（OpenAI 格式）按到达顺序推送到智能体统一未读队列
	napcat.SendMessageToAgent = func(messages []map[string]any) {
		for _, msg := range messages {
			lunar.EnqueueMessage(lunar.PostMessage{
				Role:    "user",
				Content: msg["content"],
				Source:  "qq",
			})
		}
	}

	// 语音/音频地址包装为媒体URL内容项，按到达顺序写入统一未读队列
	napcat.SendAudioToAgent = func(urls []string) {
		for _, url := range urls {
			lunar.EnqueueAudioURL(url, "qq")
		}
	}

	// 启动桥接连接（内部阻塞读取 WebSocket 消息，必须放到独立 goroutine）
	go napcat.StartBridgeScanner()
}
