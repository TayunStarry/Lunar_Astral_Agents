package creation

// dts_test.go 运行时 API 契约按能力拆分并嵌入二进制：清单多份、按精确文件名读取。

import (
	"strings"
	"testing"
)

func TestReadRuntimeDocFromEmbedded(t *testing.T) {
	docs, err := listDTSDocs()
	if err != nil {
		t.Fatalf("列出文档失败: %v", err)
	}
	if len(docs) != 10 {
		t.Fatalf("契约应按能力拆分为 10 份，实得 %d: %+v", len(docs), docs)
	}
	// 契约已迁到 engine/runtime/contracts，工具按裸文件名仍须命中每一份。
	for _, doc := range docs {
		if !strings.HasSuffix(doc.Name, "API.d.ts") || doc.Size <= 0 {
			t.Fatalf("契约命名或内容异常: %+v", docs)
		}
		body, rerr := readDTSDoc(doc.Name)
		if rerr != nil || len(body) == 0 {
			t.Fatalf("read_dts 读取 %s 失败: %v", doc.Name, rerr)
		}
		firstLine := strings.SplitN(strings.TrimSpace(body), "\n", 2)[0]
		t.Logf("%-18s %5d 字节 %4d 行 首行: %s", doc.Name, len(body), strings.Count(body, "\n")+1, firstLine)
	}

	content, err := readDTSDoc("runtimeAPI.d.ts")
	if err != nil {
		t.Fatalf("读取 runtimeAPI.d.ts 失败: %v", err)
	}
	if !strings.Contains(content, "fetch") || !strings.Contains(content, "console") {
		t.Fatalf("索引契约内容不完整（前 80 字：%.80s）", content)
	}

	agentDoc, err := readDTSDoc("agentAPI.d.ts")
	if err != nil || !strings.Contains(agentDoc, "chat") || strings.Contains(agentDoc, "embed") {
		t.Fatalf("agent 契约应只含对话与搜索: err=%v", err)
	}

	// 空名：返回清单而不是报错
	list, err := readDTSDoc("")
	if err != nil || !strings.Contains(list, "runtimeAPI.d.ts") || !strings.Contains(list, "agentAPI.d.ts") {
		t.Fatalf("空名应返回文档清单: err=%v list=%q", err, list)
	}

	// 不做任何旧称 / 大小写 / 前缀兼容
	for _, name := range []string{
		"runtimeApi.d.ts",
		"runtime_api.d.ts",
		"prompts/runtimeAPI.d.ts",
		"runtimeAPI/agentAPI.d.ts",
		"prompts/runtimeAPI/agentAPI.d.ts",
		"engine-implementation.md",
	} {
		if _, err := readDTSDoc(name); err == nil {
			t.Fatalf("非精确文件名 %q 应被拒绝", name)
		}
	}
}
