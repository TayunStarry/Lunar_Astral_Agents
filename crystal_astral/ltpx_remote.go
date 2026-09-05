package main

import (
	"CrystalAstral/agent/AutoLTP"
	"CrystalAstral/agent/YaraLTP"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// ==== LTPX 远程（月华调用）协议实现 ====
// 新版 LTPX 协议：琉璃作为「通用中转层」被月华调度，工具本身由前端包提供（AtoA）。
//   1. 琉璃启动时一次性向月华提交联络 URL；月华在每条思考链起点心跳并拉取工具链
//   2. 工具链 = 动态扫描 local_data/package/*/metadata.json 中带 tools 定义的包（包自带 AtoA）
//   3. 月华调用工具 → 琉璃按工具名路由到对应包 → 通过 /ws 广播给前端 → 前端打开包页面并
//      postMessage 投递给包 → 包执行（含页面展示）→ 回执 /ltpx/result → 琉璃返回月华
//   4. 琉璃核心不随包增删而改动：加载/卸载工具只影响扫描结果

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
				AppID:       meta.ID,
			})
			pkgMap[t.Name] = meta.ID
		}
	}
	return defs, pkgMap
}

// use_the_program 聚合工具常量名：月华只需记住这一个工具名，具体目标工具经参数 tool 指定，
// 从而避免在多工具链中精确背诵/生成工具名导致的「工具迷航」。
const useTheProgramToolName = "use_the_program"

// windowAgentToolName Auto-LTP 内置桌面智能体的工具名（多角色编排式桌面智能体，当前主用）
const windowAgentToolName = "window_agent"

// windowAgentToolDescription Auto-LTP 内置桌面智能体的默认固有描述
const windowAgentToolDescription = "面向 Windows 桌面的编排式桌面智能体：先拆解任务为结构化计划，视觉+UIA 双重理解界面，再由独立角色逐步执行操作"

// yaraLTPToolName Yara-LTP 路由兼容层的内置固有工具名
const yaraLTPToolName = "yara_ltp"

// yaraLTPToolDescription Yara-LTP（LTP3 引擎）内置固有工具描述
const yaraLTPToolDescription = "LTP3（YaraFlow）引擎入口：把自然语言消息作为一条聊天消息路由到默认钩子点 chat.receive.after_process，交给所有订阅该钩子点的已加载 LTP3 插件订阅器执行，并返回各插件的处理结果"

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
		Name:        useTheProgramToolName,
		Description: b.String(),
		AppID:       "",
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
	// 内置 Auto-LTP 桌面智能体作为固有工具选项（face_ltp 已弃用并移除）
	defs = append(defs, LTPXRemoteToolDef{Name: windowAgentToolName, Description: windowAgentToolDescription})
	// 内置 Yara-LTP 路由兼容层作为固有工具选项（goja 兼容层，消息路由到 yara 订阅器）
	defs = append(defs, LTPXRemoteToolDef{Name: yaraLTPToolName, Description: yaraLTPToolDescription})
	// 收敛为单一 use_the_program 聚合工具，避免月华在多工具名间「迷航」
	aggTool := buildUseTheProgram(defs)
	LoggerGeneral.Info("CrystalAstral", "月华拉取 LTPX 工具链 (GET /ltpx/tools)，聚合为 %s（内含 %d 个目标工具）", aggTool.Name, len(defs))
	jsonOK(w, http.StatusOK, map[string]any{
		"app_id": "crystal_astral",
		"tools":  []LTPXRemoteToolDef{aggTool},
	})
}

