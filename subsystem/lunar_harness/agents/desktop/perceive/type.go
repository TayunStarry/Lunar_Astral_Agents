//go:build windows

package perceive

// type.go 「桌面感知」成员的类型定义。

// treeResult UIA 树读取结果（用于带超时调用）。
type treeResult struct {
	tree string // 原始 UI 树文本
	ok   bool   // 是否成功读取
}
