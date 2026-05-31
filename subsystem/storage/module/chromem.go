package module

import (
	"config"
	"context"
	"encoding/json"
	"fmt"
	"logger"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	chromem "github.com/philippgille/chromem-go"
)

func createEmbeddingFunc(baseURL string, apiKey string, modelName string) chromem.EmbeddingFunc {
	return chromem.NewEmbeddingFuncOpenAICompat(baseURL, apiKey, modelName, nil)
}

func Init(baseURL string, apiKey string, modelName string) error {
	initOnce.Do(func() {
		dbDir := filepath.Join(*config.LocalDir, "chromem")
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			initErr = fmt.Errorf("chromem 创建数据目录失败: %v", err)
			return
		}

		entriesFilePath = filepath.Join(dbDir, "entries.json")

		var err error
		db, err = chromem.NewPersistentDB(dbDir, true)
		if err != nil {
			initErr = fmt.Errorf("chromem 创建持久化数据库失败: %v", err)
			return
		}

		embeddingFunc := createEmbeddingFunc(baseURL, apiKey, modelName)

		collection, err = db.GetOrCreateCollection("lunar_messages", nil, embeddingFunc)
		if err != nil {
			initErr = fmt.Errorf("chromem 创建集合失败: %v", err)
			return
		}

		loadEntriesFromFile()

		logger.Info("LunarCore", "chromem-go 初始化完成, 持久化路径: %s, 模型: %s, 已加载 %d 条文档记录", dbDir, modelName, len(documentEntries))
	})
	return initErr
}
func AddMessage(ctx context.Context, role string, content string) error {
	if collection == nil {
		return fmt.Errorf("chromem 未初始化, 请先调用 Init")
	}

	if strings.TrimSpace(content) == "" {
		return nil
	}

	messageIDCounter++
	id := fmt.Sprintf("msg-%d", messageIDCounter)

	metadata := map[string]string{
		"role": role,
	}

	doc := chromem.Document{
		ID:       id,
		Metadata: metadata,
		Content:  content,
	}

	err := collection.AddDocuments(ctx, []chromem.Document{doc}, runtime.NumCPU())
	if err != nil {
		return fmt.Errorf("chromem 添加消息失败: %v", err)
	}

	documentEntriesMu.Lock()
	documentEntries = append(documentEntries, DocumentEntry{ID: id, Role: role, Content: content})
	documentEntriesMu.Unlock()

	saveEntriesToFile()

	return nil
}

// QueryMessages 查询 chromem 数据库中的消息
func QueryMessages(ctx context.Context, queryText string, topK int) ([]string, error) {
	if collection == nil {
		return nil, fmt.Errorf("chromem 未初始化, 请先调用 Init")
	}

	if topK <= 0 {
		topK = 10
	}

	docCount := collection.Count()
	if topK > docCount {
		topK = docCount
	}
	if topK == 0 {
		return []string{}, nil
	}

	results, err := collection.Query(ctx, queryText, topK, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("chromem 查询消息失败: %v", err)
	}

	messages := make([]string, 0, len(results))
	for _, result := range results {
		var msg chromemMessage
		role := "user"
		if r, ok := result.Metadata["role"]; ok {
			role = r
		}
		msg.Role = role
		msg.Content = result.Content

		jsonBytes, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		messages = append(messages, string(jsonBytes))
	}

	return messages, nil
}

// QueryMessagesWithContent 查询 chromem 数据库中的消息，包含角色和内容
func QueryMessagesWithContent(ctx context.Context, queryText string, topK int) ([]map[string]string, error) {
	if collection == nil {
		return nil, fmt.Errorf("chromem 未初始化, 请先调用 Init")
	}

	if topK <= 0 {
		topK = 10
	}

	docCount := collection.Count()
	if topK > docCount {
		topK = docCount
	}
	if topK == 0 {
		return []map[string]string{}, nil
	}

	results, err := collection.Query(ctx, queryText, topK, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("chromem 查询消息失败: %v", err)
	}

	messages := make([]map[string]string, 0, len(results))
	for _, result := range results {
		role := "user"
		if r, ok := result.Metadata["role"]; ok {
			role = r
		}
		messages = append(messages, map[string]string{
			"id":      result.ID,
			"role":    role,
			"content": result.Content,
		})
	}

	return messages, nil
}

// DeleteMessage 从 chromem 数据库中删除指定消息
func DeleteMessage(ctx context.Context, id string) error {
	if collection == nil {
		return fmt.Errorf("chromem 未初始化, 请先调用 Init")
	}

	if err := collection.Delete(ctx, nil, nil, id); err != nil {
		return fmt.Errorf("chromem 删除消息失败: %v", err)
	}

	documentEntriesMu.Lock()
	for i, entry := range documentEntries {
		if entry.ID == id {
			documentEntries = append(documentEntries[:i], documentEntries[i+1:]...)
			break
		}
	}
	documentEntriesMu.Unlock()

	saveEntriesToFile()

	return nil
}

