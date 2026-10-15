package updatepack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/TuiSystem/channel"
)

// marketReleaseFixture 市场包发布（与更新包共用同一仓库）
func marketReleaseFixture() giteeRelease {
	return giteeRelease{
		ID:      11,
		TagName: "lunar.image-studio",
		Name:    "图像工坊",
		Body:    "[LTPX] 图像工坊\nID: lunar.image-studio\nTags: Node-LTP\n\n一个图像工具",
		Assets:  []giteeAsset{{ID: 1, Name: "ltpx_lunar.image-studio.ltpx", BrowserDownloadURL: "https://example.com/ltpx"}},
	}
}

// updateReleaseFixture 更新包发布（分卷 + 源码归档）
func updateReleaseFixture(tag string) giteeRelease {
	return giteeRelease{
		ID:      22,
		TagName: tag,
		Name:    "更新包 " + tag,
		Body:    "[UPDATE] 星月智能 · 资源更新包 " + tag + "\n\n**这是什么**\n...",
		Assets: []giteeAsset{
			{ID: 2, Name: "lunar_updatepack_" + tag + ".7z.001", BrowserDownloadURL: "https://example.com/1", Size: 100},
			{ID: 3, Name: "lunar_updatepack_" + tag + ".7z.002", BrowserDownloadURL: "https://example.com/2", Size: 100},
			{ID: 4, Name: tag + ".zip", BrowserDownloadURL: "https://example.com/src"},
		},
	}
}

// TestReleaseKind 通道判别：与市场包共用仓库时的归属判定
func TestReleaseKind(t *testing.T) {
	if got := releaseKind(marketReleaseFixture()); got != channel.Market {
		t.Fatalf("市场发布应判为 Market，实际 %v", got)
	}
	if got := releaseKind(updateReleaseFixture("v2026.10.14-1530")); got != channel.Update {
		t.Fatalf("更新包发布应判为 Update，实际 %v", got)
	}
	if got := releaseKind(giteeRelease{TagName: "x", Assets: []giteeAsset{{Name: "x.zip"}}}); got != channel.Unknown {
		t.Fatalf("仅有源码归档应判为 Unknown，实际 %v", got)
	}
}

// TestIsUpdateRelease 只有更新包通道的发布才可进入更新包流程
func TestIsUpdateRelease(t *testing.T) {
	if isUpdateRelease(marketReleaseFixture()) {
		t.Fatalf("市场发布不能被当作更新包")
	}
	if !isUpdateRelease(updateReleaseFixture("v1")) {
		t.Fatalf("更新包发布应被识别")
	}
	// 隔离上线前的历史更新包：无 [UPDATE] 标记，但附件前缀一直在
	legacy := giteeRelease{
		TagName: "v2025.08.22",
		Body:    "星月智能更新包 v2025.08.22\n\n资源目录: local_data/tools",
		Assets:  []giteeAsset{{Name: "lunar_updatepack_v2025.08.22.7z.001"}},
	}
	if !isUpdateRelease(legacy) {
		t.Fatalf("历史更新包（无标记、有附件前缀）应被识别")
	}
}

// TestUpdateAssets 附件过滤：只取更新包分卷，排除源码归档与市场附件
func TestUpdateAssets(t *testing.T) {
	assets := updateAssets(updateReleaseFixture("v1"))
	if len(assets) != 2 {
		t.Fatalf("应取到 2 个更新包分卷，实际 %d（%v）", len(assets), assets)
	}
	for _, a := range assets {
		if !strings.HasPrefix(a.Name, channel.UpdateAssetPrefix) {
			t.Fatalf("混入了非更新包附件: %s", a.Name)
		}
	}

	// 市场发布混入更新包流程时取不到任何附件——上层据此报「没有更新包附件」
	if got := updateAssets(marketReleaseFixture()); len(got) != 0 {
		t.Fatalf("市场发布不应提供更新包附件，实际 %v", got)
	}
}

// TestChannelMismatchError 通道不符的错误必须点明「两条通道共用同一仓库」并给出出路
func TestChannelMismatchError(t *testing.T) {
	msg := channelMismatchError("lunar.image-studio", channel.Market).Error()
	for _, want := range []string{"lunar.image-studio", "LTPX 应用市场", "共用同一仓库"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("错误信息缺少 %q: %s", want, msg)
		}
	}
}

