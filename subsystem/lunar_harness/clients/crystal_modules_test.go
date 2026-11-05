package clients

import (
	"bytes"
	"testing"
	"testing/fstest"
)

// 内置模块清单是 /api/packages 与 AtoA 工具链扫描的唯一来源：
// 目录枚举顺序与「缺少 metadata.json 的目录」处理必须在测试里钉住，
// 否则两处调用虽同源，仍会因清单内容不同而让「桌面看得到、工具链调不到」。
func TestCrystalModulesSortedAndSkipMissingMetadata(t *testing.T) {
	fake := fstest.MapFS{
		// 故意乱序声明：返回顺序必须由目录名升序决定
		"crystal/zeta/metadata.json":  &fstest.MapFile{Data: []byte(`{"id":"zeta"}`)},
		"crystal/alpha/metadata.json": &fstest.MapFile{Data: []byte(`{"id":"alpha"}`)},
		"crystal/mid/metadata.json":   &fstest.MapFile{Data: []byte(`{"id":"mid"}`)},
		// 纯资源目录：有目录、无 metadata.json，必须跳过
		"crystal/assets/logo.svg": &fstest.MapFile{Data: []byte("<svg/>")},
		// 非目录条目必须跳过
		"crystal/README.md": &fstest.MapFile{Data: []byte("# readme")},
	}

	mods := crystalModules(fake)
	if len(mods) != 3 {
		t.Fatalf("内置模块数应为 3（跳过无 metadata 目录与非目录条目），实得 %d: %v", len(mods), namesOf(mods))
	}
	want := []string{"alpha", "mid", "zeta"}
	for i, name := range want {
		if mods[i].Name != name {
			t.Fatalf("第 %d 个模块应为 %q，实得 %q（顺序：%v）", i, name, mods[i].Name, namesOf(mods))
		}
	}
	// metadata 原文必须原样透传，解析交给调用方
	if got := mods[0].Metadata; !bytes.Equal(got, []byte(`{"id":"alpha"}`)) {
		t.Fatalf("alpha 的 metadata 原文被改动: %q", got)
	}
}

// 缺少 crystal 子树时返回 nil 而非 panic：clients 的 go:embed 若调整了目录约定，
// 调用方仍应得到空清单，而不是启动期崩溃或空指针。
func TestCrystalModulesMissingTree(t *testing.T) {
	if mods := crystalModules(fstest.MapFS{}); mods != nil {
		t.Fatalf("缺少 crystal 子树时应返回 nil，实得 %v", namesOf(mods))
	}
}

// 空 crystal 子树：返回空清单。
func TestCrystalModulesEmptyTree(t *testing.T) {
	fake := fstest.MapFS{"crystal/index.html": &fstest.MapFile{Data: []byte("<html/>")}}
	if mods := crystalModules(fake); len(mods) != 0 {
		t.Fatalf("空 crystal 子树应返回空清单，实得 %v", namesOf(mods))
	}
}

// namesOf 提取模块目录名，供失败信息展示。
func namesOf(mods []CrystalModule) []string {
	out := make([]string, 0, len(mods))
	for _, m := range mods {
		out = append(out, m.Name)
	}
	return out
}
