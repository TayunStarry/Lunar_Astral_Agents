// Package EndpointDocs 为各服务提供「自述文档端点」的共享实现。
//
// 各项目在编译时由 build.ps1 调用本模块 cmd/generator，基于源码中的
// SystemEndpoints 注册表生成 OpenAPI 结构的 endpoint_docs.gen.json
// （paths：路径 → 方法 → 操作），再通过 go:embed 嵌入可执行文件并经
// /api-docs 端点对外提供（默认 JSON，?format=html 可视化），
// 保证自述文档始终与服务器实际端点一致（端点增删后重新编译即自动更新）。
package EndpointDocs

import (
	"encoding/json"
	"html/template"
	"net/http"
	"sort"
	"strings"
)

// Handler 返回自述文档端点处理函数：
// GET 默认返回编译时生成的 JSON 文档，?format=html 返回可视化页面
func Handler(docsJSON []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Query().Get("format") == "html" {
			renderHTML(w, docsJSON)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(docsJSON)
	}
}

// pathRow HTML 页面中一行的展示数据（每个路径一行）
type pathRow struct {
	Path    string
	Labels  []string // 方法标签（保留原始方法串，如 ANY / GET/POST/DELETE / WS）
	Summary string
	Handler string
}

// docsPageData 可视化页面模板数据
type docsPageData struct {
	Doc  *Document
	Rows []pathRow
}

// displayMethodOrder HTML 页面中方法的遍历顺序（保证同一文档每次渲染一致）
var displayMethodOrder = []string{"get", "post", "put", "delete", "patch", "head", "options", "trace"}

// docsPageTemplate 自述文档可视化页面模板（html/template 自动转义各字段）
var docsPageTemplate = template.Must(template.New("docs").Funcs(template.FuncMap{
	"methodClass": methodClass,
}).Parse(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Doc.Info.Title}} · 服务自述文档</title>
<style>
body{font-family:"Segoe UI","Microsoft YaHei",system-ui,sans-serif;max-width:1080px;margin:2rem auto;padding:0 1rem;color:#1f2430;background:#f7f8fa}
h1{font-size:1.4rem;margin:0 0 .2rem}
.meta{color:#6b7280;font-size:.9rem;margin-bottom:1.2rem}
table{width:100%;border-collapse:collapse;background:#fff;border-radius:8px;overflow:hidden;box-shadow:0 1px 3px rgba(0,0,0,.08)}
th,td{text-align:left;padding:.55rem .8rem;border-bottom:1px solid #eef0f3;font-size:.92rem;vertical-align:top}
th{background:#f0f2f5}
tr:last-child td{border-bottom:none}
code.path{background:#eef3ff;padding:.1rem .35rem;border-radius:4px;font-size:.88em;word-break:break-all}
.method{display:inline-block;min-width:3.4em;text-align:center;padding:.12rem .4rem;border-radius:4px;font-size:.78rem;font-weight:600;color:#fff;white-space:nowrap;margin:0 .2rem .2rem 0}
.m-GET{background:#2f8f4e}.m-POST{background:#c2831b}.m-DELETE{background:#c0392b}.m-PUT{background:#2f6f9f}.m-ANY{background:#5b6b8c}.m-WS{background:#7a5ea8}.m-HEAD{background:#6b7280}.m-PATCH{background:#9a6a1b}.m-OTHER{background:#8a8f98}
.handler{color:#9aa3b2;font-size:.8rem;word-break:break-all}
footer{margin-top:1rem;color:#9aa3b2;font-size:.8rem}
</style>
</head>
<body>
<h1>{{.Doc.Info.Title}}</h1>
<div class="meta">服务标识 <code>{{.Doc.Info.XService}}</code> · 共 {{len .Doc.Paths}} 个端点</div>
<table>
<tr><th>方法</th><th>路径</th><th>说明</th><th>处理器</th></tr>
{{range .Rows}}<tr>
<td>{{range .Labels}}<span class="method {{methodClass .}}">{{.}}</span> {{end}}</td>
<td><code class="path">{{.Path}}</code></td>
<td>{{.Summary}}</td>
<td class="handler">{{.Handler}}</td>
</tr>
{{end}}</table>
<footer>本文档由编译脚本在编译时基于端点注册表自动生成（OpenAPI {{.Doc.OpenAPI}}，结构化数据请读取本页同路径的 JSON）</footer>
</body>
</html>
`))

// methodClass 把方法标签映射为 CSS 类名（组合方法如 GET/POST/DELETE 取首段归入对应色系）
func methodClass(label string) string {
	name := strings.SplitN(label, "/", 2)[0]
	switch name {
	case "GET", "POST", "DELETE", "PUT", "ANY", "WS", "HEAD", "PATCH":
		return "m-" + name
	default:
		return "m-OTHER"
	}
}

// renderHTML 解析编译时生成的 OpenAPI 文档并渲染可视化页面
func renderHTML(w http.ResponseWriter, docsJSON []byte) {
	var doc Document
	if err := json.Unmarshal(docsJSON, &doc); err != nil {
		http.Error(w, "自述文档数据损坏（编译时生成失败或被篡改）: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// 响应头已写出，模板执行失败无法再改变状态码，忽略错误
	_ = docsPageTemplate.Execute(w, docsPageData{Doc: &doc, Rows: buildPathRows(&doc)})
}

// buildPathRows 把 paths 映射整理为按路径排序、每路径一行的展示数据；
// 同一路径的多个操作折叠为一行，方法标签去重（如 ANY 展开的 7 个操作共享一个 ANY 标签）
func buildPathRows(doc *Document) []pathRow {
	paths := make([]string, 0, len(doc.Paths))
	for path := range doc.Paths {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	rows := make([]pathRow, 0, len(paths))
	for _, path := range paths {
		item := doc.Paths[path]
		row := pathRow{Path: path}
		seen := map[string]bool{}
		for _, key := range displayMethodOrder {
			operation, ok := item[key]
			if !ok {
				continue
			}
			if row.Summary == "" {
				row.Summary = operation.Summary
				row.Handler = operation.XHandler
			}
			label := operation.XMethod
			if label == "" {
				label = strings.ToUpper(key)
			}
			if !seen[label] {
				seen[label] = true
				row.Labels = append(row.Labels, label)
			}
		}
		rows = append(rows, row)
	}
	return rows
}
