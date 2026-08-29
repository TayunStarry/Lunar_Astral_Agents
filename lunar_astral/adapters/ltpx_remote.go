package adapters

import (
	"LunarSubsystem/LoggerGeneral"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// LTPX 远程（琉璃）工具链协调
// 设计要点（新版 LTPX 协议）：
//   1. 琉璃启动时一次性向月华提交其联络 URL（多开时以最新注册为准，只记录一个）
//   2. 月华在每条思考链起点主动向琉璃心跳并拉取最新工具链（琉璃可能动态增删 LTPX 插件）
//   3. 工具调用由月华转发到琉璃，琉璃执行并驱动其页面，返回文本结果
//   4. 琉璃下线时，月华需将琉璃提供的工具从对话者可调用工具列表移除

// 远程 HTTP 调用统一超时（琉璃进程心跳/工具链拉取）
const ltpRemoteHTTPTimeout = 8 * time.Second

// 工具调用超时：需覆盖琉璃端 pending call 等待（120s）及浏览器 iframe 中转执行耗时
const ltpRemoteCallTimeout = 150 * time.Second

// RegisterLTPXRemoteURL 记录琉璃联络 URL（琉璃启动时调用；多开时以最新注册为准）
func RegisterLTPXRemoteURL(url string) {
	url = strings.TrimRight(strings.TrimSpace(url), "/")
	if url == "" {
		return
	}
	ltpRemoteMutex.Lock()
	ltpRemoteURL = url
	ltpRemoteMutex.Unlock()
	LoggerGeneral.Info("LunarCore", "LTPX 琉璃联络 URL 已注册: %s", url)
}

// GetLTPXRemoteURL 获取当前记录的琉璃联络 URL
func GetLTPXRemoteURL() string {
	ltpRemoteMutex.RLock()
	defer ltpRemoteMutex.RUnlock()
	return ltpRemoteURL
}

// clearLTPXRemoteState 清空琉璃联络 URL 与工具链缓存（掉线/无响应时调用）
// 琉璃重启后会通过 /ltpx/register 重新注册 URL，因此清空后不影响其再次上线
func clearLTPXRemoteState(reason string) {
	ltpRemoteMutex.Lock()
	ltpRemoteURL = ""
	ltpRemoteTools = nil
	ltpRemoteMutex.Unlock()
	LoggerGeneral.Warn("LunarCore", "LTPX 琉璃联络失效（%s），已清空联络 URL 与工具链", reason)
}

// syncLTPXRemoteTools 向琉璃心跳并拉取最新工具链
// 返回在线状态、琉璃 URL 与最新工具链；播结束后写入内部缓存
func syncLTPXRemoteTools() *LTPXRemoteStatusResult {
	result := &LTPXRemoteStatusResult{
		Online: false,
		URL:    GetLTPXRemoteURL(),
		Tools:  []LTPXRemoteToolDef{},
	}

	target := GetLTPXRemoteURL()
	if target == "" {
		return result
	}

	// 心跳：探测琉璃是否在线
	pingBody, pingStatus, err := remoteGet(target + "/ltpx/ping")
	if err != nil || pingStatus != http.StatusOK {
		// 琉璃无响应（掉线/进程退出/网络不可达）：清空联络 URL 与工具链，
		// 后续思考链不再访问琉璃；琉璃重启注册后自动恢复
		_ = pingBody
		result.URL = ""
		clearLTPXRemoteState("心跳探测失败")
		return result
	}

	// 在线：拉取最新工具链
	body, status, err := remoteGet(target + "/ltpx/tools")
	if err != nil || status != http.StatusOK {
		LoggerGeneral.Warn("LunarCore", "LTPX 拉取琉璃工具链失败: %v (status=%d)", err, status)
		// 工具链拉取失败：清空工具链缓存，保留联络 URL，下轮思考链自动重试
		ltpRemoteMutex.Lock()
		ltpRemoteTools = nil
		ltpRemoteMutex.Unlock()
		return result
	}

	var toolResp struct {
		AppID string               `json:"app_id"`
		Tools []LTPXRemoteToolDef  `json:"tools"`
	}
	if err := json.Unmarshal(body, &toolResp); err != nil {
		LoggerGeneral.Warn("LunarCore", "LTPX 解析琉璃工具链失败: %v", err)
		return result
	}

	tools := toolResp.Tools
	if tools == nil {
		tools = []LTPXRemoteToolDef{}
	}

	ltpRemoteMutex.Lock()
	ltpRemoteTools = tools
	ltpRemoteMutex.Unlock()
	result.Tools = tools
	result.Online = true
	LoggerGeneral.Info("LunarCore", "LTPX 已同步琉璃工具链: %d 个工具", len(tools))
	return result
}

// getLTPXRemoteTools 返回当前缓存的琉璃工具链
func getLTPXRemoteTools() []LTPXRemoteToolDef {
	ltpRemoteMutex.RLock()
	defer ltpRemoteMutex.RUnlock()
	if ltpRemoteTools == nil {
		return []LTPXRemoteToolDef{}
	}
	return ltpRemoteTools
}

// callLTPXRemoteTool 转发工具调用到琉璃并返回文本结果
func callLTPXRemoteTool(toolName, argumentsJSON string) (string, error) {
	target := GetLTPXRemoteURL()
	if target == "" {
		return "", fmt.Errorf("琉璃尚未注册联络 URL，无法调用工具 %s", toolName)
	}

	// 解析 arguments（允许发来缺失/空对象）
	var args map[string]any
	if strings.TrimSpace(argumentsJSON) != "" && argumentsJSON != "{}" {
		if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
			LoggerGeneral.Warn("LunarCore", "LTPX 工具 %s 参数解析失败: %v", toolName, err)
			args = nil
		}
	}
	if args == nil {
		args = map[string]any{}
	}

	reqBody := LTPXRemoteCallRequest{Tool: toolName, Arguments: args}
	reqData, _ := json.Marshal(reqBody)

	body, status, err := remotePost(target+"/ltpx/call", reqData)
	if err != nil {
		// 琉璃无响应（掉线/连接拒绝/超时）：清空联络 URL 与工具链，判定琉璃下线
		clearLTPXRemoteState("工具调用无响应")
		return "", fmt.Errorf("请求琉璃工具 %s 失败: %v", toolName, err)
	}
	if status != http.StatusOK {
		// 异常状态码：非预期响应，同样按琉璃失效处理
		clearLTPXRemoteState(fmt.Sprintf("工具调用异常状态码 %d", status))
		return "", fmt.Errorf("请求琉璃工具 %s 返回状态 %d", toolName, status)
	}

	var callResp LTPXRemoteCallResponse
	if err := json.Unmarshal(body, &callResp); err != nil {
		// 响应无法解析：无法正确获得琉璃响应，按琉璃失效处理
		clearLTPXRemoteState("工具调用响应解析失败")
		return "", fmt.Errorf("解析琉璃工具 %s 响应失败: %v", toolName, err)
	}
	if !callResp.Success {
		// 业务执行失败（琉璃在线，仅工具执行报错）：保留联络 URL，仅返回错误
		if callResp.Error != "" {
			return "", fmt.Errorf("琉璃工具 %s 执行失败: %s", toolName, callResp.Error)
		}
		return "", fmt.Errorf("琉璃工具 %s 执行失败", toolName)
	}
	return callResp.Text, nil
}

