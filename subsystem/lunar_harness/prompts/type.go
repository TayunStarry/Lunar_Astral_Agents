package prompts

// type.go 提示词包的类型定义集中管理。

// DTSDoc 一份运行时 API 契约文档（名称与字节数），供按需查阅时列出清单。
type DTSDoc struct {
	Name string
	Size int64
}
