package main

// ==== 更新应用器核心逻辑：解锁等待 / 备份覆盖 / 备份剪枝 / 重启 ====
//
// 主程序 exe 与已加载 DLL 在运行期间被系统锁定，无法被自身覆盖；本工具以独立进程、
// 独立文件运行，等主程序退出释放占用后，再把暂存目录内容备份并覆盖到程序根目录。

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// openLog 打开日志文件（追加模式）；失败时静默回退标准输出
func openLog(path string) {
	if path == "" {
		return
	}
	if dir := filepath.Dir(path); dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		logFile = f
	}
}

// logf 统一日志出口（时间戳 + 标准输出 + 日志文件）
func logf(format string, v ...any) {
	msg := time.Now().Format("2006-01-02 15:04:05") + "  " + fmt.Sprintf(format, v...)
	fmt.Println(msg)
	if logFile != nil {
		fmt.Fprintln(logFile, msg)
	}
}

// fileExists 判断文件/目录是否存在
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// selfPath 当前进程可执行文件的绝对路径（取不到时返回空串）
func selfPath() string {
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return ""
}

// samePath 判断两个路径是否指向同一个文件。
// 用 os.SameFile（Windows 下比较卷序列号 + 文件索引）而非字符串比较：
// 短路径名（8.3）、大小写、符号链接都不会误判——「不等」会把更新器自己
// 当成待解锁文件而白等 60 秒，正是本次要修的问题。
func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	fa, errA := os.Stat(a)
	fb, errB := os.Stat(b)
	if errA != nil || errB != nil {
		return false
	}
	return os.SameFile(fa, fb)
}

// waitForUnlock 轮询等待目标文件可写（Windows 下运行中的 exe/DLL 以写方式打开会返回
// 拒绝访问，退出后即可打开），超时返回最后一次错误。
func waitForUnlock(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last error
	for {
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err == nil {
			_ = f.Close()
			return nil
		}
		last = err
		if time.Now().After(deadline) {
			return last
		}
		time.Sleep(unlockPollInterval)
	}
}

// applyTree 将 staging 内容合并到 target：
//   - keep>0 时，覆盖前先把已有文件备份到 {target}/.backup/<时间戳>/（保留回滚能力）；
//   - 单文件搬迁带重试，应对主程序退出后句柄释放的延迟；
//   - 重试用尽仍失败的文件保留原文件并计入 skipped，不中断其余文件的更新。
func applyTree(staging, target string, keep int) (updated int, skipped []string, err error) {
	var backupDir string
	if keep > 0 {
		backupDir = filepath.Join(target, ".backup", time.Now().Format("20060102-150405"))
	}
	backedUp := 0

	walkErr := filepath.Walk(staging, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if fi.IsDir() {
			return nil
		}
		rel, rErr := filepath.Rel(staging, path)
		if rErr != nil {
			return rErr
		}
		dest := filepath.Join(target, rel)
		if mkErr := os.MkdirAll(filepath.Dir(dest), 0755); mkErr != nil {
			return mkErr
		}

		var backup string
		if backupDir != "" && fileExists(dest) {
			backup = filepath.Join(backupDir, rel)
			if mkErr := os.MkdirAll(filepath.Dir(backup), 0755); mkErr != nil {
				return mkErr
			}
			if mvErr := moveWithRetry(dest, backup); mvErr != nil {
				logf("[WARN] 备份失败，跳过该文件（%s）：%v", rel, mvErr)
				skipped = append(skipped, rel)
				return nil
			}
			backedUp++
		}

		if mvErr := moveWithRetry(path, dest); mvErr != nil {
			if backup != "" {
				_ = os.Rename(backup, dest) // 尽力回滚
			}
			logf("[WARN] 覆盖失败，保留原文件（%s）：%v", rel, mvErr)
			skipped = append(skipped, rel)
			return nil
		}
		updated++
		return nil
	})
	if walkErr != nil {
		return updated, skipped, walkErr
	}
	if backedUp > 0 {
		logf("覆盖前已备份 %d 个文件 → %s", backedUp, backupDir)
	}
	return updated, skipped, nil
}

// moveWithRetry 带重试的文件搬迁（先改名，跨卷退回复制后删除）
func moveWithRetry(src, dest string) error {
	var last error
	for i := 0; i < fileMoveAttempts; i++ {
		if err := moveOnce(src, dest); err == nil {
			return nil
		} else {
			last = err
		}
		time.Sleep(fileMoveInterval)
	}
	return last
}

// moveOnce 单次搬迁：优先改名（同卷零拷贝），跨卷时退回复制后删除源文件
func moveOnce(src, dest string) error {
	if err := os.Rename(src, dest); err == nil {
		return nil
	}
	if err := copyFile(src, dest); err != nil {
		return err
	}
	return os.Remove(src)
}

