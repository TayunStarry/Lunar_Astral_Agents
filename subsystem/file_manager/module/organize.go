package module

import (
	"LunarSubsystem/general_config"
	"LunarSubsystem/general_logger"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExecuteOrganize 执行批量文件整理操作
func ExecuteOrganize(req OrganizeRequest) OrganizeResponse {
	response := OrganizeResponse{
		Total: len(req.Operations),
	}

	// 解析基础路径
	basePath := filepath.Clean(filepath.Join(*config.LocalDir, req.BasePath))
	localDir := filepath.Clean(*config.LocalDir)

	// 验证基础路径在 LocalDir 内
	if !strings.HasPrefix(basePath, localDir) {
		response.Error = "基础路径不在允许的范围内"
		return response
	}

	// 确保基础目录存在
	if err := os.MkdirAll(basePath, 0755); err != nil {
		response.Error = fmt.Sprintf("创建基础目录失败: %v", err)
		return response
	}

	for _, op := range req.Operations {
		result := OrganizeResult{
			Type:   op.Type,
			Source: op.Source,
			Target: op.Target,
		}

		switch op.Type {
		case "move":
			err := executeMove(basePath, op.Source, op.Target, localDir)
			if err != nil {
				result.Error = err.Error()
			} else {
				result.Success = true
			}
		case "rename":
			err := executeRename(basePath, op.Source, op.Target, localDir)
			if err != nil {
				result.Error = err.Error()
			} else {
				result.Success = true
			}
		case "merge":
			err := executeMerge(basePath, op.Source, op.Target, localDir)
			if err != nil {
				result.Error = err.Error()
			} else {
				result.Success = true
			}
		case "delete":
			err := executeDelete(basePath, op.Source, localDir)
			if err != nil {
				result.Error = err.Error()
			} else {
				result.Success = true
			}
		default:
			result.Error = fmt.Sprintf("不支持的操作类型: %s", op.Type)
		}

		if result.Success {
			response.SuccessCount++
		} else {
			response.FailCount++
		}
		response.Results = append(response.Results, result)
	}

	response.Success = response.FailCount == 0
	return response
}

// resolvePath 解析并验证路径，确保在允许范围内
func resolvePath(basePath, relativePath, localDir string) (string, error) {
	if relativePath == "" {
		return "", fmt.Errorf("路径不能为空")
	}
	fullPath := filepath.Clean(filepath.Join(basePath, relativePath))
	if !strings.HasPrefix(fullPath, localDir) {
		return "", fmt.Errorf("路径不在允许的范围内")
	}
	return fullPath, nil
}

// executeMove 移动文件或目录
func executeMove(basePath, source, target, localDir string) error {
	srcPath, err := resolvePath(basePath, source, localDir)
	if err != nil {
		return fmt.Errorf("源路径无效: %w", err)
	}
	tgtPath, err := resolvePath(basePath, target, localDir)
	if err != nil {
		return fmt.Errorf("目标路径无效: %w", err)
	}

	// 检查源是否存在
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return fmt.Errorf("源文件不存在: %s", source)
	}

	// 确保目标目录存在
	tgtDir := filepath.Dir(tgtPath)
	if err := os.MkdirAll(tgtDir, 0755); err != nil {
		return fmt.Errorf("创建目标目录失败: %w", err)
	}

	// 尝试直接重命名（同文件系统）
	if err := os.Rename(srcPath, tgtPath); err != nil {
		// 跨文件系统时回退到复制+删除
		if copyErr := copyFile(srcPath, tgtPath); copyErr != nil {
			return fmt.Errorf("移动文件失败: %w", copyErr)
		}
		if err := os.RemoveAll(srcPath); err != nil {
			logger.SubError("Storage", "Organize", "删除源文件失败: %s, %v", srcPath, err)
		}
	}

	logger.SubInfo("Storage", "Organize", "移动: %s -> %s", source, target)
	return nil
}

