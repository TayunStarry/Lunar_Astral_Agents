package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 配置常量
const (
	ServerPort            = "36789"
	ServerURL             = "https://localhost:" + ServerPort
	SystemKey             = "key-520-1314-2000-02-18"
	MultimodalModel       = "system-multimodal"
	EmbeddingModel        = "system-embedding"
	VisionPrompt          = "请仅输出逗号分隔的标签，并尽可能准确的描述图片内容，这些标签将用于表情包的识别与响应, 标签的描述内容需包括情绪, 表情, 动作, 文字, 场景, 衣物等信息。"
	OptimizePrompt        = "我有一张图片，之前对它有过以下描述：\"%s\"。现在重新观察图片后，得到新的描述：\"%s\"。请你结合这两次描述，输出一个更加全面、准确、完善的逗号分隔的标签描述，保留有价值的信息，修正可能的错误，优化表达方式。"
	MetaVersion           = "25.1230"
	KnowledgeFileOnServer = "knowledge/meme_model.json"
	DefaultImageDir       = "../../meme-images"
)

// InferencePayload 推理请求负载
type InferencePayload struct {
	Model    string        `json:"model"`
	Messages []interface{} `json:"messages,omitempty"`
	Input    interface{}   `json:"input,omitempty"`
	Stream   bool          `json:"stream"`
}

// TextMessage 纯文本消息
type TextMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// MultimodalMessage 图文混合消息
type MultimodalMessage struct {
	Role    string        `json:"role"`
	Content []interface{} `json:"content"`
}

// ImageContent 图片内容块
type ImageContent struct {
	Type     string `json:"type"`
	ImageURL struct {
		URL string `json:"url"`
	} `json:"image_url"`
}

// TextContent 文本内容块
type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// 历史消息相关结构体
type HistoryMessage struct {
	Role        string    `json:"role"`
	Content     string    `json:"content"`
	IsPrompt    bool      `json:"isPrompt"`
	NoRender    bool      `json:"noRender"`
	ImageURL    *string   `json:"imageUrl"`
	Deletable   *bool     `json:"deletable"`
	UUID        string    `json:"uuid"`
	EmbedVector []float64 `json:"embedVector"`
}

// HistoryDocument 历史会话导出文档结构
type HistoryDocument struct {
	Meta    Meta             `json:"meta"`
	History []HistoryMessage `json:"history"`
}

type Meta struct {
	ExportedAt string `json:"exportedAt"`
	Version    string `json:"version"`
}

// API响应结构体
type ChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type EmbeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

// SaveFileResponse 保存文件响应
type SaveFileResponse struct {
	Filename string `json:"filename"`
}

// Config 配置信息
type Config struct {
	ImageDir string // 图片目录路径
}

func toBtoaString(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func calculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	hashBytes := hash.Sum(nil)
	fullHash := fmt.Sprintf("%x", hashBytes)
	return fullHash[:16], nil
}

// 发送HTTP请求
func sendHTTPRequest(method, url string, headers map[string]string, body []byte) (*http.Response, error) {
	client := &http.Client{}
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}

	return resp, nil
}

// 上传图片到服务器
func saveImageToServer(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %v", err)
	}
	defer file.Close()

	// 计算文件哈希
	fileHash, err := calculateFileHash(filePath)
	if err != nil {
		return "", fmt.Errorf("计算哈希失败: %v", err)
	}

	// 获取文件扩展名
	ext := strings.ToLower(filepath.Ext(filePath))
	newFileName := fileHash + ext

	// 读取文件内容
	fileData, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %v", err)
	}

	// 构建请求头
	headers := map[string]string{
		"X-File-Name": toBtoaString("resources/images/" + newFileName),
		"X-Overwrite": "true",
	}

	// 发送请求
	resp, err := sendHTTPRequest("POST", ServerURL+"/save", headers, fileData)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("上传失败: %s", string(body))
	}

	// 读取响应
	var result SaveFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	return "/read/resources/images/" + newFileName, nil
}

