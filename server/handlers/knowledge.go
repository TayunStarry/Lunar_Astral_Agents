package handlers

import (
	"Lunar-Astral-Agents/server/config" // 导入项目配置包，用于获取配置信息
	"encoding/json"                     // JSON编码/解码包，用于处理JSON数据
	"io"                                // 输入/输出包，用于处理IO操作
	"math"                              // 数学包，用于数学计算
	"net/http"                          // HTTP协议包，用于处理HTTP请求/响应
	"os"                                // 文件操作包，用于文件读写等操作
	"path/filepath"                     // 文件路径操作包，用于处理文件路径
	"strings"                           // 字符串操作包，用于字符串处理
	"sync"                              // 同步包，用于并发编程
	"time"                              // 时间包，用于处理时间
)

// 定义全局缓存结构，用于存储待写入和待删除的消息
var (
	knowledgeWriteCache  = make(map[string][]HistoryMessage) // key: 文件路径, value: 待写入的消息列表
	knowledgeDeleteCache = make(map[string]map[string]bool)  // key: 文件路径, value: 待删除的UUID集合（map实现set）
	knowledgeCacheMutex  = &sync.RWMutex{}                   // 保护缓存的互斥锁
)

// 定义请求和响应的数据结构
type KnowledgeQueryRequest struct {
	FilePath    string    `json:"filePath"`
	QueryVector []float64 `json:"queryVector"`
	TopK        int       `json:"topK"`
}

type KnowledgeWriteRequest struct {
	FilePath string         `json:"filePath"`
	Message  HistoryMessage `json:"message"`
}

type KnowledgeDeleteRequest struct {
	FilePath string `json:"filePath"`
	UUID     string `json:"uuid"`
}

type HistoryMessage struct {
	Role        string    `json:"role"`
	Content     string    `json:"content"`
	IsPrompt    bool      `json:"isPrompt"`
	NoRender    bool      `json:"noRender"`
	ImageUrl    string    `json:"imageUrl"`
	Deletable   bool      `json:"deletable"`
	UUID        string    `json:"uuid"`
	EmbedVector []float64 `json:"embedVector"`
}

type KnowledgeMessage struct {
	Role     string `json:"role"`
	Content  string `json:"content"`
	ImageUrl string `json:"imageUrl"`
	UUID     string `json:"uuid"`
}

type WeightedKnowledgeMessage struct {
	Message  KnowledgeMessage `json:"message"`
	Weighted float64          `json:"weighted"`
}

type HistoryDocument struct {
	Meta struct {
		ExportedAt string `json:"exportedAt"`
		Version    string `json:"version"`
	} `json:"meta"`
	History []HistoryMessage `json:"history"`
}

