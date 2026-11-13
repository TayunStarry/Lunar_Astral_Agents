package memory

// prefix.go 会话前缀剥离与内部分区派生。
//
// 这里的正则与拆分函数是唯一权威实现：入库存正文、查询拼串都只复用本文件。
// 全部为纯函数，不触发模型、不读写存储，也不改变任何现有调用路径。

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

var (
	// GroupPrefixRegex 群聊消息的 `[群聊: 群名]` 前缀（可缺省）。
	GroupPrefixRegex = regexp.MustCompile(`^\[群聊:\s*([^\]]*)\]\s*`)
	// UserPrefixRegex 用户消息的 `[用户: 昵称]: ` 前缀（可缺省，冒号全角/半角均可）。
	UserPrefixRegex = regexp.MustCompile(`^\[用户:\s*([^\]]*)\]\s*[:：]\s*`)
)

// ConversationParts 一条会话原文拆分后的中间结构。
//
// GroupName / UserName 只用于派生内部分区键与发言者键，派生后必须丢弃，
// 不得写入 Content，也不得进入任何渲染路径。
type ConversationParts struct {
	GroupName string // 群名原文（可空）
	UserName  string // 用户名原文（可空）
	Body      string // 剥离前缀后的正文
}

// SplitConversationPrefix 剥离串首的群聊 / 用户前缀。
//
// 边界行为：
//   - 只有完整匹配串首才剥离，正文中间的方括号原样保留；
//   - 群名与用户名允许为空，`[用户: ]: 正文` 视为匿名发言者；
//   - 嵌套方括号不支持，按第一个 `]` 截断（与前端展示口径一致）；
//   - 返回的 Body 不做 trim，调用方自行决定空白与最短长度。
func SplitConversationPrefix(content string) ConversationParts {
	parts := ConversationParts{Body: content}
	if loc := GroupPrefixRegex.FindStringSubmatchIndex(parts.Body); loc != nil {
		parts.GroupName = parts.Body[loc[2]:loc[3]]
		parts.Body = parts.Body[loc[1]:]
	}
	if loc := UserPrefixRegex.FindStringSubmatchIndex(parts.Body); loc != nil {
		parts.UserName = parts.Body[loc[2]:loc[3]]
		parts.Body = parts.Body[loc[1]:]
	}
	return parts
}

// ConversationChannel 返回会话通道：群聊优先，其次私聊，无前缀返回空串。
func ConversationChannel(parts ConversationParts) string {
	if strings.TrimSpace(parts.GroupName) != "" {
		return ChannelGroup
	}
	if strings.TrimSpace(parts.UserName) != "" {
		return ChannelPrivate
	}
	return ""
}

// ConversationNamespace 由拆分结果派生内部分区键。
//
// 规则：群聊 → group:<群名哈希>；私聊 → private:<用户名哈希>；两者皆无 → shared。
// 前缀里没有稳定 ID，因此只能用名称哈希；与 AddFact 显式传 ID 的路径共用
// NamespaceFor 的拼接格式，但键来源不同。
func ConversationNamespace(parts ConversationParts) string {
	if group := strings.TrimSpace(parts.GroupName); group != "" {
		return NamespaceFor(ChannelGroup, shortHash(group))
	}
	if name := strings.TrimSpace(parts.UserName); name != "" {
		return NamespaceFor(ChannelPrivate, shortHash(name))
	}
	return NamespaceShared
}

// NamespaceFor 按通道与键拼接分区名；键为空时退化为共享分区。
func NamespaceFor(channel, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return NamespaceShared
	}
	switch channel {
	case ChannelPrivate:
		return ChannelPrivate + ":" + key
	case ChannelGroup:
		return ChannelGroup + ":" + key
	default:
		return NamespaceShared
	}
}

// SpeakerKeyFor 返回发言者内部键：优先用户 ID，无 ID 时用昵称哈希；都为空返回空串。
func SpeakerKeyFor(userID, userName string) string {
	if key := strings.TrimSpace(userID); key != "" {
		return key
	}
	if name := strings.TrimSpace(userName); name != "" {
		return shortHash(name)
	}
	return ""
}

// SpeakerKey 返回拆分结果对应的发言者键（无用户 ID，只能用昵称哈希）。
func SpeakerKey(parts ConversationParts) string {
	return SpeakerKeyFor("", parts.UserName)
}

// shortHash 返回文本的短哈希（sha256 前 16 位十六进制）。
func shortHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:16]
}
