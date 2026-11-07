package resource

// parse_fresh_test.go 解析文本有效期判定与 parsed_at 写入语义

import (
	"testing"
	"time"
)

// TestParseTextFreshTiers 三档有效期：网页 7 天、B 站 30 天、本地永不过期
func TestParseTextFreshTiers(t *testing.T) {
	now := time.Now().Unix()
	cases := []struct {
		name     string
		res      Resource
		category ParseCategory
		want     bool
	}{
		{
			name:     "网页6天命中",
			res:      Resource{ParseState: ParseReady, ParseText: "正文", ParsedAt: now - 6*24*3600},
			category: ParseCategoryWeb,
			want:     true,
		},
		{
			name:     "网页8天过期",
			res:      Resource{ParseState: ParseReady, ParseText: "正文", ParsedAt: now - 8*24*3600},
			category: ParseCategoryWeb,
			want:     false,
		},
		{
			name:     "B站29天命中",
			res:      Resource{ParseState: ParseReady, ParseText: "印象", ParsedAt: now - 29*24*3600},
			category: ParseCategoryBilibili,
			want:     true,
		},
		{
			name:     "B站31天过期",
			res:      Resource{ParseState: ParseReady, ParseText: "印象", ParsedAt: now - 31*24*3600},
			category: ParseCategoryBilibili,
			want:     false,
		},
		{
			name:     "本地资源十年也不过期",
			res:      Resource{ParseState: ParseReady, ParseText: "理解", ParsedAt: now - 10*365*24*3600},
			category: ParseCategoryLocal,
			want:     true,
		},
		{
			name:     "未解析命中失败",
			res:      Resource{ParseState: ParseNone, ParseText: "正文", ParsedAt: now},
			category: ParseCategoryWeb,
			want:     false,
		},
		{
			name:     "空文本命中失败",
			res:      Resource{ParseState: ParseReady, ParseText: "  ", ParsedAt: now},
			category: ParseCategoryWeb,
			want:     false,
		},
		{
			name:     "缺写入时间的网页过期",
			res:      Resource{ParseState: ParseReady, ParseText: "正文", ParsedAt: 0},
			category: ParseCategoryWeb,
			want:     false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseTextFresh(tc.res, tc.category); got != tc.want {
				t.Fatalf("ParseTextFresh = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestParseCategoryOf B 站只认视频 BV：专栏 / 番剧 / 短链都走网页档
func TestParseCategoryOf(t *testing.T) {
	cases := []struct {
		name string
		res  Resource
		want ParseCategory
	}{
		{
			name: "BV视频",
			res:  Resource{URL: "https://www.bilibili.com/video/BV1xx411c7mD"},
			want: ParseCategoryBilibili,
		},
		{
			name: "专栏",
			res:  Resource{URL: "https://www.bilibili.com/read/cv123456"},
			want: ParseCategoryWeb,
		},
		{
			name: "番剧页",
			res:  Resource{URL: "https://www.bilibili.com/bangumi/play/ep123456"},
			want: ParseCategoryWeb,
		},
		{
			name: "b23短链",
			res:  Resource{URL: "https://b23.tv/abcdEF"},
			want: ParseCategoryWeb,
		},
		{
			name: "本地视频",
			res:  Resource{URL: "/file/read/multimedia/a.mp4", BlobPath: "multimedia/a.mp4"},
			want: ParseCategoryLocal,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseCategoryOf(tc.res); got != tc.want {
				t.Fatalf("ParseCategoryOf = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestWebPageFreshUsesUnifiedTTL 搜索侧入口委托统一判定，且 ttl<=0 回落 7 天
func TestWebPageFreshUsesUnifiedTTL(t *testing.T) {
	setupArchiveTest(t)
	res, err := AdmitURL(URLAdmitInput{URL: "https://example.com/post", KindHint: KindURL})
	if err != nil {
		t.Fatalf("登记链接失败: %v", err)
	}
	if err := SetParseText(res.URL, "网页正文"); err != nil {
		t.Fatalf("写入解析文本失败: %v", err)
	}
	old := time.Now().Add(-8 * 24 * time.Hour).Unix()
	if _, err := archiveDB.db.Exec(`UPDATE url_master SET parsed_at = ? WHERE id = ?`, old, res.ID); err != nil {
		t.Fatalf("回填 parsed_at 失败: %v", err)
	}
	if WebPageFresh(res.URL, 0) {
		t.Fatal("8 天前的网页应判定过期（ttl<=0 回落 7 天）")
	}
	if _, err := archiveDB.db.Exec(`UPDATE url_master SET parsed_at = ? WHERE id = ?`,
		time.Now().Add(-6*24*time.Hour).Unix(), res.ID); err != nil {
		t.Fatalf("回填 parsed_at 失败: %v", err)
	}
	if !WebPageFresh(res.URL, 0) {
		t.Fatal("6 天前的网页应判定新鲜")
	}
	if WebPageFresh(res.URL, 24*time.Hour) {
		t.Fatal("覆盖 TTL=1 天时，6 天前的网页应判定过期")
	}
}

// TestUpdateResourceURLExtrasKeepsParsedAtOnTitleOnly 只改标题不能续期
func TestUpdateResourceURLExtrasKeepsParsedAtOnTitleOnly(t *testing.T) {
	setupArchiveTest(t)
	res, err := AdmitURL(URLAdmitInput{URL: "https://example.com/post", KindHint: KindURL})
	if err != nil {
		t.Fatalf("登记链接失败: %v", err)
	}
	if err := SetParseText(res.URL, "旧正文"); err != nil {
		t.Fatalf("写入解析文本失败: %v", err)
	}
	if _, err := archiveDB.db.Exec(`UPDATE url_master SET parsed_at = 1000 WHERE id = ?`, res.ID); err != nil {
		t.Fatalf("回填 parsed_at 失败: %v", err)
	}
	if err := archiveDB.UpdateResourceURLExtras(res.ID, "新标题", "旧正文"); err != nil {
		t.Fatalf("更新标题失败: %v", err)
	}
	got, found := archiveDB.GetResourceByID(res.ID)
	if !found {
		t.Fatal("资源应仍存在")
	}
	if got.ParsedAt != 1000 {
		t.Fatalf("只改标题不应续期 parsed_at，got %d", got.ParsedAt)
	}
	if err := archiveDB.UpdateResourceURLExtras(res.ID, "新标题", "新正文"); err != nil {
		t.Fatalf("更新正文失败: %v", err)
	}
	got, found = archiveDB.GetResourceByID(res.ID)
	if !found {
		t.Fatal("资源应仍存在")
	}
	if got.ParsedAt <= 1000 {
		t.Fatalf("正文变化应续期 parsed_at，got %d", got.ParsedAt)
	}
}

// TestRefreshResourceContentInvalidatesOnHashChange 指纹变化才清空旧理解
func TestRefreshResourceContentInvalidatesOnHashChange(t *testing.T) {
	setupArchiveTest(t)
	res, err := AdmitURL(URLAdmitInput{URL: "https://example.com/doc", KindHint: KindURL})
	if err != nil {
		t.Fatalf("登记链接失败: %v", err)
	}
	if err := SetParseText(res.URL, "旧描述"); err != nil {
		t.Fatalf("写入解析文本失败: %v", err)
	}
	if _, err := archiveDB.db.Exec(`UPDATE url_master SET content_hash = 'oldhash' WHERE id = ?`, res.ID); err != nil {
		t.Fatalf("回填指纹失败: %v", err)
	}
	if err := archiveDB.RefreshResourceContent(res.ID, "doc.txt", ".txt", "text/plain", "newhash", 10); err != nil {
		t.Fatalf("刷新内容失败: %v", err)
	}
	got, found := archiveDB.GetResourceByID(res.ID)
	if !found {
		t.Fatal("资源应仍存在")
	}
	if got.ParseText != "" || got.ParseState != ParseNone {
		t.Fatalf("指纹变化应清空解析文本，got text=%q state=%q", got.ParseText, got.ParseState)
	}
	// 指纹相同：保留
	if err := SetParseText(res.URL, "新描述"); err != nil {
		t.Fatalf("写入解析文本失败: %v", err)
	}
	if err := archiveDB.RefreshResourceContent(res.ID, "doc.txt", ".txt", "text/plain", "newhash", 10); err != nil {
		t.Fatalf("刷新内容失败: %v", err)
	}
	got, _ = archiveDB.GetResourceByID(res.ID)
	if got.ParseText != "新描述" {
		t.Fatalf("指纹未变应保留解析文本，got %q", got.ParseText)
	}
	// 新指纹取不到：保留，避免扫描失败误清
	if err := archiveDB.RefreshResourceContent(res.ID, "doc.txt", ".txt", "text/plain", "", 10); err != nil {
		t.Fatalf("刷新内容失败: %v", err)
	}
	got, _ = archiveDB.GetResourceByID(res.ID)
	if got.ParseText != "新描述" {
		t.Fatalf("新指纹为空应保留解析文本，got %q", got.ParseText)
	}
}
