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
//
// previous 是上一镜成功产出的图片句柄：作为参考图交给绘制智能体，
// 由它自己的图生图机制承接角色外观与画面基调；首镜没有参考图时走纯文生图。
func paintShot(ctx context.Context, shot plannedShot, previous string) (PaintResult, error) {
	if PaintHook == nil {
		return PaintResult{}, fmt.Errorf("图片绘制智能体未接入，无法执行绘制")
	}
	references := make([]string, 0, 1)
	if handle := strings.TrimSpace(previous); handle != "" {
		references = append(references, handle)
	}
	var lastErr error
	for attempt := 1; attempt <= maxPaintAttempts; attempt++ {
		if ctx.Err() != nil {
			return PaintResult{}, fmt.Errorf("任务已取消或超时：%w", ctx.Err())
		}
		attemptCtx, cancel := context.WithTimeout(ctx, shotPaintTimeout)
		result, err := PaintHook(attemptCtx, PaintRequest{
			Prompt:     shot.Prompt,
			Ratio:      shot.Ratio,
			Style:      shot.Style,
			Task:       fmt.Sprintf("演绎链第 %d 镜（%s）", shot.Index, shot.Scene),
			Output:     "默认多媒体资源目录，完成后回复资源句柄",
			References: references,
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
