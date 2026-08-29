package main

import (
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
	"time"
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

// scanAtoaToolchain 扫描包目录，收集「提供 AtoA 能力」的包（metadata.json 含非空 tools 数组）的工具链
// 返回：工具定义列表（含 app_id=包 ID）、工具名→包 ID 映射（供调用路由）
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
				Type     string `json:"type"`
				Function struct {
					Name        string `json:"name"`
					Description string `json:"description"`
					Parameters  any    `json:"parameters"`
				} `json:"function"`
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
			if t.Type != "function" || t.Function.Name == "" {
				continue
			}
			defs = append(defs, LTPXRemoteToolDef{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				AppID:       meta.ID,
				Parameters:  t.Function.Parameters,
			})
			pkgMap[t.Function.Name] = meta.ID
		}
	}
	return defs, pkgMap
}

// ltpRemoteToolsHandler 月华拉取琉璃工具链（GET /ltpx/tools）
// 每次请求动态扫描包目录，保证包增删后工具链即时反映
func ltpRemoteToolsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tools, _ := scanAtoaToolchain()
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name)
	}
	LoggerGeneral.Info("CrystalAstral", "月华拉取 LTPX 工具链 (GET /ltpx/tools)，返回 %d 个工具: %v", len(tools), names)
	jsonOK(w, http.StatusOK, map[string]any{
		"app_id": "crystal_astral",
		"tools":  tools,
	})
}

// ltpRemoteCallHandler 月华调用琉璃工具（POST /ltpx/call）
// 按工具名路由到对应包 → 登记待定调用 → 通过 /ws 广播给前端 → 等待包执行回执
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

	// 路由：工具名 → 提供该工具的包 ID
	_, toolPkgMap := scanAtoaToolchain()
	appID := toolPkgMap[req.Tool]
	if appID == "" {
		jsonOK(w, http.StatusOK, LTPXRemoteCallResponse{Success: false, Error: "未找到提供工具 " + req.Tool + " 的包（该包可能已卸载）"})
		return
	}

	// 登记待定调用
	requestID := fmt.Sprintf("%d-%04d", time.Now().UnixNano(), rand.Intn(10000))
	done := make(chan LTPXRemoteCallResponse, 1)
	ltpPendingMutex.Lock()
	ltpPendingCalls[requestID] = done
	ltpPendingMutex.Unlock()

	// 广播给前端：前端负责打开对应包页面并投递执行
	msg, _ := json.Marshal(map[string]any{
		"type":       "ltpx_call",
		"request_id": requestID,
		"tool":       req.Tool,
		"app_id":     appID,
		"arguments":  req.Arguments,
	})
	argsJSON, _ := json.Marshal(req.Arguments)
	LoggerGeneral.Info("CrystalAstral", "月华调用 LTPX 工具 → %s (app=%s, request_id=%s)\n请求参数: %s", req.Tool, appID, requestID, string(argsJSON))
	if StudioHubInstance != nil {
		StudioHubInstance.Broadcast <- msg
	}

	// 等待前端包回执
	select {
	case resp := <-done:
		LoggerGeneral.Info("CrystalAstral", "LTPX 工具 %s (request_id=%s) 执行完成: success=%v\ntext: %s\nerror: %s", req.Tool, requestID, resp.Success, resp.Text, resp.Error)
		jsonOK(w, http.StatusOK, resp)
	case <-time.After(ltpCallTimeout):
		ltpPendingMutex.Lock()
		delete(ltpPendingCalls, requestID)
		ltpPendingMutex.Unlock()
		LoggerGeneral.Warn("CrystalAstral", "LTPX 工具 %s (request_id=%s) 等待回执超时（120s）", req.Tool, requestID)
		jsonOK(w, http.StatusOK, LTPXRemoteCallResponse{Success: false, Error: "等待琉璃前端包执行工具 " + req.Tool + " 超时（琉璃页面可能未打开）"})
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

// ltpStartupVoiceHandler 前端读取启动语音决策（GET /lunar/sync/startup-voice）
func ltpStartupVoiceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	startupVoiceMutex.RLock()
	voice := lastStartupVoice
	startupVoiceMutex.RUnlock()
	jsonOK(w, http.StatusOK, voice)
}

// setStartupVoice 记录启动语音决策（含月华在线状态），每次决策分配新的序号供前端去重
func setStartupVoice(lunarOnline bool) {
	startupVoiceMutex.Lock()
	switch {
	case lunarOnline:
		lastStartupVoice = StartupVoice{Voice: "sent", Lunar: true, Seq: time.Now().UnixNano()}
	default:
		lastStartupVoice = StartupVoice{Voice: "failed", Lunar: false, Seq: time.Now().UnixNano()}
	}
	startupVoiceMutex.Unlock()
}

// registerToLunar 琉璃启动时一次性向月华提交联络 URL（POST /ltpx/register）
// 月华固定端口（BasicPort），琉璃随机端口；多开时月华以最新注册为准。
// 注册结果同时决定启动语音：月华在线 → 工具包已发送；离线 → 无法交给月华。
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
		setStartupVoice(false)
		// 离线：广播一次启动语音决策，前端连接后可自动播放
		broadcastStartupVoice(false)
		return false, err
	}

	_ = respBody
	LoggerGeneral.Info("CrystalAstral", "LTPX 已向月华注册联络 URL: %s (月华在线)", selfURL)
	setStartupVoice(true)
	broadcastStartupVoice(true)
	return true, nil
}

// broadcastStartupVoice 将启动语音决策广播到前端 /ws，使其自动播放对应语音
func broadcastStartupVoice(lunarOnline bool) {
	startupVoiceMutex.RLock()
	sv := lastStartupVoice
	startupVoiceMutex.RUnlock()
	msg, _ := json.Marshal(map[string]any{
		"type":   "lunar_sync",
		"action": "startup_voice",
		"voice":  sv.Voice,
		"lunar":  sv.Lunar,
		"seq":    sv.Seq,
	})
	if StudioHubInstance != nil {
		StudioHubInstance.Broadcast <- msg
	}
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
