package Creator

// variable.go 创作体系的常量、工具目录与全局变量。

import (
	"net/http"
	"sync"
	"time"
)

const (
	// Module 日志模块名
	Module = "创作"
	// plannerLabel 调度者角色名
	plannerLabel = "创作.调度者"
	// journalFile 创作规划日志（相对 LocalDir）
	journalFile = "database/life_chain/creative_journal.json"
	// documentsRootRel 文件操作的统一根目录（相对 LocalDir）
	documentsRootRel = "documents"
	// packageRootRel LTP9 包根目录（相对 LocalDir）
	packageRootRel = "package"
	// runtimePackageID 创作运行时的 LTP9 包 ID（也是管线名）
	runtimePackageID = "lunar.life_creative"
	// runtimePackageTitle 运行时包标题
	runtimePackageTitle = "月华.生活.创作"

	// maxPlannerRounds 调度者推理轮次上限
	maxPlannerRounds = 6
	// maxExecutorRounds 单步智能体推理轮次上限
	maxExecutorRounds = 8
	// maxRepeats 同一输出/工具调用重复达到该次数即判定循环输出
	maxRepeats = 3
	// modelTimeout 单次模型调用超时
	modelTimeout = 90 * time.Second
	// runTimeout 整轮创作的总超时
	runTimeout = 20 * time.Minute
	// maxReferences 规划日志保留的参考资料条数
	maxReferences = 40
	// maxErrors 规划日志保留的错误条数
	maxErrors = 20
	// maxMemos 规划日志保留的备忘条数
	maxMemos = 20
	// maxReferenceRunes 单条参考资料最大字符数
	maxReferenceRunes = 600
	// maxFileReadBytes 单次文件读取上限
	maxFileReadBytes = 256 << 10
	// maxDTSReadBytes DTS 文档读取上限
	maxDTSReadBytes = 128 << 10
	// dtsMaxResultRunes 返回给模型的 DTS 内容上限
	dtsMaxResultRunes = 4000
	// defaultModelName 未配置多模态模型时的兜底名称
	defaultModelName = "primary_reasoning"
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
	agentWriter:     "文字创作者",
	agentEngineer:   "工程创作者",
	agentResearcher: "资料查阅者",
	agentRunner:     "脚本运行者",
}

// ==== 工具名 ====

const (
	toolWebSearch      = "web_search"
	toolMemorySearch   = "memory_search"
	toolHandleSearch   = "handle_search"
	toolHandleRead     = "handle_read"
	toolFileList       = "file_list"
	toolFileRead       = "file_read"
	toolFileCreate     = "file_create"
	toolFileRewrite    = "file_rewrite"
	toolFileRemove     = "file_remove"
	toolFileDelete     = "file_delete"
	toolFileMove       = "file_move"
	toolFileReplace    = "file_replace"
	toolFileDeleteText = "file_delete_content"
	toolCodeIndex      = "code_index"
	toolReadDTS        = "read_dts"
	toolSendGroupText  = "send_group_text"
	toolSendGroupFile  = "send_group_file"
	toolRunScript      = "run_ltp9_script"
	toolScriptStatus   = "script_status"
	toolSubmitPlan     = "submit_plan"
)

// plannerTools 调度者可调度的全部能力 + 计划提交。
var plannerTools = []string{
	toolWebSearch, toolMemorySearch, toolHandleSearch, toolHandleRead,
	toolFileList, toolFileRead, toolFileCreate, toolFileRewrite, toolFileRemove,
	toolFileDelete, toolFileMove, toolFileReplace, toolFileDeleteText,
	toolCodeIndex, toolReadDTS, toolSendGroupText, toolSendGroupFile,
	toolRunScript, toolScriptStatus, toolSubmitPlan,
}

// writerTools 文字创作者：文档与文学创作、素材查阅、内容调度。
var writerTools = []string{
	toolWebSearch, toolMemorySearch, toolHandleSearch, toolHandleRead,
	toolFileList, toolFileRead, toolFileCreate, toolFileRewrite, toolFileRemove,
	toolFileDelete, toolFileMove, toolFileReplace, toolFileDeleteText,
	toolCodeIndex, toolReadDTS, toolSendGroupText, toolSendGroupFile,
}

// engineerTools 工程创作者：JS/LTP9 项目与文件、脚本、DTS。
var engineerTools = []string{
	toolWebSearch, toolMemorySearch, toolHandleSearch, toolHandleRead,
	toolFileList, toolFileRead, toolFileCreate, toolFileRewrite, toolFileRemove,
	toolFileDelete, toolFileMove, toolFileReplace, toolFileDeleteText,
	toolCodeIndex, toolReadDTS, toolSendGroupText, toolSendGroupFile,
	toolRunScript, toolScriptStatus,
}

// researcherTools 资料查阅者：只读取证，不改文件。
var researcherTools = []string{
	toolWebSearch, toolMemorySearch, toolHandleSearch, toolHandleRead,
	toolFileList, toolFileRead, toolCodeIndex, toolReadDTS,
}

// runnerTools 脚本运行者：运行、监控与必要的文件读写；同样可随时搜索与查记忆。
var runnerTools = []string{
	toolWebSearch, toolMemorySearch, toolFileList, toolFileRead,
	toolFileCreate, toolFileRewrite, toolReadDTS, toolRunScript,
	toolScriptStatus, toolSendGroupText,
}

var (
	// runMu 串行化创作事件，避免同一管线并发运行
	runMu sync.Mutex

	// httpClient 模型调用客户端
	httpClient = &http.Client{Timeout: modelTimeout}

	// 模型端点只解析一次
	modelOnce sync.Once
	modelName string
	modelURL  string
	modelKey  string

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
