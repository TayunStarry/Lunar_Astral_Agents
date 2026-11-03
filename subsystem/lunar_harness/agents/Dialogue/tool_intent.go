package Dialogue

// tool_intent.go 聚合工具意图检测：用文本嵌入模型比较「用户意图」与各聚合工具语义，
// 逐个判断月华是否按要求调用了对应工具。检测异步执行并带超时，失败时回退到原有正则规则。

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/DialogueBridging"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// aggregateToolIntents 对话者全部聚合工具的意图语义目录。
var aggregateToolIntents = []aggregateIntent{
	{
		Name:  techniqueToolName,
		Label: "技法与文件操作",
		Branches: []intentBranch{
			{Type: techniquePainter, Label: "绘画", Semantic: "画图、绘画、插画、自画像、立绘、头像、表情包、参考图改图、生成图片"},
			{Type: techniqueMusician, Label: "音乐", Semantic: "创作音乐、作曲、演奏、写曲子、生成旋律、钢琴曲"},
			{Type: techniqueScreenshoter, Label: "截图", Semantic: "截图、截屏、看当前屏幕、截取窗口、屏幕画面"},
			{Type: techniqueFileOperations, Label: "文件操作", Semantic: "文件操作、读写文件、整理文档、整理笔记、改写文档、创建文件、压缩包、运行脚本、代码文件维护、多步骤任务编排"},
		},
	},
	{
		Name:  networkExploreToolName,
		Label: "联网探索",
		Branches: []intentBranch{
			{Type: networkWebSearchTool, Label: "网页检索", Semantic: "联网搜索、查资料、搜最新消息、网页检索、查新闻、查天气、查实时信息、搜索答案"},
			{Type: networkBilibiliTool, Label: "哔哩观影", Semantic: "去B站看视频、追更UP主、看哔哩哔哩"},
			{Type: networkMutualForumTool, Label: "互助论坛", Semantic: "去互助论坛、逛论坛、发帖求助、论坛提问"},
			{Type: networkQQSendTool, Label: "QQ 通讯", Semantic: "给QQ群或好友发消息、发图片、发文件、转达"},
		},
	},
	{
		Name:  crystalToolName,
		Label: "扩展应用",
		Branches: []intentBranch{
			{Label: "打开/运行应用", Semantic: "打开应用、启动程序、运行扩展应用、使用某个应用做任务"},
		},
	},
	{
		Name:  "fetch_resources",
		Label: "资源与记忆",
		Branches: []intentBranch{
			{Type: "memory", Label: "记忆", Semantic: "回忆之前的聊天、查询记忆、还记得吗、之前说过什么"},
			{Type: "handle", Label: "句柄内容", Semantic: "查看聊天中收到的图片、视频、音频、语音或附件内容，按资源句柄打开"},
			{Type: "path", Label: "按路径找文件", Semantic: "按文件名找聊天中收到的文件、附件在哪里"},
			{Type: "list", Label: "资源清单", Semantic: "列出收到的资源、句柄清单、有哪些文件"},
		},
	},
	{
		Name:  lifePlanningToolName,
		Label: "生活规划",
		Branches: []intentBranch{
			{Type: lifePlanningPlanManager, Label: "计划管理", Semantic: "日程安排、计划表、提醒、待办、今天明天本周的计划、新增删除修改计划"},
			{Type: lifePlanningRecentActivity, Label: "最近活动", Semantic: "最近做了什么、刚才在做什么、最近在忙什么、最近的生活记录"},
			{Type: lifePlanningRecordIntent, Label: "创作意图", Semantic: "记录下一轮创作意图、记下创作想法、下轮想写什么、把创作期望记下来、让生活创作做什么"},
		},
	},
}

