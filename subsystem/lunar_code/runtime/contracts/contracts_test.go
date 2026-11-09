package contracts

// contracts_test.go 运行时 API 契约的拆分、读取语义与内容约束：
// 各 *API.d.ts 均可按裸文件名读取；带目录前缀的写法必须被拒绝；
// 契约不再包含权限描述、同步休眠、动态新建 SQL 库能力。

import (
	"strings"
	"testing"
)

func TestListDTSModules(t *testing.T) {
	docs, err := ListDTS()
	if err != nil {
		t.Fatalf("列出契约失败: %v", err)
	}
	want := []string{
		"runtimeAPI.d.ts", "commonAPI.d.ts", "fileAPI.d.ts",
		"imageAPI.d.ts", "videoAPI.d.ts", "memoryAPI.d.ts", "databaseAPI.d.ts",
		"encodingAPI.d.ts", "agentAPI.d.ts", "networkAPI.d.ts",
	}
	if len(docs) != len(want) {
		t.Fatalf("契约文件数 = %d，期望 %d：%+v", len(docs), len(want), docs)
	}
	for _, name := range want {
		content, rerr := Read(name)
		if rerr != nil || len(content) == 0 {
			t.Fatalf("读取 %s 失败: %v", name, rerr)
		}
		firstLine := strings.SplitN(strings.TrimSpace(string(content)), "\n", 2)[0]
		t.Logf("%-18s %5d 字节 %4d 行 首行: %s", name, len(content), strings.Count(string(content), "\n")+1, firstLine)
	}
}

func TestReadRejectsPathPrefixedName(t *testing.T) {
	// read_dts 的既有语义：只接受精确裸文件名，任何路径前缀都必须被拒绝。
	for _, name := range []string{"runtimeAPI/agentAPI.d.ts", "prompts/runtimeAPI.d.ts", "../contracts/agentAPI.d.ts"} {
		if _, err := Read(name); err == nil {
			t.Fatalf("带路径的 %q 应被拒绝", name)
		}
	}
}

func TestDTSContractConstraints(t *testing.T) {
	docs, err := ListDTS()
	if err != nil {
		t.Fatalf("列出契约失败: %v", err)
	}
	for _, doc := range docs {
		content, rerr := Read(doc.Name)
		if rerr != nil {
			t.Fatalf("读取 %s 失败: %v", doc.Name, rerr)
		}
		text := string(content)
		// 权限描述已移除（运行时无权限开关）
		if strings.Contains(text, "需要权限") || strings.Contains(text, "allow-") {
			t.Fatalf("%s 仍包含权限描述", doc.Name)
		}
		// 同步休眠已移除
		if strings.Contains(text, "export const sleep") || strings.Contains(text, "同步休眠") {
			t.Fatalf("%s 仍包含同步休眠声明", doc.Name)
		}
		// 动态新建 SQL 库已移除
		if strings.Contains(text, "namespace(") || strings.Contains(text, "<name>.db") {
			t.Fatalf("%s 仍包含新建数据库声明", doc.Name)
		}
		// 跨插件机制（signal / callFunction / command / event / synergy）已移除
		for _, banned := range []string{"signal", "callFunction", "command.", "export const event", "synergy"} {
			if strings.Contains(text, banned) {
				t.Fatalf("%s 仍包含已移除的跨插件机制 %q", doc.Name, banned)
			}
		}
	}

	// agent 契约只保留对话与搜索（嵌入已裁剪）
	agent, err := Read("agentAPI.d.ts")
	if err != nil {
		t.Fatalf("读取 agentAPI.d.ts 失败: %v", err)
	}
	for _, member := range []string{"chat(", "search("} {
		if !strings.Contains(string(agent), member) {
			t.Fatalf("agent 契约缺少 %s", member)
		}
	}
	if strings.Contains(string(agent), "embed(") {
		t.Fatalf("agent 契约不应再声明 embed")
	}

	// 与全局能力重叠的成员已裁剪
	trimmed := map[string]string{
		"runtimeAPI.d.ts":  "declare const time",
		"networkAPI.d.ts":  "tcpConnect",
		"encodingAPI.d.ts": "lunarEncoder",
		"imageAPI.d.ts":    "download(",
	}
	for name, banned := range trimmed {
		text, rerr := Read(name)
		if rerr != nil {
			t.Fatalf("读取 %s 失败: %v", name, rerr)
		}
		if strings.Contains(string(text), banned) {
			t.Fatalf("%s 仍包含已裁剪的能力 %q", name, banned)
		}
	}

	// video.frames 声明与实现返回字段一致
	video, err := Read("videoAPI.d.ts")
	if err != nil {
		t.Fatalf("读取 videoAPI.d.ts 失败: %v", err)
	}
	for _, key := range []string{"frames", "timestamp", "skipped", "count"} {
		if !strings.Contains(string(video), key) {
			t.Fatalf("video 契约缺少 %s", key)
		}
	}
}
