package viewer

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/agents/Vision"
	module "LunarSubsystem/LunarHarness/engine/multimodal"
)

// TestSliceUnderstandPromptIsConcrete 钉住切片任务要可复述的具体事实与固定字段，
// 而不是空泛感受或旧式分时段摘要。
func TestSliceUnderstandPromptIsConcrete(t *testing.T) {
	slice := module.WatchSlice{Start: 0, End: 60, Images: []string{"a", "b"}}
	prompt := Vision.WatchTask(1, 3, slice, "你正在看《X》，UP主：Y。")
	for _, bad := range []string{"【内容概述】", "【关键信息】", "按时间顺序归并成", "3-6 个连续时段", "描述者格式", "2-4 句感受"} {
		if strings.Contains(prompt, bad) {
			t.Fatalf("切片任务不应要求空泛感受或旧式摘要，命中 %q：%s", bad, prompt)
		}
	}
	for _, want := range []string{"场景：", "主体：", "动作：", "画面文字：", "情绪与语气：", "这段在讲什么", "不超过 200 字", "禁止空话套话"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("切片任务应要求 %q：%s", want, prompt)
		}
	}
}

// TestSampleImages 钉住采样帧上限：只给模型少量代表性画面。
func TestSampleImages(t *testing.T) {
	images := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}
	got := module.SampleImages(images, 3)
	if len(got) != 3 || got[0] != "0" || got[1] != "3" || got[2] != "6" {
		t.Fatalf("应均匀抽取 3 帧，实得 %v", got)
	}
	if got := module.SampleImages(images, 20); len(got) != len(images) {
		t.Fatalf("帧数不足上限时应原样返回，实得 %d", len(got))
	}
}

// TestFilterAndScorePreferences 钉住选片打分：
// 本轮关键词权重更高，但默认偏好仍然参与，不再硬过滤（待看列表要凑够候选数）。
func TestFilterAndScorePreferences(t *testing.T) {
	cfg := defaultConfig()
	cfg.DesireKeywords = []string{"猫咪"}
	links := []videoLink{
		{Href: "https://www.bilibili.com/video/BV1", Title: "AI 绘画入门：大肥鱼模型实测"},
		{Href: "https://www.bilibili.com/video/BV2", Title: "猫咪和 AI 的一天"},
		{Href: "https://www.bilibili.com/video/BV3", Title: "直播回放：昨天"},
	}
	out := filterAndScore(links, cfg)
	if len(out) != len(links) {
		t.Fatalf("预选阶段不应过滤候选，实际 %d 条", len(out))
	}
	if out[1].Score <= out[0].Score || out[1].Score <= out[2].Score {
		t.Fatalf("命中本轮关键词的候选得分应最高，实际 %#v", out)
	}
	if out[2].Score != 0 {
		t.Fatalf("没有命中任何关键词的候选应为 0 分，实际 %d", out[2].Score)
	}
}

// TestDefaultLifeMechanism 钉住新机制默认值：
// 单视频单切片 60 秒；评论最多 2 条、单条 80 字；单条采集评论 512 字。
func TestDefaultLifeMechanism(t *testing.T) {
	if watchSegmentSeconds != 60 {
		t.Fatalf("单切片时长应为 60 秒，实际 %d", watchSegmentSeconds)
	}
	if watchRemarkMaxComments != 2 || watchRemarkMaxRunes != 80 {
		t.Fatalf("评论机制被改动: 最多 %d 条 / 单条 %d 字", watchRemarkMaxComments, watchRemarkMaxRunes)
	}
	if collectCommentMaxRunes != 512 {
		t.Fatalf("采集机制被改动: 单条评论 %d 字", collectCommentMaxRunes)
	}
}

// TestMemoryTextHuman 钉住记忆正文过滤：机械切片文本不写入，只保留人格化内容。
func TestMemoryTextHuman(t *testing.T) {
	junk := "《X》是同一个视频，下面按时间顺序给出 1 个 60 秒切片的理解，请合成为整体观看感受"
	if !looksMechanical(junk) {
		t.Fatal("机械切片文本应被识别")
	}
	if got := cleanMemoryText("【第 1/1 个 60 秒切片】内容概述\n这段音乐听着很舒服，我循环了好几遍。"); got != "这段音乐听着很舒服，我循环了好几遍。" {
		t.Fatalf("清洗后应只剩人格化正文，实得 %q", got)
	}
	if looksMechanical("这段音乐听着很舒服，我循环了好几遍。") {
		t.Fatal("人格化感受不应被误判")
	}
}
