package ArchiveOps

// type.go 归档操作智能体的结果类型。

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// zipEntry 重建压缩包时内存中的条目
type zipEntry struct {
	Name    string
	Data    []byte
	Dir     bool
	Mode    os.FileMode
	ModTime time.Time
}

// Result 一次归档操作的结果（统一序列化为 JSON 回填给调用方 / 模型）
type Result struct {
	OK      bool           `json:"ok"`
	Op      string         `json:"op"`
	Path    string         `json:"path,omitempty"`
	Handles []string       `json:"handles"`
	Summary string         `json:"summary"`
	Entries int            `json:"entries,omitempty"`
	Detail  map[string]any `json:"detail,omitempty"`
}

// JSON 序列化为回填给模型 / 调用方的 JSON 字符串
func (r Result) JSON() string {
	raw, err := json.Marshal(r)
	if err != nil {
		return fmt.Sprintf(`{"ok":false,"op":%q,"handles":[],"summary":"结果序列化失败"}`, r.Op)
	}
	return string(raw)
}

// PreviewRunes 按字符截断文本
func PreviewRunes(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}

// okResult 构造成功结果（handles 保证为非 nil 数组）
func okResult(op, path, summary string, handles []string, entries int, detail map[string]any) Result {
	if handles == nil {
		handles = []string{}
	}
	return Result{OK: true, Op: op, Path: path, Handles: handles, Summary: summary, Entries: entries, Detail: detail}
}

// failResult 构造失败结果
func failResult(op, path string, err error) Result {
	message := op + " 失败：" + err.Error()
	return Result{
		OK:      false,
		Op:      op,
		Path:    path,
		Handles: []string{},
		Summary: PreviewRunes(message, 200),
		Detail:  map[string]any{"error": err.Error()},
	}
}
