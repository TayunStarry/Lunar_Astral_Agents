package DialogueBridging

// 一轮对话里可能先后出现三类助手消息：快速应答旁白、模型工具轮次正文、终稿。
// 本用例锁定它们共享 currentTarget 时的目标一致性与一次性份额归属。

import (
	"encoding/json"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/clients/napcat"
)

// TestTurnMessagesShareGroupTarget 三条推送（快速应答 / 工具轮次正文 / 终稿）
// 必须全部落在同一个群，且前两条不消费一次性 @ 回执，终稿仍 @ 发起者。
//
// 关键回归点：中间正文若以 auto=false 推送，会提前置位 responseStarted 并清空
// ReplyToUserID，NotifyAgentIdle 就会在终稿落地前推进队列，让下一个请求顶替
// currentTarget——终稿会被投到别的群。本用例同时断言「队列未提前推进」。
func TestTurnMessagesShareGroupTarget(t *testing.T) {
	const (
		groupID  = int64(10001)
		userID   = int64(10002)
		otherGID = int64(10003)
	)
	fake, delivered := startBridgeTest(t, groupID)
	setBridgeState(BridgeConnected)
	origTarget := currentTarget
	t.Cleanup(func() {
		setBridgeState(BridgeDisconnected)
		flowMutex.Lock()
		currentTarget = origTarget
		flowMutex.Unlock()
	})

	// 群 A 的 @ 请求正在等待回应，队列里还压着群 B 的请求
	flowMutex.Lock()
	awaitingResponse = true
	responseStarted = false
	currentTarget = BridgeTarget{ID: groupID, IsGroup: true, GroupName: "测试群", ReplyToUserID: userID}
	requestQueue = []BridgeRequest{{Target: BridgeTarget{ID: otherGID, IsGroup: true, GroupName: "其他群"}}}
	flowMutex.Unlock()

	flowState := func() (target BridgeTarget, awaiting, started bool) {
		flowMutex.Lock()
		defer flowMutex.Unlock()
		return currentTarget, awaitingResponse, responseStarted
	}

	// 1) 快速应答旁白；2) 模型工具轮次正文——都必须是「未收尾」消息
	HandleAgentResponse("text", "我先看看", true)
	HandleAgentResponse("text", "正在查资料，稍等", true)

	for _, step := range []string{"快速应答", "工具轮次正文"} {
		target, _, started := flowState()
		if started {
			t.Fatalf("%s 不应置位 responseStarted（提前释放应答份额会让终稿投错会话）", step)
		}
		if target.ID != groupID || !target.IsGroup {
			t.Fatalf("%s 改写了目标会话：%+v", step, target)
		}
		if target.ReplyToUserID != userID {
			t.Fatalf("%s 不应消费一次性 @ 回执：%+v", step, target)
		}
	}

	// 旁白阶段收到空闲信号：队列不得推进到群 B
	NotifyAgentIdle()
	if target, _, _ := flowState(); target.ID != groupID {
		t.Fatalf("终稿落地前不应推进队列，当前目标已变为 %+v", target)
	}
	if *delivered != 0 {
		t.Fatalf("终稿落地前不应向智能体推送下一条请求，实际 %d 次", *delivered)
	}

	// 3) 终稿：正常路径，消费 @ 回执并置位 responseStarted
	HandleAgentResponse("text", "查到了：今天晴", false)

	sends := fake.sentMessages()
	if len(sends) != 3 {
		t.Fatalf("应投递 3 条消息，实际 %d 条: %+v", len(sends), sends)
	}
	wantTexts := []string{"我先看看", "正在查资料，稍等", "查到了：今天晴"}
	for i, send := range sends {
		if send.Action != "/send_group_msg" || send.GroupID != groupID {
			t.Fatalf("第 %d 条投递目标不符：action=%s group=%d user=%d", i+1, send.Action, send.GroupID, send.UserID)
		}
		if got := sentText(send.Segments); got != wantTexts[i] {
			t.Fatalf("第 %d 条正文不符：%q != %q", i+1, got, wantTexts[i])
		}
	}
	// 前两条是尚未收尾的旁白，不得抢走 @；终稿恰好带一次 @ 发起者
	if atQQ(sends[0].Segments) != "" || atQQ(sends[1].Segments) != "" {
		t.Fatalf("旁白不应携带 @ 回执：第一条=%q 第二条=%q", atQQ(sends[0].Segments), atQQ(sends[1].Segments))
	}
	if got := atQQ(sends[2].Segments); got != "10002" {
		t.Fatalf("终稿应 @ 发起者 10002，实际 %q", got)
	}

	// 终稿之后空闲信号才允许推进队列：下一条请求的群 B 成为新目标
	NotifyAgentIdle()
	if target, awaiting, started := flowState(); target.ID != otherGID || !awaiting || started {
		t.Fatalf("终稿后应推进到群 B 并重新等待回应，实际 target=%+v awaiting=%v started=%v", target, awaiting, started)
	}
	if *delivered != 1 {
		t.Fatalf("终稿后应向智能体投递下一条请求 1 次，实际 %d 次", *delivered)
	}
}