// 调用视觉模型
func callVisionModel(imageURL string, prompt string) (string, error) {
	// 构建图片内容
	imageContent := ImageContent{
		Type: "image_url",
		ImageURL: struct {
			URL string `json:"url"`
		}{
			URL: ServerURL + imageURL,
		},
	}

	// 构建文本内容
	textContent := TextContent{
		Type: "text",
		Text: prompt,
	}

	// 构建多模态消息
	multimodalMsg := MultimodalMessage{
		Role:    "user",
		Content: []interface{}{imageContent, textContent},
	}

	// 构建请求负载
	payload := InferencePayload{
		Model:    MultimodalModel,
		Messages: []interface{}{multimodalMsg},
		Stream:   false,
	}

	// 转换为JSON
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("序列化请求体失败: %v", err)
	}

	// 设置请求头
	headers := map[string]string{
		"Authorization": "Bearer " + SystemKey,
		"Content-Type":  "application/json",
	}

	// 发送请求
	resp, err := sendHTTPRequest("POST", ServerURL+"/v1/chat/completions", headers, payloadJSON)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("服务器返回错误: %d, 响应: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var chatResp ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("模型返回空响应")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// 调用视觉模型进行描述优化
func callVisionModelOptimize(imageURL string, previousDesc string, newDesc string) (string, error) {
	// 构建优化提示
	optimizePrompt := fmt.Sprintf(OptimizePrompt, previousDesc, newDesc)
	return callVisionModel(imageURL, optimizePrompt)
}

// 调用嵌入模型
func callEmbeddingModel(text string) ([]float64, error) {
	// 限制文本长度
	maxLength := 2000
	if len(text) > maxLength {
		text = text[:maxLength]
	}

	// 构建请求负载
	payload := InferencePayload{
		Model:  EmbeddingModel,
		Input:  []string{text},
		Stream: false,
	}

	// 转换为JSON
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化请求体失败: %v", err)
	}

	// 设置请求头
	headers := map[string]string{
		"Authorization": "Bearer " + SystemKey,
		"Content-Type":  "application/json",
	}

	// 发送请求
	resp, err := sendHTTPRequest("POST", ServerURL+"/v1/embeddings", headers, payloadJSON)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("服务器返回错误: %d, 响应: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var embedResp EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	if len(embedResp.Data) == 0 || len(embedResp.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("嵌入模型返回空向量")
	}

	// 截取前256个维度
	embedding := embedResp.Data[0].Embedding
	if len(embedding) > 256 {
		embedding = embedding[:256]
	}

	return embedding, nil
}

// 生成UUID
func generateUUID() string {
	timestamp := time.Now().UnixNano()
	hash := sha256.Sum256([]byte(fmt.Sprintf("%d-%s", timestamp, SystemKey)))
	return fmt.Sprintf("%x", hash)[:32]
}

// 从服务器读取现有的知识库文件
func readExistingKnowledge() (*HistoryDocument, map[string]*HistoryMessage, error) {
	// 构建请求URL
	reqURL := fmt.Sprintf("%s/read/%s", ServerURL, KnowledgeFileOnServer)

	// 发送GET请求
	client := &http.Client{}
	resp, err := client.Get(reqURL)
	if err != nil {
		// 如果网络错误，返回空文档
		return &HistoryDocument{
			Meta: Meta{
				ExportedAt: time.Now().Format("2006.01.02-15:04:05"),
				Version:    MetaVersion,
			},
			History: []HistoryMessage{},
		}, make(map[string]*HistoryMessage), nil
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		// 如果文件不存在，返回空文档
		if resp.StatusCode == http.StatusNotFound {
			return &HistoryDocument{
				Meta: Meta{
					ExportedAt: time.Now().Format("2006.01.02-15:04:05"),
					Version:    MetaVersion,
				},
				History: []HistoryMessage{},
			}, make(map[string]*HistoryMessage), nil
		}
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("服务器返回错误: %d, 响应: %s", resp.StatusCode, string(body))
	}

	// 读取响应体
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("读取响应失败: %v", err)
	}

	// 解析JSON
	var doc HistoryDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, nil, fmt.Errorf("解析JSON失败: %v", err)
	}

	// 构建图片URL到消息的映射
	imageToMessage := make(map[string]*HistoryMessage)
	for i := range doc.History {
		if doc.History[i].ImageURL != nil {
			imageToMessage[*doc.History[i].ImageURL] = &doc.History[i]
		}
	}

	return &doc, imageToMessage, nil
}

