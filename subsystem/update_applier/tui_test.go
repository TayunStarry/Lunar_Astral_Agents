package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TestTUIViewsFitTerminal 五屏都必须满足「高度恒定、行宽不超过终端」——
// 这条不变式一旦破坏，界面就会滚动叠画（与主控制台同一套约束）。
func TestTUIViewsFitTerminal(t *testing.T) {
	root := t.TempDir()
	cfg := &ReleaseConfig{GiteeOwner: "owner", GiteeRepo: "repo", ProgramFiles: []string{"app.exe"}}
	m := newUIModel(root, cfg)
	m.width, m.height = 96, 28
	m.curVersion = "v2026.10.18-0847"
	m.releases = []UpdateRelease{
		{Tag: "v2026.11.12-0900", Name: "更新包", Valid: true, Volumes: []string{"a.7z.001"}, CreatedAt: "2026-11-12T09:00:00Z"},
		{Tag: "v2026.10.01-1200", Name: "旧包", Valid: false, Problem: "缺少 SHA-256"},
	}
	m.install = uiInstallState{Stage: stageBackup, Percent: 42, FilesDone: 3, FilesTotal: 10, VerifyResult: "10 个文件校验通过"}
	m.publish = uiPublishState{Version: "v2026.11.12-0900", Stage: "pack", Percent: 30, Detail: "压缩中", Busy: true}
	m.appendLog("测试日志")

	for screen := 0; screen < screenCount; screen++ {
		m.screen = screen
		m.cursor = screen
		out := m.View()
		lines := strings.Split(out, "\n")
		if len(lines) != m.height {
			t.Fatalf("屏 %d 行数 = %d，期望 %d", screen, len(lines), m.height)
		}
		for i, line := range lines {
			if w := dispWidth(line); w > m.width-1 {
				t.Fatalf("屏 %d 第 %d 行宽 = %d，超过 %d", screen, i, w, m.width-1)
			}
		}
	}
}

// TestTUIExitAndConfirmKeys 退出键与降级/重装确认
func TestTUIExitAndConfirmKeys(t *testing.T) {
	m := newUIModel(t.TempDir(), &ReleaseConfig{})
	m.width, m.height = 96, 28
	m.curVersion = "v2026.11.12-0900"
	m.releases = []UpdateRelease{{Tag: "v2026.01.01-0000", Valid: true, Volumes: []string{"a.7z.001"}}}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatal("x 应触发退出命令")
	}
}

// TestTUIInstallConfirm 进入安装屏后，降级版本需要 Enter 二次确认
func TestTUIInstallConfirm(t *testing.T) {
	m := newUIModel(t.TempDir(), &ReleaseConfig{})
	m.width, m.height = 96, 28
	m.screen = screenInstall
	m.curVersion = "v2026.11.12-0900"
	m.releases = []UpdateRelease{{Tag: "v2026.01.01-0000", Valid: true, Volumes: []string{"a.7z.001"}}}

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := model.(*uiModel); !got.install.Pending {
		t.Fatal("降级安装应先进入二次确认")
	}
}

// TestStartupFirstRun 首次运行：无 state.json / journal / version.json 时
// 恢复流程不报错，界面给出「未知版本 / 无记录」而不是空白。
func TestStartupFirstRun(t *testing.T) {
	root := t.TempDir()
	if recovered, err := RecoverOnStart(root); err != nil || recovered {
		t.Fatalf("空目录启动恢复应为 (false,nil)，实际 (%v,%v)", recovered, err)
	}
	_, cfgErr := loadReleaseConfig(root)

	m := newUIModel(root, nil)
	m.relErr = cfgErr
	m.width, m.height = 100, 32
	out := m.View()
	for _, want := range []string{"当前版本", "未知", "无记录", "未找到 release.json"} {
		if !strings.Contains(out, want) {
			t.Fatalf("首次运行总览缺少 %q:\n%s", want, out)
		}
	}
}

// TestStartupMissingMainProgram 目标目录没有主程序时，总览要明确提示找不到文件。
func TestStartupMissingMainProgram(t *testing.T) {
	root := t.TempDir()
	cfg := &ReleaseConfig{ProgramFiles: []string{"Lunar_Astral_Agents.exe"}}
	m := newUIModel(root, cfg)
	m.width, m.height = 100, 32
	out := m.View()
	if !strings.Contains(out, "未找到 Lunar_Astral_Agents.exe") {
		t.Fatalf("缺少主程序时应提示文件名:\n%s", out)
	}
}

// TestStartupMissingConfig 缺少 release.json 时，错误信息必须指向具体路径。
func TestStartupMissingConfig(t *testing.T) {
	_, err := loadReleaseConfig(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "release.json") {
		t.Fatalf("缺少配置时应返回可读错误，实际 %v", err)
	}
}

