package agentinfo

// names.go 全部 Go 智能体显示名的唯一来源。
//
// 工具目录、数据导出、故障诊断与推理实况都从这里取名字，禁止在业务包内
// 再写中文显示名。唯一例外是「记忆整理」为三个字，其余均为四个汉字。

const (
	// NameVision 视觉理解
	NameVision = "视觉理解"
	// NameArchiveOps 归档操作
	NameArchiveOps = "归档操作"
	// NameDocumentReader 文献解析
	NameDocumentReader = "文献解析"
	// NameFileEditor 文件编辑
	NameFileEditor = "文件编辑"
	// NamePlanManager 计划管理
	NamePlanManager = "计划管理"
	// NameDialogueBridging 通讯桥接
	NameDialogueBridging = "通讯桥接"
	// NameMutualForum 互助论坛
	NameMutualForum = "互助论坛"
	// NameResponder 通用应答
	NameResponder = "通用应答"
	// NameSavior 故障诊断
	NameSavior = "故障诊断"
	// NameScreenshoter 屏幕截图
	NameScreenshoter = "屏幕截图"
	// NamePainter 图像绘制
	NamePainter = "图像绘制"
	// NameMusician 音乐演奏
	NameMusician = "音乐演奏"
	// NameSearchPlanner 搜索规划
	NameSearchPlanner = "搜索规划"
	// NameSearchSummarizer 搜索摘要
	NameSearchSummarizer = "搜索摘要"
	// NameViewerScreening 观影阅片
	NameViewerScreening = "观影阅片"
	// NameViewerCensor 观影审查
	NameViewerCensor = "观影审查"
	// NameViewerCritic 观影评论
	NameViewerCritic = "观影评论"
	// NameViewerShare 观影分享
	NameViewerShare = "观影分享"
	// NameViewerRemark 观影观感
	NameViewerRemark = "观影观感"
	// NameDialogue 人格对话
	NameDialogue = "人格对话"
	// NameCreator 创作调度
	NameCreator = "创作调度"
	// NameLifeMemorizer 记忆整理（三字例外）
	NameLifeMemorizer = "记忆整理"
	// NameDesktopOperator 桌面操作
	NameDesktopOperator = "桌面操作"
)

// DisplayNames 智能体 ID → 显示名。
var DisplayNames = map[string]string{
	"vision":            NameVision,
	"archive_ops":       NameArchiveOps,
	"document_reader":   NameDocumentReader,
	"file_editor":       NameFileEditor,
	"plan_manager":      NamePlanManager,
	"dialogue_bridging": NameDialogueBridging,
	"mutual_forum":      NameMutualForum,
	"responder":         NameResponder,
	"savior":            NameSavior,
	"screenshoter":      NameScreenshoter,
	"painter":           NamePainter,
	"musician":          NameMusician,
	"search_planner":    NameSearchPlanner,
	"search_summarizer": NameSearchSummarizer,
	"viewer_screening":  NameViewerScreening,
	"viewer_censor":     NameViewerCensor,
	"viewer_critic":     NameViewerCritic,
	"viewer_share":      NameViewerShare,
	"viewer_remark":     NameViewerRemark,
	"dialogue":          NameDialogue,
	"creator":           NameCreator,
	"life_memorizer":    NameLifeMemorizer,
	"desktop_operator":  NameDesktopOperator,
}

// Name 返回智能体显示名；未登记的 ID 原样返回，便于调用方暴露真实 ID 排查。
func Name(id string) string {
	if name, ok := DisplayNames[id]; ok {
		return name
	}
	return id
}
