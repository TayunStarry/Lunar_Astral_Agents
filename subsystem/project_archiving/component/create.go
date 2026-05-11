package component

import (
	"fmt"           // 用于格式化输出
	"os"            // 用于操作文件系统
	"os/exec"       // 用于执行外部命令
	"path/filepath" // 用于处理文件路径
	"strings"       // 用于字符串操作
)

// scanDirectory 递归扫描目录，返回符合条件的文件列表
// 参数:
//   - dir: 要扫描的目录路径
//   - baseDir: 基准目录，用于计算相对路径
//
// 返回:
//   - 符合条件的相对路径文件列表
func scanDirectory(dir string, baseDir string) ([]string, error) {
	var files []string

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		fullPath := filepath.Join(dir, entry.Name())
		isDir := entry.IsDir()

		if IsExcluded(entry.Name(), isDir) {
			fmt.Printf("跳过排除项: %s\n", fullPath)
			continue
		}

		if isDir {
			subFiles, err := scanDirectory(fullPath, baseDir)
			if err != nil {
				fmt.Printf("WARNING: 无法扫描目录 %s: %v\n", fullPath, err)
				continue
			}
			files = append(files, subFiles...)
		} else {
			relPath, err := filepath.Rel(baseDir, fullPath)
			if err != nil {
				fmt.Printf("WARNING: 无法计算相对路径 %s: %v\n", fullPath, err)
				continue
			}
			files = append(files, relPath)
		}
	}

	return files, nil
}

// createVolume 使用7-Zip工具创建分卷压缩包
// 参数:
//   - sevenZipPath: 7-Zip可执行文件的路径
//   - sources: 要压缩的源文件或目录列表
//   - outBase: 输出压缩包的基础路径（不包含扩展名）
//   - partSizeMB: 分卷大小（单位：MB）
//   - compressionLevel: 压缩级别（0-9）
//
// 返回:
//   - 错误信息，如果操作过程中出现错误
func createVolume(sevenZipPath string, sources []string, outBase string, partSizeMB int, compressionLevel int) error {
	var absSources []string
	for _, src := range sources {
		absSrc, err := filepath.Abs(src)
		if err != nil {
			fmt.Printf("WARNING: 无法获取绝对路径 %s: %v\n", src, err)
			continue
		}
		if !fileExists(absSrc) {
			fmt.Printf("WARNING: %s 不存在，跳过\n", src)
			continue
		}
		absSources = append(absSources, absSrc)
	}
	if len(absSources) == 0 {
		return fmt.Errorf("错误: 没有有效的源文件")
	}
	baseDir := filepath.Dir(absSources[0])
	fmt.Printf("基准目录: %s\n", baseDir)
	if !fileExists(baseDir) {
		return fmt.Errorf("基准目录不存在: %s", baseDir)
	}
	var relSources []string
	for _, src := range absSources {
		if info, err := os.Stat(src); err == nil && info.IsDir() {
			scannedFiles, err := scanDirectory(src, baseDir)
			if err != nil {
				fmt.Printf("WARNING: 无法扫描目录 %s: %v\n", src, err)
				continue
			}
			relSources = append(relSources, scannedFiles...)
		} else {
			fileName := filepath.Base(src)
			if !IsExcluded(fileName, false) {
				rel, err := filepath.Rel(baseDir, src)
				if err != nil {
					fmt.Printf("WARNING: 无法计算相对路径 %s: %v，将使用绝对路径\n", src, err)
					rel = src
				}
				relSources = append(relSources, rel)
			} else {
				fmt.Printf("跳过排除文件: %s\n", src)
			}
		}
	}
	if len(relSources) == 0 {
		return fmt.Errorf("错误: 没有有效的源文件")
	}
	// 创建临时文件列表，避免Windows命令行参数过长的问题
	listFile := filepath.Join(os.TempDir(), "7z_file_list.txt")
	listContent := strings.Join(relSources, "\n")
	if err := os.WriteFile(listFile, []byte(listContent), 0644); err != nil {
		return fmt.Errorf("无法创建文件列表: %v", err)
	}
	defer os.Remove(listFile)
	if *SystemDevMode {
		fmt.Printf("文件列表: %s (包含 %d 个文件)\n", listFile, len(relSources))
	}

	out7z, err := filepath.Abs(outBase + ".7z")
	if err != nil {
		return fmt.Errorf("无法解析输出路径: %v", err)
	}
	fmt.Printf("输出文件: %s\n", out7z)
	outDir := filepath.Dir(out7z)
	mkdirErr := os.MkdirAll(outDir, 0755)
	if mkdirErr != nil {
		return fmt.Errorf("无法创建输出目录: %v", mkdirErr)
	}
	cmdArgs := []string{
		"a",
		fmt.Sprintf("-v%dm", partSizeMB),
		fmt.Sprintf("-mx=%d", compressionLevel),
		"-mmt",
		"-bb1",
		"-bso1",
		"-bsp1",
		out7z,
		"@" + listFile,
	}
	if *SystemDevMode {
		fmt.Printf("执行命令: %s %s\n", sevenZipPath, strings.Join(cmdArgs, " "))
	}
	cmd := exec.Command(sevenZipPath, cmdArgs...)
	cmd.Dir = baseDir
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("无法获取标准输出管道: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("无法获取标准错误管道: %v", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动命令失败: %v", err)
	}
	go readOutput(stdout, false)
	go readOutput(stderr, true)
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("7z命令执行失败: %v", err)
	}
	return nil
}
