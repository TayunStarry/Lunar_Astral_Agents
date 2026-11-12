package main

import "time"

// ==== 事务核心类型 ====

// FileEntry 安装计划中的一个文件：目标相对路径、大小、期望哈希与是否已有旧文件。
// HadOld 在计划阶段一次性确定，回滚因此不依赖每文件的实时状态。
type FileEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	HadOld bool   `json:"had_old"`
}

// Journal 事务日志：写入目标前落盘，记录足以确定性回滚的全部信息。
type Journal struct {
	Schema      int         `json:"schema"`
	Nonce       string      `json:"nonce"`
	Phase       string      `json:"phase"`
	Target      string      `json:"target"`
	FromVersion string      `json:"from_version"`
	ToVersion   string      `json:"to_version"`
	Staging     string      `json:"staging"`
	Backup      string      `json:"backup"`
	CreatedDirs []string    `json:"created_dirs"`
	Files       []FileEntry `json:"files"`
	StateBefore *State      `json:"state_before,omitempty"`
	StateAfter  *State      `json:"state_after,omitempty"`
}

// State 更新器维护的安装状态（.update/state.json）。
type State struct {
	Schema      int    `json:"schema"`
	Version     string `json:"version"`
	InstalledAt string `json:"installed_at"`
	SourceTag   string `json:"source_tag"`
	FileCount   int    `json:"file_count"`
	Bytes       int64  `json:"bytes"`
	LastResult  string `json:"last_result"`
	LastError   string `json:"last_error"`
}

// InstallPlan 已校验完成的安装计划（由解压与清单校验阶段产出）。
type InstallPlan struct {
	Target      string
	Staging     string
	Backup      string
	FromVersion string
	ToVersion   string
	Files       []FileEntry
}

// Progress 事务进度（阶段名 + 已完成/总数 + 百分比 + 说明）。
// Item / State / Detail / ItemSize 只在分卷上传阶段使用，其余阶段为零值。
type Progress struct {
	Phase    string
	Done     int
	Total    int
	Percent  int
	Message  string
	Item     int    // 分卷下标（0 起）
	State    string // 分卷状态：uploading / done / failed
	Detail   string // 分卷附加说明（已传 / 总量、HTTP 状态、错误摘要）
	ItemSize int64  // 分卷字节数
}

// Manifest 更新包内的文件清单（包根目录 .lunar_update_manifest.json）。
type Manifest struct {
	Schema  int         `json:"schema"`
	Version string      `json:"version"`
	BuiltAt string      `json:"built_at"`
	Files   []FileEntry `json:"files"`
}

// VersionBeacon 主程序启动时写出的只读版本信标。
type VersionBeacon struct {
	Version string `json:"version"`
	WriteAt string `json:"write_at"`
}

// ==== 发布与下载类型 ====

// ReleaseConfig 更新器配置（local_data/config/release.json）。
type ReleaseConfig struct {
	GiteeOwner      string   `json:"gitee_owner"`
	GiteeRepo       string   `json:"gitee_repo"`
	GiteeToken      string   `json:"gitee_token"`
	Include         []string `json:"include"`
	ProgramFiles    []string `json:"program_files"`
	Exclude         []string `json:"exclude"`
	ExcludeDirsWith []string `json:"exclude_dirs_with"`
	BackupKeep      int      `json:"backup_keep"`
	RestartArgs     []string `json:"restart_args"`
	TargetDir       string   `json:"target_dir"`
	PackageDir      string   `json:"package_dir"`
	PackSizeMB      int      `json:"pack_size_mb"`
	PackLevel       int      `json:"pack_level"`
	SevenZip        string   `json:"seven_zip"`
}

// GiteeAsset Gitee Release 附件。
type GiteeAsset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// GiteeRelease Gitee Release 原始结构。
type GiteeRelease struct {
	ID        int64        `json:"id"`
	TagName   string       `json:"tag_name"`
	Name      string       `json:"name"`
	Body      string       `json:"body"`
	CreatedAt string       `json:"created_at"`
	Assets    []GiteeAsset `json:"assets"`
}

// ReleaseAsset 仅保留更新器需要的附件信息与声明校验值。
type ReleaseAsset struct {
	Name   string
	Size   int64
	URL    string
	SHA256 string
}

// UpdateRelease 解析后的更新包发布。
type UpdateRelease struct {
	ID        int64
	Tag       string
	Name      string
	Body      string
	CreatedAt string
	Assets    []ReleaseAsset
	Volumes   []string
	Valid     bool
	Problem   string
}

// TransferProgress 下载进度。
type TransferProgress struct {
	Asset   string
	Done    int64
	Total   int64
	Percent int
	Message string
}

// PackResult 打包产物。
type PackResult struct {
	Version   string
	Volumes   []string
	SHAs      map[string]string
	Manifest  *Manifest
	FileCount int
	Bytes     int64
}