// KnowledgeQueryHandler 处理知识库查询请求
func KnowledgeQueryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "KnowledgeQuery请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	// 解析请求体
	var req KnowledgeQueryRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "KnowledgeQuery请求[ERROR] -> 读取请求体失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	err = json.Unmarshal(body, &req)
	if err != nil {
		http.Error(w, "KnowledgeQuery请求[ERROR] -> 解析请求体失败", http.StatusBadRequest)
		return
	}

	// 验证请求参数
	if req.FilePath == "" {
		http.Error(w, "KnowledgeQuery请求[ERROR] -> 缺少文件路径", http.StatusBadRequest)
		return
	}

	if len(req.QueryVector) != 256 {
		http.Error(w, "KnowledgeQuery请求[ERROR] -> queryVector长度必须为256", http.StatusBadRequest)
		return
	}

	if req.TopK <= 0 {
		req.TopK = 15
	}

	// 读取知识库文件
	knowledgePath := filepath.Join(config.LocalDir, req.FilePath)
	if !strings.HasPrefix(knowledgePath, filepath.Clean(config.LocalDir)) {
		http.Error(w, "KnowledgeQuery请求[ERROR] -> 访问被拒绝", http.StatusForbidden)
		return
	}

	var doc HistoryDocument

	// 尝试打开文件
	file, openErr := os.Open(knowledgePath)
	if openErr != nil {
		// 文件不存在，初始化文件
		if os.IsNotExist(openErr) {
			doc, err = initializeKnowledgeFile(knowledgePath)
			if err != nil {
				http.Error(w, "KnowledgeQuery请求[ERROR] -> 初始化文件失败", http.StatusInternalServerError)
				return
			}
		} else {
			// 其他错误
			http.Error(w, "KnowledgeQuery请求[ERROR] -> 打开文件失败", http.StatusInternalServerError)
			return
		}
	} else {
		// 文件存在，读取文件
		defer file.Close()
		if err := json.NewDecoder(file).Decode(&doc); err != nil {
			http.Error(w, "KnowledgeQuery请求[ERROR] -> 解析文件失败", http.StatusInternalServerError)
			return
		}
	}

	// 计算相似度并排序
	var results []WeightedKnowledgeMessage
	for _, msg := range doc.History {
		if len(msg.EmbedVector) == 0 {
			continue // 跳过没有嵌入向量的消息
		}

		// 计算余弦相似度
		similarity := calculateCosineSimilarity(req.QueryVector, msg.EmbedVector)

		// 转换为KnowledgeMessage
		knowledgeMsg := KnowledgeMessage{
			Role:     msg.Role,
			Content:  msg.Content,
			ImageUrl: msg.ImageUrl,
			UUID:     msg.UUID,
		}

		results = append(results, WeightedKnowledgeMessage{
			Message:  knowledgeMsg,
			Weighted: similarity,
		})
	}

	// 按相似度降序排序
	quickSort(results, 0, len(results)-1)

	// 返回topK个结果
	if len(results) > req.TopK {
		results = results[:req.TopK]
	}

	// 提取纯KnowledgeMessage数组，不包含权重
	pureResults := []KnowledgeMessage{}
	for _, item := range results {
		pureResults = append(pureResults, item.Message)
	}

	// 返回结果
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pureResults)
}

// KnowledgeWriteHandler 处理知识库写入请求 - 只将消息缓存到内存中
func KnowledgeWriteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "KnowledgeWrite请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	// 解析请求体
	var req KnowledgeWriteRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "KnowledgeWrite请求[ERROR] -> 读取请求体失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, "KnowledgeWrite请求[ERROR] -> 解析请求体失败", http.StatusBadRequest)
		return
	}

	// 验证请求参数
	if req.FilePath == "" {
		http.Error(w, "KnowledgeWrite请求[ERROR] -> 缺少文件路径", http.StatusBadRequest)
		return
	}

	// 验证文件路径安全性
	knowledgePath := filepath.Join(config.LocalDir, req.FilePath)
	if !strings.HasPrefix(knowledgePath, filepath.Clean(config.LocalDir)) {
		http.Error(w, "KnowledgeWrite请求[ERROR] -> 访问被拒绝", http.StatusForbidden)
		return
	}

	newMsg := req.Message
	// 如果嵌入向量为空，设置为空切片（将由客户端生成）
	if newMsg.EmbedVector == nil {
		newMsg.EmbedVector = []float64{}
	}

	// 将消息缓存到内存中
	knowledgeCacheMutex.Lock()
	if _, exists := knowledgeWriteCache[knowledgePath]; !exists {
		knowledgeWriteCache[knowledgePath] = []HistoryMessage{}
	}

	// 检查缓存中是否已有相同UUID的消息
	hasDuplicate := false
	for _, msg := range knowledgeWriteCache[knowledgePath] {
		if msg.UUID == newMsg.UUID {
			hasDuplicate = true
			break
		}
	}

	if !hasDuplicate {
		knowledgeWriteCache[knowledgePath] = append(knowledgeWriteCache[knowledgePath], newMsg)
	}
	knowledgeCacheMutex.Unlock()

	if hasDuplicate {
		// 内容已在缓存中，不重复添加
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "内容已存在于缓存中，不重复添加"})
	} else {
		// 返回成功响应
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "消息已缓存，等待写入文件"})
	}
}

