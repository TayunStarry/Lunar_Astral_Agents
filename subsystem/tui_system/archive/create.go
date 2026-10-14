package archive

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// scanDirectory 递归扫描目录，跳过排除项，返回相对于 baseDir 的路径列表
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
			logf("  跳过排除项: %s", fullPath)
			continue
		}

		if isDir {
			subFiles, err := scanDirectory(fullPath, baseDir)
			if err != nil {
				logf("  [WARN] 无法扫描目录 %s: %v", fullPath, err)
				continue
			}
			files = append(files, subFiles...)
		} else {
			relPath, err := filepath.Rel(baseDir, fullPath)
			if err != nil {
				logf("  [WARN] 无法计算相对路径 %s: %v", fullPath, err)
				continue
			}
			files = append(files, relPath)
		}
	}

	return files, nil
}

// createVolume 创建分卷压缩包：扫描源文件 → 生成文件列表 → 调用 7z 压缩
func createVolume(sources []string, outputPath string, partSizeMB int, compressionLevel int) error {
	logf("开始创建分卷压缩包...")
	logf("  源文件/目录数量: %d", len(sources))

	var absSources []string
	for _, src := range sources {
		abs, err := filepath.Abs(src)
		if err != nil {
			return fmt.Errorf("无效路径 %s: %v", src, err)
		}
		if !fileExists(abs) {
			return fmt.Errorf("文件不存在: %s", abs)
		}
		absSources = append(absSources, abs)
	}

	baseDir, err := GetBaseDir(absSources)
	if err != nil {
		return fmt.Errorf("无法确定基准目录: %v", err)
	}

	logf("  基准目录: %s", baseDir)

	var relSources []string
	for _, absPath := range absSources {
		info, err := os.Stat(absPath)
		if err != nil {
			return fmt.Errorf("无法获取文件信息 %s: %v", absPath, err)
		}

		if info.IsDir() {
			dirFiles, err := scanDirectory(absPath, baseDir)
			if err != nil {
				logf("  [WARN] 扫描目录失败 %s: %v", absPath, err)
				continue
			}
			relSources = append(relSources, dirFiles...)
		} else {
			relPath, err := filepath.Rel(baseDir, absPath)
			if err != nil {
				return fmt.Errorf("无法计算相对路径 %s: %v", absPath, err)
			}
			relSources = append(relSources, relPath)
		}
	}

	if len(relSources) == 0 {
		return fmt.Errorf("没有找到有效的文件")
	}

	logf("  有效文件数量: %d", len(relSources))

	// 写入临时文件列表（避免命令行参数长度限制）
	listFile := filepath.Join(os.TempDir(), "7z_file_list.txt")
	listContent := strings.Join(relSources, "\n")
	if err := os.WriteFile(listFile, []byte(listContent), 0644); err != nil {
		return fmt.Errorf("无法创建文件列表: %v", err)
	}
	defer os.Remove(listFile)

	// 查找 7z 工具
	sevenZPath, err := find7zPath()
	if err != nil {
		return fmt.Errorf("查找7z工具失败: %v", err)
	}
	logf("  使用7z工具: %s", sevenZPath)

	out7z := outputPath + ".7z"
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

	logf("开始压缩...")
	cmd := exec.Command(sevenZPath, cmdArgs...)
	cmd.Dir = baseDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("无法获取标准输出: %v", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("无法获取标准错误: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动7z进程失败: %v", err)
	}

	tracker := newProgressTracker()
	done := make(chan bool)

	// 准备阶段提示 goroutine
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if !tracker.hasProgress {
					tracker.reportPreparing()
				}
			}
		}
	}()

	// stdout 解析 goroutine
	go func() {
		stdScanner := bufio.NewScanner(stdout)
		for stdScanner.Scan() {
			tracker.updateProgress(stdScanner.Text())
		}
		_ = stdScanner.Err()
	}()

	// stderr 解析 goroutine
	go func() {
		errScanner := bufio.NewScanner(stderr)
		for errScanner.Scan() {
			tracker.updateProgress(errScanner.Text())
		}
		_ = errScanner.Err()
	}()

	// 等待子进程结束
	if err := cmd.Wait(); err != nil {
		close(done)
		return fmt.Errorf("7z压缩过程失败: %v", err)
	}

	close(done)

	logf("  压缩完成！输出文件: %s", out7z)
	return nil
}

// GetBaseDir 获取源文件列表的基准目录（第一个源文件所在的目录）
func GetBaseDir(sources []string) (string, error) {
	if len(sources) == 0 {
		return "", fmt.Errorf("源文件列表为空")
	}

	firstSource := sources[0]
	absPath, err := filepath.Abs(firstSource)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return "", err
	}

	if info.IsDir() {
		return absPath, nil
	}

	return filepath.Dir(absPath), nil
}

// nowTime / sinceTime 时间辅助（便于统一格式化）
func nowTime() time.Time { return time.Now() }

func sinceTime(t time.Time) time.Duration { return time.Since(t) }
