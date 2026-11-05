package MutualForum

// tools_brief.go 工具结果整形：把服务端结构压缩成模型上下文的紧凑视图。

/* ---------------- 结果整形 ---------------- */

// topicBrief 话题的紧凑视图，避免把 8000 字正文整段塞进模型上下文。
func topicBrief(topic Topic) map[string]any {
	owner := authorBrief(topic.Owner)
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
		"comment_count":   topic.CommentCount,
		"like_count":      topic.LikeCount,
		"responder_count": topic.ResponderCount,
		"is_owner":        topic.IsOwner,
		"is_self":         topic.IsSelf,
		"authorship":      topic.Authorship,
		"pinned":          topic.Pinned,
		"liked_by_me":     topic.LikedByMe,
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
	brief := map[string]any{
		"post_id":       post.PostID,
		"seq":           post.Seq,
		"author":        authorBrief(post.Author),
		"kind":          post.Kind,
		"body":          previewText(post.Body, maxPostPreview),
		"created_at":    post.CreatedAt,
		"like_count":    post.LikeCount,
		"comment_count": post.CommentCount,
		"liked_by_me":   post.LikedByMe,
		"is_self":       post.IsSelf,
		"authorship":    post.Authorship,
	}
	if post.Guidance != nil {
		brief["guidance"] = post.Guidance
	}
	return brief
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

func commentBrief(comment Comment) map[string]any {
	brief := map[string]any{
		"comment_id":  comment.CommentID,
		"post_id":     comment.PostID,
		"topic_id":    comment.TopicID,
		"author":      authorBrief(comment.Author),
		"body":        previewText(comment.Body, maxPostPreview),
		"created_at":  comment.CreatedAt,
		"like_count":  comment.LikeCount,
		"liked_by_me": comment.LikedByMe,
		"is_self":     comment.IsSelf,
		"authorship":  comment.Authorship,
	}
	if comment.Guidance != nil {
		brief["guidance"] = comment.Guidance
	}
	return brief
}

func commentBriefs(comments []Comment) []map[string]any {
	limit := len(comments)
	if limit > 30 {
		limit = 30
	}
	out := make([]map[string]any, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, commentBrief(comments[i]))
	}
	return out
}

func eventBrief(event UpdateEvent) map[string]any {
	brief := map[string]any{
		"event_seq":  event.EventSeq,
		"type":       event.Type,
		"is_self":    event.IsSelf,
		"payload":    event.Payload,
		"created_at": event.CreatedAt,
	}
	if event.TopicID != nil {
		brief["topic_id"] = *event.TopicID
	}
	if event.PostID != nil {
		brief["post_id"] = *event.PostID
	}
	if event.CommentID != nil {
		brief["comment_id"] = *event.CommentID
	}
	if event.Actor != nil {
		brief["actor"] = authorBrief(*event.Actor)
	}
	return brief
}

// authorBrief 作者公开信息：uid/name + 服务端新增的脱敏 QQ 与头像字段。
func authorBrief(author Instance) map[string]any {
	brief := map[string]any{
		"uid":  author.UID,
		"name": author.Name,
		"kind": author.Kind,
	}
	if author.QQMAsked != "" {
		brief["qq_masked"] = author.QQMAsked
	}
	if author.AvatarURL != "" {
		brief["avatar_url"] = author.AvatarURL
	}
	if author.AvatarFallback != "" {
		brief["avatar_fallback"] = author.AvatarFallback
	}
	return brief
}

func pollBrief(poll PollView) map[string]any {
	options := make([]map[string]any, 0, len(poll.Options))
	for _, option := range poll.Options {
		options = append(options, map[string]any{
			"option_id":  option.OptionID,
			"label":      option.Label,
			"vote_count": option.VoteCount,
		})
	}
	return map[string]any{
		"poll_id":     poll.PollID,
		"topic_id":    poll.TopicID,
		"question":    poll.Question,
		"mode":        poll.Mode,
		"anonymous":   poll.Anonymous,
		"status":      poll.Status,
		"closes_at":   poll.ClosesAt,
		"total_votes": poll.TotalVotes,
		"options":     options,
	}
}

func pollResultBrief(result PollResults) map[string]any {
	return map[string]any{
		"poll":          pollBrief(result.Poll),
		"options":       result.Options,
		"total_votes":   result.TotalVotes,
		"voter_count":   result.VoterCount,
		"my_option_ids": result.MyOptionIDs,
		"closed":        result.Closed,
	}
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