// 保存知识库文件到服务器
func saveKnowledgeFile(history []HistoryMessage) error {
	// 构建文档
	doc := HistoryDocument{
		Meta: Meta{
			ExportedAt: time.Now().Format("2006.01.02-15:04:05"),
			Version:    MetaVersion,
		},
		History: history,
	}

	// 转换为JSON
	jsonData, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("序列化JSON失败: %v", err)
	}

	// 设置请求头
	headers := map[string]string{
		"X-File-Name":  toBtoaString(KnowledgeFileOnServer),
		"X-Overwrite":  "true",
		"Content-Type": "application/json",
	}

	// 发送请求
	resp, err := sendHTTPRequest("POST", ServerURL+"/save", headers, jsonData)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("服务器返回错误: %d, 响应: %s", resp.StatusCode, string(body))
	}

	// 读取响应
	var result SaveFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
	}

	return nil
}

// 测试服务器端点
func testServerEndpoints() bool {
	endpoints := []struct {
		path    string
		method  string
		headers map[string]string
		body    []byte
	}{
		{
			path:    "/read/" + KnowledgeFileOnServer,
			method:  "GET",
			headers: map[string]string{},
			body:    nil,
		},
	}

	client := &http.Client{Timeout: 5 * time.Second}

	for _, endpoint := range endpoints {
		var resp *http.Response
		var err error

		if endpoint.method == "GET" {
			resp, err = client.Get(ServerURL + endpoint.path)
		} else {
			req, err2 := http.NewRequest(endpoint.method, ServerURL+endpoint.path, bytes.NewReader(endpoint.body))
			if err2 != nil {
				continue
			}

			for key, value := range endpoint.headers {
				req.Header.Set(key, value)
			}

			resp, err = client.Do(req)
		}

		if err != nil {
			continue
		}

		resp.Body.Close()
	}

	return true
}

// 打印分隔符
func printSeparator(symbol string, length int) {
	fmt.Println(strings.Repeat(symbol, length))
}

// 打印图片处理头
func printImageHeader(index, total int, fileName string) {
	printSeparator("═", 80)
	fmt.Printf("┌─ 图片处理 [%d/%d] %s\n", index, total, fileName)
	printSeparator("─", 80)
}

// 打印图片处理结果
func printImageResult(success bool, operationType string, modelCalls int, vectorDim int) {
	status := "✅ 成功"
	if !success {
		status = "❌ 失败"
	}

	fmt.Printf("├─ 处理结果: %s | 操作类型: %s | 模型调用: %d次 | 向量维度: %d\n",
		status, operationType, modelCalls, vectorDim)
}

// 处理单张图片
func processImage(imageURL string, existingDesc *string) (*HistoryMessage, error) {
	var finalDesc string
	var modelCalls int
	var operationType string
	var previousDesc string
	var newDesc string // 先声明变量

	// 如果已有描述，执行优化流程
	if existingDesc != nil && *existingDesc != "" {
		operationType = "优化更新"
		previousDesc = *existingDesc

		// 1. 生成新描述
		generatedDesc, err := callVisionModel(imageURL, VisionPrompt)
		if err != nil {
			return nil, fmt.Errorf("视觉模型调用失败: %v", err)
		}
		newDesc = generatedDesc // 赋值给外部变量
		modelCalls++

		// 2. 结合新旧描述生成优化描述
		optimizedDesc, err := callVisionModelOptimize(imageURL, previousDesc, newDesc)
		if err != nil {
			// 如果优化失败，使用新描述
			finalDesc = newDesc
		} else {
			finalDesc = optimizedDesc
			modelCalls++
		}
	} else {
		// 没有已有描述，直接生成新描述
		operationType = "新增"
		generatedDesc, err := callVisionModel(imageURL, VisionPrompt)
		if err != nil {
			return nil, fmt.Errorf("视觉模型调用失败: %v", err)
		}
		newDesc = generatedDesc
		finalDesc = newDesc
		modelCalls = 1
	}

	// 3. 生成向量
	embedding, err := callEmbeddingModel(finalDesc)
	if err != nil {
		return nil, fmt.Errorf("嵌入模型调用失败: %v", err)
	}

	// 4. 构建消息
	deletable := false
	msg := HistoryMessage{
		Role:        "assistant",
		Content:     finalDesc,
		IsPrompt:    false,
		NoRender:    false,
		ImageURL:    &imageURL,
		Deletable:   &deletable,
		UUID:        generateUUID(),
		EmbedVector: embedding,
	}

	// 打印详细信息
	if operationType == "优化更新" {
		printDescription("原始描述", previousDesc)
		printDescription("新生成描述", newDesc) // 现在newDesc有值了
		printDescription("优化后描述", finalDesc)
	} else {
		printDescription("生成描述", finalDesc)
	}

	printImageResult(true, operationType, modelCalls, len(embedding))
	printSeparator("─", 80)

	return &msg, nil
}

