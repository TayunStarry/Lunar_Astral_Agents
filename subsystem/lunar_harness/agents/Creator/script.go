package Creator

// script.go LTP9 脚本运行与状态监控。
//
// 管线名为「月华.生活.创作」：把 /document 下的多个脚本按顺序拼接成 execute.js，
// 写入固定运行时包目录，先调用引擎签发器生成 permissions.key（授权文件），
// 再触发引擎重扫加载；脚本文件与授权文件都随包目录一并加载。
// 监控通过引擎插件状态回报加载结果、错误、导出函数与最近一次调用结果。

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	StarLTP "LunarSubsystem/LunarHarness/engine/plugin"
)

// runLTP9Scripts 按顺序拼接并运行多个脚本文件。
func runLTP9Scripts(scripts []string) (scriptRunResult, error) {
	if len(scripts) == 0 {
		return scriptRunResult{}, errors.New("scripts 不能为空")
	}
	parts := make([]string, 0, len(scripts))
	for _, rel := range scripts {
		content, err := readDocument(rel)
		if err != nil {
			return scriptRunResult{}, fmt.Errorf("读取脚本 %s 失败: %w", rel, err)
		}
		parts = append(parts, content)
	}
	code := strings.Join(parts, "\n;\n")
	return deployAndRun(code)
}

// deployAndRun 写入运行时包、签发授权、加载并尝试执行入口。
func deployAndRun(code string) (scriptRunResult, error) {
	root := runtimePackageRoot()
	// 先移除旧包触发卸载，确保新脚本不会被同目录旧实例挡住。
	_ = os.RemoveAll(root)
	StarLTP.Rescan()

	if err := os.MkdirAll(root, 0o755); err != nil {
		return scriptRunResult{}, fmt.Errorf("创建运行时包目录失败: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, "execute.js"), []byte(code), 0o644); err != nil {
		return scriptRunResult{}, fmt.Errorf("写入 execute.js 失败: %w", err)
	}
	meta := fmt.Sprintf(`{"id":%q,"title":%q,"tags":["LTP9"]}`, runtimePackageID, runtimePackageTitle)
	if err := os.WriteFile(filepath.Join(root, "metadata.json"), []byte(meta), 0o644); err != nil {
		return scriptRunResult{}, fmt.Errorf("写入 metadata.json 失败: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte("name: "+runtimePackageTitle+"\n"), 0o644); err != nil {
		return scriptRunResult{}, fmt.Errorf("写入 config.yaml 失败: %w", err)
	}

	auth, err := StarLTP.SignPackageAuth(root)
	if err != nil {
		return scriptRunResult{}, fmt.Errorf("生成授权文件失败: %w", err)
	}
	StarLTP.Rescan()
	return invokeRuntime(auth.Granted)
}

// runtimePackageRoot 返回与 LTP9 引擎一致的包目录路径：
// 引擎按「可执行目录 + LocalDir + package」扫描，这里必须完全对齐。
func runtimePackageRoot() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), config.LocalDir, packageRootRel, runtimePackageID)
	}
	return filepath.Join(config.LocalDir, packageRootRel, runtimePackageID)
}

// invokeRuntime 读取运行时包状态，必要时调用导出入口。
func invokeRuntime(granted []string) (scriptRunResult, error) {
	result := scriptRunResult{Package: runtimePackageID, Authorized: granted}
	state, ok := findPluginState(runtimePackageID)
	if !ok {
		result.Error = "引擎未扫描到创作运行时包"
		return result, nil
	}
	result.Loaded = state.Loaded
	result.Exports = state.Exports
	result.Running = state.Loaded
	if strings.TrimSpace(state.Error) != "" {
		result.Error = state.Error
		return result, nil
	}
	if containsString(state.Exports, "main") {
		value, err := StarLTP.Call(runtimePackageID, "main", nil)
		if err != nil {
			result.Error = err.Error()
		} else {
			result.Result = value
		}
	}
	recordLastRun(result)
	return result, nil
}

// scriptStatus 查询指定包（默认创作管线）的运行状态。
func scriptStatus(name string) (scriptRunResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = runtimePackageID
	}
	state, ok := findPluginState(name)
	if !ok {
		return scriptRunResult{Package: name, Error: "未找到该包"}, nil
	}
	result := scriptRunResult{
		Package:    state.ID,
		Loaded:     state.Loaded,
		Running:    state.Loaded,
		Authorized: state.Granted,
		Exports:    state.Exports,
		Error:      state.Error,
	}
	lastRunMu.Lock()
	if last, ok := lastRunMeta[name]; ok {
		result.Result = last
	}
	lastRunMu.Unlock()
	return result, nil
}

// findPluginState 在引擎插件状态中按 ID 查找。
func findPluginState(id string) (StarLTP.PluginState, bool) {
	for _, state := range StarLTP.PluginStates() {
		if state.ID == id {
			return state, true
		}
	}
	return StarLTP.PluginState{}, false
}

// recordLastRun 记录最近一次运行结果，供 script_status 汇报。
func recordLastRun(result scriptRunResult) {
	lastRunMu.Lock()
	lastRunMeta[result.Package] = map[string]any{
		"loaded":  result.Loaded,
		"running": result.Running,
		"exports": result.Exports,
		"result":  result.Result,
		"error":   result.Error,
		"updated": nowUnix(),
	}
	lastRunMu.Unlock()
}

// containsString 判断字符串是否在切片中。
func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
