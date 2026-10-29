package DialogueBridging

// tools.go 通讯桥接智能体的私有工具：解析目标、校验句柄、发送文本/图片/文件。
//
// 这些工具不暴露给对话者：对话者只能通过 network_explore(qq_send) 提交自然语言
// 需求，由本智能体决定目标与投递方式，保证「发到哪里、发了什么」可控且可回报。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// executePlan 依次执行多目标投递计划，并汇总每个目标的成败。
func executePlan(plan sendPlan, instruction string, groups []QQGroup, friends []QQFriend) (string, error) {
	if len(plan.Deliveries) == 0 {
		reason := plan.Reason
		if reason == "" {
			reason = "没有解析出明确的投递目标"
		}
		return "未执行：" + reason, nil
	}

	total := len(plan.Deliveries)
	reports := make([]string, 0, total)
	successes := 0
	blocked := 0
	for index, delivery := range plan.Deliveries {
		report, ok := executeDelivery(delivery, instruction, groups, friends)
		if ok {
			successes++
		}
		if strings.Contains(report, whitelistBlockedText) {
			blocked++
		}
		if total > 1 {
			report = fmt.Sprintf("目标 %d/%d：\n%s", index+1, total, report)
		}
		reports = append(reports, report)
	}

	text := strings.Join(reports, "\n\n")
	if successes == 0 {
		// 白名单拦截是预期的「未执行」结果，不是系统故障：原样返回提示文本，
		// 由上层模型把确认引导转告对话者。
		if blocked == total {
			return text, nil
		}
		return text, fmt.Errorf("全部投递失败")
	}
	// 部分成功不视为整体失败：报告里已逐项写明哪些目标未完成。
	return text, nil
}

// executeDelivery 执行单个目标的投递，返回报告与该目标是否至少成功投递一项。
func executeDelivery(delivery sendDelivery, instruction string, groups []QQGroup, friends []QQFriend) (string, bool) {
	plan := delivery.asPlan()
	if plan.Target == "" {
		reason := plan.Reason
		if reason == "" {
			reason = "没有解析出明确的投递目标"
		}
		return "未执行：" + reason, false
	}
	target, err := resolveTarget(plan, groups, friends)
	if err != nil {
		return "未执行：目标解析失败：" + err.Error(), false
	}
	// 白名单闸门：目标不在 bridging_users 时一律不发送，只返回固定提示并等待确认。
	if allowed, message := authorizeActiveSend(target, instruction); !allowed {
		return message, false
	}
	if plan.Text == "" && len(plan.Images) == 0 && len(plan.Files) == 0 {
		return "未执行：没有可发送的内容（文本 / 图片 / 文件都为空）", false
	}

	images, files, failures := resolveAttachments(plan, instruction)
	outcome := deliveryOutcome{Failures: failures}

	if plan.Text != "" {
		if err := sendText(target, plan.Text); err != nil {
			outcome.Failures = append(outcome.Failures, "文本："+err.Error())
		} else {
			outcome.TextSent = true
		}
	}
	for start := 0; start < len(images); start += maxSendImages {
		end := min(start+maxSendImages, len(images))
		batch := images[start:end]
		paths := make([]string, 0, len(batch))
		for _, image := range batch {
			paths = append(paths, image.Path)
		}
		if err := sendImages(target, paths); err != nil {
			outcome.Failures = append(outcome.Failures, fmt.Sprintf("图片 %s：%v", joinHandles(batch), err))
			continue
		}
		outcome.ImagesSent += len(batch)
	}
	for _, file := range files {
		if err := sendFile(target, file.Path, file.Name); err != nil {
			outcome.Failures = append(outcome.Failures, fmt.Sprintf("文件 %s：%v", file.Handle, err))
			continue
		}
		outcome.FilesSent++
	}

	report := outcome.report(target)
	ok := outcome.TextSent || outcome.ImagesSent > 0 || outcome.FilesSent > 0
	return report, ok
}

