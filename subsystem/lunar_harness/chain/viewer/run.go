package viewer

// run.go 评论区与动态接口：读取评论、发布动态、在自己的动态评论区留言。
//
// 看视频分支在自己的动态评论区与视频评论区发言；被动收到 @/追评时统一直接走评论接口回应。

import (
	"strings"

	"LunarSubsystem/LunarHarness/engine/bilibili"
)

// postCommentOnObject 在指定评论区发一条根评论（type：1=视频，11=图文动态，17=纯文字动态）。
// targets 非空时写入 at_name/at_mid，产生真实的 @ 通知。
//
// 与消息回应共用 postReplyAPI，不维护第二套接口发布实现。
func (w *webSession) postCommentOnObject(typ int, oid, message string, targets []MentionTarget) error {
	return w.postReplyAPI(typ, oid, message, "", "", targets)
}

// commentFingerprint 取评论首行的前 24 个字作为回读指纹（视频评论即 URL）。
func commentFingerprint(message string) string {
	line := strings.TrimSpace(strings.SplitN(message, "\n", 2)[0])
	runes := []rune(line)
	if len(runes) > 24 {
		runes = runes[:24]
	}
	return normalizeInline(string(runes))
}

// normalizeInline 去掉空白，避免回读文本的换行/空格导致指纹对不上。
func normalizeInline(text string) string {
	return strings.Join(strings.Fields(text), "")
}

// publishDiary 把这一轮的观影感受作为纯文本动态发布（type=4）。
func (w *webSession) publishDiary(text string) (string, error) {
	return bilibili.PublishDynamic(w, text)
}

// finishComment 收束评论长度，但不从句子中间硬切。
//
// 评论宁可短，也要是完整的一句话：超长时退回到最近的一个句末标点收尾；
// 没有句末标点再退到逗号；只有在整段没有任何标点时才原样截断。
func finishComment(text string, maxRunes int) string {
	text = strings.TrimSpace(strings.Trim(text, `"'“”「」『』`))
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	cut := runes[:maxRunes]
	for i := len(cut) - 1; i >= 0; i-- {
		switch cut[i] {
		case '。', '！', '？', '!', '?', '…', '~', '～', '\n':
			return strings.TrimSpace(string(cut[:i+1]))
		}
	}
	for i := len(cut) - 1; i >= 0; i-- {
		switch cut[i] {
		case '，', '；', '、', ',', ';':
			return strings.TrimSpace(string(cut[:i+1]))
		}
	}
	return strings.TrimSpace(string(cut))
}
