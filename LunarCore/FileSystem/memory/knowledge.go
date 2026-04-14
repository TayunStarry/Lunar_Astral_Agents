package memory

import (
	"LunarCore/FileSystem"
	"LunarCore/config"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
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

// QueryKnowledge 查询知识库
func QueryKnowledge(filePath string, queryVector []float64, topK int) ([]KnowledgeMessage, error) {
	// 验证请求参数
	if filePath == "" {
		return nil, fmt.Errorf("缺少文件路径")
	}

	if len(queryVector) != 256 {
		return nil, fmt.Errorf("queryVector长度必须为256")
	}

	if topK <= 0 {
		topK = 15
	}

	// 读取知识库文件
	knowledgePath := filepath.Join(*config.LocalDir, filePath)
	if !strings.HasPrefix(knowledgePath, filepath.Clean(*config.LocalDir)) {
		return nil, fmt.Errorf("访问被拒绝")
	}

	var doc HistoryDocument

	// 尝试打开文件
	file, openErr := os.Open(knowledgePath)
	if openErr != nil {
		// 文件不存在，初始化文件
		if os.IsNotExist(openErr) {
			var err error
			doc, err = InitializeKnowledgeFile(knowledgePath)
			if err != nil {
				return nil, fmt.Errorf("初始化文件失败: %w", err)
			}
		} else {
			// 其他错误
			return nil, fmt.Errorf("打开文件失败: %w", openErr)
		}
	} else {
		// 文件存在，读取文件
		defer file.Close()
		if err := json.NewDecoder(file).Decode(&doc); err != nil {
			return nil, fmt.Errorf("解析文件失败: %w", err)
		}
	}

	// 计算相似度并排序
	var results []WeightedKnowledgeMessage
	for _, msg := range doc.History {
		if len(msg.EmbedVector) == 0 {
			continue // 跳过没有嵌入向量的消息
		}

		// 计算余弦相似度
		similarity := CalculateCosineSimilarity(queryVector, msg.EmbedVector)

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
	QuickSort(results, 0, len(results)-1)

	// 返回topK个结果
	if len(results) > topK {
		results = results[:topK]
	}

	// 提取纯KnowledgeMessage数组，不包含权重
	pureResults := []KnowledgeMessage{}
	for _, item := range results {
		pureResults = append(pureResults, item.Message)
	}

	return pureResults, nil
}

// WriteKnowledge 写入知识库（缓存到内存）
func WriteKnowledge(filePath string, message HistoryMessage) (bool, error) {
	// 验证请求参数
	if filePath == "" {
		return false, fmt.Errorf("缺少文件路径")
	}

	// 验证文件路径安全性
	knowledgePath := filepath.Join(*config.LocalDir, filePath)
	if !strings.HasPrefix(knowledgePath, filepath.Clean(*config.LocalDir)) {
		return false, fmt.Errorf("访问被拒绝")
	}

	newMsg := message
	// 如果嵌入向量为空，设置为空切片（将由客户端生成）
	if newMsg.EmbedVector == nil {
		newMsg.EmbedVector = []float64{}
	}

	// 将消息缓存到内存中
	knowledgeCacheMutex.Lock()
	defer knowledgeCacheMutex.Unlock()

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

	return hasDuplicate, nil
}

// FlushKnowledge 刷新知识库（将缓存写入文件）
func FlushKnowledge(filePath string) (map[string]interface{}, error) {
	// 验证请求参数
	if filePath == "" {
		return nil, fmt.Errorf("缺少文件路径")
	}

	// 验证文件路径安全性
	knowledgePath := filepath.Join(*config.LocalDir, filePath)
	if !strings.HasPrefix(knowledgePath, filepath.Clean(*config.LocalDir)) {
		return nil, fmt.Errorf("访问被拒绝")
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
		return map[string]interface{}{
			"message": "没有待处理的消息",
		}, nil
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
	if _, err := os.Stat(knowledgePath); err == nil {
		file, fileErr := os.Open(knowledgePath)
		if fileErr != nil {
			return nil, fmt.Errorf("打开文件失败: %w", fileErr)
		}
		if err = json.NewDecoder(file).Decode(&doc); err != nil {
			file.Close()
			return nil, fmt.Errorf("解析文件失败: %w", err)
		}
		file.Close()
	} else {
		// 文件不存在，初始化文件
		var err error
		doc, err = InitializeKnowledgeFile(knowledgePath)
		if err != nil {
			return nil, fmt.Errorf("初始化文件失败: %w", err)
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
	lock := FileSystem.GetFileLock(knowledgePath)
	lock.Lock()
	defer lock.Unlock()

	file, err := os.Create(knowledgePath)
	if err != nil {
		return nil, fmt.Errorf("创建文件失败: %w", err)
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(doc); err != nil {
		return nil, fmt.Errorf("写入文件失败: %w", err)
	}

	// 返回统计信息
	return map[string]interface{}{
		"message":          "刷新成功",
		"messagesWritten":  newMessagesAdded,
		"messagesInCache":  messagesInCache,
		"messagesDeleted":  messagesDeleted,
		"messagesToDelete": messagesToDelete,
	}, nil
}

// ListKnowledge 列出知识库内容
func ListKnowledge(filePath string) ([]HistoryMessage, error) {
	// 验证请求参数
	if filePath == "" {
		return nil, fmt.Errorf("缺少文件路径")
	}

	// 验证文件路径安全性
	knowledgePath := filepath.Join(*config.LocalDir, filePath)
	if !strings.HasPrefix(knowledgePath, filepath.Clean(*config.LocalDir)) {
		return nil, fmt.Errorf("访问被拒绝")
	}

	// 读取知识库文件
	var doc HistoryDocument
	var fileErr error

	// 尝试打开文件
	file, openErr := os.Open(knowledgePath)
	if openErr != nil {
		// 文件不存在，初始化文件
		if os.IsNotExist(openErr) {
			doc, fileErr = InitializeKnowledgeFile(knowledgePath)
			if fileErr != nil {
				return nil, fmt.Errorf("初始化文件失败: %w", fileErr)
			}
		} else {
			// 其他错误
			return nil, fmt.Errorf("打开文件失败: %w", openErr)
		}
	} else {
		// 文件存在，读取文件
		if fileErr = json.NewDecoder(file).Decode(&doc); fileErr != nil {
			file.Close()
			return nil, fmt.Errorf("解析文件失败: %w", fileErr)
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

	return results, nil
}

// DeleteKnowledge 删除知识库条目（缓存到内存）
func DeleteKnowledge(filePath string, uuid string) (bool, error) {
	// 验证请求参数
	if filePath == "" {
		return false, fmt.Errorf("缺少文件路径")
	}

	if uuid == "" {
		return false, fmt.Errorf("缺少UUID")
	}

	// 验证文件路径安全性
	knowledgePath := filepath.Join(*config.LocalDir, filePath)
	if !strings.HasPrefix(knowledgePath, filepath.Clean(*config.LocalDir)) {
		return false, fmt.Errorf("访问被拒绝")
	}

	// 将删除请求缓存到内存中
	knowledgeCacheMutex.Lock()
	defer knowledgeCacheMutex.Unlock()

	if _, exists := knowledgeDeleteCache[knowledgePath]; !exists {
		knowledgeDeleteCache[knowledgePath] = make(map[string]bool)
	}

	// 检查缓存中是否已有相同的删除请求
	hasDuplicate := knowledgeDeleteCache[knowledgePath][uuid]
	if !hasDuplicate {
		knowledgeDeleteCache[knowledgePath][uuid] = true
	}

	return hasDuplicate, nil
}

// CalculateCosineSimilarity 计算两个向量的余弦相似度
func CalculateCosineSimilarity(a, b []float64) float64 {
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

	return dotProduct / (Sqrt(normA) * Sqrt(normB))
}

// Sqrt 计算平方根
func Sqrt(x float64) float64 {
	if x < 0 {
		return 0
	}
	return math.Sqrt(x)
}

// QuickSort 快速排序算法
func QuickSort(arr []WeightedKnowledgeMessage, low, high int) {
	if low < high {
		pi := Partition(arr, low, high)
		QuickSort(arr, low, pi-1)
		QuickSort(arr, pi+1, high)
	}
}

// Partition 快速排序分区函数
func Partition(arr []WeightedKnowledgeMessage, low, high int) int {
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

// GetCurrentTimestamp 获取当前时间戳
func GetCurrentTimestamp() string {
	return time.Now().Format("2006.01.02-15:04:05")
}

// InitializeKnowledgeFile 初始化知识库文件
func InitializeKnowledgeFile(filePath string) (HistoryDocument, error) {
	// 创建新的HistoryDocument
	doc := HistoryDocument{}
	doc.Meta.ExportedAt = GetCurrentTimestamp()
	doc.Meta.Version = "25.1230"
	doc.History = []HistoryMessage{}

	// 确保目录存在
	if mkdirErr := os.MkdirAll(filepath.Dir(filePath), 0755); mkdirErr != nil {
		return doc, mkdirErr
	}

	// 获取文件锁
	lock := FileSystem.GetFileLock(filePath)
	lock.Lock()
	defer lock.Unlock()

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
