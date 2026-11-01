package FileEditor

// tool_run.go 运行文档库中的 js / html 项目文件。
//
// 分流规则：
//   - 文件首行含 `//Engine: Lunar Harness` 标识 → 交给月华私有 js 运行时执行；
//   - 其余 .js / .html → 打开「资源库」前端，并在其 iframe 沙箱中运行；
//     资源库窗口会自动弹出并作为浏览器端代码的主页面。

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/browser"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	runtime "LunarSubsystem/LunarHarness/engine/runtime"
)

// harnessEngineMark 文件头部标识：命中则使用月华私有 js 运行时。
const harnessEngineMark = "//Engine: Lunar Harness"

// runLimitRe 首行运行时长声明：timeout=120s / timeout: 90 / timeout=5000ms / timeout=2分
var runLimitRe = regexp.MustCompile(`(?i)\btimeout\s*[:=]\s*(\d+)\s*(ms|s|分|秒)?`)

// RunProject 以默认运行时长上限运行指定的 js / html 项目文件。
func RunProject(ctx context.Context, path string) OpResult {
	return runProject(ctx, path, 0)
}

// RunProjectWithLimit 运行项目文件，timeoutSec 为 AI 显式指定的运行时长（秒；<=0 表示不指定）。
// 优先级：显式参数 > 文件首行注释 > 默认 120 秒。
func RunProjectWithLimit(ctx context.Context, path string, timeoutSec int) OpResult {
	return runProject(ctx, path, timeoutSec)
}

// runProject 运行入口：校验文件、解析时长并分流执行。
func runProject(ctx context.Context, path string, timeoutSec int) OpResult {
	content, err := readDocument(path)
	if err != nil {
		return failResult(ToolRunProj, path, err)
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".js" && ext != ".html" && ext != ".htm" {
		return failResult(ToolRunProj, path, fmt.Errorf("只支持运行 .js / .html / .htm 文件"))
	}
	var localRel string
	if _, rel, pathErr := documentPath(path); pathErr == nil {
		localRel = rel
	}

	limit := resolveRunLimit(content, timeoutSec)
	outcome := runProjectFile(ctx, path, ext, content, limit)
	if localRel != "" {
		// 运行结束再上报：成功保持绿色，报错/警告转为红色（供资源库文件名着色）。
		publishRun(localRel, !outcome.OK)
	}
	return outcome
}

// runProjectFile 按文件头标识与扩展名分流执行。
func runProjectFile(ctx context.Context, path, ext, content string, limit time.Duration) OpResult {
	if hasHarnessMark(content) {
		return runWithHarness(ctx, path, content, limit)
	}
	if ext == ".html" || ext == ".htm" {
		return openInBrowser(path, "html")
	}
	return runJSInBrowser(path)
}

// RunProjectPath 以默认运行时长运行指定项目文件（供创作管线等进程内调用方使用）。
func RunProjectPath(path string) OpResult {
	return RunProjectPathWithLimit(path, 0)
}

// RunProjectPathWithLimit 以指定运行时长运行项目文件（timeoutSec <= 0 时使用默认值）。
func RunProjectPathWithLimit(path string, timeoutSec int) OpResult {
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	return RunProjectWithLimit(ctx, path, timeoutSec)
}

// resolveRunLimit 解析本次运行时长：显式参数 > 文件首行声明 > 默认值，并夹在允许区间内。
func resolveRunLimit(content string, timeoutSec int) time.Duration {
	limit := defaultRunLimit
	if timeoutSec > 0 {
		limit = time.Duration(timeoutSec) * time.Second
	} else if declared := runLimitFromFirstLine(content); declared > 0 {
		limit = declared
	}
	switch {
	case limit < minRunLimit:
		return minRunLimit
	case limit > maxRunLimit:
		return maxRunLimit
	}
	return limit
}

// runLimitFromFirstLine 读取文件第一行的运行时长声明。
// 约定：第一行类型注释中可写 `timeout=120s`（支持 ms / s / 分 / 秒，缺省单位为秒）。
func runLimitFromFirstLine(content string) time.Duration {
	content = strings.TrimPrefix(content, "\ufeff")
	firstLine := strings.SplitN(strings.ReplaceAll(content, "\r\n", "\n"), "\n", 2)[0]
	match := runLimitRe.FindStringSubmatch(firstLine)
	if len(match) == 0 {
		return 0
	}
	value, err := strconv.Atoi(match[1])
	if err != nil || value <= 0 {
		return 0
	}
	switch strings.ToLower(match[2]) {
	case "ms":
		return time.Duration(value) * time.Millisecond
	case "分":
		return time.Duration(value) * time.Minute
	default: // s / 秒 / 缺省
		return time.Duration(value) * time.Second
	}
}

// hasHarnessMark 检测文件头部是否包含 `//Engine: Lunar Harness` 标识。
func hasHarnessMark(content string) bool {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		if line == "" {
			continue
		}
		return strings.HasPrefix(line, harnessEngineMark)
	}
	return false
}

