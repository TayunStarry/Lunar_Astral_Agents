package context

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
	"net/http"
	"strings"
)

// SaveFileAdapter 适配TypeScript调用的文件保存功能，支持字符串、字节数组及Blob/File类型数据
func SaveFileAdapter(fileName string, overwrite bool, fileData any) (string, string, error) {
	var reader io.Reader

	switch data := fileData.(type) {
	case string:
		reader = strings.NewReader(data)
	case []byte:
		reader = bytes.NewReader(data)
	case map[string]any:
		// 处理quickjs中转换为map的Blob/File类型
		if buffer, ok := data["buffer"].([]byte); ok {
			reader = bytes.NewReader(buffer)
		} else if data, ok := data["data"].([]byte); ok {
			reader = bytes.NewReader(data)
		} else {
			return "", "", fmt.Errorf("不支持的 Blob/File 数据格式")
		}
	default:
		return "", "", fmt.Errorf("不支持的文件数据类型")
	}

	return FileSystem.SaveFile(fileName, overwrite, reader)
}

// ReadFileAdapter 适配TypeScript调用的文件读取功能，返回文件内容、大小和MIME类型
func ReadFileAdapter(filePath string) ([]byte, int64, string, error) {
	file, size, mimeType, err := FileSystem.ReadFile(filePath)
	if err != nil {
		return nil, 0, "", err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, 0, "", fmt.Errorf("读取文件内容失败")
	}

	return content, size, mimeType, nil
}

// GetFileListAdapter 适配TypeScript调用的文件列表获取功能，转换为TypeScript可处理的格式
func GetFileListAdapter(path string) ([]map[string]any, error) {
	fileList, err := FileSystem.GetFileList(path)
	if err != nil {
		return nil, err
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

	return result, nil
}

// ExecuteDatabaseRequestAdapter 适配TypeScript调用的数据库操作功能，处理请求并转换结果格式
func ExecuteDatabaseRequestAdapter(request map[string]any) (map[string]any, error) {
	// 构建数据库请求
	dbRequest := memory.DatabaseRequest{
		Transaction: request["transaction"].(bool),
	}

	// 转换操作列表
	if operations, ok := request["operations"].([]interface{}); ok {
		dbRequest.Operations = make([]interface{}, len(operations))
		for i, op := range operations {
			if opMap, ok := op.(map[string]any); ok {
				dbRequest.Operations[i] = opMap
			}
		}
	}

	// 执行数据库操作
	result := memory.ExecuteDatabaseRequest(dbRequest)

	// 转换结果格式
	return map[string]any{
		"success":       result.Success,
		"error":         result.Error,
		"results":       result.Results,
		"total_time_ms": result.TotalTime,
		"operations":    result.Operations,
	}, nil
}

// QueryCurrentAddressAdapter 适配TypeScript调用的网络地址查询功能，获取当前服务器网络地址列表
func QueryCurrentAddressAdapter() ([]string, error) {
	return server.QueryCurrentAddress(), nil
}

// VideoKeyframeExtractionAdapter 适配TypeScript调用的视频关键帧提取功能，转换为TypeScript可处理的格式
func VideoKeyframeExtractionAdapter(inputFile string, cacheDir string) ([]map[string]any, error) {
	keyFrames, err := image.VideoKeyframeExtraction(inputFile, cacheDir)
	if err != nil {
		return nil, err
	}

	// 转换为TypeScript可处理的格式
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

// ProxyFetchAdapter 适配TypeScript调用的网络请求代理功能，处理HTTP请求并返回统一格式响应
func ProxyFetchAdapter(config map[string]any) (map[string]any, error) {
	// 解析URL
	url, ok := config["url"].(string)
	if !ok {
		return nil, fmt.Errorf("无效的URL")
	}

	// 解析执行配置
	execute, ok := config["execute"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("无效的execute")
	}

	// 确定HTTP方法
	method := "GET"
	if methodVal, ok := execute["method"].(string); ok {
		method = methodVal
	}

	// 准备请求体
	var body io.Reader
	if bodyVal, ok := execute["body"]; ok {
		bodyJSON, err := json.Marshal(bodyVal)
		if err != nil {
			return nil, fmt.Errorf("请求体序列化失败: %v", err)
		}
		body = bytes.NewBuffer(bodyJSON)
	}

	// 创建HTTP请求
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	// 设置请求头
	if headers, ok := execute["headers"].(map[string]any); ok {
		for key, value := range headers {
			if valueStr, ok := value.(string); ok {
				req.Header.Set(key, valueStr)
			}
		}
	}

	// 处理跨域请求
	if crossDomain, ok := execute["crossDomain"].(bool); ok && crossDomain {
		req.Header.Set("Origin", "*")
		req.Header.Set("Access-Control-Allow-Origin", "*")
		req.Header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		req.Header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
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
		// 非JSON响应返回原始内容
		return map[string]any{
			"status":  resp.StatusCode,
			"headers": resp.Header,
			"body":    string(responseBody),
		}, nil
	}

	// 返回JSON响应
	return map[string]any{
		"status":  resp.StatusCode,
		"headers": resp.Header,
		"body":    responseJSON,
	}, nil
}
