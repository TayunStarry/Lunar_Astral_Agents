package BilibiliViewer

import (
	"strings"
	"testing"

	module "LunarSubsystem/LunarHarness/engine/multimodal"
)

// TestSliceUnderstandPromptIsFeeling 钉住切片任务是「观看感受」，不是分时段内容摘要。
func TestSliceUnderstandPromptIsFeeling(t *testing.T) {
	slice := module.WatchSlice{Start: 0, End: 60, Images: []string{"a", "b"}}
	prompt := module.WatchTask(1, 3, slice, "你正在看《X》，UP主：Y。")
	for _, bad := range []string{"【内容概述】", "【关键信息】", "按时间顺序归并成", "3-6 个连续时段", "描述者格式"} {
		if strings.Contains(prompt, bad) {
			t.Fatalf("切片任务不应要求结构化摘要，命中 %q：%s", bad, prompt)
		}
	}
	for _, want := range []string{"第一人称", "发生了什么", "2-4 句"} {
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
// 本轮关键词权重更高，但默认偏好仍然参与，不再硬过滤（预选要凑够 4-8 个候选）。
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
// 候选固定 4-8 个；切片 2-8 默认 5；评论 30 条、单条 200 字硬截断；弹幕 30 条。
func TestDefaultLifeMechanism(t *testing.T) {
	if candidateMin != 4 || candidateMax != 8 {
		t.Fatalf("候选数量应为 4-8，实际 %d-%d", candidateMin, candidateMax)
	}
	if watchSliceMin != 2 || watchSliceMax != 8 || watchSliceDefault != 5 {
		t.Fatalf("切片数边界应为 2-8、默认 5，实际 %d-%d/%d",
			watchSliceMin, watchSliceMax, watchSliceDefault)
	}
	if collectCommentLimit != 30 || collectCommentMaxRunes != 200 || collectDanmakuLimit != 30 {
		t.Fatalf("采集机制被改动: 评论 %d 条 / 单条 %d 字 / 弹幕 %d 条",
			collectCommentLimit, collectCommentMaxRunes, collectDanmakuLimit)
	}
	cfg := defaultConfig()
	if cfg.RepliesPerRound != defaultRepliesPerRound {
		t.Fatalf("默认回应额度应为 %d，实际 %d", defaultRepliesPerRound, cfg.RepliesPerRound)
	}
}

// TestNewRunnerDefaults 钉住 runner 侧零值兜底。
func TestNewRunnerDefaults(t *testing.T) {
	r := newRunner(Config{RepliesPerRound: 0}, "")
	if r.cfg.RepliesPerRound != defaultRepliesPerRound {
		t.Fatalf("零值应回落到默认回应额度 %d，实际 %d", defaultRepliesPerRound, r.cfg.RepliesPerRound)
	}
}

// TestMemoryContentFormat 钉住统一记忆格式：[B站视频][类别]<正文>。
func TestMemoryContentFormat(t *testing.T) {
	content, err := formatMemoryContent("吐槽", "这个视频太吵了")
	if err != nil {
		t.Fatalf("格式化失败: %v", err)
	}
	if want := "[B站视频][吐槽]这个视频太吵了"; content != want {
		t.Fatalf("记忆格式不符：%q", content)
	}

	entry, ok := parseMemoryContent("id-1", content)
	if !ok || entry.Kind != "吐槽" || entry.Text != "这个视频太吵了" {
		t.Fatalf("解析失败：%+v ok=%v", entry, ok)
	}

	// 前缀改了也不应丢历史条目：解析不依赖前缀文字。
	legacy, ok := parseMemoryContent("id-3", "[BiliBili-生活日记][评价]旧前缀的正文")
	if !ok || legacy.Kind != "评价" || legacy.Text != "旧前缀的正文" {
		t.Fatalf("旧前缀条目应能解析且不含前缀：%+v ok=%v", legacy, ok)
	}

	fallback, _ := formatMemoryContent("不存在的类别", "正文")
	if want := "[B站视频][印象]正文"; fallback != want {
		t.Fatalf("非法类别应回落为印象：%q", fallback)
	}
	if _, ok := parseMemoryContent("id-2", "别的智能体的记忆"); ok {
		t.Fatal("非哔哩观影记忆不应解析成功")
	}
	if _, err := formatMemoryContent("印象", "  "); err == nil {
		t.Fatal("空正文应返回错误")
	}
}

// TestMemoryTextHuman 钉住记忆正文过滤：机械切片文本不写入，只保留人格化内容。
func TestMemoryTextHuman(t *testing.T) {
	junk := "《X》是同一个视频，下面按时间顺序给出 1 个 60 秒切片的理解，请合成为整体观看感受"
	if !memoryLooksMechanical(junk) {
		t.Fatal("机械切片文本应被识别")
	}
	if got := cleanMemoryText("【第 1/1 个 60 秒切片】内容概述\n这段音乐听着很舒服，我循环了好几遍。"); got != "这段音乐听着很舒服，我循环了好几遍。" {
		t.Fatalf("清洗后应只剩人格化正文，实得 %q", got)
	}
	if memoryLooksMechanical("这段音乐听着很舒服，我循环了好几遍。") {
		t.Fatal("人格化感受不应被误判")
	}
}

// TestBuildSteps 钉住 Tick 计划表：第一步发动态，之后每个视频 N 个切片 + 1 个合并。
func TestBuildSteps(t *testing.T) {
	a := &videoRecord{Info: videoInfo{Title: "A", Duration: 600}, Slices: 3}
	b := &videoRecord{Info: videoInfo{Title: "B", Duration: 600}, Slices: 2}
	r := &runner{approved: []*videoRecord{a, b}}
	r.buildSteps()
	if len(r.steps) != 1+3+1+2+1 {
		t.Fatalf("计划表步数不符，实际 %d", len(r.steps))
	}
	if r.steps[0].Kind != stepPublish {
		t.Fatalf("第一步应先发布动态与评论")
	}
	merges := 0
	for _, step := range r.steps {
		if step.Kind == stepMerge {
			merges++
		}
	}
	if merges != 2 {
		t.Fatalf("应为每个视频各一个合并步骤，实际 %d", merges)
	}
}

// TestParseDynamicCommentTarget 钉住动态评论目标：短视频动态的评论区可能是 type=11。
func TestParseDynamicCommentTarget(t *testing.T) {
	raw := `{"code":0,"data":{"item":{"basic":{"comment_id_str":"413545034","comment_type":11,"rid_str":"413545034"}}}}`
	got, err := parseDynamicCommentTarget(raw, "1263926802716295175")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if got.Type != 11 || got.Oid != "413545034" {
		t.Fatalf("应使用详情里的 comment_type/comment_id_str，实得 %+v", got)
	}

	fallback, err := parseDynamicCommentTarget(`{"code":0,"data":{"item":{"basic":{}}}}`, "123")
	if err != nil || fallback.Type != 17 || fallback.Oid != "123" {
		t.Fatalf("缺字段时应回退为 type=17 + 动态 ID，实得 %+v err=%v", fallback, err)
	}
	if _, err := parseDynamicCommentTarget(`{"code":-404,"message":"啥都木有"}`, "123"); err == nil {
		t.Fatal("详情接口报错应返回错误")
	}
}

// TestBuildVideoComment 钉住动态评论区格式：一个视频一条，URL 一行、评价一行。
func TestBuildVideoComment(t *testing.T) {
	video := &videoRecord{
		Info:       videoInfo{URL: "https://www.bilibili.com/video/BV1"},
		Evaluation: "这段运镜太顶了",
	}
	want := "https://www.bilibili.com/video/BV1\n这段运镜太顶了"
	if got := buildVideoComment(video); got != want {
		t.Fatalf("评价条目格式不符：\n%q", got)
	}
	if got := buildVideoComment(&videoRecord{}); got != "" {
		t.Fatalf("没有 URL 的视频不应产生评论，实际 %q", got)
	}
}
