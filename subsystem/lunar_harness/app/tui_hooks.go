package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/ArchiveOps"
	"LunarSubsystem/LunarHarness/agents/ArchiveReader"
	"LunarSubsystem/LunarHarness/agents/BilibiliViewer"
	"LunarSubsystem/LunarHarness/agents/Creator"
	"LunarSubsystem/LunarHarness/agents/Dialogue"
	"LunarSubsystem/LunarHarness/agents/DialogueBridging"
	"LunarSubsystem/LunarHarness/agents/DocumentReader"
	"LunarSubsystem/LunarHarness/agents/FileEditor"
	"LunarSubsystem/LunarHarness/agents/MutualForum"
	"LunarSubsystem/LunarHarness/agents/PlanManager"
	"LunarSubsystem/LunarHarness/agents/Savior"
	"LunarSubsystem/LunarHarness/agents/Vision"
	"LunarSubsystem/LunarHarness/agents/WebSearch"
	"LunarSubsystem/LunarHarness/agents/desktop"
	"LunarSubsystem/LunarHarness/engine/browser"
	"LunarSubsystem/LunarHarness/engine/console"
	"LunarSubsystem/LunarHarness/engine/console/archive"
	module "LunarSubsystem/LunarHarness/engine/multimodal"
	"LunarSubsystem/LunarHarness/engine/websocket"
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
// 7z 压缩会长时间占用 CPU 与磁盘，若 Tick 同时推进生活链/对话链，会突然拉起
// primary_reasoning / light_reasoning / text_embedding。这里在压缩前后暂停 Tick 并卸载模型，
// 压缩完成后再恢复调度。
func WireTUIMaintenanceHook() {
	archive.SetMaintenanceHook(func(fn func() error) error {
		resume := Dialogue.BeginMaintenance()
		defer resume()
		return fn()
	})
}

// WireTUIDataDumpHooks 把「数据导出」能力注入控制台：控制台只负责触发与展示回执，
// 取数与落盘的逻辑复用同一批 HTTP 端点处理器（server/data_dump.go）。
//
// 为什么复用端点处理器而不是在 app 层重写一遍：导出的**日志/实况/上下文结构**必须
// 与对外端点保持同一份定义，否则"控制台导出的文件"和"调用端点拿到的文件"会逐渐分叉。
// 这里经 httptest 在进程内直调处理器（无 socket、无端口），只多一次 JSON 往返。
func WireTUIDataDumpHooks() {
	console.SetDataDumpHooks(
		func() (console.DataDumpResult, error) {
			return callDumpHandler(server.DataDumpLogsHandler, "/debug/dump/logs")
		},
		// 核心智能实况（模型调用轮次）——与另外两项并列，因为其数据来源互不重叠
		func() (console.DataDumpResult, error) {
			return callDumpHandler(server.DataDumpTranscriptHandler, "/debug/dump/transcript")
		},
		func(target string) (console.DataDumpResult, error) {
			return callDumpHandler(server.DataDumpContextHandler, "/debug/dump/context?agent="+target)
		},
	)
}

// WireTUIEngineHooks 把控制台需要的观测与人工入口注入宿主：
//   - 工具列表：汇总全部 Go 智能体的工具目录（控制台只负责展示）；
//   - 核心智能实况与模型计量：各 Go 智能体的模型调用统一进实况与计量；
//   - 桌面操纵：唯一人工入口在「运行状态」屏（输入要求启动 / 随时中断）。
//
// 对话链状态重建（自动/手动）已整体移除：实测无法真正恢复正常运行，
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
	// 重试仍失败时交给智能修复：智能体基座按故障归属 ID 上报。
	agentbase.SetRepairHook(func(agent string, err error) {
		Savior.ReportRun(agentIDByRole(agent), err)
	})
	// 桌面操纵的工具调用也进实况（工具名 + 参数），复盘时看得出它到底有没有真正去启动应用
	desktop.SetToolObserver(func(role, name, args string) {
		console.StreamTool(role, name, args)
	})

	// 桌面操纵智能体：唯一入口在控制台「运行状态」屏（人工输入要求启动、随时中断）。
	// 同时把中断器注入对话者：对话者开始推理与应答前会自动终止桌面操纵，
	// 避免它继续抢占桌面键鼠、与本轮响应互相干扰。
	console.SetDesktopOperatorHooks(
		desktop.Start,
		desktop.Stop,
		func() console.DesktopOperatorStatus {
			state := desktop.Snapshot()
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
	Dialogue.SetDesktopActorStopHook(desktop.Stop)
}

// agentIDByRole 把故障归属映射到智能修复使用的智能体 ID。
// 新基座直接上报标准 ID；旧调用点仍按实况角色标签上报，这里做兼容映射。
func agentIDByRole(role string) string {
	switch role {
	case "vision", "document_reader", "archive_ops", "archive_reader",
		"file_editor", "creative", "desktop", "bilibili",
		"web_search", "mutual_forum", "dialogue_bridging",
		"responder", "savior", "dialogue", "unknown":
		return role
	}
	switch {
	case strings.Contains(role, "视觉"):
		return "vision"
	case strings.Contains(role, "文献"):
		return "document_reader"
	case strings.Contains(role, "归档操作"):
		return "archive_ops"
	case strings.Contains(role, "归档"):
		return "archive_reader"
	}
	return "dialogue"
}

// buildToolCatalog 汇总全部 Go 智能体的工具目录（供控制台「工具列表」屏扫描展示）。
func buildToolCatalog() []agentinfo.Agent {
	out := Dialogue.ToolCatalog()
	out = append(out, Vision.ToolCatalog()...)
	out = append(out, DocumentReader.ToolCatalog()...)
	out = append(out, ArchiveReader.ToolCatalog()...)
	out = append(out, ArchiveOps.ToolCatalog()...)
	out = append(out, FileEditor.ToolCatalog()...)
	out = append(out, Creator.ToolCatalog()...)
	out = append(out, PlanManager.ToolCatalog()...)
	out = append(out, DialogueBridging.ToolCatalog()...)
	out = append(out, WebSearch.ToolCatalog()...)
	out = append(out, BilibiliViewer.ToolCatalog()...)
	out = append(out, desktop.ToolCatalog()...)
	out = append(out, MutualForum.ToolCatalog()...)
	return out
}

// callDumpHandler 进程内直调导出端点处理器并解析回执
func callDumpHandler(handler http.HandlerFunc, target string) (console.DataDumpResult, error) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, target, nil)
	handler(rec, req)

	var resp server.DataDumpResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		return console.DataDumpResult{}, fmt.Errorf("解析导出回执失败: %w", err)
	}
	if !resp.OK {
		reason := resp.Error
		if reason == "" {
			reason = "导出失败（HTTP " + strconv.Itoa(rec.Code) + "）"
		}
		return console.DataDumpResult{}, errors.New(reason)
	}
	return console.DataDumpResult{Path: resp.Path, Bytes: resp.Bytes, Records: resp.Records}, nil
}
