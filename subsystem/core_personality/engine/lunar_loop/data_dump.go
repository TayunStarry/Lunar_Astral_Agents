package lunarloop

import (
	"fmt"
	"strings"

	"github.com/dop251/goja"

	"CorePersonality/engine/lunar_loop/container"
	"LunarSubsystem/LoggerGeneral"
)

// 数据导出（TUI 检修用）：宿主经 dumpForJS 向 goja 运行时索要「月华主上下文」
// 或「某个子智能体的上下文」的 JSON 文本。
//
// 参数：argsJSON 为请求参数（{agent, includeImages}）
// 返回：[内容, 错误信息]（不可导出/运行时报错时内容为空、错误非空）
//
// 注意：必须在 **agent 事件循环线程**上调用（goja 运行时非 goroutine-safe）。

// dumpForJS 调用运行时内的 buildContextDump 导出上下文
func (class *Runtime) dumpForJS(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{"", fmt.Errorf("参数不足，需要 argsJSON")})
	}
	argsJSON, _ := call.Argument(0).Export().(string)

	/** 运行时内的导出入口（agent/data-dump.ts 注入） */
	fnValue := class.runtime.Get("buildContextDump")
	if goja.IsUndefined(fnValue) || goja.IsNull(fnValue) {
		return class.runtime.ToValue([]any{"", fmt.Errorf("导出入口 buildContextDump 未注册（月华智能体脚本未加载或不支持数据导出）")})
	}
	fn, ok := goja.AssertFunction(fnValue)
	if !ok {
		return class.runtime.ToValue([]any{"", fmt.Errorf("导出入口 buildContextDump 不是函数")})
	}

	res, err := fn(goja.Undefined(), class.runtime.ToValue(argsJSON))
	if err != nil {
		LoggerGeneral.Error(lunarCoreModule, "数据导出失败: %v", err)
		return class.runtime.ToValue([]any{"", fmt.Errorf("数据导出失败: %w", err)})
	}
	return class.runtime.ToValue([]any{res.String(), nil})
}

// DumpAgentContext 供宿主（TUI / HTTP 端点）从任意 goroutine 调用：
// 在 agent 事件循环上执行导出并同步取回结果。
//
// agent 取值：'lunar'（主上下文/未读队列）或子智能体名
// （dialogue / painter / musician / perceiver / description）。
func DumpAgentContext(agent string, includeImages bool) (string, error) {
	agent = strings.TrimSpace(agent)
	if agent == "" {
		agent = "lunar"
	}
	if !IsAgentReady() {
		return "", fmt.Errorf("月华智能体运行时就绪中或未启动，暂时无法导出上下文")
	}

	argsJSON := fmt.Sprintf(`{"agent":%q,"includeImages":%t}`, agent, includeImages)
	/** 结果通道（缓冲 1，避免回调无人接收时阻塞事件循环） */
	done := make(chan [2]any, 1)

	if !container.RunOnLoop(func(vm *goja.Runtime) {
		value := vm.Get("buildContextDump")
		if goja.IsUndefined(value) || goja.IsNull(value) {
			done <- [2]any{"", fmt.Errorf("导出入口未注册（智能体脚本未加载）")}
			return
		}
		fn, ok := goja.AssertFunction(value)
		if !ok {
			done <- [2]any{"", fmt.Errorf("导出入口不是函数")}
			return
		}
		// 事件循环线程内直接调用（goja 值不可跨 goroutine 传递）
		res, err := fn(goja.Undefined(), vm.ToValue(argsJSON))
		if err != nil {
			done <- [2]any{"", fmt.Errorf("数据导出失败: %w", err)}
			return
		}
		done <- [2]any{res.String(), nil}
	}) {
		return "", fmt.Errorf("月华智能体运行时未就绪，无法导出上下文")
	}

	out := <-done
	if out[1] != nil {
		return "", out[1].(error)
	}
	return out[0].(string), nil
}
