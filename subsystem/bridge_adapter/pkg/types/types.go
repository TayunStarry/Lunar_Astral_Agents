package types

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	QQAdapter QQAdapter `json:"qq_adapter"`
}

type QQAdapter struct {
	DisplayLogs      bool       `json:"display_logs"`
	NapcatWsServer   string     `json:"napcat_ws_server"`
	NapcatWsToken    string     `json:"napcat_ws_token"`
	LunarCoreUrl     string     `json:"lunar_core_url"`
	LunarWsServer    string     `json:"lunar_ws_server"`
	ListenGroupIds   Int64Slice `json:"listen_group_ids"`
	PollInterval     int        `json:"poll_interval"`
	TriggerKeywords  []string   `json:"trigger_keywords"`
	DefaultReply     string     `json:"default_reply"`
	AIRoutingEnabled bool       `json:"ai_routing_enabled"`
	AIRoutingModel   string     `json:"ai_routing_model"`
}

type Int64Slice []int64

func (s *Int64Slice) UnmarshalJSON(data []byte) error {
	var nums []int64
	if err := json.Unmarshal(data, &nums); err == nil {
		*s = nums
		return nil
	}

	var raw []interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("解析 listen_group_ids 失败: %v", err)
	}

	for _, v := range raw {
		switch val := v.(type) {
		case float64:
			*s = append(*s, int64(val))
		case string:
			parsed, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[警告] listen_group_ids 中的值 '%s' 无法转换为有效群号，已跳过\n", val)
				continue
			}
			*s = append(*s, parsed)
		default:
			fmt.Fprintf(os.Stderr, "[警告] listen_group_ids 中包含不支持的数据类型 %T，已跳过\n", v)
		}
	}

	return nil
}

type NapcatMessage struct {
	SelfID      int64            `json:"self_id"`
	UserID      int64            `json:"user_id"`
	MessageID   int64            `json:"message_id"`
	Sender      Sender           `json:"sender"`
	GroupID     int64            `json:"group_id"`
	Message     []MessageSegment `json:"message"`
	PostType    string           `json:"post_type"`
	MessageType string           `json:"message_type"`
}

type Sender struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
	Role     string `json:"role"`
}

type MessageSegment struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type TextData struct {
	Text string `json:"text"`
}

type ReplyData struct {
	ID string `json:"id"`
}

type ImageData struct {
	Summary  string `json:"summary"`
	File     string `json:"file"`
	SubType  int    `json:"sub_type"`
	URL      string `json:"url"`
	FileSize string `json:"file_size"`
}

type AtData struct {
	QQ string `json:"qq"`
}

type ForwardData struct {
	ID string `json:"id"`
}

type FileData struct {
	File     string `json:"file"`
	FileID   string `json:"file_id"`
	FileSize string `json:"file_size"`
	URL      string `json:"url"`
}

type LunarMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type LunarContextData struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

type LunarImageData struct {
	Type   string   `json:"type"`
	Images []string `json:"images"`
}

type OpenAIMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type BatchMessageRequest struct {
	Messages []OpenAIMessage `json:"messages"`
}

type NapcatWSResponse struct {
	Status  string          `json:"status"`
	Retcode int             `json:"retcode"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
	Wording string          `json:"wording"`
	Echo    string          `json:"echo"`
	Stream  string          `json:"stream"`
}

type ForwardMessageResponse struct {
	Messages []NapcatMessage `json:"messages"`
}

type CachedMessage struct {
	GroupID   int64
	UserID    int64
	Content   interface{}
	HasImages bool
}

// ==================== AI 路由相关类型 ====================

// ChatCompletionRequest OpenAI 兼容的聊天补全请求
type ChatCompletionRequest struct {
	Model    string                  `json:"model"`
	Messages []ChatCompletionMessage `json:"messages"`
}

// ChatCompletionMessage 聊天消息
type ChatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionResponse OpenAI 兼容的聊天补全响应
type ChatCompletionResponse struct {
	Choices []ChatCompletionChoice `json:"choices"`
}

// ChatCompletionChoice 聊天补全选项
type ChatCompletionChoice struct {
	Message ChatCompletionMessage `json:"message"`
}

// AIRoutingDecision AI 路由判定结果（JSON 反序列化）
type AIRoutingDecision struct {
	GroupIDs []int64 `json:"group_ids"`
}

// LunarBatchPush 来自 lunar_astral 的批量消息推送结构
type LunarBatchPush struct {
	Type     string           `json:"type"`
	Messages []LunarBatchItem `json:"messages"`
}

// LunarBatchItem 批量推送中的单条消息
type LunarBatchItem struct {
	MsgType string   `json:"msg_type"`         // "context" 或 "image"
	Content string   `json:"content"`          // 文本内容
	Images  []string `json:"images,omitempty"` // 图片base64列表
}
