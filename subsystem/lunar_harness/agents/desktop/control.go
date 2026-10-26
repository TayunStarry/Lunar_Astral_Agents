//go:build windows

package desktop

// ==== 运行控制（控制台「运行状态」屏使用） ====
// 桌面操纵直接操作真实桌面键鼠，存在安全风险，不对月华（对话者）的 LTPX 工具链暴露：
//   - 只能由控制台人工输入要求后启动；
//   - 控制台可随时中断；
//   - 对话者开始推理/应答前，宿主动作会自动调用 Stop 终止其继续运行。
//
// 中断语义：
//   - 第一次 Stop：取消上下文并等待任务自行退出（最多 stopWaitTimeout）；
//   - 等待期间再按一次 Stop：立即强制收尾，允许重新启动新任务；
//   - 等待超时：自动强制收尾，避免界面永远卡在「运行中」。

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
	runSeq++
	seq := runSeq
	done := make(chan struct{})
	runCancel = cancel
	runDone = done
	runState = RunState{Running: true, Instruction: instruction, StartedAt: time.Now()}
	runMu.Unlock()

	logger.Info("LunarHarness", "桌面操纵已启动（控制台）：%s", instruction)
	go func() {
		text, err := Run(ctx, instruction)

		runMu.Lock()
		if runSeq != seq {
			// 已被强制收尾或被新一代任务取代：不再覆盖当前状态。
			runMu.Unlock()
			cancel()
			return
		}
		runCancel = nil
		runState.Running = false
		runState.Stopping = false
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
		close(done)
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

// Stop 请求中断当前任务并返回是否确有任务在处理。
//
// 第一次调用只发出取消；若任务在 stopWaitTimeout 内没有退出，自动强制收尾。
// 已经处于「停止中」时再次调用会立即强制收尾，保证界面与状态可恢复。
func Stop() bool {
	runMu.Lock()
	if !runState.Running {
		runMu.Unlock()
		return false
	}
	if runState.Stopping {
		forceFinishLocked("已中断（强制结束）")
		runMu.Unlock()
		logger.Warn("LunarHarness", "桌面操纵被强制结束（第二次中断请求）")
		return true
	}
	cancel := runCancel
	done := runDone
	seq := runSeq
	runState.Stopping = true
	runMu.Unlock()

	logger.Warn("LunarHarness", "桌面操纵收到中断请求，正在停止")
	if cancel != nil {
		cancel()
	}
	if done != nil {
		go waitDesktopStop(seq, done)
	}
	return true
}

// waitDesktopStop 等待任务退出；超时则强制收尾。
func waitDesktopStop(seq int64, done chan struct{}) {
	select {
	case <-done:
		return
	case <-time.After(stopWaitTimeout):
	}

	runMu.Lock()
	if runSeq == seq && runState.Running {
		forceFinishLocked("已中断（超时强制结束）")
		logger.Warn("LunarHarness", "桌面操纵未在 %s 内退出，已强制收尾", stopWaitTimeout)
	}
	runMu.Unlock()
}

// forceFinishLocked 强制收尾（调用方需持有 runMu）：递增代次，让旧任务迟到的结果失效。
func forceFinishLocked(reason string) {
	runState.Running = false
	runState.Stopping = false
	runState.FinishedAt = time.Now()
	runState.Result = ""
	runState.Error = reason
	runCancel = nil
	runSeq++
}

// Snapshot 返回当前/最近一次运行状态快照。
func Snapshot() RunState {
	runMu.Lock()
	defer runMu.Unlock()
	return runState
}
