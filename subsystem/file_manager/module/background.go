package module

import (
	config "LunarSubsystem/GeneralConfig"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// GetRandomBackgroundImage 从 background 目录中随机选择一个背景图片文件名
func GetRandomBackgroundImage() (string, error) {
	backgroundDir := filepath.Join(*config.LocalDir, "images/background")
	entries, err := os.ReadDir(backgroundDir)
	if err != nil {
		return "", err
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "picture") {
			files = append(files, entry.Name())
		}
	}

	if len(files) == 0 {
		return "", fmt.Errorf("未找到 picture 开头的图片文件")
	}
	randomIndex := rand.Intn(len(files))
	return files[randomIndex], nil
}

// CopyBuffer 复制文件内容到 ResponseWriter
func CopyBuffer(w http.ResponseWriter, file *os.File) (int64, error) {
	return io.Copy(w, file)
}
