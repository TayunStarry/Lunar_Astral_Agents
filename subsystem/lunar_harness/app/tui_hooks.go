package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/CodeExecution"
	"LunarSubsystem/LunarHarness/agents/ContentRollback"
	"LunarSubsystem/LunarHarness/agents/DialogueBridging"
	"LunarSubsystem/LunarHarness/agents/DocumentReader"
	"LunarSubsystem/LunarHarness/agents/FileOperations"
	"LunarSubsystem/LunarHarness/agents/ManipulateArchive"
	"LunarSubsystem/LunarHarness/agents/ManipulateFiles"
	"LunarSubsystem/LunarHarness/agents/Musician"
	"LunarSubsystem/LunarHarness/agents/MutualForum"
	"LunarSubsystem/LunarHarness/agents/Painter"
	"LunarSubsystem/LunarHarness/agents/Savior"
	"LunarSubsystem/LunarHarness/agents/Screenshoter"
	"LunarSubsystem/LunarHarness/agents/SearchPlanner"
	"LunarSubsystem/LunarHarness/agents/SearchSummarizer"
	"LunarSubsystem/LunarHarness/agents/StoryboardPlanner"
	"LunarSubsystem/LunarHarness/agents/StoryboardResearch"
	"LunarSubsystem/LunarHarness/agents/ViewerCensor"
	"LunarSubsystem/LunarHarness/agents/ViewerCritic"
	"LunarSubsystem/LunarHarness/agents/ViewerScreening"
	"LunarSubsystem/LunarHarness/agents/ViewerShare"
	"LunarSubsystem/LunarHarness/agents/Vision"
	"LunarSubsystem/LunarHarness/agents/WishManager"
	"LunarSubsystem/LunarHarness/agents/desktop"
	"LunarSubsystem/LunarHarness/chain/desktop"
	"LunarSubsystem/LunarHarness/chain/search"
	"LunarSubsystem/LunarHarness/chain/thinking"
	"LunarSubsystem/LunarHarness/chain/viewer"
	"LunarSubsystem/LunarHarness/engine/browser"
	"LunarSubsystem/LunarHarness/engine/console"
	"LunarSubsystem/LunarHarness/engine/console/archive"
	module "LunarSubsystem/LunarHarness/engine/multimodal"
	"LunarSubsystem/LunarHarness/engine/websocket"
	"LunarSubsystem/LunarHarness/executors/ArchiveReader"
	"LunarSubsystem/LunarHarness/executors/WebCrawler"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
	"LunarSubsystem/LunarHarness/server"
)

// WireTUIFrontendHooks 将前端在线探针与窗口唤起器注入控制台的前端管理屏：
//   - 月华终端以 bus=0 接入并按 lunarTerminalChannel 声明频道 → 探针取该频道连接数；
//   - 琉璃桌面以 bus=1 接入缺省频道 → 探针取该频道总线订阅数；
//   - 唤起器复用装配层的窗口规格，重建桌面窗口（已存在时由 browser 聚焦）；
//   - 绘图引擎（sd.cpp）日志钩子接入「绘图引擎日志」屏（按行切分后统一排版）。
func WireTUIFrontendHooks() {
	module.SetEngineLogHook(console.PushEngineLog)

	console.SetFrontendHooks(
		// 月华前端探针：消息终端声明的接入频道连接数。
		// 该终端以 bus=0 接入（不订阅引擎总线），故不能用总线订阅数判定
		// ——否则前端正常在线时会被误判为离线。
		func() bool {
			return websocket.Instance != nil && websocket.Instance.ChannelClientCount(lunarTerminalChannel) > 0
		},
		// 琉璃前端探针：引擎总线频道（DefaultBusChannel）的订阅数
		func() bool {
			return websocket.Instance != nil && websocket.Instance.ChannelBusCount(websocket.DefaultBusChannel) > 0
		},
		// 资源库探针：独立频道（?channel=library）的连接数
		func() bool {
			return websocket.Instance != nil && websocket.Instance.ChannelClientCount(libraryTerminalChannel) > 0
		},
		// 月华窗口唤起器（重启语义：存活重载页面，残留死窗口清理重建）
		func() error {
			return browser.RestartDesktopWindow(lunarWindowSpec())
		},
		// 琉璃窗口唤起器
		func() error {
			return browser.RestartDesktopWindow(crystalWindowSpec())
		},
		// 资源库窗口唤起器（未打开时创建，已打开时重载）
		func() error {
			return browser.RestartDesktopWindow(libraryWindowSpec())
		},
	)
}

