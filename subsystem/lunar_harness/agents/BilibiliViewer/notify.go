package BilibiliViewer

// notify.go 被动消息处理：只在收到 @ 或追评时回应。
//
// 回应优先走页面操作（打开原帖 → 找到对方评论 → 点回复 → 输入发送），
// 由 B 站前端自己决定挂载参数；页面定位失败时用消息中心的 root/target/source
// 三个 ID 走接口兜底，并在发送后回读子评论确认真的挂上去了。

import (
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/prompts"
)

// notifyStepOrder 被动消息的固定处理顺序。
var notifyStepOrder = []notifyCategory{
	notifyAt,
	notifyVideoReply,
	notifyVideoComment,
	notifyDynamic,
}

// ownMid 读取当前登录账号的 mid；失败时返回 0。
func (w *webSession) ownMid() int64 {
	raw, err := w.pageFetchText("GET", "https://api.bilibili.com/x/web-interface/nav", "", "", 15*time.Second)
	if err != nil {
		logger.Warn("BilibiliViewer", "读取当前账号信息失败（继续执行）: %v", err)
		return 0
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Mid   int64  `json:"mid"`
			Uname string `json:"uname"`
		} `json:"data"`
	}
	if err := jsonUnmarshal(raw, &resp); err != nil || resp.Code != 0 {
		logger.Warn("BilibiliViewer", "解析当前账号信息失败（继续执行）: %v / code=%d", err, resp.Code)
		return 0
	}
	if resp.Data.Mid > 0 {
		logger.Info("BilibiliViewer", "当前登录账号：%s（mid=%d）", resp.Data.Uname, resp.Data.Mid)
	}
	return resp.Data.Mid
}

// handleNotifications 处理未读 @ 与追评，共享 replies_per_round 额度。
func (r *runner) handleNotifications() {
	items, err := r.browser.fetchInboxItems()
	if err != nil {
		logger.Warn("BilibiliViewer", "读取 B 站消息中心失败（跳过通知处理）: %v", err)
		return
	}
	unread := filterUnreadInbox(items, loadInboxSeen(), time.Now())
	pending := make([]inboxItem, 0, len(unread))
	for _, item := range unread {
		if r.mid > 0 && item.Mid == r.mid {
			continue
		}
		pending = append(pending, item)
	}
	if len(pending) == 0 {
		logger.Info("BilibiliViewer", "没有需要回应的 B 站通知")
		return
	}
	logger.Info("BilibiliViewer", "待回应通知 %d 条（额度 %d 次）", len(pending), r.cfg.RepliesPerRound)

	for _, category := range notifyStepOrder {
		for index := range pending {
			item := pending[index]
			if item.Category != category {
				continue
			}
			if r.replied >= r.cfg.RepliesPerRound {
				logger.Info("BilibiliViewer", "本轮回应额度已用完，剩余通知留到下一轮")
				return
			}
			r.setProgress(fmt.Sprintf("正在处理%s：%s", category, item.User))
			if r.replyNotification(item) {
				r.replied++
			}
			time.Sleep(replyPause)
		}
	}
}

// replyNotification 生成并通过页面操作发布一条回应；成功返回 true。
func (r *runner) replyNotification(item inboxItem) bool {
	text, err := generateInboxReply(item)
	if err != nil {
		logger.Warn("BilibiliViewer", "生成回应失败 → %s: %v", item.User, err)
		return false
	}
	if err := r.publishReply(item, text); err != nil {
		logger.Warn("BilibiliViewer", "回应%s失败 → %s: %v（%s）", item.Category, item.User, err, inboxBrief(item))
		return false
	}
	markInboxSeen(item)
	logger.Info("BilibiliViewer", "已回应%s → %s：%s", item.Category, item.User, text)
	r.logActivity(lifeActivity{
		Kind:    "inbox",
		Where:   inboxScene(item),
		Target:  item.User,
		Content: item.Message,
		Reply:   text,
		Note:    inboxNote(item) + "｜" + item.Category.String(),
	})
	return true
}

