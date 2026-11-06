package creation

// plan_build_task.go 工程 / 整理 / QQ 推送类事项的参数组合与可执行要求。

import (
	"fmt"
	"math/rand"
	"strconv"
)

// buildProject 组合「编写有趣的小项目」事项：方向 + 技术形态。
func buildProject(r *rand.Rand, _ qqTargets) (PlanItem, bool) {
	topic := pick(r, projectTopics)
	layout := pick(r, projectLayouts)
	return PlanItem{
		Kind:   KindProject,
		Title:  "做一个小项目：" + topic,
		Params: map[string]string{"方向": topic, "技术形态": layout},
		Task: fmt.Sprintf("在 /document/项目/%s/ 目录做一个「%s」，技术形态为%s。要求完全离线可用（不引用任何外部 CDN），交互完整、打开即可体验；完成后用 run_script 运行自检，确认没有脚本错误。",
			topic, topic, layout),
		Activity: "刚写完一个" + topic + "小项目",
	}, true
}

// buildTidy 组合「整理本地文件夹」事项：具体目录范围 + 整理动作。
func buildTidy(r *rand.Rand, _ qqTargets) (PlanItem, bool) {
	scope := pick(r, tidyScopes)
	action := pick(r, tidyActions)
	return PlanItem{
		Kind:   KindTidy,
		Title:  "整理 /document/" + scope,
		Params: map[string]string{"目录": "/document/" + scope, "整理动作": action},
		Task: fmt.Sprintf("只整理 /document/%s 这一范围，不触碰其他目录。先用 file_list 查看现状，%s；最后在该目录写 README.md 记录整理结果。不要删除任何文件，无法判断的文件保持原位。",
			scope, action),
		Activity: "刚整理完 " + scope + " 文件夹",
	}, true
}

// buildJoke 组合「向 QQ 推送小笑话」事项；适配器不可用或无目标时返回 false。
func buildJoke(r *rand.Rand, qq qqTargets) (PlanItem, bool) {
	if len(qq.Items) == 0 {
		return PlanItem{}, false
	}
	target := qq.Items[r.Intn(len(qq.Items))]
	style := pick(r, jokeStyles)
	tool, targetText := jokeDelivery(target)
	return PlanItem{
		Kind:  KindJoke,
		Title: "给" + target.Name + "讲个笑话",
		Params: map[string]string{
			"对象类型": map[bool]string{true: "群聊", false: "用户"}[target.Group],
			"对象":   target.Name,
			"对象ID": strconv.FormatInt(target.ID, 10),
			"风格":   style,
		},
		Task: fmt.Sprintf("写一条 80 字以内的%s，然后用 %s 工具发送给%s；只发这一条，不要连续发送、不要追问对方回复。发送前确认文本完整、没有错别字。",
			style, tool, targetText),
		Activity: "刚给" + target.Name + "讲了个笑话",
	}, true
}

// jokeDelivery 返回投递工具名与目标描述。
func jokeDelivery(target qqTarget) (tool, text string) {
	if target.Group {
		return toolSendGroupText, fmt.Sprintf("QQ 群「%s」（群号 %s）", target.Name, strconv.FormatInt(target.ID, 10))
	}
	return toolSendPrivateText, fmt.Sprintf("QQ 好友「%s」（QQ 号 %s）", target.Name, strconv.FormatInt(target.ID, 10))
}
