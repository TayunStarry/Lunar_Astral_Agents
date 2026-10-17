package lunarloop

// goja_export_test.go 确认 goja 从 JS 对象导出数值时的 Go 类型
//
// 背景：generateImage 适配器原先只认 float64，一旦 goja 给出的是 int64 就会静默取 0，
// 导致调用方传入的 1024x1536 变成"未指定"，引擎退回默认 1024x1024 方形。
// 这个测试把 goja 的真实行为钉死，避免再靠猜。

import (
	"testing"

	"github.com/dop251/goja"
)

// TestGojaNumericExportType 打印 goja 对 JS 数值的导出类型，并断言 asInt/asFloat 都能正确取值
func TestGojaNumericExportType(t *testing.T) {
	vm := goja.New()
	value, err := vm.RunString(`({ w: 1024, h: 1536, st: 0.5, s: 40 })`)
	if err != nil {
		t.Fatalf("执行 JS 失败: %v", err)
	}
	exported, ok := value.Export().(map[string]any)
	if !ok {
		t.Fatalf("对象导出类型不是 map[string]any: %T", value.Export())
	}
	t.Logf("goja 导出类型: w=%T h=%T st=%T s=%T", exported["w"], exported["h"], exported["st"], exported["s"])

	// 无论 goja 给的是 float64 还是 int64，取值都必须正确
	if got := asInt(exported["w"]); got != 1024 {
		t.Errorf("asInt(width) = %d，期望 1024（类型 %T）", got, exported["w"])
	}
	if got := asInt(exported["h"]); got != 1536 {
		t.Errorf("asInt(height) = %d，期望 1536（类型 %T）", got, exported["h"])
	}
	if got := asFloat(exported["st"]); got != 0.5 {
		t.Errorf("asFloat(strength) = %v，期望 0.5（类型 %T）", got, exported["st"])
	}
	if got := asInt(exported["s"]); got != 40 {
		t.Errorf("asInt(steps) = %d，期望 40（类型 %T）", got, exported["s"])
	}
}