// publishReply 回应一条被动消息。
//
// 目标评论是嵌套子回复（root/target 非空）时，页面很难定位：
// 视频评论区懒加载 + 按热度分页，子回复还折叠在根评论里。这类消息 ID 齐全，
// 直接用接口按 root/parent 回复并回读校验，不再先做注定失败的页面定位。
// 根级评论仍先走页面；页面失败时用接口兜底。
func (r *runner) publishReply(item inboxItem, text string) error {
	nested := strings.TrimSpace(item.RootID) != "" || strings.TrimSpace(item.TargetID) != ""
	var apiErr error
	if nested {
		apiErr = r.browser.replyByAPI(item, text)
		if apiErr == nil {
			logger.Info("BilibiliViewer", "接口回复已发布并回读确认 → %s", item.User)
			return nil
		}
		logger.Warn("BilibiliViewer", "接口回复失败，改用页面回复 → %s: %v", item.User, apiErr)
	}

	pageURL := replyPageURL(item)
	if pageURL == "" {
		if apiErr != nil {
			return fmt.Errorf("接口回复失败: %w", apiErr)
		}
		return fmt.Errorf("暂不支持在该位置（type=%d）回复", item.Type)
	}
	var pageErr error
	if err := r.browser.navigate(pageURL, 30*time.Second); err != nil {
		pageErr = fmt.Errorf("打开原帖页面失败: %w", err)
	} else {
		r.browser.wait(1500 * time.Millisecond)
		pageErr = r.browser.uiReplyToComment(item.Message, item.User, text)
	}
	if pageErr == nil {
		return nil
	}
	if nested {
		return fmt.Errorf("接口回复失败（%v），页面回复也失败: %w（%s）", apiErr, pageErr, inboxBrief(item))
	}
	logger.Info("BilibiliViewer", "页面未定位到目标评论，改用接口回复 → %s: %v（%s）", item.User, pageErr, inboxBrief(item))
	if err := r.browser.replyByAPI(item, text); err != nil {
		return fmt.Errorf("页面回复失败（%v），接口回复失败: %w（%s）", pageErr, err, inboxBrief(item))
	}
	logger.Info("BilibiliViewer", "接口回复已发布 → %s", item.User)
	return nil
}

// replyPageURL 把消息中心的对象 ID 还原成可访问的页面地址。
//
// 优先使用消息中心给的 web URI：opus/图片动态的 subject_id 并不是页面 ID。
func replyPageURL(item inboxItem) string {
	if uri := strings.TrimSpace(item.URI); strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		return uri
	}
	anchor := ""
	if item.SourceID != "" {
		anchor = "#reply" + item.SourceID
	}
	// opus/动态 URI 里带的是真正的页面 ID，subject_id 不一定是。
	if oid := dynamicOidFromURI(item.URI); oid != "" {
		return "https://www.bilibili.com/opus/" + oid + anchor
	}
	switch item.Type {
	case 1:
		if item.Oid == "" {
			return ""
		}
		// comment_on=1 才会处理 #reply 定位，否则只停在视频页顶部。
		return videoURLByAid(item.Oid) + "?comment_on=1" + anchor
	case 17, 11:
		if item.Oid == "" {
			return ""
		}
		return "https://t.bilibili.com/" + item.Oid + anchor
	default:
		return ""
	}
}

// inboxBrief 通知条目的关键字段摘要（失败日志排查用）
func inboxBrief(item inboxItem) string {
	return fmt.Sprintf("type=%d oid=%s src=%s root=%s target=%s uri=%s",
		item.Type, item.Oid, item.SourceID, item.RootID, item.TargetID, item.URI)
}

// generateInboxReply 按场景生成一句自然回应（注入人设与长期记忆）。
func generateInboxReply(item inboxItem) (string, error) {
	persona, err := prompts.Read("lifeRole.md")
	if err != nil {
		return "", fmt.Errorf("读取人设失败: %w", err)
	}
	scene := inboxScene(item)
	task := "正在回复 B 站" + scene + "里收到的消息，只输出一句自然、友好的回应，1–60 字；" +
		"不争论、不引战、不放链接/@。"
	system := string(persona) + "\n\n## 本次任务\n\n" + task
	if block := memoryMaterial(10); block != "" {
		system += "\n\n" + block
	}
	var material strings.Builder
	fmt.Fprintf(&material, "对方：%s\n对方的消息：%s\n", item.User, item.Message)
	if item.MyContent != "" {
		fmt.Fprintf(&material, "我原来发的内容：%s\n", item.MyContent)
	}
	if item.Title != "" {
		fmt.Fprintf(&material, "所在位置：《%s》\n", item.Title)
	}
	text, err := chatTextModel(modelPrimary, system, material.String())
	if err != nil {
		return "", err
	}
	text = strings.TrimSpace(strings.ReplaceAll(text, "@", ""))
	if text == "" {
		return "", fmt.Errorf("回应为空")
	}
	return finishComment(text, 120), nil
}

// inboxScene 互动发生的场景描述。
func inboxScene(item inboxItem) string {
	venue := "视频评论区"
	switch {
	case item.Venue == "dynamic" || item.Type == 17 || item.Type == 11:
		venue = "动态区"
	case item.Venue == "article" || item.Type == 12:
		venue = "专栏评论区"
	}
	if item.Title != "" {
		return "《" + item.Title + "》的" + venue
	}
	return venue
}

// inboxNote 互动类型补充（写记忆用）。
func inboxNote(item inboxItem) string {
	switch item.Category {
	case notifyAt:
		return "有人 @ 我"
	case notifyVideoComment:
		return "有人评论我的视频"
	case notifyDynamic:
		return "有人评论我的动态"
	default:
		return "回复我的评论"
	}
}
