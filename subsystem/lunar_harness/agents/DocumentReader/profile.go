package DocumentReader

// profile.go 内容格式自动识别：按扩展名、内容结构、文件大小三路判定。
//
// 识别结果直接进入探查简报与 file_profile 工具，模型据此选择
// 内联、按行读取还是严格分批的处理策略。

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"LunarSubsystem/LunarHarness/engine/resource"
)

// 扩展名分类表：同类扩展名共享处理策略，避免在各处重复判断。
var (
	archiveExts = map[string]bool{
		".zip": true, ".7z": true, ".rar": true, ".tar": true,
		".gz": true, ".bz2": true, ".xz": true, ".ltpx": true,
	}
	modelExts = map[string]bool{".gguf": true, ".safetensors": true, ".onnx": true, ".pt": true}
	codeExts  = map[string]bool{
		".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true, ".cjs": true,
		".py": true, ".rs": true, ".java": true, ".kt": true, ".c": true, ".h": true, ".cpp": true,
		".cxx": true, ".hpp": true, ".cs": true, ".rb": true, ".php": true, ".swift": true,
		".sh": true, ".ps1": true, ".bat": true, ".cmd": true, ".sql": true, ".html": true,
		".htm": true, ".css": true, ".vue": true, ".lua": true, ".r": true,
	}
	markdownExts = map[string]bool{".md": true, ".markdown": true}
	tableExts    = map[string]bool{".csv": true, ".tsv": true, ".jsonl": true, ".ndjson": true}
	configExts   = map[string]bool{
		".json": true, ".xml": true, ".yaml": true, ".yml": true, ".toml": true,
		".ini": true, ".cfg": true, ".conf": true, ".env": true, ".editorconfig": true,
	}
	binaryDocExts = map[string]bool{
		".pdf": true, ".docx": true, ".xlsx": true, ".pptx": true, ".doc": true,
		".xls": true, ".ppt": true, ".ttf": true, ".otf": true, ".woff": true, ".woff2": true,
	}
)

// detectProfile 自动识别文件内容格式（扩展名 → 内容结构 → 规模策略）
func detectProfile(res resource.Resource, size int64, text string) formatProfile {
	ext := strings.ToLower(strings.TrimSpace(res.Ext))
	if ext == "" {
		ext = strings.ToLower(filepath.Ext(res.Name))
	}
	profile := formatProfile{Ext: ext, SizeBytes: size, Encoding: "utf-8"}

	kind, label, binary := classifyExt(ext, res.Name)
	// 扩展名未知或纯文本时，用内容结构进一步判定（小说 / 源码 / 表格 / 配置）
	if !binary && (kind == "prose" || kind == "text") {
		kind, label = classifyByContent(text, kind, label)
	}
	// 二进制探针：明确二进制扩展名或采样中出现 NUL 字节
	if !binary && looksBinary(text) {
		binary = true
		kind, label = "archive", "二进制内容"
	}
	profile.Kind = kind
	profile.Label = label
	profile.Runes = utf8.RuneCountInString(text)
	if binary {
		profile.Encoding = "binary"
		profile.Lines = strings.Count(text, "\n") + 1
	} else {
		profile.Lines = strings.Count(text, "\n") + 1
	}
	profile.Binary = binary
	profile.SizeLabel, profile.Strategy, profile.Large = sizeHint(profile.Runes)
	return profile
}

// classifyExt 按扩展名给出格式类别与中文名
func classifyExt(ext, name string) (kind, label string, binary bool) {
	lowerName := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lowerName, ".d.ts"):
		return "dts", "类型声明（.d.ts）", false
	case archiveExts[ext]:
		return "archive", "压缩包", true
	case modelExts[ext]:
		return "model", "模型权重", true
	case codeExts[ext]:
		return "source", "源代码", false
	case markdownExts[ext]:
		return "markdown", "Markdown 文档", false
	case tableExts[ext]:
		return "table", "表格文本", false
	case configExts[ext]:
		return "config", "配置文档", false
	case ext == ".log":
		return "log", "日志文本", false
	case binaryDocExts[ext]:
		return "document", "二进制文档", true
	case ext == ".txt":
		return "prose", "纯文本", false
	case ext == "":
		return "text", "无扩展名文本", false
	}
	return "text", "文本", false
}

