package server

import (
	_ "embed"
	"encoding/json"

	EndpointDocs "LunarSubsystem/EndpointDocs"
)

// endpointDocsJSON 端点自述文档数据（编译时生成并嵌入）
// 由 build.ps1 在 go build 之前调用 subsystem/endpoint_docs/cmd/generator 生成：
// 解析 server/variable.go 中的 SystemEndpoints 注册表提取各端点的路径/方法/描述，
// 端点增删后重新编译文档即自动更新，无需手工维护
//
//go:embed endpoint_docs.gen.json
var endpointDocsJSON []byte

// docsVersion 本次构建的版本号（取自自述文档 info.version，即生成时刻的时间戳）
var docsVersion = parseDocsVersion(endpointDocsJSON)

// DocsVersion 返回本次编译产物的版本号（如 2026.291.0847；解析失败时为空串）
func DocsVersion() string {
	return docsVersion
}

// parseDocsVersion 从自述文档 JSON 中提取 info.version
func parseDocsVersion(data []byte) string {
	var doc EndpointDocs.Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return ""
	}
	return doc.Info.Version
}

// docsHandler 自述文档端点：默认返回 JSON 文档，?format=html 返回可视化页面
var docsHandler = EndpointDocs.Handler(endpointDocsJSON)
