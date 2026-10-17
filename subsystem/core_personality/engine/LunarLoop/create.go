package lunarloop

import (
	"CorePersonality/engine/LunarLoop/container"
	"LunarSubsystem/LoggerGeneral"
	"fmt"

	"github.com/dop251/goja"
)

// registerAdaptersToRuntime 注册自定义适配器函数到通用 goja 运行时环境
// 标准环境（console/process/require、fetch/getNetworkInterfaces/WebSocket）已由 lunar_goja 内置
func registerAdaptersToRuntime(vm *goja.Runtime) {
	// 创建Runtime实例，用于存储JavaScript运行时实例
	adapters := &Runtime{runtime: vm}

	// 注册文件操作适配器
	vm.Set("saveFile", adapters.saveFile)
	vm.Set("readFile", adapters.readFile)
	vm.Set("fileView", adapters.fileView)
	vm.Set("fileList", adapters.fileList)
	vm.Set("saveDebugFile", adapters.saveDebugFile)

	// 注册知识库操作适配器（SQLite 存储，经共享子系统 knowledge.db 通道）
	vm.Set("knowledgeLoad", adapters.knowledgeLoad)
	vm.Set("knowledgeSave", adapters.knowledgeSave)

	// 注册网络操作适配器
	vm.Set("url", adapters.getEnvironmentUrl)
	vm.Set("address", adapters.address)
	vm.Set("syncFetch", adapters.syncFetch)

	// 注册图像处理适配器
	vm.Set("videoMedia", adapters.videoMedia)
	vm.Set("videoFrames", adapters.videoFrames)
	vm.Set("audioWav", adapters.audioWav)
	vm.Set("resizeImage", adapters.resizeImage)
	vm.Set("isAnimatedImage", adapters.isAnimatedImage)
	vm.Set("animatedImageToVideo", adapters.animatedImageToVideo)
	vm.Set("animatedImageToFrames", adapters.animatedImageToFrames)
	vm.Set("generateImage", adapters.generateImage)
	vm.Set("upscaleImage", adapters.upscaleImage)
	vm.Set("visionRead", adapters.visionRead)

	// 注册统一资源归档层适配器（句柄 / 资源目录 / 内容仓库 / 回收，见 docs/adr/0001）
	vm.Set("resourceAdmit", adapters.resourceAdmit)
	vm.Set("resourceRead", adapters.resourceRead)
	vm.Set("resourceOutline", adapters.resourceOutline)
	vm.Set("resourceList", adapters.resourceList)
	vm.Set("resourceSearch", adapters.resourceSearch)
	vm.Set("resourceStats", adapters.resourceStats)
	vm.Set("resourceSetSummary", adapters.resourceSetSummary)
	vm.Set("resourceSetSections", adapters.resourceSetSections)
	vm.Set("resourceCollect", adapters.resourceCollect)
	vm.Set("resourceDelete", adapters.resourceDelete)
	vm.Set("resourceGrammar", adapters.resourceGrammar)

	// 注册base64编码解码适配器
	vm.Set("atob", adapters.atob)
	// 注册内容哈希适配器（SHA-256 前16位十六进制，与前端文件哈希命名一致）
	vm.Set("hashBytes", adapters.hashBytes)

	// 注册消息操作适配器（pullContext 为统一时序队列：文本与视频/音频URL混排）
	vm.Set("pullContext", adapters.pullContext)
	vm.Set("pushContext", adapters.pushContext)
	vm.Set("pushImage", adapters.pushImage)

	// 注册记忆库适配器
	vm.Set("memoryInit", adapters.memoryInit)
	vm.Set("memoryAdd", adapters.memoryAdd)
	vm.Set("memoryAddWithTags", adapters.memoryAddWithTags)
	vm.Set("memoryQuery", adapters.memoryQuery)
	vm.Set("memoryDelete", adapters.memoryDelete)
	vm.Set("memoryAddImage", adapters.memoryAddImage)

	// 注册TTS语音合成适配器
	vm.Set("tts", adapters.textToSpeech)

	// 注册截图子系统适配器
	vm.Set("screenshotCapture", adapters.screenshotCapture)
	vm.Set("screenshotGetDisplays", adapters.screenshotGetDisplays)

	// 注册 LTPX 远程（琉璃）工具链协调函数
	vm.Set("getLTPXRemoteStatus", adapters.getLTPXRemoteStatusForJS)
	vm.Set("callLTPXRemoteTool", adapters.callLTPXRemoteToolForJS)
	// 注册 LTPX 事件交互：月华在每个事件触发点同步推送负载到琉璃，接收插件处理结果
	vm.Set("ltpInteractEvent", adapters.interactLTPXEventForJS)

	// 注册智能体控制适配器
	vm.Set("getAgentPosition", adapters.getAgentPosition)

	// 注册数据导出入口：宿主经 dumpForJS 向运行时索要智能体上下文 JSON
	// （与 agent/data-dump.ts 的 buildContextDump 配对）
	vm.Set("dumpForJS", adapters.dumpForJS)

	// 注册引擎桥接适配器
	vm.Set("sendToEngine", adapters.sendToEngine)
	vm.Set("getAvailableActions", adapters.getAvailableActions)
}

// RunAgentContext 加载并运行嵌入式文件系统中的JavaScript文件
func RunAgentContext() error {
	runtimeMutex.Lock()
	// 如果运行时不存在，先创建并初始化
	if !container.IsReady() {
		runtimeMutex.Unlock()
		if err := container.Init(registerAdaptersToRuntime); err != nil {
			return fmt.Errorf("Lunar模块[JavaScript][ERROR] -> 创建运行时环境失败: %v", err)
		}
	} else {
		runtimeMutex.Unlock()
	}

	// 智能体脚本由本包内嵌（go:embed agentSystem.js，见 variable.go），随二进制发布、版本一致
	if len(agentSystemJS) == 0 {
		return fmt.Errorf("Lunar模块[JavaScript][ERROR] -> 内嵌的 agentSystem.js 为空")
	}
	systemJSContent := string(agentSystemJS)

	// 在事件循环中执行JavaScript代码
	container.RunOnLoop(func(vm *goja.Runtime) {
		if _, runErr := vm.RunString(systemJSContent); runErr != nil {
			LoggerGeneral.SubError("LunarCore", "JavaScript", "执行 agentSystem.js 代码失败: %v", runErr)
			return
		}
		// 脚本执行成功：标记智能体就绪（数据导出等宿主侧入口据此判断可用性）
		agentReady.Store(true)
	})
	return nil
}

// IsAgentReady 报告月华智能体脚本是否已成功加载（供宿主侧入口做可用性判断）
func IsAgentReady() bool {
	return agentReady.Load()
}

// RunOnAgentLoop 在 agent 事件循环中执行函数（供外部模块调用）
func RunOnAgentLoop(fn func(vm *goja.Runtime)) {
	if !container.IsReady() {
		LoggerGeneral.Error("LunarCore", "JavaScript 运行时未初始化，无法执行操作")
		return
	}
	container.RunOnLoop(fn)
}
