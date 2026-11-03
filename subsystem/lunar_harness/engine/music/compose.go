package music

// compose.go 音乐生成链路的引擎实现：模型互斥切换、AMT token 生成、解码与渲染。
//
// 主模型与音乐模型不并发驻留：AcquireSwap 期间其他模型调用全部等待；
// 无论生成成功、失败还是超时，defer 都会卸载音乐模型并恢复主模型。

import (
	"context"
	"fmt"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/model/llama"
	"LunarSubsystem/LunarHarness/protocol/modellease"
)

// Compose 完成一次演奏：音乐模型生成 AMT token → 解码为标准乐谱 → 渲染 WAV。
func Compose(brief Brief) (RenderResult, error) {
	ids, err := generateTokens(brief)
	if err != nil {
		return RenderResult{}, err
	}
	score, err := DecodeAMT(ids, brief)
	if err != nil {
		return RenderResult{}, fmt.Errorf("AMT 解码失败（%d tokens）: %w", len(ids), err)
	}
	logger.Info(module, "[演奏者] AMT 解码完成: %d tokens → %d 声部", len(ids), len(score.Voices))

	rendered, err := Render(score, VoiceSourceModel)
	if err != nil {
		return RenderResult{}, err
	}
	if rendered.AudioURL == "" {
		return RenderResult{}, fmt.Errorf("未生成演奏音频")
	}
	return rendered, nil
}

// generateTokens 阶段二：卸载主模型 → 加载 MIDI-LLM → 生成 AMT token → 恢复主模型。
func generateTokens(brief Brief) ([]int, error) {
	// 全局切换租约：整个「卸载主模型 → 音乐模型 → 恢复主模型」窗口内阻塞其他模型调用。
	release := modellease.AcquireSwap()
	defer release()
	// 无论成功、失败、超时甚至 panic，都会卸载音乐模型并恢复主模型。
	defer restoreModels()

	if _, err := llama.UnloadModelByID(config.AgentMultimodalModel); err != nil {
		logger.Warn(module, "[演奏者] 卸载主模型失败（继续）: %v", err)
	}
	// 卸载是异步的：必须等主模型真正退出显存，再让路由服务器加载音乐模型。
	if err := llama.WaitModelGone(config.AgentMultimodalModel, modelSwapWait); err != nil {
		logger.Warn(module, "[演奏者] 主模型释放显存超时: %v", err)
	}

	if err := llama.LoadModelByID(config.MusicGenerationModel); err != nil {
		return nil, fmt.Errorf("加载音乐模型失败: %w", err)
	}
	if err := llama.WaitModelLoaded(config.MusicGenerationModel, modelLoadWait); err != nil {
		return nil, fmt.Errorf("等待音乐模型就绪失败: %w", err)
	}

	timeout := config.MusicGenerationTimeout
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	ids, err := llama.CompletionTokens(ctx, config.MusicGenerationModel, ComposePrompt(BriefDescription(brief)), musicTokenBudget)
	if err != nil {
		return nil, err
	}
	logger.Info(module, "[演奏者] MIDI-LLM 生成完成: %d tokens, 耗时 %.1f 秒", len(ids), time.Since(start).Seconds())
	return ids, nil
}

// restoreModels 卸载音乐模型并触发主模型恢复（幂等，失败只告警）。
func restoreModels() {
	if _, err := llama.UnloadModelByID(config.MusicGenerationModel); err != nil {
		logger.Warn(module, "[演奏者] 卸载音乐模型失败: %v", err)
	}
	// 等音乐模型真正退出显存后再恢复主模型；LoadModelByID 内部还会对 400 做短重试。
	if err := llama.WaitModelGone(config.MusicGenerationModel, modelSwapWait); err != nil {
		logger.Warn(module, "[演奏者] 音乐模型释放显存超时: %v", err)
	}
	if err := llama.LoadModelByID(config.AgentMultimodalModel); err != nil {
		logger.Warn(module, "[演奏者] 恢复主模型失败（下次调用会自动加载）: %v", err)
	}
}
