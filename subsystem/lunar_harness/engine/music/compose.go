package music

// compose.go 音乐渲染：AMT token → 乐谱 → WAV。
//
// 模型调用与显存切换已上移 agents/Musician（见其 generate.go），
// 本包只保留确定性的解码与渲染，不再持有任何模型调度。

import (
	"fmt"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

// ComposeScore 把音乐模型产出的 AMT token 解码为乐谱，并渲染成 WAV。
func ComposeScore(ids []int, brief Brief) (RenderResult, error) {
	score, err := DecodeAMT(ids, brief)
	if err != nil {
		return RenderResult{}, fmt.Errorf("AMT 解码失败（%d tokens）: %w", len(ids), err)
	}
	logger.Info(module, "["+agentinfo.NameMusician+"] AMT 解码完成: %d tokens → %d 声部", len(ids), len(score.Voices))

	rendered, err := Render(score, VoiceSourceModel)
	if err != nil {
		return RenderResult{}, err
	}
	if rendered.AudioURL == "" {
		return RenderResult{}, fmt.Errorf("未生成演奏音频")
	}
	return rendered, nil
}
