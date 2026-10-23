package lunar

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

func TestParseContentSeparatesBlocks(t *testing.T) {
	raw := "<think>先想一想</think>\n你好呀（挥手）(^_^)\n```go\nfmt.Println(1)\n```\n今天天气不错。"
	parsed := parseContent(raw)
	if len(parsed.ThinkingBlocks) != 1 || parsed.ThinkingBlocks[0] != "先想一想" {
		t.Fatalf("思考区解析异常: %#v", parsed.ThinkingBlocks)
	}
	if len(parsed.CodeBlocks) != 1 || !strings.Contains(parsed.CodeBlocks[0], "fmt.Println") {
		t.Fatalf("代码块解析异常: %#v", parsed.CodeBlocks)
	}
	if len(parsed.ActionBlocks) == 0 {
		t.Fatalf("动作区/情感区未提取: %#v", parsed.ActionBlocks)
	}
	if !strings.Contains(parsed.DisplayText, "你好呀") || !strings.Contains(parsed.DisplayText, "今天天气不错") {
		t.Fatalf("正文清洗异常: %q", parsed.DisplayText)
	}
}

func TestHandleGrammarRoundTrip(t *testing.T) {
	cases := []string{"@D.003", "@I.007", "@D.003|§4", "@V.001|L120-168"}
	for _, raw := range cases {
		handle := parseHandle(raw)
		if handle == nil {
			t.Fatalf("句柄解析失败: %s", raw)
		}
		if got := formatHandle(*handle); got != raw {
			t.Fatalf("句柄格式化不一致: %s -> %s", raw, got)
		}
	}
	if parseHandle("@X.001") != nil {
		t.Fatal("非法类型码不应解析成功")
	}
}

func TestNormalizeTime(t *testing.T) {
	if got := normalizeTime("2026年6月14日 15:30"); got != "2026-06-14T15:30:00" {
		t.Fatalf("中文时间解析异常: %s", got)
	}
	if got := normalizeTime("2026-06-14T15:30:00"); got != "2026-06-14T15:30:00" {
		t.Fatalf("ISO 时间解析异常: %s", got)
	}
	if normalizeTime("不是时间") != "" {
		t.Fatal("非法时间应返回空串")
	}
}

func TestParseDateRangeRelative(t *testing.T) {
	rangeValue, errText := parseDateRange("今天", "")
	if errText != "" || rangeValue == nil {
		t.Fatalf("相对日期解析失败: %s", errText)
	}
	if !strings.HasSuffix(rangeValue.Label, "（今天）") {
		t.Fatalf("相对日期标签异常: %s", rangeValue.Label)
	}
}

// 回归：QQ 桥接给出的多模态内容是 []map[string]any，必须能被识别为内容项。
// 曾经的类型断言只认 []ContentItem / []any，导致图文消息静默退化为「无内容项」：
// 图片赋签与媒体入库被跳过，原图直送模型，随后每轮模型都只回空内容。
func TestAsItemsAcceptsQQContentSlice(t *testing.T) {
	content := []map[string]any{
		{"type": "text", "text": "[用户: 钛宇星光阁]: "},
		{"type": "image_url", "image_url": map[string]any{"url": "http://example.com/a.jpg"}},
	}
	items := asItems(content)
	if len(items) != 2 {
		t.Fatalf("内容项数量应为 2，实得 %d", len(items))
	}
	if items[0].Type != ContentText || items[1].ImageURL == nil {
		t.Fatalf("内容项解析异常: %#v", items)
	}
	if items[1].ImageURL.URL != "http://example.com/a.jpg" {
		t.Fatalf("图片地址解析异常: %s", items[1].ImageURL.URL)
	}
}

// 回归：出站图片闸门必须拦下非视觉读取产物的图片（远程 URL / 直读地址）。
func TestGuardRequestImagesBlocksRawSources(t *testing.T) {
	messages := []PostMessage{{
		Role: RoleUser,
		Content: []ContentItem{
			{Type: ContentText, Text: "看看这个"},
			{Type: ContentImageURL, ImageURL: &URLPayload{URL: "https://gchat.qpic.cn/gchatpic_new/0/0-0-abc/0"}},
			{Type: ContentImageURL, ImageURL: &URLPayload{URL: "/file/read/multimedia/raw.jpg"}},
		},
	}}
	guarded, blocked := guardRequestImages(messages)
	if len(blocked) != 2 {
		t.Fatalf("应拦截 2 张原始图片，实得 %d", len(blocked))
	}
	items := asItems(guarded[0].Content)
	for _, item := range items[1:] {
		if item.Type != ContentText || item.Text != blockedImagePlaceholder {
			t.Fatalf("被拦截的图片应替换为文本占位: %#v", item)
		}
	}
}

// 视觉读取端点产出的图片必须放行。
func TestGuardRequestImagesAllowsVisionEndpoint(t *testing.T) {
	visionURL := config.LocalBaseURL() + "/file/miniature/multimedia/demo.jpg?tag=%40I.001"
	messages := []PostMessage{{
		Role:    RoleUser,
		Content: []ContentItem{{Type: ContentImageURL, ImageURL: &URLPayload{URL: visionURL}}},
	}}
	guarded, blocked := guardRequestImages(messages)
	if len(blocked) != 0 {
		t.Fatalf("视觉读取端点不应被拦截: %#v", blocked)
	}
	items := asItems(guarded[0].Content)
	if len(items) != 1 || items[0].Type != ContentImageURL {
		t.Fatalf("放行的图片被改写: %#v", items)
	}
}

// 回归：系统提示词里的当前位置占位符必须被替换成真实地址。
// 曾经查询失败时把空串塞进占位符，模型看到「当前位置：」为空，只好反问答不上来。
func TestPromptCompletionFillsAddress(t *testing.T) {
	original := config.ServerAddress
	config.ServerAddress = []string{"浙江省", "杭州市"}
	t.Cleanup(func() { config.ServerAddress = original })

	builder := &ModelBuilder{}
	prompt := builder.promptCompletion("当前位置：{current-address}\n备用：{current_address}")
	if strings.Contains(prompt, "{current-address}") || strings.Contains(prompt, "{current_address}") {
		t.Fatalf("占位符未被替换: %q", prompt)
	}
	if !strings.Contains(prompt, "浙江省 杭州市") {
		t.Fatalf("地址未写入提示词: %q", prompt)
	}
}

// 回归：每轮重建系统提示词（追加设定）时也必须补全占位符。
// 构造时补全过、每轮又被原始模板覆盖，是「月华突然不知道自己在哪座城市」的真实成因。
func TestDialogueApplyAppendSettingsKeepsAddress(t *testing.T) {
	original := config.ServerAddress
	config.ServerAddress = []string{"江苏省", "南京市"}
	t.Cleanup(func() { config.ServerAddress = original })

	role := &DialogueRole{ModelBuilder: &ModelBuilder{}}
	role.LunarAppearance = "黑发红瞳"
	role.CrystalAppearance = "银发蓝瞳"
	role.applyAppendSettings()
	if strings.Contains(role.SystemPrompt, "{current-address}") || strings.Contains(role.SystemPrompt, "{current_address}") {
		t.Fatalf("重建系统提示词后仍残留占位符")
	}
	if !strings.Contains(role.SystemPrompt, "江苏省 南京市") {
		t.Fatalf("重建系统提示词后缺少所在地: %q", truncateRunes(role.SystemPrompt, 200))
	}
}