// WireTUIMaintenanceHook 把「打包期间暂停思维链」注入打包归档 / 更新包发布：
//
// 7z 压缩会长时间占用 CPU 与磁盘，若 Tick 同时推进生活链/思维链，会突然拉起
// primary_reasoning / light_reasoning / text_embedding。这里在压缩前后暂停 Tick 并卸载模型，
// 压缩完成后再恢复调度。
func WireTUIMaintenanceHook() {
	archive.SetMaintenanceHook(func(fn func() error) error {
		resume := thinking.BeginMaintenance()
		defer resume()
		return fn()
	})
}

// WireTUIDataDumpHooks 把「数据导出」能力注入控制台：控制台只负责触发与展示回执，
// 取数与落盘直接调用 server 的数据导出服务函数（server.DataDump），不再经 HTTP 自调。
//
// 为什么复用服务函数而不是在 app 层重写一遍：导出的**日志/实况/上下文结构**必须
// 与对外端点保持同一份定义，否则"控制台导出的文件"和"调用端点拿到的文件"会逐渐分叉。
func WireTUIDataDumpHooks() {
	console.SetDataDumpHooks(
		func() (console.DataDumpResult, error) {
			return callDataDump(server.DataDumpRequest{Kind: server.DataDumpLogs})
		},
		// 核心智能实况（模型调用轮次）——与另外两项并列，因为其数据来源互不重叠
		func() (console.DataDumpResult, error) {
			return callDataDump(server.DataDumpRequest{Kind: server.DataDumpTranscript})
		},
		func(target string) (console.DataDumpResult, error) {
			return callDataDump(server.DataDumpRequest{Kind: server.DataDumpContext, Agent: target})
		},
	)
}

// WireTUIEngineHooks 把控制台需要的观测与人工入口注入宿主：
//   - 工具列表：汇总全部 Go 智能体的工具目录（控制台只负责展示）；
//   - 核心智能实况与模型计量：各 Go 智能体的模型调用统一进实况与计量；
//   - 桌面操纵：唯一人工入口在「运行状态」屏（输入要求启动 / 随时中断）。
//
// 思维链状态重建（自动/手动）已整体移除：实测无法真正恢复正常运行，
// 控制台不再提供该入口。
func WireTUIEngineHooks() {
	// 「工具列表」屏：宿主汇总全部 Go 智能体的工具目录（控制台只负责展示）。
	console.SetToolCatalogHook(buildToolCatalog)

	// 哔哩观影的模型调用也进「推理实况」，用户不用只盯着日志干等。
	// 计量回调让这些 Go 侧调用一并计入「运行状态」的模型计量。
	meter := func(prompt, completion, cached int64, generation time.Duration, estimated bool) {
		console.RecordModelCall(prompt, completion, cached, generation, estimated)
	}
	// 网络搜索、视觉理解、文献解析、通讯桥接、互助论坛、桌面操纵与哔哩观影
	// 的模型调用统一经 protocol/agentbase 进入实况与计量。
	streamObserver := func(role string) { console.StreamBegin(role) }
	streamDelta := func(role, kind, text string) {
		streamKind := console.KindContent
		if kind == "think" {
			streamKind = console.KindThink
		}
		console.StreamDelta(role, streamKind, text)
	}
	streamEnd := func(role string) { console.StreamEnd(role) }
	// 工具调用由基座通过 Observer.Tool 上报，装配层桥接到控制台实况；
	// protocol/agentbase 不再直接依赖 engine/console。
	streamTool := func(role, name, args string) { console.StreamTool(role, name, args) }
	// 所有 Go 智能体统一经 protocol/agentbase 登记推理实况与模型计量。
	agentbase.SetObserver(agentbase.Observer{
		Begin: streamObserver,
		Delta: streamDelta,
		End:   streamEnd,
		Tool:  streamTool,
		Meter: meter,
	})
	// 重试仍失败时交给智能修复：优先走结构化简报，缺失字段由修复层降级。
	agentbase.SetRepairDetailHook(func(detail agentbase.FailureDetail) {
		detail.Agent = agentIDByRole(detail.Agent)
		Savior.ReportDetail(detail)
	})
	// 桌面操纵的工具调用也进实况（工具名 + 参数），复盘时看得出它到底有没有真正去启动应用
	desktop.SetToolObserver(func(role, name, args string) {
		console.StreamTool(role, name, args)
	})

	// 桌面操纵智能体：唯一入口在控制台「运行状态」屏（人工输入要求启动、随时中断）。
	// 同时把中断器注入对话者：对话者开始推理与应答前会自动终止桌面操纵，
	// 避免它继续抢占桌面键鼠、与本轮响应互相干扰。
	console.SetDesktopOperatorHooks(
		desktopchain.Start,
		desktopchain.Stop,
		func() console.DesktopOperatorStatus {
			state := desktopchain.Snapshot()
			return console.DesktopOperatorStatus{
				Running:     state.Running,
				Stopping:    state.Stopping,
				Instruction: state.Instruction,
				StartedAt:   state.StartedAt,
				FinishedAt:  state.FinishedAt,
				Result:      state.Result,
				Error:       state.Error,
			}
		},
	)
	thinking.SetDesktopActorStopHook(desktopchain.Stop)
}