// 分割长字符串 - 修复中文乱码问题
func splitString(s string, length int) []string {
	var result []string
	// 将字符串转换为rune数组，支持中文字符
	runes := []rune(s)
	for i := 0; i < len(runes); i += length {
		end := i + length
		if end > len(runes) {
			end = len(runes)
		}
		result = append(result, string(runes[i:end]))
	}
	return result
}

// 打印描述内容 - 修复中文乱码问题
func printDescription(label, desc string) {
	// 将描述转换为rune数组，确保中文字符计数正确
	runeDesc := []rune(desc)
	// 截断描述到256个字符（不是字节）
	if len(runeDesc) > 256 {
		desc = string(runeDesc[:253]) + "..."
	} else {
		desc = string(runeDesc)
	}

	fmt.Printf("├─ %s:\n", label)
	lines := splitString(desc, 70)
	for _, line := range lines {
		fmt.Printf("│   %s\n", line)
	}
}

// 扫描图片目录
func scanImageDir(imageDir string) ([]string, error) {
	var imageFiles []string
	imageExts := map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".gif":  true,
		".bmp":  true,
		".webp": true,
	}

	err := filepath.Walk(imageDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			ext := strings.ToLower(filepath.Ext(path))
			if imageExts[ext] {
				imageFiles = append(imageFiles, path)
			}
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("扫描目录失败: %v", err)
	}

	return imageFiles, nil
}

// 解析命令行参数
func parseConfig() *Config {
	config := &Config{
		ImageDir: DefaultImageDir,
	}

	if len(os.Args) >= 2 {
		config.ImageDir = os.Args[1]
	}

	return config
}

