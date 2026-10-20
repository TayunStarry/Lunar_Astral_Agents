package console

import (
	"fmt"
	"strings"
	"testing"
)

// TestLogsStateRefreshesWhenRingBufferFull 回归：日志缓冲满 1000 条后 len(logs) 不再变化，
// 旧实现用长度做缓存键，导致日志屏永远停在旧内容（表现为 “1453-1479/1479 行” 后不再更新）。
func TestLogsStateRefreshesWhenRingBufferFull(t *testing.T) {
	ClearLogs(SourceRaw)
	t.Cleanup(func() { ClearLogs(SourceRaw) })

	s := newLogsState(SourceRaw)
	for i := 0; i < logStoreCap; i++ {
		pushLog(LogLine{Source: "stdout", Level: "INFO", Text: fmt.Sprintf("line-%d", i)}, SourceRaw)
	}
	first := s.content(120)
	if len(first) != logStoreCap {
		t.Fatalf("首次折行行数 = %d，期望 %d", len(first), logStoreCap)
	}
	if !strings.Contains(first[len(first)-1], "line-999") {
		t.Fatalf("首次末尾不是 line-999: %q", first[len(first)-1])
	}

	// 再追加一条：环形缓冲长度仍是 logStoreCap，但内容已滚动。
	pushLog(LogLine{Source: "stdout", Level: "INFO", Text: "line-latest"}, SourceRaw)
	second := s.content(120)
	if len(second) != logStoreCap {
		t.Fatalf("追加后折行行数 = %d，期望 %d", len(second), logStoreCap)
	}
	if !strings.Contains(second[len(second)-1], "line-latest") {
		t.Fatalf("缓冲满后视图没有刷新到最新日志，末尾=%q", second[len(second)-1])
	}
	if strings.Contains(second[0], "line-0") {
		t.Fatalf("缓冲满后最旧日志应被挤出，首行=%q", second[0])
	}
}
