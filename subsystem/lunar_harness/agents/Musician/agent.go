package Musician

// agent.go 演奏者智能体：构思作品描述 → 引擎生成并渲染音频 → 回传作品介绍。
//
// 本包只保留提示词、编排与介绍文案；模型切换、音乐生成与渲染由 engine/music 承担。

import (
	"context"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/music"
)

// Perform 完成一次演奏：主模型产出描述，音乐模型生成旋律，渲染后回传结果。
func Perform(ctx context.Context, instruction string) (Result, error) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return Result{}, fmt.Errorf("创作描述不能为空")
	}

	brief, err := planBrief(ctx, instruction)
	if err != nil {
		logger.Warn(module, "[演奏者] 音乐描述生成失败，使用兜底描述: %v", err)
		brief = fallbackBrief(instruction)
	}
	brief = normalizeBrief(brief)

	rendered, err := music.Compose(brief)
	if err != nil {
		logger.Error(module, "[演奏者] 音乐生成或渲染失败: %v", err)
		return Result{}, err
	}
	return Result{
		Title:       brief.Title,
		Description: describeBrief(brief),
		Duration:    rendered.DurationSeconds,
		AudioURL:    rendered.AudioURL,
		Summary:     buildSummary(brief),
	}, nil
}
