package TuiSystem

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

// TestConsoleHandleHandoff 守护「控制台句柄交接」这条链路：
//
//	consoleOut()（O_RDWR 打开 CONOUT$）→ 作为子进程的 Std* → 子进程能否认出「输出是终端」
//
// 这条链路断掉，更新后的自动重启就会得到一个「界面无法显示」的实例（历史故障）。
// 两个易错点被本测试钉住：
//  1. 只写（O_WRONLY）打开的 CONOUT$ 过不了 GetConsoleMode（Access is denied），
//     必须 O_RDWR——否则句柄看着像控制台，实际量不到尺寸也画不出界面；
//  2. 交接后子进程拿到的必须是同一个可用的控制台句柄。
func TestConsoleHandleHandoff(t *testing.T) {
	if out := os.Getenv("DSH_HANDOFF_CHILD_OUT"); out != "" {
		// 子进程：判定自己的 stdout 是否为终端，把结论写文件
		//（stdout 就是被测句柄，不能用来回传结果）
		res := fmt.Sprintf("isTerminal=%v", isConsoleFile(os.Stdout))
		_ = os.WriteFile(out, []byte(res), 0644)
		return
	}

	out := consoleOut()
	if out == nil {
		t.Skip("当前环境没有控制台（CONOUT$ 打开失败），跳过交接验证")
	}
	defer out.Close()

	if !isConsoleFile(out) {
		t.Fatalf("consoleOut() 返回的句柄不是可用控制台句柄（只写打开会得到 Access is denied）")
	}

	resFile := t.TempDir() + "\\handoff.txt"
	cmd := exec.Command(os.Args[0], "-test.run=TestConsoleHandleHandoff")
	cmd.Env = append(os.Environ(), "DSH_HANDOFF_CHILD_OUT="+resFile)
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		t.Fatalf("子进程执行失败: %v", err)
	}

	data, err := os.ReadFile(resFile)
	if err != nil {
		t.Fatalf("子进程未回传结果: %v", err)
	}
	if got := string(data); got != "isTerminal=true" {
		t.Fatalf("控制台句柄交接失败：子进程未把 stdout 认作终端（%s）——"+
			"更新后的自动重启会因此得到一个「界面无法显示」的实例", got)
	}
}

// TestResolveRenderTarget 渲染目标解析：可用时用继承句柄，不可用时自愈，
// 两者都不可用时回退原样（由终端守卫兜底报错）。
func TestResolveRenderTarget(t *testing.T) {
	target := resolveRenderTarget()
	if target == nil {
		t.Fatalf("渲染目标不应为 nil")
	}
	// 本测试进程无控制台时只能回退；有控制台时必须拿到可用句柄
	if f, ok := target.(*os.File); ok && f == os.Stdout && !isConsoleFile(os.Stdout) {
		if consoleOut() != nil {
			t.Fatalf("控制台可用时渲染目标却回退了不可用的 stdout（自愈未生效）")
		}
	}
}
