package Dialogue

// tool_intent_test.go 聚合工具意图检测验收：每个工具的正负样例、已调用过滤、嵌入失败回退。

import (
	"errors"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

// basisVectors 为每个工具分支构造单位正交向量。
func basisVectors() ([]string, map[string][]float32) {
	keys, _ := intentTexts()
	vectors := make(map[string][]float32, len(keys))
	for index, key := range keys {
		vector := make([]float32, len(keys))
		vector[index] = 1
		vectors[key] = vector
	}
	return keys, vectors
}

// TestToolIntentEveryToolPositiveAndNegative 每个聚合工具：自身语义命中，其它语义不误判。
func TestToolIntentEveryToolPositiveAndNegative(t *testing.T) {
	_, vectors := basisVectors()
	for _, tool := range aggregateToolIntents {
		for _, branch := range tool.Branches {
			vector := vectors[intentKey(tool.Name, branch.Type)]
			matches := scoreToolIntents(vector, vectors, 0.5)
			found := false
			for _, match := range matches {
				if match.Tool == tool.Name && match.Branch == branch.Type && match.Score >= 0.99 {
					found = true
				}
			}
			if !found {
				t.Fatalf("%s/%s 的正例未命中: %+v", tool.Name, branch.Type, matches)
			}
		}
	}
	// 负例：用另一个工具的向量，不应命中当前工具
	for _, tool := range aggregateToolIntents {
		for _, other := range aggregateToolIntents {
			if other.Name == tool.Name {
				continue
			}
			otherBranch := other.Branches[0]
			matches := scoreToolIntents(vectors[intentKey(other.Name, otherBranch.Type)], vectors, 0.5)
			for _, match := range matches {
				if match.Tool == tool.Name {
					t.Fatalf("%s 的负例被误判为 %s: %+v", tool.Name, other.Name, matches)
				}
			}
		}
	}
	// 零向量不应命中任何工具
	if matches := scoreToolIntents(make([]float32, len(vectors)), vectors, 0.5); len(matches) != 0 {
		t.Fatalf("零向量不应命中: %+v", matches)
	}
}

// TestToolIntentGuardFiltersCalledTools 已调用的聚合工具不再出现在补调提示里。
func TestToolIntentGuardFiltersCalledTools(t *testing.T) {
	role := &DialogueRole{CalledTools: map[string]bool{techniqueToolName: true}}
	resultCh := make(chan toolIntentResult, 1)
	resultCh <- toolIntentResult{
		UserText: "画一张图并记一下下一轮创作",
		Matches: []toolIntentMatch{
			{Tool: techniqueToolName, ToolLabel: "技法创作/文件操作", Branch: techniquePainter, BranchLabel: "绘画", Score: 0.9},
			{Tool: lifePlanningToolName, ToolLabel: "生活规划", Branch: lifePlanningRecordIntent, BranchLabel: "创作意图", Score: 0.75},
		},
	}
	role.IntentCheck = resultCh
	guard := role.toolIntentGuard()
	if guard == nil {
		t.Fatal("存在未调用工具时应触发纠偏")
	}
	text, _ := guard.Content.(string)
	if !strings.Contains(text, lifePlanningToolName) || !strings.Contains(text, lifePlanningRecordIntent) {
		t.Fatalf("补调提示应包含未调用的生活规划: %q", text)
	}
	if strings.Contains(text, techniqueToolName) {
		t.Fatalf("已调用的技法工具不应再次提示: %q", text)
	}
}

// TestToolIntentGuardFallsBackOnEmbedError 嵌入不可用时返回 nil，由原有正则规则兜底。
func TestToolIntentGuardFallsBackOnEmbedError(t *testing.T) {
	role := &DialogueRole{CalledTools: map[string]bool{}}
	resultCh := make(chan toolIntentResult, 1)
	resultCh <- toolIntentResult{Err: errors.New("嵌入服务不可用")}
	role.IntentCheck = resultCh
	if guard := role.toolIntentGuard(); guard != nil {
		t.Fatalf("嵌入失败时不应触发新纠偏: %q", guard.Content)
	}
	// 原有正则规则仍然可用（行为不倒退）
	if !actionClaimPattern.MatchString("已经帮你打开应用了") {
		t.Fatal("原有动作声明规则应保持可用")
	}
}

// TestStartToolIntentCheckDisabledOrShort 关闭检测或文本过短时不启动异步任务。
func TestStartToolIntentCheckDisabledOrShort(t *testing.T) {
	original := config.DialogueToolIntentEnabled
	t.Cleanup(func() { config.DialogueToolIntentEnabled = original })
	config.DialogueToolIntentEnabled = false
	if ch := startToolIntentCheck("帮我画一张图"); ch != nil {
		t.Fatal("关闭检测时不应启动")
	}
	config.DialogueToolIntentEnabled = true
	if ch := startToolIntentCheck("好"); ch != nil {
		t.Fatal("过短文本不应启动")
	}
}
