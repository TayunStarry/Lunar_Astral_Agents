package modellease

// unload.go 模型卸载的单一入口：统一「取租约 → 卸载 → 释放」的时序。
//
// 卸载实现由装配层注入（protocol 不反向依赖 engine），各调用方只描述
// 需要的租约持有范围，不再各自拼装 AcquireSwap + UnloadModelByID。

import "fmt"

// SetUnloader 注入模型卸载实现（由装配层在启动时调用一次）。
func SetUnloader(fn func(model string) (bool, error)) {
	unloader = fn
}

// Unload 在模型切换租约内卸载指定模型，租约在卸载完成后立即释放。
// 返回是否确实发生了卸载；未注入卸载实现时返回错误，由调用方按原逻辑降级。
func Unload(model string) (bool, error) {
	if unloader == nil {
		return false, fmt.Errorf("模型卸载实现未注入")
	}
	release := AcquireSwap()
	defer release()
	return unloader(model)
}

// UnloadHeld 在「已持有切换租约」的前提下卸载指定模型（不重复取租约）。
// 供 AcquireUnload 的调用方在租约期内收尾使用。
func UnloadHeld(model string) (bool, error) {
	if unloader == nil {
		return false, fmt.Errorf("模型卸载实现未注入")
	}
	return unloader(model)
}

// AcquireUnload 取租约并卸载指定模型，但租约留给调用方释放。
// 供需要在随后一段时间内独占切换窗口的调用方（如轻量扫描全程持有）使用；
// 无论卸载是否成功，返回的释放函数都必须调用。
func AcquireUnload(model string) (release func(), unloaded bool, err error) {
	if unloader == nil {
		return func() {}, false, fmt.Errorf("模型卸载实现未注入")
	}
	release = AcquireSwap()
	unloaded, err = unloader(model)
	return release, unloaded, err
}
