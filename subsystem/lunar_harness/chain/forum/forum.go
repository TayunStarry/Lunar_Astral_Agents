package forum

// forum.go 论坛链：互助论坛的多轮调度、上下文与状态推进。
//
// 分层职责：
//   - 本链保留：整体超时、最近 40 条「需求 + 答复」上下文、上下文导出/修复；
//   - 写路径串行化不在此层：发帖/回复/轮次/申领统一由 engine/forum 的唯一写锁保证，
//     自动回应与工具链拿到的是同一把锁，因此本链不再持有额外的运行互斥锁；
//   - 模型调用、工具声明与实现、实况上报留在 agents/MutualForum（RunRound）；
//   - 本链不内嵌任何模型调用。

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/agents/MutualForum"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

const (
	// runTimeout 单次需求的整体超时
	runTimeout = 2 * time.Minute
	// historyLimitMessages 独立上下文保留的消息条数（约 20 轮问答）
	historyLimitMessages = 40
	// historyInstructionRunes 写入上下文时单条需求的字符上限
	historyInstructionRunes = 3000
	// historyAnswerRunes 写入上下文时单条答复的字符上限
	historyAnswerRunes = 4000
)

var (
	// historyMu 保护独立上下文
	historyMu sync.Mutex
	// history 最近若干轮「用户需求 + 最终答复」
	history []agentbase.Message

	// runRound 单轮模型调用入口；测试可替换为桩函数以隔离模型与论坛服务。
	runRound = MutualForum.RunRound
)

// Ready 返回互助论坛是否就绪（桥接层健康检查）。
func Ready() bool {
	return MutualForum.Ready()
}

// Run 接收对话者的自然语言需求：执行一轮模型调用，并把问答写入上下文。
// 具体写操作由 agents/MutualForum 内的唯一写锁串行化，不在此处整轮持锁。
func Run(instruction string) (string, error) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return "", errors.New("请用自然语言说明要在论坛里做什么")
	}

	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	text, err := runRound(ctx, instruction, HistorySnapshot())
	if err != nil {
		return "", err
	}
	Remember(instruction, text)
	return text, nil
}

// Remember 记录一轮问答；超出上限时裁掉最早的消息。
func Remember(instruction, answer string) {
	historyMu.Lock()
	defer historyMu.Unlock()
	history = append(history,
		agentbase.Message{Role: "user", Content: agentbase.TruncateRunes(instruction, historyInstructionRunes)},
		agentbase.Message{Role: "assistant", Content: agentbase.TruncateRunes(answer, historyAnswerRunes)},
	)
	if len(history) > historyLimitMessages {
		history = history[len(history)-historyLimitMessages:]
	}
}

// HistorySnapshot 返回历史上下文的副本。
func HistorySnapshot() []agentbase.Message {
	historyMu.Lock()
	defer historyMu.Unlock()
	if len(history) == 0 {
		return nil
	}
	out := make([]agentbase.Message, len(history))
	copy(out, history)
	return out
}

// ResetContext 清空独立上下文。
func ResetContext() {
	historyMu.Lock()
	history = nil
	historyMu.Unlock()
}
