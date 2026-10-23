package lunarloop

// descriptor.go 描述者（句柄系统）的 goja 适配器
//
// 观影者/感知者已从 goja 层移除：视频与动态图在消息里只留下句柄，
// 真正的内容描述由 Go 侧 agents/Descriptor 现场观看并写回资源目录。
// 文本句柄的自然语言探查同样走这里，由描述者用自己的查询工具分片读取。

import (
	"strings"

	"LunarSubsystem/LunarHarness/agents/Descriptor"

	"github.com/dop251/goja"
)

// descriptorDescribeMedia 取回图片 / 动图 / 视频描述（已有描述直接返回，否则现场观看）
//
// 入参: (handle, question?)  返回值: [描述文本, error]
func (class *Runtime) descriptorDescribeMedia(call goja.FunctionCall) goja.Value {
	handle := strings.TrimSpace(call.Argument(0).String())
	question := ""
	if arg := call.Argument(1); arg != nil && !goja.IsUndefined(arg) && !goja.IsNull(arg) {
		question = strings.TrimSpace(arg.String())
	}
	text, err := Descriptor.DescribeMedia(handle, question)
	return class.runtime.ToValue([]any{text, err})
}

// descriptorInspect 按文本句柄执行自然语言探查
//
// 入参: (handle, request)  返回值: [答复文本, error]
func (class *Runtime) descriptorInspect(call goja.FunctionCall) goja.Value {
	handle := strings.TrimSpace(call.Argument(0).String())
	request := ""
	if arg := call.Argument(1); arg != nil && !goja.IsUndefined(arg) && !goja.IsNull(arg) {
		request = arg.String()
	}
	text, err := Descriptor.InspectText(handle, request)
	return class.runtime.ToValue([]any{text, err})
}