// AddMessageWithID 添加消息并返回生成的消息 ID
func AddMessageWithID(ctx context.Context, role string, content string) (string, error) {
	if collection == nil {
		return "", fmt.Errorf("chromem 未初始化, 请先调用 Init")
	}

	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("消息内容不能为空")
	}

	messageIDCounter++
	id := fmt.Sprintf("msg-%d", messageIDCounter)

	metadata := map[string]string{
		"role": role,
	}

	doc := chromem.Document{
		ID:       id,
		Metadata: metadata,
		Content:  content,
	}

	err := collection.AddDocuments(ctx, []chromem.Document{doc}, runtime.NumCPU())
	if err != nil {
		return "", fmt.Errorf("chromem 添加消息失败: %v", err)
	}

	documentEntriesMu.Lock()
	documentEntries = append(documentEntries, DocumentEntry{ID: id, Role: role, Content: content})
	documentEntriesMu.Unlock()

	saveEntriesToFile()

	return id, nil
}

func GetCollectionCount() int {
	if collection == nil {
		return 0
	}
	return collection.Count()
}

func IsInitialized() bool {
	return collection != nil
}

func GetDocuments(offset int, limit int) ([]DocumentEntry, int) {
	documentEntriesMu.RLock()
	defer documentEntriesMu.RUnlock()

	total := len(documentEntries)

	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return []DocumentEntry{}, total
	}

	end := offset + limit
	if end > total {
		end = total
	}

	entries := make([]DocumentEntry, end-offset)
	copy(entries, documentEntries[offset:end])
	return entries, total
}

func GetEntryCount() int {
	documentEntriesMu.RLock()
	defer documentEntriesMu.RUnlock()
	return len(documentEntries)
}

func HasSyncMismatch() bool {
	if collection == nil {
		return false
	}
	return collection.Count() != GetEntryCount()
}

func RebuildEntries(ctx context.Context) (int, error) {
	if collection == nil {
		return 0, fmt.Errorf("chromem 未初始化, 请先调用 Init")
	}

	chromemCount := collection.Count()
	if chromemCount == 0 {
		documentEntriesMu.Lock()
		documentEntries = nil
		documentEntriesMu.Unlock()
		saveEntriesToFile()
		return 0, nil
	}

	results, err := collection.Query(ctx, " ", chromemCount, nil, nil)
	if err != nil {
		return 0, fmt.Errorf("chromem 查询所有文档失败: %v", err)
	}

	seenIDs := make(map[string]bool)
	var newEntries []DocumentEntry

	for _, result := range results {
		if seenIDs[result.ID] {
			continue
		}
		seenIDs[result.ID] = true

		role := "user"
		if r, ok := result.Metadata["role"]; ok {
			role = r
		}

		newEntries = append(newEntries, DocumentEntry{
			ID:      result.ID,
			Role:    role,
			Content: result.Content,
		})
	}

	documentEntriesMu.Lock()
	documentEntries = newEntries
	documentEntriesMu.Unlock()

	saveEntriesToFile()

	maxNum := 0
	for _, entry := range newEntries {
		var num int
		if _, scanErr := fmt.Sscanf(entry.ID, "msg-%d", &num); scanErr == nil && num > maxNum {
			maxNum = num
		}
	}
	if maxNum > messageIDCounter {
		messageIDCounter = maxNum
	}

	logger.Info("LunarCore", "chromem 重建 entries 完成, 共 %d 条文档", len(newEntries))

	return len(newEntries), nil
}

func loadEntriesFromFile() {
	data, err := os.ReadFile(entriesFilePath)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warn("LunarCore", "chromem 读取 entries.json 失败: %v", err)
		}
		return
	}

	if len(data) == 0 {
		return
	}

	var entries []DocumentEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		logger.Warn("LunarCore", "chromem entries.json 解析失败: %v", err)
		return
	}

	documentEntriesMu.Lock()
	documentEntries = entries
	documentEntriesMu.Unlock()

	maxNum := 0
	for _, entry := range entries {
		var num int
		if _, scanErr := fmt.Sscanf(entry.ID, "msg-%d", &num); scanErr == nil && num > maxNum {
			maxNum = num
		}
	}
	if maxNum > messageIDCounter {
		messageIDCounter = maxNum
	}

	logger.Info("LunarCore", "chromem 从 entries.json 加载了 %d 条文档, ID计数器重置为 %d", len(entries), messageIDCounter)
}

func saveEntriesToFile() {
	documentEntriesMu.RLock()
	data, err := json.MarshalIndent(documentEntries, "", "  ")
	documentEntriesMu.RUnlock()
	if err != nil {
		logger.Error("LunarCore", "chromem entries 序列化失败: %v", err)
		return
	}

	if err := os.WriteFile(entriesFilePath, data, 0644); err != nil {
		logger.Error("LunarCore", "chromem entries.json 写入失败: %v", err)
	}
}