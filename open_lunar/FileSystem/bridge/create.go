package bridge

import (
	"LunarCore/FileSystem"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"

	"modernc.org/quickjs"
)

// CreateContextCriterion 创建一个新的 QuickJS 上下文，并加载指定路径的 JS 文件
func CreateContextCriterion(path string) (*Context, error) {
	// 创建新的 QuickJS 虚拟机
	vm, err := quickjs.NewVM()
	if err != nil {
		return nil, fmt.Errorf("创建虚拟机失败: %v", err)
	}

	// 读取 JS 文件内容
	jsContent, err := os.ReadFile(path)
	if err != nil {
		vm.Close()
		return nil, fmt.Errorf("读取 JS 文件失败: %v", err)
	}

	// 执行 JS 文件内容，加载所有函数
	_, err = vm.Eval(string(jsContent), quickjs.EvalGlobal)
	if err != nil {
		vm.Close()
		return nil, fmt.Errorf("加载 JS 文件失败: %v", err)
	}

	// 返回封装的上下文
	return &Context{vm: vm}, nil
}

// CreateContextUrl 创建一个新的 QuickJS 上下文，并加载指定 URL 的 JS 文件
func CreateContextUrl(url string) (*Context, error) {
	// 创建新的 QuickJS 虚拟机
	vm, err := quickjs.NewVM()
	if err != nil {
		return nil, fmt.Errorf("创建虚拟机失败: %v", err)
	}

	// 从 URL 获取 JS 文件内容
	resp, err := http.Get(url)
	if err != nil {
		vm.Close()
		return nil, fmt.Errorf("获取 JS 文件失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应内容
	jsContent, err := io.ReadAll(resp.Body)
	if err != nil {
		vm.Close()
		return nil, fmt.Errorf("读取响应内容失败: %v", err)
	}

	// 执行 JS 文件内容，加载所有函数
	_, err = vm.Eval(string(jsContent), quickjs.EvalGlobal)
	if err != nil {
		vm.Close()
		return nil, fmt.Errorf("加载 JS 文件失败: %v", err)
	}

	// 返回封装的上下文
	return &Context{vm: vm}, nil
}

// CreateContext 创建一个新的 QuickJS 上下文，并从嵌入式文件系统加载指定路径的 JS 文件
func CreateContext(path string) (*Context, error) {
	// 创建新的 QuickJS 虚拟机
	vm, err := quickjs.NewVM()
	if err != nil {
		return nil, fmt.Errorf("创建虚拟机失败: %v", err)
	}

	// 直接使用嵌入式文件系统，创建子文件系统
	subFS, err := fs.Sub(FileSystem.EmbeddedFiles, "assets")
	if err != nil {
		vm.Close()
		return nil, fmt.Errorf("创建子文件系统失败: %v", err)
	}

	// 打开并读取 JS 文件内容
	file, err := subFS.Open(path)
	if err != nil {
		vm.Close()
		return nil, fmt.Errorf("打开嵌入式 JS 文件失败: %v", err)
	}
	defer file.Close()

	// 读取文件内容
	jsContent, err := io.ReadAll(file)
	if err != nil {
		vm.Close()
		return nil, fmt.Errorf("读取嵌入式 JS 文件失败: %v", err)
	}

	// 执行 JS 文件内容，加载所有函数
	_, err = vm.Eval(string(jsContent), quickjs.EvalGlobal)
	if err != nil {
		vm.Close()
		return nil, fmt.Errorf("加载 JS 文件失败: %v", err)
	}

	// 返回封装的上下文
	return &Context{vm: vm}, nil
}
