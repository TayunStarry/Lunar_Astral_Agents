package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"CorePersonality/engine/console"
	"LunarSubsystem/GeneralConfig"
)

// 清掉本次用例写入的导出文件（避免污染工作目录）
func cleanupDumps(t *testing.T, pattern string) {
	t.Helper()
	glob := filepath.Join(*GeneralConfig.LocalDir, "logs", "dump", pattern)
	t.Cleanup(func() {
		if entries, err := filepath.Glob(glob); err == nil {
			for _, e := range entries {
				_ = os.Remove(e)
			}
		}
	})
}

// doDump 进程内直调导出端点并解析回执
func doDump(t *testing.T, handler http.HandlerFunc, target string) DataDumpResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodPost, target, nil))
	var resp DataDumpResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析导出回执失败: %v（body=%s）", err, rec.Body.String())
	}
	return resp
}

// readDump 读回导出文件内容
func readDump(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读回导出文件失败（%s）: %v", path, err)
	}
	return string(raw)
}

// 覆盖：`/debug/dump/logs` 导出的 JSONL 里**不得残留终端控制序列**。
//
// 这是「导出原始日志时应剔除颜色代码」的直接验收。历史实现只在写 console.log 时做
// ansi.Strip，**入缓冲与导出仍带转义码**，导出的 .log 里混着 `\u001b[32m` 与裸 `\r`。
//
// 本测试走真实链路：TuiSystem 日志管线入缓冲 → 导出端点读缓冲 → 落盘 → 读回断言。
func TestDataDumpLogsStripsEscapeCodes(t *testing.T) {
	console.Install() // 幂等；确保环形缓冲可用
	cleanupDumps(t, "*-raw-logs.log")

	// 经管线入口投递带颜色与回车的日志：清理成多行后应各自成为一条 JSONL 记录
	console.PushTaskLog("\x1b[31m[ERROR]\x1b[0m 模型加载失败")
	console.PushTaskLog("采样 40%\r采样 100%")
	console.PushTaskLog("\x1b]0;窗口标题\x07完成标记")

	resp := doDump(t, DataDumpLogsHandler, "/debug/dump/logs")
	if !resp.OK {
		t.Fatalf("导出失败: %s", resp.Error)
	}
	if resp.Records == 0 {
		t.Fatal("导出记录数为 0")
	}
	content := readDump(t, resp.Path)

	// 断言 1：不得出现 ESC 字节、裸回车、OSC 残渣的 ESC 引导
	if strings.ContainsRune(content, 0x1b) {
		t.Fatalf("导出文件仍含 ESC 字节:\n%s", content)
	}
	if strings.ContainsRune(content, '\r') {
		t.Fatalf("导出文件仍含裸回车:\n%s", content)
	}
	if strings.Contains(content, "]0;") {
		t.Fatalf("OSC 终端标题序列未被剔除:\n%s", content)
	}

	// 断言 2：逐行合法 JSON，正文收齐后核对（既要可读，也要保住 \r 的分行信息）
	texts := make([]string, 0, resp.Records)
	for i, line := range strings.Split(strings.TrimRight(content, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("第 %d 行不是合法 JSON: %v（%s）", i, err, line)
		}
		text, _ := entry["text"].(string)
		texts = append(texts, text)
	}

	// 断言 3：正文恰好是剥净后的文本
	for _, want := range []string{"[ERROR] 模型加载失败", "采样 40%", "采样 100%", "完成标记"} {
		found := false
		for _, got := range texts {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("导出内容缺少剥净后的正文 %q\n实际记录:\n%s", want, strings.Join(texts, "\n"))
		}
	}
	// 反向断言：\r 不能被拼成一行（"采样 40%采样 100%" 属回归）
	for _, got := range texts {
		if strings.Contains(got, "40%采样") {
			t.Fatalf("\\r 未拆分为独立记录: %q", got)
		}
	}
}

