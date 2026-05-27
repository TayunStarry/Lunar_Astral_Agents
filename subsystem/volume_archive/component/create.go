package component

import (
	"bufio"
	"fmt"
	"logger"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

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
			logger.Info("VolumeArchive", "跳过排除项: %s", fullPath)
			continue
		}

		if isDir {
			subFiles, err := scanDirectory(fullPath, baseDir)
			if err != nil {
				logger.Error("VolumeArchive", "无法扫描目录 %s: %v", fullPath, err)
				continue
			}
			files = append(files, subFiles...)
		} else {
			relPath, err := filepath.Rel(baseDir, fullPath)
			if err != nil {
				logger.Error("VolumeArchive", "无法计算相对路径 %s: %v", fullPath, err)
				continue
			}
			files = append(files, relPath)
		}
	}

	return files, nil
}

func createVolume(sources []string, outputPath string, partSizeMB int, compressionLevel int) error {
	PrintInfo("开始创建分卷压缩包...\n")
	PrintInfo("源文件数量: %d\n", len(sources))

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

	PrintInfo("基准目录: %s\n", baseDir)

	var relSources []string
	for _, absPath := range absSources {
		info, err := os.Stat(absPath)
		if err != nil {
			return fmt.Errorf("无法获取文件信息 %s: %v", absPath, err)
		}

		if info.IsDir() {
			dirFiles, err := scanDirectory(absPath, baseDir)
			if err != nil {
				PrintWarning("扫描目录失败 %s: %v\n", absPath, err)
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

	PrintInfo("有效文件数量: %d\n", len(relSources))

	listFile := filepath.Join(os.TempDir(), "7z_file_list.txt")
	listContent := strings.Join(relSources, "\n")
	if err := os.WriteFile(listFile, []byte(listContent), 0644); err != nil {
		return fmt.Errorf("无法创建文件列表: %v", err)
	}
	defer os.Remove(listFile)

	sevenZPath, err := find7zPath()
	if err != nil {
		return fmt.Errorf("查找7z工具失败: %v", err)
	}
	PrintInfo("使用7z工具: %s\n", sevenZPath)

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

	PrintInfo("开始压缩...\n")
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

	tracker := NewProgressTracker()
	done := make(chan bool)

	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if !tracker.HasProgress {
					tracker.displayPreparing()
				}
			}
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			tracker.UpdateProgress(line)
		}
	}()

	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()
		tracker.UpdateProgress(line)
	}

	close(done)

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("7z压缩过程失败: %v", err)
	}

	PrintSuccess("压缩完成！输出文件: %s\n", out7z)
	return nil
}
