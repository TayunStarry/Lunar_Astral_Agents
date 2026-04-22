package context

import (
	"LunarCore/browser"
	"LunarCore/config"
	"LunarCore/hierarchy"
	"LunarCore/hierarchy/image"
	"LunarCore/hierarchy/image/generate"
	"LunarCore/hierarchy/memory"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// IPInfo 存储IP地址信息
type IPInfo struct {
	Region string `json:"region"`
	City   string `json:"city"`
}

// SaveFileAdapter 适配TypeScript调用的文件保存功能，支持字符串、字节数组及Blob/File类型数据
func SaveFileAdapter(args ...any) (string, string, error) {
	if len(args) < 3 {
		return "", "", fmt.Errorf("参数不足")
	}

	fileName, ok := args[0].(string)
	if !ok {
		return "", "", fmt.Errorf("fileName必须是字符串")
	}

	overwrite, ok := args[1].(bool)
	if !ok {
		overwrite = false
	}

	fileData := args[2]
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
			return "", "", fmt.Errorf("不支持的 Blob/File 数据格式")
		}
	default:
		return "", "", fmt.Errorf("不支持的文件数据类型")
	}

	return hierarchy.SaveFile(fileName, overwrite, reader)
}

// ReadFileAdapter 适配TypeScript调用的文件读取功能，返回文件内容、大小和MIME类型
func ReadFileAdapter(args ...any) ([]byte, int64, string, error) {
	if len(args) < 1 {
		return nil, 0, "", fmt.Errorf("参数不足")
	}

	filePath, ok := args[0].(string)
	if !ok {
		return nil, 0, "", fmt.Errorf("filePath必须是字符串")
	}

	file, size, mimeType, err := hierarchy.ReadFile(filePath)
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
func GetFileListAdapter(args ...any) ([]map[string]any, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("参数不足")
	}

	path, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("path必须是字符串")
	}

	fileList, err := hierarchy.GetFileList(path)
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
func ExecuteDatabaseRequestAdapter(args ...any) (map[string]any, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("参数不足")
	}

	request, ok := args[0].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("request必须是对象")
	}

	// 构建数据库请求
	dbRequest := memory.DatabaseRequest{}

	if transaction, ok := request["transaction"].(bool); ok {
		dbRequest.Transaction = transaction
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
func QueryCurrentAddressAdapter(args ...any) ([]string, error) {
	// 如果当前地址已缓存，直接返回
	if len(config.ServerAddress) > 0 {
		return config.ServerAddress, nil
	}

	// 从IP地址查询位置信息
	resp, err := http.Get("https://ipapi.co/json/")
	if err != nil {
		log.Printf("获取位置失败: %v\n", err)
		return []string{"江苏省", "南京市"}, err
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		log.Printf("获取位置失败: %s\n", resp.Status)
		return []string{"江苏省", "南京市"}, err
	}

	// 解析JSON响应
	var data IPInfo
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Printf("解析位置信息失败: %v\n", err)
		return []string{"江苏省", "南京市"}, err
	}

	// 确保省份和城市信息存在
	if data.Region == "" || data.City == "" {
		log.Println("获取位置失败: 省份或城市信息缺失")
		return []string{"江苏省", "南京市"}, err
	}
	// 缓存当前地址
	config.ServerAddress = []string{data.Region, data.City}
	return config.ServerAddress, nil
}

// GetSystemUrlAdapter 适配TypeScript调用的系统URL获取功能，返回系统访问地址
func GetSystemUrlAdapter(args ...any) (string, error) {
	ip, err := browser.GetLocalIP([]string{})
	if err != nil {
		log.Printf("获取本地IP失败: %v\n", err)
		return fmt.Sprintf("http://localhost:%d", *config.BasicPort), nil
	}
	return fmt.Sprintf("http://%s:%d", ip, *config.BasicPort), nil
}

