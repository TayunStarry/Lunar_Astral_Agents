package Painter

// instruction.go 把结构化绘图需求整理成绘制智能体可直接执行的指令。

import "strings"

// InstructionRequest 交给绘制智能体的结构化绘图需求。
type InstructionRequest struct {
	Prompt     string
	Ratio      string
	Style      string
	References []string
	Change     string
	Output     string
	Task       string
}

// BuildInstruction 把结构化需求整理成自然语言指令。
func BuildInstruction(req InstructionRequest) string {
	var b strings.Builder
	b.WriteString("有一个绘图任务交给你，请直接产出图片。\n\n")
	b.WriteString("画面描述：")
	b.WriteString(strings.TrimSpace(req.Prompt))
	b.WriteString("\n")
	if ratio := strings.TrimSpace(req.Ratio); ratio != "" {
		b.WriteString("画幅/尺寸：")
		b.WriteString(ratio)
		b.WriteString("\n")
	}
	if style := strings.TrimSpace(req.Style); style != "" {
		b.WriteString("风格约束：")
		b.WriteString(style)
		b.WriteString("\n")
	}
	references := cleanReferences(req.References)
	if len(references) > 0 {
		b.WriteString("参考资源：\n")
		for index, reference := range references {
			label := "附加参考图"
			if index == 0 {
				label = "主参考图（图生图基座）"
			}
			b.WriteString("- ")
			b.WriteString(label)
			b.WriteString("：")
			b.WriteString(reference)
			b.WriteString("\n")
		}
		b.WriteString("任务类型：图生图。参考图已作为视觉副本随任务附上；" +
			"必须调用 image_to_image，以主参考图为基座（合照可用 group_photo，它同样以参考图为基座），" +
			"不要改用 diffusion_generation 从零重画。\n")
		if change := strings.TrimSpace(req.Change); change != "" {
			b.WriteString("修改要求：")
			b.WriteString(change)
			b.WriteString("\n")
		} else {
			b.WriteString("修改要求：按画面描述执行\n")
		}
	} else {
		b.WriteString("参考资源：无\n")
		b.WriteString("任务类型：纯文生图。没有参考图，不得把上一次生成的画作当作基座；" +
			"请调用 diffusion_generation（人物立绘 / 合照按对应工具）从零生成。\n")
	}
	if output := strings.TrimSpace(req.Output); output != "" {
		b.WriteString("输出去向：")
		b.WriteString(output)
		b.WriteString("\n")
	}
	if task := strings.TrimSpace(req.Task); task != "" {
		b.WriteString("补充说明：")
		b.WriteString(task)
		b.WriteString("\n")
	}
	b.WriteString("\n要求：不要用文字、SVG、Canvas 或代码绘图替代图片产出；" +
		"完成后在回复中给出资源句柄；无法按描述绘制时明确说明失败原因。")
	return b.String()
}

// cleanReferences 去空、去重。
func cleanReferences(raw []string) []string {
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}