// startToolIntentCheck 启动本轮异步意图检测；禁用或文本过短时返回 nil。
func startToolIntentCheck(userText string) <-chan toolIntentResult {
	if !config.DialogueToolIntentEnabled {
		return nil
	}
	userText = strings.TrimSpace(userText)
	if len([]rune(userText)) < config.DialogueToolIntentMinRunes {
		return nil
	}
	toolIntentFailMu.Lock()
	cooling := time.Now().Before(toolIntentFailUntil)
	toolIntentFailMu.Unlock()
	if cooling {
		return nil
	}
	resultCh := make(chan toolIntentResult, 1)
	timeout := time.Duration(config.DialogueToolIntentTimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		matches, err := evaluateToolIntent(ctx, userText)
		if err != nil {
			toolIntentFailMu.Lock()
			toolIntentFailUntil = time.Now().Add(toolIntentFailCooldown)
			toolIntentFailMu.Unlock()
		}
		threshold := config.DialogueToolIntentThreshold
		if threshold <= 0 || threshold >= 1 {
			threshold = 0.58
		}
		resultCh <- toolIntentResult{
			UserText:  userText,
			Matches:   matches,
			Threshold: threshold,
			Err:       err,
		}
	}()
	return resultCh
}

// evaluateToolIntent 计算用户意图与全部聚合工具的相似度。
func evaluateToolIntent(ctx context.Context, userText string) ([]toolIntentMatch, error) {
	keys, texts := intentTexts()
	threshold := config.DialogueToolIntentThreshold
	if threshold <= 0 || threshold >= 1 {
		threshold = 0.58
	}

	toolIntentCacheMu.Lock()
	cached := make(map[string][]float32, len(toolIntentCache))
	for key, vector := range toolIntentCache {
		cached[key] = vector
	}
	toolIntentCacheMu.Unlock()

	var vectors map[string][]float32
	var intentVec []float32
	if len(cached) == len(keys) {
		raw, err := toolIntentEmbedHook(ctx, []string{userText})
		if err != nil {
			return nil, err
		}
		if len(raw) != 1 || len(raw[0]) == 0 {
			return nil, fmt.Errorf("意图嵌入结果为空")
		}
		vectors = cached
		intentVec = raw[0]
	} else {
		inputs := append(append([]string(nil), texts...), userText)
		raw, err := toolIntentEmbedHook(ctx, inputs)
		if err != nil {
			return nil, err
		}
		if len(raw) != len(inputs) {
			return nil, fmt.Errorf("意图嵌入数量不匹配：期望 %d，实际 %d", len(inputs), len(raw))
		}
		vectors = make(map[string][]float32, len(keys))
		for index, key := range keys {
			vectors[key] = raw[index]
		}
		intentVec = raw[len(raw)-1]
		toolIntentCacheMu.Lock()
		for key, vector := range vectors {
			toolIntentCache[key] = vector
		}
		toolIntentCacheMu.Unlock()
	}
	return scoreToolIntents(intentVec, vectors, threshold), nil
}

// intentTexts 返回全部分支缓存键与语义文本。
func intentTexts() ([]string, []string) {
	keys := make([]string, 0, 16)
	texts := make([]string, 0, 16)
	for _, tool := range aggregateToolIntents {
		for _, branch := range tool.Branches {
			keys = append(keys, intentKey(tool.Name, branch.Type))
			texts = append(texts, branch.Semantic)
		}
	}
	return keys, texts
}

// intentKey 生成分支缓存键。
func intentKey(tool, branch string) string {
	return tool + "|" + branch
}

// scoreToolIntents 按聚合工具粒度取分支最高相似度，达到阈值的进入结果（按分数降序）。
func scoreToolIntents(intent []float32, vectors map[string][]float32, threshold float64) []toolIntentMatch {
	matches := make([]toolIntentMatch, 0, len(aggregateToolIntents))
	for _, tool := range aggregateToolIntents {
		best := toolIntentMatch{Tool: tool.Name, ToolLabel: tool.Label, Score: -1}
		for _, branch := range tool.Branches {
			score := cosine32(intent, vectors[intentKey(tool.Name, branch.Type)])
			if score <= best.Score {
				continue
			}
			best.Branch = branch.Type
			best.BranchLabel = branch.Label
			best.Score = score
		}
		if best.Score >= threshold {
			matches = append(matches, best)
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].Score > matches[j].Score })
	return matches
}