// asPlan 把单个投递目标转成旧的 sendPlan 视图，复用已有的目标解析与附件校验。
func (delivery sendDelivery) asPlan() sendPlan {
	return sendPlan{
		TargetType: delivery.TargetType,
		Target:     delivery.Target,
		Text:       delivery.Text,
		Images:     delivery.Images,
		Files:      delivery.Files,
		Reason:     delivery.Reason,
	}
}

// attachment 已解析的句柄附件
type attachment struct {
	Handle string
	Path   string
	Name   string
	Kind   string
}

// resolveAttachments 校验图片/文件句柄，返回可投递附件与失败说明。
//
// 句柄必须出现在对话者需求里：模型偶尔会把上下文里见过的句柄也塞进计划，
// 这里拒绝「需求里没提过」的句柄，避免误发无关资源。
func resolveAttachments(plan sendPlan, instruction string) (images, files []attachment, failures []string) {
	if len(plan.Files) > maxSendFiles {
		failures = append(failures, fmt.Sprintf("文件数量 %d 超过单次上限 %d", len(plan.Files), maxSendFiles))
		plan.Files = plan.Files[:maxSendFiles]
	}
	for _, raw := range plan.Images {
		handle := cleanHandle(raw)
		if !handleMentioned(handle, instruction) {
			failures = append(failures, handle+"：需求中没有提到这个句柄")
			continue
		}
		item, err := resolveResource(handle)
		if err != nil {
			failures = append(failures, handle+"："+err.Error())
			continue
		}
		if item.Kind != resource.KindImage {
			failures = append(failures, handle+"：不是图片资源")
			continue
		}
		images = append(images, item)
	}
	for _, raw := range plan.Files {
		handle := cleanHandle(raw)
		if !handleMentioned(handle, instruction) {
			failures = append(failures, handle+"：需求中没有提到这个句柄")
			continue
		}
		item, err := resolveResource(handle)
		if err != nil {
			failures = append(failures, handle+"："+err.Error())
			continue
		}
		files = append(files, item)
	}
	return images, files, failures
}

// cleanHandle 去掉模型偶尔包在句柄外的引号/括号/逗号
func cleanHandle(raw string) string {
	return strings.Trim(strings.TrimSpace(raw), "\"'`「」『』《》[]()（）.,;；，。、 ")
}

// resolveTarget 把计划中的目标解析为群号 / QQ 号；名称必须唯一匹配。
func resolveTarget(plan sendPlan, groups []QQGroup, friends []QQFriend) (resolvedTarget, error) {
	kind := strings.ToLower(strings.TrimSpace(plan.TargetType))
	raw := strings.Trim(strings.TrimSpace(plan.Target), "「」『』《》\"'`[]()（） ")
	if kind != "group" && kind != "user" {
		kind = inferTargetKind(raw, groups, friends)
	}
	switch kind {
	case "group":
		group, err := matchGroup(raw, groups)
		if err != nil {
			return resolvedTarget{}, err
		}
		return resolvedTarget{Kind: "group", ID: group.GroupID, Name: group.GroupName}, nil
	case "user":
		friend, err := matchFriend(raw, friends)
		if err != nil {
			return resolvedTarget{}, err
		}
		name := strings.TrimSpace(friend.Remark)
		if name == "" {
			name = strings.TrimSpace(friend.Nickname)
		}
		return resolvedTarget{Kind: "user", ID: friend.UserID, Name: name}, nil
	default:
		return resolvedTarget{}, fmt.Errorf("无法确定目标类型：%s", raw)
	}
}