// copyFile 复制文件并显式关闭两端句柄（Windows 下句柄未释放会阻塞后续删除）
func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		_ = in.Close()
		return err
	}
	_, copyErr := io.Copy(out, in)
	outErr := out.Close()
	inErr := in.Close()
	if copyErr != nil {
		return copyErr
	}
	if outErr != nil {
		return outErr
	}
	return inErr
}

// pruneBackups 只保留最近 n 次备份目录，其余删除（n<=0 时清空全部）。
// 备份目录名形如 20060102-150405，按字典序即时间序排列。
func pruneBackups(targetDir string, n int) {
	root := filepath.Join(targetDir, ".backup")
	entries, err := os.ReadDir(root)
	if err != nil {
		return // 无备份目录
	}
	var stamps []string
	for _, e := range entries {
		if e.IsDir() {
			stamps = append(stamps, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(stamps)))

	removed := 0
	for i := n; i < len(stamps); i++ {
		if err := os.RemoveAll(filepath.Join(root, stamps[i])); err == nil {
			removed++
		}
	}
	if removed > 0 {
		logf("已清理 %d 个过期备份（保留最近 %d 次）", removed, n)
	}
}

// restartProgram 启动目标可执行文件（工作目录设为该文件所在目录，随后更新器自身退出）。
//
// 必须显式接回控制台句柄：os/exec 的 Std* 为 nil 时会把子进程的标准流接到 NUL 设备，
// 于是重启后的主程序「进程在跑、前端窗口也开、控制台却永远空白」（实测故障）——
// 控制台是它的唯一操作入口，看不见也按不动，只能去任务管理器结束进程。
//
// 优先用 CONIN$/CONOUT$ 直接向控制台取句柄：它不依赖继承链（主程序自身已把 stdout
// 换成日志管道，更新器拿到的句柄未必是终端），取不到时再回退继承句柄。
func restartProgram(exe string, args []string) error {
	cmd := exec.Command(exe, args...)
	cmd.Dir = filepath.Dir(exe)

	in, out := consoleHandles()
	if out != nil {
		cmd.Stdout, cmd.Stderr = out, out
	} else {
		logf("[WARN] 未能取得控制台输出句柄（无控制台或以分离方式运行）：重启后的主程序可能没有可见界面")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	}
	if in != nil {
		cmd.Stdin = in
	} else {
		cmd.Stdin = os.Stdin
	}
	return cmd.Start()
}

// consoleHandles 取得当前控制台的输入/输出句柄（Windows 控制台设备名）。
// 任一取不到时对应返回 nil，由调用方回退继承句柄。
//
// 两个都必须 **O_RDWR**（实测踩过）：
//   - 输入：bubbletea 进原始模式要 SetConsoleMode，需写权限，只读打开会报
//     "error making raw: Access is denied"；
//   - 输出：GetConsoleMode / GetConsoleScreenBufferInfo 需读权限，只写打开会报
//     "Access is denied"，子进程据此认为"输出不是终端"而拒绝启动界面。
func consoleHandles() (in *os.File, out *os.File) {
	if f, err := os.OpenFile("CONOUT$", os.O_RDWR, 0); err == nil {
		out = f
	} else if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		out = f
	}
	if f, err := os.OpenFile("CONIN$", os.O_RDWR, 0); err == nil {
		in = f
	} else if f, err := os.OpenFile("CONIN$", os.O_RDONLY, 0); err == nil {
		in = f
	}
	return in, out
}

// extractSevenZ 用 7z 解压更新包到暂存目录（资源释放）。
// 分卷须与起始卷同目录——7z 会自动顺序查找其余分卷（主程序下载流程已保证齐备）。
// 7z 在中文 Windows 上的输出为 GBK 编码，本工具保持纯标准库不做转码，失败时原样透传。
func extractSevenZ(volPath, targetDir string) error {
	sevenZip, err := findSevenZip()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return err
	}
	cmd := exec.Command(sevenZip, "x", "-y", "-o"+targetDir, volPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if detail := strings.TrimSpace(string(out)); detail != "" {
			logf("7z 原始输出：%s", detail)
		}
		return fmt.Errorf("7z 解压失败: %w", err)
	}
	return nil
}

// findSevenZip 查找 7z 可执行文件：发行版自带（程序根下 local_data/package/archive/7z.exe）
// 优先，再退回系统安装路径。返回绝对路径（子进程 cwd 可能不同）。
func findSevenZip() (string, error) {
	rel := filepath.Join("local_data", "package", "archive", "7z.exe")
	candidates := []string{filepath.Join(*targetDir, rel)}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), rel))
	}
	candidates = append(candidates,
		`C:\Program Files\7-Zip\7z.exe`,
		`C:\Program Files (x86)\7-Zip\7z.exe`,
	)
	for _, p := range candidates {
		if fileExists(p) {
			if abs, err := filepath.Abs(p); err == nil {
				return abs, nil
			}
			return p, nil
		}
	}
	return "", fmt.Errorf("未找到 7z 工具（解压更新包需要），请安装 7-Zip 或确认 local_data/package/archive/7z.exe 存在")
}
