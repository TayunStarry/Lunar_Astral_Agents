package module

import (
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExecuteMove 执行文件移动操作
// 冲突策略:
//   - "ask": 先预检同名冲突并返回 conflicts 列表，不执行移动（由前端提示用户选择后续策略）
//   - "auto_rename": 目标存在同名项时自动追加 " (1)"、" (2)" 序号重命名
//   - "overwrite": 目标存在同名项时直接覆盖
func ExecuteMove(req MoveItemRequest) MoveResponse {
	response := MoveResponse{Success: false}

	if len(req.Sources) == 0 {
		response.Error = "sources 不能为空"
		return response
	}

	localDir := filepath.Clean(*GeneralConfig.LocalDir)

	// 解析并校验目标目录
	var targetBase string
	if req.TargetDir == "" {
		targetBase = localDir
	} else {
		targetBase = filepath.Clean(filepath.Join(localDir, req.TargetDir))
		if !strings.HasPrefix(targetBase, localDir) {
			response.Error = "目标目录不在允许的范围内"
			return response
		}
	}

	// 目标目录存在性处理
	if req.CreateDirs {
		if err := os.MkdirAll(targetBase, 0755); err != nil {
			response.Error = fmt.Sprintf("创建目标目录失败: %v", err)
			return response
		}
	} else if info, err := os.Stat(targetBase); err != nil || !info.IsDir() {
		response.Error = "目标目录不存在，可设置 create_dirs 自动创建"
		return response
	}

	// 解析源路径列表，预检存在性与自移动
	type sourceEntry struct {
		source  string
		srcPath string
		isDir   bool
	}
	var entries []sourceEntry
	for _, source := range req.Sources {
		srcPath, err := resolvePath(localDir, source, localDir)
		if err != nil {
			response.Error = fmt.Sprintf("源路径无效: %s", source)
			return response
		}
		info, err := os.Stat(srcPath)
		if os.IsNotExist(err) {
			response.Error = fmt.Sprintf("源文件不存在: %s", source)
			return response
		}
		// 防止将目录移动到自己内部（递归自包含）
		if info.IsDir() && strings.HasPrefix(targetBase, srcPath) {
			response.Error = fmt.Sprintf("不能将文件夹移动到其自身内部: %s", source)
			return response
		}
		entries = append(entries, sourceEntry{source: source, srcPath: srcPath, isDir: info.IsDir()})
	}

	// 预检同名冲突
	var conflicts []MoveConflict
	for _, entry := range entries {
		tgtPath := filepath.Join(targetBase, filepath.Base(entry.srcPath))
		if _, err := os.Stat(tgtPath); err == nil {
			conflicts = append(conflicts, MoveConflict{
				Source: entry.source,
				Target: relativeToLocal(tgtPath, localDir),
				IsDir:  entry.isDir,
			})
		}
	}

	// ask 策略且存在冲突：返回冲突列表，不执行移动
	if req.ConflictStrategy == "ask" && len(conflicts) > 0 {
		response.Conflicts = conflicts
		response.Error = "存在同名冲突，等待用户确认处理方式"
		return response
	}

	// 逐个执行移动
	var failCount int
	for _, entry := range entries {
		result := MoveItemResult{Source: entry.source}
		tgtPath := filepath.Join(targetBase, filepath.Base(entry.srcPath))

		// 冲突处理
		if _, err := os.Stat(tgtPath); err == nil {
			result.Conflict = true
			switch req.ConflictStrategy {
			case "overwrite":
				if err := os.RemoveAll(tgtPath); err != nil {
					result.Error = fmt.Sprintf("覆盖同名目标失败: %v", err)
				}
			case "auto_rename":
				tgtPath = autoRenamePath(tgtPath)
				result.Renamed = true
			default:
				result.Error = "目标位置存在同名项，未执行移动"
			}
		}

		// 未因冲突失败时执行移动
		if result.Error == "" {
			lock := GetFileLock(entry.srcPath)
			lock.Lock()
			err := moveEntry(entry.srcPath, tgtPath)
			lock.Unlock()
			if err != nil {
				result.Error = err.Error()
			}
		}

		if result.Error != "" {
			failCount++
		} else {
			result.Target = relativeToLocal(tgtPath, localDir)
			LoggerGeneral.SubInfo("FileManager", "Move", "移动: %s -> %s", result.Source, result.Target)
		}
		response.Results = append(response.Results, result)
	}

	response.Success = failCount == 0
	return response
}

// moveEntry 移动文件或目录（同文件系统 os.Rename，跨文件系统回退复制+删除）
func moveEntry(srcPath, tgtPath string) error {
	// 源与目标相同（如移动到自身所在目录）视为成功
	if srcPath == tgtPath {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(tgtPath), 0755); err != nil {
		return fmt.Errorf("创建目标目录失败: %w", err)
	}

	if err := os.Rename(srcPath, tgtPath); err != nil {
		if copyErr := copyFile(srcPath, tgtPath); copyErr != nil {
			return fmt.Errorf("移动失败: %w", copyErr)
		}
		if err := os.RemoveAll(srcPath); err != nil {
			LoggerGeneral.SubError("FileManager", "Move", "删除源失败: %s, %v", srcPath, err)
		}
	}
	return nil
}

// autoRenamePath 为存在冲突的目标路径生成不冲突的新路径（追加 " (1)"、" (2)" 序号）
func autoRenamePath(tgtPath string) string {
	dir := filepath.Dir(tgtPath)
	name := filepath.Base(tgtPath)
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

// relativeToLocal 将绝对路径转换为相对 LocalDir 的路径
func relativeToLocal(fullPath, localDir string) string {
	rel, err := filepath.Rel(localDir, fullPath)
	if err != nil {
		return filepath.ToSlash(fullPath)
	}
	return filepath.ToSlash(rel)
}