// classifyByContent 用内容结构细化未知文本的类别
func classifyByContent(text, kind, label string) (string, string) {
	switch {
	case looksMarkdown(text):
		return "markdown", "Markdown 文档"
	case looksConfig(text):
		return "config", "结构化配置"
	case looksTable(text):
		return "table", "表格文本"
	case looksSource(text):
		return "source", "源代码"
	}
	return kind, label
}

// looksBinary 采样中是否出现 NUL 字节
func looksBinary(text string) bool {
	sample := text
	if len(sample) > profileHeadBytes {
		sample = sample[:profileHeadBytes]
	}
	return strings.IndexByte(sample, 0) >= 0
}

// looksMarkdown 是否存在 Markdown 标题行
func looksMarkdown(text string) bool {
	return scanHeadLines(text, 40, func(line string) bool {
		return reMDHeading.MatchString(strings.TrimSpace(line))
	})
}

// looksSource 是否呈现源码特征（顶格声明 / 预处理指令 / 大括号块）
func looksSource(text string) bool {
	return scanHeadLines(text, 60, func(line string) bool {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			return false
		}
		for _, prefix := range []string{"package ", "func ", "#include", "import ", "from ", "def ", "class ", "public ", "private "} {
			if strings.HasPrefix(trimmed, prefix) {
				return true
			}
		}
		return false
	})
}

// looksTable 前几行是否呈稳定的逗号 / 制表符分列结构
func looksTable(text string) bool {
	lines := headLines(text, 8)
	commas, tabs := 0, 0
	for _, line := range lines {
		if strings.Contains(line, "\t") {
			tabs++
		}
		if strings.Contains(line, ",") {
			commas++
		}
	}
	return len(lines) >= 2 && (commas >= 2 || tabs >= 2)
}

// looksConfig 是否以 JSON / XML 结构开头
func looksConfig(text string) bool {
	trimmed := strings.TrimSpace(text)
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "<?xml")
}

// scanHeadLines 在前 n 行中查找满足条件的一行
func scanHeadLines(text string, n int, match func(string) bool) bool {
	for _, line := range headLines(text, n) {
		if match(line) {
			return true
		}
	}
	return false
}

// headLines 取前 n 行（按字节采样后再切行）
func headLines(text string, n int) []string {
	sample := text
	if len(sample) > profileHeadBytes {
		sample = sample[:profileHeadBytes]
	}
	lines := strings.Split(sample, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return lines
}

// sizeHint 按字符数给出规模档位与处理策略
func sizeHint(runes int) (sizeLabel, strategy string, large bool) {
	switch {
	case runes <= inlineContentLimit:
		return "小型", "整份内联", false
	case runes <= largeRunes:
		return "中型", "分批读取（单次 ≤10000 字，按行区间）", true
	case runes <= hugeRunes:
		return "大型", "分批读取（单次 ≤10000 字，优先 chunk_plan）", true
	default:
		return "超大型", "强制分批（单次 ≤10000 字，优先 chunk_plan；超上限截断保护）", true
	}
}

// formatProfileText 把格式画像渲染为模型可读的一行
func formatProfileText(profile formatProfile) string {
	return fmt.Sprintf("%s（%s）｜编码 %s｜规模 %s：%d 字 / %d 行 / %d 字节｜策略：%s",
		profile.Label, emptyExt(profile.Ext), profile.Encoding, profile.SizeLabel,
		profile.Runes, profile.Lines, profile.SizeBytes, profile.Strategy)
}

// emptyExt 扩展名为空时给出占位
func emptyExt(ext string) string {
	if strings.TrimSpace(ext) == "" {
		return "无扩展名"
	}
	return ext
}