// VideoKeyframeExtractionAdapter 适配TypeScript调用的视频关键帧提取功能，转换为TypeScript可处理的格式
func VideoKeyframeExtractionAdapter(args ...any) ([]map[string]any, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("参数不足")
	}

	inputFile, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("inputFile必须是字符串")
	}

	cacheDir, ok := args[1].(string)
	if !ok {
		return nil, fmt.Errorf("cacheDir必须是字符串")
	}

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
func ProxyFetchAdapter(args ...any) (map[string]any, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("参数不足")
	}

	config, ok := args[0].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("config必须是对象")
	}

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

// ResizeImageAdapter 适配TypeScript调用的图片缩放功能，处理图片数据并返回缩放结果
func ResizeImageAdapter(args ...any) (map[string]any, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("参数不足")
	}

	imgData := args[0]
	// 处理不同类型的图片数据
	var bytesData []byte

	switch data := imgData.(type) {
	case string:
		// 处理base64编码的图片
		if strings.HasPrefix(data, "data:image/") {
			// 移除base64头部
			data = strings.Split(data, ",")[1]
			var err error
			bytesData, err = base64.StdEncoding.DecodeString(data)
			if err != nil {
				return nil, fmt.Errorf("解码base64图片失败: %v", err)
			}
		} else {
			// 直接将字符串转换为字节数组
			bytesData = []byte(data)
		}
	case []byte:
		// 直接使用字节数组
		bytesData = data
	case map[string]any:
		// 处理goja中转换为map的Blob/File类型
		if buffer, ok := data["buffer"].([]byte); ok {
			bytesData = buffer
		} else if data, ok := data["data"].([]byte); ok {
			bytesData = data
		} else {
			return nil, fmt.Errorf("不支持的 Blob/File 数据格式")
		}
	default:
		return nil, fmt.Errorf("不支持的图片数据类型")
	}

	// 调用图片缩放函数
	return image.ResizeImage(bytesData)
}

// GenerateImageAdapter 适配TypeScript调用的图片生成功能，处理图片生成参数并返回结果
func GenerateImageAdapter(args ...any) (map[string]any, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("参数不足")
	}

	params, ok := args[0].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("params必须是对象")
	}

	// 提取参数
	prompt, ok := params["prompt"].(string)
	if !ok {
		return nil, fmt.Errorf("提示词不能为空")
	}

	negativePrompt := ""
	if np, ok := params["negativePrompt"].(string); ok {
		negativePrompt = np
	}

	batchSize := 1
	if bs, ok := params["batchSize"].(float64); ok {
		batchSize = int(bs)
	}

	width := 512
	if w, ok := params["width"].(float64); ok {
		width = int(w)
	}

	height := 512
	if h, ok := params["height"].(float64); ok {
		height = int(h)
	}

	steps := 20
	if s, ok := params["steps"].(float64); ok {
		steps = int(s)
	}

	strength := 0.7
	if st, ok := params["strength"].(float64); ok {
		strength = st
	}

	cfgScale := 7.5
	if cs, ok := params["cfgScale"].(float64); ok {
		cfgScale = cs
	}

	seed := int64(0)
	if sd, ok := params["seed"].(float64); ok {
		seed = int64(sd)
	}

	initImg := ""
	if ii, ok := params["initImg"].(string); ok {
		initImg = ii
	}

	// 调用图片生成函数
	return generate.GenerateImage(prompt, negativePrompt, batchSize, width, height, steps, strength, cfgScale, seed, initImg)
}


// LogAdapter 适配TypeScript调用的日志打印功能，使用Go的log模块打印字符串
func LogAdapter(args ...any) (any, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("参数不足")
	}

	message := args[0]
	var msg string
	switch m := message.(type) {
	case string:
		msg = m
	default:
		msg = fmt.Sprintf("%v", m)
	}

	// 使用Go的log模块打印消息
	log.Println(msg)
	return msg, nil
}
