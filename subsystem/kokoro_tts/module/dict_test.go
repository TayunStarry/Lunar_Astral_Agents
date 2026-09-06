package module

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPronunciationDict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pronunciation_dict.json")

	d, err := LoadPronunciationDict(path)
	if err != nil {
		t.Fatalf("LoadPronunciationDict: %v", err)
	}
	pronunciationDict = d

	// 空词典
	if pys, ok := d.Get("行长"); ok || pys != nil {
		t.Errorf("空词典不应命中: %v %v", pys, ok)
	}

	// 添加词条
	if err := d.Set("行长", "hang2 zhang3"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	pys, ok := d.Get("行长")
	if !ok || len(pys) != 2 || pys[0] != "hang2" || pys[1] != "zhang3" {
		t.Errorf("Get 行长 = %v, ok=%v", pys, ok)
	}

	// 带声调符号的拼音归一化
	if err := d.Set("音乐", "yīn yuè"); err != nil {
		t.Fatalf("Set 音乐: %v", err)
	}
	pys, _ = d.Get("音乐")
	if pys[0] != "yin1" || pys[1] != "yue4" {
		t.Errorf("音乐归一化 = %v", pys)
	}

	// 无调拼音默认轻声
	if err := d.Set("看看", "kan kan"); err != nil {
		t.Fatalf("Set 看看: %v", err)
	}
	pys, _ = d.Get("看看")
	if pys[1] != "kan5" {
		t.Errorf("看看轻声默认 = %v", pys)
	}

	// 音节数与字数不一致应报错
	if err := d.Set("行长", "hang2"); err == nil {
		t.Errorf("音节数不匹配应报错")
	}

	// 持久化后重新加载
	if err := d.Set("角色", "jue2 se4"); err != nil {
		t.Fatalf("Set 角色: %v", err)
	}
	d2, err := LoadPronunciationDict(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, ok := d2.Get("角色"); !ok {
		t.Errorf("重新加载后角色丢失")
	}

	// 删除
	if err := d.Delete("角色"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := d.Get("角色"); ok {
		t.Errorf("删除后仍存在")
	}

	_ = os.Remove(path)
}

func TestDictAffectsG2P(t *testing.T) {
	dir := t.TempDir()
	d, _ := LoadPronunciationDict(filepath.Join(dir, "d.json"))
	pronunciationDict = d

	// 覆盖前：只 默认读音
	ini0, fin0 := getInitialsFinals("只有")
	t.Logf("覆盖前 只有 -> %v %v", ini0, fin0)

	// 覆盖后
	if err := d.Set("只有", "zhi3 you3"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	ini1, fin1 := getInitialsFinals("只有")
	t.Logf("覆盖后 只有 -> %v %v", ini1, fin1)
	if ini1[0] != "zh" || fin1[0] != "iii3" {
		t.Errorf("只 应为 zhi3(即 zh+iii3), 得到 %s%s", ini1[0], fin1[0])
	}

	// 覆盖后：行长
	if err := d.Set("行长", "hang2 zhang3"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	ini2, fin2 := getInitialsFinals("行长")
	if ini2[0] != "h" || fin2[0] != "ang2" || ini2[1] != "zh" || fin2[1] != "ang3" {
		t.Errorf("行长读音错误: %v %v", ini2, fin2)
	}
}

func TestReconstructPinyin(t *testing.T) {
	dir := t.TempDir()
	d, _ := LoadPronunciationDict(filepath.Join(dir, "d.json"))
	pronunciationDict = d

	cases := []string{"银行", "音乐", "快乐", "只有", "不", "一二三四五", "儿", "我爱北京天安门"}
	for _, w := range cases {
		ini, fin := getInitialsFinals(w)
		pys := reconstructPinyin(ini, fin)
		t.Logf("%s -> %s (%v %v)", w, joinSpace(pys), ini, fin)
	}
}

func joinSpace(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += " "
		}
		out += s
	}
	return out
}
