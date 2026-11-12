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
	// defaultWaitTimeout 等待主程序退出的默认超时（秒）
	defaultWaitTimeout = 300
	// fileMoveAttempts 单文件搬迁的尝试次数（应对主程序退出/句柄释放延迟）
	fileMoveAttempts = 40
	// fileMoveInterval 单文件搬迁重试间隔
	fileMoveInterval = 250 * time.Millisecond
	// unlockPollInterval 主程序文件解锁探测的轮询间隔
	unlockPollInterval = 300 * time.Millisecond
	// lockHeartbeat 持有单实例锁时刷新时间戳的间隔（避免长时间开窗被误判为过期锁）
	lockHeartbeat = 2 * time.Minute
)

// ==== 事务核心常量 ====

const (
	// updateDirName 更新器工作区目录名（位于程序根目录，与目标同卷）
	updateDirName = ".update"
	// updateStateFile 安装状态文件名
	updateStateFile = "state.json"
	// updateJournalFile 事务日志文件名
	updateJournalFile = "journal.json"
	// applyLockFile 单实例锁文件名
	applyLockFile = "apply.lock"
	// updateSchema 事务日志与状态的版本号
	updateSchema = 1
	// manifestFileName 更新包内文件清单的保留名（位于包根目录）
	manifestFileName = ".lunar_update_manifest.json"
	// versionBeaconRel 主程序版本信标相对路径（主程序启动时只读写出）
	versionBeaconRel = "local_data/config/version.json"
	// staleWorkAge 无事务日志时可清理的下载/解压残留年龄
	staleWorkAge = 24 * time.Hour
	// backupDirName 备份子目录名
	backupDirName = "backup"
	// stagingDirName 解压子目录名
	stagingDirName = "staging"
	// downloadDirName 下载子目录名
	downloadDirName = "download"
	// trashDirName 回滚暂存子目录名
	trashDirName = "trash"
)

// ==== 事务阶段 ====

const (
	// phasePrepared 计划已落盘，尚未改动目标
	phasePrepared = "prepared"
	// phaseBackup 正在把旧文件移入备份
	phaseBackup = "backup"
	// phaseApplying 正在把新文件移入目标
	phaseApplying = "applying"
	// phaseCommitted 新文件与状态均已落盘
	phaseCommitted = "committed"
	// phaseRollingBack 正在回滚
	phaseRollingBack = "rolling_back"
	// phaseRolledBack 回滚完成
	phaseRolledBack = "rolled_back"
)

// ==== TUI 常量 ====

const (
	// uiBrand 标题栏品牌名
	uiBrand = "星月智能 · 更新器"
	// uiLogLimit 内存日志环形缓冲上限
	uiLogLimit = 500
	// uiNoticeTTL 标题栏结果提示保留时长
	uiNoticeTTL = 8 * time.Second
	// uiFrameMinWidth / uiFrameMinHeight 框体可渲染的最小终端尺寸
	uiFrameMinWidth  = 56
	uiFrameMinHeight = 14
)

// 屏索引（同时作为侧栏顺序）
const (
	screenOverview = iota
	screenReleases
	screenInstall
	screenPublish
	screenLogs
	screenCount
)

// screenNames 侧栏与标题栏使用的屏名
var screenNames = [screenCount]string{"总览", "版本列表", "下载安装", "打包上传", "日志"}

// screenKeys 侧栏与帮助页使用的快捷键标记
var screenKeys = [screenCount]string{"1", "2", "3", "4", "5"}

// screenGroups 侧栏分组（与主控制台侧栏同样按组插入标题）
var screenGroups = [screenCount]string{"概览", "更新", "更新", "更新", "系统"}

