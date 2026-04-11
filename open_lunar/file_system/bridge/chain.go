package bridge

import "time"

// Chain 链式调用支持
func (class *Context) Chain() *ChainContext {
	return &ChainContext{ctx: class}
}

// ChainContext 链式调用上下文
type ChainContext struct {
	ctx *Context
	err error
}

// Load 链式调用 Load 方法
func (class *ChainContext) Load(code string) *ChainContext {
	if class.err != nil {
		return class
	}
	_, class.err = class.ctx.Load(code)
	return class
}

// LoadModule 链式调用 LoadModule 方法
func (class *ChainContext) LoadModule(name, code string) *ChainContext {
	if class.err != nil {
		return class
	}
	class.err = class.ctx.LoadModule(name, code)
	return class
}

// Run 链式调用 Run 方法
func (class *ChainContext) Run(name string, args ...any) *ChainContext {
	if class.err != nil {
		return class
	}
	_, class.err = class.ctx.Run(name, args...)
	return class
}

// RunWithTimeout 链式调用 RunWithTimeout 方法
func (class *ChainContext) RunWithTimeout(name string, timeout time.Duration, args ...any) *ChainContext {
	if class.err != nil {
		return class
	}
	_, class.err = class.ctx.RunWithTimeout(name, timeout, args...)
	return class
}

// Register 链式调用 Register 方法
func (class *ChainContext) Register(name string, function any, async bool) *ChainContext {
	if class.err != nil {
		return class
	}
	class.err = class.ctx.Register(name, function, async)
	return class
}

// SetGlobal 链式调用 SetGlobal 方法
func (class *ChainContext) SetGlobal(name string, value any) *ChainContext {
	if class.err != nil {
		return class
	}
	class.err = class.ctx.SetGlobal(name, value)
	return class
}

// GetGlobal 链式调用 GetGlobal 方法
func (class *ChainContext) GetGlobal(name string) (any, error) {
	if class.err != nil {
		return nil, class.err
	}
	return class.ctx.GetGlobal(name)
}

// Error 获取链式调用过程中的错误
func (class *ChainContext) Error() error {
	return class.err
}
