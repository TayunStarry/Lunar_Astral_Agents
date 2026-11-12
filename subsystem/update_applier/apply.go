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
	"time"
)

// openLog 打开日志文件（追加模式）。
// 目录创建或文件打开失败必须返回明确原因：静默回退会让「报错但查不到日志」再次发生。
func openLog(path string) error {
	if path == "" {
		return nil
	}
	dir := filepath.Dir(path)
	if dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建日志目录失败（%s）：%w", dir, err)
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开日志文件失败（%s）：%w", path, err)
	}
	logFile = f
	logFilePath = path
	return nil
}

// logf 统一日志出口（时间戳 + logSink：TUI 接管或写标准输出，并落盘日志文件）
func logf(format string, v ...any) {
	msg := time.Now().Format("2006-01-02 15:04:05") + "  " + fmt.Sprintf(format, v...)
	logSink(msg)
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
