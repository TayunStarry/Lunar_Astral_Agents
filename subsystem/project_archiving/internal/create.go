package internal

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

	// 读取目录内容
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		fullPath := filepath.Join(dir, entry.Name())

		if entry.IsDir() {
			// 递归扫描子目录
			subFiles, err := scanDirectory(fullPath, baseDir)
			if err != nil {
				fmt.Printf("WARNING: 无法扫描目录 %s: %v\n", fullPath, err)
				continue
			}
			files = append(files, subFiles...)
		} else {
			// 检查文件扩展名，排除.ts和.go文件
			ext := filepath.Ext(entry.Name())
			if ext != ".ts" && ext != ".go" {
				// 计算相对路径
				relPath, err := filepath.Rel(baseDir, fullPath)
				if err != nil {
					fmt.Printf("WARNING: 无法计算相对路径 %s: %v\n", fullPath, err)
					continue
				}
				files = append(files, relPath)
			}
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
	// 初始化绝对路径源文件列表
	var absSources []string
	// 遍历所有源文件，转换为绝对路径并检查文件是否存在
	for _, src := range sources {
		// 转换为绝对路径
		absSrc, err := filepath.Abs(src)
		// 检查转换是否成功
		if err != nil {
			fmt.Printf("WARNING: 无法获取绝对路径 %s: %v\n", src, err)
			continue
		}
		// 检查文件是否存在
		if !fileExists(absSrc) {
			fmt.Printf("WARNING: %s 不存在，跳过\n", src)
			continue
		}
		// 写入有效源文件到列表
		absSources = append(absSources, absSrc)
	}
	// 如果没有有效的源文件，返回错误
	if len(absSources) == 0 {
		return fmt.Errorf("错误: 没有有效的源文件")
	}
	// 以第一个有效源文件的目录作为基准目录
	baseDir := filepath.Dir(absSources[0])
	fmt.Printf("基准目录: %s\n", baseDir)
	// 检查基准目录是否存在
	if !fileExists(baseDir) {
		return fmt.Errorf("基准目录不存在: %s", baseDir)
	}
	// 初始化相对路径源文件列表
	var relSources []string
	// 遍历所有绝对路径源文件，计算相对于基准目录的相对路径
	for _, src := range absSources {
		// 检查是否为目录
		if info, err := os.Stat(src); err == nil && info.IsDir() {
			// 递归扫描目录，过滤文件
			scannedFiles, err := scanDirectory(src, baseDir)
			if err != nil {
				fmt.Printf("WARNING: 无法扫描目录 %s: %v\n", src, err)
				continue
			}
			// 追加扫描到的文件到列表
			relSources = append(relSources, scannedFiles...)
		} else {
			// 检查文件扩展名，排除.ts和.go文件
			ext := filepath.Ext(src)
			if ext != ".ts" && ext != ".go" {
				// 计算相对于基准目录的相对路径
				rel, err := filepath.Rel(baseDir, src)
				// 检查计算是否成功
				if err != nil {
					fmt.Printf("WARNING: 无法计算相对路径 %s: %v，将使用绝对路径\n", src, err)
					rel = src
				}
				// 写入有效相对路径源文件到列表
				relSources = append(relSources, rel)
			}
		}
	}
	// 如果没有有效的源文件，返回错误
	if len(relSources) == 0 {
		return fmt.Errorf("错误: 没有有效的源文件")
	}
	// 获取输出压缩包的绝对路径
	out7z, err := filepath.Abs(outBase + ".7z")
	// 检查输出路径解析是否成功
	if err != nil {
		return fmt.Errorf("无法解析输出路径: %v", err)
	}
	// 打印最终输出路径
	fmt.Printf("输出文件: %s\n", out7z)
	// 获取输出目录并创建（如果不存在）
	outDir := filepath.Dir(out7z)
	// 检查输出目录是否存在，不存在则创建
	mkdirErr := os.MkdirAll(outDir, 0755)
	// 检查目录创建是否成功
	if mkdirErr != nil {
		return fmt.Errorf("无法创建输出目录: %v", mkdirErr)
	}
	// 构建7-Zip命令参数
	cmdArgs := []string{
		// 添加文件到压缩包
		"a",
		// 设置分卷大小
		fmt.Sprintf("-v%dm", partSizeMB),
		// 设置压缩级别
		fmt.Sprintf("-mx=%d", compressionLevel),
		// 启用多线程压缩
		"-mmt",
		// 设置日志级别为1（显示进度）
		"-bb1",
		// 将进度信息输出到标准输出
		"-bso1",
		// 将进度信息显示在标准输出
		"-bsp1",
		// 输出压缩包路径
		out7z,
		// 追加所有有效相对路径源文件到命令参数
	}
	// 追加所有有效相对路径源文件到命令参数
	cmdArgs = append(cmdArgs, relSources...)
	// 打印最终命令参数（仅在开发模式下）
	if *SystemDevMode {
		fmt.Printf("执行命令: %s %s\n", sevenZipPath, strings.Join(cmdArgs, " "))
	}
	// 创建命令对象
	cmd := exec.Command(sevenZipPath, cmdArgs...)
	// 设置命令的工作目录为基准目录
	cmd.Dir = baseDir
	// 获取命令的标准输出管道
	stdout, err := cmd.StdoutPipe()
	// 检查标准输出管道是否创建成功
	if err != nil {
		return fmt.Errorf("无法获取标准输出管道: %v", err)
	}
	// 获取命令的标准错误管道
	stderr, err := cmd.StderrPipe()
	// 检查标准错误管道是否创建成功
	if err != nil {
		return fmt.Errorf("无法获取标准错误管道: %v", err)
	}
	// 启动命令
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动命令失败: %v", err)
	}
	// 启动协程读取标准输出
	go readOutput(stdout, false)
	// 启动协程读取标准错误输出
	go readOutput(stderr, true)
	// 等待命令执行完成
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("7z命令执行失败: %v", err)
	}
	// 命令执行成功，返回 nil
	return nil
}
