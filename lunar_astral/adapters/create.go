package adapters

import (
	"LunarAstral/hierarchy"
	"LunarAstral/learner"
	"LunarSubsystem/LoggerGeneral"
	"LunarSubsystem/LunarGoja"
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

	// 注册知识库操作适配器（JSON 文件存储）
	vm.Set("knowledgeLoad", adapters.knowledgeLoad)
	vm.Set("knowledgeSave", adapters.knowledgeSave)

	// 注册网络操作适配器
	vm.Set("url", adapters.getEnvironmentUrl)
	vm.Set("address", adapters.address)
	vm.Set("syncFetch", adapters.syncFetch)

	// 注册图像处理适配器
	vm.Set("keyframe", adapters.keyframe)
	vm.Set("resizeImage", adapters.resizeImage)
	vm.Set("generateImage", adapters.generateImage)

	// 注册base64编码解码适配器
	vm.Set("atob", adapters.atob)

	// 注册消息操作适配器
	vm.Set("pullVideoUrl", adapters.pullVideoUrl)
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
	vm.Set("clearLTPXRemoteTools", adapters.clearLTPXRemoteToolsForJS)

	// 注册智能体控制适配器
	vm.Set("getAgentPosition", adapters.getAgentPosition)
	vm.Set("pushAgentEvent", adapters.pushAgentEvent)

	// 注册引擎桥接适配器
	vm.Set("sendToEngine", adapters.sendToEngine)
	vm.Set("getAvailableActions", adapters.getAvailableActions)

	// 注册学习者智能体适配器
	learner.BindLearnerToRuntime(vm)
}

// RunAgentContext 加载并运行嵌入式文件系统中的JavaScript文件
func RunAgentContext() error {
	runtimeMutex.Lock()
	// 如果运行时不存在，先创建并初始化
	if !LunarGoja.IsReady() {
		runtimeMutex.Unlock()
		if err := LunarGoja.Init(registerAdaptersToRuntime); err != nil {
			return fmt.Errorf("Lunar模块[JavaScript][ERROR] -> 创建运行时环境失败: %v", err)
		}
	} else {
		runtimeMutex.Unlock()
	}

	// 从嵌入式文件系统中读取agentSystem.js文件
	systemJS, err := hierarchy.EmbeddedFiles.ReadFile("assets/agentSystem.js")
	if err != nil {
		return fmt.Errorf("Lunar模块[JavaScript][ERROR] -> 读取 agentSystem.js 失败: %v", err)
	}

	systemJSContent := string(systemJS)
	// 在事件循环中执行JavaScript代码
	LunarGoja.RunOnLoop(func(vm *goja.Runtime) {
		_, err = vm.RunString(systemJSContent)
		if err != nil {
			LoggerGeneral.SubError("LunarCore", "JavaScript", "执行 agentSystem.js 代码失败: %v", err)
			return
		}
	})
	return nil
}

// RunOnAgentLoop 在 agent 事件循环中执行函数（供外部模块调用）
func RunOnAgentLoop(fn func(vm *goja.Runtime)) {
	if !LunarGoja.IsReady() {
		LoggerGeneral.Error("LunarCore", "JavaScript 运行时未初始化，无法执行操作")
		return
	}
	LunarGoja.RunOnLoop(fn)
}