// KnowledgeFlushHandler 处理知识库刷新请求 - 接收哨兵信号，将缓存的消息写入文件并执行删除操作
func KnowledgeFlushHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "KnowledgeFlush请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	// 解析请求体
	type KnowledgeFlushRequest struct {
		FilePath string `json:"filePath"`
	}

	var req KnowledgeFlushRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "KnowledgeFlush请求[ERROR] -> 读取请求体失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, "KnowledgeFlush请求[ERROR] -> 解析请求体失败", http.StatusBadRequest)
		return
	}

	// 验证请求参数
	if req.FilePath == "" {
		http.Error(w, "KnowledgeFlush请求[ERROR] -> 缺少文件路径", http.StatusBadRequest)
		return
	}

	// 验证文件路径安全性
	knowledgePath := filepath.Join(config.LocalDir, req.FilePath)
	if !strings.HasPrefix(knowledgePath, filepath.Clean(config.LocalDir)) {
		http.Error(w, "KnowledgeFlush请求[ERROR] -> 访问被拒绝", http.StatusForbidden)
		return
	}

	// 从缓存中获取待写入的消息和待删除的UUID
	knowledgeCacheMutex.Lock()
	messagesToWrite, writeExists := knowledgeWriteCache[knowledgePath]
	deleteUUIDs, deleteExists := knowledgeDeleteCache[knowledgePath]

	// 准备返回的统计信息
	messagesInCache := 0
	messagesToDelete := 0

	if writeExists {
		messagesInCache = len(messagesToWrite)
		// 清空写入缓存
		delete(knowledgeWriteCache, knowledgePath)
	}

	if deleteExists {
		messagesToDelete = len(deleteUUIDs)
		// 清空删除缓存
		delete(knowledgeDeleteCache, knowledgePath)
	}

	// 如果没有待处理的消息，直接返回
	if !writeExists && !deleteExists {
		knowledgeCacheMutex.Unlock()
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "没有待处理的消息",
		})
		return
	}

	// 复制待处理的数据，释放锁
	var messagesToWriteCopy []HistoryMessage
	if writeExists {
		messagesToWriteCopy = make([]HistoryMessage, len(messagesToWrite))
		copy(messagesToWriteCopy, messagesToWrite)
	}

	var deleteUUIDsCopy map[string]bool
	if deleteExists {
		deleteUUIDsCopy = make(map[string]bool)
		for uuid := range deleteUUIDs {
			deleteUUIDsCopy[uuid] = true
		}
	}
	knowledgeCacheMutex.Unlock()

	// 读取知识库文件
	var doc HistoryDocument
	if _, err = os.Stat(knowledgePath); err == nil {
		file, fileErr := os.Open(knowledgePath)
		if fileErr != nil {
			http.Error(w, "KnowledgeFlush请求[ERROR] -> 打开文件失败", http.StatusInternalServerError)
			return
		}
		if err = json.NewDecoder(file).Decode(&doc); err != nil {
			http.Error(w, "KnowledgeFlush请求[ERROR] -> 解析文件失败", http.StatusInternalServerError)
			return
		}
		file.Close()
	} else {
		// 文件不存在，初始化文件
		doc, err = initializeKnowledgeFile(knowledgePath)
		if err != nil {
			http.Error(w, "KnowledgeFlush请求[ERROR] -> 初始化文件失败", http.StatusInternalServerError)
			return
		}
	}

	// 1. 先应用删除操作
	messagesDeleted := 0
	if len(deleteUUIDsCopy) > 0 {
		newHistory := []HistoryMessage{}
		for _, msg := range doc.History {
			if !deleteUUIDsCopy[msg.UUID] {
				newHistory = append(newHistory, msg)
			} else {
				messagesDeleted++
			}
		}
		doc.History = newHistory
	}

	// 2. 再合并写入消息，去重（使用UUID作为唯一标识）
	uuidMap := make(map[string]bool)
	for _, msg := range doc.History {
		uuidMap[msg.UUID] = true
	}

	newMessagesAdded := 0
	if len(messagesToWriteCopy) > 0 {
		for _, msg := range messagesToWriteCopy {
			if !uuidMap[msg.UUID] {
				doc.History = append(doc.History, msg)
				uuidMap[msg.UUID] = true
				newMessagesAdded++
			}
		}
	}

	// 写入文件
	fileLock := getFileLock(knowledgePath)
	fileLock.Lock()
	defer fileLock.Unlock()

	file, err := os.Create(knowledgePath)
	if err != nil {
		http.Error(w, "KnowledgeFlush请求[ERROR] -> 创建文件失败", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(doc); err != nil {
		http.Error(w, "KnowledgeFlush请求[ERROR] -> 写入文件失败", http.StatusInternalServerError)
		return
	}

	// 返回成功响应
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":          "刷新成功",
		"messagesWritten":  newMessagesAdded,
		"messagesInCache":  messagesInCache,
		"messagesDeleted":  messagesDeleted,
		"messagesToDelete": messagesToDelete,
	})
}

