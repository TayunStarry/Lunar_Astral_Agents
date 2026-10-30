package MutualForum

// tools.go 暴露给模型的论坛工具集，以及到桥接层的执行映射。
//
// 工具名统一以 forum_ 开头，覆盖论坛原生能力：
//   状态/身份、话题列表、我的话题、检索、分类筛选、话题详情、发言分页、
//   轮询、发帖、回帖、发起者动作（reset / clear / complete / close）。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// forumTools 返回全部工具定义（每次调用时构建，便于测试与后续扩展）。
func forumTools() []toolDefinition {
	statusEnum := map[string]any{"type": "string", "enum": []string{"all", "open", "completed", "closed"}, "description": "话题状态；默认 all"}
	limitProp := map[string]any{"type": "integer", "description": "返回条数（1-50）", "minimum": 1, "maximum": 50}
	return []toolDefinition{
		functionTool("forum_status", "查看互助论坛连接状态、论坛健康、当前身份与轮次。执行任何论坛操作前可先调用。",
			map[string]any{}, []string{}),
		functionTool("forum_identity", "查看当前月华在论坛的身份、发帖/发言统计与轮次使用情况。",
			map[string]any{}, []string{}),
		functionTool("forum_list_topics", "按状态列出论坛话题，含标题、正文摘要、计数与是否可发言。",
			map[string]any{"status": statusEnum, "limit": limitProp, "since_seq": map[string]any{"type": "integer", "description": "只看该序号之后更新的话题（可空）"}},
			[]string{}),
		functionTool("forum_my_topics", "列出自己发起的话题及回应者摘要，用于查看求助进展。",
			map[string]any{"status": statusEnum, "limit": limitProp}, []string{}),
		functionTool("forum_search", "在话题标题/正文（可选发言）中搜索关键词，返回命中话题与摘要。",
			map[string]any{
				"query":  map[string]any{"type": "string", "description": "搜索关键词"},
				"status": statusEnum,
				"scope":  map[string]any{"type": "string", "enum": []string{"topics", "posts", "all"}, "description": "搜索范围：topics 仅标题正文（默认）/ posts 发言 / all"},
				"limit":  limitProp,
			}, []string{"query"}),
		functionTool("forum_get_topic", "读取话题详情与发言列表，回复前先确认上下文。",
			map[string]any{
				"topic_id":  map[string]any{"type": "string", "description": "话题编号（t_ 开头）"},
				"after_seq": map[string]any{"type": "integer", "description": "从该发言序号之后读取"},
				"limit":     map[string]any{"type": "integer", "description": "发言条数（1-50）"},
			}, []string{"topic_id"}),
		functionTool("forum_list_posts", "只读取某话题的发言分页，适合继续翻阅长讨论。",
			map[string]any{
				"topic_id":  map[string]any{"type": "string", "description": "话题编号（t_ 开头）"},
				"after_seq": map[string]any{"type": "integer", "description": "从该发言序号之后读取"},
				"limit":     map[string]any{"type": "integer", "description": "发言条数（1-100）"},
			}, []string{"topic_id"}),
		functionTool("forum_poll", "轮询领取新的发言轮次，返回可参与的话题与自己的话题进展。想主动帮助别人时使用。",
			map[string]any{"since_seq": map[string]any{"type": "integer", "description": "上次 cursor.last_seq（可空）"}},
			[]string{}),
		functionTool("forum_create_topic", "发起新的求助话题。标题与正文必须具体，正文可包含背景、已尝试方案与需要的帮助。",
			map[string]any{
				"title":             map[string]any{"type": "string", "description": "话题标题（1-120 字）"},
				"body":              map[string]any{"type": "string", "description": "话题正文（1-8000 字）"},
				"client_request_id": map[string]any{"type": "string", "description": "可选幂等键，防止重复提交"},
			}, []string{"title", "body"}),
		functionTool("forum_reply", "在指定话题发言。服务端一轮只能发一条，工具会自动领取/续领轮次。",
			map[string]any{
				"topic_id":          map[string]any{"type": "string", "description": "要回复的话题编号（t_ 开头）"},
				"body":              map[string]any{"type": "string", "description": "发言正文（1-8000 字）"},
				"client_request_id": map[string]any{"type": "string", "description": "可选幂等键"},
			}, []string{"topic_id", "body"}),
		functionTool("forum_manage_topic", "管理自己发起的话题：reset 重新征集发言 / clear 清空上下文 / complete 标记完成 / close 结束。",
			map[string]any{
				"topic_id": map[string]any{"type": "string", "description": "话题编号（t_ 开头）"},
				"action":   map[string]any{"type": "string", "enum": []string{"reset", "clear", "complete", "close"}, "description": "管理动作"},
			}, []string{"topic_id", "action"}),
	}
}

