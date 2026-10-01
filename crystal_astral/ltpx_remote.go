package main

import (
	"CrystalAstral/agent/AutoLTP"
	"CrystalAstral/agent/WebLTP"
	"CrystalAstral/engine/9.1-Flash"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ==== LTPX 远程（月华调用）协议实现 ====
// 新版 LTPX 协议：琉璃作为「通用中转层」被月华调度，工具本身由前端包提供（AtoA）。
//   1. 琉璃不主动向月华推送；月华在每条思考链起点按固定端口（36792）心跳并拉取工具链
//   2. 工具链 = 动态扫描 local_data/package/*/metadata.json 中带 tools 定义的包（包自带 AtoA）
//   3. 月华调用工具 → 琉璃异步受理（立即返回 pending + request_id）→ 按工具名路由到对应包
//      → 通过 /ws 广播给前端 → 前端打开包页面并 postMessage 投递给包 → 包执行（含页面展示）
//      → 回执 /ltpx/result 写入结果通道；月华经 /ltpx/poll 心跳轮询取回结果
//      （内置 Auto-LTP / Web-LTP 智能体同样异步受理，在 goroutine 中执行后写入结果通道）
//   4. 月华在每个「xx事件发生前」触发点把原始负载 POST /ltpx/event 到琉璃，
//      经 LTP9 插件处理；无插件订阅/未改写/琉璃离线时，月华回退使用原始数据
//   5. 琉璃核心不随包增删而改动：加载/卸载工具只影响扫描结果

// jsonOK 写入统一 JSON 响应
func jsonOK(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// ltpRemotePingHandler 月华探测琉璃是否在线（GET /ltpx/ping）
func ltpRemotePingHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	LoggerGeneral.Info("CrystalAstral", "收到月华 LTPX 心跳探测 (GET /ltpx/ping)")
	jsonOK(w, http.StatusOK, map[string]any{"ok": true, "name": "crystal_astral"})
}

// scanAtoaToolchain 扫描包目录，收集「提供 AtoA 能力」的包（metadata.json 含非空 tools 数组）的工具链。
// 每个包 tools 使用简化格式：仅声明工具名与功能简述（{name, description}），
// 具体的参数 schema 由 use_the_program 聚合工具统一提供，避免各包重复携带相同的 instruction 参数定义。
// 返回：原始工具定义列表（含 app_id=包 ID）、工具名→包 ID 映射（供调用路由）
func scanAtoaToolchain() ([]LTPXRemoteToolDef, map[string]string) {
	defs := []LTPXRemoteToolDef{}
	pkgMap := map[string]string{}

	execPath, err := os.Executable()
	if err != nil {
		return defs, pkgMap
	}
	execDir := filepath.Dir(execPath)
	packageDir := filepath.Join(execDir, *GeneralConfig.LocalDir, "package")

	entries, err := os.ReadDir(packageDir)
	if err != nil {
		return defs, pkgMap
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		metaPath := filepath.Join(packageDir, entry.Name(), "metadata.json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}

		var meta struct {
			ID    string `json:"id"`
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"tools"`
		}
		if err := json.Unmarshal(data, &meta); err != nil {
			LoggerGeneral.Warn("CrystalAstral", "LTPX 解析包元数据失败 %s: %v", metaPath, err)
			continue
		}
		// 包未提供 AtoA 工具（无 tools 数组）则跳过
		if meta.ID == "" || len(meta.Tools) == 0 {
			continue
		}

		for _, t := range meta.Tools {
			if t.Name == "" {
				continue
			}
			defs = append(defs, LTPXRemoteToolDef{
				Name:        t.Name,
				Description: t.Description,
			})
			pkgMap[t.Name] = meta.ID
		}
	}
	return defs, pkgMap
}

// Node-LTP 聚合体的工具名
const integratedToolsName = "use_program"

// Auto-LTP 智能体的工具名
const operateToolName = "window_operate"

// Auto-LTP 智能体的工具描述
const operateToolDescription = "操作 Windows 系统来执行任务"

// Web-LTP 智能体的工具名
const searchToolName = "web_search"

// Web-LTP 智能体的工具描述
const searchToolDescription = "调用(网页浏览器)去获取所需情报"

// buildUseTheProgram 将扫描到的全部 AtoA 工具收敛为单一 use_the_program 聚合工具。
// 其 description 内嵌「工具名：功能简述」清单，供模型在调用时从清单中挑取正确的 tool 参数；
// 参数 schema 统一为 {tool, instruction} 两个必填项。
func buildUseTheProgram(defs []LTPXRemoteToolDef) LTPXRemoteToolDef {
	var b strings.Builder
	b.WriteString("使用本工具可以调用以下应用程序（tool 参数指定要使用的目标工具，instruction 传入对该工具的自然语言指令，工具会自动识别意图并执行）：")
	for _, d := range defs {
		b.WriteString("\n- ")
		b.WriteString(d.Name)
		if d.Description != "" {
			b.WriteString("：")
			b.WriteString(ltpxFirstSentence(d.Description))
		}
	}
	// 已有工具清单时不追加说明；无可用工具则明确提示
	count := len(defs)
	if count > 0 {
		b.WriteString("\n\n请根据用户需求从上述清单中选择合适的 tool，并填入对应的自然语言 instruction。")
	}
	return LTPXRemoteToolDef{
		Name:        integratedToolsName,
		Description: b.String(),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tool": map[string]any{
					"type":        "string",
					"description": "要使用的目标工具名（从描述清单中选择，如 weather_news_query 或 file_manager）",
				},
				"instruction": map[string]any{
					"type":        "string",
					"description": "要对目标工具发出的自然语言指令，例如：查询北京今天的天气；把 README.md 移动到 docs 目录",
				},
			},
			"required": []string{"tool", "instruction"},
		},
	}
}

// ltpxFirstSentence 截取工具描述的首句（按中英文句号/分号切分），避免聚合清单过长占用上下文
func ltpxFirstSentence(desc string) string {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return ""
	}
	// 跳过开头的定式引导语（如「驱动/接受」等），直接取首句
	var end = len(desc)
	for i, r := range desc {
		if r == '。' || r == '；' || r == ';' || r == '.' || r == '！' || r == '？' {
			end = i + 1
			break
		}
	}
	s := strings.TrimSpace(desc[:end])
	if len([]rune(s)) > 60 {
		runes := []rune(s)
		s = string(runes[:60]) + "…"
	}
	return s
}

// ltpRemoteToolsHandler 月华拉取琉璃工具链（GET /ltpx/tools）
// 每次请求动态扫描包目录，保证包增删后工具链即时反映
func ltpRemoteToolsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defs, _ := scanAtoaToolchain()
	// 声明 Auto-LTP <window 操作>智能体
	defs = append(defs, LTPXRemoteToolDef{Name: operateToolName, Description: operateToolDescription})
	// 声明 Web-LTP <网络搜索>智能体
	defs = append(defs, LTPXRemoteToolDef{Name: searchToolName, Description: searchToolDescription})
	// 收敛为单一 use_the_program 聚合工具，避免月华在多工具名间「迷航」
	aggTool := buildUseTheProgram(defs)
	LoggerGeneral.Info("CrystalAstral", "月华拉取 LTPX 工具链 (GET /ltpx/tools)，聚合为 %s（内含 %d 个目标工具）", aggTool.Name, len(defs))
	jsonOK(w, http.StatusOK, map[string]any{
		"tool": aggTool,
	})
}

// ltpRemoteCallHandler 月华调用琉璃工具（POST /ltpx/call）
// 异步受理协议：登记待定调用后立即返回 pending=true + request_id，不做同步等待。
//   - 内置智能体（Auto-LTP / Web-LTP）：在独立 goroutine 中执行，完成后写入结果通道
//   - 前端包工具：登记后经 /ws 广播给前端，包执行完毕回执 /ltpx/result 写入结果通道
// 月华以 /ltpx/poll 心跳轮询取回结果，长任务（如联网搜索）不会再因同步等待超时被误判下线
func ltpRemoteCallHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LTPXRemoteCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonOK(w, http.StatusBadRequest, LTPXRemoteCallResponse{Success: false, Error: "无效的请求体: " + err.Error()})
		return
	}
	if req.Tool == "" {
		jsonOK(w, http.StatusBadRequest, LTPXRemoteCallResponse{Success: false, Error: "tool 为必填项"})
		return
	}

	// 解析目标工具名：月华统一调用 use_the_program，实际目标工具在 arguments.tool 中
	targetTool := req.Tool
	if req.Tool == integratedToolsName {
		if raw, ok := req.Arguments["tool"]; ok {
			if s, sok := raw.(string); sok && s != "" {
				targetTool = s
			}
		}
	}
	if targetTool == "" {
		jsonOK(w, http.StatusBadRequest, LTPXRemoteCallResponse{Success: false, Error: "use_the_program 需在 arguments 提供 tool 目标工具名"})
		return
	}

	// 登记待定调用（内置智能体与前端包工具共用同一张注册表与轮询端点）
	requestID := fmt.Sprintf("%d-%04d", time.Now().UnixNano(), rand.Intn(10000))
	done := make(chan LTPXRemoteCallResponse, 1)
	ltpPendingMutex.Lock()
	ltpPendingCalls[requestID] = &ltpPendingCall{done: done, toolName: targetTool, startedAt: time.Now()}
	ltpPendingMutex.Unlock()

	// 声明 Auto-LTP <window 操作>智能体：进程内调用多角色编排智能体（goroutine 异步执行）
	if targetTool == operateToolName {
		instruction := ""
		if raw, exists := req.Arguments["instruction"]; exists {
			if s, sok := raw.(string); sok {
				instruction = s
			}
		}
		LoggerGeneral.Info("CrystalAstral", "月华调用内置 Auto-LTP 智能体（工具=%s, request_id=%s）：%s", targetTool, requestID, instruction)
		go func() {
			text, err := AutoLTP.Run(instruction)
			resp := LTPXRemoteCallResponse{Text: text}
			if err != nil {
				resp.Success, resp.Error = false, err.Error()
			} else {
				resp.Success = !strings.HasPrefix(text, "已达到最大执行轮次")
				if text == "" {
					resp.Text = "任务已执行完成"
				}
			}
			LoggerGeneral.Info("CrystalAstral", "Auto-LTP 智能体 (request_id=%s) 执行完成: success=%v\ntext: %s\nerror: %s", requestID, resp.Success, resp.Text, resp.Error)
			writePendingResult(requestID, resp)
		}()
		jsonOK(w, http.StatusOK, LTPXRemoteCallResponse{Pending: true, RequestID: requestID})
		return
	}

	// 声明 Web-LTP <网络搜索>智能体：进程内独立执行搜索流水线（goroutine 异步执行）
	if targetTool == searchToolName {
		instruction := ""
		if raw, exists := req.Arguments["instruction"]; exists {
			if s, sok := raw.(string); sok {
				instruction = s
			}
		}
		LoggerGeneral.Info("CrystalAstral", "月华调用内置 Web-LTP 网络搜索（工具=%s, request_id=%s）：%s", targetTool, requestID, instruction)
		go func() {
			text, err := WebLTP.Run(instruction)
			resp := LTPXRemoteCallResponse{Text: text}
			if err != nil {
				resp.Success, resp.Error = false, err.Error()
			} else {
				resp.Success = true
				if resp.Text == "" {
					resp.Text = "搜索已完成"
				}
			}
			LoggerGeneral.Info("CrystalAstral", "Web-LTP 网络搜索 (request_id=%s) 执行完成: success=%v\ntext: %s\nerror: %s", requestID, resp.Success, resp.Text, resp.Error)
			writePendingResult(requestID, resp)
		}()
		jsonOK(w, http.StatusOK, LTPXRemoteCallResponse{Pending: true, RequestID: requestID})
		return
	}

	// 路由：目标工具名 → 提供该工具的包 ID
	_, toolPkgMap := scanAtoaToolchain()
	appID := toolPkgMap[targetTool]
	if appID == "" {
		ltpPendingMutex.Lock()
		delete(ltpPendingCalls, requestID)
		ltpPendingMutex.Unlock()
		jsonOK(w, http.StatusOK, LTPXRemoteCallResponse{Success: false, Error: "未找到提供工具 " + targetTool + " 的包（该包可能已卸载）"})
		return
	}

	// 前端包工具：广播给前端，前端负责打开对应包页面并投递执行（tool 传实际目标工具名便于前端复用同包页面）
	msg, _ := json.Marshal(map[string]any{
		"type":       "ltpx_call",
		"request_id": requestID,
		"tool":       targetTool,
		"app_id":     appID,
		"arguments":  req.Arguments,
	})
	argsJSON, _ := json.Marshal(req.Arguments)
	LoggerGeneral.Info("CrystalAstral", "月华调用 LTPX 工具 → %s (app=%s, request_id=%s)\n请求参数: %s", targetTool, appID, requestID, string(argsJSON))
	if StudioHubInstance != nil {
		StudioHubInstance.Broadcast <- msg
	}
	// 异步受理：立即返回 request_id，结果由月华经 /ltpx/poll 心跳轮询取回
	jsonOK(w, http.StatusOK, LTPXRemoteCallResponse{Pending: true, RequestID: requestID})
}

// ltpRemotePollHandler 月华心跳轮询工具执行状态（GET /ltpx/poll?request_id=xxx）。
// 心跳语义：每次成功轮询响应即证明琉璃在线，月华据此重置无响应计数；
// 任务仍在执行时返回 done=false，完成时返回 done=true 并携带结果。
// 未知 request_id（已取走/琉璃重启丢失状态）返回 done=true + 失败，避免月华无限轮询。
func ltpRemotePollHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	requestID := r.URL.Query().Get("request_id")
	if requestID == "" {
		jsonOK(w, http.StatusBadRequest, map[string]any{"done": true, "success": false, "error": "request_id 为必填项"})
		return
	}

	ltpPendingMutex.Lock()
	call, ok := ltpPendingCalls[requestID]
	if !ok {
		ltpPendingMutex.Unlock()
		jsonOK(w, http.StatusOK, map[string]any{"done": true, "success": false, "error": "任务不存在或已结束（琉璃可能已重启）"})
		return
	}
	// 超过最长保留时长：判定失败并清理，防止注册表泄漏
	if time.Since(call.startedAt) > ltpPendingMaxAge {
		delete(ltpPendingCalls, requestID)
		ltpPendingMutex.Unlock()
		LoggerGeneral.Warn("CrystalAstral", "LTPX 工具 %s (request_id=%s) 超过最长保留时长（%v），判定失败", call.toolName, requestID, ltpPendingMaxAge)
		jsonOK(w, http.StatusOK, map[string]any{"done": true, "success": false, "error": fmt.Sprintf("等待工具 %s 执行回执超时（%v）", call.toolName, ltpPendingMaxAge)})
		return
	}
	// 非阻塞读取结果：仍在执行时返回 done=false（本次轮询即一次心跳）
	select {
	case resp := <-call.done:
		delete(ltpPendingCalls, requestID)
		ltpPendingMutex.Unlock()
		LoggerGeneral.Info("CrystalAstral", "LTPX 工具 %s (request_id=%s) 经轮询取回结果: success=%v", call.toolName, requestID, resp.Success)
		jsonOK(w, http.StatusOK, map[string]any{"done": true, "success": resp.Success, "text": resp.Text, "error": resp.Error})
	default:
		ltpPendingMutex.Unlock()
		jsonOK(w, http.StatusOK, map[string]any{"done": false})
	}
}

// writePendingResult 将工具执行结果写入待定调用结果通道（容量 1，非阻塞写入，绝不卡死执行方）
func writePendingResult(requestID string, resp LTPXRemoteCallResponse) {
	ltpPendingMutex.Lock()
	call, ok := ltpPendingCalls[requestID]
	ltpPendingMutex.Unlock()
	if !ok {
		return
	}
	select {
	case call.done <- resp:
	default:
	}
}

// ltpRemoteResultHandler 前端包执行完毕后回执结果（POST /ltpx/result）
func ltpRemoteResultHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LTPXResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "无效的请求体", http.StatusBadRequest)
		return
	}
	if req.RequestID == "" {
		http.Error(w, "request_id 为必填项", http.StatusBadRequest)
		return
	}

	ltpPendingMutex.Lock()
	call, ok := ltpPendingCalls[req.RequestID]
	ltpPendingMutex.Unlock()

	if ok {
		select {
		case call.done <- LTPXRemoteCallResponse{Success: req.Success, Text: req.Text, Error: req.Error}:
		default:
		}
	}

	LoggerGeneral.Info("CrystalAstral", "收到 AtoA 包回执 (request_id=%s, keep_open=%v): success=%v\ntext: %s\nerror: %s", req.RequestID, req.KeepOpen, req.Success, req.Text, req.Error)
	jsonOK(w, http.StatusOK, map[string]any{"ok": true})
}

// ltpRemoteEventHandler 月华事件推送端点（POST /ltpx/event）。
// 月华在每个「xx事件发生前」触发点调用：把原始负载推送到琉璃，
// 经 LTP9 引擎派发到订阅该 topic 的插件；插件可经 return 回传业务结果，或经 modifiedData 改写负载。
// 返回：无插件订阅 / 未回传 → returned=false；有插件经 return 回传 → returned=true 且 return=业务结果。
// 月华据此决定采用插件回传的业务结果，或回退到本地默认处理。
func ltpRemoteEventHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LTPXRemoteEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonOK(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "无效的事件负载: " + err.Error()})
		return
	}
	topic := req.Topic
	if topic == "" {
		topic = req.Event
	}
	if topic == "" {
		jsonOK(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "topic/event 为必填项"})
		return
	}
	payload := req.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	result := StarLTP.Emit(topic, payload, fmt.Sprintf("ltp9-ev-%d", time.Now().UnixNano()))
	data := result.Data
	if data == nil {
		data = payload
	}
	LoggerGeneral.Info("CrystalAstral", "月华事件推送 %s (sub=%d, mod=%v, ret=%v)", topic, result.Summary.Subscribed, result.Modified, result.Returned)
	jsonOK(w, http.StatusOK, map[string]any{
		"ok":       true,
		"topic":    topic,
		"modified": result.Modified,
		"data":     data,
		"returned": result.Returned,
		"return":   result.Return,
		"summary":  result.Summary,
	})
}
