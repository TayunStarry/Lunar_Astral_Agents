package viewer

// trace.go 单轮观看轨迹：保证「切片 → 解读 → 结论」在本地可追溯。
//
// 轨迹只写 local_data/database/bilibili/trace/<轮次>.jsonl，不写统一记忆库，
// 因此过程数据不会混进长期记忆；轨迹文件最多保留 traceKeepFiles 轮。

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// traceDir 轨迹目录。
func traceDir() string {
	return filepath.Join(stateDir(), traceDirName)
}

// traceFile 本轮轨迹文件（以启动时间命名，保证同一轮追加到同一文件）。
func (r *runner) traceFile() string {
	if r == nil {
		return ""
	}
	stamp := r.startedAt.Format("20060102-150405")
	if r.startedAt.IsZero() {
		stamp = time.Now().Format("20060102-150405")
	}
	return filepath.Join(traceDir(), stamp+".jsonl")
}

// recordSliceTrace 记录一个切片的完整理解（不截断，便于单轮内回溯）。
func (r *runner) recordSliceTrace(video *videoRecord, index, total int, text string) {
	if video == nil || strings.TrimSpace(text) == "" {
		return
	}
	r.appendTrace(viewerTraceRecord{
		Kind:  "slice",
		Bvid:  video.Info.Bvid,
		Title: video.Info.Title,
		Index: index,
		Total: total,
		Text:  text,
	})
}

// recordMergeTrace 记录单视频的合并结论。
func (r *runner) recordMergeTrace(video *videoRecord, summary string) {
	if video == nil || strings.TrimSpace(summary) == "" {
		return
	}
	r.appendTrace(viewerTraceRecord{
		Kind:  "merge",
		Bvid:  video.Info.Bvid,
		Title: video.Info.Title,
		Text:  summary,
	})
}

// appendTrace 追加一条轨迹；失败只告警，不影响观看流程。
func (r *runner) appendTrace(record viewerTraceRecord) {
	path := r.traceFile()
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		logger.Warn("BilibiliViewer", "创建观看轨迹目录失败: %v", err)
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		logger.Warn("BilibiliViewer", "写入观看轨迹失败: %v", err)
		return
	}
	defer file.Close()

	record.Time = time.Now().Unix()
	raw, err := json.Marshal(record)
	if err != nil {
		return
	}
	writer := bufio.NewWriter(file)
	if _, err := writer.WriteString(string(raw) + "\n"); err != nil {
		logger.Warn("BilibiliViewer", "写入观看轨迹失败: %v", err)
		return
	}
	if err := writer.Flush(); err != nil {
		logger.Warn("BilibiliViewer", "写入观看轨迹失败: %v", err)
	}
}

// pruneTraces 只保留最近 traceKeepFiles 轮轨迹（按文件名时间倒序）。
func pruneTraces() int {
	dir := traceDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		names = append(names, entry.Name())
	}
	if len(names) <= traceKeepFiles {
		return 0
	}
	sort.Strings(names) // 文件名以启动时间开头，字典序即时间序
	removed := 0
	for _, name := range names[:len(names)-traceKeepFiles] {
		if err := os.Remove(filepath.Join(dir, name)); err == nil {
			removed++
		}
	}
	if removed > 0 {
		logger.Info("BilibiliViewer", "已清理 %d 份过期观看轨迹", removed)
	}
	return removed
}
