package image

import (
	"encoding/json"
	"log"
	"open-lunar/config"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// CleanupResult 清理结果结构体
type CleanupResult struct {
	TotalImages      int `json:"totalImages"`
	ReferencedImages int `json:"referencedImages"`
	DeletedImages    int `json:"deletedImages"`
	RetainedImages   int `json:"retainedImages"`
}

// CleanupHistoryMessage 清理用的历史消息结构体
type CleanupHistoryMessage struct {
	ImageUrl string `json:"image_url"`
}

// CleanupHistoryDocument 清理用的历史文档结构体
type CleanupHistoryDocument struct {
	History []CleanupHistoryMessage `json:"history"`
}

// CleanupUnreferencedImages 清理未被引用的图片
func CleanupUnreferencedImages() (CleanupResult, error) {
	// 打印开始清理的日志
	log.Println("[清理任务] 开始清理未被引用的图片...")

	// 1. 扫描历史记录，收集引用的图片
	referencedImages, err := CollectReferencedImages()
	if err != nil {
		return CleanupResult{}, err
	}

	// 2. 扫描图片文件夹，收集所有图片
	allImages, err := CollectAllImages()
	if err != nil {
		return CleanupResult{}, err
	}

	// 3. 删除未引用的图片
	deletedCount, err := DeleteUnreferencedImages(referencedImages)
	if err != nil {
		return CleanupResult{}, err
	}

	// 4. 构建清理结果
	result := CleanupResult{
		TotalImages:      len(allImages),
		ReferencedImages: len(referencedImages),
		DeletedImages:    deletedCount,
		RetainedImages:   len(allImages) - deletedCount,
	}

	// 打印清理完成的日志
	log.Printf("[清理任务] 清理完成，共处理 %d 张图片，删除 %d 张未被引用的图片", len(allImages), deletedCount)

	return result, nil
}

// CollectReferencedImages 收集引用的图片
func CollectReferencedImages() (map[string]bool, error) {
	referencedImages := make(map[string]bool)
	var mu sync.RWMutex

	// 定义历史记录目录
	knowledgeDir := filepath.Join(*config.LocalDir, "knowledge")

	// 递归扫描历史记录文件
	err := filepath.Walk(knowledgeDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 只处理JSON文件
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".json") {
			// 读取文件内容
			data, err := os.ReadFile(path)
			if err != nil {
				log.Printf("警告: 无法读取文件 %s: %v", path, err)
				return nil
			}

			// 解析JSON文件
			var doc CleanupHistoryDocument
			if err := json.Unmarshal(data, &doc); err != nil {
				log.Printf("警告: 无法解析文件 %s: %v", path, err)
				return nil
			}

			// 提取图片引用
			for _, msg := range doc.History {
				if msg.ImageUrl != "" && !strings.HasPrefix(msg.ImageUrl, "data:image") {
					// 提取图片文件名
					imageFilename := ExtractImageFilename(msg.ImageUrl)
					if imageFilename != "" {
						mu.Lock()
						referencedImages[imageFilename] = true
						mu.Unlock()
					}
				}
			}
		}

		return nil
	})

	return referencedImages, err
}

// CollectAllImages 收集所有图片
func CollectAllImages() (map[string]bool, error) {
	allImages := make(map[string]bool)

	// 定义图片目录
	imagesDir := filepath.Join(*config.LocalDir, "images")

	// 递归扫描图片文件夹
	err := filepath.Walk(imagesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 只处理文件
		if !info.IsDir() && IsImageFile(info.Name()) {
			// 提取文件名
			imageFilename := filepath.Base(path)
			allImages[imageFilename] = true
		}

		return nil
	})

	return allImages, err
}

// DeleteUnreferencedImages 删除未引用的图片
func DeleteUnreferencedImages(referencedImages map[string]bool) (int, error) {
	deletedCount := 0

	// 定义图片目录
	imagesDir := filepath.Join(*config.LocalDir, "images")

	// 递归扫描图片文件夹，删除未引用的图片
	err := filepath.Walk(imagesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 只处理文件
		if !info.IsDir() && IsImageFile(info.Name()) {
			// 提取文件名
			imageFilename := filepath.Base(path)

			// 检查是否被引用
			if !referencedImages[imageFilename] {
				// 删除未引用的图片
				if err := os.Remove(path); err != nil {
					log.Printf("警告: 无法删除文件 %s: %v", path, err)
				} else {
					deletedCount++
					log.Printf("已删除未引用的图片: %s", imageFilename)
				}
			}
		}

		return nil
	})

	return deletedCount, err
}

// ExtractImageFilename 从图片URL中提取文件名
func ExtractImageFilename(imageURL string) string {
	// 处理 "/read/images/image.jpg" 格式
	if strings.HasPrefix(imageURL, "/read/images/") {
		// 移除 "/read/images/" 前缀
		path := strings.TrimPrefix(imageURL, "/read/images/")
		// 提取文件名部分
		if strings.Contains(path, "/") {
			parts := strings.Split(path, "/")
			return parts[len(parts)-1]
		}
		return path
	}

	// 处理其他可能的格式
	if strings.Contains(imageURL, "/") {
		// 仅提取文件名
		parts := strings.Split(imageURL, "/")
		return parts[len(parts)-1]
	}

	return imageURL
}

// IsImageFile 检查是否是图片文件
func IsImageFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	imageExtensions := []string{".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".svg", ".ico"}

	for _, imgExt := range imageExtensions {
		if ext == imgExt {
			return true
		}
	}
	return false
}
