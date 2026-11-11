package agentinfo

// names.go 全部 Go 智能体显示名的唯一来源。
//
// 工具目录、数据导出、故障诊断与推理实况都从这里取名字，禁止在业务包内
// 再写中文显示名。多数名字为四个汉字；「记忆整理」为三字，「分镜规划-*」
// 为同一智能体家族的分阶段角色名，统一使用「家族-阶段」格式。

const (
	// NameVision 视觉理解
	NameVision = "视觉理解"
	// NameArchiveOps 归档操作
	NameArchiveOps = "归档操作"
	// NameDocumentReader 文献解析
	NameDocumentReader = "文献解析"
	// NameFileEditor 文件编辑
	NameFileEditor = "文件编辑"
	// NameWishManager 愿望管理
	NameWishManager = "愿望管理"
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
	// NameLifeMemorizer 记忆整理（三字例外）
	NameLifeMemorizer = "记忆整理"
	// NameDesktopOperator 桌面操作
	NameDesktopOperator = "桌面操作"
	// NameDesktopPerceive 桌面感知
	NameDesktopPerceive = "桌面感知"
	// NameDesktopPlan 桌面规划
	NameDesktopPlan = "桌面规划"
	// NameDesktopExecute 桌面执行
	NameDesktopExecute = "桌面执行"
	// NameSQLWriter 语句生成
	NameSQLWriter = "语句生成"
	// NameStoryboardResearch 分镜规划-资料
	NameStoryboardResearch = "分镜规划-资料"
	// NameStoryboardPlan 分镜规划-拆解
	NameStoryboardPlan = "分镜规划-拆解"
	// NameManipulateFiles 文件操纵
	NameManipulateFiles = "文件操纵"
	// NameManipulateArchive 归档操纵
	NameManipulateArchive = "归档操纵"
	// NameCodeExecution 代码执行
	NameCodeExecution = "代码执行"
	// NameContentRollback 内容回滚
	NameContentRollback = "内容回滚"
	// NameFileOperations 文件操作
	NameFileOperations = "文件操作"
	// NameMemoryTagger 记忆标签
	NameMemoryTagger = "记忆标签"
	// NameMemoryFuser 记忆融合
	NameMemoryFuser = "记忆融合"
	// NameAudioTranscriber 语音转写
	NameAudioTranscriber = "语音转写"
)

// DisplayNames 智能体 ID → 显示名。
var DisplayNames = map[string]string{
	"vision":            NameVision,
	"archive_ops":       NameArchiveOps,
	"document_reader":   NameDocumentReader,
	"wish_manager":      NameWishManager,
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
	"life_memorizer":    NameLifeMemorizer,
	"desktop_operator":  NameDesktopOperator,
	"desktop_perceive":  NameDesktopPerceive,
	"desktop_plan":      NameDesktopPlan,
	"desktop_execute":   NameDesktopExecute,
	"sql_writer":        NameSQLWriter,

	"storyboard_research": NameStoryboardResearch,
	"storyboard_planner":  NameStoryboardPlan,

	"manipulate_files":   NameManipulateFiles,
	"manipulate_archive": NameManipulateArchive,
	"code_execution":     NameCodeExecution,
	"content_rollback":   NameContentRollback,
	"file_operations":    NameFileOperations,

	"memory_tagger":     NameMemoryTagger,
	"memory_fuser":      NameMemoryFuser,
	"audio_transcriber": NameAudioTranscriber,
}

// Name 返回智能体显示名；未登记的 ID 原样返回，便于调用方暴露真实 ID 排查。
func Name(id string) string {
	if name, ok := DisplayNames[id]; ok {
		return name
	}
	return id
}
