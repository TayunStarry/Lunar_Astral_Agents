package main

import "testing"

// TestBeaconTag 版本信标必须与更新包 tag 落在同一数字空间（日历形式）。
func TestBeaconTag(t *testing.T) {
	cases := map[string]string{
		"2026.291.0847": "v2026.10.18-0847",
		"2026.001.0000": "v2026.01.01-0000",
		"2026.365.2359": "v2026.12.31-2359",
		"":              "",
		"v1.2.3":        "v1.2.3",
	}
	for in, want := range cases {
		if got := beaconTag(in); got != want {
			t.Fatalf("beaconTag(%q) = %q，期望 %q", in, got, want)
		}
	}
}
