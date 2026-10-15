package main

import (
	"flag"
	"os"
	"path/filepath"
	"time"
)

// ==== 更新器常量与参数集中区 ====

const (
	// defaultBackupKeep 未显式配置时的备份保留次数
	defaultBackupKeep = 3
	// defaultWaitTimeout 等待主程序文件解锁的默认超时（秒）
	defaultWaitTimeout = 60
	// fileMoveAttempts 单文件搬迁的尝试次数（应对主程序退出/句柄释放延迟）
	fileMoveAttempts = 40
	// fileMoveInterval 单文件搬迁重试间隔
	fileMoveInterval = 250 * time.Millisecond
	// unlockPollInterval 主程序文件解锁探测的轮询间隔
	unlockPollInterval = 300 * time.Millisecond
)

// defaultTarget 程序根目录默认值：更新器与主程序同层编译，取自身所在目录
func defaultTarget() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	return "."
}

var (
	// packPath 更新包 7z 分卷起始卷（.7z.001）路径；空 = 未以运行参数启动（直接退出）
	packPath = flag.String("updatepack", "", "更新包 7z 分卷起始卷（.7z.001）路径（必填）")
	// packDir 更新包分卷所在的临时目录（应用完成后整体清理；空则不清理）
	packDir = flag.String("pack-dir", "", "更新包分卷所在的临时目录（应用完成后清理）")
	// targetDir 覆盖目标（程序根目录，默认取自身所在目录——与主程序同层）
	targetDir = flag.String("target", defaultTarget(), "程序根目录（覆盖目标，默认为自身所在目录）")
	// waitPID 主程序进程号（仅记录，用于日志排查）
	waitPID = flag.Int("pid", 0, "主程序进程号（仅用于日志）")
	// backupKeep 覆盖前备份的保留次数（<=0 表示不备份）
	backupKeep = flag.Int("backup-keep", defaultBackupKeep, "覆盖前备份的保留次数（<=0 不备份）")
	// waitTimeout 等待主程序文件解锁的超时（秒）
	waitTimeout = flag.Int("timeout", defaultWaitTimeout, "等待主程序文件解锁的超时（秒）")
	// restartExe 应用完成后重启的主程序路径（空则不重启）
	restartExe = flag.String("restart", "", "应用完成后重启的主程序路径（空则不重启）")
	// logPath 日志文件路径（空时写入 {target}/local_data/logs/update_apply.log）
	logPath = flag.String("log", "", "日志文件路径（空时写入 target/local_data/logs/update_apply.log）")
)

// waitFiles 必须先解锁的主程序文件（相对 target 的路径，如 Lunar_Astral_Agents.exe）
var waitFiles repeatableFlag

// restartArgs 重启主程序时携带的命令行参数
var restartArgs repeatableFlag

// logFile 日志文件句柄（打开失败时为 nil，仅回退标准输出）
var logFile *os.File

func init() {
	flag.Var(&waitFiles, "wait-file", "等待解锁的主程序文件（相对 target，可重复）")
	flag.Var(&restartArgs, "restart-arg", "重启主程序时的命令行参数（可重复）")
}