// KnowledgeListHandler 处理知识库列表请求 - 返回知识库下所有条目的SmallHistoryMessage数组
func KnowledgeListHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "KnowledgeList请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	// 解析请求体
	type KnowledgeListRequest struct {
		FilePath string `json:"filePath"`
	}

	var req KnowledgeListRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "KnowledgeList请求[ERROR] -> 读取请求体失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, "KnowledgeList请求[ERROR] -> 解析请求体失败", http.StatusBadRequest)
		return
	}

	// 验证请求参数
	if req.FilePath == "" {
		http.Error(w, "KnowledgeList请求[ERROR] -> 缺少文件路径", http.StatusBadRequest)
		return
	}

	// 验证文件路径安全性
	knowledgePath := filepath.Join(config.LocalDir, req.FilePath)
	if !strings.HasPrefix(knowledgePath, filepath.Clean(config.LocalDir)) {
		http.Error(w, "KnowledgeList请求[ERROR] -> 访问被拒绝", http.StatusForbidden)
		return
	}

	// 读取知识库文件
	var doc HistoryDocument
	var fileErr error

	// 尝试打开文件
	file, openErr := os.Open(knowledgePath)
	if openErr != nil {
		// 文件不存在，初始化文件
		if os.IsNotExist(openErr) {
			doc, fileErr = initializeKnowledgeFile(knowledgePath)
			if fileErr != nil {
				http.Error(w, "KnowledgeList请求[ERROR] -> 初始化文件失败", http.StatusInternalServerError)
				return
			}
		} else {
			// 其他错误
			http.Error(w, "KnowledgeList请求[ERROR] -> 打开文件失败", http.StatusInternalServerError)
			return
		}
	} else {
		// 文件存在，读取文件
		if fileErr = json.NewDecoder(file).Decode(&doc); fileErr != nil {
			http.Error(w, "KnowledgeList请求[ERROR] -> 解析文件失败", http.StatusInternalServerError)
			return
		}
		file.Close()
	}

	results := []HistoryMessage{}
	for _, msg := range doc.History {
		smallMsg := HistoryMessage{
			Role:        msg.Role,
			Content:     msg.Content,
			IsPrompt:    msg.IsPrompt,
			NoRender:    msg.NoRender,
			ImageUrl:    msg.ImageUrl,
			Deletable:   msg.Deletable,
			UUID:        msg.UUID,
			EmbedVector: []float64{}, // 始终为空数组，减少数据体积
		}
		results = append(results, smallMsg)
	}

	// 返回结果
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// KnowledgeDeleteHandler 处理知识库删除请求 - 只将删除请求缓存到内存中
func KnowledgeDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "KnowledgeDelete请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	// 解析请求体
	var req KnowledgeDeleteRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "KnowledgeDelete请求[ERROR] -> 读取请求体失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, "KnowledgeDelete请求[ERROR] -> 解析请求体失败", http.StatusBadRequest)
		return
	}

	// 验证请求参数
	if req.FilePath == "" {
		http.Error(w, "KnowledgeDelete请求[ERROR] -> 缺少文件路径", http.StatusBadRequest)
		return
	}

	if req.UUID == "" {
		http.Error(w, "KnowledgeDelete请求[ERROR] -> 缺少UUID", http.StatusBadRequest)
		return
	}

	// 验证文件路径安全性
	knowledgePath := filepath.Join(config.LocalDir, req.FilePath)
	if !strings.HasPrefix(knowledgePath, filepath.Clean(config.LocalDir)) {
		http.Error(w, "KnowledgeDelete请求[ERROR] -> 访问被拒绝", http.StatusForbidden)
		return
	}

	// 将删除请求缓存到内存中
	knowledgeCacheMutex.Lock()
	if _, exists := knowledgeDeleteCache[knowledgePath]; !exists {
		knowledgeDeleteCache[knowledgePath] = make(map[string]bool)
	}

	// 检查缓存中是否已有相同的删除请求
	hasDuplicate := knowledgeDeleteCache[knowledgePath][req.UUID]
	if !hasDuplicate {
		knowledgeDeleteCache[knowledgePath][req.UUID] = true
	}
	knowledgeCacheMutex.Unlock()

	if hasDuplicate {
		// 已存在相同的删除请求，不重复添加
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "删除请求已存在于缓存中，不重复添加"})
	} else {
		// 返回成功响应
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "删除请求已缓存，等待执行"})
	}
}

