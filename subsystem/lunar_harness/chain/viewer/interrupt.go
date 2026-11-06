package viewer

// interrupt.go 观影链的协作式让出。
//
// 观影是后台活动：用户消息到达时，观影在下一个安全边界（两次模型调用之间、
// 播放/发布等待的可中断点）让出，让对话链先回应，回应结束后由生活链的下一回合续跑。
//
// 让出不会推进任何已完成的阶段：已入库的候选、已发布的里程碑都留在持久化状态里，
// 续跑时按状态跳过。

import (
	"errors"
	"sync/atomic"
)

// ErrPaused 本回合在安全边界让出（当前步骤未完成，下一回合从同一点继续）。
var ErrPaused = errors.New("观影阶段让出")

// pauseHook 「是否有用户消息等待回应」的判定入口，由思考链注入；未注入时永不让出。
var pauseHook atomic.Pointer[func() bool]

// SetPauseHook 注入让出判定；传 nil 取消注入。
func SetPauseHook(fn func() bool) {
	if fn == nil {
		pauseHook.Store(nil)
		return
	}
	pauseHook.Store(&fn)
}

// PauseRequested 对外暴露让出判定，供生活链在机会性维护之间设置让出点。
func PauseRequested() bool { return pauseRequested() }

// pauseRequested 报告当前是否需要让出。
func pauseRequested() bool {
	fn := pauseHook.Load()
	return fn != nil && (*fn)()
}
