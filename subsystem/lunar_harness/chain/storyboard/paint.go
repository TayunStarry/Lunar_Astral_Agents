package storyboard

// paint.go 逐镜调用现有图片绘制智能体；超时、重试与失败原因在此收敛。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// paintShot 绘制单个分镜，失败时按上限重试；返回最后一次失败原因。
func paintShot(ctx context.Context, shot plannedShot, outputFolder string) (PaintResult, error) {
	if PaintHook == nil {
		return PaintResult{}, fmt.Errorf("图片绘制智能体未接入，无法执行绘制")
	}
	var lastErr error
	for attempt := 1; attempt <= maxPaintAttempts; attempt++ {
		if ctx.Err() != nil {
			return PaintResult{}, fmt.Errorf("任务已取消或超时：%w", ctx.Err())
		}
		attemptCtx, cancel := context.WithTimeout(ctx, shotPaintTimeout)
		result, err := PaintHook(attemptCtx, PaintRequest{
			Prompt: shot.Prompt,
			Ratio:  shot.Ratio,
			Style:  shot.Style,
			Task: fmt.Sprintf("演绎链第 %d 镜（%s）。本任务优先纯文生图：请尽量不使用图生图，"+
				"不要沿用上一镜或历史画作，按本镜画面描述独立生成。", shot.Index, shot.Scene),
			Output:       "演绎项目图片目录，完成后回复资源句柄",
			OutputFolder: outputFolder,
			Silent:       true,
		})
		cancel()
		if err == nil {
			handles := cleanHandles(result.Handles)
			if len(handles) > 0 {
				return PaintResult{Summary: strings.TrimSpace(result.Summary), Handles: handles}, nil
			}
			err = fmt.Errorf("未返回任何资源句柄")
		}
		lastErr = err
		logger.Warn(Module, "第 %d 镜第 %d 次绘制失败: %v", shot.Index, attempt, err)
		if attempt < maxPaintAttempts {
			select {
			case <-ctx.Done():
				return PaintResult{}, fmt.Errorf("任务已取消或超时：%w", ctx.Err())
			case <-time.After(retryBackoff):
			}
		}
	}
	return PaintResult{}, fmt.Errorf("第 %d 镜绘制失败（已重试 %d 次）：%v", shot.Index, maxPaintAttempts, lastErr)
}

// cleanHandles 去空、去重并保留顺序。
func cleanHandles(raw []string) []string {
	out := make([]string, 0, len(raw))
	seen := make(map[string]bool, len(raw))
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
