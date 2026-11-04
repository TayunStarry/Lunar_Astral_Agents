package DialogueBridging

// 一轮对话里可能先后出现三类助手消息：快速应答旁白、模型工具轮次正文、终稿。
// 本用例锁定它们共享 currentTarget 时的目标一致性与一次性份额归属。

import (
	"encoding/json"
	"strings"
	"testing"
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

// sentText 拼接出站消息段中的全部文本
func sentText(segments []MessageSegment) string {
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
func atQQ(segments []MessageSegment) string {
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
