package ltpxmarket

import (
	"archive/zip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestParseMarketBody 验证市场 Release body 的结构化解析
func TestParseMarketBody(t *testing.T) {
	body := "[LTPX] 实时资讯\n" +
		"ID: lunar.search-weather\n" +
		"Tags: Node-LTP, 实用工具\n" +
		"SHA-256: abc123\n" +
		"\n" +
		"一个天气查询工具 #测试\n" +
		"第二行描述\n"
	r := giteeRelease{
		TagName: "lunar.search-weather",
		Body:    body,
		Assets: []giteeAsset{
			{Name: "lunar.search-weather.zip", Size: 111}, // Gitee 自动附加的源码归档
			{Name: "ltpx_lunar.search-weather.ltpx", Size: 222, BrowserDownloadURL: "https://example.com/x.ltpx"},
		},
	}
	item := parseMarketBody(r)
	if item == nil {
		t.Fatalf("市场 body 应解析成功")
	}
	if item.TagName != "lunar.search-weather" {
		t.Errorf("TagName = %q", item.TagName)
	}
	if item.Title != "实时资讯" {
		t.Errorf("Title = %q", item.Title)
	}
	if !reflect.DeepEqual(item.Tags, []string{"Node-LTP", "实用工具"}) {
		t.Errorf("Tags = %v", item.Tags)
	}
	if item.AssetSHA != "abc123" {
		t.Errorf("AssetSHA = %q", item.AssetSHA)
	}
	if item.AssetName != "ltpx_lunar.search-weather.ltpx" || item.AssetURL != "https://example.com/x.ltpx" || item.AssetSize != 222 {
		t.Errorf("应取市场前缀附件: name=%q url=%q size=%d", item.AssetName, item.AssetURL, item.AssetSize)
	}
	wantDesc := "一个天气查询工具 #测试\n第二行描述"
	if item.Description != wantDesc {
		t.Errorf("Description = %q，期望 %q", item.Description, wantDesc)
	}
}

// TestParseMarketBodyRejectsNonMarket 非市场发布（更新包）不解析
func TestParseMarketBodyRejectsNonMarket(t *testing.T) {
	r := giteeRelease{TagName: "v2026.10.14", Body: "星月智能更新包 v2026.10.14\n\nSHA-256: abc"}
	if item := parseMarketBody(r); item != nil {
		t.Fatalf("更新包 body 不应被解析为市场项: %+v", item)
	}
}

// TestParseMarketBodyNoDescription 无描述正文时 Description 为空且不panic
func TestParseMarketBodyNoDescription(t *testing.T) {
	r := giteeRelease{TagName: "a.b", Body: "[LTPX] 标题\nID: a.b\nTags: T1\nSHA-256: h\n"}
	item := parseMarketBody(r)
	if item == nil {
		t.Fatalf("应解析成功")
	}
	if item.Description != "" {
		t.Errorf("Description = %q，期望空", item.Description)
	}
	if item.AssetSHA != "h" {
		t.Errorf("AssetSHA = %q", item.AssetSHA)
	}
}

// TestExtractHashTags 验证 #标签 识别：中英文/混合/markdown 标题不误识别
func TestExtractHashTags(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"一个天气查询工具 #测试", []string{"测试"}},
		{"#Node-LTP 和 #ltp9 都要", []string{"Node-LTP", "ltp9"}},
		{"标签紧贴文字#实用工具结尾", []string{"实用工具结尾"}},
		{"#tag.name 与 #a-b_c", []string{"tag.name", "a-b_c"}},
		{"# 标题不是标签 ## 也不是", nil},
		{"C#语言不算标签前缀但也无法区分", []string{"语言不算标签前缀但也无法区分"}},
		{"没有任何标签", nil},
		{"", nil},
	}
	for _, c := range cases {
		got := extractHashTags(c.text)
		if len(got) != len(c.want) {
			t.Errorf("extractHashTags(%q) = %v，期望 %v", c.text, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("extractHashTags(%q)[%d] = %q，期望 %q", c.text, i, got[i], c.want[i])
			}
		}
	}
}

