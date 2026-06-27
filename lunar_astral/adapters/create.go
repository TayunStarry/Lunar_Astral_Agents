package adapters

import (
	"context"
	"fmt"
	"logger"
	"lunar_astral/hierarchy"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/console"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/process"
	"github.com/dop251/goja_nodejs/require"
)

// registerAdaptersToRuntime 注册适配器函数到指定的JavaScript运行时环境
func registerAdaptersToRuntime(vm *goja.Runtime) {
	// 创建Runtime实例，用于存储JavaScript运行时实例
	adapters := &Runtime{runtime: vm}

	// 注册文件操作适配器
	vm.Set("saveFile", adapters.saveFile)
	vm.Set("readFile", adapters.readFile)
	vm.Set("fileView", adapters.fileView)
	vm.Set("fileList", adapters.fileList)

	// 注册数据库操作适配器
	vm.Set("database", adapters.database)

	// 注册网络操作适配器
	vm.Set("url", adapters.url)
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

	// 注册向量数据库适配器
	vm.Set("vectorInit", adapters.vectorInit)
	vm.Set("vectorAdd", adapters.vectorAdd)
	vm.Set("vectorQuery", adapters.vectorQuery)
	vm.Set("vectorDelete", adapters.vectorDelete)

	// 注册TTS语音合成适配器
	vm.Set("tts", adapters.tts)

	// 注册网络检索子系统适配器
	vm.Set("webSearchInit", adapters.webSearchInit)
	vm.Set("webSearchWebpage", adapters.webSearchWebpage)
	vm.Set("webSearchSimple", adapters.webSearchSimple)
	vm.Set("webSearchDepth", adapters.webSearchDepth)
	vm.Set("webSearchIsReady", adapters.webSearchIsReady)

	// 注册截图子系统适配器
	vm.Set("screenshotCapture", adapters.screenshotCapture)
	vm.Set("screenshotGetDisplays", adapters.screenshotGetDisplays)

	// 注册 LTPX 工具动态管理函数
	vm.Set("getLTPXToolStatus", adapters.getLTPXToolStatusForJS)
	vm.Set("processLTPXChanges", adapters.processLTPXChangesForJS)
}

// createAgentContext 创建并初始化JavaScript运行时环境
func createAgentContext() error {
	runtimeMutex.Lock()
	defer runtimeMutex.Unlock()

	// 如果运行时已存在，返回错误
	if runtime != nil {
		return fmt.Errorf("JavaScript运行时已存在")
	}

	// 创建上下文，用于控制运行时生命周期
	runtimeCtx, runtimeCancel = context.WithCancel(context.Background())

	// 初始化require模块系统
	registry := require.NewRegistry()

	// 加载eventloop模块
	runtime = eventloop.NewEventLoop(eventloop.WithRegistry(registry))

	// 启动eventloop
	runtime.Start()

	// 在eventloop中初始化运行时环境
	done := make(chan error, 1)
	runtime.RunOnLoop(func(vm *goja.Runtime) {
		// 加载console模块
		console.Enable(vm)

		// 加载process模块
		process.Enable(vm)

		// 注册适配器函数到JavaScript环境
		registerAdaptersToRuntime(vm)

		done <- nil
	})

	// 等待初始化完成
	err := <-done
	if err != nil {
		return err
	}
	return nil
}

// RunAgentContext 加载并运行嵌入式文件系统中的JavaScript文件
func RunAgentContext() error {
	runtimeMutex.Lock()
	// 如果运行时不存在，先创建
	if runtime == nil {
		runtimeMutex.Unlock()
		if err := createAgentContext(); err != nil {
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
	// 在eventloop中执行JavaScript代码
	runtime.RunOnLoop(func(vm *goja.Runtime) {
		_, err = vm.RunString(systemJSContent)
		if err != nil {
			logger.SubError("LunarCore", "JavaScript", "执行 agentSystem.js 代码失败: %v", err)
			return
		}
	})
	return nil
}

// RunOnAgentLoop 在 agent 事件循环中执行函数（供外部模块调用）
func RunOnAgentLoop(fn func(vm *goja.Runtime)) {
	if runtime == nil {
		logger.Error("LunarCore", "JavaScript 运行时未初始化，无法执行操作")
		return
	}
	runtime.RunOnLoop(fn)
}

// CloseAgentContext 关闭JavaScript运行时环境
func CloseAgentContext() {
	runtimeMutex.Lock()
	defer runtimeMutex.Unlock()

	// 如果运行时不存在，直接返回
	if runtime == nil {
		return
	}

	// 停止eventloop
	runtime.Stop()

	// 取消运行时上下文
	if runtimeCancel != nil {
		runtimeCancel()
	}

	// 清空运行时引用
	runtimeCtx = nil
	runtimeCancel = nil
	runtime = nil
}