// TestQuoteCandidatesOnlyConsumedByFinalReply 随机触发的引用候选只能被终稿消费一次：
// 快速应答 / 工具轮次正文既不能抢引用，也不能把候选清空。
func TestQuoteCandidatesOnlyConsumedByFinalReply(t *testing.T) {
	const (
		groupID = int64(10001)
		quoteID = int64(9001)
	)
	fake, _ := startBridgeTest(t, groupID)
	setBridgeState(BridgeConnected)
	origTarget := currentTarget
	t.Cleanup(func() {
		setBridgeState(BridgeDisconnected)
		flowMutex.Lock()
		currentTarget = origTarget
		flowMutex.Unlock()
	})

	flowMutex.Lock()
	awaitingResponse = true
	responseStarted = false
	currentTarget = BridgeTarget{
		ID:        groupID,
		IsGroup:   true,
		GroupName: "测试群",
		QuoteCandidates: []QuoteCandidate{
			{MessageID: quoteID, Content: "今天好热啊"},
			{MessageID: 9002, Content: "明天要下雨"},
		},
	}
	flowMutex.Unlock()

	HandleAgentResponse("text", "我先看看", true)
	flowMutex.Lock()
	pending := len(currentTarget.QuoteCandidates)
	flowMutex.Unlock()
	if pending != 2 {
		t.Fatalf("快速应答不应消费引用候选，剩余 %d 条", pending)
	}

	HandleAgentResponse("text", "今天真的好热", false)
	flowMutex.Lock()
	pending = len(currentTarget.QuoteCandidates)
	flowMutex.Unlock()
	if pending != 0 {
		t.Fatalf("终稿应消费引用候选，剩余 %d 条", pending)
	}

	sends := fake.sentMessages()
	if len(sends) != 2 {
		t.Fatalf("应投递 2 条消息，实际 %d 条", len(sends))
	}
	if got := replyID(sends[0].Segments); got != "" {
		t.Fatalf("快速应答不应携带引用，实际引用 %q", got)
	}
	// 引用由 embedding 相关性挑选；测试环境可能无嵌入服务而退化为最新一条，
	// 这里只锁定「必须来自本轮候选」，不锁定具体选哪条。
	if got := replyID(sends[1].Segments); got != "9001" && got != "9002" {
		t.Fatalf("终稿应引用本轮候选消息，实际 %q", got)
	}
}

// TestPrivateTurnKeepsSingleTarget 私聊路径同样只允许终稿收尾：
// 中间旁白不置位、不推进队列，终稿落到发起私聊的用户。
func TestPrivateTurnKeepsSingleTarget(t *testing.T) {
	const (
		userID  = int64(20001)
		otherID = int64(20002)
	)
	fake, delivered := startBridgeTest(t, userID)
	setBridgeState(BridgeConnected)
	origTarget := currentTarget
	t.Cleanup(func() {
		setBridgeState(BridgeDisconnected)
		flowMutex.Lock()
		currentTarget = origTarget
		flowMutex.Unlock()
	})

	flowMutex.Lock()
	awaitingResponse = true
	responseStarted = false
	currentTarget = BridgeTarget{ID: userID, IsGroup: false}
	requestQueue = []BridgeRequest{{Target: BridgeTarget{ID: otherID, IsGroup: false}}}
	flowMutex.Unlock()

	HandleAgentResponse("text", "稍等", true)
	flowMutex.Lock()
	started := responseStarted
	flowMutex.Unlock()
	if started {
		t.Fatal("私聊旁白不应置位 responseStarted")
	}

	NotifyAgentIdle()
	if *delivered != 0 {
		t.Fatalf("终稿前不应推进私聊队列，实际投递 %d 次", *delivered)
	}

	HandleAgentResponse("text", "好了", false)
	sends := fake.sentMessages()
	if len(sends) != 2 {
		t.Fatalf("应投递 2 条私聊消息，实际 %d 条", len(sends))
	}
	for i, send := range sends {
		if send.Action != "/send_private_msg" || send.UserID != userID {
			t.Fatalf("第 %d 条私聊投递目标不符：action=%s user=%d", i+1, send.Action, send.UserID)
		}
	}

	NotifyAgentIdle()
	flowMutex.Lock()
	next := currentTarget
	flowMutex.Unlock()
	if next.ID != otherID || next.IsGroup {
		t.Fatalf("终稿后应推进到下一个私聊用户 %d，实际 %+v", otherID, next)
	}
}

