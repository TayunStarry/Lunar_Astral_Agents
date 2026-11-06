package creation

// plan_build_text.go 文字类事项（小说 / 事件研究 / 小故事）的参数组合与可执行要求。

import (
	"fmt"
	"math/rand"
)

// pick 从参数池中随机取一项；空池返回零值。
func pick[T any](r *rand.Rand, pool []T) T {
	var zero T
	if len(pool) == 0 {
		return zero
	}
	return pool[r.Intn(len(pool))]
}

// buildNovel 组合「写小说」事项：题材 + 篇幅 + 视角。
func buildNovel(r *rand.Rand, _ qqTargets) (PlanItem, bool) {
	theme := pick(r, novelThemes)
	length := pick(r, novelLengths)
	voice := pick(r, novelVoices)
	return PlanItem{
		Kind:   KindNovel,
		Title:  "写一篇" + theme + "小说",
		Params: map[string]string{"题材": theme, "篇幅": length, "视角": voice},
		Task: fmt.Sprintf("写一篇%s小说，篇幅为%s，采用%s。先在正文前用三行写清主要人物、核心冲突与结局走向，再完成正文；标题自拟并写在文首。成品保存到 /document/创作/小说/ 目录，文件名包含标题。",
			theme, length, voice),
		Activity: "刚写完一篇" + theme + "小说",
	}, true
}

// buildResearch 组合「分析研究特定事件」事项：具体事件 + 研究角度。
func buildResearch(r *rand.Rand, _ qqTargets) (PlanItem, bool) {
	event := pick(r, researchEvents)
	angle := pick(r, researchAngles)
	return PlanItem{
		Kind:   KindResearch,
		Title:  "研究「" + event.Short + "」",
		Params: map[string]string{"事件": event.Name, "角度": angle},
		Task: fmt.Sprintf("针对「%s」做一次资料研究，重点输出%s。先用 web_search 查证至少 3 条可靠资料，再在 /document/研究/ 目录写一篇不少于 1200 字的分析报告；正文末尾列出参考链接，不得编造未查证的数据与时间。",
			event.Name, angle),
		Activity: "刚研究完「" + event.Short + "」",
	}, true
}

// buildStory 组合「创作小故事」事项：基调 + 篇幅 + 主角。
func buildStory(r *rand.Rand, _ qqTargets) (PlanItem, bool) {
	tone := pick(r, storyTones)
	length := pick(r, storyLengths)
	lead := pick(r, storyLeads)
	return PlanItem{
		Kind:   KindStory,
		Title:  "写一篇" + tone + "小故事",
		Params: map[string]string{"基调": tone, "篇幅": length, "主角": lead},
		Task: fmt.Sprintf("写一篇%s的小故事，篇幅%s，主角是%s。要求有完整的起承转合和一个意外的结尾，不要写成大纲或设定说明；标题自拟。成品保存到 /document/创作/故事/ 目录。",
			tone, length, lead),
		Activity: "刚写完一篇" + tone + "小故事",
	}, true
}
