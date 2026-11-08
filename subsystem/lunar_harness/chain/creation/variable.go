package creation

// variable.go 创作体系的常量、工具目录与全局变量。

import (
	"context"
	"sync"
	"time"
)

const (
	// Module 日志模块名
	Module = "自由创作"
	// plannerLabel 调度者角色名
	plannerLabel = "自由创作.调度者"
	// documentsRootRel 文件操作的统一根目录（相对 LocalDir）
	documentsRootRel = "documents"
	// runtimePackageID 私有 js 运行时标识（也是创作管线名）
	runtimePackageID = "lunar.life_creative"
	// runtimePackageTitle 运行时包标题
	runtimePackageTitle = "月华.生活.自由创作"

	// runTimeout 整轮创作的总超时
	runTimeout = 20 * time.Minute
	// ==== 创作任务状态 ====
	// statusQueued 已受理，等待创作队列
	statusQueued = "queued"
	// statusPlanning 调度者正在生成计划
	statusPlanning = "planning"
	// statusExecuting 正在按计划执行
	statusExecuting = "executing"
	// statusCompleted 已完成
	statusCompleted = "completed"
	// statusFailed 执行失败
	statusFailed = "failed"
	// maxReferences 规划日志保留的参考资料条数
	maxReferences = 40
	// maxErrors 规划日志保留的错误条数
	maxErrors = 20
	// maxMemos 规划日志保留的备忘条数
	maxMemos = 20
	// maxDirectives 规划日志保留的用户要求条数
	maxDirectives = 6
	// maxDirectiveRunes 单条用户要求最大字符数
	maxDirectiveRunes = 512
	// maxReferenceRunes 单条参考资料最大字符数
	maxReferenceRunes = 1200
	// maxSummaryRunes 规划日志「上一轮收尾」的最大字符数
	maxSummaryRunes = 2000
	// maxErrorRunes 单条错误记录最大字符数
	maxErrorRunes = 600
	// maxMemoRunes 单条备忘最大字符数
	maxMemoRunes = 512
	// maxFileReadBytes 单次文件读取上限
	maxFileReadBytes = 256 << 10
	// maxPaintPromptRunes 交给图片绘制的画面描述最大字符数
	maxPaintPromptRunes = 1200
	// maxPaintMetaRunes 画幅 / 风格 / 输出去向等附加约束最大字符数
	maxPaintMetaRunes = 200
	// maxPaintChangeRunes 基于参考图的修改要求最大字符数
	maxPaintChangeRunes = 600
	// maxPaintReferences 单张绘图最多携带的参考图数量
	maxPaintReferences = 4

	// ==== 调度者路径树快照 ====
	// pathSnapshotMaxRunes 单次注入规划提示的路径树正文字符上限（含统计头与截断标注）
	pathSnapshotMaxRunes = 4000
	// pathSnapshotMaxEntries 单次扫描的条目上限；超出即停止，避免超大目录拖慢规划
	pathSnapshotMaxEntries = 2000
	// pathSnapshotTTL 路径快照缓存有效期；写入后会立即失效，这里只兜底外部改动
	pathSnapshotTTL = 30 * time.Second
	// pathSnapshotTruncated 快照超限时的显式标注，避免模型误以为列表完整
	pathSnapshotTruncated = "\n（已截断，其余需用工具查询）"
	// maxDuplicateCandidates 同名检测回传的候选路径上限
	maxDuplicateCandidates = 5
	// paintTimeout 单张图片绘制调用的超时（图片绘制智能体内部还有模型超时）
	paintTimeout = 8 * time.Minute
	// maxTasks 内存中保留的创作任务上限
	maxTasks = 64
	// maxTaskList 单次任务列表查询上限
	maxTaskList = 50
	// fileReadChunkRunes 单次 file_read 返回给模型的默认字符数（可用 offset 分批翻页）
	fileReadChunkRunes = 4000
	// fileReadChunkLimit 单次 file_read 允许的最大字符数
	fileReadChunkLimit = 8000

	// ==== 路由阈值 ====
	// largeFileBytes 单文件超过该字节数时交给文献解析（与文件编辑直读上限对齐）
	largeFileBytes = 256 << 10
	// largeFolderFiles 目录文件数超过该值时判定为大型目录
	largeFolderFiles = 50
	// largeFolderBytes 目录总字节超过该值时判定为大型目录
	largeFolderBytes = 4 << 20
	// largeFolderLargest 目录内最大文件超过该值时判定为大型目录
	largeFolderLargest = 256 << 10
	// maxFolderScanEntries 路由扫描目录时的条目上限
	maxFolderScanEntries = 2000

	// ==== 上下文留存 ====
	// maxContextOperations 注入下一轮提示词的历史操作记录条数
	maxContextOperations = 20
	// maxChangeQuery 单次 change_query 返回的文件变更次数上限（与存储窗口一致）
	maxChangeQuery = 10
	// maxContextDeltas 单条操作在提示词里最多展示的内容差异块数
	maxContextDeltas = 3
	// maxDeltaRunes 单个差异片段在提示词里保留的字符数
	maxDeltaRunes = 512
	// maxAssignedSummaryRunes 指派模式返回摘要的最大字符数
	maxAssignedSummaryRunes = 2000
	// maxExpectsPerStep 单个步骤最多声明的规划预期条数
	maxExpectsPerStep = 8
	// maxExpectFragments 单条预期最多包含的文本片段数
	maxExpectFragments = 8
)

