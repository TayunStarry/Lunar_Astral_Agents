package debugsnap

// snapshot.go 开发者模式「告警调试快照」。
//
// 需求：启用 system.developer 时，系统出现警告及以上日志，自动把「原始日志」
// 「推理实况」「全部智能体上下文」持久化到 local_data/logs/debug/<时间戳>/，
// 且**不打开**月华/琉璃任何前端窗口。
//
// 分层：logger 只负责在「开发者模式 + 告警级别」时回调钩子（engine/console/logger），
// 本包负责去重限流 + 取数 + 落盘，app 层在装配时把两者接上。
// 本包刻意不 import app / server / browser：快照链路里没有任何窗口打开逻辑。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/agentdump"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// HandleAlert 告警钩子实现（由 app 装配为 logger.SetAlertHook）。
//
// 非阻塞是硬约束：本函数运行在每一次告警的日志调用路径上，只做
// 「开发者/级别判定 → 原子抢占 + 限流判定 → 启动后台 goroutine」，
// 取数与写盘全部在 goroutine 内完成。并发告警由 snapshotBusy 合并为一次快照。
func HandleAlert(level, module, sub, msg string) {
	if !config.Developer {
		return
	}
	switch level {
	case "WARN", "ERROR", "FATAL":
	default:
		return
	}

	now := time.Now()
	if !claimSnapshot(now) {
		return
	}

	alert := Alert{
		Level:   level,
		Module:  module,
		Sub:     sub,
		Message: truncateRunes(msg, maxMessageRunes),
		At:      now,
	}
	snapshotWG.Add(1)
	go func() {
		defer snapshotWG.Done()
		defer snapshotBusy.Store(false)
		dir, err := writeSnapshot(alert)
		if err != nil {
			// 写失败只记一条日志；此时 snapshotBusy 尚未释放、且限流时间已占位，
			// 不会因这条 Error 再次触发快照。
			logger.Error("DebugSnapshot", "调试快照写入失败: %v", err)
			return
		}
		logger.Info("DebugSnapshot", "调试快照已写入: %s", dir)
	}()
}

// claimSnapshot 原子抢占快照资格并执行 60 秒限流。
//
// 先 CAS 置忙再判限流：并发告警只有一个能走到取数，其余直接放弃；
// 被限流拦下时立刻释放忙标记，不影响后续（冷却期结束后）的第一次告警。
func claimSnapshot(now time.Time) bool {
	if !snapshotBusy.CompareAndSwap(false, true) {
		return false
	}
	last := lastSnapshotAt.Load()
	if last != 0 && now.UnixNano()-last < int64(snapshotCooldown) {
		snapshotBusy.Store(false)
		return false
	}
	lastSnapshotAt.Store(now.UnixNano())
	return true
}

// writeSnapshot 同步写入一次快照（供后台 goroutine 与单测直接调用）。
// 返回快照目录绝对/相对当前工作目录的路径。
func writeSnapshot(alert Alert) (string, error) {
	dir, err := allocateSnapshotDir(alert.At)
	if err != nil {
		return "", err
	}

	// write 写入一个文件；失败即中止（目录已建好，留半份证据也比静默失败好排查）
	write := func(name, body string) error {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
			return fmt.Errorf("写入 %s 失败: %w", name, err)
		}
		return nil
	}

	if err := write(rawLogsFile, sources.rawLogs()); err != nil {
		return "", err
	}

	reasoning, reasoningErr := sources.reasoning()
	if reasoningErr != nil {
		reasoning = errorJSON(reasoningErr)
	}
	if err := write(reasoningFile, reasoning); err != nil {
		return "", err
	}

	agents, agentsErr := sources.agents()
	if agentsErr != nil {
		agents = errorJSON(agentsErr)
	}
	if err := write(agentsFile, agents); err != nil {
		return "", err
	}

	version := strings.TrimSpace(console.BuildVersion())
	if version == "" {
		version = "unknown"
	}
	meta, err := marshalIndent(Meta{
		Kind:            snapshotKind,
		TriggeredAt:     alert.At.Format(time.RFC3339),
		Level:           alert.Level,
		Module:          alert.Module,
		Sub:             alert.Sub,
		Message:         alert.Message,
		Version:         version,
		Developer:       config.Developer,
		RawLogsSource:   rawLogsSourceNote,
		ReasoningSource: reasoningSourceNote,
		AgentsSource:    agentsSourceNote,
		Files:           []string{rawLogsFile, reasoningFile, agentsFile, metaFile},
		Note:            snapshotNote,
	})
	if err != nil {
		return "", fmt.Errorf("序列化 meta.json 失败: %w", err)
	}
	if err := write(metaFile, meta); err != nil {
		return "", err
	}
	return dir, nil
}