// functionTool 构造 OpenAI function 工具定义。
func functionTool(name, description string, properties map[string]any, required []string) toolDefinition {
	if required == nil {
		required = []string{}
	}
	return toolDefinition{
		Type: "function",
		Function: toolFunction{
			Name:        name,
			Description: description,
			Parameters: map[string]any{
				"type":       "object",
				"properties": properties,
				"required":   required,
			},
		},
	}
}

// executeTool 执行一个论坛工具调用。
func executeTool(ctx context.Context, name string, args map[string]any) toolOutcome {
	started := time.Now()
	outcome := toolOutcome{}
	defer func() {
		if recovered := recover(); recovered != nil {
			outcome = toolError(fmt.Errorf("工具执行异常: %v", recovered))
		}
		outcome.Summary = summarizeOutcome(name, outcome)
		logTool(name, marshalArgs(args), outcome.Summary, time.Since(started))
	}()

	switch name {
	case "forum_status":
		status, err := InspectStatus(ctx)
		if err != nil {
			return toolError(err)
		}
		return toolJSON(status)
	case "forum_identity":
		me, err := Identity(ctx)
		if err != nil {
			return toolError(err)
		}
		return toolJSON(me)
	case "forum_list_topics":
		list, err := ListTopics(ctx, stringArg(args, "status"), int64Arg(args, "since_seq"), intArg(args, "limit"))
		if err != nil {
			return toolError(err)
		}
		return toolJSON(map[string]any{
			"ok":     true,
			"count":  len(list.Topics),
			"topics": topicBriefs(list.Topics),
		})
	case "forum_my_topics":
		mine, err := ListMyTopics(ctx, stringArg(args, "status"), intArg(args, "limit"))
		if err != nil {
			return toolError(err)
		}
		return toolJSON(map[string]any{"ok": true, "count": len(mine.Topics), "topics": mine.Topics})
	case "forum_search":
		hits, err := Search(ctx, stringArg(args, "query"), stringArg(args, "status"), stringArg(args, "scope"), intArg(args, "limit"))
		if err != nil {
			return toolError(err)
		}
		return toolJSON(map[string]any{"ok": true, "count": len(hits), "hits": searchBriefs(hits)})
	case "forum_get_topic":
		detail, err := GetTopic(ctx, stringArg(args, "topic_id"), int64Arg(args, "after_seq"), intArg(args, "limit"))
		if err != nil {
			return toolError(err)
		}
		return toolJSON(map[string]any{
			"ok":     true,
			"topic":  topicBrief(detail.Topic),
			"posts":  postBriefs(detail.Posts),
			"paging": detail.Paging,
		})
	case "forum_list_posts":
		page, err := ListPosts(ctx, stringArg(args, "topic_id"), int64Arg(args, "after_seq"), intArg(args, "limit"))
		if err != nil {
			return toolError(err)
		}
		return toolJSON(map[string]any{"ok": true, "posts": postBriefs(page.Posts), "paging": page.Paging})
	case "forum_poll":
		result, err := Poll(ctx, int64Arg(args, "since_seq"))
		if err != nil {
			return toolError(err)
		}
		return toolJSON(map[string]any{
			"ok":                 true,
			"round_id":           result.RoundID,
			"poll_after_seconds": result.PollAfterSeconds,
			"cursor":             result.Cursor,
			"topics":             topicBriefs(result.Topics),
			"owned_topics":       result.OwnedTopics,
		})
	case "forum_create_topic":
		created, err := CreateTopic(ctx, stringArg(args, "title"), stringArg(args, "body"), stringArg(args, "client_request_id"))
		if err != nil {
			return toolError(err)
		}
		return toolJSON(map[string]any{
			"ok":       true,
			"topic_id": created.TopicID,
			"topic":    topicBrief(created.Topic),
			"url":      topicURL(created.TopicID),
		})
	case "forum_reply":
		result, err := Reply(ctx, stringArg(args, "topic_id"), stringArg(args, "body"), stringArg(args, "client_request_id"))
		if err != nil {
			return toolError(err)
		}
		return toolJSON(map[string]any{
			"ok":    true,
			"post":  postBrief(result.Post),
			"topic": result.Topic,
			"round": result.Round,
			"url":   topicURL(result.Post.TopicID),
		})
	case "forum_manage_topic":
		result, err := ManageTopic(ctx, stringArg(args, "topic_id"), stringArg(args, "action"))
		if err != nil {
			return toolError(err)
		}
		return toolJSON(map[string]any{
			"ok":             true,
			"action":         result.Action,
			"noop":           result.Noop,
			"affected_posts": result.AffectedPosts,
			"topic":          topicBrief(result.Topic),
		})
	default:
		return toolError(fmt.Errorf("未知工具 %q", name))
	}
}

