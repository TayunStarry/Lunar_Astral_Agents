package DialogueBridging

// 消息解析与单线程收发流程（按职责拆分）

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

func enqueueRequest(req BridgeRequest) {
	flowMutex.Lock()
	requestQueue = append(requestQueue, req)
	flowMutex.Unlock()
	pumpNext()
}

// pumpNext 在空闲时（无待回应请求）推送队列头的请求给月华
func pumpNext() {
	flowMutex.Lock()
	defer flowMutex.Unlock()

	if awaitingResponse || len(requestQueue) == 0 {
		return
	}

	req := requestQueue[0]
	requestQueue = requestQueue[1:]
	currentTarget = req.Target
	awaitingResponse = true

	if SendMessageToAgent != nil {
		SendMessageToAgent(req.Messages)
	} else {
		awaitingResponse = false
		logger.SubError("LunarCore", "Napcat", "SendMessageToAgent 回调未注册，无法推送消息")
	}
	if SendAudioToAgent != nil && len(req.AudioObjects) > 0 {
		SendAudioToAgent(req.AudioObjects)
	}
}

// parseMessageSegments 解析消息段列表，返回 (内容, 是否含多模态内容项, 音频对象列表)
//
// 纯文本返回 string，包含图片/视频/动图返回 []map[string]any；媒体一律按来源方
// 声明的类型写成多模态对象，音频对象另行返回，由智能体侧的语音管线完成入库与转写。
// groupID 用于 @ 目标与回复引用的成员名称解析（私聊传 0）
