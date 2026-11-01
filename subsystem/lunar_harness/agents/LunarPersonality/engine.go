package LunarPersonality

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"sync"
	"time"

	broadcaster "LunarSubsystem/LunarHarness/engine/broadcaster"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	proxy "LunarSubsystem/LunarHarness/engine/console/proxy"
	"LunarSubsystem/LunarHarness/engine/memory"
	"LunarSubsystem/LunarHarness/transport/ws"
)

// ==== 引擎上行帧（ws 直连） ====

// lunarEngineChannels 月华自己使用的引擎频道。
//
// 渲染引擎（lunar.rendering_engine / lunar.live2d_engine）经各自 /ws 连接上行
// animation_list 与 telemetry；月华只接收这两个频道，工作室编辑器引擎用独立频道，
// 避免两边的动作列表与位置遥测互相覆盖。
var lunarEngineChannels = map[string]bool{
	"message-terminal-renderer": true, // 桌面消息终端
	"lunar-astral-renderer":     true, // 移动端
}

// HandleEngineFrame 处理引擎经 /ws 上行的原始帧（问答两端均为 ws，不再经 HTTP 中转）。
func HandleEngineFrame(channel string, raw []byte) {
	if !lunarEngineChannels[channel] {
		return
	}
	var env struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if json.Unmarshal(raw, &env) != nil {
		return
	}
	switch env.Type {
	case "animation_list":
		applyAnimationList(env.Payload)
	case "telemetry":
		applyTelemetry(env.Payload)
	}
}

// applyAnimationList 缓存引擎上报的可用动作。
func applyAnimationList(payload json.RawMessage) {
	var parsed struct {
		ActionDefinitions []ActionDefinition `json:"actionDefinitions"`
	}
	if json.Unmarshal(payload, &parsed) != nil || len(parsed.ActionDefinitions) == 0 {
		return
	}
	animMu.Lock()
	animActions = parsed.ActionDefinitions
	animUpdatedAt = time.Now().UnixMilli()
	animMu.Unlock()
	logger.Info(lunarCoreModule, "动画列表缓存已更新: %d 个动作", len(parsed.ActionDefinitions))
}

// applyTelemetry 缓存角色位置遥测。
func applyTelemetry(payload json.RawMessage) {
	var parsed struct {
		Character *struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
			Z float64 `json:"z"`
		} `json:"character"`
	}
	if json.Unmarshal(payload, &parsed) != nil || parsed.Character == nil {
		return
	}
	agentPositionMu.Lock()
	agentPosition = AgentPositionData{X: parsed.Character.X, Y: parsed.Character.Y, Z: parsed.Character.Z}
	agentPositionMu.Unlock()
}

// currentAgentPosition 获取缓存的 3D 位置
func currentAgentPosition() AgentPositionData {
	agentPositionMu.RLock()
	defer agentPositionMu.RUnlock()
	return agentPosition
}

// sendToEngine 向引擎发送命令
func sendToEngine(messageType, payloadJSON string) {
	var payload any
	if payloadJSON != "" && payloadJSON != "undefined" {
		if json.Unmarshal([]byte(payloadJSON), &payload) != nil {
			payload = payloadJSON
		}
	}
	command := EngineCommand{
		Type:      messageType,
		Source:    engineCommandSource,
		Payload:   payload,
		Timestamp: time.Now().UnixMilli(),
	}
	PushMessageFunc("engine", command)
	logger.Info(lunarCoreModule, "[引擎桥接] 命令已广播: type=%s", messageType)
}

// matchActionName 在动画列表缓存中为 query 选最佳动作名
func matchActionName(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	animMu.RLock()
	actions := append([]ActionDefinition(nil), animActions...)
	animMu.RUnlock()
	names := make([]string, 0, len(actions))
	for _, action := range actions {
		if name := strings.TrimSpace(action.Name); name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	vectors, err := actionVectorsFor(names)
	if err != nil {
		logger.Warn(lunarCoreModule, "动作匹配嵌入失败（已退回字面匹配）: %v", err)
		return lexicalActionMatch(query, names)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	queryVector, err := memory.EmbedQuery(ctx, query)
	if err != nil {
		return lexicalActionMatch(query, names)
	}
	bestName := ""
	bestSimilarity := float32(-2)
	for i, name := range names {
		if len(vectors[i]) != len(queryVector) {
			continue
		}
		similarity := actionCosine(queryVector, vectors[i])
		if similarity > bestSimilarity {
			bestSimilarity = similarity
			bestName = name
		}
	}
	if bestName != "" && bestSimilarity >= 0.30 {
		return bestName
	}
	return lexicalActionMatch(query, names)
}

var actionIndex struct {
	sync.Mutex
	signature string
	vectors   [][]float32
}

func actionVectorsFor(names []string) ([][]float32, error) {
	signature := strings.Join(names, "\x00")
	actionIndex.Lock()
	defer actionIndex.Unlock()
	if actionIndex.signature == signature && len(actionIndex.vectors) == len(names) {
		return actionIndex.vectors, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vectors, err := memory.EmbedDocuments(ctx, names)
	if err != nil {
		return nil, err
	}
	actionIndex.signature = signature
	actionIndex.vectors = vectors
	return vectors, nil
}

func lexicalActionMatch(query string, names []string) string {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return ""
	}
	best := ""
	for _, name := range names {
		n := strings.ToLower(strings.TrimSpace(name))
		if n == "" {
			continue
		}
		if n == q {
			return name
		}
		if strings.Contains(q, n) || strings.Contains(n, q) {
			if len([]rune(name)) > len([]rune(best)) {
				best = name
			}
		}
	}
	return best
}

func actionCosine(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return -2
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}

// frontendOnline 月华前端或 HTTPS 代理是否在线
func frontendOnline() bool {
	if ws.Instance != nil && ws.Instance.ChannelClientCount(ws.ChannelLunarTerminal) > 0 {
		return true
	}
	return proxy.IsRunning()
}

// synthesizeText 经统一播报链路合成语音
func synthesizeText(text string) (string, error) {
	return broadcaster.SynthesizeBase64(broadcaster.TTSRequest{
		Text:              text,
		Temperature:       0.8,
		TopK:              50,
		TopP:              0.9,
		MaxTokens:         2048,
		RepetitionPenalty: 1.1,
		Threads:           4,
	})
}
