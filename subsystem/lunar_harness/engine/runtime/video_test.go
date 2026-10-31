package runtime

// video_test.go video.frames 采样计划：times > count > fps 的优先级与帧数上限。

import (
	"testing"

	multimodal "LunarSubsystem/LunarHarness/engine/multimodal"
)

func TestVideoSamplingPlanPriority(t *testing.T) {
	// times 显式时间点优先
	plan := videoSamplingPlan(10, map[string]any{"times": []any{int64(1), int64(4)}})
	if len(plan) != 2 || plan[0] != 1 || plan[1] != 4 {
		t.Fatalf("times 应精确抽取，实得 %+v", plan)
	}

	// count 等距
	plan = videoSamplingPlan(10, map[string]any{"count": int64(3)})
	if len(plan) != 3 {
		t.Fatalf("count 应等距抽取 3 帧，实得 %+v", plan)
	}

	// fps 均匀网格（10 秒 × 2fps = 21 点）
	plan = videoSamplingPlan(10, map[string]any{"fps": float64(2)})
	if len(plan) != 21 {
		t.Fatalf("fps 抽帧数不正确，实得 %d", len(plan))
	}
}

func TestVideoSamplingPlanCap(t *testing.T) {
	// fps 过高时按上限等距抽样，不会超过 MaxVideoFrames
	plan := videoSamplingPlan(120, map[string]any{"fps": float64(60)})
	if len(plan) > multimodal.MaxVideoFrames {
		t.Fatalf("抽帧数不应超过上限 %d，实得 %d", multimodal.MaxVideoFrames, len(plan))
	}
}
