package Screenshoter

// agent.go 截图者对外入口：一次截图任务的编排。

import (
	"context"
	"errors"
	"strings"
)

// Perform 执行一次截图任务。
//
// 截图者只负责截取与回传：推送与落盘经 Env 注入，视觉副本入库仍由对话者消费。
// 串行执行避免多帧推送与任务之间交叠。
func Perform(ctx context.Context, instruction string, env Env) (Result, error) {
	if strings.TrimSpace(instruction) == "" {
		return Result{}, errors.New("截图要求不能为空")
	}

	stateMu.Lock()
	defer stateMu.Unlock()

	s := &session{
		ctx:     ctx,
		env:     env,
		system:  systemPrompt(),
		history: state.history,
	}
	s.history = append(s.history, baseMessage{Role: "user", Content: instruction})
	if len(s.history) > contextLimit {
		s.history = s.history[len(s.history)-contextLimit:]
	}
	summary := s.run()
	state.history = s.history

	return Result{Summary: summary, Image: s.primary}, nil
}
