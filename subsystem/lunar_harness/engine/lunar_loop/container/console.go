package container

import (
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"

	"github.com/dop251/goja"
)

// ==== 月华 vm 自定义 console 绑定 ====
// 替换 goja_nodejs 的 console.Enable：JS 层 console 输出不再直写 stdout，
// 而是接入 logger 日志管线（模块 LunarCore / 子标签 JavaScript），
// 由 TuiSystem 统一路由到「月华 JS 日志」视图，并同步进入原始日志与警告错误视图。

// bindLunarConsole 构建沙箱全局 console 对象（log/info/warn/error/debug）
func bindLunarConsole(vm *goja.Runtime) *goja.Object {
	o := vm.NewObject()
	o.Set("log", func(args ...any) { lunarConsolePrint("log", args...) })
	o.Set("info", func(args ...any) { lunarConsolePrint("info", args...) })
	o.Set("warn", func(args ...any) { lunarConsolePrint("warn", args...) })
	o.Set("error", func(args ...any) { lunarConsolePrint("error", args...) })
	o.Set("debug", func(args ...any) { lunarConsolePrint("debug", args...) })
	return o
}

// lunarConsolePrint 控制台输出按级别路由到日志管线（保留 [console.*] 标记便于溯源）
func lunarConsolePrint(level string, args ...any) {
	msg := "[console." + level + "] " + joinConsoleArgs(args...)
	switch level {
	case "warn":
		logger.SubWarn(lunarCoreModule, lunarJavaScriptSub, "%s", msg)
	case "error":
		logger.SubError(lunarCoreModule, lunarJavaScriptSub, "%s", msg)
	default:
		logger.SubInfo(lunarCoreModule, lunarJavaScriptSub, "%s", msg)
	}
}

// joinConsoleArgs 拼接 console 参数：字符串原样保留，其余值转 JSON 便于阅读
func joinConsoleArgs(args ...any) string {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		switch v := a.(type) {
		case string:
			parts = append(parts, v)
		case nil:
			parts = append(parts, "null")
		default:
			if b, err := json.Marshal(v); err == nil {
				parts = append(parts, string(b))
			} else {
				parts = append(parts, fmt.Sprint(v))
			}
		}
	}
	return strings.Join(parts, " ")
}