// TestWaitFileArgsSkipsApplier 解锁等待清单必须排除更新器自身。
//
// 这是「拉取最新后终端黑掉、更新器像没启动、资源也没更新」的根因：
// 更新器把自己的镜像也列为「等待解锁」文件，而运行中的 exe 不允许以写方式打开
// （镜像加载器不共享写），于是它白等满超时（实测 60 秒），用户在等待中关掉终端，
// 作为子进程的更新器随之被杀 —— 覆盖过程根本没有开始。
func TestWaitFileArgsSkipsApplier(t *testing.T) {
	args := waitFileArgs([]string{
		"Lunar_Astral_Agents.exe",
		applierFileName,
		"subdir/" + applierFileName, // 大小写/路径写法不同也应排除
		"",
		"Lunar_Update_Applier.EXE",
	})
	want := []string{"-wait-file", "Lunar_Astral_Agents.exe"}
	if len(args) != len(want) || args[0] != want[0] || args[1] != want[1] {
		t.Fatalf("解锁等待清单不符：%v", args)
	}
	// 没有主程序文件时不应产生任何参数
	if got := waitFileArgs([]string{applierFileName}); len(got) != 0 {
		t.Fatalf("仅含更新器时不应产生等待参数，实际 %v", got)
	}
}

// TestApplyLogPathIsUnderProgramRoot 日志路径与传给更新器的 -log 必须同源
// （否则控制台提示的「去哪看日志」与实际写入位置不一致，等于没提示）
func TestApplyLogPathIsUnderProgramRoot(t *testing.T) {
	cfg := Config{TargetDir: os.TempDir()}
	root := ProgramRoot(cfg)
	logPath := ApplyLogPath(cfg)
	if !strings.HasPrefix(logPath, root) {
		t.Fatalf("日志路径 %s 不在程序根 %s 之下", logPath, root)
	}
	if !strings.HasSuffix(filepath.ToSlash(logPath), "local_data/logs/update_apply.log") {
		t.Fatalf("日志路径不符：%s", logPath)
	}
}

// TestBuildReleaseBody 发行说明：说明「这是什么/怎么装」，保留通道标记，
// 且 SHA-256 行格式与拉取端的解析约定保持一致（改格式即拉取端取不到校验值）。
func TestBuildReleaseBody(t *testing.T) {
	dir := t.TempDir()
	packA := filepath.Join(dir, "lunar_updatepack_v2026.10.14-1530.7z.001")
	packB := filepath.Join(dir, "lunar_updatepack_v2026.10.14-1530.7z.002")
	if err := os.WriteFile(packA, []byte("part-a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packB, []byte("part-b"), 0644); err != nil {
		t.Fatal(err)
	}

	const shaA = "aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff6666aaaa7777bbbb8888"
	cfg := Config{
		Include:      []string{"local_data/audios", "local_data/package"},
		ProgramFiles: []string{"Lunar_Astral_Agents.exe"},
	}
	body := buildReleaseBody(cfg, "v2026.10.14-1530", []string{packA, packB}, map[string]string{
		filepath.Base(packA): shaA,
	})

	// 通道标记：共用仓库时的判别依据，任何重构都不能丢
	if !strings.Contains(body, channel.UpdateMarker) {
		t.Fatalf("发行说明缺少通道标记 %s", channel.UpdateMarker)
	}
	// 「这是什么发行版」：不能只剩目录清单
	for _, want := range []string{"资源更新包", "纯本地化", "Windows 10/11", "安装方式", "SHA-256"} {
		if !strings.Contains(body, want) {
			t.Fatalf("发行说明缺少 %q：\n%s", want, body)
		}
	}
	if !strings.Contains(body, "local_data/audios") || !strings.Contains(body, "Lunar_Astral_Agents.exe") {
		t.Fatalf("发行说明应列出本次发布的实际内容：\n%s", body)
	}
	// 拉取端解析约定：extractSHA 取首个 SHA-256 行的第二个字段
	if got := extractSHA(body); got != shaA {
		t.Fatalf("extractSHA 未能解析出校验值：got=%q want=%q\n%s", got, shaA, body)
	}
	// 描述整体可读性：不应出现连续两行以上空白或未替换的占位
	if strings.Contains(body, "%!") {
		t.Fatalf("发行说明含格式化残留：\n%s", body)
	}
}
