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
	if SendAudioToAgent != nil && len(req.AudioURLs) > 0 {
		SendAudioToAgent(req.AudioURLs)
	}
}

// parseMessageSegments 解析消息段列表，返回 (内容, 是否含图片, 语音/音频地址列表)
//
// 纯文本返回 string，包含图片返回 []map[string]any；视频与文件在解析阶段即收进
// 资源归档层、正文只写规范句柄（句柄不可得时退化为文件名），语音地址另行返回，
// 由智能体侧的语音管线完成入库与转写。
// groupID 用于 @ 目标与回复引用的成员名称解析（私聊传 0）
