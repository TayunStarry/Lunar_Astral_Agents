package adapters

import "github.com/dop251/goja"

// IPInfo 存储IP地址信息
type IPInfo struct {
	Region string `json:"region"`
	City   string `json:"city"`
}

// Adapters 存储JavaScript运行时实例，用于调用适配器函数
type Adapters struct {
	runtime *goja.Runtime
}
