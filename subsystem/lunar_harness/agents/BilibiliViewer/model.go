package BilibiliViewer

// model.go 哔哩观影的模型调用：统一走 protocol/agentbase 的流式调用、
// 推理实况、循环监管与智能修复；本文件只保留按阶段切换模型与重试策略。

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/model/llama"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
	"LunarSubsystem/LunarHarness/protocol/loopguard"
)

const lifeModelRole = "哔哩观影"

// chatCompletion 带思考监管的模型调用：输出卡循环时中断并重新生成，
// 最多重试 lifeLoopMaxRetries 次；仍失败则返回错误，由调用方跳过本次流程。
func chatCompletion(messages []chatMessage) (string, error) {
	return chatCompletionWith(messages, nil, loopguard.Options{})
}

// chatCompletionWith 与 chatCompletion 相同，额外支持两项按调用定制：
//   - display 非空时，正文不逐段推送，流结束后先规整再送入「推理实况」；
//   - loopOpts 指定重复检测参数（零值字段回退到默认阈值）。
func chatCompletionWith(messages []chatMessage, display func(string) string, loopOpts loopguard.Options) (string, error) {
	return chatCompletionRetry("", messages, display, loopOpts)
}

// isTransientModelError 判断是否为值得重试的瞬时错误：网络失败或 HTTP 5xx。
// 使用结构化类型判断，不再依赖错误文本匹配。
func isTransientModelError(err error) bool {
	if err == nil {
		return false
	}
	var status *agentbase.StatusError
	if errors.As(err, &status) {
		return status.Status >= 500
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

// chatCompletionOnce 单次模型调用（流式输出受思考监管约束）。
func chatCompletionOnce(messages []chatMessage, display func(string) string, loopOpts loopguard.Options) (string, error) {
	return chatCompletionOnceModel("", messages, display, loopOpts)
}

// chatCompletionWithModel 用指定模型执行一次带思考监管的调用。
func chatCompletionWithModel(model string, messages []chatMessage, display func(string) string, loopOpts loopguard.Options) (string, error) {
	return chatCompletionRetry(model, messages, display, loopOpts)
}

// chatCompletionRetry 统一的模型重试：循环输出与瞬时错误分别按各自上限重试。
// model 非空时使用指定模型，并沿用原「模型[name]」日志文案。
func chatCompletionRetry(model string, messages []chatMessage, display func(string) string, loopOpts loopguard.Options) (string, error) {
	named := model != ""
	for attempt := 0; ; attempt++ {
		text, err := chatCompletionOnceModel(model, messages, display, loopOpts)
		if err == nil {
			return text, nil
		}
		if errors.Is(err, agentbase.ErrLoop) {
			if attempt >= lifeLoopMaxRetries {
				if !named {
					logger.Error("BilibiliViewer", "模型连续 %d 次陷入重复输出，跳过本次调用", attempt+1)
				}
				return "", fmt.Errorf("模型连续 %d 次陷入重复输出，已跳过本次调用: %w", attempt+1, agentbase.ErrLoop)
			}
			if named {
				logger.Warn("BilibiliViewer", "模型[%s]重复输出，重新生成（%d/%d）", model, attempt+1, lifeLoopMaxRetries)
			} else {
				logger.Warn("BilibiliViewer", "检测到模型重复输出，已中断并重新生成（重试 %d/%d）", attempt+1, lifeLoopMaxRetries)
			}
			continue
		}
		if isTransientModelError(err) {
			if attempt >= modelHTTPRetries {
				if !named {
					logger.Error("BilibiliViewer", "模型连续 %d 次瞬时错误，跳过本次调用: %v", attempt+1, err)
				}
				return "", err
			}
			wait := time.Duration(attempt+1) * modelRetryBackoff
			if named {
				logger.Warn("BilibiliViewer", "模型[%s]瞬时错误（重试 %d/%d）: %v", model, attempt+1, modelHTTPRetries, err)
			} else {
				logger.Warn("BilibiliViewer", "模型瞬时错误（重试 %d/%d，等待 %v）: %v",
					attempt+1, modelHTTPRetries, wait, err)
			}
			time.Sleep(wait)
			continue
		}
		return "", err
	}
}

// unloadModel 主动卸载指定模型释放显存；失败只告警（下次调用会按需重载）。
func unloadModel(model string) {
	if strings.TrimSpace(model) == "" {
		return
	}
	if _, err := llama.UnloadModelByID(model); err != nil {
		logger.Warn("BilibiliViewer", "卸载模型[%s]失败（不影响流程）: %v", model, err)
		return
	}
}

// chatCompletionOnceModel 指定模型名的单次调用（流式输出受思考监管约束）。
func chatCompletionOnceModel(model string, messages []chatMessage, display func(string) string, loopOpts loopguard.Options) (string, error) {
	reply, err := agentbase.Stream(context.Background(), agentbase.StreamRequest{
		Agent:        "bilibili",
		Role:         lifeModelRole,
		Messages:     messages,
		Model:        model,
		Display:      display,
		DisplayAtEnd: display != nil,
		LoopOptions:  loopOpts,
		Timeout:      5 * time.Minute,
	})
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(reply.Text)
	if text == "" {
		return "", fmt.Errorf("模型返回空内容")
	}
	return text, nil
}

func chatText(system, user string) (string, error) {
	return chatCompletion([]chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	})
}

// chatTextModel 用指定模型跑一次纯文本对话。
func chatTextModel(model, system, user string) (string, error) {
	return chatCompletionWithModel(model, []chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}, nil, loopguard.Options{})
}

// chatWithImagesModel 用指定模型跑一次图像理解（单次，不重试）。
func chatWithImagesModel(model, system, prompt string, images []string) (string, error) {
	return chatCompletionOnceModel(model, imageMessages(system, prompt, images), nil, loopguard.Options{})
}

// chatTextWithDisplay 同 chatText，但正文先按 display 规整再送入实况。
func chatTextWithDisplay(system, user string, display func(string) string) (string, error) {
	return chatCompletionWith([]chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}, display, loopguard.Options{})
}

func chatWithImages(system, prompt string, images []string) (string, error) {
	return chatCompletion(imageMessages(system, prompt, images))
}

// chatWithImagesOnce 单次图像理解（不重试）：思考监管命中重复时直接返回 loopguard.ErrLoop。
func chatWithImagesOnce(system, prompt string, images []string) (string, error) {
	return chatCompletionOnce(imageMessages(system, prompt, images), nil, loopguard.Options{})
}

// imageMessages 组装图像理解请求消息
func imageMessages(system, prompt string, images []string) []chatMessage {
	parts := make([]contentPart, 0, len(images)+1)
	parts = append(parts, contentPart{Type: "text", Text: prompt})
	for _, image := range images {
		if strings.TrimSpace(image) == "" {
			continue
		}
		parts = append(parts, contentPart{Type: "image_url", ImageURL: &imageURL{URL: image}})
	}
	return []chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: parts},
	}
}

func truncate(text string, limit int) string {
	return agentbase.TruncateRunes(text, limit)
}
