package context

import (
	"LunarCore/FileSystem"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"

	"modernc.org/quickjs"
)

// create 创建一个新的 QuickJS 上下文，并从嵌入式文件系统加载指定路径的 JS 文件
func create(path string) (*Context, error) {
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

// createUrl 创建一个新的 QuickJS 上下文，并加载指定 URL 的 JS 文件
func createUrl(url string) (*Context, error) {
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

// createfile 创建一个新的 QuickJS 上下文，并加载指定路径的 JS 文件
func createfile(path string) (*Context, error) {
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

// init 初始化上下文，注册所有函数
func init() {
	var system, err = create("system.js")
	if err != nil {
		log.Fatalf("创建系统上下文失败: %v", err)
	}
	system.Register("SaveFile", SaveFileAdapter, false)
	system.Register("ReadFile", ReadFileAdapter, false)
	system.Register("GetFileList", GetFileListAdapter, false)
	system.Register("ExecuteDatabaseRequest", ExecuteDatabaseRequestAdapter, false)
	system.Register("QueryCurrentAddress", QueryCurrentAddressAdapter, false)
	system.Register("VideoKeyframeExtraction", VideoKeyframeExtractionAdapter, false)
	system.Register("ProxyFetch", ProxyFetchAdapter, true)
}
