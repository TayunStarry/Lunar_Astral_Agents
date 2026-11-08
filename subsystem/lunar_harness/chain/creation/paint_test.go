package creation

// paint_test.go 创作链与图片绘制智能体的交接契约。

import (
	"context"
	"strings"
	"testing"
)

// TestParsePlanRequiresPaintBrief 交给图片绘制的步骤必须带结构化绘图需求。
func TestParsePlanRequiresPaintBrief(t *testing.T) {
	_, err := parsePlan(map[string]any{
		"steps": []any{
			map[string]any{"agent": agentPainter, "task": "画一只猫"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "paint.prompt") {
		t.Fatalf("缺少 paint 时应拒绝计划: %v", err)
	}

	plan, err := parsePlan(map[string]any{
		"steps": []any{
			map[string]any{
				"agent": agentPainter,
				"task":  "把参考图改成水彩风格",
				"paint": map[string]any{
					"prompt":    "橘猫趴在窗台",
					"ratio":     "16:9",
					"style":     "水彩",
					"reference": "@I.003",
					"change":    "把背景换成雨夜街道，保留橘猫",
					"output":    "多媒体资源目录",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("合法绘图步骤不应被拒绝: %v", err)
	}
	paint := plan.Steps[0].Paint
	if paint == nil || paint.Prompt != "橘猫趴在窗台" || paint.Ratio != "16:9" ||
		paint.Style != "水彩" || len(paint.References) != 1 || paint.References[0] != "@I.003" ||
		paint.Change == "" || paint.Output == "" {
		t.Fatalf("绘图需求解析不完整: %#v", paint)
	}
}

// TestParsePaintMultiReference 单步多参考图：reference 与 references 合并去重。
func TestParsePaintMultiReference(t *testing.T) {
	plan, err := parsePlan(map[string]any{
		"steps": []any{
			map[string]any{
				"agent": agentPainter,
				"task":  "以两张参考图合成新画面",
				"paint": map[string]any{
					"prompt":     "雪夜中的城堡",
					"reference":  "@I.003",
					"references": []any{"@D.020|§1", "@I.003", "@I.004"},
					"change":     "保留人物，把背景换成雪夜",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("多参考图不应被拒绝: %v", err)
	}
	refs := plan.Steps[0].Paint.References
	want := []string{"@I.003", "@D.020|§1", "@I.004"}
	if len(refs) != len(want) {
		t.Fatalf("参考图数量不符: %#v", refs)
	}
	for index, item := range want {
		if refs[index] != item {
			t.Fatalf("参考图顺序或去重不符: %#v", refs)
		}
	}
}

// TestParsePaintReferenceRequiresChange 有参考图时必须说明基于它改什么。
func TestParsePaintReferenceRequiresChange(t *testing.T) {
	_, err := parsePlan(map[string]any{
		"steps": []any{
			map[string]any{
				"agent": agentPainter,
				"task":  "改图",
				"paint": map[string]any{
					"prompt":    "雪夜中的城堡",
					"reference": "@I.003",
				},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "paint.change") {
		t.Fatalf("有参考图但缺少 change 时应拒绝计划: %v", err)
	}
}

// TestPaintInstructionCarriesHandoff 交接指令必须包含描述、画幅、风格、参考、修改要求与输出去向。
func TestPaintInstructionCarriesHandoff(t *testing.T) {
	instruction := PaintInstruction(PaintRequest{
		Prompt:     "雨夜霓虹街道",
		Ratio:      "1:1",
		Style:      "赛博朋克",
		References: []string{"@I.007", "@D.020|§1"},
		Change:     "保留人物，把背景换成雨夜霓虹街道",
		Output:     "/多媒体/创作",
		Task:       "自由创作绘图",
	})
	for _, want := range []string{
		"雨夜霓虹街道", "1:1", "赛博朋克", "@I.007", "@D.020|§1",
		"主参考图（图生图基座）", "附加参考图", "修改要求", "image_to_image", "/多媒体/创作", "资源句柄",
	} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("交接指令缺少 %q: %s", want, instruction)
		}
	}
}

// TestPaintInstructionTextOnly 没有参考图时必须明确走纯文生图。
func TestPaintInstructionTextOnly(t *testing.T) {
	instruction := PaintInstruction(PaintRequest{Prompt: "星空下的樱花树"})
	for _, want := range []string{"纯文生图", "diffusion_generation", "不得把上一次生成的画作当作基座"} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("无参考图指令缺少 %q: %s", want, instruction)
		}
	}
	if strings.Contains(instruction, "image_to_image") {
		t.Fatalf("无参考图不应要求图生图: %s", instruction)
	}
}

// TestRunPaintStepSharesReference 多步骤共享同一参考图时，每一步都原样带上参考图与修改要求。
func TestRunPaintStepSharesReference(t *testing.T) {
	original := PaintHook
	defer func() { PaintHook = original }()

	requests := make([]PaintRequest, 0, 2)
	PaintHook = func(_ context.Context, req PaintRequest) (PaintResult, error) {
		requests = append(requests, req)
		return PaintResult{Summary: "已产出", Handles: []string{"@I.010"}}, nil
	}
	step := planStep{
		Agent: agentPainter,
		Task:  "把这张图改成雪夜",
		Paint: &paintBrief{
			Prompt:     "雪夜中的城堡",
			References: []string{"@I.003"},
			Change:     "保留人物，把背景换成雪夜",
		},
	}
	for round := 0; round < 2; round++ {
		if _, _, err := (&session{}).runPaintStep(context.Background(), step); err != nil {
			t.Fatalf("第 %d 次绘图失败: %v", round+1, err)
		}
	}
	if len(requests) != 2 {
		t.Fatalf("应调用两次绘制，实际 %d 次", len(requests))
	}
	for index, req := range requests {
		if len(req.References) != 1 || req.References[0] != "@I.003" || req.Change == "" {
			t.Fatalf("第 %d 次未带上共享参考图或修改要求: %#v", index+1, req)
		}
	}
}

// TestSubmitTaskValidation 提交入口只接受合法模式与必要描述。
func TestSubmitTaskValidation(t *testing.T) {
	if _, err := SubmitTask(SubmitRequest{Mode: "unknown"}); err == nil {
		t.Fatal("未知 mode 应被拒绝")
	}
	if _, err := SubmitTask(SubmitRequest{Mode: "assigned"}); err == nil {
		t.Fatal("assigned 缺少 description 应被拒绝")
	}
}