// toolIntentGuard 读取本轮异步检测结果；未就绪时不阻塞，返回 nil 由调用方继续原规则。
func (role *DialogueRole) toolIntentGuard() *PostMessage {
	if role.IntentCheck == nil || role.ToolIntentGuarded {
		return nil
	}
	select {
	case result := <-role.IntentCheck:
		role.IntentCheck = nil
		if result.Err != nil {
			logger.Warn(lunarCoreModule, "[工具检测] 嵌入不可用，回退正则规则: %v", result.Err)
			return nil
		}
		missing := role.missingToolIntents(result.Matches)
		threshold := result.Threshold
		if threshold <= 0 {
			threshold = config.DialogueToolIntentThreshold
		}
		for _, match := range result.Matches {
			logger.Info(lunarCoreModule,
				"[工具检测] 意图=%q 工具=%s 分支=%s 相似度=%.3f 阈值=%.3f 已调用=%t",
				result.UserText, match.Tool, match.Branch, match.Score, threshold, role.CalledTools[match.Tool])
		}
		if len(missing) == 0 {
			return nil
		}
		role.ToolIntentGuarded = true
		lines := make([]string, 0, len(missing))
		for _, match := range missing {
			lines = append(lines, formatToolIntentLine(match))
		}
		content := fmt.Sprintf("【系统校验】（意图相似度阈值 %.2f）根据本轮用户要求，以下聚合工具应被真实调用：\n", threshold) +
			strings.Join(lines, "\n") +
			"\n请直接调用对应工具；若判断本轮确实不需要，请如实说明原因，不要只口头宣称已完成。"
		logger.Warn(lunarCoreModule, "[工具检测] 触发工具补调纠偏，共 %d 个聚合工具", len(missing))
		return &PostMessage{Role: RoleUser, Content: content}
	default:
		return nil
	}
}

// missingToolIntents 过滤掉本轮已经真实调用过 / 当前不可用的聚合工具。
func (role *DialogueRole) missingToolIntents(matches []toolIntentMatch) []toolIntentMatch {
	missing := make([]toolIntentMatch, 0, len(matches))
	for _, match := range matches {
		if role.CalledTools[match.Tool] {
			continue
		}
		if !branchAvailable(match.Tool, match.Branch) {
			continue
		}
		missing = append(missing, match)
	}
	return missing
}

// branchAvailable 判断目标分支在当前运行环境下是否可调用。
func branchAvailable(tool, branch string) bool {
	switch tool {
	case networkExploreToolName:
		if branch == networkQQSendTool {
			return DialogueBridging.Ready()
		}
	case crystalToolName:
		return len(crystalProgramNames()) > 0
	}
	return true
}

// markToolCalls 记录本轮真实调用过的聚合工具。
func (role *DialogueRole) markToolCalls(calls []ToolCallItem) {
	if role.CalledTools == nil {
		role.CalledTools = map[string]bool{}
	}
	for _, call := range calls {
		role.CalledTools[call.Function.Name] = true
	}
}

// formatToolIntentLine 生成一条可解释的补调提示。
func formatToolIntentLine(match toolIntentMatch) string {
	typeText := ""
	if match.Branch != "" {
		typeText = " type=" + match.Branch
	}
	return fmt.Sprintf("- %s%s（%s，相似度 %.3f）", match.Tool, typeText, match.BranchLabel, match.Score)
}

// cosine32 余弦相似度；零向量或维度不一致返回 0。
func cosine32(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		av := float64(a[i])
		bv := float64(b[i])
		dot += av * bv
		normA += av * av
		normB += bv * bv
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
