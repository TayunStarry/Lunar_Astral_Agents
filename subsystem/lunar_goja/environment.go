package LunarGoja

import (
	"LunarSubsystem/LoggerGeneral"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/console"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/process"
	"github.com/dop251/goja_nodejs/require"
)

// ==== 通用 goja 运行时环境 ====

// Init 创建并启动通用 goja 运行时环境（线程安全，只能成功初始化一次）
// 内置标准环境：console / process / require 模块系统、fetch、getNetworkInterfaces、WebSocket
// extra 回调在标准环境注册完成后调用，用于注册调用方自定义的适配器函数
// 运行时已存在时返回错误
func Init(extra func(vm *goja.Runtime)) error {
	runtimeMutex.Lock()
	if loop != nil {
		runtimeMutex.Unlock()
		return fmt.Errorf("goja 运行时已存在")
	}

	// 初始化 require 模块系统
	registry := require.NewRegistry()

	// 创建并启动事件循环
	newLoop := eventloop.NewEventLoop(eventloop.WithRegistry(registry))
	loop = newLoop
	newLoop.Start()
	runtimeMutex.Unlock()

	// 在事件循环中注册标准环境与调用方自定义函数
	done := make(chan error, 1)
	if !newLoop.RunOnLoop(func(vm *goja.Runtime) {
		// 加载 console / process 模块
		console.Enable(vm)
		process.Enable(vm)

		// 注册标准环境（fetch / getNetworkInterfaces / WebSocket）
		registerStandardEnvironment(vm)

		// 注册调用方自定义适配器
		if extra != nil {
			extra(vm)
		}

		done <- nil
	}) {
		return fmt.Errorf("goja 运行时已终止，无法初始化")
	}
	return <-done
}

// registerStandardEnvironment 注册标准网络环境函数到运行时
func registerStandardEnvironment(vm *goja.Runtime) {
	// 创建标准环境实例，用于存储当前 JavaScript 运行时实例
	env := &standardEnv{runtime: vm}

	// 注册标准 Fetch API
	vm.Set("fetch", env.fetch)

	// 注册网络接口枚举
	vm.Set("getNetworkInterfaces", env.getNetworkInterfaces)

	// 注册标准 WebSocket 客户端（含静态常量）
	env.registerWebSocket()
}

// RunOnLoop 在运行时事件循环上调度执行函数（线程安全，可在任意 goroutine 调用）
// 返回 false 表示运行时未初始化
func RunOnLoop(fn func(vm *goja.Runtime)) bool {
	runtimeMutex.Lock()
	cur := loop
	runtimeMutex.Unlock()

	if cur == nil {
		LoggerGeneral.Error("LunarGoja", "goja 运行时未初始化，无法执行操作")
		return false
	}
	return cur.RunOnLoop(fn)
}

// RunScript 在事件循环上执行一段 JavaScript 脚本，返回执行错误
func RunScript(script string) error {
	var execErr error
	if !RunOnLoop(func(vm *goja.Runtime) {
		_, execErr = vm.RunString(script)
	}) {
		return fmt.Errorf("goja 运行时未初始化，无法执行脚本")
	}
	return execErr
}

// Close 停止并清理运行时环境（线程安全）
func Close() {
	runtimeMutex.Lock()
	defer runtimeMutex.Unlock()

	if loop == nil {
		return
	}
	loop.Stop()
	loop = nil
}

// IsReady 返回运行时环境是否已初始化
func IsReady() bool {
	runtimeMutex.Lock()
	defer runtimeMutex.Unlock()
	return loop != nil
}
