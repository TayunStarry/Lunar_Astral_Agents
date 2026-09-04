package LunarGoja

import "github.com/dop251/goja"

// standardEnv 标准网络环境适配器，持有当前 JavaScript 运行时实例
// 用于注册 fetch / getNetworkInterfaces / WebSocket 等标准环境函数
type standardEnv struct {
	runtime *goja.Runtime
}