// 覆盖：导出正文里的 `>` `<` `&` 必须保持原样，不得被写成 `\u003e` / `\u003c` / `\u0026`。
//
// 这是 `json.Marshal` 的默认行为（HTML 转义，防"把 JSON 嵌进 HTML"），而普通日志里
// `->`、`=>`、`&&` 极常见——实测用户导出 261 行日志里几乎每条都带 `\u003e`，
// 肉眼扫读与 grep `->` 全部失效。导出文件只落本地磁盘，故用 Encoder 关闭转义。
func TestDataDumpLogsKeepsHtmlSensitiveCharsReadable(t *testing.T) {
	console.Install()
	cleanupDumps(t, "*-raw-logs.log")

	console.PushTaskLog("[CorePersonality] -> 统一 WebSocket 集线器已启动")
	console.PushTaskLog(`路径 D:\a\b 且 "引号" & <标签> 都在`)

	resp := doDump(t, DataDumpLogsHandler, "/debug/dump/logs")
	if !resp.OK {
		t.Fatalf("导出失败: %s", resp.Error)
	}
	content := readDump(t, resp.Path)

	for _, bad := range []string{`\u003e`, `\u003c`, `\u0026`, `\u003E`, `\u003C`} {
		if strings.Contains(content, bad) {
			t.Fatalf("导出文件仍含 HTML 转义 %s:\n%s", bad, content)
		}
	}

	want := map[string]bool{
		"[CorePersonality] -> 统一 WebSocket 集线器已启动": false,
		`路径 D:\a\b 且 "引号" & <标签> 都在`:               false,
	}
	for i, line := range strings.Split(strings.TrimRight(content, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("第 %d 行不是合法 JSON（关闭 HTML 转义不能破坏必要转义）: %v（%s）", i, err, line)
		}
		if text, _ := entry["text"].(string); want[text] == false {
			if _, ok := want[text]; ok {
				want[text] = true
			}
		}
	}
	for text, found := range want {
		if !found {
			t.Fatalf("未按原样导出正文 %q\n实际内容:\n%s", text, content)
		}
	}
	if !strings.Contains(content, "-> 统一 WebSocket") {
		t.Fatalf("`->` 无法直接 grep:\n%s", content)
	}
}

// 覆盖：`/debug/dump/transcript` 能导出「核心智能实况」的模型调用轮次。
//
// 这条数据是独一份的：原始日志没有轮次结构，`/debug/dump/context` 只有给模型看的
// messages（输入）——模型的思考流、工具调用顺序与每轮耗时只存在于实况轮次缓冲里。
func TestDataDumpTranscriptExportsLiveTurns(t *testing.T) {
	cleanupDumps(t, "*-transcript.json")

	// 走真实管道：向实况轮次缓冲投递一轮，再经端点导出
	console.StreamBegin("对话者")
	console.StreamDelta("对话者", console.KindThink, "先看图再回答")
	console.StreamDelta("对话者", console.KindContent, "这是回答 a -> b")
	console.StreamTool("对话者", "list_images", `{"limit":5}`)
	console.StreamEnd("对话者")

	resp := doDump(t, DataDumpTranscriptHandler, "/debug/dump/transcript")
	if !resp.OK {
		t.Fatalf("导出失败: %s", resp.Error)
	}
	if resp.Target != "transcript" {
		t.Fatalf("target 错误: %q", resp.Target)
	}
	if resp.Records < 1 {
		t.Fatalf("记录轮数应 ≥1，实际 %d", resp.Records)
	}
	content := readDump(t, resp.Path)

	// 键名与结构必须与 console.TranscriptDump 一致（端点只落盘，不重塑结构）
	var parsed struct {
		Meta struct {
			Kind      string `json:"kind"`
			TurnCount int    `json:"turn_count"`
		} `json:"meta"`
		Turns []struct {
			Role     string   `json:"role"`
			Thinking string   `json:"thinking"`
			Content  string   `json:"content"`
			Tools    []string `json:"tools"`
		} `json:"turns"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("导出内容不是合法 JSON: %v", err)
	}
	if parsed.Meta.Kind != "lunar-agent-transcript" {
		t.Fatalf("kind 错误: %q", parsed.Meta.Kind)
	}
	found := false
	for _, turn := range parsed.Turns {
		if turn.Role == "对话者" && turn.Content == "这是回答 a -> b" {
			found = true
			if turn.Thinking != "先看图再回答" {
				t.Fatalf("思考内容错误: %q", turn.Thinking)
			}
			if len(turn.Tools) != 1 {
				t.Fatalf("工具序列错误: %#v", turn.Tools)
			}
		}
	}
	if !found {
		t.Fatalf("未导出投递的轮次:\n%s", content)
	}
	if strings.Contains(content, `\u003e`) {
		t.Fatalf("含 HTML 转义 \\u003e:\n%s", content)
	}
}
