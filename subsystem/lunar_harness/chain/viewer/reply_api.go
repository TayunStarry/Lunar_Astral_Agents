package viewer

// reply_api.go 页面定位失败时的接口回复兜底。
//
// 消息中心已经给出 root_id / target_id / source_id，这里只做三件事：
//   - 推出评论接口需要的 root 与 parent（回复挂到对方那条评论下）；
//   - 调 x/v2/reply/add 发布；
//   - 回读根评论的子评论，确认回应真的可见，而不是只信接口 code=0。

import (
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/bilibili"
)

// replyIDs 从消息中心条目推出评论接口需要的 root / parent。
//
//	root：根评论 rpid（子回复挂在它下面）
//	parent：对方那条评论的 rpid
func replyIDs(item inboxItem) (root, parent string, ok bool) {
	parent = strings.TrimSpace(item.SourceID)
	root = strings.TrimSpace(item.RootID)
	if root == "" {
		if target := strings.TrimSpace(item.TargetID); target != "" {
			root = target
		} else {
			root = parent
		}
	}
	if parent == "" {
		parent = strings.TrimSpace(item.TargetID)
	}
	if parent == "" {
		parent = root
	}
	// @ 消息可能只有对象 ID（没有 root/target/source）：按根评论发布，
	// 只要 oid 可用就不应判定为「缺 ID」而放弃回应。
	if strings.TrimSpace(item.Oid) == "" {
		return "", "", false
	}
	return root, parent, true
}

// replyByAPI 用消息中心的评论 ID 发布回复，并回读确认可见。
func (w *webSession) replyByAPI(item inboxItem, message string) error {
	root, parent, ok := replyIDs(item)
	if !ok {
		return fmt.Errorf("消息缺少对象或评论 ID，无法接口回复")
	}
	if err := w.postReplyAPI(item.Type, item.Oid, message, root, parent, nil); err != nil {
		return err
	}

	// 只有挂在根评论下才需要回读子评论；根评论发布以接口 code=0 为准
	if root != "" {
		w.wait(3 * time.Second)
		if !w.replyVisible(item, root, message) {
			return fmt.Errorf("接口已受理但回读未见（可能仍在审核）")
		}
	}
	return nil
}

// postReplyAPI 调 x/v2/reply/add 发布评论/回复，是全模块唯一的接口发布实现。
//
// root/parent 同时为空表示发布根评论；targets 非空时写入 at_name/at_mid 产生真实 @。
// 表单组装、csrf 与响应解包在 engine/bilibili。
func (w *webSession) postReplyAPI(typ int, oid, message, root, parent string, targets []MentionTarget) error {
	return bilibili.PostReply(w, bilibili.ReplyRequest{
		Type:    typ,
		Oid:     oid,
		Message: message,
		Root:    root,
		Parent:  parent,
		Targets: targets,
	})
}

// replyVisible 回读根评论下的子评论，确认刚才的回应已经出现。
func (w *webSession) replyVisible(item inboxItem, root, message string) bool {
	fingerprint := commentFingerprint(message)
	if fingerprint == "" {
		return false
	}
	replies, err := bilibili.SubReplies(w, item.Type, item.Oid, root, 20)
	if err != nil {
		return false
	}
	for _, reply := range replies {
		if strings.Contains(normalizeInline(reply.Content), fingerprint) {
			return true
		}
	}
	return false
}
