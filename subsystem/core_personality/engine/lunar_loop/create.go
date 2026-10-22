package lunarloop

import (
	"CorePersonality/agents/BilibiliLife"
	"CorePersonality/engine/lunar_loop/container"
	"LunarSubsystem/LoggerGeneral"
	"fmt"
	"time"

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
	vm.Set("readTextFile", adapters.readTextFile)
	vm.Set("fileView", adapters.fileView)
	vm.Set("fileList", adapters.fileList)
	vm.Set("saveDebugFile", adapters.saveDebugFile)

	// 注册知识条目操作适配器（SQLite 存储，经共享子系统并按逻辑表路由到对应领域库）
	vm.Set("knowledgeLoad", adapters.knowledgeLoad)
	vm.Set("knowledgeSave", adapters.knowledgeSave)

	// 注册网络操作适配器
	vm.Set("url", adapters.getEnvironmentUrl)
	vm.Set("address", adapters.address)
	vm.Set("syncFetch", adapters.syncFetch)

	// 注册图像处理适配器
	//
	// 这里**没有** resizeImage：思维链不允许自己缩放图片。图片进入模型上下文的唯一
	// 形态是视觉读取管线产物（/file/miniature/ 端点或 visionRead），缩放与尺寸
	// 约束在 Go 侧统一完成。任何"模型自己把原图缩一缩"的路径都会被这条缺失的导出
	// 直接掐断（见 docs/adr/0008）。
	vm.Set("videoMedia", adapters.videoMedia)
	vm.Set("videoFrames", adapters.videoFrames)
	vm.Set("audioWav", adapters.audioWav)
	vm.Set("isAnimatedImage", adapters.isAnimatedImage)
	vm.Set("animatedImageToFrames", adapters.animatedImageToFrames)
	vm.Set("generateImage", adapters.generateImage)
	vm.Set("upscaleImage", adapters.upscaleImage)
	// 绘图模型方案：状态记录在 Go 层，与图像工坊前端 /generate/scheme 共享
	vm.Set("getDiffusionScheme", adapters.getDiffusionScheme)
	vm.Set("setDiffusionScheme", adapters.setDiffusionScheme)
	vm.Set("visionRead", adapters.visionRead)

	// 注册统一资源归档层适配器（句柄 / 资源目录 / 内容仓库 / 回收，见 docs/adr/0001）
	vm.Set("resourceAdmit", adapters.resourceAdmit)
	vm.Set("resourceAdmitDocument", adapters.resourceAdmitDocument)
	vm.Set("resourceRead", adapters.resourceRead)
	vm.Set("resourceOutline", adapters.resourceOutline)
	vm.Set("resourceList", adapters.resourceList)
	vm.Set("resourceSearch", adapters.resourceSearch)
	vm.Set("resourceStats", adapters.resourceStats)
	vm.Set("resourceSetSummary", adapters.resourceSetSummary)
	vm.Set("resourceSetSections", adapters.resourceSetSections)
	vm.Set("resourceDelete", adapters.resourceDelete)
	vm.Set("resourceGrammar", adapters.resourceGrammar)

	// 注册描述者适配器（句柄系统的图片/动图/视频描述与文件内容探查，见 agents/Descriptor）
	vm.Set("descriptorDescribeMedia", adapters.descriptorDescribeMedia)
	vm.Set("descriptorInspect", adapters.descriptorInspect)

	// 注册base64编码解码适配器
	vm.Set("atob", adapters.atob)
	// 二进制 base64 解码：返回字节数组（atob 返回字符串，不能用于图片/音频）
	vm.Set("base64Decode", adapters.base64Decode)
	// 注册内容哈希适配器（SHA-256 前16位十六进制，与前端文件哈希命名一致）
	vm.Set("hashBytes", adapters.hashBytes)

	// 注册消息操作适配器（pullContext 为统一时序队列：文本与视频/音频URL混排）
	vm.Set("pullContext", adapters.pullContext)
	// 未读条数（只读，不消费）：生活链进入闸门用
	vm.Set("unreadCount", adapters.unreadCount)
	vm.Set("pushContext", adapters.pushContext)
	vm.Set("pushImage", adapters.pushImage)

	// 注册记忆库适配器
	vm.Set("memoryInit", adapters.memoryInit)
	vm.Set("memoryListCollections", adapters.memoryListCollections)
	vm.Set("memoryAdd", adapters.memoryAdd)
	vm.Set("memoryAddWithTags", adapters.memoryAddWithTags)
	vm.Set("memoryQuery", adapters.memoryQuery)
	vm.Set("memoryKeywordSearch", adapters.memoryKeywordSearch)
	vm.Set("memoryGet", adapters.memoryGet)
	vm.Set("memoryList", adapters.memoryList)
	vm.Set("memoryDelete", adapters.memoryDelete)
	vm.Set("memoryAddImage", adapters.memoryAddImage)
	vm.Set("memoryAddAudio", adapters.memoryAddAudio)

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

	// 注册引擎自愈入口：JS 上报应答成败、取回重载前的上下文快照
	// （与 agent/loop.ts、agent/recovery.ts 配对；见 reload.go）
	vm.Set("notifyAgentIdle", adapters.notifyAgentIdle)
	vm.Set("notifyAgentFailure", adapters.notifyAgentFailure)
	vm.Set("takeEngineSnapshot", adapters.takeEngineSnapshot)

	// 生活链桥：进度标题 / 空闲自动判定 / 对话完成通知
	vm.Set("bilibiliLifeProgress", func(goja.FunctionCall) goja.Value {
		return vm.ToValue(BilibiliLife.Progress())
	})
	vm.Set("bilibiliLifeAutoTick", func(goja.FunctionCall) goja.Value {
		return vm.ToValue(BilibiliLife.AutoTick())
	})
	vm.Set("bilibiliLifeDialogueIdle", func(goja.FunctionCall) goja.Value {
		BilibiliLife.NotifyDialogueIdle()
		return goja.Undefined()
	})
	vm.Set("bilibiliLifeSetDialogueBusy", func(call goja.FunctionCall) goja.Value {
		BilibiliLife.SetDialogueBusy(call.Argument(0).ToBoolean())
		return goja.Undefined()
	})
	BilibiliLife.SetUnreadCounter(UnreadCount)
	// 生活日记回执：每轮生活结束把「日记写在了哪个句柄」送进未读队列，
	// 月华由此知道可用 read_resource 按行回看自己刷了什么
	BilibiliLife.SetDiaryNotifier(func(message string) {
		EnqueueMessage(PostMessage{Role: string(RoleUser), Content: "[月华的生活] " + message, Source: "bilibili_life"})
	})

	// 注册引擎桥接适配器
	vm.Set("sendToEngine", adapters.sendToEngine)
	vm.Set("getAvailableActions", adapters.getAvailableActions)
	// 动作词表进程内匹配（animation_list 缓存 + 嵌入索引；不写记忆库）
	vm.Set("matchAction", adapters.matchAction)
}

