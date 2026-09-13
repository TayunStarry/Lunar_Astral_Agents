package EndpointDocs

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sampleDocs 模拟编译时生成并嵌入的 OpenAPI 文档 JSON
const sampleDocs = `{
  "openapi": "3.1.0",
  "info": {
    "title": "测试服务",
    "version": "1.0.0",
    "x-service": "TestSvc"
  },
  "paths": {
    "/echo": {
      "post": {
        "summary": "回显 <body> 含 HTML 字符",
        "operationId": "echoHandler",
        "x-handler": "echoHandler",
        "responses": {"200": {"description": "成功"}}
      }
    },
    "/ping": {
      "get": {
        "summary": "探活",
        "operationId": "pingHandler",
        "x-handler": "pingHandler",
        "responses": {"200": {"description": "成功"}}
      }
    }
  }
}`

func TestHandlerServesJSON(t *testing.T) {
	srv := httptest.NewServer(Handler([]byte(sampleDocs)))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("期望状态码 200，实际 %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("期望 Content-Type 为 application/json，实际 %q", got)
	}

	body, _ := io.ReadAll(resp.Body)
	var doc Document
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("响应不是合法 JSON: %v", err)
	}
	if doc.OpenAPI != "3.1.0" || doc.Info.Title != "测试服务" {
		t.Fatalf("文档内容不符: openapi=%q title=%q", doc.OpenAPI, doc.Info.Title)
	}
	ping, ok := doc.Paths["/ping"]["get"]
	if !ok || ping.Summary != "探活" || ping.OperationID != "pingHandler" {
		t.Fatalf("paths 结构不符: %+v", doc.Paths["/ping"])
	}
}

func TestHandlerServesHTML(t *testing.T) {
	srv := httptest.NewServer(Handler([]byte(sampleDocs)))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?format=html")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("期望 Content-Type 为 text/html，实际 %q", got)
	}

	body, _ := io.ReadAll(resp.Body)
	html := string(body)
	for _, want := range []string{"测试服务", "/ping", "/echo", "echoHandler"} {
		if !strings.Contains(html, want) {
			t.Fatalf("页面缺少内容 %q", want)
		}
	}
	// html/template 应把摘要中的 HTML 字符转义为实体，防止注入
	if !strings.Contains(html, "&lt;body&gt;") || strings.Contains(html, "<body> 含") {
		t.Fatal("摘要中的 HTML 字符未被转义")
	}
}

func TestHandlerRejectsNonGet(t *testing.T) {
	srv := httptest.NewServer(Handler([]byte(sampleDocs)))
	defer srv.Close()

	resp, err := http.Post(srv.URL, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("期望状态码 405，实际 %d", resp.StatusCode)
	}
}

func TestHandlerCorruptedPayload(t *testing.T) {
	srv := httptest.NewServer(Handler([]byte("not-json")))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?format=html")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("期望状态码 500，实际 %d", resp.StatusCode)
	}
}

func TestBuildPathRowsSortedAndFolded(t *testing.T) {
	var doc Document
	if err := json.Unmarshal([]byte(sampleDocs), &doc); err != nil {
		t.Fatalf("样例 JSON 解析失败: %v", err)
	}
	rows := buildPathRows(&doc)
	if len(rows) != 2 {
		t.Fatalf("期望 2 行，实际 %d 行", len(rows))
	}
	// 路径按字典序展示：/echo 在 /ping 之前
	if rows[0].Path != "/echo" || rows[1].Path != "/ping" {
		t.Fatalf("路径排序不符: %q, %q", rows[0].Path, rows[1].Path)
	}
	if rows[0].Labels[0] != "POST" {
		t.Fatalf("方法标签不符: %q", rows[0].Labels[0])
	}
}