// executeRename 重命名文件或目录
func executeRename(basePath, source, target, localDir string) error {
	srcPath, err := resolvePath(basePath, source, localDir)
	if err != nil {
		return fmt.Errorf("源路径无效: %w", err)
	}
	tgtPath, err := resolvePath(basePath, target, localDir)
	if err != nil {
		return fmt.Errorf("目标路径无效: %w", err)
	}

	// 检查源是否存在
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return fmt.Errorf("源文件不存在: %s", source)
	}

	// 确保目标目录存在
	tgtDir := filepath.Dir(tgtPath)
	if err := os.MkdirAll(tgtDir, 0755); err != nil {
		return fmt.Errorf("创建目标目录失败: %w", err)
	}

	if err := os.Rename(srcPath, tgtPath); err != nil {
		return fmt.Errorf("重命名失败: %w", err)
	}

	logger.SubInfo("Storage", "Organize", "重命名: %s -> %s", source, target)
	return nil
}

// executeMerge 合并文件夹：将源文件夹内容移动到目标文件夹
func executeMerge(basePath, source, target, localDir string) error {
	srcPath, err := resolvePath(basePath, source, localDir)
	if err != nil {
		return fmt.Errorf("源路径无效: %w", err)
	}
	tgtPath, err := resolvePath(basePath, target, localDir)
	if err != nil {
		return fmt.Errorf("目标路径无效: %w", err)
	}

	// 检查源是否存在
	srcInfo, err := os.Stat(srcPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("源文件夹不存在: %s", source)
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("源路径不是文件夹: %s", source)
	}

	// 确保目标目录存在
	if err := os.MkdirAll(tgtPath, 0755); err != nil {
		return fmt.Errorf("创建目标目录失败: %w", err)
	}

	// 遍历源目录，移动所有内容
	entries, err := os.ReadDir(srcPath)
	if err != nil {
		return fmt.Errorf("读取源目录失败: %w", err)
	}

	var moveErrors []string
	for _, entry := range entries {
		entrySrc := filepath.Join(srcPath, entry.Name())
		entryTgt := filepath.Join(tgtPath, entry.Name())

		// 如果目标已存在同名项，添加后缀
		if _, err := os.Stat(entryTgt); err == nil {
			ext := filepath.Ext(entry.Name())
			base := strings.TrimSuffix(entry.Name(), ext)
			entryTgt = filepath.Join(tgtPath, fmt.Sprintf("%s_merged%s", base, ext))
		}

		if err := os.Rename(entrySrc, entryTgt); err != nil {
			moveErrors = append(moveErrors, fmt.Sprintf("%s: %v", entry.Name(), err))
		}
	}

	// 删除源目录（如果为空）
	remaining, _ := os.ReadDir(srcPath)
	if len(remaining) == 0 {
		if err := os.Remove(srcPath); err != nil {
			logger.SubError("Storage", "Organize", "删除空源目录失败: %s, %v", srcPath, err)
		}
	}

	if len(moveErrors) > 0 {
		return fmt.Errorf("部分文件合并失败: %s", strings.Join(moveErrors, "; "))
	}

	logger.SubInfo("Storage", "Organize", "合并: %s -> %s", source, target)
	return nil
}

// executeDelete 删除文件或目录
func executeDelete(basePath, source, localDir string) error {
	srcPath, err := resolvePath(basePath, source, localDir)
	if err != nil {
		return fmt.Errorf("路径无效: %w", err)
	}

	// 检查源是否存在
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return fmt.Errorf("文件不存在: %s", source)
	}

	if err := os.RemoveAll(srcPath); err != nil {
		return fmt.Errorf("删除失败: %w", err)
	}

	logger.SubInfo("Storage", "Organize", "删除: %s", source)
	return nil
}

// copyFile 复制文件（用于跨文件系统移动）
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("打开源文件失败: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("创建目标文件失败: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("复制文件内容失败: %w", err)
	}

	// 保留文件权限
	srcInfo, err := os.Stat(src)
	if err == nil {
		os.Chmod(dst, srcInfo.Mode())
	}

	return dstFile.Sync()
}