// allocateSnapshotDir 创建 <LocalDir>/logs/debug/<YYYYMMDD-HHMMSS> 目录。
// 同一秒内重复触发（测试或极端告警）时追加 -2/-3 后缀，绝不覆盖既有快照。
func allocateSnapshotDir(at time.Time) (string, error) {
	root := filepath.Join(config.LocalDir, "logs", "debug")
	if err := os.MkdirAll(root, 0755); err != nil {
		return "", fmt.Errorf("创建快照目录失败: %w", err)
	}
	base := at.Format("20060102-150405")
	for i := 1; i <= 100; i++ {
		name := base
		if i > 1 {
			name = fmt.Sprintf("%s-%d", base, i)
		}
		dir := filepath.Join(root, name)
		mkErr := os.Mkdir(dir, 0755)
		if mkErr == nil {
			return dir, nil
		}
		if !os.IsExist(mkErr) {
			return "", fmt.Errorf("创建快照目录失败: %w", mkErr)
		}
	}
	return "", fmt.Errorf("创建快照目录失败: %s 下同名目录过多", base)
}

// collectRawLogs 复用「原始日志」缓冲，按 JSON Lines 写出。
func collectRawLogs() string {
	content, _ := console.BuildRawLogsJSONL()
	return content
}

// collectReasoning 复用核心智能实况的导出（轮次环形缓冲）。
func collectReasoning() (string, error) {
	return console.BuildTranscriptJSON()
}

// collectAgentsContext 导出全部已注册智能体的上下文并合并为一个 JSON 文件。
//
// 单个目标导出失败只记入该条目的 error 字段，不影响其余目标——
// 告警时刻正需要「能拿到多少写多少」，不能让一个坏 provider 毁掉整份快照。
func collectAgentsContext() (string, error) {
	providers := agentdump.List()
	entries := make([]AgentEntry, 0, len(providers))
	for _, provider := range providers {
		entry := AgentEntry{ID: provider.ID, Label: provider.Label, Group: provider.Group}
		content, err := agentdump.Export(provider.ID, false)
		switch {
		case err != nil:
			entry.Error = err.Error()
		case strings.TrimSpace(content) == "":
			entry.Error = "导出内容为空"
		case json.Valid([]byte(content)):
			entry.Content = json.RawMessage(content)
		default:
			// 非 JSON 内容按字符串保存，保证文件仍是合法 JSON
			quoted, quoteErr := json.Marshal(content)
			if quoteErr != nil {
				entry.Error = quoteErr.Error()
			} else {
				entry.Content = quoted
			}
		}
		entries = append(entries, entry)
	}
	return marshalIndent(AgentsContext{
		Kind:        "lunar-agents-context",
		GeneratedAt: time.Now().Format(time.RFC3339),
		Source:      agentsSourceNote,
		Count:       len(entries),
		Agents:      entries,
	})
}

// marshalIndent 缩进序列化（关闭 HTML 转义，便于人工阅读）
func marshalIndent(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// errorJSON 把取数错误写成合法 JSON（该文件仍会落盘，便于看出缺数据的原因）
func errorJSON(err error) string {
	content, marshalErr := marshalIndent(map[string]string{"error": err.Error()})
	if marshalErr != nil {
		return "{\"error\":\"取数失败\"}\n"
	}
	return content
}

// truncateRunes 按字符（而非字节）截断，避免切出半个 UTF-8 字符
func truncateRunes(s string, limit int) string {
	if limit <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "…"
}