// calculateCosineSimilarity 计算两个向量的余弦相似度
func calculateCosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}

	dotProduct := 0.0
	normA := 0.0
	normB := 0.0

	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (sqrt(normA) * sqrt(normB))
}

// sqrt 计算平方根
func sqrt(x float64) float64 {
	if x < 0 {
		return 0
	}
	return math.Sqrt(x)
}

// quickSort 快速排序算法
func quickSort(arr []WeightedKnowledgeMessage, low, high int) {
	if low < high {
		pi := partition(arr, low, high)
		quickSort(arr, low, pi-1)
		quickSort(arr, pi+1, high)
	}
}

// partition 快速排序分区函数
func partition(arr []WeightedKnowledgeMessage, low, high int) int {
	pivot := arr[high].Weighted
	i := low - 1

	for j := low; j < high; j++ {
		if arr[j].Weighted >= pivot {
			i++
			arr[i], arr[j] = arr[j], arr[i]
		}
	}

	arr[i+1], arr[high] = arr[high], arr[i+1]
	return i + 1
}

// getCurrentTimestamp 获取当前时间戳
func getCurrentTimestamp() string {
	return time.Now().Format("2006.01.02-15:04:05")
}

// initializeKnowledgeFile 初始化知识库文件
func initializeKnowledgeFile(filePath string) (HistoryDocument, error) {
	// 创建新的HistoryDocument
	doc := HistoryDocument{}
	doc.Meta.ExportedAt = getCurrentTimestamp()
	doc.Meta.Version = "25.1230"
	doc.History = []HistoryMessage{}

	// 确保目录存在
	if mkdirErr := os.MkdirAll(filepath.Dir(filePath), 0755); mkdirErr != nil {
		return doc, mkdirErr
	}

	// 获取文件锁
	fileLock := getFileLock(filePath)
	fileLock.Lock()
	defer fileLock.Unlock()

	// 写入文件
	file, err := os.Create(filePath)
	if err != nil {
		return doc, err
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(doc); err != nil {
		return doc, err
	}

	return doc, nil
}
