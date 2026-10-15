package channel

import "testing"

// TestCompareTagNatural 标签自然序：数字段必须按数值比，而不是字典序。
// 这直接对应线上故障：v2026.10.15-2232 被当成比 v2026.10.15-2250 新。
func TestCompareTagNatural(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v2026.10.15-2232", "v2026.10.15-2250", -1}, // 2232 更旧
		{"v2026.10.15-2250", "v2026.10.15-2232", 1},
		{"v2026.10.15-930", "v2026.10.15-1030", -1}, // 字典序会判反的一例
		{"v2026.10.15-1030", "v2026.10.15-930", 1},
		{"v2026.10.15-2232", "v2026.10.15-2232", 0},
		{"v2026.10.15-2232", "v2026.10.16-0001", -1},
		{"v2026.10.16-0001", "v2026.10.15-2232", 1},
		{"v2026.10.15-2232", "v2026.10.15-2232-extra", -1},
		{"ltpx_v1", "ltpx_v2", -1},
	}
	for _, c := range cases {
		if got := CompareTagNatural(c.a, c.b); got != c.want {
			t.Errorf("CompareTagNatural(%q, %q) = %d，期望 %d", c.a, c.b, got, c.want)
		}
		if got := CompareTagNatural(c.b, c.a); got != -c.want {
			t.Errorf("CompareTagNatural(%q, %q) = %d，期望 %d（反序应取反）", c.b, c.a, got, -c.want)
		}
	}
}

// TestNewer 排序判定：有时间戳时时间优先，缺失时退化到标签自然序
func TestNewer(t *testing.T) {
	cases := []struct {
		name           string
		ca, ta, cb, tb string
		want           bool
	}{
		{"时间戳决定（即使标签顺序相反）", "2026-10-15T22:50:12+08:00", "v-old", "2026-10-15T22:32:01+08:00", "v-new", true},
		{"时间戳相同则比标签", "2026-10-15T22:50:12+08:00", "v2026.10.15-2250", "2026-10-15T22:50:12+08:00", "v2026.10.15-2232", true},
		{"时间戳缺失退化到标签", "", "v2026.10.15-2250", "", "v2026.10.15-2232", true},
		{"单侧缺失退化到标签", "2026-10-15T22:50:12+08:00", "v2026.10.15-2250", "", "v2026.10.15-2232", true},
		{"更旧", "2026-10-15T22:32:01+08:00", "v2026.10.15-2232", "2026-10-15T22:50:12+08:00", "v2026.10.15-2250", false},
	}
	for _, c := range cases {
		if got := Newer(c.ca, c.ta, c.cb, c.tb); got != c.want {
			t.Errorf("%s: Newer(%q,%q,%q,%q) = %v，期望 %v", c.name, c.ca, c.ta, c.cb, c.tb, got, c.want)
		}
	}
}
