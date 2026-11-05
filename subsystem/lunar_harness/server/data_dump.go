package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logbuf"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	persistence "LunarSubsystem/LunarHarness/engine/persistence"
	"LunarSubsystem/LunarHarness/protocol/agentdump"
)

// ==== 数据导出（TUI「数据导出」与 HTTP 端点共用同一实现） ====
//
// 控制台需要把运行期数据落到磁盘以便离线排查，而两类数据的**产出位置不同**：
//   - 原始日志：已在 TUI 日志管线（同进程内存环形缓冲）里 → 本包直接序列化落盘；
//   - 智能体上下文：Go 思维链在进程内持有 → 经 thinking.DumpAgentContext 直接导出
//     事件循环上取回 JSON 文本，再落盘。
//
// 因此这里只做「取数 → 写文件 → 回执」三件事，导出内容的构造分别由上两处负责，
// 避免出现第三份"日志格式"或"上下文结构"的定义。
//
// 分层：DataDump 是可直接调用的服务函数（app 的 TUI 钩子直接调用，无 HTTP 往返），
// 三个 Handler 只做方法校验与查询参数适配。

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

// DataDump 执行一次数据导出。
//
// 返回值语义：
//   - err != nil：内部失败（写盘失败等），HTTP handler 映射为 500；
//   - err == nil 且 resp.OK == false：业务性失败（缓冲为空 / 目标不存在等），
//     HTTP handler 仍返回 200，与既有端点行为一致。
func DataDump(ctx context.Context, req DataDumpRequest) (DataDumpResponse, error) {
	if err := ctx.Err(); err != nil {
		return DataDumpResponse{OK: false, Error: "导出已取消"}, nil
	}
	switch req.Kind {
	case DataDumpLogs:
		return dumpLogs()
	case DataDumpTranscript:
		return dumpTranscript()
	case DataDumpContext:
		return dumpContext(req.Agent, req.IncludeImages)
	default:
		return DataDumpResponse{}, fmt.Errorf("未知导出类型: %s", req.Kind)
	}
}

// dumpLogs 导出原始日志为 JSON Lines。
//
// 用 JSONL 而非单个 JSON 数组：日志是流式数据，逐行 JSON 可被 jq/grep 直接处理，
// 也便于只看末尾若干行（tail）而不必解析整个文件。
//
// 两个「为可读性服务」的取舍：
//  1. 正文一律经 logbuf.ScrubLogText 再写出：导出给的是**离线排查用的文本**，
//     终端控制序列（彩色工具的 `\u001b[…`、进度行的 `\r`）只会干扰阅读与 grep。
//     清洗本已在日志入缓冲时做过，这里再兜一次——避免今后有人绕过管线直接写入
//     时把转义码漏进导出文件。
//  2. **关闭 encoding/json 的 HTML 转义**：`json.Marshal` 默认把 `>` `<` `&` 写成
//     `\u003e` / `\u003c` / `\u0026`（防的是"把 JSON 直接嵌进 HTML"这种场景）。
//     普通日志里 `->`、`=>`、`&&` 极常见，转义后整份文件几乎没法用肉眼扫读，
//     而导出文件只落本地磁盘、不做 HTML 内嵌，故改用 Encoder 并 SetEscapeHTML(false)。
func dumpLogs() (DataDumpResponse, error) {
	lines := logbuf.GetLogs(logbuf.SourceRaw)
	if len(lines) == 0 {
		return DataDumpResponse{OK: false, Error: "原始日志缓冲为空：还没有任何输出"}, nil
	}

	// 序列化口径与「开发者调试快照」共用 logbuf.BuildRawLogsJSONL，避免两处导出格式漂移
	content, written := logbuf.BuildRawLogsJSONL()
	path, size, err := writeDumpFile("raw-logs", "log", []byte(content))
	if err != nil {
		return DataDumpResponse{OK: false, Error: err.Error()}, err
	}
	logger.Info("LunarHarness", "原始日志已导出: %s（%d 行 / %d 字节）", path, written, size)
	return DataDumpResponse{
		OK: true, Path: path, Bytes: size, Records: written,
		Filename: filepath.Base(path), Target: "raw-logs",
	}, nil
}

