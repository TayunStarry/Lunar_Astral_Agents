package module

import (
	"LunarSubsystem/LoggerGeneral"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// HashRenameDir 对指定目录（相对 LocalDir）下的全部文件执行哈希命名
// 命名规则：文件名 = 文件内容 MD5 的前 16 位十六进制字符 + 原扩展名；
// 遇到重名（内容相同或与已有条目冲突）时，在文件名后追加 '+' 直至不冲突。
// 仅处理当前层级的文件，不递归子目录；目录条目本身不重命名。
func HashRenameDir(localDir string, relPath string) (HashRenameResponse, error) {
	response := HashRenameResponse{Success: false, Results: []HashRenameItem{}}

	baseDir := filepath.Clean(localDir)
	dirPath := filepath.Clean(filepath.Join(baseDir, filepath.FromSlash(relPath)))
	if dirPath != baseDir && !strings.HasPrefix(dirPath, baseDir+string(os.PathSeparator)) {
		response.Error = "路径越界"
		return response, fmt.Errorf("路径越界: %s", relPath)
	}
	info, err := os.Stat(dirPath)
	if err != nil {
		response.Error = "目录不存在"
		return response, err
	}
	if !info.IsDir() {
		response.Error = "路径不是目录"
		return response, fmt.Errorf("路径不是目录: %s", relPath)
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		response.Error = "读取目录失败"
		return response, err
	}

	// 已占用的文件名集合：目录内所有现有条目（含文件夹）+ 本次批量中已生成的新名字
	usedNames := make(map[string]bool, len(entries))
	for _, entry := range entries {
		usedNames[entry.Name()] = true
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue // 仅处理文件
		}
		fullPath := filepath.Join(dirPath, entry.Name())

		hash16, err := md5First16(fullPath)
		if err != nil {
			response.Error = fmt.Sprintf("计算哈希失败: %s: %v", entry.Name(), err)
			return response, err
		}

		ext := filepath.Ext(entry.Name())
		// 已是哈希名（hash16 + 扩展名，可带若干个 '+'）则跳过，避免重复计算与无意义重命名
		if isHashName(entry.Name(), hash16, ext) {
			response.Results = append(response.Results, HashRenameItem{
				OldName:   entry.Name(),
				NewName:   entry.Name(),
				Hash:      hash16,
				Unchanged: true,
			})
			continue
		}

		// 重名处理：在文件名后追加 '+' 直至不冲突
		base := hash16
		for usedNames[base+ext] {
			base += "+"
		}
		newName := base + ext
		duplicate := base != hash16

		if err := os.Rename(fullPath, filepath.Join(dirPath, newName)); err != nil {
			response.Error = fmt.Sprintf("重命名失败: %s -> %s: %v", entry.Name(), newName, err)
			return response, err
		}
		usedNames[newName] = true
		response.Renamed++
		response.Results = append(response.Results, HashRenameItem{
			OldName:   entry.Name(),
			NewName:   newName,
			Hash:      hash16,
			Duplicate: duplicate,
		})
	}

	response.Success = true
	LoggerGeneral.SubInfo("FileManager", "HashRename", "哈希命名完成: %s, 重命名 %d 个文件", relPath, response.Renamed)
	return response, nil
}

// isHashName 判断文件名是否已是哈希名（hash16 开头，其后为原扩展名或若干个 '+' + 原扩展名）
func isHashName(name, hash16, ext string) bool {
	if !strings.HasPrefix(name, hash16) {
		return false
	}
	rest := name[len(hash16):]
	if !strings.HasSuffix(rest, ext) {
		return false
	}
	mid := strings.TrimSuffix(rest, ext)
	return mid == "" || strings.Trim(mid, "+") == ""
}

// md5First16 计算文件内容的 MD5 并返回前 16 位十六进制字符
func md5First16(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}
