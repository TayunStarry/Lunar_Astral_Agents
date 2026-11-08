package creation

// paint.go 把创作链的绘图需求交给现有图片绘制智能体，创作链只保留编排与验收。

import (
	"context"
	"fmt"
	"strings"
)

// PaintInstruction 把结构化绘图需求整理成图片绘制智能体可直接执行的指令。
func PaintInstruction(req PaintRequest) string {
	var b strings.Builder
	b.WriteString("创作链有一个绘图任务交给你，请直接产出图片。\n\n")
	b.WriteString("画面描述：" + strings.TrimSpace(req.Prompt) + "\n")
	if ratio := strings.TrimSpace(req.Ratio); ratio != "" {
		b.WriteString("画幅/尺寸：" + ratio + "\n")
	}
	if style := strings.TrimSpace(req.Style); style != "" {
		b.WriteString("风格约束：" + style + "\n")
	}
	references := cleanHandles(req.References)
	if len(references) > 0 {
		b.WriteString("参考资源：\n")
		for index, reference := range references {
			label := "附加参考图"
			if index == 0 {
				label = "主参考图（图生图基座）"
			}
			b.WriteString("- " + label + "：" + reference + "\n")
		}
		b.WriteString("任务类型：图生图。参考图已作为视觉副本随任务附上；" +
			"必须调用 image_to_image，以主参考图为基座（合照可用 group_photo，它同样以参考图为基座），" +
			"不要改用 diffusion_generation 从零重画。\n")
		if change := strings.TrimSpace(req.Change); change != "" {
			b.WriteString("修改要求：" + change + "\n")
		} else {
			b.WriteString("修改要求：按画面描述与调度者补充执行\n")
		}
	} else {
		b.WriteString("参考资源：无\n")
		b.WriteString("任务类型：纯文生图。没有参考图，不得把上一次生成的画作当作基座；" +
			"请调用 diffusion_generation（人物立绘 / 合照按对应工具）从零生成。\n")
	}
	if output := strings.TrimSpace(req.Output); output != "" {
		b.WriteString("输出去向：" + output + "\n")
	} else {
		b.WriteString("输出去向：默认多媒体资源目录，完成后回复资源句柄\n")
	}
	if task := strings.TrimSpace(req.Task); task != "" {
		b.WriteString("调度者补充：" + task + "\n")
	}
	b.WriteString("\n要求：不要用文字、SVG、Canvas 或代码绘图替代图片产出；" +
		"完成后在回复中给出资源句柄；无法按描述绘制时明确说明失败原因。")
	return b.String()
}

// runPaintStep 执行一个图片绘制步骤，返回执行摘要与产出的资源句柄。
// 失败时返回明确原因，交由后续规划决定重试或调整描述，不做静默跳过。
func (s *session) runPaintStep(ctx context.Context, step planStep) (string, []string, error) {
	if PaintHook == nil {
		return "", nil, fmt.Errorf("图片绘制智能体未接入，无法执行绘图步骤")
	}
	req := PaintRequest{Task: step.Task}
	if step.Paint != nil {
		req.Prompt = step.Paint.Prompt
		req.Ratio = step.Paint.Ratio
		req.Style = step.Paint.Style
		req.References = append([]string(nil), step.Paint.References...)
		req.Change = step.Paint.Change
		req.Output = step.Paint.Output
	}
	if strings.TrimSpace(req.Prompt) == "" {
		req.Prompt = strings.TrimSpace(step.Task)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return "", nil, fmt.Errorf("绘图步骤缺少画面描述")
	}

	paintCtx, cancel := context.WithTimeout(ctx, paintTimeout)
	defer cancel()
	result, err := PaintHook(paintCtx, req)
	if err != nil {
		return "", nil, fmt.Errorf("图片绘制失败：%w", err)
	}
	handles := cleanHandles(result.Handles)
	if len(handles) == 0 {
		reason := clipAtSentence(strings.TrimSpace(result.Summary), maxErrorRunes)
		if reason == "" {
			reason = "未返回任何资源句柄"
		}
		return "", nil, fmt.Errorf("图片绘制未产出图片：%s", reason)
	}

	s.paintHandles = appendUnique(s.paintHandles, handles...)
	s.refs = appendUnique(s.refs, handles...)
	summary := strings.TrimSpace(result.Summary)
	if summary == "" {
		summary = "图片绘制智能体已按需求产出图片"
	}
	return fmt.Sprintf("%s（句柄：%s）", clipAtSentence(summary, maxSummaryRunes), strings.Join(handles, "、")), handles, nil
}

// cleanHandles 清洗句柄列表：去空、去重、去空白。
func cleanHandles(raw []string) []string {
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item != "" {
			out = appendUnique(out, item)
		}
	}
	return out
}

// appendUnique 追加不重复的字符串。
func appendUnique(list []string, items ...string) []string {
	seen := make(map[string]bool, len(list)+len(items))
	for _, item := range list {
		seen[item] = true
	}
	for _, item := range items {
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		list = append(list, item)
	}
	return list
}

// stepPercent 把步骤序号换算为 20-80 的执行阶段进度。
func stepPercent(index, total int) int {
	if total <= 0 {
		return 20
	}
	if index < 0 {
		index = 0
	}
	return 20 + index*60/total
}
