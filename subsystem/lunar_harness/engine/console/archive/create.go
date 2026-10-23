package archive

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
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

		// 目录级排除：目录内含标记文件（如 package/*/metadata.json 属 LTPX 工具包）时整体跳过
		if isDir {
			if marker, hit := dirHasMarker(fullPath); hit {
				logf("  跳过含 %s 的包目录: %s", marker, fullPath)
				continue
			}
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

// dirHasMarker 判断目录内是否含「目录级排除」标记文件（返回命中的文件名）
func dirHasMarker(dir string) (string, bool) {
	for _, name := range GetExcludeDirMarkers() {
		if name == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return name, true
		}
	}
	return "", false
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

	// stderr 解析 goroutine：进度照常上报，同时保留尾部若干行（失败时并入错误信息，
	// 否则 7z 的真实失败原因会被进度管线吞掉，无从诊断）。
	// 7z 在中文 Windows 上输出 GBK 编码，先转 UTF-8 再扫描。
	gbkReader := transform.NewReader(stderr, simplifiedchinese.GBK.NewDecoder())
	var errTailMu sync.Mutex
	var errTail []string
	go func() {
		errScanner := bufio.NewScanner(gbkReader)
		for errScanner.Scan() {
			line := errScanner.Text()
			errTailMu.Lock()
			errTail = append(errTail, line)
			if len(errTail) > 8 {
				errTail = errTail[len(errTail)-8:]
			}
			errTailMu.Unlock()
			tracker.updateProgress(line)
		}
		_ = errScanner.Err()
	}()

	// 等待子进程结束
	if err := cmd.Wait(); err != nil {
		close(done)
		errTailMu.Lock()
		detail := strings.Join(errTail, " | ")
		errTailMu.Unlock()
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			// 7z 退出码 1 = 存在告警（如个别文件被占用而跳过）：产物可用，
			// 不视为失败，但告警内容必须浮出（可能意味着打包缺文件）
			logf("  [WARN] 7z 报告告警（产物已生成，请核对缺失项）: %s", detail)
			logf("  压缩完成！输出文件: %s", out7z)
			return nil
		}
		if detail != "" {
			return fmt.Errorf("7z压缩过程失败: %v（7z: %s）", err, detail)
		}
		return fmt.Errorf("7z压缩过程失败: %v", err)
	}

	close(done)

	logf("  压缩完成！输出文件: %s", out7z)
	return nil
}

// GetBaseDir 获取源文件列表的公共基准目录（全部源的最近公共祖先）。
// 打包条目按相对该目录的路径记录：源分散在多个子目录时（如更新包的
// package/database/certs），以公共祖先为基准才能保证条目布局一致且不含
// ".." 回溯（含 ".." 的条目在解压侧会被视为可疑路径丢弃）。
// 既有行为兼容：首个源为根目录下的文件时（打包归档配置的典型形态），
// 公共祖先即为其所在目录，与历史行为一致。
func GetBaseDir(sources []string) (string, error) {
	if len(sources) == 0 {
		return "", fmt.Errorf("源文件列表为空")
	}

	absDir := func(p string) string {
		a, err := filepath.Abs(p)
		if err != nil {
			return p
		}
		if info, err := os.Stat(a); err != nil || !info.IsDir() {
			return filepath.Dir(a)
		}
		return a
	}

	base := absDir(sources[0])
	for _, src := range sources[1:] {
		cur := absDir(src)
		for base != cur && !isAncestorOrSelf(base, cur) {
			parent := filepath.Dir(base)
			if parent == base {
				break // 到达盘根
			}
			base = parent
		}
	}
	return base, nil
}

// isAncestorOrSelf 判断 ancestor 是否为 path 的祖先目录或与其相等
func isAncestorOrSelf(ancestor, path string) bool {
	rel, err := filepath.Rel(ancestor, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

// nowTime / sinceTime 时间辅助（便于统一格式化）
func nowTime() time.Time { return time.Now() }

func sinceTime(t time.Time) time.Duration { return time.Since(t) }
