package context

import (
	"LunarCore/hierarchy"
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/console"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/process"
	"github.com/dop251/goja_nodejs/require"
)

// 全局变量，存储JavaScript运行时实例
var (
	runtimeMutex  sync.Mutex
	runtimeCtx    context.Context
	runtimeCancel context.CancelFunc
	el            *eventloop.EventLoop
)

// registerAdaptersToRuntime 注册适配器函数到指定的JavaScript运行时环境
func registerAdaptersToRuntime(vm *goja.Runtime) {
	// 注册文件操作适配器
	vm.Set("SaveFileAdapter", SaveFileAdapter)
	vm.Set("ReadFileAdapter", ReadFileAdapter)
	vm.Set("GetFileListAdapter", GetFileListAdapter)

	// 注册数据库操作适配器
	vm.Set("ExecuteDatabaseRequestAdapter", ExecuteDatabaseRequestAdapter)

	// 注册网络操作适配器
	vm.Set("QueryCurrentAddressAdapter", QueryCurrentAddressAdapter)
	vm.Set("GetSystemUrlAdapter", GetSystemUrlAdapter)
	vm.Set("ProxyFetchAdapter", ProxyFetchAdapter)

	// 注册图像处理适配器
	vm.Set("VideoKeyframeExtractionAdapter", VideoKeyframeExtractionAdapter)
	vm.Set("ResizeImageAdapter", ResizeImageAdapter)
	vm.Set("GenerateImageAdapter", GenerateImageAdapter)

	// 注册工具适配器
	vm.Set("AdapterLog", LogAdapter)
	// 注意：setTimeout/setInterval 等已由 eventloop 在 NewEventLoop 中自动注册
}

// CreateAgentContext 创建并初始化JavaScript运行时环境
func CreateAgentContext() error {
	runtimeMutex.Lock()
	defer runtimeMutex.Unlock()

	// 如果运行时已存在，返回错误
	if el != nil {
		return fmt.Errorf("JavaScript运行时已存在")
	}

	// 创建上下文，用于控制运行时生命周期
	runtimeCtx, runtimeCancel = context.WithCancel(context.Background())

	// 初始化require模块系统
	registry := require.NewRegistry()

	// 加载eventloop模块
	el = eventloop.NewEventLoop(eventloop.WithRegistry(registry))

	// 启动eventloop
	el.Start()

	// 在eventloop中初始化运行时环境
	done := make(chan error, 1)
	el.RunOnLoop(func(vm *goja.Runtime) {
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

	log.Println("Lunar模块[JavaScript] -> 运行时环境创建成功")
	return nil
}

// RunAgentContext 加载并运行嵌入式文件系统中的JavaScript文件
func RunAgentContext() error {
	runtimeMutex.Lock()
	// 如果运行时不存在，先创建
	if el == nil {
		runtimeMutex.Unlock()
		if err := CreateAgentContext(); err != nil {
			return fmt.Errorf("创建JavaScript运行时失败: %v", err)
		}
	} else {
		runtimeMutex.Unlock()
	}

	// 从嵌入式文件系统中读取system.js文件
	systemJS, err := hierarchy.EmbeddedFiles.ReadFile("assets/system.js")
	if err != nil {
		return fmt.Errorf("读取system.js文件失败: %v", err)
	}

	systemJSContent := string(systemJS)
	// 在eventloop中执行JavaScript代码
	el.RunOnLoop(func(vm *goja.Runtime) {
		_, err = vm.RunString(systemJSContent)
		if err != nil {
			log.Printf("Lunar模块[JavaScript][ERROR] -> 执行system.js代码失败: %v", err)
			return
		}

		log.Println("Lunar模块[JavaScript] -> system.js文件执行成功")
	})
	return nil
}

// CloseAgentContext 关闭JavaScript运行时环境
func CloseAgentContext() {
	runtimeMutex.Lock()
	defer runtimeMutex.Unlock()

	// 如果运行时不存在，直接返回
	if el == nil {
		return
	}

	// 停止eventloop
	el.Stop()

	// 取消运行时上下文
	if runtimeCancel != nil {
		runtimeCancel()
	}

	// 清空运行时引用
	runtimeCtx = nil
	runtimeCancel = nil
	el = nil

	log.Println("Lunar模块[JavaScript] -> 运行时环境关闭成功")
}