// TestMergeTags 验证标签合并：大小写不敏感去重、顺序保持
func TestMergeTags(t *testing.T) {
	got := mergeTags([]string{"Node-LTP", "LTPX"}, []string{"node-ltp", "实用工具", "LTPX"})
	want := []string{"Node-LTP", "LTPX", "实用工具"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mergeTags = %v，期望 %v", got, want)
	}
}

// TestZipDirExtractZipRoundtrip 打包 → 解压回环：顶层目录保留、内容一致
func TestZipDirExtractZipRoundtrip(t *testing.T) {
	src := t.TempDir()
	top := "my-package"
	if err := os.MkdirAll(filepath.Join(src, top, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(top, "index.html"):     "<html>hello</html>",
		filepath.Join(top, "metadata.json"):  `{"id":"my.package"}`,
		filepath.Join(top, "assets", "a.js"): "console.log(1)",
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(src, path), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	dest := filepath.Join(t.TempDir(), "out.zip")
	// 与生产语义一致：打包源为包目录本身，topDir 仅作为包内条目顶层名（源目录内不含该层级）
	if err := zipDir(filepath.Join(src, top), top, dest, dirTotalSize(filepath.Join(src, top))); err != nil {
		t.Fatalf("zipDir 失败: %v", err)
	}

	// zip 内条目应以顶层目录开头
	reader, err := zip.OpenReader(dest)
	if err != nil {
		t.Fatalf("打开 zip 失败: %v", err)
	}
	defer reader.Close()
	for _, f := range reader.File {
		if !strings.HasPrefix(f.Name, top+"/") {
			t.Errorf("zip 条目 %s 缺少顶层目录前缀 %s/", f.Name, top)
		}
	}
	reader.Close()

	unpack := filepath.Join(t.TempDir(), "unpack")
	if err := extractZip(dest, unpack); err != nil {
		t.Fatalf("extractZip 失败: %v", err)
	}
	for path, content := range files {
		got, err := os.ReadFile(filepath.Join(unpack, path))
		if err != nil {
			t.Fatalf("解压后缺少 %s: %v", path, err)
		}
		if string(got) != content {
			t.Errorf("%s 内容不一致: %q", path, got)
		}
	}
}

// TestExtractZipRejectsTraversal 解压拒绝路径穿越条目
func TestExtractZipRejectsTraversal(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "evil.zip")
	buf, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(buf)
	for _, name := range []string{"..\\evil.txt", "../evil.txt"} {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		fw, fwErr := w.CreateHeader(h)
		if fwErr != nil {
			t.Fatal(fwErr)
		}
		if _, err := fw.Write([]byte("bad")); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	_ = buf.Close()

	dest := t.TempDir()
	if err := extractZip(zipPath, dest); err != nil {
		t.Fatalf("含穿越条目的 zip 不应报错（应跳过）: %v", err)
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 0 {
		t.Errorf("穿越条目应被跳过，实际解出 %d 项", len(entries))
	}
}

// TestListLocalPackages 验证本地包扫描：仅收录含 metadata.json 的目录
func TestListLocalPackages(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("pkg.a/metadata.json", `{"id":"pkg.a","title":"包A","tags":["T1"]}`)
	write("pkg.b/metadata.json", `{"id":"pkg.b","title":"包B"}`)
	write("plain/not-package.txt", "资源目录无 metadata.json")
	write("broken/metadata.json", "{invalid json")

	cfg := Config{PackageDir: root}
	pkgs, err := ListLocalPackages(cfg)
	if err != nil {
		t.Fatalf("ListLocalPackages 失败: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("应扫描到 2 个包，实际 %d: %+v", len(pkgs), pkgs)
	}
	if pkgs[0].DirName != "pkg.a" || pkgs[1].DirName != "pkg.b" {
		t.Errorf("应按目录名排序: %s, %s", pkgs[0].DirName, pkgs[1].DirName)
	}
	if pkgs[0].Meta.ID != "pkg.a" || pkgs[0].Meta.Title != "包A" {
		t.Errorf("元数据解析不符: %+v", pkgs[0].Meta)
	}
	if pkgs[0].Size == 0 {
		t.Errorf("包大小应大于 0")
	}
}

// TestListLocalPackagesMissingDir 包目录不存在时返回空列表而非报错
func TestListLocalPackagesMissingDir(t *testing.T) {
	cfg := Config{PackageDir: filepath.Join(t.TempDir(), "not-exist")}
	pkgs, err := ListLocalPackages(cfg)
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if len(pkgs) != 0 {
		t.Fatalf("应返回空列表")
	}
}

// TestSanitizeName 验证目录名规范化拒绝穿越字符
func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"pkg.a-b_c":    "pkg.a-b_c",
		"..\\evil":     "..evil", // 点保留，但 Join 后是字面目录名而非父级引用，无穿越风险
		"../../x":      "....x",
		"com/ltp9:tag": "comltp9tag",
		"###":          "unnamed-package",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Errorf("sanitizeName(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// TestValidateConfig 验证配置校验：仓库必填、发布必令牌
func TestValidateConfig(t *testing.T) {
	if err := validateConfig(Config{}, false); err == nil {
		t.Errorf("缺仓库配置应报错")
	}
	cfg := Config{GiteeOwner: "o", GiteeRepo: "r"}
	if err := validateConfig(cfg, false); err != nil {
		t.Errorf("拉取无需令牌: %v", err)
	}
	if err := validateConfig(cfg, true); err == nil {
		t.Errorf("发布缺令牌应报错")
	}
	if err := validateConfig(Config{GiteeOwner: "o", GiteeRepo: "r", GiteeToken: "t"}, true); err != nil {
		t.Errorf("配置完备不应报错: %v", err)
	}
}

// TestMarketAssetName 附件规范名固定为 ltpx_<id>.ltpx
func TestMarketAssetName(t *testing.T) {
	if got := marketAssetName("lunar.search-weather"); got != "ltpx_lunar.search-weather.ltpx" {
		t.Errorf("marketAssetName = %q", got)
	}
}

// writeLtpx 构造测试用 .ltpx 文件（zip 内容 + 任意后缀）
func writeLtpx(t *testing.T, dir, name string, entries map[string]string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for entry, content := range entries {
		fw, fwErr := w.Create(entry)
		if fwErr != nil {
			t.Fatal(fwErr)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestReadArchivePackage 包内读取 metadata.json（不解压）：琉璃导出布局与根级布局均兼容
func TestReadArchivePackage(t *testing.T) {
	dir := t.TempDir()

	// 布局一：琉璃导出格式 <顶层目录>/metadata.json
	p1 := writeLtpx(t, dir, "fake.ltpx", map[string]string{
		"fake.pkg/index.html":     "<html></html>",
		"fake.pkg/metadata.json":  `{"id":"fake.pkg","title":"假包","tags":["T9"]}`,
		"fake.pkg/nested/md.json": "{}",
	})
	pkg, err := ReadArchivePackage(p1)
	if err != nil {
		t.Fatalf("琉璃布局读取失败: %v", err)
	}
	if pkg.Meta.ID != "fake.pkg" || pkg.Meta.Title != "假包" {
		t.Errorf("元数据不符: %+v", pkg.Meta)
	}
	if pkg.DirName != "fake.ltpx" || pkg.Size == 0 {
		t.Errorf("文件信息不符: dir=%s size=%d", pkg.DirName, pkg.Size)
	}

	// 布局二：根级 metadata.json
	p2 := writeLtpx(t, dir, "loose.ltpx", map[string]string{
		"metadata.json": `{"id":"loose.pkg"}`,
	})
	pkg2, err := ReadArchivePackage(p2)
	if err != nil {
		t.Fatalf("根级布局读取失败: %v", err)
	}
	if pkg2.Meta.ID != "loose.pkg" {
		t.Errorf("元数据不符: %+v", pkg2.Meta)
	}

	// 缺少 metadata.json → 报错
	p3 := writeLtpx(t, dir, "empty.ltpx", map[string]string{
		"something/index.html": "<html></html>",
	})
	if _, err := ReadArchivePackage(p3); err == nil {
		t.Fatalf("缺少 metadata.json 应报错")
	}

	// 非 zip 内容 → 报错
	p4 := filepath.Join(dir, "bad.ltpx")
	if err := os.WriteFile(p4, []byte("not a zip"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadArchivePackage(p4); err == nil {
		t.Fatalf("非 zip 内容应报错")
	}

	// 文件不存在 → 报错
	if _, err := ReadArchivePackage(filepath.Join(dir, "nope.ltpx")); err == nil {
		t.Fatalf("文件不存在应报错")
	}
}