// ==== TUI 类型 ====

// procInfo 枚举到的一个进程：PID 与可执行文件完整路径
type procInfo struct {
	PID int
	Exe string
}

// uiKeyHint 一条键位提示（键 + 说明）
type uiKeyHint struct {
	Key   string
	Label string
}

// uiNavRow 侧栏的一行：idx>=0 指向屏幕，idx<0 表示分组标题（group 有效）
type uiNavRow struct {
	idx   int
	group string
}

// uiLogLine 界面日志行
type uiLogLine struct {
	At   time.Time
	Text string
}

// uiInstallState 下载安装屏的状态
type uiInstallState struct {
	Stage         string
	Detail        string
	Percent       int
	Downloaded    int64
	DownloadTotal int64
	FilesDone     int
	FilesTotal    int
	FromVersion   string
	ToVersion     string
	VerifyResult  string
	Err           error
	StartedAt     time.Time
	Pending       bool   // 等待二次确认（降级/重装）
	PendingReason string // 二次确认原因
}

// uiUploadVolume 打包上传屏的单个分卷状态
type uiUploadVolume struct {
	Name   string
	Size   int64
	State  string // pending / uploading / done / failed
	Detail string // 已传 / 总量、HTTP 状态或错误摘要
}

// uiPublishState 打包上传屏的状态
type uiPublishState struct {
	Version   string
	Editing   bool
	Stage     string
	Detail    string
	Percent   int
	Result    *PackResult
	Volumes   []uiUploadVolume
	Err       error
	ErrStage  string
	Busy      bool
	StartedAt time.Time
}

// uiModel bubbletea 模型
type uiModel struct {
	width  int
	height int
	root   string
	cfg    *ReleaseConfig

	screen   int // 当前屏
	cursor   int // 侧栏游标
	showHelp bool
	quit     bool

	curVersion string
	state      *State

	releases   []UpdateRelease
	relIdx     int
	relErr     error
	relLoading bool

	install uiInstallState
	publish uiPublishState

	logs      []uiLogLine
	logFollow bool
	logSeq    int64 // 已读到的内存日志游标（按 Seq 补读，不依赖通道）

	busy      string
	notice    string
	noticeErr bool
	noticeAt  time.Time
	startAt   time.Time

	confirmQuit bool
	events      chan any

	// 主程序存活判定（每秒刷新，总览与安装等待共用同一判定）
	progTargets []string
	progMissing []string
	progPids    []int
	progErr     error

	// 日志滚动（Ctrl+U/Ctrl+D 翻页；follow 为真时贴底）
	logOffset int
}

// ==== TUI 异步消息 ====

// msgReleases 版本列表刷新完成
type msgReleases struct {
	Releases []UpdateRelease
	Err      error
}

// msgInstallStage 安装流水线阶段变化
type msgInstallStage struct {
	Stage   string
	Detail  string
	Percent int
	Verify  string
}

// msgDownload 下载进度
type msgDownload struct {
	Progress TransferProgress
}

// msgInstallProgress 事务进度
type msgInstallProgress struct {
	Progress Progress
}

// msgInstallDone 安装结束
type msgInstallDone struct {
	Err    error
	Detail string
}

// msgPackProgress 打包/上传进度
type msgPackProgress struct {
	Progress Progress
}

// msgPublishDone 打包/上传结束
type msgPublishDone struct {
	Result *PackResult
	Err    error
}

// msgLog TUI 日志唤醒信号：真正的日志行从内存环形缓冲读取，
// 通道拥塞时也不会丢掉任何一条。
type msgLog struct{}

// logEntry 内存日志环形缓冲的一条记录（Seq 单调递增，用于补读）
type logEntry struct {
	Seq  int64
	Line string
}

// GiteePermission 当前令牌对仓库的权限
type GiteePermission struct {
	Push  bool `json:"push"`
	Admin bool `json:"admin"`
}

// GiteeRepoInfo 仓库元信息：预检默认分支与推送权限
type GiteeRepoInfo struct {
	FullName      string           `json:"full_name"`
	DefaultBranch string           `json:"default_branch"`
	Permission    *GiteePermission `json:"permission"`
}

// msgTick 定时刷新（时钟与提示过期）
type msgTick struct{}

// uiFrame 屏幕骨架：标题栏 / 定高内容区 / 键位栏
type uiFrame struct {
	width, height int
	brand         string
	crumbs        string
	title         string
	context       string
	extra         string
	keys          []uiKeyHint
	status        string
	navW          int
	navFn         func(width, height int) []string
}

// uiRowBuilder 定宽行组装器：左片段靠左、右片段靠右，中间自动填充
type uiRowBuilder struct {
	width    int
	selected bool
	left     []string
	right    []string
	sep      int
}
