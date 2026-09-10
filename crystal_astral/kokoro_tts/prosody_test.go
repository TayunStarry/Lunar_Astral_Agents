package kokoro_tts

import (
	"math"
	"testing"
)

func TestSanitizeText(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"你好世界", "你好世界"},
		{"你好😀世界", "你好世界"},
		{"👨‍👩‍👧‍👦一家人", "一家人"},        // Emoji ZWJ 组合
		{"你好\tab\n你好", "你好\tab\n你好"}, // 空白保留
		{"A\x01B", "AB"},                   // 控制符滤除
		{"✓完成✅", "完成"},                     // 符号象形滤除
		{"private\ue000区", "private区"},     // 私用区滤除
		{"你好[↑][↓][•50]", "你好[↑][↓][•50]"}, // 韵律提示符保留
	}
	for _, c := range cases {
		if got := sanitizeText(c.in); got != c.want {
			t.Errorf("sanitizeText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseProsody(t *testing.T) {
	cases := []struct {
		in   string
		want []ProsodySeg
	}{
		{
			"你好呀! [↑][•50]",
			[]ProsodySeg{{Text: "你好呀! ", Rise: true, PauseAfter: 0.5}},
		},
		{
			"你好[↑]世界",
			[]ProsodySeg{{Text: "你好", Rise: true}, {Text: "世界"}},
		},
		{
			"你好[↓]世界",
			[]ProsodySeg{{Text: "你好", Fall: true}, {Text: "世界"}},
		},
		{
			"你好[•30]世界",
			[]ProsodySeg{{Text: "你好", PauseAfter: 0.3}, {Text: "世界"}},
		},
		{
			"[↑]你好",
			[]ProsodySeg{{Text: "你好", Rise: true}},
		},
		{
			"[•50]你好",
			[]ProsodySeg{{Text: "", PauseAfter: 0.5}, {Text: "你好"}},
		},
		{
			"他说[你好]啊",
			[]ProsodySeg{{Text: "他说[你好]啊"}},
		},
		{
			"你好[↑][↓]",
			[]ProsodySeg{{Text: "你好", Fall: true}}, // 后者覆盖前者
		},
		{
			"无提示文本",
			[]ProsodySeg{{Text: "无提示文本"}},
		},
		{
			"[↑]",
			nil, // 纯提示符无文本，不入列
		},
	}
	for _, c := range cases {
		got := parseProsody(c.in)
		if len(got) != len(c.want) {
			t.Errorf("parseProsody(%q) 段数 = %d, want %d (%+v)", c.in, len(got), len(c.want), got)
			continue
		}
		for i := range got {
			if got[i].Text != c.want[i].Text || got[i].Rise != c.want[i].Rise ||
				got[i].Fall != c.want[i].Fall || !almostEqual(got[i].PauseAfter, c.want[i].PauseAfter) {
				t.Errorf("parseProsody(%q)[%d] = %+v, want %+v", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestPauseSamples(t *testing.T) {
	if n := pauseSamples(0.5); n != SampleRate/2 {
		t.Errorf("pauseSamples(0.5) = %d, want %d", n, SampleRate/2)
	}
	if n := pauseSamples(-1); n != 0 {
		t.Errorf("pauseSamples(-1) = %d, want 0", n)
	}
}

// TestPitchShift 验证音高偏移：长度近似不变、输出有限、频率随因子升降
func TestPitchShift(t *testing.T) {
	n := SampleRate
	samples := make([]float32, n)
	for i := range samples {
		samples[i] = float32(math.Sin(2 * math.Pi * 1000 * float64(i) / float64(SampleRate)))
	}
	base := zeroCrossings(samples)

	for _, f := range []float32{RiseFactor, FallFactor} {
		out := pitchShift(samples, f)
		if d := len(out) - n; d < -3 || d > 3 {
			t.Errorf("pitchShift(%v) 长度 = %d, want ≈%d", f, len(out), n)
		}
		for _, s := range out {
			if math.IsNaN(float64(s)) || math.IsInf(float64(s), 0) {
				t.Fatalf("pitchShift(%v) 输出含 NaN/Inf", f)
			}
		}
		cross := zeroCrossings(out)
		if f > 1 && cross <= int(float64(base)*1.05) {
			t.Errorf("升调 factor=%v 过零次数 = %d, 应显著高于基线 %d", f, cross, base)
		}
		if f < 1 && cross >= int(float64(base)*0.95) {
			t.Errorf("降调 factor=%v 过零次数 = %d, 应显著低于基线 %d", f, cross, base)
		}
	}
}

func zeroCrossings(samples []float32) int {
	if len(samples) == 0 {
		return 0
	}
	var cnt int
	prev := samples[0]
	for _, s := range samples[1:] {
		if (prev < 0 && s >= 0) || (prev >= 0 && s < 0) {
			cnt++
		}
		prev = s
	}
	return cnt
}

func almostEqual(a, b float32) bool {
	return math.Abs(float64(a-b)) < 1e-6
}