// TestMusicMessageIsMidTurnProduct 音乐在工具轮次中途产出，本轮收尾仍是终稿文本：
// 音乐只落到发起群、不置位、不消费 @/引用；终稿才收尾并推进队列。
//
// 关键回归点：音乐若按终稿语义置位 responseStarted（或改造前无条件置位），空闲信号
// 会在终稿落地前释放应答份额，队列里的群 B 顶替 currentTarget，音乐/终稿都会错投。
func TestMusicMessageIsMidTurnProduct(t *testing.T) {
	const (
		groupID  = int64(10001)
		userID   = int64(10002)
		otherGID = int64(10003)
		quoteID  = int64(9001)
	)
	fake, delivered := startBridgeTest(t, groupID)
	setBridgeState(BridgeConnected)
	origTarget := currentTarget
	t.Cleanup(func() {
		setBridgeState(BridgeDisconnected)
		flowMutex.Lock()
		currentTarget = origTarget
		flowMutex.Unlock()
	})

	flowMutex.Lock()
	awaitingResponse = true
	responseStarted = false
	currentTarget = BridgeTarget{
		ID:            groupID,
		IsGroup:       true,
		GroupName:     "测试群",
		ReplyToUserID: userID,
		QuoteCandidates: []QuoteCandidate{
			{MessageID: quoteID, Content: "弹首歌听听"},
		},
	}
	requestQueue = []BridgeRequest{{Target: BridgeTarget{ID: otherGID, IsGroup: true, GroupName: "其他群"}}}
	flowMutex.Unlock()

	state := func() (BridgeTarget, bool, bool) {
		flowMutex.Lock()
		defer flowMutex.Unlock()
		return currentTarget, awaitingResponse, responseStarted
	}

	HandleAgentMusicResponse("/file/read/audios/test.wav", "测试小曲", "描述", 40.07)

	sends := fake.sentMessages()
	if len(sends) != 1 {
		t.Fatalf("音乐应投递 1 条消息，实际 %d 条: %+v", len(sends), sends)
	}
	if sends[0].Action != "/send_group_msg" || sends[0].GroupID != groupID {
		t.Fatalf("音乐投递目标不符：action=%s group=%d", sends[0].Action, sends[0].GroupID)
	}
	// 音乐不得抢占一次性份额，也不得改写目标状态
	target, _, started := state()
	if started {
		t.Fatal("工具轮次中的音乐不应置位 responseStarted")
	}
	if target.ID != groupID || target.ReplyToUserID != userID || len(target.QuoteCandidates) != 1 {
		t.Fatalf("音乐不应消费 @ 回执或引用候选：%+v", target)
	}

	NotifyAgentIdle()
	if target, _, _ := state(); target.ID != groupID {
		t.Fatalf("终稿落地前不应推进队列，当前目标已变为 %+v", target)
	}
	if *delivered != 0 {
		t.Fatalf("终稿落地前不应向智能体推送下一条请求，实际 %d 次", *delivered)
	}

	// 终稿：恰好消费一次 @ 与引用，并收尾本轮
	HandleAgentResponse("text", "弹好啦，听听看", false)
	sends = fake.sentMessages()
	if len(sends) != 2 {
		t.Fatalf("应投递 2 条消息（音乐 + 终稿），实际 %d 条: %+v", len(sends), sends)
	}
	if sends[1].Action != "/send_group_msg" || sends[1].GroupID != groupID {
		t.Fatalf("终稿投递目标不符：action=%s group=%d", sends[1].Action, sends[1].GroupID)
	}
	if got := atQQ(sends[1].Segments); got != "10002" {
		t.Fatalf("终稿应 @ 发起者 10002，实际 %q", got)
	}
	if got := replyID(sends[1].Segments); got != "9001" {
		t.Fatalf("终稿应引用候选消息 9001，实际 %q", got)
	}
	target, _, started = state()
	if !started {
		t.Fatal("终稿应置位 responseStarted")
	}
	if len(target.QuoteCandidates) != 0 || target.ReplyToUserID != 0 {
		t.Fatalf("终稿应恰好消费一次 @ 与引用，剩余 %+v", target)
	}

	NotifyAgentIdle()
	if target, awaiting, started := state(); target.ID != otherGID || !awaiting || started {
		t.Fatalf("终稿后应推进到群 B 并重新等待回应，实际 target=%+v awaiting=%v started=%v", target, awaiting, started)
	}
	if *delivered != 1 {
		t.Fatalf("终稿后应向智能体投递下一条请求 1 次，实际 %d 次", *delivered)
	}
}