// 可用智能体：全部通过自然语言指令接收任务。
const (
	agentWriter     = "writer"
	agentEngineer   = "engineer"
	agentResearcher = "researcher"
	agentRunner     = "runner"
	// agentPainter 图片绘制：由创作链把绘图需求显式交给现有图片绘制智能体执行
	agentPainter = "painter"
)

// agentLabels 智能体名称 → 中文角色名。
var agentLabels = map[string]string{
	agentWriter:     "文字创作",
	agentEngineer:   "工程创作",
	agentResearcher: "资料查阅",
	agentRunner:     "脚本运行",
	agentPainter:    "图片绘制",
}

// ==== 工具名 ====

const (
	toolWebSearch       = "web_search"
	toolFetchResources  = "fetch_resources"
	toolFileList        = "file_list"
	toolFileRead        = "file_read"
	toolFileCreate      = "file_create"
	toolFileRewrite     = "file_rewrite"
	toolFileRemove      = "file_remove"
	toolFileDelete      = "file_delete"
	toolFileMove        = "file_move"
	toolFileReplace     = "file_replace"
	toolFileDeleteText  = "file_delete_content"
	toolArchiveRead     = "archive_read"
	toolArchiveWrite    = "archive_write"
	toolCodeIndex       = "code_index"
	toolReadDTS         = "read_dts"
	toolSendGroupText   = "send_group_text"
	toolSendGroupFile   = "send_group_file"
	toolSendPrivateText = "send_private_text"
	toolRunScript       = "run_script"
	toolScriptStatus    = "script_status"
	toolChangeQuery     = "change_query"
	toolSubmitPlan      = "submit_plan"
)

// plannerTools 调度者只做节点编排：可读取证 + 提交计划，不直接写文件、不运行脚本。
// 所有写操作必须落到计划步骤，由执行节点经路由 / 快照 / 审查 / 回滚完成。
var plannerTools = []string{
	toolWebSearch, toolFetchResources,
	toolFileList, toolFileRead, toolArchiveRead,
	toolCodeIndex, toolReadDTS, toolChangeQuery, toolSubmitPlan,
}

// pathSnapshotRoots 注入调度者的路径树根目录（相对 LocalDir）；用户明确只要这两个根。
var pathSnapshotRoots = []string{"documents", "unknown"}

// writerTools 文字创作者：文档与文学创作、素材查阅、内容调度。
var writerTools = []string{
	toolWebSearch, toolFetchResources,
	toolFileList, toolFileRead, toolFileCreate, toolFileRewrite, toolFileRemove,
	toolFileDelete, toolFileMove, toolFileReplace, toolFileDeleteText,
	toolArchiveRead, toolArchiveWrite,
	toolCodeIndex, toolReadDTS, toolSendGroupText, toolSendGroupFile, toolSendPrivateText,
}

// engineerTools 工程创作者：JS 项目与文件、脚本、开发文档。
var engineerTools = []string{
	toolWebSearch, toolFetchResources,
	toolFileList, toolFileRead, toolFileCreate, toolFileRewrite, toolFileRemove,
	toolFileDelete, toolFileMove, toolFileReplace, toolFileDeleteText,
	toolArchiveRead, toolArchiveWrite,
	toolCodeIndex, toolReadDTS, toolSendGroupText, toolSendGroupFile,
	toolRunScript, toolScriptStatus,
}

// researcherTools 资料查阅者：只读取证，不改文件。
var researcherTools = []string{
	toolWebSearch, toolFetchResources,
	toolFileList, toolFileRead, toolArchiveRead, toolCodeIndex, toolReadDTS,
}

// runnerTools 脚本运行者：运行、监控与必要的文件读写；同样可随时搜索与查记忆。
var runnerTools = []string{
	toolWebSearch, toolFetchResources, toolFileList, toolFileRead,
	toolFileCreate, toolFileRewrite, toolArchiveRead, toolArchiveWrite, toolReadDTS, toolRunScript,
	toolScriptStatus, toolSendGroupText, toolSendPrivateText,
}

var (
	// WebSearchHook 联网搜索实现，由装配层注入（避免创作链直接依赖检索链）
	WebSearchHook func(query string) (string, error)

	// PaintHook 图片绘制实现，由装配层注入现有图片绘制智能体
	PaintHook func(ctx context.Context, req PaintRequest) (PaintResult, error)

	// TaskEventHook 创作任务状态变化回调，由装配层注入（推送到资源库 WebSocket）
	TaskEventHook func(TaskView)

	// runMu 串行化创作事件，避免同一管线并发运行
	runMu sync.Mutex

	// taskMu 保护资源库提交的创作任务表
	taskMu sync.Mutex
	// tasks 任务 ID → 任务状态
	tasks = map[string]*taskState{}
	// taskOrder 按创建顺序保存的任务 ID，便于清理与列表
	taskOrder []string
	// taskSeq 任务序号
	taskSeq uint64

	// lastRunMeta 最近一次脚本运行摘要（供 script_status 汇报）
	lastRunMu   sync.Mutex
	lastRunMeta = map[string]any{}

	// pathSnapshotMu 保护路径树快照缓存
	pathSnapshotMu sync.Mutex
	// pathSnapshotMemo 最近一次路径树扫描结果
	pathSnapshotMemo pathSnapshotCache
)

// toolsFor 返回指定智能体的工具名清单。
func toolsFor(agent string) []string {
	switch agent {
	case agentWriter:
		return writerTools
	case agentEngineer:
		return engineerTools
	case agentResearcher:
		return researcherTools
	case agentRunner:
		return runnerTools
	default:
		return researcherTools
	}
}

// validAgent 判断智能体名是否合法。
func validAgent(name string) bool {
	_, ok := agentLabels[name]
	return ok
}
