package EndpointDocs

import "strings"

// OpenAPIVersion 生成的文档遵循的 OpenAPI 规范版本
const OpenAPIVersion = "3.1.0"

// DefaultAPIVersion 写入 info.version 的默认文档版本号
const DefaultAPIVersion = "1.0.0"

// Endpoint 提取阶段的单个端点条目（源码注册表中的一行）
type Endpoint struct {
	Path        string `json:"path"`
	Method      string `json:"method"`
	Description string `json:"description"`
	Handler     string `json:"handler,omitempty"`
}

// Response 操作响应的占位描述（满足 OpenAPI 规范对操作 responses 的必填要求）
type Response struct {
	Description string `json:"description"`
}

// Operation OpenAPI 操作对象：单个 HTTP 方法下对端点的说明
type Operation struct {
	Summary     string              `json:"summary,omitempty"`     // 端点功能摘要（源码 Description）
	Description string              `json:"description,omitempty"` // 与 summary 相同，兼容只读 description 的消费方
	OperationID string              `json:"operationId,omitempty"` // 处理器标识（同一路径展开多个方法时追加方法后缀保证唯一）
	XHandler    string              `json:"x-handler,omitempty"`   // 源码处理器表达式原文（如 file.SaveHandler）
	XMethod     string              `json:"x-method,omitempty"`    // 原始方法串（ANY/组合方法/WS 时保留，弥补标准方法键的表达力）
	XWebsocket  bool                `json:"x-websocket,omitempty"` // WebSocket 端点标记（升级握手以 HTTP GET 发起，故挂在 get 操作下）
	Responses   map[string]Response `json:"responses"`             // 占位响应描述
}

// PathItem 单个路径下「HTTP 方法 → 操作」的映射
type PathItem map[string]Operation

// Info OpenAPI info 对象
type Info struct {
	Title    string `json:"title"`               // 服务显示名
	Version  string `json:"version"`             // 文档版本
	XService string `json:"x-service,omitempty"` // 服务标识（如 CrystalAstral）
}

// Server OpenAPI servers 条目（同源相对地址）
type Server struct {
	URL string `json:"url"`
}

// ExternalDocs 指向可视化页面的外链
type ExternalDocs struct {
	Description string `json:"description,omitempty"`
	URL         string `json:"url"`
}

// Document OpenAPI 风格的自述文档顶层结构
type Document struct {
	OpenAPI      string              `json:"openapi"`                    // 规范版本
	Info         Info                `json:"info"`                       // 服务信息
	Servers      []Server            `json:"servers,omitempty"`          // 同源访问地址
	ExternalDocs *ExternalDocs       `json:"externalDocs,omitempty"`     // 可视化页面入口
	XSources     []string            `json:"x-generated-from,omitempty"` // 生成文档所解析的源文件
	Paths        map[string]PathItem `json:"paths"`                      // 端点清单：路径 → 方法 → 操作
}

// anyMethodKeys "ANY"（处理器不限定请求方法）展开成的标准方法集合
var anyMethodKeys = []string{"get", "put", "post", "delete", "options", "head", "patch"}

// standardMethodKeys OpenAPI 支持的全部 HTTP 方法操作键
var standardMethodKeys = map[string]bool{
	"get": true, "put": true, "post": true, "delete": true,
	"options": true, "head": true, "patch": true, "trace": true,
}

// operationKeys 把端点的 Method 串映射为 OpenAPI 操作键：
//   - "GET"             → ["get"]
//   - "GET/POST/DELETE" → ["get", "post", "delete"]
//   - "ANY"（或空）     → 全部标准方法
//   - "WS"（WebSocket） → ["get"]（升级握手以 HTTP GET 发起，配合 x-websocket 标记）
func operationKeys(method string) []string {
	normalized := strings.ToUpper(strings.TrimSpace(method))
	switch normalized {
	case "", "ANY":
		return anyMethodKeys
	case "WS":
		return []string{"get"}
	default:
		var keys []string
		for _, part := range strings.Split(normalized, "/") {
			key := strings.ToLower(strings.TrimSpace(part))
			if standardMethodKeys[key] {
				keys = append(keys, key)
			}
		}
		if len(keys) == 0 {
			return []string{"get"}
		}
		return keys
	}
}

// BuildPaths 把端点清单转换为 OpenAPI paths 结构。
// 同一端点的 Method 串可展开为多个方法操作（ANY/组合方法），
// 展开出的多个操作共享摘要与处理器，并以 x-method 保留原始方法串。
func BuildPaths(endpoints []Endpoint) map[string]PathItem {
	paths := make(map[string]PathItem, len(endpoints))
	for _, endpoint := range endpoints {
		item, ok := paths[endpoint.Path]
		if !ok {
			item = PathItem{}
			paths[endpoint.Path] = item
		}
		keys := operationKeys(endpoint.Method)
		for _, key := range keys {
			operationID := endpoint.Handler
			if len(keys) > 1 && operationID != "" {
				// OpenAPI 要求 operationId 全文档唯一，多方法展开时追加方法名
				operationID = operationID + "_" + key
			}
			operation := Operation{
				Summary:     endpoint.Description,
				Description: endpoint.Description,
				OperationID: operationID,
				XHandler:    endpoint.Handler,
				Responses:   map[string]Response{"200": {Description: "成功"}},
			}
			// 原始方法串与操作键不一致（ANY/组合方法/WS）时保留原始信息
			if !strings.EqualFold(strings.TrimSpace(endpoint.Method), key) {
				operation.XMethod = strings.TrimSpace(endpoint.Method)
			}
			if strings.EqualFold(strings.TrimSpace(endpoint.Method), "WS") {
				operation.XWebsocket = true
			}
			item[key] = operation
		}
	}
	return paths
}