// inferTargetKind 模型没给类型时，用号码/名称在两边目录中的唯一匹配推断。
func inferTargetKind(raw string, groups []QQGroup, friends []QQFriend) string {
	if id, ok := parseID(raw); ok {
		for _, group := range groups {
			if group.GroupID == id {
				return "group"
			}
		}
		for _, friend := range friends {
			if friend.UserID == id {
				return "user"
			}
		}
	}
	// 先看精确同名：避免「测试群」被包含匹配到「测试群二」而误判歧义
	groupExact, friendExact := countGroupExact(raw, groups), countFriendExact(raw, friends)
	switch {
	case groupExact == 1 && friendExact == 0:
		return "group"
	case friendExact == 1 && groupExact == 0:
		return "user"
	case groupExact+friendExact > 1:
		return ""
	}
	// 没有精确同名时再退到包含匹配
	inGroup := countGroupMatches(raw, groups)
	inFriend := countFriendMatches(raw, friends)
	switch {
	case inGroup == 1 && inFriend == 0:
		return "group"
	case inFriend == 1 && inGroup == 0:
		return "user"
	default:
		return ""
	}
}

// matchGroup 按号码或名称定位唯一群
func matchGroup(raw string, groups []QQGroup) (QQGroup, error) {
	if id, ok := parseID(raw); ok {
		for _, group := range groups {
			if group.GroupID == id {
				return group, nil
			}
		}
		return QQGroup{}, fmt.Errorf("群 %d 不在可投递列表中", id)
	}
	var matched []QQGroup
	for _, group := range groups {
		if strings.EqualFold(strings.TrimSpace(group.GroupName), raw) {
			matched = append(matched, group)
		}
	}
	if len(matched) == 0 {
		for _, group := range groups {
			if containsFold(group.GroupName, raw) {
				matched = append(matched, group)
			}
		}
	}
	switch len(matched) {
	case 1:
		return matched[0], nil
	case 0:
		return QQGroup{}, fmt.Errorf("找不到群「%s」", raw)
	default:
		return QQGroup{}, fmt.Errorf("群「%s」匹配到 %d 个，请提供群号", raw, len(matched))
	}
}

// matchFriend 按号码或名称（备注/昵称）定位唯一好友
func matchFriend(raw string, friends []QQFriend) (QQFriend, error) {
	if id, ok := parseID(raw); ok {
		for _, friend := range friends {
			if friend.UserID == id {
				return friend, nil
			}
		}
		return QQFriend{}, fmt.Errorf("好友 %d 不在可投递列表中", id)
	}
	var matched []QQFriend
	for _, friend := range friends {
		if friendNameEquals(friend, raw) {
			matched = append(matched, friend)
		}
	}
	if len(matched) == 0 {
		for _, friend := range friends {
			if containsFold(friend.Remark, raw) || containsFold(friend.Nickname, raw) {
				matched = append(matched, friend)
			}
		}
	}
	switch len(matched) {
	case 1:
		return matched[0], nil
	case 0:
		return QQFriend{}, fmt.Errorf("找不到好友「%s」", raw)
	default:
		return QQFriend{}, fmt.Errorf("好友「%s」匹配到 %d 个，请提供 QQ 号", raw, len(matched))
	}
}

// countGroupExact / countFriendExact 精确同名计数
func countGroupExact(raw string, groups []QQGroup) int {
	count := 0
	for _, group := range groups {
		if strings.EqualFold(strings.TrimSpace(group.GroupName), raw) {
			count++
		}
	}
	return count
}

func countFriendExact(raw string, friends []QQFriend) int {
	count := 0
	for _, friend := range friends {
		if friendNameEquals(friend, raw) {
			count++
		}
	}
	return count
}

// countGroupMatches / countFriendMatches 包含匹配计数（用于类型推断）
func countGroupMatches(raw string, groups []QQGroup) int {
	count := 0
	for _, group := range groups {
		if containsFold(group.GroupName, raw) {
			count++
		}
	}
	return count
}

func countFriendMatches(raw string, friends []QQFriend) int {
	count := 0
	for _, friend := range friends {
		if containsFold(friend.Remark, raw) || containsFold(friend.Nickname, raw) {
			count++
		}
	}
	return count
}

func friendNameEquals(friend QQFriend, raw string) bool {
	return strings.EqualFold(strings.TrimSpace(friend.Nickname), raw) ||
		strings.EqualFold(strings.TrimSpace(friend.Remark), raw)
}

