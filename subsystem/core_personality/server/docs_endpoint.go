package server

import (
	EndpointDocs "LunarSubsystem/EndpointDocs"
	_ "embed"
)

// endpointDocsJSON 端点自述文档数据（编译时生成并嵌入）
// 由 build.ps1 在 go build 之前调用 subsystem/endpoint_docs/cmd/generator 生成：
// 解析 server/variable.go 中的 SystemEndpoints 注册表提取各端点的路径/方法/描述，
// 端点增删后重新编译文档即自动更新，无需手工维护
//
//go:embed endpoint_docs.gen.json
var endpointDocsJSON []byte

// docsHandler 自述文档端点：默认返回 JSON 文档，?format=html 返回可视化页面
var docsHandler = EndpointDocs.Handler(endpointDocsJSON)
