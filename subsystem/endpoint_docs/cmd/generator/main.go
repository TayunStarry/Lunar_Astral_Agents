// 端点自述文档生成器 —— 在编译时由各项目 build.ps1 调用（go run ./cmd/generator）。
//
// 解析项目源码中登记全部 HTTP 端点的注册表变量（SystemEndpoints），提取每个端点的
// Path/Method/Description/Handler，生成 OpenAPI 结构（paths：路径 → 方法 → 操作）的
// endpoint_docs.gen.json。该 JSON 由各项目通过 go:embed 嵌入可执行文件并经 /api-docs
// 端点对外提供，保证自述文档与服务器实际端点一致：端点增删后重新编译，文档即自动更新。
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	EndpointDocs "LunarSubsystem/EndpointDocs"
)

// extraFlags 接收重复的 -extra 参数（METHOD|PATH|DESCRIPTION）。
// 用于登记源码注册表变量之外、在运行时另行注册的端点（如 WebSocket /ws，
// 其处理器依赖启动期初始化的实例，无法进入包级 SystemEndpoints 字面量）。
type extraFlags []EndpointDocs.Endpoint

func (e *extraFlags) String() string { return "METHOD|PATH|DESCRIPTION" }

func (e *extraFlags) Set(value string) error {
	parts := strings.SplitN(value, "|", 3)
	if len(parts) != 3 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return fmt.Errorf("-extra 需要格式 METHOD|PATH|DESCRIPTION，收到: %q", value)
	}
	*e = append(*e, EndpointDocs.Endpoint{Method: parts[0], Path: parts[1], Description: parts[2]})
	return nil
}

func main() {
	var (
		service  = flag.String("service", "", "服务标识（写入 info.x-service，必填）")
		display  = flag.String("display", "", "服务显示名（写入 info.title，缺省取 -service）")
		src      = flag.String("src", "", "登记端点注册表的 Go 源文件路径（必填）")
		varName  = flag.String("var", "SystemEndpoints", "端点注册表变量名")
		out      = flag.String("out", "", "输出 JSON 路径（必填）")
		docsPath = flag.String("docs-path", "", "自述文档端点自身路径（写入 externalDocs，便于消费方发现可视化页面）")
	)
	var extras extraFlags
	flag.Var(&extras, "extra", "额外登记的端点（格式 METHOD|PATH|DESCRIPTION，可重复传入）")
	flag.Parse()

	if *service == "" || *src == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "-service、-src、-out 均为必填")
		flag.Usage()
		os.Exit(2)
	}

	endpoints, err := extractEndpoints(*src, *varName)
	if err != nil {
		fail("解析端点注册表失败: %v", err)
	}

	// 追加注册表之外的端点；路径重复时跳过（以注册表为准，保证重复生成结果稳定）
	for _, extra := range extras {
		if containsEndpoint(endpoints, extra.Path) {
			continue
		}
		endpoints = append(endpoints, extra)
	}

	doc := EndpointDocs.Document{
		OpenAPI: EndpointDocs.OpenAPIVersion,
		Info: EndpointDocs.Info{
			Title:    *display,
			Version:  EndpointDocs.DefaultAPIVersion,
			XService: *service,
		},
		Servers:  []EndpointDocs.Server{{URL: "/"}},
		XSources: []string{filepath.Base(*src)},
		Paths:    EndpointDocs.BuildPaths(endpoints),
	}
	if doc.Info.Title == "" {
		doc.Info.Title = *service
	}
	if *docsPath != "" {
		doc.ExternalDocs = &EndpointDocs.ExternalDocs{
			Description: "可视化文档页面",
			URL:         *docsPath + "?format=html",
		}
	}

	if err := writeJSON(*out, doc); err != nil {
		fail("写出文档失败: %v", err)
	}
	fmt.Printf("✓ 端点自述文档已生成: %s（%d 个路径）\n", *out, len(doc.Paths))
}

// extractEndpoints 解析 Go 源文件，提取形如
//
//	var <varName> = []SystemEndpoint{{Path: "...", Handler: x, Method: "...", Description: "..."}, ...}
//
// 的端点注册表，按源码顺序返回各端点信息
func extractEndpoints(srcFile, varName string) ([]EndpointDocs.Endpoint, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, srcFile, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.VAR {
			continue
		}
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok || len(valueSpec.Names) == 0 || valueSpec.Names[0].Name != varName || len(valueSpec.Values) == 0 {
				continue
			}
			composite, ok := valueSpec.Values[0].(*ast.CompositeLit)
			if !ok {
				return nil, fmt.Errorf("%s: 变量 %s 的初始化表达式不是复合字面量", fset.Position(valueSpec.Pos()), varName)
			}
			var endpoints []EndpointDocs.Endpoint
			for _, elt := range composite.Elts {
				endpoint, err := parseEndpoint(fset, elt)
				if err != nil {
					return nil, err
				}
				endpoints = append(endpoints, endpoint)
			}
			return endpoints, nil
		}
	}
	return nil, fmt.Errorf("未找到端点注册表变量 %s（文件: %s）", varName, srcFile)
}

// parseEndpoint 解析单个 {Path: ..., Handler: ..., Method: ..., Description: ...} 结构体字面量
func parseEndpoint(fset *token.FileSet, elt ast.Expr) (EndpointDocs.Endpoint, error) {
	literal, ok := elt.(*ast.CompositeLit)
	if !ok {
		return EndpointDocs.Endpoint{}, fmt.Errorf("%s: 注册表元素不是结构体字面量", fset.Position(elt.Pos()))
	}
	var endpoint EndpointDocs.Endpoint
	for _, item := range literal.Elts {
		keyValue, ok := item.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := keyValue.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "Path":
			endpoint.Path = stringOf(fset, keyValue.Value)
		case "Method":
			endpoint.Method = stringOf(fset, keyValue.Value)
		case "Description":
			endpoint.Description = stringOf(fset, keyValue.Value)
		case "Handler":
			endpoint.Handler = types.ExprString(keyValue.Value)
		}
	}
	if endpoint.Path == "" {
		return EndpointDocs.Endpoint{}, fmt.Errorf("%s: 端点缺少 Path 字段", fset.Position(elt.Pos()))
	}
	return endpoint, nil
}

// stringOf 提取字符串字面量的实际值（处理转义序列）；非常量表达式原样记录其源码形式
func stringOf(fset *token.FileSet, expr ast.Expr) string {
	literal, ok := expr.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return types.ExprString(expr)
	}
	value, err := strconv.Unquote(literal.Value)
	if err != nil {
		return literal.Value
	}
	return value
}

// containsEndpoint 判断端点清单中是否已存在指定路径
func containsEndpoint(endpoints []EndpointDocs.Endpoint, path string) bool {
	for _, endpoint := range endpoints {
		if endpoint.Path == path {
			return true
		}
	}
	return false
}

// writeJSON 以稳定格式写出文档（禁用 HTML 转义、固定缩进，保证重复构建输出一致、可提交入库）
func writeJSON(outPath string, doc EndpointDocs.Document) error {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(doc); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outPath, buffer.Bytes(), 0o644)
}

// fail 输出错误并退出（编译脚本依据退出码判定生成失败）
func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[gen_endpoint_docs] "+format+"\n", args...)
	os.Exit(1)
}
