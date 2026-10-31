package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/agentdump"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	persistence "LunarSubsystem/LunarHarness/engine/persistence"
)

// ==== 数据导出端点（TUI「数据导出」调用） ====
//
// 控制台需要把运行期数据落到磁盘以便离线排查，而两类数据的**产出位置不同**：
//   - 原始日志：已在 TUI 日志管线（同进程内存环形缓冲）里 → 本包直接序列化落盘；
//   - 智能体上下文：Go 对话链在进程内持有 → 经 LunarPersonality.DumpAgentContext 直接导出
//     事件循环上取回 JSON 文本，再落盘。
//
// 因此这里只做「取数 → 写文件 → 回执」三件事，导出内容的构造分别由上两处负责，
// 避免出现第三份"日志格式"或"上下文结构"的定义。

// DataDumpResponse 导出回执（TUI 据此提示用户）
type DataDumpResponse struct {
	OK       bool   `json:"ok"`
	Path     string `json:"path,omitempty"`     // 落盘路径（相对程序根目录，便于用户查找）
	Bytes    int    `json:"bytes,omitempty"`    // 文件字节数
	Records  int    `json:"records,omitempty"`  // 记录条数（日志行数 / 上下文消息数）
	Target   string `json:"target,omitempty"`   // 导出目标（智能体名）
	Error    string `json:"error,omitempty"`    // 失败原因
	Filename string `json:"filename,omitempty"` // 文件名（回执展示用）
}

// dumpDir 导出目录（相对程序根目录）
func dumpDir() string {
	return filepath.Join(config.LocalDir, "logs", "dump")
}

// writeDumpFile 写入导出文件，返回绝对路径与字节数。
// 文件名格式：<时间戳>-<名称>.<扩展名>，时间戳保证多次导出不互相覆盖。
// 文件写入统一经 persistence（Lunar Harness 内文件操作的唯一实现）。
func writeDumpFile(name, ext string, data []byte) (string, int, error) {
	// 名称中的路径分隔符与空白替换为下划线，避免越出导出目录
	safe := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_").Replace(strings.TrimSpace(name))
	if safe == "" {
		safe = "dump"
	}
	filename := fmt.Sprintf("%s-%s.%s", time.Now().Format("20060102-150405"), safe, ext)
	rel := filepath.ToSlash(filepath.Join("logs", "dump", filename))
	result, err := persistence.SaveFileEx(rel, persistence.SaveOptions{Overwrite: true}, bytes.NewReader(data))
	if err != nil {
		return "", 0, fmt.Errorf("写入导出文件失败: %w", err)
	}
	return result.Path, len(data), nil
}

// DataDumpLogsHandler 导出原始日志为 JSON Lines（POST /debug/dump/logs）。
//
// 用 JSONL 而非单个 JSON 数组：日志是流式数据，逐行 JSON 可被 jq/grep 直接处理，
// 也便于只看末尾若干行（tail）而不必解析整个文件。
//
// 两个「为可读性服务」的取舍：
//  1. 正文一律经 console.ScrubLogText 再写出：导出给的是**离线排查用的文本**，
//     终端控制序列（彩色工具的 `\u001b[…`、进度行的 `\r`）只会干扰阅读与 grep。
//     清洗本已在日志入缓冲时做过，这里再兜一次——避免今后有人绕过管线直接写入
//     （例如经 console.PushTaskLog 之外的路径）时把转义码漏进导出文件。
//  2. **关闭 encoding/json 的 HTML 转义**：`json.Marshal` 默认把 `>` `<` `&` 写成
//     `\u003e` / `\u003c` / `\u0026`（防的是"把 JSON 直接嵌进 HTML"这种场景）。
//     普通日志里 `->`、`=>`、`&&` 极常见，转义后整份文件几乎没法用肉眼扫读，
//     而导出文件只落本地磁盘、不做 HTML 内嵌，故改用 Encoder 并 SetEscapeHTML(false)。
func DataDumpLogsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	lines := console.GetLogs(console.SourceRaw)
	if len(lines) == 0 {
		writeJSON(w, http.StatusOK, DataDumpResponse{OK: false, Error: "原始日志缓冲为空：还没有任何输出"})
		return
	}

	// 序列化口径与「开发者调试快照」共用 console.BuildRawLogsJSONL，避免两处导出格式漂移
	content, written := console.BuildRawLogsJSONL()
	path, size, err := writeDumpFile("raw-logs", "log", []byte(content))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, DataDumpResponse{OK: false, Error: err.Error()})
		return
	}
	logger.Info("LunarHarness", "原始日志已导出: %s（%d 行 / %d 字节）", path, written, size)
	writeJSON(w, http.StatusOK, DataDumpResponse{
		OK: true, Path: path, Bytes: size, Records: written,
		Filename: filepath.Base(path), Target: "raw-logs",
	})
}