// 主函数
func main() {
	fmt.Println("")
	printSeparator("═", 80)
	fmt.Println("                          📷 表情包模型训练器")
	printSeparator("═", 80)
	fmt.Println("")

	// 解析配置
	config := parseConfig()

	// 显示配置信息
	fmt.Println("┌─ 系统配置")
	printSeparator("─", 80)
	fmt.Printf("│ 图片目录: %s\n", config.ImageDir)
	fmt.Printf("│ 服务器地址: %s\n", ServerURL)
	fmt.Printf("│ 知识库文件: %s\n", KnowledgeFileOnServer)
	fmt.Println("└──────────────────────────────────────────────────────────────────")
	fmt.Println("")

	// 检查图片目录是否存在
	if _, err := os.Stat(config.ImageDir); os.IsNotExist(err) {
		fmt.Println("❌ 错误: 目录不存在:", config.ImageDir)
		fmt.Println("💡 提示: 请指定正确的图片目录，或创建默认目录: mkdir -p", DefaultImageDir)
		os.Exit(1)
	}

	// 测试服务器端点
	fmt.Print("🔍 测试服务器连接... ")
	if testServerEndpoints() {
		fmt.Println("✅")
	} else {
		fmt.Println("❌")
		fmt.Println("⚠️ 警告: 服务器连接测试失败，但将继续尝试")
	}
	fmt.Println("")

	// 读取现有的知识库
	fmt.Println("┌─ 知识库信息")
	printSeparator("─", 80)
	existingDoc, imageToMessage, err := readExistingKnowledge()
	if err != nil {
		fmt.Printf("│ ❌ 读取失败: %v\n", err)
		fmt.Println("│ 📝 将创建新的知识库")
		existingDoc = &HistoryDocument{
			Meta: Meta{
				ExportedAt: time.Now().Format("2006.01.02-15:04:05"),
				Version:    MetaVersion,
			},
			History: []HistoryMessage{},
		}
		imageToMessage = make(map[string]*HistoryMessage)
	} else {
		fmt.Printf("│ ✅ 读取成功\n")
	}
	fmt.Printf("│ 现有记录数: %d\n", len(existingDoc.History))
	fmt.Printf("│ 图片映射数: %d\n", len(imageToMessage))
	fmt.Println("└──────────────────────────────────────────────────────────────────")
	fmt.Println("")

	// 扫描图片目录
	fmt.Println("┌─ 图片扫描")
	printSeparator("─", 80)
	imageFiles, err := scanImageDir(config.ImageDir)
	if err != nil {
		fmt.Printf("│ ❌ 扫描失败: %v\n", err)
		os.Exit(1)
	}

	if len(imageFiles) == 0 {
		fmt.Println("│ ❌ 未找到图片文件")
		fmt.Println("│ 💡 支持的格式: .jpg, .jpeg, .png, .gif, .bmp, .webp")
		os.Exit(1)
	}

	fmt.Printf("│ ✅ 发现 %d 张图片\n", len(imageFiles))
	fmt.Println("└──────────────────────────────────────────────────────────────────")
	fmt.Println("")

	// 准备消息列表
	allMessages := make([]HistoryMessage, 0, len(existingDoc.History)+len(imageFiles))
	allMessages = append(allMessages, existingDoc.History...)

	// 初始化统计
	totalImages := len(imageFiles)
	successCount := 0
	failCount := 0
	newCount := 0
	optimizedCount := 0

	fmt.Println("🚀 开始图片处理流程")
	fmt.Println("")

	// 处理每张图片
	for i, imagePath := range imageFiles {
		fileName := filepath.Base(imagePath)
		printImageHeader(i+1, totalImages, fileName)

		// 上传图片
		fmt.Print("├─ 上传图片... ")
		imageURL, err := saveImageToServer(imagePath)
		if err != nil {
			fmt.Printf("❌ 失败: %v\n", err)
			printImageResult(false, "失败", 0, 0)
			printSeparator("─", 80)
			failCount++
			continue
		}
		fmt.Println("✅")

		// 检查是否已有记录
		var existingDesc *string
		var existingMsg *HistoryMessage
		if msg, exists := imageToMessage[imageURL]; exists {
			existingDesc = &msg.Content
			existingMsg = msg
			fmt.Printf("├─ 状态: 🔄 已存在 (UUID: %s)\n", msg.UUID[:8])
		} else {
			fmt.Println("├─ 状态: 🆕 新图片")
		}

		// 处理图片
		msg, err := processImage(imageURL, existingDesc)
		if err != nil {
			fmt.Printf("├─ ❌ 处理失败: %v\n", err)
			printImageResult(false, "失败", 0, 0)
			printSeparator("─", 80)
			failCount++
			continue
		}

		successCount++

		// 更新或添加记录
		if existingMsg != nil {
			// 替换现有记录
			for j := range allMessages {
				if allMessages[j].UUID == existingMsg.UUID {
					allMessages[j] = *msg
					optimizedCount++
					break
				}
			}
		} else {
			// 添加新记录
			allMessages = append(allMessages, *msg)
			newCount++
		}

		// 短暂延迟
		if i < totalImages-1 {
			fmt.Println("")
			time.Sleep(300 * time.Millisecond)
		}
	}

	// 统计信息
	fmt.Println("")
	printSeparator("═", 80)
	fmt.Println("                          📊 处理结果统计")
	printSeparator("═", 80)

	// 创建统计表格
	stats := []struct {
		label string
		value interface{}
		icon  string
	}{
		{"总图片数", totalImages, "📁"},
		{"成功处理", successCount, "✅"},
		{"处理失败", failCount, "❌"},
		{"新增记录", newCount, "🆕"},
		{"优化更新", optimizedCount, "🔄"},
		{"总记录数", len(allMessages), "📋"},
	}

	for _, stat := range stats {
		fmt.Printf("│ %s %-10s: %v\n", stat.icon, stat.label, stat.value)
	}

	printSeparator("─", 80)

	if successCount == 0 {
		fmt.Println("│ ⚠️ 警告: 没有成功处理的图片")
		os.Exit(1)
	}

	fmt.Println("")

	// 保存知识库
	fmt.Println("┌─ 保存知识库")
	printSeparator("─", 80)
	fmt.Print("│ 保存到服务器... ")
	if err := saveKnowledgeFile(allMessages); err != nil {
		fmt.Printf("❌ 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅")
	fmt.Printf("│ 保存位置: %s\n", KnowledgeFileOnServer)
	fmt.Println("└──────────────────────────────────────────────────────────────────")
	fmt.Println("")

	// 完成信息
	printSeparator("═", 80)
	fmt.Println("                          🎉 处理完成!")
	fmt.Printf("                          完成时间: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	printSeparator("═", 80)
	fmt.Println("")
}