// runWithHarness 在月华私有 js 运行时中执行脚本。
//
// 运行时长上限同时约束两件事：
//   - main 同步调用不得超过该时长；
//   - 脚本加载完成后，私有运行时最多驻留该时长，到期自动卸载
//     （订阅事件等长驻脚本不会无限占用事件循环与内存）。
func runWithHarness(ctx context.Context, path, code string, limit time.Duration) OpResult {
	// 运行时长由文件头声明 / 工具参数 / 默认值统一解析，外层 ctx 只负责整轮编辑的总超时。
	_ = ctx
	deadline := time.Now().Add(limit)
	id := "file-" + sanitizeID(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))) + "-" + fmt.Sprint(time.Now().UnixNano()%1_000_000)
	rt, err := runtime.Run(id, path, code)
	if err != nil {
		return failResult(ToolRunProj, path, err)
	}
	exports := rt.Exports()
	d := map[string]any{
		"engine":        "lunar-harness",
		"runtime":       id,
		"exports":       exports,
		"limit_seconds": int(limit.Seconds()),
	}
	// 操作简述不写运行时长：资源库只展示运行结果，不展示时长配置；
	// 时长信息放在结构化字段 limit_seconds 中，供调用方与排查使用。
	summary := "已在月华私有 js 运行时执行 " + path
	if containsString(exports, "main") {
		value, callErr := callMainWithLimit(rt, time.Until(deadline), limit)
		if callErr != nil {
			rt.Close()
			return failResult(ToolRunProj, path, callErr)
		}
		d["result"] = value
		summary += "（main 已调用）"
	}
	// 剩余时长即运行时驻留宽限：到期自动卸载，避免订阅型脚本常驻。
	scheduleRuntimeClose(rt, time.Until(deadline))
	return newResult(ToolRunProj, path, summary, nil, d)
}

// callMainWithLimit 在给定剩余时长内调用导出函数 main。
// 超时后关闭运行时以中断事件循环，并返回可读错误。
func callMainWithLimit(rt *runtime.Runtime, remain, limit time.Duration) (any, error) {
	if remain <= 0 {
		return nil, fmt.Errorf("运行时长已用尽（上限 %d 秒）", int(limit.Seconds()))
	}
	type callOutcome struct {
		value any
		err   error
	}
	done := make(chan callOutcome, 1)
	go func() {
		value, err := rt.Call("main")
		done <- callOutcome{value: value, err: err}
	}()
	select {
	case result := <-done:
		return result.value, result.err
	case <-time.After(remain):
		rt.Close()
		return nil, fmt.Errorf("main 调用超过运行时长上限（%d 秒）", int(limit.Seconds()))
	}
}

// scheduleRuntimeClose 在 after 时长后卸载私有运行时（after <= 0 表示立即卸载）。
func scheduleRuntimeClose(rt *runtime.Runtime, after time.Duration) {
	runID := "unknown"
	if rt != nil {
		// 运行时就绪后 PID 已注册，日志里带上实例标识便于排查「谁被自动卸载」。
		if exports := rt.Exports(); len(exports) > 0 {
			runID = fmt.Sprintf("exports=%d", len(exports))
		}
	}
	if after <= 0 {
		after = 0
	}
	go func() {
		timer := time.NewTimer(after)
		defer timer.Stop()
		<-timer.C
		rt.Close()
		logger.Info(Module, "[文件编辑] 私有 js 运行时已到期自动卸载（%s）", runID)
	}()
}

// runJSInBrowser 把纯 JS 交给「资源库」前端，在页面内的 iframe 沙箱执行。
func runJSInBrowser(path string) OpResult {
	return openLibraryRunner(path, "js")
}

// openInBrowser 用「资源库」前端打开 html 文件，并在沙箱中运行。
func openInBrowser(path, kind string) OpResult {
	return openLibraryRunner(path, kind)
}

// openLibraryRunner 打开（或聚焦）资源库窗口，并带上待运行文件。
//
// 资源库是浏览器端代码的唯一主页面：窗口不存在时自动弹出，已存在时重载到新地址，
// 前端读取 run 参数后在 iframe 沙箱里执行，宿主机不额外起进程。
func openLibraryRunner(path, kind string) OpResult {
	target := libraryRunnerURL(path, kind)
	spec := browser.DesktopWindowSpec{
		ID:     libraryHostWindowID,
		URL:    target,
		Title:  "[星月智能] : 资源库",
		Width:  libraryHostWindowWidth,
		Height: libraryHostWindowHeight,
	}
	if err := browser.RestartDesktopWindow(spec); err != nil {
		logger.Warn(Module, "[运行] 资源库窗口打开失败，回退系统浏览器: %v", err)
		browser.OpenBrowser(target)
	}
	return newResult(ToolRunProj, path,
		"已弹出「资源库」并在 iframe 沙箱中运行 "+path, nil,
		map[string]any{"engine": "library", "run": libraryRunnerRel(path), "url": target})
}

// libraryRunnerRel 待运行文件相对 LocalDir 的路径。
func libraryRunnerRel(path string) string {
	return filepath.ToSlash(filepath.Join(documentsRootRel, normalizeDocPath(path)))
}

// libraryRunnerURL 资源库自动运行地址：/library/?run=<相对路径>&kind=<类型>。
func libraryRunnerURL(path, kind string) string {
	params := url.Values{}
	params.Set("run", libraryRunnerRel(path))
	if kind != "" {
		params.Set("kind", kind)
	}
	return config.LocalBaseURL() + "/library/?" + params.Encode()
}

// sanitizeID 文件名 → 安全标识。
func sanitizeID(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		}
		return '_'
	}, name)
}

// containsString 判断字符串是否在切片中。
func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
