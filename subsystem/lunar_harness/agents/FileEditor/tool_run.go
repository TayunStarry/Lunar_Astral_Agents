package FileEditor

// tool_run.go 运行文档库中的 js / html 项目文件。
//
// 分流规则：
//   - 文件首行含 `//Engine: Lunar Harness` 标识 → 交给月华私有 js 运行时执行；
//   - 其余 .js / .html → 统一使用 engine/browser 浏览器运行环境；
//     纯 js 自动套一层可展示运行结果的模板网页容器（控制台输出与异常都会显示在页面上）。

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
	"LunarSubsystem/LunarHarness/engine/persistence"
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
		// 运行结束再上报：成功保持黄色，报错/警告转为红色（供「编辑显示」文件名着色）。
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
	return runJSInBrowser(path, content)
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
	// 操作简述不写运行时长：TUI「编辑显示」只提供运行入口，不展示时长配置；
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

// runJSInBrowser 把纯 JS 套进模板网页容器后交给浏览器运行环境。
func runJSInBrowser(path, code string) OpResult {
	page := jsRunnerTemplate(path, code)
	name := sanitizeID(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	localRel := filepath.ToSlash(filepath.Join("cache", "run", fmt.Sprintf("%s-%d.html", name, time.Now().UnixNano()%1_000_000)))
	if err := persistence.WriteFileAtomic(localRel, []byte(page)); err != nil {
		return failResult(ToolRunProj, path, fmt.Errorf("生成运行容器失败: %w", err))
	}
	target := webURL(localRel)
	browser.OpenBrowser(target)
	return newResult(ToolRunProj, path,
		"已生成模板网页容器并在浏览器运行环境打开 "+path, nil,
		map[string]any{"engine": "browser", "container": localRel, "url": target})
}

// openInBrowser 直接用浏览器运行环境打开 html 文件。
func openInBrowser(path, kind string) OpResult {
	localRel := filepath.ToSlash(filepath.Join(documentsRootRel, normalizeDocPath(path)))
	target := webURL(localRel)
	browser.OpenBrowser(target)
	return newResult(ToolRunProj, path,
		"已在浏览器运行环境打开 "+path, nil,
		map[string]any{"engine": "browser", "kind": kind, "url": target})
}

// webURL 把 LocalDir 下的相对路径转成本机 Web 服务的只读地址（浏览器运行环境可直接渲染）。
func webURL(localRel string) string {
	escaped := (&url.URL{Path: "/file/read/" + filepath.ToSlash(localRel)}).EscapedPath()
	return config.LocalBaseURL() + escaped
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

// jsRunnerTemplate 纯 JS 运行容器：页面顶部标注文件与引擎，下方实时展示 console 输出与异常。
func jsRunnerTemplate(path, code string) string {
	escaped := strings.ReplaceAll(code, "</script>", "<\\/script>")
	title := "JS 运行结果 · " + filepath.Base(path)
	return `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<title>` + title + `</title>
<style>
  :root { color-scheme: dark; }
  body { margin:0; font:14px/1.6 "Microsoft YaHei","PingFang SC",sans-serif; background:#14121c; color:#e9e6f5; }
  header { padding:14px 20px; border-bottom:1px solid #2a2740; display:flex; gap:14px; align-items:baseline; }
  header b { color:#b39cff; }
  header span { color:#8f8aa8; font-size:12px; }
  #out { padding:16px 20px; white-space:pre-wrap; word-break:break-word; }
  .line { padding:4px 8px; border-left:3px solid #3a3550; margin-bottom:6px; }
  .log { border-color:#7ec8ff; }
  .warn { border-color:#ffc069; color:#ffc069; }
  .error { border-color:#ff7b8a; color:#ff9aa5; }
  .done { color:#7ee2a8; margin-top:14px; }
</style>
</head>
<body>
<header><b>JS 运行结果</b><span>` + filepath.ToSlash(path) + ` · 浏览器运行环境</span></header>
<div id="out"></div>
<script>
(function(){
  var out = document.getElementById('out');
  function push(cls, args){
    var line = document.createElement('div');
    line.className = 'line ' + cls;
    line.textContent = Array.prototype.map.call(args, function(v){
      if (typeof v === 'object') { try { return JSON.stringify(v); } catch(e) { return String(v); } }
      return String(v);
    }).join(' ');
    out.appendChild(line);
  }
  var orig = { log: console.log, warn: console.warn, error: console.error, info: console.info };
  console.log = function(){ push('log', arguments); orig.log.apply(console, arguments); };
  console.info = function(){ push('log', arguments); orig.info.apply(console, arguments); };
  console.warn = function(){ push('warn', arguments); orig.warn.apply(console, arguments); };
  console.error = function(){ push('error', arguments); orig.error.apply(console, arguments); };
  window.addEventListener('error', function(e){ push('error', ['运行异常: ' + (e.message || e.error)]); });
  try {
` + escaped + `
    push('done', ['执行完成']);
  } catch (e) {
    push('error', ['运行异常: ' + (e && e.stack ? e.stack : e)]);
  }
})();
</script>
</body>
</html>`
}