// getLTPXRemoteStatusForJS 供 JS 端在思考链起点调用的 Go 函数
// 主动向琉璃心跳并拉取工具链，返回 JSON 字符串（含 online / url / tools）
func (class *Runtime) getLTPXRemoteStatusForJS() goja.Value {
	result := syncLTPXRemoteTools()
	data, _ := json.Marshal(result)
	return class.runtime.ToValue(string(data))
}

// callLTPXRemoteToolForJS 供 JS 端工具函数异步转发调用的 Go 函数
// 参数：toolName, argumentsJSON；返回 Promise，异步 resolve 文本结果
// 琉璃离线/未注册时 resolve 带标识的错误文本，JS 端据此清除该工具
// 异步化设计：HTTP 调用在独立 goroutine 中执行，完成后经 RunOnAgentLoop 回到
// agent 事件循环 resolve（goja Promise 非 goroutine-safe）。
// 这样即使琉璃挂起导致 150s 超时，也不会阻塞 agent 事件循环，月华可继续运行。
func (class *Runtime) callLTPXRemoteToolForJS(toolName string, argumentsJSON string) goja.Value {
	promise, resolve, _ := class.runtime.NewPromise()
	go func() {
		text, err := callLTPXRemoteTool(toolName, argumentsJSON)
		// 回到 eventloop 线程执行 resolve（goja Promise 必须在其创建线程上 resolve）
		RunOnAgentLoop(func(vm *goja.Runtime) {
			if err != nil {
				LoggerGeneral.Warn("LunarCore", "LTPX 远程工具调用失败: %v", err)
				resolve("【琉璃工具调用失败】" + err.Error())
				return
			}
			resolve(text)
		})
	}()
	// *goja.Promise 需经 ToValue 转换后才能作为 goja.Value 返回给 JS
	return class.runtime.ToValue(promise)
}

// clearLTPXRemoteToolsForJS 供 JS 端在判断琉璃离线时清空缓存工具链
func (class *Runtime) clearLTPXRemoteToolsForJS() goja.Value {
	ltpRemoteMutex.Lock()
	ltpRemoteTools = nil
	ltpRemoteMutex.Unlock()
	return class.runtime.ToValue(true)
}

// ==== 内部 HTTP 辅助 ====

func remoteGet(url string) ([]byte, int, error) {
	client := &http.Client{Timeout: ltpRemoteHTTPTimeout}
	resp, err := client.Get(url)
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

func remotePost(url string, payload []byte) ([]byte, int, error) {
	client := &http.Client{Timeout: ltpRemoteCallTimeout}
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