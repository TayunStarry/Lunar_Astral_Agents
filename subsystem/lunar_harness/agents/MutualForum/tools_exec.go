package MutualForum

// tools_exec.go 论坛工具调用到桥接层操作的执行映射。

import (
	"context"
	"fmt"
	"strings"
	"time"
)

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
			"paging": list.Paging,
		})
	case "forum_my_topics":
		mine, err := ListMyTopics(ctx, stringArg(args, "status"), int64Arg(args, "before_seq"), intArg(args, "limit"))
		if err != nil {
			return toolError(err)
		}
		return toolJSON(map[string]any{"ok": true, "count": len(mine.Topics), "topics": mine.Topics, "paging": mine.Paging})
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
		result, err := Poll(ctx, int64Arg(args, "since_seq"), stringArg(args, "status"))
		if err != nil {
			return toolError(err)
		}
		return toolJSON(map[string]any{
			"ok":                 true,
			"round_id":           result.RoundID,
			"poll_after_seconds": result.PollAfterSeconds,
			"round_ttl_seconds":  result.RoundTTLSeconds,
			"round_required":     result.RoundRequired,
			"status":             result.Status,
			"owned_topics_order": result.OwnedTopicsOrder,
			"cursor":             result.Cursor,
			"event_cursor":       result.EventCursor,
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
	case "forum_comment":
		if strings.ToLower(stringArg(args, "action")) == "create" {
			comment, err := CreateComment(
				ctx,
				stringArg(args, "post_id"),
				stringArg(args, "body"),
				stringArg(args, "quote_comment_uid"),
				stringSliceArg(args, "mentions"),
			)
			if err != nil {
				return toolError(err)
			}
			return toolJSON(map[string]any{
				"ok":      true,
				"comment": commentBrief(*comment),
				"post_id": comment.PostID,
			})
		}
		page, err := ListComments(ctx, stringArg(args, "post_id"), stringArg(args, "sort"), int64Arg(args, "after_seq"), intArg(args, "limit"))
		if err != nil {
			return toolError(err)
		}
		return toolJSON(map[string]any{
			"ok":       true,
			"count":    len(page.Comments),
			"sort":     page.Sort,
			"comments": commentBriefs(page.Comments),
			"paging":   page.Paging,
		})
	case "forum_react":
		result, err := React(ctx, stringArg(args, "target"), stringArg(args, "id"), stringArg(args, "action"))
		if err != nil {
			return toolError(err)
		}
		return toolJSON(map[string]any{
			"ok":         true,
			"target":     stringArg(args, "target"),
			"id":         stringArg(args, "id"),
			"liked":      result.Liked,
			"changed":    result.Changed,
			"like_count": result.LikeCount,
		})
	case "forum_updates":
		action := strings.ToLower(stringArg(args, "action"))
		switch action {
		case "guidance":
			list, err := ListGuidance(ctx, intArg(args, "limit"))
			if err != nil {
				return toolError(err)
			}
			return toolJSON(map[string]any{"ok": true, "count": len(list.Guidances), "guidance": list.Guidances})
		case "ack":
			resolved, err := AckGuidance(ctx, stringArg(args, "guidance_id"))
			if err != nil {
				return toolError(err)
			}
			return toolJSON(map[string]any{
				"ok":          true,
				"guidance_id": stringArg(args, "guidance_id"),
				"resolved":    resolved,
			})
		default:
			feed, err := Updates(ctx, int64Arg(args, "since_seq"), stringSliceArg(args, "types"), intArg(args, "wait_seconds"), intArg(args, "limit"))
			if err != nil {
				return toolError(err)
			}
			events := make([]map[string]any, 0, len(feed.Events))
			for _, event := range feed.Events {
				events = append(events, eventBrief(event))
			}
			return toolJSON(map[string]any{
				"ok":               true,
				"count":            len(events),
				"events":           events,
				"cursor":           feed.Cursor,
				"guidance_pending": feed.GuidancePending,
			})
		}
	case "forum_vote":
		action := strings.ToLower(stringArg(args, "action"))
		switch action {
		case "create":
			poll, err := CreatePoll(
				ctx,
				stringArg(args, "topic_id"),
				stringArg(args, "question"),
				stringSliceArg(args, "options"),
				stringArg(args, "mode"),
				boolArg(args, "anonymous"),
				stringArg(args, "closes_at"),
			)
			if err != nil {
				return toolError(err)
			}
			return toolJSON(map[string]any{"ok": true, "poll": pollBrief(*poll)})
		case "vote":
			result, err := VotePoll(ctx, stringArg(args, "poll_id"), intSliceArg(args, "option_ids"))
			if err != nil {
				return toolError(err)
			}
			return toolJSON(map[string]any{
				"ok":            true,
				"poll":          pollResultBrief(*result),
				"created_votes": result.CreatedVotes,
				"my_option_ids": result.MyOptionIDs,
			})
		case "close":
			result, err := ClosePoll(ctx, stringArg(args, "poll_id"))
			if err != nil {
				return toolError(err)
			}
			return toolJSON(map[string]any{"ok": true, "poll": pollResultBrief(*result), "closed": result.Closed})
		default:
			result, err := GetPoll(ctx, stringArg(args, "poll_id"))
			if err != nil {
				return toolError(err)
			}
			return toolJSON(map[string]any{"ok": true, "poll": pollResultBrief(*result)})
		}
	case "forum_edit":
		result, err := EditContent(ctx, stringArg(args, "target"), stringArg(args, "id"), stringArg(args, "body"))
		if err != nil {
			return toolError(err)
		}
		return toolJSON(map[string]any{
			"ok":     true,
			"target": stringArg(args, "target"),
			"id":     stringArg(args, "id"),
			"result": result,
		})
	case "forum_delete":
		result, err := DeleteContent(ctx, stringArg(args, "target"), stringArg(args, "id"))
		if err != nil {
			return toolError(err)
		}
		return toolJSON(map[string]any{
			"ok":      true,
			"target":  stringArg(args, "target"),
			"id":      stringArg(args, "id"),
			"deleted": result.Deleted,
		})
	case "forum_rename":
		instance, err := Rename(ctx, stringArg(args, "name"))
		if err != nil {
			return toolError(err)
		}
		return toolJSON(map[string]any{"ok": true, "instance": instance})
	default:
		return toolError(fmt.Errorf("未知工具 %q", name))
	}
}
