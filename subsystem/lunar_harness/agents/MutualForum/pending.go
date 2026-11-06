package MutualForum

// pending.go 未读回应的重试队列与本地持久化。
//
// 设计目的：服务端事件游标是单调序号，无法只推进一部分。为了让「提交失败的条目」
// 不被越过，这里不把服务端的「最后事件序号」当作已读判据，而是：
//   - AckedSeq 记录已经安全落盘的最后事件序号（事件先进入本地队列，再推进它）；
//   - Items 保存尚未成功提交的条目及其重试次数，下一轮会继续取到；
//   - 提交成功的条目立即从队列移除。
//
// 队列文件与账号文件同目录，默认是 forum_account.pending.json。

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// pendingReply 一条尚未成功提交的回应，带重试信息。
type pendingReply struct {
	Key       string `json:"key"`
	EventSeq  int64  `json:"event_seq"`
	Type      string `json:"type"`
	TopicID   string `json:"topic_id,omitempty"`
	PostID    string `json:"post_id,omitempty"`
	CommentID string `json:"comment_id,omitempty"`
	ActorName string `json:"actor_name,omitempty"`
	Preview   string `json:"preview,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	Retries   int    `json:"retries"`
	LastError string `json:"last_error,omitempty"`
}

// autoReplyState 未读回应的本地队列状态。
type autoReplyState struct {
	Version  int            `json:"version"`
	AckedSeq int64          `json:"acked_seq"`
	Items    []pendingReply `json:"items"`
}

// replyStateStore 队列读写接口；测试可注入内存实现。
type replyStateStore interface {
	Load() (autoReplyState, error)
	Save(autoReplyState) error
}

// fileReplyStateStore 基于 JSON 文件的队列存储（临时文件 + 重命名原子落盘）。
type fileReplyStateStore struct {
	path string
}

func (s fileReplyStateStore) Load() (autoReplyState, error) {
	state := autoReplyState{Version: autoReplyStateVersion}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, fmt.Errorf("读取未读回应重试状态失败: %w", err)
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return state, fmt.Errorf("解析未读回应重试状态失败: %w", err)
	}
	state.Version = autoReplyStateVersion
	return state, nil
}

func (s fileReplyStateStore) Save(state autoReplyState) error {
	state.Version = autoReplyStateVersion
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("创建未读回应重试状态目录失败: %w", err)
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化未读回应重试状态失败: %w", err)
	}
	temp := s.path + ".tmp"
	if err := os.WriteFile(temp, raw, 0o600); err != nil {
		return fmt.Errorf("写入未读回应重试状态失败: %w", err)
	}
	if err := os.Rename(temp, s.path); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("保存未读回应重试状态失败: %w", err)
	}
	return nil
}

// pendingStorePath 由账号文件路径派生队列文件路径。
func pendingStorePath(cfg Config) string {
	accountPath := cfg.AccountPath()
	ext := filepath.Ext(accountPath)
	return strings.TrimSuffix(accountPath, ext) + pendingStoreSuffix
}

// pendingFromUnread 把事件转成待回应条目。
func pendingFromUnread(item unreadReply) pendingReply {
	return pendingReply{
		Key:       unreadKey(item.CommentID, item.PostID, item.EventSeq),
		EventSeq:  item.EventSeq,
		Type:      item.Type,
		TopicID:   item.TopicID,
		PostID:    item.PostID,
		CommentID: item.CommentID,
		ActorName: item.ActorName,
		Preview:   item.Preview,
		CreatedAt: item.CreatedAt,
	}
}

// reply 还原成提交阶段使用的条目。
func (p pendingReply) reply() unreadReply {
	return unreadReply{
		EventSeq:  p.EventSeq,
		Type:      p.Type,
		TopicID:   p.TopicID,
		PostID:    p.PostID,
		CommentID: p.CommentID,
		ActorName: p.ActorName,
		Preview:   p.Preview,
		CreatedAt: p.CreatedAt,
	}
}

// mergePendingReplies 合并新事件与旧队列；同一目标只保留一份，旧条目保留重试次数。
func mergePendingReplies(existing []pendingReply, incoming []unreadReply) []pendingReply {
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	merged := make([]pendingReply, 0, len(existing)+len(incoming))
	for _, item := range existing {
		key := item.Key
		if key == "" {
			key = unreadKey(item.CommentID, item.PostID, item.EventSeq)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, item)
	}
	for _, item := range incoming {
		pending := pendingFromUnread(item)
		if _, ok := seen[pending.Key]; ok {
			continue
		}
		seen[pending.Key] = struct{}{}
		merged = append(merged, pending)
	}
	return merged
}

// unreadKey 生成去重键：评论优先，其次发言，最后退回事件序号。
func unreadKey(commentID, postID string, eventSeq int64) string {
	if commentID != "" {
		return "comment:" + commentID
	}
	if postID != "" {
		return "post:" + postID
	}
	return fmt.Sprintf("event:%d", eventSeq)
}
