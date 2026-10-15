package channel

import "testing"

// TestClassify 通道判别：更新包与 LTPX 应用市场共用同一仓库，
// 判别错一次就会「拉更新包拉到工具包」，因此逐条钉住判定规则。
func TestClassify(t *testing.T) {
	cases := []struct {
		name   string
		assets []string
		body   string
		want   Kind
	}{
		{
			name:   "更新包附件（无标记，兼容隔离前的历史发布）",
			assets: []string{"lunar_updatepack_v2026.10.14-1530.7z.001"},
			body:   "星月智能更新包 v2026.10.14-1530\n\n资源目录: local_data/tools",
			want:   Update,
		},
		{
			name:   "更新包附件（多分卷）",
			assets: []string{"lunar_updatepack_v1.7z.001", "lunar_updatepack_v1.7z.002"},
			want:   Update,
		},
		{
			name:   "更新包标记（附件尚未上传完）",
			assets: nil,
			body:   "[UPDATE] 星月智能 · 资源更新包 v2026.10.14\n\n**这是什么**",
			want:   Update,
		},
		{
			name:   "市场附件",
			assets: []string{"ltpx_lunar.image-studio.ltpx"},
			body:   "[LTPX] 图像工坊\nID: lunar.image-studio",
			want:   Market,
		},
		{
			name:   "市场标记（大小写不敏感）",
			assets: nil,
			body:   "[ltpx] 图像工坊",
			want:   Market,
		},
		{
			name:   "两类附件同时存在时以更新包为准",
			assets: []string{"ltpx_x.ltpx", "lunar_updatepack_v1.7z.001"},
			want:   Update,
		},
		{
			name:   "Gitee 自动源码归档不构成通道证据",
			assets: []string{"v2026.10.14.zip", "v2026.10.14.tar.gz"},
			body:   "随手写的发布",
			want:   Unknown,
		},
		{
			name: "空发布",
			want: Unknown,
		},
	}
	for _, c := range cases {
		if got := Classify(c.assets, c.body); got != c.want {
			t.Fatalf("%s: Classify = %v，期望 %v", c.name, got, c.want)
		}
	}
}

// TestKindString 通道名（错误信息可读性）
func TestKindString(t *testing.T) {
	for kind, want := range map[Kind]string{
		Update:  "更新包",
		Market:  "LTPX 应用市场",
		Unknown: "未识别",
	} {
		if got := kind.String(); got != want {
			t.Fatalf("Kind(%d).String() = %q，期望 %q", kind, got, want)
		}
	}
}

// TestAssetPredicates 附件名前缀判定
func TestAssetPredicates(t *testing.T) {
	if !IsUpdateAsset("lunar_updatepack_v1.7z.001") {
		t.Fatalf("更新包附件应被识别")
	}
	if IsUpdateAsset("ltpx_x.ltpx") {
		t.Fatalf("市场附件不应被认作更新包附件")
	}
	if !IsMarketAsset("ltpx_lunar.memory-manager.ltpx") {
		t.Fatalf("市场附件应被识别")
	}
	if IsMarketAsset("lunar_updatepack_v1.7z.001") {
		t.Fatalf("更新包附件不应被认作市场附件")
	}
}

// TestAssetNames 附件名映射（调用方各自的响应结构 → 纯字符串列表）
func TestAssetNames(t *testing.T) {
	type asset struct{ N string }
	names := AssetNames([]asset{{N: "a"}, {N: "b"}}, func(a asset) string { return a.N })
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Fatalf("AssetNames = %v", names)
	}
	if got := AssetNames(nil, func(a asset) string { return a.N }); len(got) != 0 {
		t.Fatalf("空附件列表应返回空切片，实际 %v", got)
	}
}
