package TuiSystem

import (
	"bufio"
	"log"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"LunarSubsystem/LoggerGeneral"
)

// 月华 JS 日志的模块/子标签（与 engine 绑定自定义 console 时写入的标签保持一致）
const (
	lunarModule = "LunarCore"
	lunarSubJS  = "JavaScript"
	// ltp9 沙箱日志的模块标签（engine/ltp9/variable.go ServiceName）
	ltp9Module = "StarLTP"
	// ltp9 沙箱 console 输出携带的标记（binder.go consolePrint）
	ltp9ConsoleMark = "[console."
	// 绘图模块的日志模块标签（multimodal_analysis/module）
	drawModule = "MultimodalAnalysis"
)

// Install 装配统一日志处理管线（幂等）：
//  1. 以管道接管 os.Stdout/os.Stderr，后台逐行扫描送入原始日志缓冲；
//  2. 注册 LoggerGeneral 日志钩子，按模块/级别路由到各日志视图缓冲。
//
// 必须在宿主程序启动最早期（任何子系统装配前）调用，确保启动期日志不逃逸到终端。
func Install() {
	pipelineOnce.Do(func() {
		// ==== stdout/stderr 管道接管 ====
		stdoutR, stdoutW, err := os.Pipe()
		if err == nil {
			// 渲染目标：继承来的 stdout 未必可用（被重定向 / NUL / 只写句柄），
			// 需要时改由控制台设备句柄承担（见 console.go）
			inheritedStdout := os.Stdout
			realStdout = resolveRenderTarget()
			realStderr = os.Stderr
			os.Stdout = stdoutW
			os.Stderr = stdoutW
			LoggerGeneral.SetOutput(stdoutW)
			// 标准库 log 也必须改道：它内部持有的是「init 时的 os.Stderr 文件对象」
			// （即真实控制台），只替换 os.Stderr 变量对它无效。漏掉这一步时，
			// httputil / net/http 等库用 log.Printf 打的错误会直接糊在 TUI 界面上
			// （实测：`httputil: ReverseProxy read error during body copy` 就是这么漏出来的）。
			log.SetOutput(stdoutW)
			intercepted = true

			// ==== 进程级标准流接管：拦住进程内原生库的直写 ====
			// 上面换掉的只是 Go 层输出，**进程级标准句柄与 CRT 的 fd 1/2 仍指向真实控制台**。
			// OmniVoice TTS 引擎以 DLL 形式在进程内加载（engine/omnivoice_tts），其
			// maskgit-tts.h 的逐步骤 fprintf(stderr, "[MaskGIT-Step] 25/32 demask=12 …")
			// 与 backend.h 的 ggml 日志桥 fputs(text, stderr) 就这样一帧帧打在 TUI 界面上
			// （实测故障：合成语音时整屏被进度行刷乱）。把标准句柄槽与 CRT 的 fd 1/2 一并接到
			// 本管道后，原生输出与 Go 输出同一口径：只进「原始日志」视图与 console.log。
			// 仅在渲染目标已是自取句柄时执行——否则接管会把界面正在用的句柄关掉（见 console.go）。
			if f, ok := realStdout.(*os.File); ok && f != inheritedStdout {
				if note := captureNativeStdio(stdoutW); note != "" {
					AppendConsoleLog("[TUI] 进程级标准流接管不完整（" + note + "），原生库诊断可能仍直写终端")
				}
			}

			go scanPipeLines(stdoutR)
		}

		// ==== LoggerGeneral 结构化日志钩子 ====
		LoggerGeneral.SetLogHook(routeLogRecord)

		// ==== 运行状态采样器 ====
		initSampler()
		go samplerLoop()
	})
}

// GetRealStdout 返回原始终端输出（TUI 渲染目标；未接管时为 os.Stdout）
func GetRealStdout() interface{ Write([]byte) (int, error) } {
	if realStdout != nil {
		return realStdout
	}
	return os.Stdout
}

// ScrubLogText 对外暴露的日志正文清洗（供导出等下游消费方复用同一份实现）。
//
// 与内部 scrubLogText 是同一函数：导出的 `.log` / 控制台落盘 / 界面渲染必须共享
// 同一套清洗口径，否则三处会各自漂移（历史缺陷：只有落盘做了 ansi.Strip，
// 缓冲与导出仍带转义码，导出文件里混着 `\u001b[…` 与裸 `\r`）。
func ScrubLogText(raw string) []string { return scrubLogText(raw) }

