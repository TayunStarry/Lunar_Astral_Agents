package napcat

// music.go QQ 语音条出站（传输适配）
//
// 演奏音频由 fluidsynth 渲染为 44.1kHz 双声道 WAV（约 7MB / 40 秒）。
// NapCat 的 record 段（OneBot voice）在发送前会自行把非 silk 音频转成 NTSilk
// 并读取时长，因此这里不额外引入 ffmpeg / silk 编码器：直接把本机路径交给 NapCat。
//
// 本文件只回答「怎么把一个 record 段送到 NapCat」；语音条失败后降级为文字 / 文件
// 还是只保留链接，属于投递策略，由调用方（agents/DialogueBridging）决定。

import (
	"fmt"
	"strings"
)

// SendGroupRecordMessage 发送群语音条（音频转换与时长读取由 NapCat 完成）
func SendGroupRecordMessage(groupID int64, audio string) error {
	return sendRecordMessage("/send_group_msg", map[string]any{"group_id": groupID}, audio)
}

// SendPrivateRecordMessage 发送私聊语音条
func SendPrivateRecordMessage(userID int64, audio string) error {
	return sendRecordMessage("/send_private_msg", map[string]any{"user_id": userID}, audio)
}

// sendRecordMessage 组装 record 段并调用现有发送接口。
// 语音条需要 NapCat 先把 WAV 转成 silk，7MB / 40 秒的演奏可能超过普通接口的
// 10 秒超时，因此复用文件上传的长超时客户端。
func sendRecordMessage(action string, target map[string]any, audio string) error {
	source, ok := AudioSource(audio)
	if !ok {
		return fmt.Errorf("演奏音频不可读: %s", strings.TrimSpace(audio))
	}
	params := map[string]any{
		"message": []map[string]any{
			{"type": "record", "data": map[string]string{"file": source}},
		},
	}
	for key, value := range target {
		params[key] = value
	}
	client := Default()
	_, err := client.callAPIWithClient(action, params, client.uploadHTTPClient)
	return err
}
