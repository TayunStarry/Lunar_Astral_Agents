package module

import (
	"fmt"
	"testing"
)

func TestNumbersToChinese(t *testing.T) {
	cases := map[string]string{
		"123":       "一百二十三",
		"10":        "十",
		"15":        "十五",
		"100":       "一百",
		"101":       "一百零一",
		"1001":      "一千零一",
		"12345":     "一万二千三百四十五",
		"100000001": "一亿零一",
		"0":         "零",
		"1.5":       "一点五",
		"50%":       "百分之五十",
		"-3":        "负三",
		"1/2":       "二分之一",
		"第3":       "第三",
	}
	for in, want := range cases {
		got := numbersToChinese(in)
		if got != want {
			t.Errorf("numbersToChinese(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestZhFrontend(t *testing.T) {
	f, err := newZhFrontend()
	if err != nil {
		t.Fatalf("newZhFrontend: %v", err)
	}
	samples := []string{
		"你好世界",
		"你好，世界！",
		"我今天去了北京，看到了长城。",
		"小明是银行的行长，他喜欢音乐和快乐的生活。",
		"我不怕困难，一步一步向前走。",
		"小猫追着小狗跑，真有趣。",
	}
	for _, s := range samples {
		out := f.Call(s)
		fmt.Printf("%s\n  -> %s\n", s, out)
	}
}

func TestPhonemizeRouting(t *testing.T) {
	f, err := newZhFrontend()
	if err != nil {
		t.Fatalf("newZhFrontend: %v", err)
	}
	_ = f
	out := f.Call(numbersToChinese("今天温度是25度，湿度50%"))
	fmt.Printf("温度示例 -> %s\n", out)
}
