package console

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// keyMsg 构造一个字符按键（bubbletea v1 的 KeyMsg 是 Key 的别名）
func keyMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// 覆盖：引擎屏在「宿主未接入自愈钩子」时也要给出明确结论，而不是一片空白。
//
// 该屏的存在意义是「思维链停摆时的现场急救台」：如果连"当前构建不支持重载"都不说，
// 用户只会看到一个没有按钮的页面，仍然只能去重启程序。
func TestEngineScreenWithoutHooks(t *testing.T) {
	SetEngineHooks(nil, nil)
	defer SetEngineHooks(nil, nil)

	m := newAppModel()
	m.width, m.height = 120, 34
	m.screen = screenEngine
	out := m.View()
	if !strings.Contains(out, "月华引擎") {
		t.Fatalf("引擎屏标题缺失:\n%s", out)
	}
	if !strings.Contains(out, "宿主未接入") {
		t.Fatalf("未接入钩子时应明确提示:\n%s", out)
	}
}

// 覆盖：注入钩子后，状态表把关键信息都摆出来（就绪 / 重载次数 / 连续失败 / 自动重载策略）。
func TestEngineScreenShowsStatus(t *testing.T) {
	booted := time.Now().Add(-12 * time.Minute)
	SetEngineHooks(func() EngineStatus {
		return EngineStatus{
			RuntimeReady:        true,
			AgentReady:          true,
			BootedAt:            booted,
			ReloadCount:         3,
			LastReloadAt:        time.Now().Add(-2 * time.Minute),
			LastReloadReason:    "连续 2 次应答失败：连接被拒绝",
			LastReloadResult:    "运行时就绪；模型服务可用；上下文：已恢复 6 条对话上下文、1 条未读消息",
			ConsecutiveFailures: 2,
			LastFailure:         "Could not establish connection",
			LastFailureAt:       time.Now().Add(-3 * time.Minute),
			AutoReload:          true,
			AutoReloadThreshold: 2,
		}
	}, func(string) error { return nil })
	defer SetEngineHooks(nil, nil)

	m := newAppModel()
	m.width, m.height = 120, 40
	m.screen = screenEngine
	out := m.View()
	for _, want := range []string{"运行时", "智能体脚本", "重载次数", "3 次", "连续失败", "自动重载", "开启", "R"} {
		if !strings.Contains(out, want) {
			t.Fatalf("状态表缺少 %q:\n%s", want, out)
		}
	}
}

// 覆盖：按 R 触发宿主重载，并把结果（成功/失败）回显到屏上。
func TestEngineScreenManualReload(t *testing.T) {
	var called []string
	SetEngineHooks(func() EngineStatus {
		return EngineStatus{RuntimeReady: true, AgentReady: true, AutoReload: true, AutoReloadThreshold: 2}
	}, func(reason string) error {
		called = append(called, reason)
		return nil
	})
	defer SetEngineHooks(nil, nil)

	m := newAppModel()
	m.width, m.height = 120, 34
	m.screen = screenEngine

	// 第一次按 R：进入"重载中"
	model, cmd := m.updateEngine(keyMsg("R"))
	m = model.(appModel)
	if !m.engine.busy {
		t.Fatal("按 R 后应标记为重载中")
	}
	if cmd == nil {
		t.Fatal("按 R 后应返回重载命令")
	}
	// 执行命令（此时才真正调用宿主钩子）
	done := cmd()
	if _, ok := done.(engineDoneMsg); !ok {
		t.Fatalf("命令应产出 engineDoneMsg，实得 %T", done)
	}
	if len(called) != 1 || called[0] == "" {
		t.Fatalf("宿主重载钩子应被调用一次，实得 %#v", called)
	}

	// 重载进行中再按 R：不应重复触发
	model, _ = m.updateEngine(keyMsg("R"))
	m = model.(appModel)
	if len(called) != 1 {
		t.Fatalf("重载中不应重复触发，实得 %d 次", len(called))
	}

	// 完成事件：清除 busy 并给出成功提示
	model, _ = m.updateEngine(engineDoneMsg{})
	m = model.(appModel)
	if m.engine.busy || m.engine.err || !strings.Contains(m.engine.msg, "重载完成") {
		t.Fatalf("完成事件处理错误: busy=%v err=%v msg=%q", m.engine.busy, m.engine.err, m.engine.msg)
	}
}

// 覆盖：重载失败时界面必须说清楚（否则用户会以为已经修好了）。
func TestEngineScreenReloadFailure(t *testing.T) {
	SetEngineHooks(func() EngineStatus { return EngineStatus{} }, func(string) error {
		return errors.New("创建运行时失败: boom")
	})
	defer SetEngineHooks(nil, nil)

	m := newAppModel()
	m.width, m.height = 120, 34
	m.screen = screenEngine
	model, _ := m.updateEngine(keyMsg("R"))
	m = model.(appModel)
	cmd := reloadEngineCmd("测试")
	done := cmd()
	model, _ = m.updateEngine(done)
	m = model.(appModel)
	if !m.engine.err || !strings.Contains(m.engine.msg, "boom") {
		t.Fatalf("失败应回显原因: err=%v msg=%q", m.engine.err, m.engine.msg)
	}
}
