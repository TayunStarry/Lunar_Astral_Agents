package console

import "testing"

// TestRouteLogRecordSeparatesMediaFromDraw 钉住日志路由边界：
// 视频抽帧/关键帧/图片缩放（MediaAnalysis）不得进入绘图屏，
// 绘图屏只接收 sd.cpp 绘图链路（MultimodalAnalysis）。
func TestRouteLogRecordSeparatesMediaFromDraw(t *testing.T) {
	ClearLogs(SourceRaw)
	ClearLogs(SourceDraw)
	t.Cleanup(func() {
		ClearLogs(SourceRaw)
		ClearLogs(SourceDraw)
	})

	routeLogRecord("INFO", "MediaAnalysis", "", "VideoToWatchSlices: 时长 600s → 8 片（每片 12 帧）")
	if got := GetLogs(SourceDraw); len(got) != 0 {
		t.Fatalf("视频切片日志不应进入绘图屏，实际 %d 条: %q", len(got), got[0].Text)
	}
	if got := GetLogs(SourceRaw); len(got) != 1 {
		t.Fatalf("视频切片日志仍应进入原始日志，实际 %d 条", len(got))
	}

	routeLogRecord("INFO", "MultimodalAnalysis", "", "sd.cpp 采样 10/20")
	if got := GetLogs(SourceDraw); len(got) != 1 {
		t.Fatalf("绘图模块日志应进入绘图屏，实际 %d 条", len(got))
	}
}