// roleAgentIDs 实况角色标签 → 智能修复使用的智能体 ID（显式映射，禁止前缀猜测）。
var roleAgentIDs = map[string]string{
	agentinfo.NameVision:             "vision",
	agentinfo.NameDocumentReader:     "document_reader",
	"归档解析":                           "archive_reader",
	agentinfo.NameManipulateFiles:    "manipulate_files",
	agentinfo.NameManipulateArchive:  "manipulate_archive",
	agentinfo.NameCodeExecution:      "code_execution",
	agentinfo.NameContentRollback:    "content_rollback",
	agentinfo.NameFileOperations:     "file_operations",
	agentinfo.NameLifeMemorizer:      "life_memorizer",
	"生活记忆整理者":                        "life_memorizer",
	agentinfo.NamePainter:            "painter",
	"绘制者":                            "painter",
	agentinfo.NameScreenshoter:       "screenshoter",
	"截图者":                            "screenshoter",
	agentinfo.NameDesktopOperator:    "desktop_operator",
	"桌面操作-模型":                        "desktop_operator",
	"桌面操纵":                           "desktop_operator",
	agentinfo.NameMusician:           "musician",
	"演奏者":                            "musician",
	agentinfo.NameDialogueBridging:   "dialogue_bridging",
	agentinfo.NameMutualForum:        "mutual_forum",
	"互助论坛-模型":                        "mutual_forum",
	agentinfo.NameSearchPlanner:      "search_planner",
	"搜索者-规划":                         "search_planner",
	agentinfo.NameSearchSummarizer:   "search_summarizer",
	"搜索者-摘要":                         "search_summarizer",
	agentinfo.NameSQLWriter:          "sql_writer",
	agentinfo.NameViewerScreening:    "viewer_screening",
	"观影者-阅片":                         "viewer_screening",
	agentinfo.NameViewerCensor:       "viewer_censor",
	"观影者-审查":                         "viewer_censor",
	agentinfo.NameViewerCritic:       "viewer_critic",
	"观影者-评论":                         "viewer_critic",
	agentinfo.NameViewerShare:        "viewer_share",
	"观影者-分享":                         "viewer_share",
	agentinfo.NameViewerRemark:       "viewer_remark",
	"观影者-观感":                         "viewer_remark",
	agentinfo.NameDialogue:           "dialogue",
	"对话者":                            "dialogue",
	agentinfo.NameMemoryTagger:       "memory_tagger",
	agentinfo.NameMemoryFuser:        "memory_fuser",
	agentinfo.NameAudioTranscriber:   "audio_transcriber",
	agentinfo.NameResponder:          "responder",
	agentinfo.NameSavior:             "savior",
	"智能修复":                           "savior",
	"哔哩观影":                           "bilibili",
	"网络搜索":                           "web_search",
	agentinfo.NameStoryboardResearch: "storyboard_research",
	agentinfo.NameStoryboardPlan:     "storyboard_planner",
	agentinfo.NameWishManager:        "wish_manager",
}

