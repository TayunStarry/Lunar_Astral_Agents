package StarLTP

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// ==== 脚本权限推断 ====

// TestRequiredPermissions 覆盖推断规则的正反例：
// 只认「调用根对象」形态；注释与同行闭合字符串里的同名文本不算引用；双权限 API 追加 allow-network。
func TestRequiredPermissions(t *testing.T) {
	cases := []struct {
		name   string
		script string
		want   []string
	}{
		{
			name:   "根对象调用按权限全集顺序输出",
			script: `signal.all({ok:1}); const r = file.read("a.json"); hash.sha256(r.text); agent.chat([], {});`,
			want:   []string{"allow-file", "allow-agent", "allow-signal", "allow-certificate"},
		},
		{
			name:   "仅常驻能力（config/console/exportFunction/sleep/event）无需任何权限",
			script: `const c = config.read(); console.log("hi"); exportFunction("f", x => x); event.subscribe("t", () => {}); sleep(10);`,
			want:   nil,
		},
		{
			name:   "注释与字符串里的同名文本不算引用",
			script: "// 网络走 http.get/post，图片 image.download\nconst tip = \"使用 agent.chat 对话\";\nconsole.log('调用 file.read 失败');\n",
			want:   nil,
		},
		{
			name:   "fetch / WebSocket / callFunction 形态",
			script: `const a = fetch("https://x/y"); const ws = WebSocket("ws://z"); const v = callFunction("pkg", "fn", []);`,
			want:   []string{"allow-network", "allow-call"},
		},
		{
			name:   "双权限 API：image.download 追加 allow-network",
			script: `image.download("https://x/y.png", "y.png");`,
			want:   []string{"allow-file", "allow-network"},
		},
		{
			name:   "双权限 API：agent.search 追加 allow-network",
			script: `const r = agent.search("今天的新闻");`,
			want:   []string{"allow-network", "allow-agent"},
		},
		{
			name:   "变量成员访问不误判（x.file / $file / mySignal）",
			script: `const x = {}; x.file.save(); const mySignal = 1; mySignal.toFixed(); const y = $file;`,
			want:   nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RequiredPermissions([]byte(tc.script))
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("推断权限不符\n期望: %v\n实得: %v", tc.want, got)
			}
		})
	}
}

// ==== 签发与现状读取 ====

// TestSignPermissionRoundTrip 验证「签发 → 现状读取」闭环，以及代码改动后密钥失效的判定。
func TestSignPermissionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	script := `const r = file.read("daily.json"); signal.all(r); http.post("https://x", "{}");`
	writeFile(t, filepath.Join(dir, DefaultMain), script)

	before, err := InspectPackageAuth(dir)
	if err != nil {
		t.Fatalf("读取现状失败: %v", err)
	}
	if !before.HasScript || before.HasKey || before.Signed {
		t.Fatalf("未签发状态不符: %+v", before)
	}
	if want := []string{"allow-file", "allow-network", "allow-signal"}; !reflect.DeepEqual(before.Required, want) {
		t.Fatalf("推断权限不符\n期望: %v\n实得: %v", want, before.Required)
	}

	after, err := SignPackageAuth(dir)
	if err != nil {
		t.Fatalf("签发失败: %v", err)
	}
	if !after.HasKey || !after.Signed {
		t.Fatalf("签发后状态不符: %+v", after)
	}
	if !reflect.DeepEqual(after.Granted, after.Required) {
		t.Fatalf("签发权限与推断不一致\n推断: %v\n签发: %v", after.Required, after.Granted)
	}
	if _, err := os.Stat(filepath.Join(dir, KeyFileName)); err != nil {
		t.Fatalf("permissions.key 未落盘: %v", err)
	}

	// 代码一改，旧密钥必须判为「对不上」（Signed=false），与运行时校验语义一致
	writeFile(t, filepath.Join(dir, DefaultMain), script+"\nconsole.log('changed');")
	stale, err := InspectPackageAuth(dir)
	if err != nil {
		t.Fatalf("复读现状失败: %v", err)
	}
	if !stale.HasKey || stale.Signed {
		t.Fatalf("代码变更后应判定为需重签: %+v", stale)
	}
}

// TestSignWithoutPermissions 只使用常驻能力的脚本不做任何写入。
func TestSignWithoutPermissions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, DefaultMain), `config.read(); console.log("only resident");`)

	auth, err := SignPackageAuth(dir)
	if err != nil {
		t.Fatalf("签发（无需权限）不应报错: %v", err)
	}
	if auth.HasKey || auth.Signed || len(auth.Required) != 0 {
		t.Fatalf("无需签发时不应写入密钥: %+v", auth)
	}
	if _, err := os.Stat(filepath.Join(dir, KeyFileName)); !os.IsNotExist(err) {
		t.Fatalf("permissions.key 不应存在（err=%v）", err)
	}
}

// TestShippedPackages 检查随仓库分发的 LTP9 包：必须能读出脚本与引用权限；
// 已带 permissions.key 的包必须与当前脚本对应（否则运行时会被判为无权限）。
func TestShippedPackages(t *testing.T) {
	root := repoRoot(t)
	for _, dirName := range []string{"com.ltp9-flash.painting", "com.ltp9-flash.weather"} {
		root := filepath.Join(root, "local_data", "package", dirName)
		if _, err := os.Stat(root); err != nil {
			t.Skipf("包目录不存在（%s），跳过", root)
		}
		auth, err := InspectPackageAuth(root)
		if err != nil {
			t.Fatalf("%s 读取现状失败: %v", dirName, err)
		}
		if !auth.HasScript {
			t.Fatalf("%s 缺少 %s", dirName, DefaultMain)
		}
		if len(auth.Required) == 0 {
			t.Fatalf("%s 未推断出任何权限引用（脚本形态可能已变）", dirName)
		}
		t.Logf("%s: 哈希=%s 已签发=%v 现有=%v 引用=%v", dirName, auth.Hash[:8], auth.Signed, auth.Granted, auth.Required)
		if auth.HasKey && !auth.Signed {
			t.Fatalf("%s 的 permissions.key 与当前 %s 不对应（运行时将拒绝全部权限）", dirName, DefaultMain)
		}
	}
}

// ==== 测试辅助 ====

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("写入 %s 失败: %v", path, err)
	}
}

// repoRoot 从当前工作目录向上找到含 local_data/package 的仓库根
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Skipf("取工作目录失败: %v", err)
	}
	for {
		marker := filepath.Join(dir, "local_data", "package")
		if info, err := os.Stat(marker); err == nil && info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		// 到根（Windows 盘符根 'D:\' 的 Dir 仍是自身）即停
		if parent == dir {
			t.Skip("未找到仓库根（local_data/package），跳过")
		}
		dir = parent
	}
}
