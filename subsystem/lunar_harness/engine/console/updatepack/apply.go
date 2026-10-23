package updatepack

// ==== 主程序更新：更新器的定位与拉起 ====
//
// 主程序 exe 与已加载 DLL 在运行期间被系统锁定，无法被自身覆盖。含主程序文件的更新包
// 下载后不就地合并，而是由本文件提供的能力拉起独立的「更新器」（与主程序同层编译于
// 程序根目录）：主程序随后退出释放占用 → 更新器用 7z 解压分卷 → 备份并覆盖 →
// 清理分卷临时目录 → 重启主程序。

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// applierFileName 更新器文件名（与主程序同层位于程序根目录）
var applierFileName = "Lunar_Update_Applier.exe"

// ApplierPath 返回更新器的绝对路径（以主程序可执行文件目录为基准）
func ApplierPath() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), applierFileName)
	}
	return applierFileName
}

// ApplyLogPath 更新器日志路径（与 ApplyProgramUpdate 传给更新器的 -log 一致）。
// 供控制台在交接提示中告知用户去哪里看更新过程。
func ApplyLogPath(cfg Config) string {
	return filepath.Join(ProgramRoot(cfg), "local_data", "logs", "update_apply.log")
}

// ProgramRoot 程序根目录（更新包覆盖目标，绝对路径）
func ProgramRoot(cfg Config) string {
	return resolveTargetDir(cfg)
}

// waitFileArgs 组装「等待解锁」文件清单的参数。
//
// 更新器自身必须排除：运行中的 exe 不允许以写方式打开（镜像加载器不共享写），
// 等自己只会白等到超时（实测空转 60 秒，用户看到的是「程序消失后毫无动静」）；
// 而更新器覆盖自身并不需要它可写——改名/删除只需 DELETE 共享，实测零跳过。
func waitFileArgs(programFiles []string) []string {
	args := make([]string, 0, len(programFiles)*2)
	for _, rel := range programFiles {
		if rel == "" || strings.EqualFold(filepath.Base(rel), applierFileName) {
			continue
		}
		args = append(args, "-wait-file", rel)
	}
	return args
}

// ApplyProgramUpdate 拉起更新器并立即返回；调用方随后应尽快退出本进程以释放文件占用。
// 更新器将等待本进程退出，把 -updatepack 指向的 7z 分卷解压备份并覆盖到程序根目录、
// 清理分卷临时目录、重启主程序。
func ApplyProgramUpdate(res PullResult, cfg Config) error {
	if res.PackPath == "" {
		return fmt.Errorf("缺少更新包分卷路径，无法应用主程序更新")
	}
	src := ApplierPath()
	if !fileExists(src) {
		return fmt.Errorf("缺少更新器 %s（请将其编译到主程序同层目录）", src)
	}

	target := ProgramRoot(cfg)
	args := []string{
		"-updatepack", res.PackPath,
		"-pack-dir", filepath.Dir(res.PackPath),
		"-target", target,
		"-pid", strconv.Itoa(os.Getpid()),
		"-backup-keep", strconv.Itoa(res.BackupKeep),
		"-log", ApplyLogPath(cfg),
	}
	args = append(args, waitFileArgs(res.ProgramFiles)...)
	// 重启命令：以当前可执行文件为准，沿用本进程启动参数 + 配置的附加参数
	if self, err := os.Executable(); err == nil && self != "" {
		args = append(args, "-restart", self)
		for _, a := range os.Args[1:] {
			args = append(args, "-restart-arg", a)
		}
	}
	for _, a := range cfg.RestartArgs {
		args = append(args, "-restart-arg", a)
	}

	cmd := exec.Command(src, args...)
	cmd.Dir = target
	// 更新器的进度接到真实终端：主程序接管 stdout 后自身输出进不了控制台，
	// 但用户必须能看见「更新器已启动 → 正在覆盖 → 已重启」，否则整条链路对他就是黑盒。
	// stdin 也要一并交出去：更新器重启主程序时要把它传回去，重启后的控制台才收得到按键。
	cmd.Stdin = os.Stdin
	if w := consoleWriter(); w != nil {
		cmd.Stdout = w
		cmd.Stderr = w
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动更新器失败: %w", err)
	}
	logf("已拉起更新器（%s）：本进程退出后由其解压覆盖并重启；过程日志 %s", filepath.Base(src), ApplyLogPath(cfg))
	return nil
}
