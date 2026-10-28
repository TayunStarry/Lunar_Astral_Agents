package resource

// archive_test.go 资源句柄系统的冒烟测试
//
// 覆盖三类来源归一（path / bytes / data URI）、句柄分配与单调性、
// 段落表解析与锚点展开、内容去重、回收后降级语义、跨资源搜索。
//
// 运行：go test ./engine/resource/ -run TestArchive -v

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"image"
	"image/color"
	"image/gif"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/knowledge"
	persistence "LunarSubsystem/LunarHarness/engine/persistence"
)

// setupArchiveTest 在临时目录中准备隔离的资源目录、媒体派生库与内容仓库
func setupArchiveTest(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	// 覆盖全局配置指向临时目录（测试结束后由 defer 还原）
	origLocal := config.LocalDir
	origArchive := config.ArchiveDBPath
	origMedia := config.MediaDBPath
	config.LocalDir = tmp
	config.ArchiveDBPath = filepath.Join(tmp, "database", "archive.db")
	config.MediaDBPath = filepath.Join(tmp, "database", "media.db")
	// 重置单例，保证每个测试从干净状态开始
	archiveDB = nil
	archiveSeqLoaded = false
	archiveSeqCache = map[string]int64{}
	t.Cleanup(func() {
		// 先关闭 SQLite 连接，否则 Windows 上临时目录无法删除
		_ = Close()
		_ = knowledge.Close()
		config.LocalDir = origLocal
		config.ArchiveDBPath = origArchive
		config.MediaDBPath = origMedia
		archiveDB = nil
		archiveSeqLoaded = false
		archiveSeqCache = map[string]int64{}
	})
	if err := os.MkdirAll(filepath.Join(tmp, "multimedia"), 0755); err != nil {
		t.Fatalf("准备临时目录失败: %v", err)
	}
	if err := Init(config.ArchiveDBPath); err != nil {
		t.Fatalf("初始化资源目录失败: %v", err)
	}
	return tmp
}

// TestArchiveAdmitBytesAndRead 字节入仓 → 按句柄整段读取
func TestArchiveAdmitBytesAndRead(t *testing.T) {
	setupArchiveTest(t)

	content := "# 第一章\n第一行正文\n# 第二章\n第二行正文\n"
	result, err := Admit(AdmitInput{
		SourceKind: SourceKindBytes,
		Data:       []byte(content),
		Name:       "note.md",
		Origin:     OriginClientUpload,
	})
	if err != nil {
		t.Fatalf("入库失败: %v", err)
	}
	if result.Handle != "@D.001" {
		t.Errorf("句柄应为 @D.001，实际 %s", result.Handle)
	}
	if result.Kind != KindDocument {
		t.Errorf("类型码应为 D，实际 %s", result.Kind)
	}
	// 文档类资源的归档副本落在文档树根目录，并使用原文件名
	if result.BlobPath != "documents/note.md" {
		t.Errorf("文档类应按原文件名落在 documents/ 之下: %s", result.BlobPath)
	}

	read, err := Read(result.Handle, 0)
	if err != nil {
		t.Fatalf("取回失败: %v", err)
	}
	if read.Lost {
		t.Fatalf("刚入仓的资源不应为 lost")
	}
	if read.Content != content {
		t.Errorf("正文不一致:\n期望 %q\n实际 %q", content, read.Content)
	}
}

// TestArchiveAdmitURLHandle 链接资源：同一地址复用同一 @U 句柄，取回即「地址 + 标题 + 摘要」
func TestArchiveAdmitURLHandle(t *testing.T) {
	setupArchiveTest(t)

	first, err := AdmitURL(URLAdmitInput{
		URL:     "https://www.bilibili.com/video/BV1xx411c7mD",
		Title:   "一段视频",
		Summary: "看完了，讲的是海边日落。",
		Origin:  OriginChannel,
	})
	if err != nil {
		t.Fatalf("链接入仓失败: %v", err)
	}
	if first.Handle != "@U.001" || first.Kind != KindURL {
		t.Fatalf("首个链接应为 @U.001，实际 %s / %s", first.Handle, first.Kind)
	}
	second, err := AdmitURL(URLAdmitInput{
		URL:    "https://www.bilibili.com/video/BV1xx411c7mD",
		Origin: OriginChannel,
	})
	if err != nil {
		t.Fatalf("重复链接入仓失败: %v", err)
	}
	if second.Handle != first.Handle || !second.Deduped {
		t.Errorf("同一地址应复用句柄 %s，实际 %s（deduped=%v）", first.Handle, second.Handle, second.Deduped)
	}

	read, err := Read(first.Handle, 0)
	if err != nil || read.Lost {
		t.Fatalf("链接取回失败: %v（lost=%v）", err, read.Lost)
	}
	if !strings.Contains(read.Content, "海边日落") || !strings.Contains(read.Content, "bilibili.com") {
		t.Errorf("链接正文应含站点与摘要，实际 %q", read.Content)
	}
	// 已移除「不得回显完整地址与 BV 号」的限制：链接取回应直接带完整地址。
	if !strings.Contains(read.Content, "BV1xx411c7mD") {
		t.Errorf("链接取回应包含完整地址: content=%q", read.Content)
	}
	if read.Title != "一段视频" {
		t.Errorf("有标题时应保留标题，实际 %q", read.Title)
	}
}

