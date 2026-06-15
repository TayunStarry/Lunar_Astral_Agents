package adapters

import (
	"config"
	"encoding/json"
	"logger"
	"os"
	"path/filepath"
	"slices"

	"github.com/dop251/goja"
)

// LTP2PackageInfo LTP2 工具包配置结构
type LTP2PackageInfo struct {
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Tags        []string         `json:"tags"`
	URL         string           `json:"url"`
	Tools       []map[string]any `json:"tools"`
}

// scanLTP2Packages 扫描 local_data/package/ 下所有带有 "LTP2" 标签的工具包
// 返回包信息列表和对应的 tool.js 源码（key 为包名）
func scanLTP2Packages() ([]LTP2PackageInfo, map[string]string) {
	packageDir := filepath.Join(*config.LocalDir, "package")

	entries, err := os.ReadDir(packageDir)
	if err != nil {
		logger.Error("LunarCore", "LTP2 扫描包目录失败: %v", err)
		return nil, nil
	}

	var packages []LTP2PackageInfo
	toolSources := make(map[string]string)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		configPath := filepath.Join(packageDir, entry.Name(), "metadata.json")
		data, err := os.ReadFile(configPath)
		if err != nil {
			continue
		}

		var pkg LTP2PackageInfo
		if err = json.Unmarshal(data, &pkg); err != nil {
			logger.Warn("LunarCore", "LTP2 解析包配置失败 %s: %v", configPath, err)
			continue
		}

		// 仅接受带有 "LTP2" 标签的工具包
		hasLTP2 := slices.Contains(pkg.Tags, "LTP2")
		if !hasLTP2 {
			continue
		}

		// 读取 tool.js 文件
		toolPath := filepath.Join(packageDir, entry.Name(), "tool.js")
		toolCode, err := os.ReadFile(toolPath)
		if err != nil {
			logger.Warn("LunarCore", "LTP2 读取工具文件失败 %s: %v", toolPath, err)
			continue
		}

		toolSources[pkg.ID] = string(toolCode)
		packages = append(packages, pkg)
	}

	return packages, toolSources
}

// loadLTP2ToolPackages 在 goja 运行时中加载所有 LTP2 工具包
// 必须在 agentSystem.js 执行完毕（agentSystem 全局变量可用）后调用
// 返回工具定义 JSON 字符串；每类工具的函数处理器会注册到 globalThis.__ltp2Handlers
func loadLTP2ToolPackages(vm *goja.Runtime) string {
	packages, toolSources := scanLTP2Packages()
	if len(packages) == 0 {
		return "[]"
	}

	// 获取 agentSystem.OnlyData，注入为全局变量供 tool.js 使用
	agentSystemVal := vm.Get("agentSystem")
	if agentSystemVal == nil || goja.IsUndefined(agentSystemVal) {
		logger.Warn("LunarCore", "LTP2 agentSystem 不可用，跳过工具注册")
		return "[]"
	}

	agentSystemObj := agentSystemVal.ToObject(vm)
	onlyDataVal := agentSystemObj.Get("OnlyData")
	if onlyDataVal == nil || goja.IsUndefined(onlyDataVal) {
		logger.Warn("LunarCore", "LTP2 OnlyData 不可用，跳过工具注册")
		return "[]"
	}

	// 将 OnlyData 注入为全局变量
	vm.Set("OnlyData", onlyDataVal)

	// 收集所有工具定义
	var allTools []map[string]any

	for _, pkg := range packages {
		toolCode, ok := toolSources[pkg.ID]
		if !ok {
			continue
		}

		// 在 goja 中执行工具代码（工具会自行注册到 OnlyData.LTPfunction）
		_, err := vm.RunString(toolCode)
		if err != nil {
			logger.Error("LunarCore", "LTP2 执行工具代码失败 %s: %v", pkg.ID, err)
			continue
		}

		if len(pkg.Tools) > 0 {
			allTools = append(allTools, pkg.Tools...)
		}

		logger.Info("LunarCore", "LTP2 工具包加载成功: %s (%d 个工具)", pkg.ID, len(pkg.Tools))
	}

	// 序列化工具定义为 JSON
	toolsJSON, err := json.Marshal(allTools)
	if err != nil {
		logger.Error("LunarCore", "LTP2 序列化工具定义失败: %v", err)
		return "[]"
	}

	logger.Info("LunarCore", "LTP2 工具包加载完成，共 %d 个工具", len(allTools))
	return string(toolsJSON)
}