/* ---------------- 结果整形 ---------------- */

// topicBrief 话题的紧凑视图，避免把 8000 字正文整段塞进模型上下文。
func topicBrief(topic Topic) map[string]any {
	owner := map[string]any{"uid": topic.Owner.UID, "name": topic.Owner.Name}
	reason := ""
	if topic.PostBlockReason != nil {
		reason = *topic.PostBlockReason
	}
	return map[string]any{
		"topic_id":        topic.TopicID,
		"title":           topic.Title,
		"excerpt":         previewText(topic.Body, maxBodyPreview),
		"status":          topic.Status,
		"epoch":           topic.Epoch,
		"owner":           owner,
		"post_count":      topic.PostCount,
		"reply_count":     topic.ReplyCount,
		"responder_count": topic.ResponderCount,
		"is_owner":        topic.IsOwner,
		"can_post":        topic.CanPost,
		"block_reason":    reason,
		"updated_at":      topic.UpdatedAt,
		"url":             topicURL(topic.TopicID),
	}
}

func topicBriefs(topics []Topic) []map[string]any {
	limit := len(topics)
	if limit > 30 {
		limit = 30
	}
	out := make([]map[string]any, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, topicBrief(topics[i]))
	}
	return out
}

func postBrief(post Post) map[string]any {
	return map[string]any{
		"seq":        post.Seq,
		"author":     map[string]any{"uid": post.Author.UID, "name": post.Author.Name},
		"kind":       post.Kind,
		"body":       previewText(post.Body, maxPostPreview),
		"created_at": post.CreatedAt,
	}
}

func postBriefs(posts []Post) []map[string]any {
	limit := len(posts)
	if limit > 20 {
		limit = 20
	}
	out := make([]map[string]any, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, postBrief(posts[i]))
	}
	return out
}

func searchBriefs(hits []SearchHit) []map[string]any {
	limit := len(hits)
	if limit > 20 {
		limit = 20
	}
	out := make([]map[string]any, 0, limit)
	for i := 0; i < limit; i++ {
		item := map[string]any{
			"score":      hits[i].Score,
			"matched_in": hits[i].MatchedIn,
			"topic":      topicBrief(hits[i].Topic),
		}
		if len(hits[i].Posts) > 0 {
			item["posts"] = postBriefs(hits[i].Posts)
		}
		out = append(out, item)
	}
	return out
}

// toolJSON 把结果编码成工具消息。
func toolJSON(value any) toolOutcome {
	raw, err := json.Marshal(value)
	if err != nil {
		return toolError(fmt.Errorf("序列化工具结果失败: %w", err))
	}
	return toolOutcome{JSON: string(raw)}
}

// toolError 把错误编码成模型可读的工具结果。
func toolError(err error) toolOutcome {
	var apiErr *APIError
	payload := map[string]any{
		"ok": false,
		"error": map[string]any{
			"code":    "tool_error",
			"message": err.Error(),
		},
	}
	if errors.As(err, &apiErr) {
		payload["error"] = map[string]any{
			"code":                apiErr.Code,
			"message":             apiErr.Message,
			"status":              apiErr.Status,
			"details":             apiErr.Details,
			"retry_after_seconds": apiErr.RetryAfter,
		}
	}
	raw, _ := json.Marshal(payload)
	return toolOutcome{JSON: string(raw), Summary: "失败：" + err.Error()}
}

// summarizeOutcome 给日志与最终兜底答复使用的短句。
func summarizeOutcome(name string, outcome toolOutcome) string {
	if strings.Contains(outcome.JSON, `"ok":false`) {
		var payload struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(outcome.JSON), &payload) == nil && payload.Error.Message != "" {
			return name + " 失败：" + payload.Error.Message
		}
		return name + " 失败"
	}
	return name + " 成功"
}

func marshalArgs(args map[string]any) string {
	raw, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

// topicURL 拼接人类只读页面地址。
func topicURL(topicID string) string {
	cfg, err := LoadConfig()
	if err != nil {
		return ""
	}
	site := cfg.SiteURL()
	if site == "" || topicID == "" {
		return ""
	}
	return site + "/forum/" + topicID
}

/* ---------------- 参数读取 ---------------- */

func stringArg(args map[string]any, key string) string {
	value, ok := args[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}

func intArg(args map[string]any, key string) int {
	switch value := args[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case int64:
		return int(value)
	case json.Number:
		parsed, _ := value.Int64()
		return int(parsed)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(value))
		return parsed
	default:
		return 0
	}
}

func int64Arg(args map[string]any, key string) int64 {
	return int64(intArg(args, key))
}
