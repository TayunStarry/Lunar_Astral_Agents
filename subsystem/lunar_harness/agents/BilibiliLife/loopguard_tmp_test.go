package BilibiliLife

import "testing"

func TestLoopDetectedTemporary(t *testing.T) {
	loop := ""
	for i := 0; i < 8; i++ {
		loop += "他觉得这个镜头很有意思，然后又把刚才的想法重复了一遍。"
	}
	if !loopDetected(loop) {
		t.Fatalf("重复文本未被识别")
	}

	normal := "今天看的这个视频讲的是城市里的老建筑改造，作者从外立面一直讲到内部动线。" +
		"前半段节奏偏慢，但镜头里的光影处理很舒服，配乐也没有喧宾夺主。" +
		"中段开始介绍施工细节，能看出团队在保留原有砖墙纹理上花了不少心思。" +
		"结尾回到街区生活，店主们对新空间的评价各不相同，这种对比挺有意思。"
	if loopDetected(normal) {
		t.Fatalf("正常文本被误判为循环")
	}

	shifted := ""
	for i := 0; i < 40; i++ {
		shifted += "abcdefg"
	}
	if !loopDetected(shifted) {
		t.Fatalf("错位周期重复未被识别")
	}
}

func loopDetected(loop string) bool {
	panic("unimplemented")
}