// scrubLogText 清洗日志正文：剥离 ANSI 转义序列、归位控制符、按行拆分。
//
// 必须做这一步的原因（实测）：日志正文里普遍带终端控制序列——
//   - `putchar('\r')` 原地刷新进度（模型加载、sd.cpp 采样、7z 进度）；
//   - 彩色工具输出（git / npm / 编译器）与 OSC 序列（终端标题、超链接）。
//
// 不清洗会有三处坏结果：
//  1. 导出文件里混入 `\u001b[…` 甚至裸 `\r`，用 grep / jq / 文本编辑器都难以阅读；
//  2. 界面渲染是「正文 + lipgloss 着色」，正文再带自己的颜色时颜色错乱；
//  3. `\r` 返回行首会让「按显示宽度折行」的宽度计算与实际占据列数不符，界面错位。
//
// 实现顺序有讲究（踩过坑）：
//   - `ansi.Strip` 实测**保留** `\r`（它只剥转义序列），所以 `\r` 必须由本函数自己消费；
//   - 清理其余控制符时**必须显式放行 `\r`**，否则"进度行"会被拼成一整行、丢掉分行信息；
//   - 先剥序列再按 `\r` 分行：顺序反了会把 `\x1b[2K`（清行）截断，留下残缺的 `[2K` 当正文。
//
// 返回切片：正文可能被 `\r` 拆成多行（进度行的历史片段本身就是有价值的日志）。
func scrubLogText(raw string) []string {
	if raw == "" {
		return nil
	}
	clean := ansi.Strip(raw)
	// 去掉无显示意义的控制符（\x00-\x08、\x0b、\x0c、\x0e-\x1f、\x7f）。
	// 注意：\n 与 \t 有意义需保留；\r 也要放行——它是下面分行的依据，
	// 若在此处删掉，"10%\r55%\r完成" 会变成一行 "10%55%完成"。
	clean = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r == '\r' || r >= 0x20 {
			return r
		}
		return -1
	}, clean)

	parts := strings.Split(clean, "\r")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		// \r 拆分处常残留空格（原地刷新时常先写空格覆盖旧内容）
		if p = strings.TrimRight(p, " \t"); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// scrubSingleLine 清洗单行日志正文（取清洗后的首行；无内容时返回空串）。
// 供「一条记录对应一行」的场景使用（结构化日志、直接追加写盘）。
func scrubSingleLine(raw string) string {
	if lines := scrubLogText(raw); len(lines) > 0 {
		return lines[0]
	}
	return ""
}

// scanPipeLines 逐行扫描管道内容并送入原始日志缓冲。
// 同时把每一行镜像到 {程序根}/local_data/logs/console.log：进程退出后仍有据可查
// （含 Go 运行时的 panic 栈——它们同样经 stderr 进入本管道）。
func scanPipeLines(r *os.File) {
	defer r.Close()
	consoleLog := openConsoleLog()
	defer consoleLog.Close()

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		// 正文清洗统一走 scrubLogText：落盘与入缓冲共用同一份结果，
		// 避免"文件里干净、界面/导出里带转义码"这种两套口径。
		lines := scrubLogText(scanner.Text())
		if len(lines) == 0 {
			continue
		}
		for _, line := range lines {
			consoleLog.WriteLine(line)
			pushLog(LogLine{
				Time:   time.Now(),
				Source: "stdout",
				Level:  "RAW",
				Text:   line,
			}, SourceRaw)
		}
	}
	if err := scanner.Err(); err != nil {
		consoleLog.WriteLine("stdout 管道扫描异常: " + err.Error())
		pushLog(LogLine{
			Time:   time.Now(),
			Source: "stdout",
			Level:  "ERROR",
			Text:   "stdout 管道扫描异常: " + err.Error(),
		}, SourceRaw)
	}
}

// routeLogRecord LoggerGeneral 钩子：按模块/级别分类路由
func routeLogRecord(level, module, sub, msg string) {
	// 结构化日志同样要清洗：消息体可能内嵌彩色工具输出或含 \r 的进度串
	// （例如把引擎原始输出整段塞进一条日志的场合）。
	text := scrubSingleLine(msg)
	if text == "" {
		return
	}
	line := LogLine{
		Time:   time.Now(),
		Source: "log",
		Level:  level,
		Module: module,
		Sub:    sub,
		Text:   text,
	}
	// 原始日志视图接收全量
	pushLog(line, SourceRaw)

	switch {
	// 月华 JS 层日志
	case module == lunarModule && sub == lunarSubJS:
		pushLog(line, SourceLunarJS)
	// ltp9 沙箱 JS 层日志（console 输出带 [console.*] 标记）
	case module == ltp9Module && strings.Contains(msg, ltp9ConsoleMark):
		pushLog(line, SourceLTP9JS)
	// 绘图（sd.cpp）模块编排日志：与引擎输出合并展示于绘图引擎视图
	case module == drawModule:
		pushLog(line, SourceDraw)
	}

	// Go 警告与错误（独立于上面分支，原始日志不受影响）
	if level == "WARN" || level == "ERROR" || level == "FATAL" {
		pushLog(line, SourceGoWarn)
	}
}

