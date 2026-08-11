package adapters

import (
	"LunarSubsystem/file_manager/module"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"lunar_astral/hierarchy"
	"os"
	"path/filepath"
	"strings"

	"github.com/dop251/goja"
)

// saveFile 适配TypeScript调用的文件保存功能，支持字符串、字节数组及Blob/File类型数据
// 返回值: [string, string, error] 文件名、路径和错误信息
func (class *Runtime) saveFile(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 3 {
		return class.runtime.ToValue([]any{"", "", fmt.Errorf("参数不足")})
	}

	fileName, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{"", "", fmt.Errorf("fileName必须是字符串")})
	}

	overwrite, ok := call.Argument(1).Export().(bool)
	if !ok {
		overwrite = false
	}

	fileData := call.Argument(2).Export()
	var reader io.Reader

	switch data := fileData.(type) {
	case string:
		reader = strings.NewReader(data)
	case []byte:
		reader = bytes.NewReader(data)
	case map[string]any:
		// 处理goja中转换为map的Blob/File类型
		if buffer, ok := data["buffer"].([]byte); ok {
			reader = bytes.NewReader(buffer)
		} else if data, ok := data["data"].([]byte); ok {
			reader = bytes.NewReader(data)
		} else {
			return class.runtime.ToValue([]any{"", "", fmt.Errorf("不支持的 Blob/File 数据格式")})
		}
	default:
		return class.runtime.ToValue([]any{"", "", fmt.Errorf("不支持的文件数据类型")})
	}

	fileName, path, err := module.SaveFile(fileName, overwrite, reader)
	if err != nil {
		return class.runtime.ToValue([]any{"", "", err})
	}
	return class.runtime.ToValue([]any{fileName, path, nil})
}

// readFile 适配TypeScript调用的文件读取功能，返回文件内容、大小和MIME类型
// 返回值: [string, number, string, error] 文件内容、大小、MIME类型和错误信息
func (class *Runtime) readFile(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{nil, 0, "", fmt.Errorf("参数不足")})
	}

	filePath, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{nil, 0, "", fmt.Errorf("filePath必须是字符串")})
	}

	file, size, mimeType, err := module.ReadFile(filePath)
	if err != nil {
		return class.runtime.ToValue([]any{nil, 0, "", err})
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return class.runtime.ToValue([]any{nil, 0, "", fmt.Errorf("读取文件内容失败")})
	}

	// 将文件内容转换为base64编码
	base64Content := base64.StdEncoding.EncodeToString(content)

	return class.runtime.ToValue([]any{base64Content, size, mimeType, nil})
}

// fileList 适配TypeScript调用的文件列表获取功能，转换为TypeScript可处理的格式
// 返回值: [Array<{name: string, size: number, isDir: boolean, lastModified: string, path: string}>, error] 文件列表和错误信息
func (class *Runtime) fileList(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("参数不足")})
	}

	path, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("path必须是字符串")})
	}

	fileList, err := module.GetFileList(path)
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}

	// 转换为TypeScript可处理的格式
	result := make([]map[string]any, len(fileList))
	for i, file := range fileList {
		result[i] = map[string]any{
			"name":         file.Name,
			"size":         file.Size,
			"isDir":        file.IsDir,
			"lastModified": file.LastModified.Format("2006-01-02 15:04:05"),
			"path":         file.Path,
		}
	}

	return class.runtime.ToValue([]any{result, nil})
}

// fileView 适配TypeScript调用读取嵌入式文件系统中的内容
// assets是嵌入式文件系统的根目录，调用时只需传入相对于assets的文件路径
// 返回值: [string, error] 文件内容和错误信息
func (class *Runtime) fileView(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{"", fmt.Errorf("参数不足")})
	}

	filePath, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{"", fmt.Errorf("filePath必须是字符串")})
	}

	fullPath := "assets/" + filePath
	content, err := hierarchy.EmbeddedFiles.ReadFile(fullPath)
	if err != nil {
		return class.runtime.ToValue([]any{"", fmt.Errorf("读取嵌入式文件失败: %v", err)})
	}

	return class.runtime.ToValue([]any{string(content), nil})
}

// saveDebugFile 将调试内容写入本地文件（覆写模式）
// 参数: filePath(绝对路径), content(字符串内容)
// 返回值: [string, error] 文件路径和错误信息
func (class *Runtime) saveDebugFile(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		return class.runtime.ToValue([]any{"", fmt.Errorf("参数不足，需要 filePath 和 content")})
	}

	filePath, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{"", fmt.Errorf("filePath必须是字符串")})
	}

	content, ok := call.Argument(1).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{"", fmt.Errorf("content必须是字符串")})
	}

	// 确保目录存在
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return class.runtime.ToValue([]any{"", fmt.Errorf("创建目录失败: %w", err)})
	}

	// 覆写写入文件
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return class.runtime.ToValue([]any{"", fmt.Errorf("写入文件失败: %w", err)})
	}

	return class.runtime.ToValue([]any{filePath, nil})
}
