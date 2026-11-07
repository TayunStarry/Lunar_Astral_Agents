package Musician

// generate.go 演奏者的音乐模型生成阶段：显存互斥切换 → 多候选 AMT token 生成 → 恢复主模型。
//
// 该阶段从 engine/music 上移：模型调度属于智能体职责（租约、加载/卸载、超时与上报），
// engine/music 只保留确定性的解码、体检与渲染。
//
// MIDI-LLM 是 1.4B 专用 text-to-MIDI 模型，单次采样方差很大；因此每次演奏生成有限个
// 候选，用 engine/music 的客观体检挑选通过项，未通过时把诊断追加进描述后重写。

import (
	"context"
	"fmt"
	"math"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/model/llama"
	"LunarSubsystem/LunarHarness/engine/music"
	"LunarSubsystem/LunarHarness/protocol/modellease"
)

// generateBestTokens 卸载主模型 → 加载 MIDI-LLM → 生成若干候选并选出体检最优者 → 恢复主模型。
func generateBestTokens(brief music.Brief) ([]int, music.Review, error) {
	// 全局切换租约：整个「卸载主模型 → 音乐模型 → 恢复主模型」窗口内阻塞其他模型调用。
	release := modellease.AcquireSwap()
	defer release()
	// 无论成功、失败、超时甚至 panic，都会卸载音乐模型并恢复主模型。
	defer restoreModels()

	if _, err := llama.UnloadModelByID(config.AgentMultimodalModel); err != nil {
		logger.Warn(module, "["+roleLabel+"] 卸载主模型失败（继续）: %v", err)
	}
	// 卸载是异步的：必须等主模型真正退出显存，再让路由服务器加载音乐模型。
	if err := llama.WaitModelGone(config.AgentMultimodalModel, modelSwapWait); err != nil {
		logger.Warn(module, "["+roleLabel+"] 主模型释放显存超时: %v", err)
	}

	if err := llama.LoadModelByID(config.MusicGenerationModel); err != nil {
		return nil, music.Review{}, fmt.Errorf("加载音乐模型失败: %w", err)
	}
	if err := llama.WaitModelLoaded(config.MusicGenerationModel, modelLoadWait); err != nil {
		return nil, music.Review{}, fmt.Errorf("等待音乐模型就绪失败: %w", err)
	}

	description := music.BriefDescription(brief)
	baseSeed := time.Now().UnixNano() % 1_000_000_000
	var best []int
	var bestReview music.Review
	bestScore := math.Inf(1)

	for attempt := 0; attempt < candidateCount; attempt++ {
		descriptionAttempt := description
		temperature, topP := candidateTemperature, candidateTopP
		if attempt > 0 {
			// 带诊断重写：把上一轮体检问题翻译成英文约束，追加在描述后面
			if hint := music.Diagnosis(bestReview); hint != "" {
				descriptionAttempt = description + " " + hint
			}
			temperature, topP = retryTemperature, retryTopP
		}
		ctx, cancel := context.WithTimeout(context.Background(), generationTimeout())
		start := time.Now()
		ids, err := llama.CompletionTokens(
			ctx, config.MusicGenerationModel, music.ComposePrompt(descriptionAttempt), musicTokenBudget,
			llama.TokenOptions{
				Temperature: temperature,
				TopP:        topP,
				Seed:        baseSeed + int64(attempt*7919),
			},
		)
		cancel()
		if err != nil {
			logger.Warn(module, "["+roleLabel+"] 候选 %d 生成失败: %v", attempt+1, err)
			continue
		}
		review := music.ReviewTokens(ids, brief)
		logger.Info(module, "["+roleLabel+"] 候选 %d 体检: %s（%.1f 秒）", attempt+1, review.String(), time.Since(start).Seconds())
		if review.Score < bestScore {
			best, bestReview, bestScore = ids, review, review.Score
		}
		if len(review.Issues) == 0 {
			break
		}
	}
	if len(best) == 0 {
		return nil, music.Review{}, fmt.Errorf("音乐模型未产出可用旋律")
	}
	return best, bestReview, nil
}

// generationTimeout 单次候选生成超时。
func generationTimeout() time.Duration {
	if config.MusicGenerationTimeout > 0 {
		return config.MusicGenerationTimeout
	}
	return 3 * time.Minute
}

// restoreModels 卸载音乐模型并触发主模型恢复（幂等，失败只告警）。
func restoreModels() {
	if _, err := llama.UnloadModelByID(config.MusicGenerationModel); err != nil {
		logger.Warn(module, "["+roleLabel+"] 卸载音乐模型失败: %v", err)
	}
	// 等音乐模型真正退出显存后再恢复主模型；LoadModelByID 内部还会对 400 做短重试。
	if err := llama.WaitModelGone(config.MusicGenerationModel, modelSwapWait); err != nil {
		logger.Warn(module, "["+roleLabel+"] 音乐模型释放显存超时: %v", err)
	}
	if err := llama.LoadModelByID(config.AgentMultimodalModel); err != nil {
		logger.Warn(module, "["+roleLabel+"] 恢复主模型失败（下次调用会自动加载）: %v", err)
	}
}
