package component

// 导入必要的包
import (
	"fmt"           // 用于格式化输入输出
	"os"            // 提供与操作系统交互的功能
	"path/filepath" // 用于处理文件路径
	"strings"       // 提供字符串操作的功能
)

// cleanOldParts 清理旧的分卷文件
// 参数:
// outBase - 输出文件的基础路径
// 功能: 清理指定目录下与基础文件名相关的旧分卷文件和完整的.7z文件
func cleanOldParts(outBase string) {
	// 获取输出文件所在的目录
	dirName := filepath.Dir(outBase)
	// 获取输出文件的基础名称
	baseName := filepath.Base(outBase)
	// 如果目录名为空，默认使用当前目录
	if dirName == "" {
		dirName = "."
	}
	// 检查目录是否存在
	if !fileExists(dirName) {
		// 若目录不存在，则尝试创建目录
		if err := os.MkdirAll(dirName, 0755); err != nil {
			// 创建失败时打印错误信息
			fmt.Printf("创建目录失败 %s: %v\n", dirName, err)
		}
		// 目录创建失败或已创建，直接返回
		return
	}
	// 记录已删除的文件数量
	removed := 0
	// 读取目录下的所有条目
	entries, err := os.ReadDir(dirName)
	if err != nil {
		// 读取目录失败时打印错误信息
		fmt.Printf("读取目录失败 %s: %v\n", dirName, err)
		return
	}
	// 遍历目录下的所有条目
	for _, entry := range entries {
		// 跳过目录条目
		if entry.IsDir() {
			continue
		}
		// 获取文件名称
		fname := entry.Name()
		// 拼接文件的完整路径
		fullPath := filepath.Join(dirName, fname)
		// 检查文件是否为旧的分卷文件
		if strings.HasPrefix(fname, baseName) && strings.Contains(fname, ".7z.") {
			// 尝试删除文件
			if err := os.Remove(fullPath); err != nil {
				// 删除失败时打印错误信息
				fmt.Printf("删除文件失败 %s: %v\n", fullPath, err)
			} else {
				// 删除成功，增加计数并打印删除信息
				removed++
				fmt.Printf("已删除旧文件: %s\n", fullPath)
			}
		}
	}
	// 拼接完整的.7z文件路径
	full7z := filepath.Join(dirName, baseName+".7z")
	// 检查完整的.7z文件是否存在
	if fileExists(full7z) {
		// 尝试删除完整的.7z文件
		if err := os.Remove(full7z); err != nil {
			// 删除失败时打印错误信息
			fmt.Printf("删除文件失败 %s: %v\n", full7z, err)
		} else {
			// 删除成功，增加计数并打印删除信息
			removed++
			fmt.Printf("已删除旧文件: %s\n", full7z)
		}
	}
	// 如果有文件被删除，打印清理完成信息
	if removed > 0 {
		fmt.Printf("清理完成，删除 %d 个旧分卷\n", removed)
	}
}