// TestTUIKeysMatchConsole 键位与主控制台一致：PgUp/PgDn 与 [ ] 切屏、→ 进入、Esc 回总览
func TestTUIKeysMatchConsole(t *testing.T) {
	m := newUIModel(t.TempDir(), &ReleaseConfig{})
	m.width, m.height = 100, 30

	m.screen, m.cursor = screenOverview, screenOverview
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.screen != screenReleases {
		t.Fatalf("PgDn 应切到下一屏，实际 %d", m.screen)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	if m.screen != screenOverview {
		t.Fatalf("PgUp 应切到上一屏，实际 %d", m.screen)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if m.screen != screenReleases {
		t.Fatalf("] 应切到下一屏，实际 %d", m.screen)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.screen != screenOverview {
		t.Fatalf("Esc 应返回总览，实际 %d", m.screen)
	}

	m.cursor = screenPublish
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.screen != screenPublish {
		t.Fatalf("→ 应进入游标屏，实际 %d", m.screen)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if !m.showHelp {
		t.Fatal("? 应打开帮助")
	}
	out := m.View()
	if !strings.Contains(out, "全局键位") || !strings.Contains(out, "当前屏") {
		t.Fatalf("帮助页应按「全局键位 / 当前屏」组织:\n%s", out)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.showHelp {
		t.Fatal("Esc 应关闭帮助")
	}
}

// TestTUIHelpShowsSingleX 底栏与帮助页只显示单个 X，x 与 X 都能退出
func TestTUIHelpShowsSingleX(t *testing.T) {
	m := newUIModel(t.TempDir(), &ReleaseConfig{})
	m.width, m.height = 120, 34
	m.screen, m.cursor = screenOverview, screenOverview

	out := m.View()
	if !strings.Contains(out, "X 退出") {
		t.Fatalf("底栏应提示 X 退出:\n%s", out)
	}
	if strings.Contains(out, "x / X") || strings.Contains(out, "x/X") {
		t.Fatalf("底栏不应出现双写法:\n%s", out)
	}

	m.showHelp = true
	help := m.View()
	if !strings.Contains(help, "退出程序") {
		t.Fatalf("帮助页应列出退出程序的键位:\n%s", help)
	}
	if strings.Contains(help, "x / X") || strings.Contains(help, "x/X") {
		t.Fatalf("帮助页不应出现双写法:\n%s", help)
	}
	m.showHelp = false

	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}); cmd == nil {
		t.Fatal("x 应触发退出")
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}}); cmd == nil {
		t.Fatal("X 应触发退出")
	}
}

// TestRunningProgramProcessesSelf 存活判定按 exe 路径 + PID 复核，能认出同路径的子进程
func TestRunningProgramProcessesSelf(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	cmd := exec.Command(exe, "-test.run=TestHelperProcess", "--")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	if err := cmd.Start(); err != nil {
		t.Skip(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		procs, err := runningProgramProcesses([]string{exe})
		if err != nil {
			t.Fatalf("进程枚举失败：%v", err)
		}
		for _, p := range procs {
			if p.PID == cmd.Process.Pid {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("未枚举到子进程 %d（%s）", cmd.Process.Pid, exe)
}

// TestHelperProcess 供进程枚举测试派生的睡眠子进程
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	time.Sleep(30 * time.Second)
}

// TestMainProgramTargetsSkipSelf 更新器自身不能成为「主程序」目标
func TestMainProgramTargetsSkipSelf(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	targets, missing := mainProgramTargets(filepath.Dir(exe), &ReleaseConfig{ProgramFiles: []string{filepath.Base(exe)}})
	if len(targets) != 0 || len(missing) != 0 {
		t.Fatalf("更新器自身应被跳过，实际 targets=%v missing=%v", targets, missing)
	}
}

// TestProgramStateSkipUpdaterSelf 回归：程序文件清单含更新器自身时，
// 不能因为「更新器自己正在运行」而显示主程序运行中。
func TestProgramStateSkipUpdaterSelf(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	m := newUIModel(filepath.Dir(exe), &ReleaseConfig{ProgramFiles: []string{filepath.Base(exe)}})
	if got := m.programStateText(); strings.Contains(got, "运行中") {
		t.Fatalf("更新器自身运行不应被判为主程序运行中：%q", got)
	}
}

// TestProgramStateNotRunning 主程序文件存在但没有进程时，明确显示未运行
func TestProgramStateNotRunning(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Lunar_Astral_Agents.exe"), []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newUIModel(root, &ReleaseConfig{ProgramFiles: []string{"Lunar_Astral_Agents.exe"}})
	if got := m.programStateText(); !strings.Contains(got, "未运行") {
		t.Fatalf("没有进程时应显示未运行，实际 %q", got)
	}
}

// TestWaitProgramsExitNotRunning 主程序未运行时，安装等待段必须立即通过而不是空等
func TestWaitProgramsExitNotRunning(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Lunar_Astral_Agents.exe"), []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := waitProgramsExit(root, &ReleaseConfig{ProgramFiles: []string{"Lunar_Astral_Agents.exe"}}, 3*time.Second); err != nil {
		t.Fatalf("未运行时应直接通过，实际 %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("未运行却等待了 %s", elapsed)
	}
}
