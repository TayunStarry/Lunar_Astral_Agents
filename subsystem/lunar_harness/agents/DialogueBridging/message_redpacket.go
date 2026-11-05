package DialogueBridging

// 消息解析与单线程收发流程（按职责拆分）

import (
	"LunarSubsystem/LunarHarness/clients/napcat"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

func parseRawRedPacket(raw json.RawMessage) *RedPacketInfo {
	if len(raw) == 0 {
		return nil
	}
	var payload struct {
		Elements []struct {
			Wallet *napcat.WalletElement `json:"walletElement"`
		} `json:"elements"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	for _, e := range payload.Elements {
		if e.Wallet == nil {
			continue
		}
		return &RedPacketInfo{
			IsRedPacket: true,
			IsPhrase:    e.Wallet.RedChannel == 32 || e.Wallet.MsgType == 6,
			Blessing:    walletBlessing(e.Wallet),
			BillNo:      e.Wallet.BillNo,
		}
	}
	return nil
}

// walletBlessing 提取红包祝福语：receiver.title 优先，notice 去掉 "[QQ红包]" 前缀兜底
func walletBlessing(w *napcat.WalletElement) string {
	if w.Receiver.Title != "" {
		return w.Receiver.Title
	}
	if notice := strings.TrimPrefix(w.Receiver.Notice, "[QQ红包]"); notice != "" {
		return notice
	}
	return w.Receiver.Content
}

// buildRedPacketText 将红包信息渲染为文本（口令红包附带复读领取指令）
func buildRedPacketText(rp *RedPacketInfo) string {
	if rp.IsPhrase {
		return fmt.Sprintf("[口令红包] 口令：『%s』，请直接复读口令领取红包 ", rp.Blessing)
	}
	return fmt.Sprintf("[红包] 祝福语: %s ", rp.Blessing)
}

// randomRedPacketDelay 领取口令红包前的随机等待时间（0.5~3 秒）
func randomRedPacketDelay() time.Duration {
	return time.Duration(500+rand.Intn(2500)) * time.Millisecond
}

// resolveNickname 解析私聊用户昵称：优先 sender.nickname，其次 card，最后通过接口查询