// ensureRuntime 确保 goja 运行时存在（幂等；重载后重新创建）
func ensureRuntime() error {
	runtimeMutex.Lock()
	defer runtimeMutex.Unlock()
	if container.IsReady() {
		return nil
	}
	if err := container.Init(registerAdaptersToRuntime); err != nil {
		return fmt.Errorf("创建运行时环境失败: %w", err)
	}
	return nil
}

// runAgentScript 在当前运行时里执行内嵌的 agentSystem.js，并标记智能体就绪
//
// 每次重载都会重新跑一遍：整块 JS 状态（角色实例、setInterval 思考循环）随之重建。
func runAgentScript() error {
	// 智能体脚本由本包内嵌（go:embed agentSystem.js，见 variable.go），随二进制发布、版本一致
	if len(agentSystemJS) == 0 {
		return fmt.Errorf("内嵌的 agentSystem.js 为空（检查 npm run server.side 产物）")
	}
	systemJSContent := string(agentSystemJS)

	var runErr error
	// 自愈入口自检结果：宿主重载时靠 agentSystem.snapshotContext / restoreContext 抢救上下文
	var rescueReady bool
	// 等闭包真的跑完：eventloop 的 RunOnLoop 是**异步入队**（addAuxJob），
	// 不等待就立刻读 runErr/rescueReady，读到的永远是零值——
	// 上一版正是因此把"脚本执行失败"吞掉、又在启动时误报"上下文抢救入口不可用"。
	done := make(chan struct{})
	if !container.RunOnLoop(func(vm *goja.Runtime) {
		defer close(done)
		_, runErr = vm.RunString(systemJSContent)
		if runErr != nil {
			return
		}
		// 这两个函数不是 bundle 的导出，而是 roles/index.ts 用
		// `globalThis.agentSystem = Object.assign(...)` 挂到宿主全局上的。
		// 历史上打包产物的 `var agentSystem = (function(){})()` 会在脚本末尾把这坨覆盖掉，
		// 表现为 reload.go 每次取快照都拿到 undefined、上下文静默丢失。
		// 所以在启动时就探一次，别等重载时才发现。
		probe, probeErr := vm.RunString(
			`typeof globalThis.agentSystem === 'object' && globalThis.agentSystem !== null &&` +
				` typeof globalThis.agentSystem.snapshotContext === 'function' &&` +
				` typeof globalThis.agentSystem.restoreContext === 'function'`)
		rescueReady = probeErr == nil && probe != nil && probe.ToBoolean()
	}) {
		return fmt.Errorf("goja 运行时未初始化，无法执行 agentSystem.js")
	}
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		return fmt.Errorf("执行 agentSystem.js 超时（事件循环 60 秒内没有执行到脚本）")
	}
	if runErr != nil {
		return fmt.Errorf("执行 agentSystem.js 代码失败: %w", runErr)
	}
	if !rescueReady {
		LoggerGeneral.Warn(lunarCoreModule,
			"上下文抢救入口不可用（agentSystem.snapshotContext / restoreContext）：引擎重载将只保证「能继续想」，上下文会丢失。"+
				"检查打包产物的 rollup --name 是否被改成 agentSystem（它会覆盖 roles/index.ts 挂的宿主全局）")
	}
	// 脚本执行成功：标记智能体就绪（数据导出等宿主侧入口据此判断可用性）
	agentReady.Store(true)
	return nil
}

// RunAgentContext 加载并运行嵌入式文件系统中的JavaScript文件
func RunAgentContext() error {
	if err := ensureRuntime(); err != nil {
		return fmt.Errorf("Lunar模块[JavaScript][ERROR] -> %w", err)
	}
	if err := runAgentScript(); err != nil {
		LoggerGeneral.SubError("LunarCore", "JavaScript", "%v", err)
		return fmt.Errorf("Lunar模块[JavaScript][ERROR] -> %w", err)
	}
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
