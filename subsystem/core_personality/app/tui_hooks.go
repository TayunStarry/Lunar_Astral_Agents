package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"

	"CorePersonality/engine/console"
	"CorePersonality/engine/lunar_loop"
	module "CorePersonality/engine/multimodal"
	"CorePersonality/server"
	"CorePersonality/transport/ws"
	"LunarSubsystem/BrowserClient"
)

// WireTUIFrontendHooks 将前端在线探针与窗口唤起器注入控制台的前端管理屏：
//   - 月华终端以 bus=0 接入并按 lunarTerminalChannel 声明频道 → 探针取该频道连接数；
//   - 琉璃桌面以 bus=1 接入缺省频道 → 探针取该频道总线订阅数；
//   - 唤起器复用装配层的窗口规格，重建桌面窗口（已存在时由 BrowserClient 聚焦）；
//   - 绘图引擎（sd.cpp）日志钩子接入「绘图引擎日志」屏（按行切分后统一排版）。
func WireTUIFrontendHooks() {
	module.SetEngineLogHook(console.PushEngineLog)

	console.SetFrontendHooks(
		// 月华前端探针：消息终端声明的接入频道连接数。
		// 该终端以 bus=0 接入（不订阅引擎总线），故不能用总线订阅数判定
		// ——否则前端正常在线时会被误判为离线。
		func() bool {
			return ws.Instance != nil && ws.Instance.ChannelClientCount(lunarTerminalChannel) > 0
		},
		// 琉璃前端探针：引擎总线频道（DefaultBusChannel）的订阅数
		func() bool {
			return ws.Instance != nil && ws.Instance.ChannelBusCount(ws.DefaultBusChannel) > 0
		},
		// 月华窗口唤起器（重启语义：存活重载页面，残留死窗口清理重建）
		func() error {
			return BrowserClient.RestartDesktopWindow(lunarWindowSpec())
		},
		// 琉璃窗口唤起器
		func() error {
			return BrowserClient.RestartDesktopWindow(crystalWindowSpec())
		},
	)
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

// WireTUIEngineHooks 把「月华引擎自愈」能力注入控制台：
//   - status：读取引擎状态（运行时/脚本就绪、重载次数、连续失败、自动重载策略）；
//   - reload：手动重载 goja 运行时（宿主侧还会顺手把崩掉的 llama-server 拉起来）。
//
// 为什么要给控制台一个手动入口：goja 运行时是一整块进程内状态，一旦走进死路，
// 除了重启整个程序没有别的办法；而重启会连带重启前端窗口与全部子系统。
// 控制台的重载按钮是"最小代价的恢复手段"，自动重载则是它的兜底自动化。
func WireTUIEngineHooks() {
	console.SetEngineHooks(
		func() console.EngineStatus {
			state := lunarloop.EngineState()
			return console.EngineStatus{
				RuntimeReady:        state.RuntimeReady,
				AgentReady:          state.AgentReady,
				BootedAt:            state.BootedAt,
				ReloadCount:         state.ReloadCount,
				LastReloadAt:        state.LastReloadAt,
				LastReloadReason:    state.LastReloadReason,
				LastReloadResult:    state.LastReloadResult,
				ConsecutiveFailures: state.ConsecutiveFailures,
				LastFailure:         state.LastFailure,
				LastFailureAt:       state.LastFailureAt,
				AutoReload:          state.AutoReload,
				AutoReloadThreshold: state.AutoReloadThreshold,
				Reloading:           lunarloop.ReloadRunning(),
			}
		},
		func(reason string) error {
			return lunarloop.ReloadMoonEngine(reason)
		},
	)
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
