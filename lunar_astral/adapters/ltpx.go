package adapters

import (
	logger "LunarSubsystem/LoggerGeneral"
	"encoding/json"

	"github.com/dop251/goja"
)

// LoadLTPXTool 将工具加入待加载队列（由 HTTP handler 调用）
func LoadLTPXTool(name, defJSON, jsCode string) {
	ltpMutex.Lock()
	defer ltpMutex.Unlock()
	pendingLoads = append(pendingLoads, &LTPXToolInfo{
		Name: name, Definition: defJSON, JS: jsCode,
	})
	logger.Info("LunarCore", "LTPX 工具 %s 已加入待加载队列", name)
}

// UnloadLTPXTool 将工具加入待卸载队列（由 HTTP handler 调用）
func UnloadLTPXTool(name string) {
	ltpMutex.Lock()
	defer ltpMutex.Unlock()
	pendingUnloads = append(pendingUnloads, name)
	logger.Info("LunarCore", "LTPX 工具 %s 已加入待卸载队列", name)
}

// getLTPXToolStatus 返回当前工具状态（供 JS 端 getLTPXToolStatus 调用）
// 返回后清空 pending 队列
func getLTPXToolStatus() *LTPXStatus {
	ltpMutex.Lock()
	defer ltpMutex.Unlock()

	loadedNames := make([]string, 0, len(loadedTools))
	for name := range loadedTools {
		loadedNames = append(loadedNames, name)
	}

	status := &LTPXStatus{
		Loaded:         loadedNames,
		PendingLoads:   pendingLoads,
		PendingUnloads: pendingUnloads,
	}

	// 清空 pending 队列
	pendingLoads = nil
	pendingUnloads = nil

	return status
}

// ensureGlobalConfigGlobal 确保 GlobalConfig 作为全局变量可用（兼容旧工具包代码）
func ensureGlobalConfigGlobal(vm *goja.Runtime) {
	agentSystemVal := vm.Get("agentSystem")
	if agentSystemVal == nil || goja.IsUndefined(agentSystemVal) {
		return
	}
	agentSystemObj := agentSystemVal.ToObject(vm)
	GlobalConfigVal := agentSystemObj.Get("GlobalConfig")
	if GlobalConfigVal == nil || goja.IsUndefined(GlobalConfigVal) {
		return
	}
	vm.Set("GlobalConfig", GlobalConfigVal)
}

// applyLTPXLoad 在 goja 事件循环中执行工具加载
func applyLTPXLoad(vm *goja.Runtime, info *LTPXToolInfo) {
	// 确保 GlobalConfig 全局变量可用（兼容旧工具包直接引用 GlobalConfig 的代码）
	ensureGlobalConfigGlobal(vm)

	// 执行工具 JS 代码（工具会自行注册到 GlobalConfig.LTPfunction）
	_, err := vm.RunString(info.JS)
	if err != nil {
		logger.Error("LunarCore", "LTPX 执行工具代码失败 %s: %v", info.Name, err)
		return
	}

	// 注入工具定义到 GlobalConfig.LTPdefinition
	_, err = vm.RunString(`agentSystem.GlobalConfig.LTPdefinition.push(` + info.Definition + `);`)
	if err != nil {
		logger.Error("LunarCore", "LTPX 注入工具定义失败 %s: %v", info.Name, err)
		return
	}

	ltpMutex.Lock()
	loadedTools[info.Name] = info
	ltpMutex.Unlock()

	logger.Info("LunarCore", "LTPX 工具加载成功: %s", info.Name)
}

// applyLTPXUnload 在 goja 事件循环中执行工具卸载
func applyLTPXUnload(vm *goja.Runtime, name string) {
	// 从 GlobalConfig.LTPdefinition 中移除
	_, err := vm.RunString(`
		(function() {
			var defs = agentSystem.GlobalConfig.LTPdefinition;
			for (var i = defs.length - 1; i >= 0; i--) {
				if (defs[i].function && defs[i].function.name === '` + name + `') {
					defs.splice(i, 1);
				}
			}
		})();
	`)
	if err != nil {
		logger.Error("LunarCore", "LTPX 移除工具定义失败 %s: %v", name, err)
	}

	// 从 GlobalConfig.LTPfunction 中移除
	_, err = vm.RunString(`
		(function() {
			agentSystem.GlobalConfig.LTPfunction.delete('` + name + `');
		})();
	`)
	if err != nil {
		logger.Error("LunarCore", "LTPX 移除工具函数失败 %s: %v", name, err)
	}

	ltpMutex.Lock()
	delete(loadedTools, name)
	ltpMutex.Unlock()

	logger.Info("LunarCore", "LTPX 工具卸载成功: %s", name)
}

// ProcessPendingLTPXChanges 在 goja 事件循环中处理所有待处理的加载/卸载
// 由 getLTPXToolStatus 的 JS 端调用后自动触发
func ProcessPendingLTPXChanges(vm *goja.Runtime, statusJSON string) {
	var status LTPXStatus
	if err := json.Unmarshal([]byte(statusJSON), &status); err != nil {
		logger.Error("LunarCore", "LTPX 解析状态失败: %v", err)
		return
	}

	for _, info := range status.PendingLoads {
		applyLTPXLoad(vm, info)
	}

	for _, name := range status.PendingUnloads {
		applyLTPXUnload(vm, name)
	}
}

// getLTPXToolStatusForJS 供 JS 端调用的 Go 函数，返回工具状态 JSON
func (class *Runtime) getLTPXToolStatusForJS() goja.Value {
	status := getLTPXToolStatus()
	data, _ := json.Marshal(status)
	return class.runtime.ToValue(string(data))
}

// processLTPXChangesForJS 供 JS 端调用的 Go 函数，在事件循环中处理待处理的加载/卸载
func (class *Runtime) processLTPXChangesForJS(statusJSON string) goja.Value {
	ProcessPendingLTPXChanges(class.runtime, statusJSON)
	return class.runtime.ToValue(true)
}

// LoadLTPXToolOnLoop 在事件循环中完整加载工具（供 HTTP handler 调用）
func LoadLTPXToolOnLoop(name, defJSON, jsCode string) {
	RunOnAgentLoop(func(vm *goja.Runtime) {
		info := &LTPXToolInfo{Name: name, Definition: defJSON, JS: jsCode}
		applyLTPXLoad(vm, info)
	})
}

// UnloadLTPXToolOnLoop 在事件循环中完整卸载工具（供 HTTP handler 调用）
func UnloadLTPXToolOnLoop(name string) {
	RunOnAgentLoop(func(vm *goja.Runtime) {
		applyLTPXUnload(vm, name)
	})
}

// GetLTPXToolStatus 导出函数，供外部查询当前工具状态
func GetLTPXToolStatus() *LTPXStatus {
	return getLTPXToolStatus()
}
