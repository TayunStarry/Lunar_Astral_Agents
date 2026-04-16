package context

import (
	"fmt"
	"log"
	"time"

	"modernc.org/quickjs"
)

// Context 是 QuickJS 虚拟机的封装结构体
type Context struct {
	vm *quickjs.VM
}

// Load 执行指定的 JS 代码
func (class *Context) Load(code string) (any, error) {
	result, err := class.vm.Eval(code, quickjs.EvalGlobal)
	if err != nil {
		return nil, fmt.Errorf("执行 JS 代码失败: %v", err)
	}
	return result, nil
}

// LoadModule 加载 ES 模块
func (class *Context) LoadModule(name, code string) *Context {
	// 实现 ES 模块加载
	moduleCode := fmt.Sprintf(`
		(function() {
			const module = { exports: {} };
			const exports = module.exports;
			%s
			globalThis.%s = module.exports;
		})();
	`, code, name)
	_, err := class.vm.Eval(moduleCode, quickjs.EvalGlobal)
	if err != nil {
		log.Printf("加载模块 %s 失败: %v", name, err)
		return class
	}
	return class
}

// Run 调用指定的 JS 函数 并 传入参数
func (class *Context) Run(name string, args ...any) (any, error) {
	result, err := class.vm.Call(name, args...)
	if err != nil {
		return nil, fmt.Errorf("调用 JS 函数失败: %v", err)
	}
	return result, nil
}

// RunWithTimeout 带超时控制的函数调用
func (class *Context) RunWithTimeout(name string, timeout time.Duration, args ...any) (any, error) {
	resultChan := make(chan struct {
		result any
		err    error
	})

	go func() {
		result, err := class.vm.Call(name, args...)
		resultChan <- struct {
			result any
			err    error
		}{result, err}
	}()

	select {
	case res := <-resultChan:
		if res.err != nil {
			return nil, fmt.Errorf("调用 JS 函数失败: %v", res.err)
		}
		return res.result, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("执行超时")
	}
}

// Register 注册 Go 函数到 JS 环境
func (class *Context) Register(name string, function any, async bool) *Context {
	err := class.vm.RegisterFunc(name, function, async)
	if err != nil {
		log.Printf("注册 %s 函数失败: %v", name, err)
		return class
	}
	return class
}

// SetGlobal 设置全局变量
func (class *Context) SetGlobal(name string, value any) *Context {
	// 使用 Eval 方法设置全局变量
	setCode := fmt.Sprintf("globalThis.%s = %v", name, value)
	_, err := class.vm.Eval(setCode, quickjs.EvalGlobal)
	if err != nil {
		log.Printf("设置全局变量 %s 失败: %v", name, err)
		return class
	}
	return class
}

// GetGlobal 获取全局变量
func (class *Context) GetGlobal(name string) (any, error) {
	// 使用 Eval 方法获取全局变量
	getCode := fmt.Sprintf("globalThis.%s", name)
	result, err := class.vm.Eval(getCode, quickjs.EvalGlobal)
	if err != nil {
		return nil, fmt.Errorf("获取全局变量失败: %v", err)
	}
	return result, nil
}

// Close 关闭 QuickJS 上下文，释放资源
func (class *Context) Close() {
	if class.vm != nil {
		class.vm.Close()
		class.vm = nil
	}
}
