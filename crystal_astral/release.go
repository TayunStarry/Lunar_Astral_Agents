package main

import (
	"config"
	"io/fs"
	"logger"
	"os"
	"path/filepath"
	"strings"
)

// embeddedDataRoot 嵌入资源的根目录名（与 //go:embed embedded_data/* 对应）
const embeddedDataRoot = "embedded_data"

// EnsureLocalData 启动时备用机制：检查 local_data 目录，从嵌入资源中补全缺失的文件。
//
// 策略：仅补全缺失文件，已存在的文件保留不动（避免覆盖用户的修改或自定义内容）。
// 失败仅打印警告，不阻断服务启动。
//
// 涵盖的资源（由 build.ps1 的 Sync-EmbeddedData 同步到 embedded_data/）：
//   - audios/                       音频数据
//   - images/background/            背景图片
//   - images/placeholder/           未知文件占位图
//   - package/ 下无 metadata.json 的子目录与裸露的 js 文件（库/资源）
func EnsureLocalData() {
	execPath, err := os.Executable()
	if err != nil {
		logger.Warn("CrystalAstral", "获取可执行文件路径失败，跳过 local_data 补全: %v", err)
		return
	}
	execDir := filepath.Dir(execPath)
	localDataDir := filepath.Join(execDir, *config.LocalDir)

	// 确保根目录存在
	if err := os.MkdirAll(localDataDir, 0755); err != nil {
		logger.Warn("CrystalAstral", "创建 local_data 目录失败: %v", err)
		return
	}

	releasedCount := 0
	skippedCount := 0

	// 遍历嵌入资源，逐项检查并补全
	err = fs.WalkDir(EmbeddedLocalData, embeddedDataRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// 跳过根目录本身
		if path == embeddedDataRoot {
			return nil
		}

		// 计算相对路径（去掉 "embedded_data/" 前缀）
		relPath := strings.TrimPrefix(path, embeddedDataRoot+"/")
		if relPath == "" {
			return nil
		}

		// embed.FS 使用正斜杠，转换为 OS 路径分隔符后拼接目标路径
		targetPath := filepath.Join(localDataDir, filepath.FromSlash(relPath))

		if d.IsDir() {
			// 目录：创建（已存在不报错）
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				logger.Warn("CrystalAstral", "创建目录失败 %s: %v", targetPath, err)
			}
			return nil
		}

		// 文件：检查是否已存在
		if _, err := os.Stat(targetPath); err == nil {
			// 文件已存在，跳过（仅补全缺失，不覆盖用户修改）
			skippedCount++
			return nil
		}

		// 文件不存在，从嵌入资源释放
		data, readErr := EmbeddedLocalData.ReadFile(path)
		if readErr != nil {
			logger.Warn("CrystalAstral", "读取嵌入资源失败 %s: %v", path, readErr)
			return nil
		}

		// 确保父目录存在
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			logger.Warn("CrystalAstral", "创建父目录失败 %s: %v", filepath.Dir(targetPath), err)
			return nil
		}

		if err := os.WriteFile(targetPath, data, 0644); err != nil {
			logger.Warn("CrystalAstral", "释放文件失败 %s: %v", targetPath, err)
			return nil
		}
		releasedCount++
		return nil
	})

	if err != nil {
		logger.Warn("CrystalAstral", "遍历嵌入资源失败: %v", err)
		return
	}

	if releasedCount > 0 {
		logger.Info("CrystalAstral", "local_data 补全完成: 释放 %d 个文件, 跳过 %d 个已存在文件",
			releasedCount, skippedCount)
	} else {
		logger.Info("CrystalAstral", "local_data 资源完整，无需补全")
	}
}