// ltpRemoteCallHandler 月华调用琉璃工具（POST /ltpx/call）
// use_the_program 聚合工具：从 arguments.tool 解析目标工具 → 路由到对应包 → 登记待定调用
// → 通过 /ws 广播给前端 → 等待包执行回执
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
	if req.Tool == useTheProgramToolName {
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

	// 内置 Auto-LTP 桌面智能体（window_agent）：进程内调用多角色编排智能体
	if targetTool == windowAgentToolName {
		instruction := ""
		if raw, exists := req.Arguments["instruction"]; exists {
			if s, sok := raw.(string); sok {
				instruction = s
			}
		}
		text, err := AutoLTP.Run(instruction)
		LoggerGeneral.Info("CrystalAstral", "月华调用内置 Auto-LTP 智能体（工具=%s）：%s", targetTool, instruction)
		if err != nil {
			jsonOK(w, http.StatusOK, LTPXRemoteCallResponse{Success: false, Text: text, Error: err.Error()})
			return
		}
		success := !strings.HasPrefix(text, "已达到最大执行轮次")
		if text == "" {
			text = "任务已执行完成"
		}
		jsonOK(w, http.StatusOK, LTPXRemoteCallResponse{Success: success, Text: text})
		return
	}

	// 内置 Yara-LTP 路由兼容层（yara_ltp）：进程内路由自然语言消息到 yara 事件/钩子订阅器
	if targetTool == yaraLTPToolName {
		instruction := ""
		if raw, exists := req.Arguments["instruction"]; exists {
			if s, sok := raw.(string); sok {
				instruction = s
			}
		}
		text, err := YaraLTP.Run(instruction)
		LoggerGeneral.Info("CrystalAstral", "月华调用内置 Yara-LTP 路由兼容层（工具=%s）：%s", targetTool, instruction)
		if err != nil {
			jsonOK(w, http.StatusOK, LTPXRemoteCallResponse{Success: false, Text: text, Error: err.Error()})
			return
		}
		if text == "" {
			text = "路由完成"
		}
		jsonOK(w, http.StatusOK, LTPXRemoteCallResponse{Success: true, Text: text})
		return
	}

	// 路由：目标工具名 → 提供该工具的包 ID
	_, toolPkgMap := scanAtoaToolchain()
	appID := toolPkgMap[targetTool]
	if appID == "" {
		jsonOK(w, http.StatusOK, LTPXRemoteCallResponse{Success: false, Error: "未找到提供工具 " + targetTool + " 的包（该包可能已卸载）"})
		return
	}

	// 登记待定调用
	requestID := fmt.Sprintf("%d-%04d", time.Now().UnixNano(), rand.Intn(10000))
	done := make(chan LTPXRemoteCallResponse, 1)
	ltpPendingMutex.Lock()
	ltpPendingCalls[requestID] = done
	ltpPendingMutex.Unlock()

	// 广播给前端：前端负责打开对应包页面并投递执行（tool 传实际目标工具名便于前端复用同包页面）
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

	// 等待前端包回执
	select {
	case resp := <-done:
		LoggerGeneral.Info("CrystalAstral", "LTPX 工具 %s (request_id=%s) 执行完成: success=%v\ntext: %s\nerror: %s", targetTool, requestID, resp.Success, resp.Text, resp.Error)
		jsonOK(w, http.StatusOK, resp)
	case <-time.After(ltpCallTimeout):
		ltpPendingMutex.Lock()
		delete(ltpPendingCalls, requestID)
		ltpPendingMutex.Unlock()
		LoggerGeneral.Warn("CrystalAstral", "LTPX 工具 %s (request_id=%s) 等待回执超时（120s）", targetTool, requestID)
		jsonOK(w, http.StatusOK, LTPXRemoteCallResponse{Success: false, Error: "等待琉璃前端包执行工具 " + targetTool + " 超时（琉璃页面可能未打开）"})
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
	done, ok := ltpPendingCalls[req.RequestID]
	if ok {
		delete(ltpPendingCalls, req.RequestID)
	}
	ltpPendingMutex.Unlock()

	if ok {
		done <- LTPXRemoteCallResponse{Success: req.Success, Text: req.Text, Error: req.Error}
	}

	LoggerGeneral.Info("CrystalAstral", "收到 AtoA 包回执 (request_id=%s, keep_open=%v): success=%v\ntext: %s\nerror: %s", req.RequestID, req.KeepOpen, req.Success, req.Text, req.Error)
	jsonOK(w, http.StatusOK, map[string]any{"ok": true})
}

// playStartupVoice 由后端直接播放启动语音 WAV（winmm PlaySoundW，SND_ASYNC 异步播放）
// 前端 WebView 受浏览器自动播放策略限制（必须用户交互后才允许发声），
// 因此启动语音改由后端原生进程播放，绕开自动播放限制，无需用户点击。
// 音频文件位于 {LocalDir}/audios/ 下，voice 为文件名（如 enable_tool_package.wav）。
func playStartupVoice(fileName string) {
	if fileName == "" {
		return
	}
	execPath, err := os.Executable()
	if err != nil {
		LoggerGeneral.Warn("CrystalAstral", "定位可执行目录失败，无法播放启动语音 %s: %v", fileName, err)
		return
	}
	execDir := filepath.Dir(execPath)
	audioPath := filepath.Join(execDir, *GeneralConfig.LocalDir, "audios", fileName)
	pathPtr, err := syscall.UTF16PtrFromString(audioPath)
	if err != nil {
		LoggerGeneral.Warn("CrystalAstral", "启动语音路径转换失败 %s: %v", audioPath, err)
		return
	}
	// SND_FILENAME=0x00020000 SND_ASYNC=0x0001 SND_NODEFAULT=0x0002
	flags := uintptr(0x00020000 | 0x0001 | 0x0002)
	ret, _, _ := procPlaySoundW.Call(uintptr(unsafe.Pointer(pathPtr)), 0, flags)
	if ret == 0 {
		LoggerGeneral.Warn("CrystalAstral", "启动语音播放失败（PlaySoundW 返回 0）: %s", audioPath)
		return
	}
	LoggerGeneral.Info("CrystalAstral", "已由后端播放启动语音: %s", audioPath)
}

// setStartupVoice 记录启动语音决策（含月华在线状态），并立即由后端播放对应语音
// voice 取值：sent（工具包已推送，月华在线）/ failed（无法交给月华，月华离线）/ disable（工具包停用，琉璃关闭）
func setStartupVoice(voice string, lunarOnline bool) {
	startupVoiceMutex.Lock()
	lastStartupVoice = StartupVoice{Voice: voice, Lunar: lunarOnline, Seq: time.Now().UnixNano()}
	startupVoiceMutex.Unlock()
	// 直接由后端播放，前端不再参与（避免 WebView 自动播放限制）
	switch voice {
	case "sent":
		playStartupVoice("enable_tool_package.wav")
	case "failed":
		playStartupVoice("tool_package_failed.wav")
	case "disable":
		playStartupVoice("disable_tool_package.wav")
	}
}

// registerToLunar 琉璃启动时一次性向月华提交联络 URL（POST /ltpx/register）
// 月华固定端口（BasicPort），琉璃随机端口；多开时月华以最新注册为准。
// 注册结果同时决定启动语音：月华在线且 URL 推送成功 → 工具包已发送；离线/推送失败 → 无法交给月华。
func registerToLunar(port int) (bool, error) {
	if StudioHubInstance == nil {
		return false, nil
	}
	selfURL := "http://127.0.0.1:" + strconv.Itoa(port)
	lunarURL := "http://127.0.0.1:" + strconv.Itoa(*GeneralConfig.BasicPort) + "/ltpx/register"

	payload, _ := json.Marshal(map[string]string{"url": selfURL})
	respBody, status, err := ltpHTTPPost(lunarURL, payload)
	if err != nil || status != http.StatusOK {
		LoggerGeneral.Warn("CrystalAstral", "向月华注册联络 URL 失败(或月华离线): %v (status=%d)", err, status)
		// 月华离线：后端直接播放「无法交给月华」语音
		setStartupVoice("failed", false)
		return false, err
	}

	// 校验月华响应：仅当 HTTP 200 且 success=true 才视为「URL 推送成功」（月华在线）
	var regResp LunarRegisterResponse
	if err := json.Unmarshal(respBody, &regResp); err != nil || !regResp.Success {
		LoggerGeneral.Warn("CrystalAstral", "月华未确认 LTPX 注册 (success=%v, err=%v)，按推送失败处理", regResp.Success, err)
		setStartupVoice("failed", false)
		return false, fmt.Errorf("月华未确认 LTPX 注册")
	}

	LoggerGeneral.Info("CrystalAstral", "LTPX 已向月华注册联络 URL: %s (月华在线)", selfURL)
	// 月华在线：后端直接播放「工具包已发送」语音
	setStartupVoice("sent", true)
	return true, nil
}

// notifyToolPackageDisabled 优雅关闭时由后端播放「工具包停用」语音（disable_tool_package.wav）
// 仅当本次启动月华在线（启动语音为 sent）时播放；离线时无需停用提示。
// 播放采用异步（SND_ASYNC），随后等待 3 秒再关闭服务器，确保停用语音完整播放完毕
// （前端浏览器窗口关闭或收到中断信号后均适用）。
func notifyToolPackageDisabled() {
	startupVoiceMutex.RLock()
	enabled := lastStartupVoice.Voice == "sent"
	startupVoiceMutex.RUnlock()
	if !enabled {
		return
	}
	LoggerGeneral.Info("CrystalAstral", "月华在线期间琉璃关闭，后端播放工具包停用语音")
	setStartupVoice("disable", true)
	// 等待 3 秒再关闭服务器，确保停用语音播放完毕
	time.Sleep(3 * time.Second)
}

// ltpHTTPPost 小型 HTTP POST 辅助（JSON 请求体）
func ltpHTTPPost(url string, payload []byte) ([]byte, int, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}
