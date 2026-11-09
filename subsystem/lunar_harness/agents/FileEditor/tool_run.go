package FileEditor

// tool_run.go 运行文档库中的 js / html 项目文件。
//
// 统一分流：.js / .html / .htm 一律打开「资源库」前端，并在其 iframe 沙箱中运行；
// 资源库窗口会自动弹出并作为浏览器端代码的主页面。
// 宿主侧不再内置 js 运行时，脚本不再由进程内的脚本引擎执行。

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/browser"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// RunProject 运行指定的 js / html 项目文件。
func RunProject(ctx context.Context, path string) OpResult {
	return runProject(ctx, path)
}

// RunProjectWithLimit 运行项目文件。
//
// timeoutSec 为历史参数（早期宿主侧脚本运行时的时长上限），
// 现在脚本统一在浏览器沙箱执行，该参数不再生效，保留签名以兼容既有调用方。
func RunProjectWithLimit(ctx context.Context, path string, timeoutSec int) OpResult {
	_ = timeoutSec
	return runProject(ctx, path)
}

// runProject 运行入口：校验文件并分流执行。
func runProject(ctx context.Context, path string) OpResult {
	_ = ctx
	if _, err := readDocument(path); err != nil {
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

	outcome := runProjectFile(path, ext)
	if localRel != "" {
		// 运行结束再上报：成功保持绿色，报错/警告转为红色（供资源库文件名着色）。
		publishRun(localRel, !outcome.OK)
	}
	return outcome
}

// runProjectFile 按扩展名分流执行。
func runProjectFile(path, ext string) OpResult {
	if ext == ".html" || ext == ".htm" {
		return openInBrowser(path, "html")
	}
	return runJSInBrowser(path)
}

// RunProjectPath 运行指定项目文件（供创作管线等进程内调用方使用）。
func RunProjectPath(path string) OpResult {
	return RunProjectPathWithLimit(path, 0)
}

// RunProjectPathWithLimit 运行项目文件。
func RunProjectPathWithLimit(path string, timeoutSec int) OpResult {
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	return RunProjectWithLimit(ctx, path, timeoutSec)
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