// agentIDByRole 把故障归属映射到智能修复使用的智能体 ID。
// 新基座直接上报标准 ID；旧调用点仍按实况角色标签上报，这里做显式兼容映射。
func agentIDByRole(role string) string {
	role = strings.TrimSpace(role)
	if id, ok := roleAgentIDs[role]; ok {
		return id
	}
	if base, _, ok := strings.Cut(role, "·"); ok {
		if id, ok := roleAgentIDs[base]; ok {
			return id
		}
	}
	switch role {
	case "vision", "document_reader", "archive_reader",
		"creative", "life_memorizer", "painter", "screenshoter",
		"manipulate_files", "manipulate_archive", "code_execution", "content_rollback",
		"file_operations",
		"desktop", "desktop_operator", "bilibili",
		"web_search", "search_planner", "search_summarizer", "sql_writer", "mutual_forum", "dialogue_bridging",
		"storyboard_research", "storyboard_planner",
		"viewer_screening", "viewer_censor", "viewer_critic", "viewer_share", "viewer_remark",
		"responder", "savior", "dialogue", "unknown",
		"memory_tagger", "memory_fuser", "audio_transcriber", "wish_manager":
		return role
	}
	return "dialogue"
}

// buildToolCatalog 汇总全部 Go 智能体的工具目录（供控制台「工具列表」屏扫描展示）。
func buildToolCatalog() []agentinfo.Agent {
	out := thinking.ToolCatalog()
	// 思维链内的扁平智能体按既有展示顺序紧随对话者。
	out = append(out, Painter.ToolCatalog()...)
	out = append(out, Musician.ToolCatalog()...)
	out = append(out, Screenshoter.ToolCatalog()...)
	out = append(out, Vision.ToolCatalog()...)
	out = append(out, DocumentReader.ToolCatalog()...)
	out = append(out, ArchiveReader.ToolCatalog()...)
	out = append(out, WebCrawler.ToolCatalog()...)
	out = append(out, ManipulateFiles.ToolCatalog()...)
	out = append(out, ManipulateArchive.ToolCatalog()...)
	out = append(out, CodeExecution.ToolCatalog()...)
	out = append(out, ContentRollback.ToolCatalog()...)
	out = append(out, FileOperations.ToolCatalog()...)
	out = append(out, WishManager.ToolCatalog()...)
	out = append(out, DialogueBridging.ToolCatalog()...)
	out = append(out, search.ToolCatalog()...)
	out = append(out, SearchPlanner.ToolCatalog()...)
	out = append(out, SearchSummarizer.ToolCatalog()...)
	out = append(out, ViewerScreening.ToolCatalog()...)
	out = append(out, ViewerCensor.ToolCatalog()...)
	out = append(out, ViewerCritic.ToolCatalog()...)
	out = append(out, ViewerShare.ToolCatalog()...)
	out = append(out, viewer.ToolCatalog()...)
	out = append(out, desktop.ToolCatalog()...)
	out = append(out, MutualForum.ToolCatalog()...)
	out = append(out, StoryboardResearch.ToolCatalog()...)
	out = append(out, StoryboardPlanner.ToolCatalog()...)
	return out
}

// callDataDump 直接调用导出服务并解析回执。
func callDataDump(req server.DataDumpRequest) (console.DataDumpResult, error) {
	resp, err := server.DataDump(context.Background(), req)
	if err != nil {
		return console.DataDumpResult{}, err
	}
	if !resp.OK {
		reason := resp.Error
		if reason == "" {
			reason = "导出失败"
		}
		return console.DataDumpResult{}, errors.New(reason)
	}
	return console.DataDumpResult{Path: resp.Path, Bytes: resp.Bytes, Records: resp.Records}, nil
}
