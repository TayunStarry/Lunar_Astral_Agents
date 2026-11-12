package main

// ==== 日志链路测试：落盘失败可观测、每条日志都进内存缓冲 ====

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resetLogState 清理全局日志状态（测试之间互不影响）
func resetLogState(t *testing.T) {
	t.Helper()
	if logFile != nil {
		_ = logFile.Close()
	}
	logFile = nil
	logHook = nil
	logFilePath = ""
	logInitErr = nil
	logRingMu.Lock()
	logRing = nil
	logRingSeq = 0
	logRingMu.Unlock()
}

// TestOpenLogReportsDirectoryFailure 日志目录不可创建时必须返回明确原因，不能静默吞掉
func TestOpenLogReportsDirectoryFailure(t *testing.T) {
	resetLogState(t)
	defer resetLogState(t)

	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := openLog(filepath.Join(blocker, "logs", "update_apply.log"))
	if err == nil {
		t.Fatal("父路径是文件时 openLog 应报错")
	}
	if !strings.Contains(err.Error(), "日志") {
		t.Fatalf("错误应说明日志目录/文件问题，实际 %v", err)
	}
	if logFilePath != "" {
		t.Fatalf("失败时不应记录日志路径，实际 %q", logFilePath)
	}
}

// TestLogSinkWritesFileAndRing 日志同时进文件与内存缓冲；TUI 钩子不写 stdout 也不丢行
func TestLogSinkWritesFileAndRing(t *testing.T) {
	resetLogState(t)
	defer resetLogState(t)

	path := filepath.Join(t.TempDir(), "logs", "update_apply.log")
	if err := openLog(path); err != nil {
		t.Fatalf("openLog 失败: %v", err)
	}
	logHook = func(string) {} // 模拟 TUI：只发唤醒信号
	logf("上传失败 HTTP 400：%s", `{"message":"目标分支 master 不存在"}`)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取日志文件失败: %v", err)
	}
	if !strings.Contains(string(data), "HTTP 400") || !strings.Contains(string(data), "master") {
		t.Fatalf("日志文件缺少完整原因:\n%s", data)
	}

	entries, last := readLogRing(0)
	if len(entries) == 0 || last == 0 {
		t.Fatal("内存日志缓冲应记录该行")
	}
	if !strings.Contains(entries[len(entries)-1].Line, "HTTP 400") {
		t.Fatalf("内存缓冲最后一行不正确: %q", entries[len(entries)-1].Line)
	}
	if again, _ := readLogRing(last); len(again) != 0 {
		t.Fatalf("游标之后不应再有重复行，实际 %d 行", len(again))
	}
}

// TestDrainLogsKeepsDroppedLines 模拟通道拥塞：只靠环形缓冲也能补读全部日志
func TestDrainLogsKeepsDroppedLines(t *testing.T) {
	resetLogState(t)
	defer resetLogState(t)

	logHook = func(string) {} // 唤醒信号全部丢弃
	for i := 0; i < 5; i++ {
		logf("第 %d 条", i)
	}
	m := newUIModel(t.TempDir(), nil)
	m.drainLogs()
	var got []string
	for _, l := range m.logs {
		if strings.Contains(l.Text, "第 ") {
			got = append(got, l.Text)
		}
	}
	if len(got) != 5 {
		t.Fatalf("应补读 5 条日志，实际 %d 条: %v", len(got), got)
	}
}