// dumpTranscript 导出「核心智能实况」的模型调用轮次。
//
// 为什么单独一条：实况轮次（每轮的角色 / 思考全文 / 输出全文 / 工具调用序列 / 时长）
// 是**独一份**的运行时资料，任何日志通道都不覆盖——
//   - 原始日志只有一行行启动与工具文本，没有"轮次"结构；
//   - 月华日志是思维链运行输出，与模型调用轮次没有一一对应关系；
//   - `/debug/dump/context` 导出的是**给模型看的 messages（输入）**，
//     不含模型思考流，也不含工具调用顺序与耗时。
//
// 数据来自 TUI 的轮次环形缓冲，因此带 created/dropped 与逐轮 truncated 标记，
// 如实告知"这份导出是全部还是仅最近一段"。
func dumpTranscript() (DataDumpResponse, error) {
	content, err := logbuf.BuildTranscriptJSON()
	if err != nil {
		return DataDumpResponse{OK: false, Error: err.Error()}, err
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
		return DataDumpResponse{OK: false, Error: err.Error()}, err
	}
	logger.Info("LunarHarness", "实况轮次已导出: %s（%d 轮 / %d 字节）", path, probe.Meta.TurnCount, size)
	return DataDumpResponse{
		OK: true, Path: path, Bytes: size, Records: probe.Meta.TurnCount,
		Filename: filepath.Base(path), Target: "transcript",
	}, nil
}

// dumpContext 导出 Go 智能体上下文/状态为 JSON。
//
// agent 取值来自 protocol/agentdump 的注册表：dialogue（对话者与主上下文）、web_search、
// bilibili、desktop、dialogue_bridging、mutual_forum 等。旧目标 core / lunar 作为兼容
// 别名映射到 dialogue（保留一个发布周期，待调用方与文档全部迁移后移除）。
// includeImages 为真时保留图片原始 base64（体积很大，仅深度排查用）。
func dumpContext(agent string, includeImages bool) (DataDumpResponse, error) {
	agent = strings.TrimSpace(agent)
	switch agent {
	case "", "core", "lunar":
		agent = "dialogue"
	}

	content, err := agentdump.Export(agent, includeImages)
	if err != nil {
		return DataDumpResponse{OK: false, Target: agent, Error: err.Error()}, nil
	}
	if strings.TrimSpace(content) == "" {
		return DataDumpResponse{OK: false, Target: agent, Error: "导出内容为空"}, nil
	}
	// 运行时的导出实现以 {"error":…} 形式表达"目标不存在"等失败，这里透传为可读原因
	if strings.HasPrefix(strings.TrimSpace(content), "{\"error\"") {
		var parsed struct {
			Error string `json:"error"`
		}
		if json.Unmarshal([]byte(content), &parsed) == nil && parsed.Error != "" {
			return DataDumpResponse{OK: false, Target: agent, Error: parsed.Error}, nil
		}
	}

	path, size, err := writeDumpFile("context-"+agent, "json", []byte(content))
	if err != nil {
		return DataDumpResponse{OK: false, Target: agent, Error: err.Error()}, err
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
	return DataDumpResponse{
		OK: true, Path: path, Bytes: size, Records: records,
		Filename: filepath.Base(path), Target: agent,
	}, nil
}

// DataDumpLogsHandler 导出原始日志为 JSON Lines（POST /debug/dump/logs）。
func DataDumpLogsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp, err := DataDump(r.Context(), DataDumpRequest{Kind: DataDumpLogs})
	writeDataDumpResult(w, resp, err)
}

// DataDumpTranscriptHandler 导出「核心智能实况」的模型调用轮次（POST /debug/dump/transcript）。
func DataDumpTranscriptHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp, err := DataDump(r.Context(), DataDumpRequest{Kind: DataDumpTranscript})
	writeDataDumpResult(w, resp, err)
}

// DataDumpContextHandler 导出 Go 智能体上下文/状态为 JSON（POST /debug/dump/context?agent=xxx）。
func DataDumpContextHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	agent := r.URL.Query().Get("agent")
	includeImages := r.URL.Query().Get("include_images") == "1"
	resp, err := DataDump(r.Context(), DataDumpRequest{
		Kind:          DataDumpContext,
		Agent:         agent,
		IncludeImages: includeImages,
	})
	writeDataDumpResult(w, resp, err)
}

// writeDataDumpResult 输出导出回执：内部失败统一 500，业务性失败保持 200。
func writeDataDumpResult(w http.ResponseWriter, resp DataDumpResponse, err error) {
	if err != nil {
		resp.OK = false
		if resp.Error == "" {
			resp.Error = err.Error()
		}
		writeJSON(w, http.StatusInternalServerError, resp)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
