package tickpermit

// controller.go 特许运行的状态机与有界等待。

import "time"

// New 创建独立控制器（测试与多实例场景使用）。
func New() *Controller {
	return &Controller{changed: make(chan struct{})}
}

// Begin 申请一次特许运行；返回的凭据用于幂等释放。
func (c *Controller) Begin(owner string, ttl time.Duration) *Permit {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	if owner == "" {
		owner = DefaultOwner
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	c.active = true
	c.owner = owner
	c.expiresAt = time.Now().Add(ttl)
	generation := c.generation
	c.notifyLocked()
	return &Permit{controller: c, generation: generation}
}

// End 撤销当前特许并等待在途响应步骤结束。
// 返回 false 表示等待超时；此时许可仍会被撤销，系统不会因响应链卡死而锁死。
func (c *Controller) End(timeout time.Duration) bool {
	c.mu.Lock()
	generation := c.generation
	c.mu.Unlock()
	return c.endGeneration(generation, timeout)
}

// Release 幂等释放本凭据；若期间已有更新的凭据，则不清除新状态。
func (p *Permit) Release() {
	if p == nil || p.controller == nil {
		return
	}
	p.controller.endGeneration(p.generation, EndWaitTimeout)
}

// Active 报告当前是否存在有效特许。
func (c *Controller) Active() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.active
}

// Owner 返回当前特许归属。
func (c *Controller) Owner() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.owner
}

// ExpireIfNeeded 过期回收；返回是否回收及原归属。
func (c *Controller) ExpireIfNeeded(now time.Time) (bool, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.active || c.expiresAt.IsZero() || !now.After(c.expiresAt) {
		return false, ""
	}
	owner := c.owner
	c.active = false
	c.owner = ""
	c.expiresAt = time.Time{}
	c.notifyLocked()
	return true, owner
}

// ForceClear 无条件回收特许（孤儿许可、恢复路径使用）。
func (c *Controller) ForceClear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.active && !c.responseRunning {
		return
	}
	c.active = false
	c.owner = ""
	c.expiresAt = time.Time{}
	c.notifyLocked()
}

// MarkResponse 标记响应链步骤开始/结束；End 的返回屏障依赖该状态。
func (c *Controller) MarkResponse(running bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.responseRunning == running {
		return
	}
	c.responseRunning = running
	c.notifyLocked()
}

// TryBeginResponse 在特许仍有效时原子地标记响应步骤开始。
//
// 返回 false 表示特许已被撤销（或已有响应在途），调用方不得再启动响应步骤。
// 该判定与 End 的撤销在同一把锁内完成，保证「End 返回后不会再有新的响应步骤」。
func (c *Controller) TryBeginResponse() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.active || c.responseRunning {
		return false
	}
	c.responseRunning = true
	return true
}

// ResponseRunning 报告是否有响应链步骤在途。
func (c *Controller) ResponseRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.responseRunning
}

// Status 返回一次性快照，供调度器决策与日志使用。
func (c *Controller) Status() (active bool, owner string, expiresAt time.Time, responseRunning bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.active, c.owner, c.expiresAt, c.responseRunning
}

// endGeneration 只释放匹配代次的许可，随后有界等待响应步骤结束。
func (c *Controller) endGeneration(generation uint64, timeout time.Duration) bool {
	if timeout <= 0 {
		timeout = EndWaitTimeout
	}
	deadline := time.Now().Add(timeout)

	c.mu.Lock()
	if generation != 0 && generation == c.generation {
		c.active = false
		c.owner = ""
		c.expiresAt = time.Time{}
		c.notifyLocked()
	}
	for c.responseRunning {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			c.mu.Unlock()
			return false
		}
		changed := c.changed
		c.mu.Unlock()
		timer := time.NewTimer(remaining)
		select {
		case <-changed:
		case <-timer.C:
		}
		timer.Stop()
		c.mu.Lock()
	}
	c.mu.Unlock()
	return true
}

// notifyLocked 广播状态变化；调用方必须持有锁。
func (c *Controller) notifyLocked() {
	close(c.changed)
	c.changed = make(chan struct{})
}
