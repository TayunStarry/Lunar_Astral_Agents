package Creator

// dts_test.go 运行时 API 契约已嵌入二进制：只能读到唯一一份 runtimeAPI.d.ts。

import (
	"strings"
	"testing"
)

func TestReadRuntimeDocFromEmbedded(t *testing.T) {
	docs, err := listDTSDocs()
	if err != nil {
		t.Fatalf("列出文档失败: %v", err)
	}
	if len(docs) != 1 || docs[0].Name != runtimeDocName || docs[0].Size <= 0 {
		t.Fatalf("应只有一份内嵌契约 %s，实得 %+v", runtimeDocName, docs)
	}

	content, err := readDTSDoc(runtimeDocName)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", runtimeDocName, err)
	}
	if !strings.Contains(content, "signal") || !strings.Contains(content, "file") {
		t.Fatalf("契约内容不完整（前 80 字：%.80s）", content)
	}

	// 空名：返回清单而不是报错
	list, err := readDTSDoc("")
	if err != nil || !strings.Contains(list, runtimeDocName) {
		t.Fatalf("空名应返回文档清单: err=%v list=%q", err, list)
	}

	// 不做任何旧称 / 大小写 / 前缀兼容
	for _, name := range []string{
		"runtimeApi.d.ts",
		"runtime_api.d.ts",
		"LTP 9.1 Flash.d.ts",
		"prompts/runtimeAPI.d.ts",
		"engine-implementation.md",
	} {
		if _, err := readDTSDoc(name); err == nil {
			t.Fatalf("非精确文件名 %q 应被拒绝", name)
		}
	}
}