// TestImageMessageIsMidTurnProduct 图片同理：截图、绘制结果与表情包都在工具轮次
// 中途产出，只快照目标，终稿文本才消费 @/引用并收尾。
func TestImageMessageIsMidTurnProduct(t *testing.T) {
	const (
		groupID  = int64(10001)
		userID   = int64(10002)
		otherGID = int64(10003)
		quoteID  = int64(9001)
	)
	fake, delivered := startBridgeTest(t, groupID)
	setBridgeState(BridgeConnected)
	origTarget := currentTarget
	t.Cleanup(func() {
		setBridgeState(BridgeDisconnected)
		flowMutex.Lock()
		currentTarget = origTarget
		flowMutex.Unlock()
	})

	flowMutex.Lock()
	awaitingResponse = true
	responseStarted = false
	currentTarget = BridgeTarget{
		ID:            groupID,
		IsGroup:       true,
		GroupName:     "测试群",
		ReplyToUserID: userID,
		QuoteCandidates: []QuoteCandidate{
			{MessageID: quoteID, Content: "看看这张图"},
		},
	}
	requestQueue = []BridgeRequest{{Target: BridgeTarget{ID: otherGID, IsGroup: true, GroupName: "其他群"}}}
	flowMutex.Unlock()

	state := func() (BridgeTarget, bool, bool) {
		flowMutex.Lock()
		defer flowMutex.Unlock()
		return currentTarget, awaitingResponse, responseStarted
	}

	HandleAgentImageResponse([]string{"http://example.com/a.png"})

	sends := fake.sentMessages()
	if len(sends) != 1 {
		t.Fatalf("图片应投递 1 条消息，实际 %d 条: %+v", len(sends), sends)
	}
	if sends[0].Action != "/send_group_msg" || sends[0].GroupID != groupID {
		t.Fatalf("图片投递目标不符：action=%s group=%d", sends[0].Action, sends[0].GroupID)
	}
	target, _, started := state()
	if started {
		t.Fatal("工具轮次中的图片不应置位 responseStarted")
	}
	if target.ID != groupID || target.ReplyToUserID != userID || len(target.QuoteCandidates) != 1 {
		t.Fatalf("图片不应消费 @ 回执或引用候选：%+v", target)
	}

	NotifyAgentIdle()
	if target, _, _ := state(); target.ID != groupID {
		t.Fatalf("终稿落地前不应推进队列，当前目标已变为 %+v", target)
	}
	if *delivered != 0 {
		t.Fatalf("终稿落地前不应向智能体推送下一条请求，实际 %d 次", *delivered)
	}

	HandleAgentResponse("text", "画好啦", false)
	sends = fake.sentMessages()
	if len(sends) != 2 {
		t.Fatalf("应投递 2 条消息（图片 + 终稿），实际 %d 条: %+v", len(sends), sends)
	}
	if sends[1].Action != "/send_group_msg" || sends[1].GroupID != groupID {
		t.Fatalf("终稿投递目标不符：action=%s group=%d", sends[1].Action, sends[1].GroupID)
	}
	if got := atQQ(sends[1].Segments); got != "10002" {
		t.Fatalf("终稿应 @ 发起者 10002，实际 %q", got)
	}
	if got := replyID(sends[1].Segments); got != "9001" {
		t.Fatalf("终稿应引用候选消息 9001，实际 %q", got)
	}
	target, _, started = state()
	if !started {
		t.Fatal("终稿应置位 responseStarted")
	}
	if len(target.QuoteCandidates) != 0 || target.ReplyToUserID != 0 {
		t.Fatalf("终稿应恰好消费一次 @ 与引用，剩余 %+v", target)
	}

	NotifyAgentIdle()
	if target, awaiting, started := state(); target.ID != otherGID || !awaiting || started {
		t.Fatalf("终稿后应推进到群 B 并重新等待回应，实际 target=%+v awaiting=%v started=%v", target, awaiting, started)
	}
	if *delivered != 1 {
		t.Fatalf("终稿后应向智能体投递下一条请求 1 次，实际 %d 次", *delivered)
	}
}

// replyID 取出出站消息段中的引用消息 ID，没有则返回空串
func replyID(segments []napcat.MessageSegment) string {
	for _, segment := range segments {
		if segment.Type != "reply" {
			continue
		}
		var data struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(segment.Data, &data) == nil {
			return data.ID
		}
	}
	return ""
}

// sentText 拼接出站消息段中的全部文本
func sentText(segments []napcat.MessageSegment) string {
	var sb strings.Builder
	for _, segment := range segments {
		if segment.Type != "text" {
			continue
		}
		var data struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(segment.Data, &data) == nil {
			sb.WriteString(data.Text)
		}
	}
	return strings.TrimSpace(sb.String())
}

// atQQ 取出出站消息段中第一个 @ 的目标 QQ 号，没有则返回空串
func atQQ(segments []napcat.MessageSegment) string {
	for _, segment := range segments {
		if segment.Type != "at" {
			continue
		}
		var data struct {
			QQ string `json:"qq"`
		}
		if json.Unmarshal(segment.Data, &data) == nil {
			return data.QQ
		}
	}
	return ""
}
