package chromem

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
	"sync"

	chromem "github.com/philippgille/chromem-go"
)

var (
	db         *chromem.DB
	collection *chromem.Collection
	initOnce   sync.Once
	initErr    error
)

var messageIDCounter int

type chromemMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// createEmbeddingFunc 创建 chromem 嵌入函数
func createEmbeddingFunc(baseURL string, apiKey string, modelName string) chromem.EmbeddingFunc {
	return chromem.NewEmbeddingFuncOpenAICompat(baseURL, apiKey, modelName, nil)
}

// Init 初始化 chromem 数据库
func Init(baseURL string, apiKey string, modelName string) error {
	initOnce.Do(func() {
		dbDir := filepath.Join(*config.LocalDir, "chromem")
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			initErr = fmt.Errorf("chromem 创建数据目录失败: %v", err)
			return
		}

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

		logger.Info("LunarCore", "chromem-go 初始化完成, 持久化路径: %s, 模型: %s", dbDir, modelName)
	})
	return initErr
}

// AddMessage 添加消息到 chromem 数据库
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
			"role":    role,
			"content": result.Content,
		})
	}

	return messages, nil
}

// IsInitialized 检查 chromem 数据库是否已初始化
func IsInitialized() bool {
	return collection != nil
}
