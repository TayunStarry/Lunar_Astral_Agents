//go:build windows

package desktop

// 程序启动关键字的别名展开：中文口语名必须先转换成真实可执行名，
// 否则开始菜单与 PATH 都搜不到（实测「记事本」导致启动失败）。

import (
	"os/exec"
	"strings"
	"testing"
)

func TestProgramAliasesExpandBuiltin(t *testing.T) {
	keys := dtProgramAliases("记事本")
	if len(keys) != 2 || keys[1] != "notepad" {
		t.Fatalf("「记事本」应展开为 notepad，实得 %#v", keys)
	}
	candidates := dtExecutableCandidates(keys)
	found := false
	for _, c := range candidates {
		if c == "notepad.exe" {
			found = true
		}
	}
	if !found {
		t.Fatalf("可执行候选应包含 notepad.exe，实得 %#v", candidates)
	}
}

func TestProgramAliasesKeepUnknownKey(t *testing.T) {
	keys := dtProgramAliases("chrome")
	if len(keys) != 1 || keys[0] != "chrome" {
		t.Fatalf("未知程序应保留原始关键字，实得 %#v", keys)
	}
}

// 覆盖：PATH 搜索能命中系统自带记事本（正是本次故障的直接原因）。
func TestLookPathFindsNotepad(t *testing.T) {
	p, err := exec.LookPath("notepad.exe")
	if err != nil {
		t.Skip("当前环境 PATH 中无 notepad.exe（非完整 Windows 环境）")
	}
	if !strings.HasSuffix(strings.ToLower(p), "notepad.exe") {
		t.Fatalf("notepad 路径异常: %s", p)
	}
}
