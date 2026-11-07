package creation

// variable.go 创作体系的常量、工具目录与全局变量。

import (
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
)

// agentLabels 智能体名称 → 中文角色名。
var agentLabels = map[string]string{
	agentWriter:     "文字创作",
	agentEngineer:   "工程创作",
	agentResearcher: "资料查阅",
	agentRunner:     "脚本运行",
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

	// runMu 串行化创作事件，避免同一管线并发运行
	runMu sync.Mutex

	// lastRunMeta 最近一次脚本运行摘要（供 script_status 汇报）
	lastRunMu   sync.Mutex
	lastRunMeta = map[string]any{}
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
