package context

import (
	"LunarCore/hierarchy"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"

	"modernc.org/quickjs"
)

// ReadEmbed 从嵌入式文件系统读取 JS 文件内容
func ReadEmbed(path string) (string, error) {
	// 创建嵌入式子文件系统
	subFS, err := fs.Sub(hierarchy.EmbeddedFiles, "assets")
	if err != nil {
		return "", fmt.Errorf("创建子文件系统失败: %v", err)
	}
	// 打开并读取 JS 文件内容
	file, err := subFS.Open(path)
	if err != nil {
		return "", fmt.Errorf("打开嵌入式 JS 文件失败: %v", err)
	}
	defer file.Close()
	// 读取文件内容
	content, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("读取嵌入式 JS 文件失败: %v", err)
	}
	// 返回文件内容
	return string(content), nil
}

// ReadFile 从磁盘读取 JS 文件内容
func ReadFile(path string) (string, error) {
	// 读取文件内容
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %v", err)
	}
	// 返回文件内容
	return string(content), nil
}

// ReadUrl 从 URL 获取 JS 文件内容
func ReadUrl(url string) (string, error) {
	// 从 URL 获取 JS 文件内容
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("获取 JS 文件失败: %v", err)
	}
	defer resp.Body.Close()
	// 读取响应内容
	jsContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应内容失败: %v", err)
	}
	// 返回文件内容
	return string(jsContent), nil
}

// CreateContext 创建智能体系统上下文
func CreateContext(content string) (*Context, error) {
	// 创建新的 QuickJS 虚拟机
	vm, err := quickjs.NewVM()
	if err != nil {
		return nil, fmt.Errorf("创建虚拟机失败: %v", err)
	}
	system := &Context{
		vm: vm,
	}
	// 注册内嵌函数适配器
	system.Register("_SaveFile", SaveFileAdapter, false)
	system.Register("_ReadFile", ReadFileAdapter, false)
	system.Register("_GetFileList", GetFileListAdapter, false)
	system.Register("_ExecuteDatabaseRequest", ExecuteDatabaseRequestAdapter, false)
	system.Register("_QueryCurrentAddress", QueryCurrentAddressAdapter, false)
	system.Register("_GetSystemUrl", GetSystemUrlAdapter, false)
	system.Register("_VideoKeyframeExtraction", VideoKeyframeExtractionAdapter, false)
	system.Register("_ProxyFetch", ProxyFetchAdapter, true)
	system.Register("_ResizeImage", ResizeImageAdapter, false)
	system.Register("_GenerateImage", GenerateImageAdapter, true)
	system.Register("_waiter", WaiterAdapter, true)
	system.Register("_log", LogAdapter, true)
	// 执行 JS 文件内容，加载所有业务函数
	_, err = vm.Eval(content, quickjs.EvalGlobal)
	if err != nil {
		vm.Close()
		return nil, fmt.Errorf("加载 JS 文件失败: %v", err)
	}
	// 返回封装的上下文
	return system, nil
}

// RunAgentContext 初始化智能体系统上下文
func RunAgentContext() {
	content, err := ReadEmbed("system.js")
	if err != nil {
		log.Fatalf("读取系统 JS 文件失败: %v", err)
	}
	_, err = CreateContext(content)
	if err != nil {
		log.Fatalf("创建系统上下文失败: %v", err)
	}
}
