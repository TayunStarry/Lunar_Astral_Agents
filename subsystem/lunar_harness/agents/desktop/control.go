//go:build windows

package desktop

// ==== 运行控制（仅控制台「月华链路」屏使用） ====
// 桌面操纵直接操作真实桌面键鼠，存在安全风险，不对月华（对话者）的 LTPX 工具链暴露：
//   - 只能由控制台人工输入要求后启动；
//   - 控制台可随时中断；
//   - 对话者开始推理/应答前，宿主动作会自动调用 Stop 终止其继续运行。

import (
	"context"
	"errors"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// Start 启动一次桌面操纵任务；已有任务运行时拒绝启动。
func Start(instruction string) error {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return errors.New("请先填写要桌面操纵执行的桌面任务")
	}

	runMu.Lock()
	if runState.Running {
		runMu.Unlock()
		return errors.New("桌面操纵正在运行，请先中断当前任务")
	}
	ctx, cancel := context.WithCancel(context.Background())
	runCancel = cancel
	runState = RunState{Running: true, Instruction: instruction, StartedAt: time.Now()}
	runMu.Unlock()

	logger.Info("LunarHarness", "桌面操纵已启动（控制台）：%s", instruction)
	go func() {
		text, err := Run(ctx, instruction)

		runMu.Lock()
		runCancel = nil
		runState.Running = false
		runState.FinishedAt = time.Now()
		if err != nil {
			if errors.Is(err, ErrCanceled) {
				runState.Error = "已中断"
			} else {
				runState.Error = err.Error()
			}
		} else {
			runState.Result = text
		}
		runMu.Unlock()

		switch {
		case err == nil:
			logger.Info("LunarHarness", "桌面操纵执行完成: %s", text)
		case errors.Is(err, ErrCanceled):
			logger.Warn("LunarHarness", "桌面操纵已中断")
		default:
			logger.Error("LunarHarness", "桌面操纵执行失败: %v", err)
		}

		cancel() // 释放上下文资源
	}()
	return nil
}

// Stop 请求中断当前任务并返回是否确有任务在运行（无任务时静默忽略）。
func Stop() bool {
	runMu.Lock()
	cancel := runCancel
	running := runState.Running
	runMu.Unlock()
	if cancel == nil || !running {
		return false
	}
	logger.Warn("LunarHarness", "桌面操纵收到中断请求，正在停止")
	cancel()
	return true
}

// Snapshot 返回当前/最近一次运行状态快照。
func Snapshot() RunState {
	runMu.Lock()
	defer runMu.Unlock()
	return runState
}