// pushLog 追加一条日志到指定来源缓冲（环形截断 + 唤醒 TUI）
func pushLog(line LogLine, source LogSource) {
	storeMu.Lock()
	store := &logStores[source]
	*store = append(*store, line)
	if len(*store) > logStoreCap {
		*store = (*store)[len(*store)-logStoreCap:]
	}
	storeMu.Unlock()
	wakeTUI()
}

// PushEngineLog 绘图引擎（sd.cpp）单行日志出口：由 multimodal 模块的引擎日志钩子调用。
// 同时并入原始日志与绘图引擎视图（与 PushTaskLog 同构）。
func PushEngineLog(stream, line string) {
	// 引擎输出带大量 \r 进度刷新与 ANSI 控制：清洗后按拆分出的行逐条入缓冲，
	// 否则界面里每个进度片段都会叠在「同一行」上互相覆盖。
	for _, text := range scrubLogText(line) {
		l := LogLine{
			Time:   time.Now(),
			Source: "engine",
			Level:  classifyEngineLevel(text),
			Module: "sd.cpp",
			Sub:    stream,
			Text:   text,
		}
		pushLog(l, SourceRaw)
		pushLog(l, SourceDraw)
	}
}

// classifyEngineLevel 依文本推断引擎日志级别（引擎输出无级别字段）
func classifyEngineLevel(line string) string {
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(line, "ERROR"), strings.Contains(lower, "error"), strings.Contains(lower, "fail"):
		return "ERROR"
	case strings.Contains(line, "WARN"), strings.Contains(lower, "warn"):
		return "WARN"
	default:
		return "INFO"
	}
}

// PushTaskLog 功能子包（代理/打包/端口释放/更新包）日志统一出口：
// 同时并入原始日志视图与功能任务回显缓冲。
func PushTaskLog(msg string) {
	for _, text := range scrubLogText(msg) {
		line := LogLine{
			Time:   time.Now(),
			Source: "task",
			Level:  classifyTaskLevel(text),
			Text:   text,
		}
		pushLog(line, SourceRaw)
		pushLog(line, SourceTask)
	}
}

// classifyTaskLevel 按文本标记推断任务日志级别（沿用 environment_repair 的 [ERROR]/[WARN] 前缀约定）
func classifyTaskLevel(msg string) string {
	switch {
	case strings.Contains(msg, "[ERROR]"), strings.Contains(msg, "✗"):
		return "ERROR"
	case strings.Contains(msg, "[WARN]"):
		return "WARN"
	default:
		return "INFO"
	}
}

// GetLogs 返回指定来源的日志缓冲副本（TUI 视图渲染数据源）
func GetLogs(source LogSource) []LogLine {
	storeMu.RLock()
	defer storeMu.RUnlock()
	out := make([]LogLine, len(logStores[source]))
	copy(out, logStores[source])
	return out
}

// ClearLogs 清空指定来源的日志缓冲
func ClearLogs(source LogSource) {
	storeMu.Lock()
	logStores[source] = nil
	storeMu.Unlock()
	wakeTUI()
}

// pushScrubbedLog 清洗后入缓冲：正文可能被 `\r` 拆成多行，则逐行入缓冲。
//
// 供 TUI 内部自产日志（终端尺寸变化、字形校准摘要）使用——这类正文同样应走
// 同一套清洗口径，避免"管线入口洗了、别的入口没洗"的两套标准。
func pushScrubbedLog(source LogSource, level, module, sub, text string) {
	for _, line := range scrubLogText(text) {
		pushLog(LogLine{
			Time:   time.Now(),
			Source: "stdout",
			Level:  level,
			Module: module,
			Sub:    sub,
			Text:   line,
		}, source)
	}
}

// wakeTUI 非阻塞唤醒 TUI 刷新（缓冲 1，重复通知合并）
func wakeTUI() {
	select {
	case notifyCh <- struct{}{}:
	default:
	}
}
