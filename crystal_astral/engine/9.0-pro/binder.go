package YaraLTP

// ==== yara 全局对象组装 ====

import (
	"fmt"

	"github.com/dop251/goja"
)

// bindYara 为插件注入 yara 全局 API，并按授权权限集合裁剪未授权的命名空间/方法。
func bindYara(p *plugin) {
	vm := p.vm
	p.granted = p.verifyPermissions()
	yara := vm.NewObject()

	bindLogger(vm, yara)
	bindEvent(p, yara)
	bindHook(p, yara)
	bindEventHandler(p, yara)
	bindCommand(p, yara)
	bindTool(p, yara)
	bindApi(p, yara)
	bindLLMProvider(p, yara)
	bindSend(p, yara)
	bindHTTP(p, yara)
	bindNetwork(vm, yara)
	bindPlatform(vm, yara)
	bindEncoding(vm, yara)
	bindTime(vm, yara)
	bindCrypto(vm, yara)
	bindModel(vm, yara)
	bindConfig(p, yara)
	bindFile(p, yara)
	bindDatabase(vm, yara)
	bindKnowledge(vm, yara)
	bindImage(p, yara)
	bindAsync(p, yara)
	bindEmoji(p, yara)

	pruneYaraByPerms(yara, p.granted)
	vm.Set("yara", yara)
}

// pruneYaraByPerms 依据插件已授权权限集合裁剪 yara 对象：
// 多权限命名空间按方法粒度裁剪，其余命名空间整体移除。未授权 API 呈 undefined，
// 插件脚本若在顶层使用会触发加载失败并给出权限密钥提示。
func pruneYaraByPerms(yara *goja.Object, granted map[string]bool) {
	anyGranted := func(perms ...string) bool {
		for _, perm := range perms {
			if granted[perm] {
				return true
			}
		}
		return false
	}
	// 方法级裁剪（多权限命名空间）
	delMethod := func(ns, m string, perms ...string) {
		if anyGranted(perms...) {
			return
		}
		if obj, ok := yara.Get(ns).(*goja.Object); ok {
			obj.Delete(m)
		}
	}
	delMethod("send", "text", "send.text")
	delMethod("send", "image", "send.image")
	delMethod("send", "emoji", "send.emoji")
	delMethod("send", "hybrid", "send.hybrid")
	delMethod("event", "publish", "event.publish")
	delMethod("api", "call", "api.call")
	delMethod("config", "setFile", "plugin.config.write")
	delMethod("file", "write", "plugin.file.write")
	delMethod("file", "readData", "data.directory.read")
	delMethod("file", "writeData", "data.directory.write")
	delMethod("file", "listData", "data.directory.read")
	delMethod("file", "getDataPath", "data.directory.read")
	delMethod("network", "tcpConnect", "network.tcp")
	delMethod("network", "udpConnect", "network.udp")
	delMethod("network", "udpListen", "network.udp")

	// 命名空间级裁剪
	namespacePerms := map[string][]string{
		"send":         {"send.text", "send.image", "send.emoji", "send.hybrid"},
		"event":        {"event.subscribe", "event.publish"},
		"hook":         {"hook.register"},
		"command":      {"command.register"},
		"tool":         {"tool.register"},
		"eventHandler": {"event_handler.register"},
		"llmProvider":  {"llm_provider.register"},
		"api":          {"api.register", "api.call"},
		"http":         {"http.request"},
		"network":      {"network.tcp", "network.udp"},
		"platform":     {"platform.command"},
		"encoding":     {"encoding.use"},
		"time":         {"time.use"},
		"crypto":       {"crypto.use"},
		"model":        {"model.access"},
		"config":       {"plugin.config.read", "plugin.config.write"},
		"file":         {"plugin.file.read", "plugin.file.write", "data.directory.read", "data.directory.write"},
		"database":     {"database.read"},
		"knowledge":    {"knowledge.search"},
		"image":        {"plugin.file.read"},
		"async":        {"async_task.execute"},
		"emoji":        {"emoji.access"},
	}
	for ns, perms := range namespacePerms {
		if !anyGranted(perms...) {
			yara.Delete(ns)
		}
	}
}

// newObj 在 VM 中创建普通对象。
func newObj(vm *goja.Runtime) *goja.Object { return vm.NewObject() }

// objSetFn 便捷：把 Go 函数绑定到对象键上。
func objSetFn(obj *goja.Object, name string, fn any) { obj.Set(name, fn) }

// argString 读取调用参数并转为字符串（nil → 空串）。
func argString(call goja.FunctionCall, i int) string {
	v := call.Argument(i)
	if goja.IsUndefined(v) || goja.IsNull(v) {
		return ""
	}
	return v.String()
}

// argInt 读取调用参数并转为 int64。
func argInt(call goja.FunctionCall, i int) int64 {
	v := call.Argument(i)
	if goja.IsUndefined(v) || goja.IsNull(v) {
		return 0
	}
	switch n := v.Export().(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	case int:
		return int64(n)
	}
	return 0
}

// argFloat 读取调用参数并转为 float64。
func argFloat(call goja.FunctionCall, i int) float64 {
	v := call.Argument(i)
	if goja.IsUndefined(v) || goja.IsNull(v) {
		return 0
	}
	switch n := v.Export().(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	}
	return 0
}

// argBool 读取调用参数并转为 bool。
func argBool(call goja.FunctionCall, i int) bool {
	v := call.Argument(i)
	if goja.IsUndefined(v) || goja.IsNull(v) {
		return false
	}
	if b, ok := v.Export().(bool); ok {
		return b
	}
	return v.ToBoolean()
}

// argMap 读取调用参数并转为 map[string]any（对象参数）。
func argMap(call goja.FunctionCall, i int) map[string]any {
	v := call.Argument(i)
	if goja.IsUndefined(v) || goja.IsNull(v) {
		return map[string]any{}
	}
	ex := v.Export()
	if m, ok := ex.(map[string]any); ok {
		return m
	}
	if m, ok := ex.(map[string]interface{}); ok {
		return m
	}
	return map[string]any{}
}

// argExport 读取参数并返回其导出的 Go 值。
func argExport(call goja.FunctionCall, i int) any {
	v := call.Argument(i)
	if goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	return v.Export()
}

// mapGetStr 从 map 读取字符串字段。
func mapGetStr(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// mapGetStrSlice 从 map 读取字符串数组字段。
func mapGetStrSlice(m map[string]any, key string) []string {
	if m == nil {
		return nil
	}
	if arr, ok := m[key].([]any); ok {
		out := make([]string, 0, len(arr))
		for _, v := range arr {
			out = append(out, toString(v))
		}
		return out
	}
	return nil
}

// toString 把任意导出值转换为字符串。
func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	case []byte:
		return string(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}