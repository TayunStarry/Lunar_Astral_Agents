package memory

import (
	"strings"
	"testing"
)

func TestSplitConversationPrefix(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		group     string
		user      string
		body      string
		channel   string
		namespace string
	}{
		{
			name:      "群聊加用户",
			input:     "[群聊: 星月][用户: 月华]: 今天好累",
			group:     "星月",
			user:      "月华",
			body:      "今天好累",
			channel:   ChannelGroup,
			namespace: NamespaceFor(ChannelGroup, shortHash("星月")),
		},
		{
			name:      "仅用户",
			input:     "[用户: 星野]: 只学得动日语",
			user:      "星野",
			body:      "只学得动日语",
			channel:   ChannelPrivate,
			namespace: NamespaceFor(ChannelPrivate, shortHash("星野")),
		},
		{
			name:      "仅群聊",
			input:     "[群聊: 学习小组] 明晚八点开会",
			group:     "学习小组",
			body:      "明晚八点开会",
			channel:   ChannelGroup,
			namespace: NamespaceFor(ChannelGroup, shortHash("学习小组")),
		},
		{
			name:      "匿名用户",
			input:     "[用户: ]: 没有名字",
			body:      "没有名字",
			channel:   "",
			namespace: NamespaceShared,
		},
		{
			name:      "正文中间方括号不剥离",
			input:     "这里提到 [用户: 月华]: 但不在串首",
			body:      "这里提到 [用户: 月华]: 但不在串首",
			channel:   "",
			namespace: NamespaceShared,
		},
		{
			name:      "全角冒号与无空格",
			input:     "[用户:星野]：无空格",
			user:      "星野",
			body:      "无空格",
			channel:   ChannelPrivate,
			namespace: NamespaceFor(ChannelPrivate, shortHash("星野")),
		},
		{
			name:      "无前缀",
			input:     "普通正文",
			body:      "普通正文",
			channel:   "",
			namespace: NamespaceShared,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parts := SplitConversationPrefix(tc.input)
			if parts.GroupName != tc.group || parts.UserName != tc.user || parts.Body != tc.body {
				t.Fatalf("拆分结果不符: %+v", parts)
			}
			if got := ConversationChannel(parts); got != tc.channel {
				t.Fatalf("通道不符: 期望 %q 实际 %q", tc.channel, got)
			}
			if got := ConversationNamespace(parts); got != tc.namespace {
				t.Fatalf("分区不符: 期望 %q 实际 %q", tc.namespace, got)
			}
			if tc.channel != "" && (strings.Contains(parts.Body, "[用户:") || strings.Contains(parts.Body, "[群聊:")) {
				t.Fatalf("正文仍含前缀: %q", parts.Body)
			}
		})
	}
}

func TestConversationNamespaceStable(t *testing.T) {
	first := ConversationNamespace(ConversationParts{UserName: "月华"})
	second := ConversationNamespace(ConversationParts{UserName: "月华"})
	if first != second {
		t.Fatalf("同名分区不稳定: %q != %q", first, second)
	}
	if first == ConversationNamespace(ConversationParts{UserName: "星野"}) {
		t.Fatal("不同用户不应共用同一分区")
	}
	if !strings.HasPrefix(first, ChannelPrivate+":") {
		t.Fatalf("私聊分区前缀不符: %q", first)
	}
	group := ConversationNamespace(ConversationParts{GroupName: "月华"})
	if !strings.HasPrefix(group, ChannelGroup+":") {
		t.Fatalf("群聊分区前缀不符: %q", group)
	}
	if group == first {
		t.Fatal("群聊与私聊同名时不应共用分区")
	}
}

func TestSpeakerKeyFor(t *testing.T) {
	if got := SpeakerKeyFor("u-1", "月华"); got != "u-1" {
		t.Fatalf("应优先用户 ID，实际 %q", got)
	}
	hashed := SpeakerKeyFor("", "月华")
	if len(hashed) != 16 {
		t.Fatalf("昵称哈希长度应为 16，实际 %q", hashed)
	}
	if hashed != SpeakerKey(ConversationParts{UserName: "月华"}) {
		t.Fatal("SpeakerKey 应与 SpeakerKeyFor 的昵称哈希一致")
	}
	if got := SpeakerKeyFor("", ""); got != "" {
		t.Fatalf("空身份应返回空串，实际 %q", got)
	}
}

func TestNamespaceFor(t *testing.T) {
	cases := []struct{ channel, key, want string }{
		{ChannelPrivate, "u-1", "private:u-1"},
		{ChannelGroup, "g-1", "group:g-1"},
		{ChannelPrivate, "", NamespaceShared},
		{ChannelGroup, "  ", NamespaceShared},
		{"", "u-1", NamespaceShared},
	}
	for _, tc := range cases {
		if got := NamespaceFor(tc.channel, tc.key); got != tc.want {
			t.Fatalf("NamespaceFor(%q, %q) = %q, 期望 %q", tc.channel, tc.key, got, tc.want)
		}
	}
}