// TestArchiveLegacySchemaMigratesURL 老库升级：没有 url 列的既有库必须能补齐列并正常登记链接
//
// 这里刻意先造一个「只有 blob_owned、没有 url」的旧表：若迁移顺序写错（先建 url 索引再 ALTER），
// 整个资源目录初始化都会失败，表现为所有资源句柄退化。
func TestArchiveLegacySchemaMigratesURL(t *testing.T) {
	tmp := t.TempDir()
	origLocal, origArchive := config.LocalDir, config.ArchiveDBPath
	config.LocalDir = tmp
	config.ArchiveDBPath = filepath.Join(tmp, "database", "archive.db")
	archiveDB = nil
	archiveSeqLoaded = false
	archiveSeqCache = map[string]int64{}
	t.Cleanup(func() {
		_ = Close()
		_ = knowledge.Close()
		config.LocalDir, config.ArchiveDBPath = origLocal, origArchive
		archiveDB = nil
		archiveSeqLoaded = false
		archiveSeqCache = map[string]int64{}
	})
	if err := os.MkdirAll(filepath.Dir(config.ArchiveDBPath), 0755); err != nil {
		t.Fatalf("准备目录失败: %v", err)
	}

	// 1) 造一个老版本表结构（没有 url 列）
	legacy, err := sql.Open("sqlite3", config.ArchiveDBPath)
	if err != nil {
		t.Fatalf("打开老库失败: %v", err)
	}
	if _, err := legacy.Exec(`CREATE TABLE resources (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		kind TEXT NOT NULL DEFAULT '',
		seq INTEGER NOT NULL DEFAULT 0,
		name TEXT NOT NULL DEFAULT '',
		ext TEXT NOT NULL DEFAULT '',
		mime TEXT NOT NULL DEFAULT '',
		content_hash TEXT NOT NULL DEFAULT '',
		blob_path TEXT NOT NULL DEFAULT '',
		size_bytes INTEGER NOT NULL DEFAULT 0,
		width INTEGER NOT NULL DEFAULT 0,
		height INTEGER NOT NULL DEFAULT 0,
		duration_ms INTEGER NOT NULL DEFAULT 0,
		title TEXT NOT NULL DEFAULT '',
		summary TEXT NOT NULL DEFAULT '',
		tags TEXT NOT NULL DEFAULT '[]',
		origin TEXT NOT NULL DEFAULT '',
		retain TEXT NOT NULL DEFAULT 'normal',
		state TEXT NOT NULL DEFAULT 'active',
		section_count INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL DEFAULT 0,
		used_at INTEGER NOT NULL DEFAULT 0,
		blob_owned INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		t.Fatalf("创建老表失败: %v", err)
	}
	_ = legacy.Close()

	// 2) 用当前代码初始化：迁移必须成功，链接登记必须可用
	if err := Init(config.ArchiveDBPath); err != nil {
		t.Fatalf("老库迁移失败: %v", err)
	}
	result, err := AdmitURL(URLAdmitInput{
		URL:    "https://www.bilibili.com/video/BV1xx411c7mD",
		Origin: OriginChannel,
	})
	if err != nil {
		t.Fatalf("迁移后链接入仓失败: %v", err)
	}
	if result.Handle != "@U.001" {
		t.Errorf("迁移后首个链接应为 @U.001，实际 %s", result.Handle)
	}
}

// TestArchiveSectionOutlineAndAnchor 段落表解析 + |§n / |L 锚点整段展开
func TestArchiveSectionOutlineAndAnchor(t *testing.T) {
	setupArchiveTest(t)

	content := "# 一、功能\n功能正文\n## 1.1 用法\n用法正文甲\n用法正文乙\n"
	result, err := Admit(AdmitInput{
		SourceKind: SourceKindBytes,
		Data:       []byte(content),
		Name:       "doc.md",
	})
	if err != nil {
		t.Fatalf("入库失败: %v", err)
	}
	drafts := ParseMarkdownSections(content)
	if len(drafts) != 2 {
		t.Fatalf("应解析出 2 个段落，实际 %d", len(drafts))
	}
	if drafts[0].Path != "一、功能" {
		t.Errorf("首段路径错误: %s", drafts[0].Path)
	}
	if drafts[1].Path != "一、功能 / 1.1 用法" {
		t.Errorf("次段层级路径错误: %s", drafts[1].Path)
	}
	if err := ReplaceSections(result.ID, drafts); err != nil {
		t.Fatalf("写段落表失败: %v", err)
	}

	// |§2 → 第二节整段（含标题行到文档末）
	sectionRead, err := Read(result.Handle+"|§2", 0)
	if err != nil {
		t.Fatalf("按章节取回失败: %v", err)
	}
	if sectionRead.SectionTitle != "1.1 用法" {
		t.Errorf("命中章节标题错误: %s", sectionRead.SectionTitle)
	}
	if !strings.Contains(sectionRead.Content, "用法正文甲") || !strings.Contains(sectionRead.Content, "用法正文乙") {
		t.Errorf("章节正文不完整: %q", sectionRead.Content)
	}

	// |L4-5 → 第 4~5 行
	lineRead, err := Read(result.Handle+"|L4-5", 0)
	if err != nil {
		t.Fatalf("按行区间取回失败: %v", err)
	}
	if lineRead.Content != "用法正文甲\n用法正文乙" {
		t.Errorf("行区间内容错误: %q", lineRead.Content)
	}

	// 大纲接口
	sections, canonical, err := Outline(result.Handle)
	if err != nil {
		t.Fatalf("读大纲失败: %v", err)
	}
	if canonical != result.Handle || len(sections) != 2 {
		t.Errorf("大纲返回异常: handle=%s sections=%d", canonical, len(sections))
	}
}

// TestArchiveDocumentNameIdentityAndSeqMonotonic 文本类按原文件名复用句柄；不同文件名各自独立
func TestArchiveDocumentNameIdentityAndSeqMonotonic(t *testing.T) {
	setupArchiveTest(t)

	payload := []byte("same content")
	first, err := Admit(AdmitInput{SourceKind: SourceKindBytes, Data: payload, Name: "a.txt"})
	if err != nil {
		t.Fatalf("首次入库失败: %v", err)
	}
	if first.BlobPath != "documents/a.txt" {
		t.Errorf("文本类应按原文件名落盘，实际 %s", first.BlobPath)
	}
	// 同一文件名再次入库：复用同一句柄与实体
	again, err := Admit(AdmitInput{SourceKind: SourceKindBytes, Data: payload, Name: "a.txt"})
	if err != nil {
		t.Fatalf("同名二次入库失败: %v", err)
	}
	if again.Handle != first.Handle {
		t.Errorf("同名文档应复用同一句柄: %s / %s", first.Handle, again.Handle)
	}
	if !again.Deduped {
		t.Errorf("同名文档应标记为复用，实际 deduped=%v", again.Deduped)
	}
	if again.BlobPath != first.BlobPath || again.Seq != first.Seq {
		t.Errorf("同名文档应复用同一实体与序号: %s/%d vs %s/%d",
			first.BlobPath, first.Seq, again.BlobPath, again.Seq)
	}
	// 不同文件名即使内容相同，也各占一个实体与句柄
	second, err := Admit(AdmitInput{SourceKind: SourceKindBytes, Data: payload, Name: "b.txt"})
	if err != nil {
		t.Fatalf("不同名入库失败: %v", err)
	}
	if second.Handle == first.Handle {
		t.Errorf("不同文件名应各自分配句柄: %s / %s", first.Handle, second.Handle)
	}
	if second.BlobPath == first.BlobPath || second.BlobPath != "documents/b.txt" {
		t.Errorf("不同文件名应各自落盘: %s / %s", first.BlobPath, second.BlobPath)
	}
	if second.Seq != first.Seq+1 {
		t.Errorf("序号应单调 +1：%d → %d", first.Seq, second.Seq)
	}
}

// TestArchiveImageDimensions 图片入仓与头部尺寸探测
func TestArchiveImageDimensions(t *testing.T) {
	setupArchiveTest(t)

	// 8x5 的合法 PNG（含完整 IHDR）
	png := []byte{
		0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x08, // width = 8
		0x00, 0x00, 0x00, 0x05, // height = 5
		0x08, 0x02, 0x00, 0x00, 0x00,
	}
	result, err := Admit(AdmitInput{SourceKind: SourceKindBytes, Data: png, Name: "pic.png"})
	if err != nil {
		t.Fatalf("图片入库失败: %v", err)
	}
	if result.Kind != KindImage || result.Handle != "@I.001" {
		t.Fatalf("图片类型码/句柄错误: kind=%s handle=%s", result.Kind, result.Handle)
	}
	entry, ok := ByHash(KindImage, result.ContentHash)
	if !ok {
		t.Fatalf("按内容哈希查不到资源")
	}
	if entry.Width != 8 || entry.Height != 5 {
		t.Errorf("尺寸探测错误: %dx%d（期望 8x5）", entry.Width, entry.Height)
	}
}

// TestArchiveLostDegrade 回收后降级：句柄仍可解析，读回标记 lost
func TestArchiveLostDegrade(t *testing.T) {
	setupArchiveTest(t)

	result, err := Admit(AdmitInput{SourceKind: SourceKindBytes, Data: []byte("临时参考物"), Name: "ref.txt", Origin: OriginEngineTemp})
	if err != nil {
		t.Fatalf("入库失败: %v", err)
	}
	// 模拟回收：删除 blob 并标记 lost
	abs := filepath.Join(config.LocalDir, filepath.FromSlash(result.BlobPath))
	if err := os.Remove(abs); err != nil {
		t.Fatalf("删除 blob 失败: %v", err)
	}
	if err := MarkLost(result.ID); err != nil {
		t.Fatalf("标记 lost 失败: %v", err)
	}

	read, err := Read(result.Handle, 0)
	if err != nil {
		t.Fatalf("回收后句柄应仍可解析，实际报错: %v", err)
	}
	if !read.Lost {
		t.Errorf("回收后应标记 lost")
	}
	if read.Handle != result.Handle {
		t.Errorf("句柄文本应保持可用: %s", read.Handle)
	}
}

// TestArchiveWorkspaceSource 工作区来源：只读收录、原文件保留、白名单与越界防护
func TestArchiveWorkspaceSource(t *testing.T) {
	setupArchiveTest(t)

	// 以临时目录为「工作目录」，模拟服务端与工作区同根启动
	workdir := t.TempDir()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("获取当前工作目录失败: %v", err)
	}
	if err := os.Chdir(workdir); err != nil {
		t.Fatalf("切换工作目录失败: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })

	// 1) 文本类文件可被收录，且**原文件必须原地保留**
	docBody := "# 工作区文档\n内容正文\n"
	if err := os.WriteFile(filepath.Join(workdir, "AGENTS.md"), []byte(docBody), 0644); err != nil {
		t.Fatalf("写入工作区文件失败: %v", err)
	}
	result, err := Admit(AdmitInput{SourceKind: SourceKindWorkspace, Source: "AGENTS.md", Name: "AGENTS.md"})
	if err != nil {
		t.Fatalf("工作区文件入库失败: %v", err)
	}
	if result.Kind != KindDocument {
		t.Errorf("工作区 Markdown 应为文档类型，实际 %s", result.Kind)
	}
	if result.BlobPath != "documents/AGENTS.md" {
		t.Errorf("工作区文本应按原文件名落盘，实际 %s", result.BlobPath)
	}
	if _, err := os.Stat(filepath.Join(workdir, "AGENTS.md")); err != nil {
		t.Errorf("工作区原文件绝不能被移动或删除: %v", err)
	}
	read, err := Read(result.Handle, 0)
	if err != nil {
		t.Fatalf("取回工作区资源失败: %v", err)
	}
	if read.Content != docBody {
		t.Errorf("工作区资源正文不一致: %q", read.Content)
	}

	// 2) 非文本扩展名被白名单拒绝
	if err := os.WriteFile(filepath.Join(workdir, "evil.exe"), []byte("MZ"), 0644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}
	if _, err := Admit(AdmitInput{SourceKind: SourceKindWorkspace, Source: "evil.exe", Name: "evil.exe"}); err == nil {
		t.Errorf("非文本扩展名应被拒绝")
	}

	// 3) 目录穿越被拒绝
	if _, err := Admit(AdmitInput{SourceKind: SourceKindWorkspace, Source: "../../../etc/passwd", Name: "passwd"}); err == nil {
		t.Errorf("越界路径应被拒绝")
	}

	// 4) 不存在的文件报错而非静默成功
	if _, err := Admit(AdmitInput{SourceKind: SourceKindWorkspace, Source: "not-exists.md", Name: "not-exists.md"}); err == nil {
		t.Errorf("不存在的文件应报错")
	}

	// 5) 同一工作区文件重复收录：内容哈希相同 → 复用实体
	again, err := Admit(AdmitInput{SourceKind: SourceKindWorkspace, Source: "AGENTS.md", Name: "AGENTS.md"})
	if err != nil {
		t.Fatalf("二次收录失败: %v", err)
	}
	if !again.Deduped {
		t.Errorf("同一内容的二次收录应命中去重")
	}
	if again.BlobPath != result.BlobPath {
		t.Errorf("同一内容应复用同一 blob: %s / %s", result.BlobPath, again.BlobPath)
	}
}

// TestArchivePathSourceKeepsOriginal 回归：path 来源绝不能被移走或删除
//
// 库中曾出现「导入一份配置文件后原文件消失」——根因是早期实现把仓库外的来源
// moveFile 到规范位置（先 rename，跨卷则复制后删除源文件）。本测试覆盖
// LocalDir 内任意子目录（documents/ 等），断言源文件原地保留、内容可经句柄取回。
func TestArchivePathSourceKeepsOriginal(t *testing.T) {
	tmp := setupArchiveTest(t)

	// 模拟前端把文本类文件保存在 LocalDir 的 documents/ 子目录
	docDir := filepath.Join(tmp, "documents")
	if err := os.MkdirAll(docDir, 0755); err != nil {
		t.Fatalf("准备 documents 目录失败: %v", err)
	}
	body := "{\"模型的路径\": \"/models/llama.cpp\", \"端口\": 36789}\n"
	source := filepath.Join(docDir, "config.json")
	if err := os.WriteFile(source, []byte(body), 0644); err != nil {
		t.Fatalf("写入源文件失败: %v", err)
	}

	result, err := Admit(AdmitInput{
		SourceKind: SourceKindPath,
		Source:     "documents/config.json",
		Name:       "config.json",
		Origin:     OriginClientUpload,
	})
	if err != nil {
		t.Fatalf("入库失败: %v", err)
	}

	// 1) 源文件必须仍在原位
	if !fileExists(source) {
		t.Fatalf("path 来源的源文件被移走或删除了 —— 这正是「导入后文件消失」的回归点")
	}
	got, err := os.ReadFile(source)
	if err != nil || string(got) != body {
		t.Fatalf("源文件内容被改动: err=%v body=%q", err, string(got))
	}
	// 2) 内容仓库里应有可引用的存档副本
	abs := filepath.Join(tmp, filepath.FromSlash(result.BlobPath))
	if !fileExists(abs) {
		t.Fatalf("内容仓库未落地实体: %s", result.BlobPath)
	}
	// 3) 句柄可取回原文
	read, err := Read(result.Handle, 0)
	if err != nil {
		t.Fatalf("取回失败: %v", err)
	}
	if read.Content != body {
		t.Errorf("取回内容与源文件不一致:\n期望 %q\n实际 %q", body, read.Content)
	}
	// 4) 二次入库（同内容）同样不能动源文件
	if _, err := Admit(AdmitInput{
		SourceKind: SourceKindPath,
		Source:     "documents/config.json",
		Name:       "config.json",
		Origin:     OriginClientUpload,
	}); err != nil {
		t.Fatalf("二次入库失败: %v", err)
	}
	if !fileExists(source) {
		t.Fatalf("二次入库后源文件消失")
	}
}

// TestArchiveSearch 跨资源搜索命中标题
func TestArchiveSearch(t *testing.T) {
	setupArchiveTest(t)

	if _, err := Admit(AdmitInput{SourceKind: SourceKindBytes, Data: []byte("配置项说明"), Name: "配置手册.md"}); err != nil {
		t.Fatalf("入库失败: %v", err)
	}
	hits, err := Search("配置手册", "", 10)
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}
	if len(hits) == 0 {
		t.Fatalf("搜索「配置手册」应有命中")
	}
	if hits[0].Kind != KindDocument {
		t.Errorf("命中类型码错误: %s", hits[0].Kind)
	}
}

// TestArchiveDataURI data URI 来源归一
func TestArchiveDataURI(t *testing.T) {
	setupArchiveTest(t)

	raw := []byte("# 内联文档\n正文\n")
	dataURI := "data:text/markdown;base64," + base64.StdEncoding.EncodeToString(raw)
	result, err := Admit(AdmitInput{SourceKind: SourceKindData, Source: dataURI, Name: "inline.md"})
	if err != nil {
		t.Fatalf("data URI 入库失败: %v", err)
	}
	if result.BlobPath != "documents/inline.md" {
		t.Errorf("data URI 文本应按原文件名落盘，实际 %s", result.BlobPath)
	}
	read, err := Read(result.Handle, 0)
	if err != nil {
		t.Fatalf("取回失败: %v", err)
	}
	if read.Content != string(raw) {
		t.Errorf("内联内容不一致: %q", read.Content)
	}
}

// TestArchiveLocalPathNoCopy 前端已落盘的本地文件只登记不复制（零拷贝路径）
func TestArchiveLocalPathNoCopy(t *testing.T) {
	tmp := setupArchiveTest(t)

	// 模拟前端已按 sha256 前 16 位命名落盘
	source := filepath.Join(tmp, "multimedia", "client-uploaded.png")
	png := []byte{
		0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x10,
		0x08, 0x02, 0x00, 0x00, 0x00,
	}
	if err := os.WriteFile(source, png, 0644); err != nil {
		t.Fatalf("写入模拟上传文件失败: %v", err)
	}

	result, err := Admit(AdmitInput{
		SourceKind: SourceKindPath,
		Source:     "multimedia/client-uploaded.png",
		Name:       "client-uploaded.png",
		Origin:     OriginClientUpload,
	})
	if err != nil {
		t.Fatalf("本地路径入库失败: %v", err)
	}
	// 原文件仍在原位（未被移动/重命名）
	if _, err := os.Stat(source); err != nil {
		t.Errorf("前端已落盘的文件不应被移动: %v", err)
	}
	// 媒体存储内的实体直接引用：既不搬移，也不复制副本
	if result.BlobPath != "multimedia/client-uploaded.png" {
		t.Errorf("媒体存储内的实体应原地引用，实际 blob=%s", result.BlobPath)
	}
	// 受保护实体：不得复制副本，也不得被回收器回收（blob_owned=false）
	if _, err := os.Stat(filepath.Join(tmp, "documents", filepath.Base(source))); err == nil {
		t.Errorf("不应为已有实体创建归档副本")
	}
	entry, ok := ByHash(KindImage, result.ContentHash)
	if !ok {
		t.Fatalf("按哈希查不到资源")
	}
	if entry.BlobOwned {
		t.Errorf("前端上传件不得标记为归档所有（否则回收器会删掉用户可见文件）")
	}
	if result.Kind != KindImage {
		t.Errorf("类型码应为 I，实际 %s", result.Kind)
	}
}

// TestArchiveOwnedBlobLayout 归档副本按分类进两棵树根目录，归属由 blob_owned 表达
func TestArchiveOwnedBlobLayout(t *testing.T) {
	setupArchiveTest(t)

	// 非媒体类（JSON 配置）→ documents/ 根目录，且标记为归档所有（可回收）
	doc, err := Admit(AdmitInput{
		SourceKind: SourceKindBytes,
		Data:       []byte("{\"port\": 36789}\n"),
		Name:       "config.json",
		Origin:     OriginClientUpload,
	})
	if err != nil {
		t.Fatalf("文档入库失败: %v", err)
	}
	if !strings.HasPrefix(doc.BlobPath, "documents/") || strings.Contains(doc.BlobPath, "archive/") {
		t.Errorf("非媒体类归档副本应直接落在 documents/ 根目录，实际 %s", doc.BlobPath)
	}
	// 文本类不记录内容哈希，按实体路径查询
	docEntry, ok := archiveDB.FindResourceByBlobPath(doc.BlobPath)
	if !ok {
		t.Fatalf("按路径查不到文档资源")
	}
	if doc.ContentHash != "" {
		t.Errorf("文本类不应记录内容哈希，实际 %s", doc.ContentHash)
	}
	if !docEntry.BlobOwned {
		t.Errorf("字节流来源的副本应标记为归档所有（可回收）")
	}

	// 媒体类：让媒体树内**已有**实体（模拟前端上传件）→ 应直接引用且受保护
	png := []byte{
		0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x04,
		0x08, 0x02, 0x00, 0x00, 0x00,
	}
	protectedDir := filepath.Join(config.LocalDir, "multimedia")
	if err := os.MkdirAll(protectedDir, 0755); err != nil {
		t.Fatalf("准备媒体目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(protectedDir, "uploaded.png"), png, 0644); err != nil {
		t.Fatalf("写入模拟上传件失败: %v", err)
	}
	media, err := Admit(AdmitInput{
		SourceKind: SourceKindPath,
		Source:     "multimedia/uploaded.png",
		Name:       "uploaded.png",
		Origin:     OriginClientUpload,
	})
	if err != nil {
		t.Fatalf("媒体入库失败: %v", err)
	}
	if media.BlobPath != "multimedia/uploaded.png" {
		t.Errorf("媒体树内已有实体应原地引用，实际 %s", media.BlobPath)
	}
	mediaEntry, ok := ByHash(KindImage, media.ContentHash)
	if !ok {
		t.Fatalf("按哈希查不到媒体资源")
	}
	if mediaEntry.BlobOwned {
		t.Errorf("前端上传件必须标记为受保护（blob_owned=false）")
	}
	if IsMediaExtension(".json") {
		t.Errorf(".json 不应被判为媒体类")
	}
	if !IsMediaExtension(".png") || !IsMediaExtension(".mp4") || !IsMediaExtension(".wav") {
		t.Errorf("图片/视频/音频扩展名应判为媒体类")
	}

	// 实体必须真的落地，且能按句柄取回
	abs := filepath.Join(config.LocalDir, filepath.FromSlash(doc.BlobPath))
	if !fileExists(abs) {
		t.Fatalf("归档副本未落地: %s", abs)
	}
	read, err := Read(doc.Handle, 0)
	if err != nil || read.Content != "{\"port\": 36789}\n" {
		t.Errorf("取回内容异常: err=%v content=%q", err, read.Content)
	}
}

// TestArchiveSeqPersistAcrossRestart 序号跨重启持久（句柄重启后可复活）
func TestArchiveSeqPersistAcrossRestart(t *testing.T) {
	setupArchiveTest(t)

	first, err := Admit(AdmitInput{SourceKind: SourceKindBytes, Data: []byte("持久化测试"), Name: "persist.txt"})
	if err != nil {
		t.Fatalf("入库失败: %v", err)
	}
	// 模拟进程重启：关闭连接（实际进程退出即关闭），仅保留数据库文件与序号元数据
	if err := Close(); err != nil {
		t.Fatalf("关闭资源目录连接失败: %v", err)
	}
	archiveSeqLoaded = false
	archiveSeqCache = map[string]int64{}
	if err := Init(config.ArchiveDBPath); err != nil {
		t.Fatalf("重启后初始化失败: %v", err)
	}

	// 原句柄仍解析到同一资源
	read, err := Read(first.Handle, 0)
	if err != nil {
		t.Fatalf("重启后句柄应仍可解析: %v", err)
	}
	if read.Lost || read.Content != "持久化测试" {
		t.Errorf("重启后内容不一致: lost=%v content=%q", read.Lost, read.Content)
	}
	// 新入库序号继续递增，不复用
	second, err := Admit(AdmitInput{SourceKind: SourceKindBytes, Data: []byte("第二份"), Name: "second.txt"})
	if err != nil {
		t.Fatalf("重启后入库失败: %v", err)
	}
	if second.Seq <= first.Seq {
		t.Errorf("重启后序号应继续递增: %d → %d", first.Seq, second.Seq)
	}
}

// TestStorageScanRegistersAndCleans 启动扫描：三棵内容仓库树全部登记，失效句柄被清理
func TestStorageScanRegistersAndCleans(t *testing.T) {
	tmp := setupArchiveTest(t)

	// 文本（无哈希）、音频、图片各一份
	if err := os.MkdirAll(filepath.Join(tmp, "documents"), 0755); err != nil {
		t.Fatalf("创建 documents 失败: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "audios"), 0755); err != nil {
		t.Fatalf("创建 audios 失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "documents", "note.md"), []byte("# 标题\n正文\n"), 0644); err != nil {
		t.Fatalf("写入文本失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "audios", "clip.wav"), []byte("RIFF....WAVE"), 0644); err != nil {
		t.Fatalf("写入音频失败: %v", err)
	}
	png := []byte{
		0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x04,
		0x08, 0x02, 0x00, 0x00, 0x00,
	}
	if err := os.WriteFile(filepath.Join(tmp, "multimedia", "pic.png"), png, 0644); err != nil {
		t.Fatalf("写入图片失败: %v", err)
	}

	if err := SyncStorageHandles(); err != nil {
		t.Fatalf("同步失败: %v", err)
	}
	textRes, ok := ForPath("documents/note.md")
	if !ok || textRes.Kind != KindDocument || textRes.ContentHash != "" {
		t.Errorf("文本登记异常: ok=%v kind=%s hash=%q", ok, textRes.Kind, textRes.ContentHash)
	}
	audioRes, ok := ForPath("audios/clip.wav")
	if !ok || audioRes.Kind != KindAudio || audioRes.ContentHash == "" {
		t.Errorf("音频登记异常: ok=%v kind=%s hash=%q", ok, audioRes.Kind, audioRes.ContentHash)
	}
	imageRes, ok := ForPath("multimedia/pic.png")
	if !ok || imageRes.Kind != KindImage || imageRes.ContentHash == "" {
		t.Errorf("图片登记异常: ok=%v kind=%s hash=%q", ok, imageRes.Kind, imageRes.ContentHash)
	}

	// 删除音频实体后再次同步：句柄必须被清理
	if err := os.Remove(filepath.Join(tmp, "audios", "clip.wav")); err != nil {
		t.Fatalf("删除音频失败: %v", err)
	}
	if err := SyncStorageHandles(); err != nil {
		t.Fatalf("二次同步失败: %v", err)
	}
	if _, ok := ForPath("audios/clip.wav"); ok {
		t.Errorf("实体丢失后句柄应被清理")
	}
}

// TestStorageScanDedupesRootCopy 启动扫描同哈希的根目录副本与生成目录实体：保留生成目录
func TestStorageScanDedupesRootCopy(t *testing.T) {
	tmp := setupArchiveTest(t)
	png := []byte{
		0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x04,
		0x08, 0x02, 0x00, 0x00, 0x00,
	}
	generatedDir := filepath.Join(tmp, "multimedia", "generated")
	if err := os.MkdirAll(generatedDir, 0755); err != nil {
		t.Fatalf("创建生成目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(generatedDir, "pic.png"), png, 0644); err != nil {
		t.Fatalf("写入生成目录实体失败: %v", err)
	}
	rootPath := filepath.Join(tmp, "multimedia", "pic.png")
	if err := os.WriteFile(rootPath, png, 0644); err != nil {
		t.Fatalf("写入根目录副本失败: %v", err)
	}

	if err := SyncStorageHandles(); err != nil {
		t.Fatalf("同步失败: %v", err)
	}
	if _, err := os.Stat(rootPath); !os.IsNotExist(err) {
		t.Errorf("根目录重复副本应被扫描清理")
	}
	if _, err := os.Stat(filepath.Join(generatedDir, "pic.png")); err != nil {
		t.Errorf("生成目录实体应保留: %v", err)
	}
	res, ok := ForPath("multimedia/generated/pic.png")
	if !ok || res.Handle == "" {
		t.Errorf("生成目录实体应获得句柄")
	}
	if _, ok := ForPath("multimedia/pic.png"); ok {
		t.Errorf("根目录重复件不应保留目录行")
	}
}

// TestHashUniquenessPrefersGeneratedDir 同哈希只保留一份实体：生成目录优先于根目录
func TestHashUniquenessPrefersGeneratedDir(t *testing.T) {
	tmp := setupArchiveTest(t)
	png := []byte{
		0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x04,
		0x08, 0x02, 0x00, 0x00, 0x00,
	}

	// 场景 1：生成目录先有实体，随后根目录收到同内容副本 → 根目录副本被舍弃
	generated, err := persistence.SaveFileEx("multimedia/generated/pic.png", persistence.SaveOptions{Overwrite: true, MustStore: true}, bytes.NewReader(png))
	if err != nil {
		t.Fatalf("生成目录写入失败: %v", err)
	}
	copied, err := persistence.SaveFileEx("multimedia/pic.png", persistence.SaveOptions{Overwrite: true, MustStore: true}, bytes.NewReader(png))
	if err != nil {
		t.Fatalf("根目录副本写入失败: %v", err)
	}
	if copied.RelPath != generated.RelPath || copied.Handle != generated.Handle || !copied.Deduped {
		t.Errorf("应复用生成目录实体: generated=%s/%s copied=%s/%s", generated.Handle, generated.RelPath, copied.Handle, copied.RelPath)
	}
	if _, statErr := os.Stat(filepath.Join(tmp, "multimedia", "pic.png")); !os.IsNotExist(statErr) {
		t.Errorf("根目录重复副本应被舍弃")
	}
	if _, statErr := os.Stat(filepath.Join(tmp, "multimedia", "generated", "pic.png")); statErr != nil {
		t.Errorf("生成目录实体应保留: %v", statErr)
	}

	// 场景 2：根目录先有实体，随后生成目录出现同内容 → 根目录被删除，句柄迁移到生成目录
	png2 := append(append([]byte{}, png...), 0x00)
	root, err := persistence.SaveFileEx("multimedia/root2.png", persistence.SaveOptions{Overwrite: true, MustStore: true}, bytes.NewReader(png2))
	if err != nil {
		t.Fatalf("根目录写入失败: %v", err)
	}
	migrated, err := persistence.SaveFileEx("multimedia/generated/pic2.png", persistence.SaveOptions{Overwrite: true, MustStore: true}, bytes.NewReader(png2))
	if err != nil {
		t.Fatalf("生成目录迁移写入失败: %v", err)
	}
	if migrated.RelPath != "multimedia/generated/pic2.png" || migrated.Handle != root.Handle {
		t.Errorf("句柄应迁移到生成目录: root=%s/%s migrated=%s/%s", root.Handle, root.RelPath, migrated.Handle, migrated.RelPath)
	}
	if _, statErr := os.Stat(filepath.Join(tmp, "multimedia", "root2.png")); !os.IsNotExist(statErr) {
		t.Errorf("根目录旧实体应被删除")
	}

	// 每个内容哈希在目录中只能有一行
	rows, err := archiveDB.ListAllResources()
	if err != nil {
		t.Fatalf("列目录失败: %v", err)
	}
	byHash := map[string]int{}
	for _, row := range rows {
		if row.ContentHash != "" {
			byHash[row.Kind+"\x00"+row.ContentHash]++
		}
	}
	for key, count := range byHash {
		if count != 1 {
			t.Errorf("内容哈希 %s 存在 %d 条目录行，应唯一", key, count)
		}
	}
}

// TestArchiveGrammar 句柄文法解析与格式化
func TestArchiveGrammar(t *testing.T) {
	cases := []struct {
		raw     string
		kind    string
		seq     int64
		section int
		lineA   int
	}{
		{"@D.003", KindDocument, 3, 0, 0},
		{"@I.007", KindImage, 7, 0, 0},
		{"@V.001", KindVideo, 1, 0, 0},
		{"@A.002", KindAudio, 2, 0, 0},
		{"@D.003|§4", KindDocument, 3, 4, 0},
		{"@D.003|L120-168", KindDocument, 3, 0, 120},
		{"@D.1234", KindDocument, 1234, 0, 0},
	}
	for _, c := range cases {
		handle, ok := ParseHandle(c.raw)
		if !ok {
			t.Errorf("应能解析 %s", c.raw)
			continue
		}
		if handle.Kind != c.kind || handle.Seq != c.seq {
			t.Errorf("%s 解析错误: kind=%s seq=%d", c.raw, handle.Kind, handle.Seq)
		}
		if handle.SectionOrdinal != c.section {
			t.Errorf("%s 章节锚点错误: %d", c.raw, handle.SectionOrdinal)
		}
		if handle.LineStart != c.lineA {
			t.Errorf("%s 行锚点错误: %d", c.raw, handle.LineStart)
		}
		if FormatHandle(handle.Kind, handle.Seq) != "@"+c.kind+"."+strings.TrimPrefix(strings.Split(c.raw, "|")[0], "@"+c.kind+".") {
			t.Errorf("%s 格式化不一致: %s", c.raw, FormatHandle(handle.Kind, handle.Seq))
		}
	}
	// 非法输入必须被拒绝
	for _, bad := range []string{"图片.003", "#file.md", "@X.001", "@D.1", "D.003", "@D.003|§", "@D.003|L5"} {
		if _, ok := ParseHandle(bad); ok {
			t.Errorf("非法句柄不应被接受: %s", bad)
		}
	}
}

// TestArchiveAdmitCorrectsImageExtByContent GIF 内容以 .png 命名入库时，
// 落盘扩展名与 MIME 必须按真实字节纠正（QQ 等渠道常把动图命名成静态扩展名）。
func TestArchiveAdmitCorrectsImageExtByContent(t *testing.T) {
	setupArchiveTest(t)

	frame := image.NewPaletted(image.Rect(0, 0, 8, 8), color.Palette{color.Black})
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, &gif.GIF{
		Image: []*image.Paletted{frame, frame, frame},
		Delay: []int{10, 10, 10},
	}); err != nil {
		t.Fatalf("编码测试动图失败: %v", err)
	}

	result, err := Admit(AdmitInput{
		SourceKind: SourceKindBytes,
		Data:       buf.Bytes(),
		Name:       "image.png",
		KindHint:   KindImage,
		Origin:     OriginChannel,
	})
	if err != nil {
		t.Fatalf("入库失败: %v", err)
	}
	if result.MIME != "image/gif" {
		t.Errorf("MIME 应按内容纠正为 image/gif，实际 %s", result.MIME)
	}
	if !strings.HasSuffix(result.BlobPath, ".gif") {
		t.Errorf("落盘扩展名应按内容纠正为 .gif，实际 %s", result.BlobPath)
	}
	if _, statErr := os.Stat(filepath.Join(config.LocalDir, filepath.FromSlash(result.BlobPath))); statErr != nil {
		t.Errorf("纠正后的实体未落盘: %v", statErr)
	}
}
