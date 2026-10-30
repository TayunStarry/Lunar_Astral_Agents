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

// TestRouteLogRecordCategories 钉住「系统日志」六个分类的路由边界：
// 对话 / 生活（含哔哩观影、创作、文件编辑、私有 js 运行时）/ 后勤（Tick、通用应答、智能修复）。
func TestRouteLogRecordCategories(t *testing.T) {
	sources := []LogSource{SourceRaw, SourceWarn, SourceDraw, SourceDialogue, SourceLife, SourceLogistics}
	clear := func() {
		for _, s := range sources {
			ClearLogs(s)
		}
	}
	clear()
	t.Cleanup(clear)

	routeLogRecord("INFO", "LunarCore", "", "对话链回合完成")
	if got := GetLogs(SourceDialogue); len(got) != 1 {
		t.Fatalf("LunarCore 日志应进入「对话」，实际 %d 条", len(got))
	}

	for _, module := range []string{"Life", "BilibiliViewer", "创作", "文件编辑", "Runtime"} {
		routeLogRecord("INFO", module, "", module+" 输出")
	}
	if got := GetLogs(SourceLife); len(got) != 5 {
		t.Fatalf("生活系统相关模块日志应全部进入「生活」，实际 %d 条", len(got))
	}

	for _, module := range []string{"TickLoop", "通用应答", "智能修复", "AgentEndpoint"} {
		routeLogRecord("INFO", module, "", module+" 输出")
	}
	if got := GetLogs(SourceLogistics); len(got) != 4 {
		t.Fatalf("后勤系统相关模块日志应全部进入「后勤」，实际 %d 条", len(got))
	}

	routeLogRecord("ERROR", "Runtime", "", "脚本执行失败")
	if got := GetLogs(SourceWarn); len(got) != 1 {
		t.Fatalf("错误日志必须同时进入「警告」，实际 %d 条", len(got))
	}
	if got := GetLogs(SourceLife); len(got) != 6 {
		t.Fatalf("出错的生活日志仍应留在「生活」，实际 %d 条", len(got))
	}
}
