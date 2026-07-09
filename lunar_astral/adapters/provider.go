package adapters

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"storage/module"
	"websearch"
)

// ==== 记忆库提供者适配器 ====

// memoryProviderAdapter 将 storage/module 适配为 websearch.MemoryProvider 接口
type memoryProviderAdapter struct{}

// Query 实现 websearch.MemoryProvider 接口
func (m *memoryProviderAdapter) Query(query string) (string, error) {
	ctx := context.Background()
	messages, err := module.QueryMessagesWithContent(ctx, "lunar_messages", query, 10)
	if err != nil {
		return "", err
	}

	if len(messages) == 0 {
		return "", nil
	}

	var sb strings.Builder
	for i, msg := range messages {
		sb.WriteString(fmt.Sprintf("[记忆%d] 相似度:%.1f%% | 角色:%s | 内容:%s\n",
			i+1, msg.Similarity*100, msg.Role, msg.Content))
	}

	return sb.String(), nil
}

// memorySystem 全局记忆库提供者实例（供 webSearchSetMemoryProvider 使用）
var memorySystem websearch.MemoryProvider = &memoryProviderAdapter{}

// ==== 文件下载辅助函数 ====

// downloadFile 下载文件到指定目录，以 groupID 为子目录
// 返回下载后的本地文件路径
func downloadFile(fileURL string, downloadDir string, groupID string) (string, error) {
	// 构建目标目录: downloadDir/groupID/
	targetDir := filepath.Join(downloadDir, groupID)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("创建下载目录失败: %w", err)
	}

	// 从 URL 提取文件名
	fileName := extractFileName(fileURL)
	targetPath := filepath.Join(targetDir, fileName)

	// 发起 HTTP GET 请求
	resp, err := http.Get(fileURL)
	if err != nil {
		return "", fmt.Errorf("下载文件失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载失败，HTTP状态码: %d", resp.StatusCode)
	}

	// 创建目标文件
	file, err := os.Create(targetPath)
	if err != nil {
		return "", fmt.Errorf("创建本地文件失败: %w", err)
	}
	defer file.Close()

	// 写入文件内容
	if _, err := io.Copy(file, resp.Body); err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}

	return targetPath, nil
}

// extractFileName 从 URL 中提取文件名
func extractFileName(fileURL string) string {
	// 去除查询参数和锚点
	cleanURL := fileURL
	if idx := strings.Index(cleanURL, "?"); idx >= 0 {
		cleanURL = cleanURL[:idx]
	}
	if idx := strings.Index(cleanURL, "#"); idx >= 0 {
		cleanURL = cleanURL[:idx]
	}

	// 提取路径最后一段
	base := filepath.Base(cleanURL)
	if base == "" || base == "." || base == "/" {
		base = "download"
	}

	return base
}