// resolveResource 句柄 → 本地绝对路径（图片与文件共用）
func resolveResource(handle string) (attachment, error) {
	res, err := resource.Resolve(handle)
	if err != nil {
		return attachment{}, err
	}
	if res.State != resource.StateActive {
		return attachment{}, fmt.Errorf("%s 的本体已丢失", handle)
	}
	abs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(res.BlobPath))
	// config.LocalDir 默认是相对的（local_data），而 NapCat 进程的工作目录与本进程不同：
	// 必须交出绝对路径，否则 NapCat 按自己的 cwd 解析会报文件不存在（上传瞬间失败）。
	if resolved, absErr := filepath.Abs(abs); absErr == nil {
		abs = resolved
	}
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		return attachment{}, fmt.Errorf("%s 的文件不可读", handle)
	}
	name := strings.TrimSpace(res.Name)
	if name == "" {
		name = filepath.Base(abs)
	}
	return attachment{Handle: handle, Path: abs, Name: name, Kind: res.Kind}, nil
}

// handleMentioned 需求里是否明确提到该句柄（大小写不敏感，忽略行/段锚点）
func handleMentioned(handle, instruction string) bool {
	handle = strings.TrimSpace(handle)
	if handle == "" {
		return false
	}
	base := handle
	if index := strings.IndexAny(base, "|"); index >= 0 {
		base = base[:index]
	}
	upper := strings.ToUpper(instruction)
	return strings.Contains(upper, strings.ToUpper(handle)) ||
		strings.Contains(upper, strings.ToUpper(base))
}

// sendText 按目标类型发送纯文本
func sendText(target resolvedTarget, text string) error {
	if target.Kind == "group" {
		return SendGroupPlainTextMessage(target.ID, text)
	}
	return SendPrivateTextMessage(target.ID, text)
}

// sendImages 按目标类型发送图片（本地绝对路径由 napcat 归一为可上传地址）
func sendImages(target resolvedTarget, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	if target.Kind == "group" {
		return SendGroupImageMessage(target.ID, paths)
	}
	return SendPrivateImageMessage(target.ID, paths)
}

// sendFile 按目标类型上传文件
func sendFile(target resolvedTarget, path, name string) error {
	if target.Kind == "group" {
		return SendGroupFile(target.ID, path, name)
	}
	return SendPrivateFile(target.ID, path, name)
}

// report 组装给对话者的简要报告
func (outcome deliveryOutcome) report(target resolvedTarget) string {
	kind := "群"
	if target.Kind == "user" {
		kind = "好友"
	}
	sent := make([]string, 0, 3)
	if outcome.TextSent {
		sent = append(sent, "文本 1 条")
	}
	if outcome.ImagesSent > 0 {
		sent = append(sent, fmt.Sprintf("图片 %d 张", outcome.ImagesSent))
	}
	if outcome.FilesSent > 0 {
		sent = append(sent, fmt.Sprintf("文件 %d 个", outcome.FilesSent))
	}
	summary := "无"
	if len(sent) > 0 {
		summary = strings.Join(sent, "、")
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "已向%s「%s」(%d) 投递：%s。", kind, target.Name, target.ID, summary)
	if len(outcome.Failures) > 0 {
		builder.WriteString("\n未完成：\n- ")
		builder.WriteString(strings.Join(outcome.Failures, "\n- "))
	}
	return builder.String()
}

// joinHandles 把一批附件句柄拼成简短列表（失败报告用）
func joinHandles(items []attachment) string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Handle)
	}
	return strings.Join(names, "、")
}

// parseID 解析纯数字号码
func parseID(raw string) (int64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	var id int64
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, false
		}
		id = id*10 + int64(r-'0')
	}
	return id, id > 0
}

// containsFold 子串匹配（大小写不敏感，兼容空串）
func containsFold(text, sub string) bool {
	sub = strings.TrimSpace(sub)
	if sub == "" {
		return false
	}
	return strings.Contains(strings.ToLower(text), strings.ToLower(sub))
}