// DataDumpTranscriptHandler 导出「核心智能实况」的模型调用轮次（POST /debug/dump/transcript）。
//
// 为什么单独一条：实况轮次（每轮的角色 / 思考全文 / 输出全文 / 工具调用序列 / 时长）
// 是**独一份**的运行时资料，任何日志通道都不覆盖——
//   - 原始日志只有一行行启动与工具文本，没有"轮次"结构；
//   - 月华日志是对话链运行输出，与模型调用轮次没有一一对应关系；
//   - `/debug/dump/context` 导出的是**给模型看的 messages（输入）**，
//     不含模型思考流，也不含工具调用顺序与耗时。
//
// 数据来自 TUI 的轮次环形缓冲，因此带 created/dropped 与逐轮 truncated 标记，
// 如实告知"这份导出是全部还是仅最近一段"。
func DataDumpTranscriptHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	content, err := console.BuildTranscriptJSON()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, DataDumpResponse{OK: false, Error: err.Error()})
		return
	}
	// 探测结果用结构体而非 map：类型明确，后续加字段不会静默漏读
	var probe struct {
		Meta struct {
			TurnCount int `json:"turn_count"`
		} `json:"meta"`
	}
	_ = json.Unmarshal([]byte(content), &probe)

	path, size, err := writeDumpFile("transcript", "json", []byte(content))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, DataDumpResponse{OK: false, Error: err.Error()})
		return
	}
	logger.Info("LunarHarness", "实况轮次已导出: %s（%d 轮 / %d 字节）", path, probe.Meta.TurnCount, size)
	writeJSON(w, http.StatusOK, DataDumpResponse{
		OK: true, Path: path, Bytes: size, Records: probe.Meta.TurnCount,
		Filename: filepath.Base(path), Target: "transcript",
	})
}

// DataDumpContextHandler 导出 Go 智能体上下文/状态为 JSON（POST /debug/dump/context?agent=xxx）。
//
// agent 取值来自 engine/agentdump 的注册表：core（核心人格集合）、web_search、
// bilibili、desktop、dialogue_bridging、mutual_forum 等。分角色的旧目标已合并到 core。
// include_images=1 时保留图片原始 base64（体积很大，仅深度排查用）。
func DataDumpContextHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	agent := strings.TrimSpace(r.URL.Query().Get("agent"))
	if agent == "" {
		agent = "core"
	}
	includeImages := r.URL.Query().Get("include_images") == "1"

	content, err := agentdump.Export(agent, includeImages)
	if err != nil {
		writeJSON(w, http.StatusOK, DataDumpResponse{OK: false, Target: agent, Error: err.Error()})
		return
	}
	if strings.TrimSpace(content) == "" {
		writeJSON(w, http.StatusOK, DataDumpResponse{OK: false, Target: agent, Error: "导出内容为空"})
		return
	}
	// 运行时的导出实现以 {"error":…} 形式表达"目标不存在"等失败，这里透传为可读原因
	if strings.HasPrefix(strings.TrimSpace(content), "{\"error\"") {
		var parsed struct {
			Error string `json:"error"`
		}
		if json.Unmarshal([]byte(content), &parsed) == nil && parsed.Error != "" {
			writeJSON(w, http.StatusOK, DataDumpResponse{OK: false, Target: agent, Error: parsed.Error})
			return
		}
	}

	path, size, err := writeDumpFile("context-"+agent, "json", []byte(content))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, DataDumpResponse{OK: false, Target: agent, Error: err.Error()})
		return
	}
	/** 记录条数（从导出结构里读，失败则记 0，不影响导出本身） */
	records := 0
	var probe struct {
		MessageCount int `json:"message_count"`
	}
	if json.Unmarshal([]byte(content), &probe) == nil {
		records = probe.MessageCount
	}
	logger.Info("LunarHarness", "智能体上下文已导出: %s（%s，%d 条 / %d 字节）", path, agent, records, size)
	writeJSON(w, http.StatusOK, DataDumpResponse{
		OK: true, Path: path, Bytes: size, Records: records,
		Filename: filepath.Base(path), Target: agent,
	})
}
