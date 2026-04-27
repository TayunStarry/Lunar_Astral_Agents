package main

import "encoding/json"

type Config struct {
	QQAdapter QQAdapter `json:"qq_adapter"`
}

type QQAdapter struct {
	DisplayLogs     bool      `json:"display_logs"`
	NapcatWsServer  string   `json:"napcat_ws_server"`
	NapcatWsToken   string   `json:"napcat_ws_token"`
	LunarCoreUrl    string   `json:"lunar_core_url"`
	LunarWsServer   string   `json:"lunar_ws_server"`
	ListenGroupIds  []int64  `json:"listen_group_ids"`
	PollInterval    int      `json:"poll_interval"`
	TriggerKeywords []string `json:"trigger_keywords"`
	DefaultReply    string   `json:"default_reply"`
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