// uiGlobalKeys 全局键位：与主控制台 engine/console 的 globalKeys 逐条对齐，
// 底栏与帮助页共用这一份数据，避免两处失配。
var uiGlobalKeys = []uiKeyHint{
	{"↑/↓ k/j", "移动选择"},
	{"PgUp/PgDn", "快速换页（切上一屏 / 下一屏）"},
	{"Ctrl+U/Ctrl+D", "屏内翻页（日志 / 列表）"},
	{"Home/End g/G", "跳到首 / 尾"},
	{"Enter", "确认 / 进入"},
	{"Esc", "返回总览"},
	{"[ / ]", "按侧栏顺序切上一屏 / 下一屏"},
	{"1-5", "直达对应屏"},
	{"?", "键位帮助"},
	{"X", "退出程序（大小写均可，输入框聚焦时让位）"},
	{"Ctrl+C", "强制退出程序"},
}

// 侧栏显示条件：与主控制台一致，窄终端收起，避免挤占内容区
const (
	uiNavShowFrom  = 96
	uiNavMinHeight = 10
	uiNavWidthMin  = 18
	uiNavWidthMax  = 26
)

// installStage* 安装流水线阶段（界面展示用，与事务阶段区分）
const (
	stageIdle       = "idle"
	stageChecking   = "checking"
	stageDownload   = "download"
	stageArchive    = "archive"
	stageExtracting = "extract"
	stageManifest   = "manifest"
	stageWaiting    = "waiting"
	stagePrepared   = "prepared"
	stageBackup     = "backup"
	stageApplying   = "applying"
	stageCommitted  = "committed"
	stageRolledBack = "rolled_back"
	stageFailed     = "failed"
)

// installSteps 安装流水线步骤（九阶段）
var installSteps = []string{"查询", "下载", "校验分卷", "解压", "清单校验", "等待退出", "备份", "覆盖", "完成"}

// installStepIndex 安装阶段 → 流水线步骤下标（-1 表示未开始）
var installStepIndex = map[string]int{
	stageChecking:   0,
	stageDownload:   1,
	stageArchive:    2,
	stageExtracting: 3,
	stageManifest:   4,
	stageWaiting:    5,
	stagePrepared:   6,
	stageBackup:     6,
	stageApplying:   7,
	stageCommitted:  8,
}

// defaultTarget 程序根目录默认值：更新器与主程序同层编译，取自身所在目录
func defaultTarget() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	return "."
}

var (
	// targetDir 覆盖目标（程序根目录，默认取自身所在目录——与主程序同层）
	targetDir = flag.String("target", defaultTarget(), "程序根目录（覆盖目标，默认为自身所在目录）")
	// logPath 日志文件路径（空时写入 {target}/local_data/logs/update_apply.log）
	logPath = flag.String("log", "", "日志文件路径（空时写入 target/local_data/logs/update_apply.log）")
	// headless 不进入 TUI：用于运维脚本（配合 -pack / -publish）
	headless = flag.Bool("headless", false, "不进入 TUI，按 -pack / -publish 执行一次后退出")
	// packOnly 仅打包（不联网）
	packOnly = flag.Bool("pack", false, "仅生成更新包分卷与清单，不上传")
	// publishNow 打包并上传到 Gitee Release
	publishNow = flag.Bool("publish", false, "打包并上传到 Gitee Release")
	// versionFlag 打包/发布使用的版本号（空时自动生成）
	versionFlag = flag.String("version", "", "打包/发布使用的版本号（空时自动生成）")
	// waitTimeout 等待主程序退出的超时（秒）
	waitTimeout = flag.Int("timeout", defaultWaitTimeout, "等待主程序退出的超时（秒）")
)

var (
	// backupKeep 覆盖前备份的保留次数（启动时由 release.json 的 backup_keep 覆盖）
	backupKeep = defaultBackupKeep
	// logFile 日志文件句柄（打开失败时为 nil，仅回退标准输出）
	logFile *os.File
	// logHook 日志钩子：TUI 运行时接管日志输出，避免 stdout 污染界面
	logHook func(string)
)

// logSink 输出一条已格式化的日志行（文件始终落盘，控制台由 logHook 决定）
func logSink(line string) {
	if logHook != nil {
		logHook(line)
	} else {
		os.Stdout.WriteString(line + "\n")
	}
	if logFile != nil {
		logFile.WriteString(line + "\n")
	}
}
