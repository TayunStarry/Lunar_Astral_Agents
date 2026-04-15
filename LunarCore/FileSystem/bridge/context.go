package bridge

import (
	"LunarCore/FileSystem"
	"LunarCore/FileSystem/image"
	"LunarCore/FileSystem/memory"
	"LunarCore/server"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// SaveFileAdapter 是SaveFile函数的适配器，用于处理来自TypeScript的调用
func SaveFileAdapter(fileName string, overwrite bool, fileData any) (string, string, error) {
	// 处理不同类型的fileData
	var reader io.Reader

	switch data := fileData.(type) {

	case string:
		reader = strings.NewReader(data)

	case []byte:
		reader = bytes.NewReader(data)

	case map[string]any:
		// 处理 Blob 或 File 类型，它们在 quickjs 中会被转换为 map
		if buffer, ok := data["buffer"].([]byte); ok {
			reader = bytes.NewReader(buffer)
		} else if data, ok := data["data"].([]byte); ok {
			reader = bytes.NewReader(data)
		} else {
			return "", "", fmt.Errorf("不支持的 Blob/File 数据格式")
		}

	default:
		// 处理其他类型，可能需要根据实际情况进行调整
		return "", "", fmt.Errorf("不支持的文件数据类型")
	}

	// 调用原始的SaveFile函数
	return FileSystem.SaveFile(fileName, overwrite, reader)
}

// ReadFileAdapter 是ReadFile函数的适配器，用于处理来自TypeScript的调用
func ReadFileAdapter(filePath string) ([]byte, int64, string, error) {
	// 调用原始的ReadFile函数
	file, size, mimeType, err := FileSystem.ReadFile(filePath)
	if err != nil {
		return nil, 0, "", err
	}
	defer file.Close()

	// 读取文件内容
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, 0, "", fmt.Errorf("读取文件内容失败")
	}

	return content, size, mimeType, nil
}

// GetFileListAdapter 是GetFileList函数的适配器，用于处理来自TypeScript的调用
func GetFileListAdapter(path string) ([]map[string]any, error) {
	// 调用原始的GetFileList函数
	fileList, err := FileSystem.GetFileList(path)
	if err != nil {
		return nil, err
	}

	// 转换为TypeScript可以处理的格式
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

	return result, nil
}

// ExecuteDatabaseRequestAdapter 是ExecuteDatabaseRequest函数的适配器，用于处理来自TypeScript的调用
func ExecuteDatabaseRequestAdapter(request map[string]any) (map[string]any, error) {
	// 转换请求格式
	dbRequest := memory.DatabaseRequest{
		Transaction: request["transaction"].(bool),
	}

	// 转换操作列表
	operations, ok := request["operations"].([]interface{})
	if ok {
		dbRequest.Operations = make([]interface{}, len(operations))
		for i, op := range operations {
			if opMap, ok := op.(map[string]any); ok {
				dbRequest.Operations[i] = opMap
			}
		}
	}

	// 调用原始的ExecuteDatabaseRequest函数
	result := memory.ExecuteDatabaseRequest(dbRequest)

	// 转换结果为TypeScript可以处理的格式
	return map[string]any{
		"success":       result.Success,
		"error":         result.Error,
		"results":       result.Results,
		"total_time_ms": result.TotalTime,
		"operations":    result.Operations,
	}, nil
}

// QueryCurrentAddressAdapter 是QueryCurrentAddress函数的适配器，用于处理来自TypeScript的调用
func QueryCurrentAddressAdapter() ([]string, error) {
	// 调用原始的QueryCurrentAddress函数
	address := server.QueryCurrentAddress()
	return address, nil
}

// AgentProxyAdapter 处理OpenAI API请求的代理
func AgentProxyAdapter(url string, requestBody map[string]any, headers map[string]string) (map[string]any, error) {
	// 将requestBody转换为JSON
	requestBodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("请求体序列化失败: %v", err)
	}

	// 创建HTTP请求
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBodyJSON))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	// 解析响应为JSON
	var responseJSON map[string]any
	err = json.Unmarshal(responseBody, &responseJSON)
	if err != nil {
		return nil, fmt.Errorf("响应解析失败: %v", err)
	}

	return responseJSON, nil
}

// ExtractKeyFramesWithLocalCacheAdapter 是ExtractKeyFramesWithLocalCache函数的适配器，用于处理来自TypeScript的调用
func ExtractKeyFramesWithLocalCacheAdapter(inputFile string, cacheDir string) ([]map[string]any, error) {
	// 调用原始的ExtractKeyFramesWithLocalCache函数
	keyFrames, err := image.ExtractKeyFramesWithLocalCache(inputFile, cacheDir)
	if err != nil {
		return nil, err
	}

	// 转换为TypeScript可以处理的格式
	result := make([]map[string]any, len(keyFrames))
	for i, frame := range keyFrames {
		result[i] = map[string]any{
			"filePath":  frame.FilePath,
			"timestamp": frame.Timestamp,
			"frameNum":  frame.FrameNum,
			"data":      base64.StdEncoding.EncodeToString(frame.Data),
		}
	}

	return result, nil
}

// register 注册函数到上下文
func register(ctx *Context, name string, function any) {
	err := ctx.Register(name, function, false)
	if err != nil {
		log.Printf("注册 %s 函数失败: %v", name, err)
	}
}

// init 初始化上下文，注册所有函数
func init() {
	var SystemContext, err = CreateContext("system.js")
	if err != nil {
		log.Fatalf("创建系统上下文失败: %v", err)
	}
	register(SystemContext, "SaveFile", SaveFileAdapter)
	register(SystemContext, "ReadFile", ReadFileAdapter)
	register(SystemContext, "GetFileList", GetFileListAdapter)
	register(SystemContext, "ExecuteDatabaseRequest", ExecuteDatabaseRequestAdapter)
	register(SystemContext, "QueryCurrentAddress", QueryCurrentAddressAdapter)
	register(SystemContext, "AgentProxy", AgentProxyAdapter)
	register(SystemContext, "ExtractKeyFramesWithLocalCache", ExtractKeyFramesWithLocalCacheAdapter)
}
