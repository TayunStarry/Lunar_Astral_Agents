package storyboard

// agent.go 演绎链的公共辅助：读取故事、进度映射、分镜视图复制。
//
// 任务编排已迁移到 advance.go 的步骤机，这里只保留无副作用的纯函数。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/resource"
)

// resolveStory 优先使用纯文本；没有文本时按资源句柄读取。
// 两条路径都受同一字数上限约束，超限直接报错而不是静默截断。
func resolveStory(req SubmitRequest) (string, error) {
	if text := strings.TrimSpace(req.Text); text != "" {
		if err := validateStoryLength(text); err != nil {
			return "", err
		}
		return text, nil
	}
	handle := strings.TrimSpace(req.Handle)
	if handle == "" {
		return "", fmt.Errorf("请提供故事正文 text 或资源句柄 handle")
	}
	result, err := resource.Read(handle, 4<<20)
	if err != nil {
		return "", fmt.Errorf("读取故事资源失败：%w", err)
	}
	if result.Lost {
		return "", fmt.Errorf("故事资源已丢失：%s", handle)
	}
	text := strings.TrimSpace(result.Content)
	if text == "" {
		return "", fmt.Errorf("资源 %s 没有可用的文本内容", handle)
	}
	if err := validateStoryLength(text); err != nil {
		return "", fmt.Errorf("资源 %s %w", handle, err)
	}
	return text, nil
}

// validateStoryLength 校验故事正文字数；超过上限返回可读错误。
func validateStoryLength(text string) error {
	count := len([]rune(text))
	if count > maxStoryRunes {
		return fmt.Errorf("共 %d 字，超过 %d 字上限；请精简后再提交", count, maxStoryRunes)
	}
	return nil
}

// planningPercent 分镜规划进度：随已规划镜数渐近 12。
func planningPercent(index int) int {
	if index <= 0 {
		return 5
	}
	return 5 + index*7/(index+4)
}

// paintingPercent 把绘制进度映射到 12-87。
func paintingPercent(index, total int) int {
	if total <= 0 {
		return 12
	}
	return 12 + index*75/total
}

// countShots 统计成功与失败镜头数。
func countShots(shots []ShotView) (success, failed int) {
	for _, shot := range shots {
		if shot.Status == shotPainted && shot.File != "" {
			success++
			continue
		}
		failed++
	}
	return success, failed
}

// cloneShots 复制分镜视图，避免并发推送时共享底层数组。
func cloneShots(shots []ShotView) []ShotView {
	if len(shots) == 0 {
		return nil
	}
	out := make([]ShotView, len(shots))
	copy(out, shots)
	return out
}

// cloneTaskView 复制任务视图，保证推送与查询拿到独立快照。
func cloneTaskView(state *taskState) TaskView {
	view := state.view
	view.Shots = cloneShots(view.Shots)
	if len(view.Handles) > 0 {
		view.Handles = append([]string(nil), view.Handles...)
	}
	return view
}
