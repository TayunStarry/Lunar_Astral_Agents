package handlers

import (
	"regexp"
	"strings"
	"testing"
)

// 测试用的多声部 ABC 乐谱
const testABCMultiVoice = `%%prog 1 56
%%prog 2 42
%%prog 3 40
X:1
T:曙光的赞歌
M:4/4
L:1/8
Q:1/4=130
K:C
!mp! [V:1] c2 e2 g2 e2 | f2 a2 g2 e2 |
!mf! [V:2] [C,,E,,G,,]4 | [F,,A,,C,]4 |
!p!   [V:3] C,2 G,2 C2 G,2 | F,2 C2 A,2 C2 |`

// ==== 问题1: indexOfPattern("K:.*$") 在 Go 非多行模式下完全不匹配 ====
// Go 的 regexp 中: "." 默认不匹配 \n，"$" 默认只匹配文本末尾。
// K:.*$ 要求: K: + 非\n字符的贪婪序列 + 文本末尾。
// 但 K:C 之后有 \n 和更多内容，所以 ".*" 停在 \n 前，"$" 无法匹配 → 整个正则失败。
// 这导致 keyIdx = -1，parseABCForMIDI 立即返回 0 个音符 — 这是首要 bug！

func TestIndexOfPattern_KLine(t *testing.T) {
	t.Run("K:.*$在Go默认模式下不匹配", func(t *testing.T) {
		re := regexp.MustCompile(`K:.*$`)
		matches := re.FindStringIndex(testABCMultiVoice)

		if matches == nil {
			t.Log("✓ 确认：K:.*$ 在 Go 默认模式下无法匹配含有换行符的 ABC 乐谱")
			t.Log("  原因：Go regexp 中 '.' 不匹配 \\n，'$' 只匹配文本末尾")
			t.Log("  K:.* 只能匹配 'K:C'（到换行符为止），但 $ 要求文本末尾，矛盾 → 不匹配")
			t.Log("  后果：indexOfPattern 返回 -1，parseABCForMIDI 立即返回 0 个音符")
		} else {
			t.Fatalf("K:.*$ 意外匹配了，位置: %v", matches)
		}
	})

	t.Run("indexOfPattern实际返回值", func(t *testing.T) {
		keyIdx := indexOfPattern(testABCMultiVoice, `K:.*$`)
		t.Logf("indexOfPattern(abcNotation, \"K:.*$\") = %d", keyIdx)
		if keyIdx < 0 {
			t.Log("✓ 确认：indexOfPattern 返回 -1，函数将提前返回 0 个音符")
		}
	})

	// 对比：正确的正则写法
	t.Run("正确写法对比", func(t *testing.T) {
		// 方案1: (?m) 多行模式，$ 匹配行尾
		reMultiline := regexp.MustCompile(`(?m)K:.*$`)
		loc1 := reMultiline.FindStringIndex(testABCMultiVoice)
		if loc1 != nil {
			t.Logf("方案1 (?m)K:.*$ 匹配成功: 位置[%d, %d), 内容=%q",
				loc1[0], loc1[1], testABCMultiVoice[loc1[0]:loc1[1]])
		}

		// 方案2: 简单的 K: 前缀匹配（不需要 .*$）
		reSimple := regexp.MustCompile(`K:`)
		loc2 := reSimple.FindStringIndex(testABCMultiVoice)
		if loc2 != nil {
			t.Logf("方案2 K: 匹配成功: 位置[%d, %d)", loc2[0], loc2[1])
		}

		// 方案3: (?s) 让 . 匹配 \n，.*$ 会从 K: 匹配到文本末尾
		reDotAll := regexp.MustCompile(`(?s)K:.*$`)
		loc3 := reDotAll.FindStringIndex(testABCMultiVoice)
		if loc3 != nil {
			t.Logf("方案3 (?s)K:.*$ 匹配成功: 位置[%d, %d), 长度=%d",
				loc3[0], loc3[1], loc3[1]-loc3[0])
		}
	})

	// 用正确的 K: 定位后，验证 musicBody 提取
	t.Run("正确K定位后musicBody提取", func(t *testing.T) {
		reSimple := regexp.MustCompile(`K:`)
		loc := reSimple.FindStringIndex(testABCMultiVoice)
		if loc == nil {
			t.Fatal("K: 仍未匹配")
		}
		keyIdx := loc[0]
		musicBody := testABCMultiVoice[keyIdx:]
		if newlineIdx := strings.Index(musicBody, "\n"); newlineIdx >= 0 {
			musicBody = musicBody[newlineIdx+1:]
		}
		t.Logf("musicBody 前120字符: %q", truncate(musicBody, 120))

		if musicBody == "" {
			t.Fatal("musicBody 为空")
		}
		if !strings.Contains(musicBody, "[V:1]") {
			t.Error("musicBody 不包含 [V:1]")
		}
		if !strings.Contains(musicBody, "[V:2]") {
			t.Error("musicBody 不包含 [V:2]")
		}
		if !strings.Contains(musicBody, "[V:3]") {
			t.Error("musicBody 不包含 [V:3]")
		}
	})
}

// ==== 问题2: findAllMatches 返回 m[1:]，但调用代码使用 m[1]~m[5] 的索引错位 ====
// findAllMatches 对每个 FindAllStringSubmatch 结果执行 m[1:]，剥离了 group0。
// 5个捕获组的正则产生 m[1:] 有5个元素（索引0-4）。
// 但 abc_midi.go L340-344 使用 m[1]~m[5]，其中 m[5] 越界 → panic。
// 即使不 panic，m[1] 实际是 group2(noteName) 而非 group1(accidental) — off-by-one。

func TestFindAllMatches_IndexMismatch(t *testing.T) {
	noteRegex := `([\^=_]*)([a-gA-Gz])([,']*)(\d*)(\/?\d*)`

	t.Run("单音符c2_验证切片结构", func(t *testing.T) {
		matches := findAllMatches("c2", noteRegex)
		if len(matches) == 0 {
			t.Fatal("findAllMatches 未匹配到任何音符")
		}
		m := matches[0]
		t.Logf("findAllMatches(\"c2\") 每组长度=%d, 内容=%v", len(m), m)

		// 正确的索引（m[1:] 后，索引从0开始）
		t.Logf("  正确: m[0]=%q(accidental) m[1]=%q(noteName) m[2]=%q(octave) m[3]=%q(durationNum) m[4]=%q(durationFrac)",
			m[0], m[1], m[2], m[3], m[4])

		// 调用代码(abc_midi.go L340-344)的索引映射
		t.Log("")
		t.Log("--- abc_midi.go L340-344 的索引映射（off-by-one）---")
		t.Logf("  accidental = m[1] = %q  ← 实际是 noteName，应该用 m[0]", m[1])
		t.Logf("  noteName   = m[2] = %q  ← 实际是 octave，应该用 m[1]", m[2])
		t.Logf("  octave     = m[3] = %q  ← 实际是 durationNum，应该用 m[2]", m[3])
		t.Logf("  durationNum= m[4] = %q  ← 实际是 durationFrac，应该用 m[3]", m[4])
		t.Logf("  durationFrac= m[5]      ← 索引越界！切片长度=%d，最大索引=%d", len(m), len(m)-1)

		// 验证值
		if m[0] != "" {
			t.Errorf("m[0](accidental) 期望\"\", 实际%q", m[0])
		}
		if m[1] != "c" {
			t.Errorf("m[1](noteName) 期望\"c\", 实际%q", m[1])
		}
		if m[3] != "2" {
			t.Errorf("m[3](durationNum) 期望\"2\", 实际%q", m[3])
		}
	})

	t.Run("复杂音符^c'2", func(t *testing.T) {
		matches := findAllMatches("^c'2", noteRegex)
		if len(matches) == 0 {
			t.Fatal("未匹配到音符")
		}
		m := matches[0]
		t.Logf("findAllMatches(\"^c'2\") = %v", m)
		if m[0] != "^" {
			t.Errorf("m[0](accidental) 期望\"^\", 实际%q", m[0])
		}
		if m[1] != "c" {
			t.Errorf("m[1](noteName) 期望\"c\", 实际%q", m[1])
		}
		if m[2] != "'" {
			t.Errorf("m[2](octave) 期望\"'\", 实际%q", m[2])
		}
		if m[3] != "2" {
			t.Errorf("m[3](durationNum) 期望\"2\", 实际%q", m[3])
		}
	})

	t.Run("和弦内音符同样的off-by-one", func(t *testing.T) {
		chordRegex := `([\^=_]*)([a-gA-G])([,']*)(\d*)`
		matches := findAllMatches("C,,E,,G,,", chordRegex)
		t.Logf("findAllMatches(\"C,,E,,G,,\", chordRegex) 返回 %d 组匹配", len(matches))
		for i, m := range matches {
			t.Logf("  匹配组 %d: %v (长度=%d)", i, m, len(m))
			if len(m) >= 4 {
				t.Logf("    正确: m[0]=%q(accidental) m[1]=%q(noteName) m[2]=%q(octave) m[3]=%q(durationNum)",
					m[0], m[1], m[2], m[3])
				t.Logf("    调用: m[1]=%q(误当accidental) m[2]=%q(误当noteName) m[3]=%q(误当octave)",
					m[1], m[2], m[3])
			}
		}
	})
}

func TestFindAllMatches_PanicRisk(t *testing.T) {
	noteRegex := `([\^=_]*)([a-gA-Gz])([,']*)(\d*)(\/?\d*)`
	matches := findAllMatches("c2", noteRegex)
	if len(matches) == 0 {
		t.Fatal("未匹配到音符")
	}

	m := matches[0]
	t.Logf("匹配结果: %v (长度=%d)", m, len(m))

	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
				t.Logf("✓ 确认：访问 m[5] 导致 panic: %v", r)
				t.Log("  findAllMatches 返回 m[1:] 有5个元素(索引0-4)")
				t.Log("  调用代码 abc_midi.go L344 访问 m[5] 越界 → panic")
			}
		}()
		_ = m[1]
		_ = m[2]
		_ = m[3]
		_ = m[4]
		_ = m[5] // 越界！
	}()

	if !panicked {
		t.Error("m[5] 访问未 panic（意外）— 切片长度可能 >= 6")
	}
}

// ==== 问题3: [V:N] 行内声部标记被 tokenizeABCLine 误解为和弦块 ====
// parseABCForMIDI 用 ^\[V:(\d+)\] 检测 [V:N]，要求 [V: 在行首(^)。
// 但当力度记号 !mp! 在 [V:1] 前面时，^ 不匹配，[V:1] 进入 tokenizeABCLine。
// tokenizeABCLine 遇到 [ 就按和弦处理，[V:1] 被解析为"和弦"，内容 V:1 不匹配任何音符。

func TestTokenizeABCLine_VoiceMarkerMisinterpreted(t *testing.T) {
	t.Run("力度记号阻止V标记识别", func(t *testing.T) {
		line := "!mp! [V:1] c2 e2 g2 e2 | f2 a2 g2 e2 |"
		trimmed := strings.TrimSpace(line)

		vm := findMatchString(trimmed, `^\[V:(\d+)\]`)
		if vm == nil {
			t.Log("✓ 确认：findMatchString 在行首有 !mp! 时无法匹配 ^\\[V:(\\d+)\\]")
			t.Log("  原因：^ 要求 [V: 在行首，但 !mp! 在前面")
			t.Log("  后果：[V:1] 不会被识别为声部标记，进入 tokenizeABCLine")
		} else {
			t.Logf("  [V:N] 被识别: %v（意外）", vm)
		}

		tokens := tokenizeABCLine(trimmed)
		t.Logf("tokenizeABCLine 输出 %d 个 token:", len(tokens))
		for i, tok := range tokens {
			label := ""
			if len(tok) > 0 && tok[0] == '[' {
				label = " ← 被当作和弦块!"
			}
			t.Logf("  token[%d] = %q%s", i, tok, label)
		}

		v1AsChord := false
		for _, tok := range tokens {
			if tok == "[V:1]" || (len(tok) >= 5 && tok[:5] == "[V:1]") {
				v1AsChord = true
				t.Logf("✓ 确认：[V:1] 被 tokenizeABCLine 当作和弦块: %q", tok)
				t.Log("  tokenizeABCLine 遇到 [ 就按和弦处理，无法区分 [V:N] 和 [CEG]")
				t.Log("  后果：[V:1] 的内容 'V:1' 被 parseChordNotes 解析，不匹配任何音符 → 丢弃")
			}
		}
		if !v1AsChord {
			t.Log("[V:1] 未被当作和弦块（可能被力度记号跳过逻辑消费了）")
		}
	})

	t.Run("无力度前缀V标记正确识别", func(t *testing.T) {
		line := "[V:1] c2 e2 g2 e2 |"
		trimmed := strings.TrimSpace(line)
		vm := findMatchString(trimmed, `^\[V:(\d+)\]`)
		if vm != nil {
			t.Logf("✓ 无力度前缀时 [V:N] 正确识别: voice=%s", vm[0])
		} else {
			t.Log("✗ 无力度前缀时 [V:N] 仍未识别（意外）")
		}
	})

	t.Run("tokenizeABCLine直接处理V1", func(t *testing.T) {
		tokens := tokenizeABCLine("[V:1] c2 e2 g2 e2 |")
		t.Logf("tokenizeABCLine(\"[V:1] c2 e2 g2 e2 |\") = %v", tokens)
		for i, tok := range tokens {
			label := ""
			if len(tok) > 0 && tok[0] == '[' {
				label = " ← 被当作和弦块!"
			}
			t.Logf("  token[%d] = %q%s", i, tok, label)
		}
	})
}

// ==== 综合测试: parseABCForMIDI 完整解析 ====

func TestParseABCForMIDI_MultiVoice(t *testing.T) {
	notes, bpm, voiceInstruments, voicePrograms := parseABCForMIDI(testABCMultiVoice)

	t.Logf("解析结果: %d 个音符, BPM=%.1f", len(notes), bpm)
	t.Logf("声部乐器: %v", voiceInstruments)
	t.Logf("声部程序号: %v", voicePrograms)

	if len(notes) == 0 {
		t.Log("")
		t.Log("=== 三个 Bug 的因果链 ===")
		t.Log("Bug1(首要): K:.*$ 在 Go 默认模式下不匹配 → keyIdx=-1 → 函数立即返回 0 音符")
		t.Log("  原因: Go regexp 中 '.' 不匹配 \\n，'$' 只匹配文本末尾")
		t.Log("  修复: 改为 K: 或 (?m)K:.*$")
		t.Log("")
		t.Log("Bug2(修复Bug1后暴露): findAllMatches 返回 m[1:]，但代码用 m[1]~m[5]")
		t.Log("  m[5] 越界 → panic; m[1]~m[4] 全部 off-by-one")
		t.Log("  修复: 将 m[1]~m[5] 改为 m[0]~m[4]")
		t.Log("")
		t.Log("Bug3(修复Bug1+2后暴露): !mp! [V:1] 的 ^\\[V:(\\d+)\\] 因 ^ 不匹配")
		t.Log("  [V:1] 被 tokenizeABCLine 当作和弦块，声部切换失败")
		t.Log("  修复: 在 [V:N] 检测中去掉 ^ 或在行首跳过力度记号")
	}

	voiceNotes := map[int][]abcNote{}
	for _, n := range notes {
		voiceNotes[n.voice] = append(voiceNotes[n.voice], n)
	}
	for voice, vNotes := range voiceNotes {
		t.Logf("声部 %d: %d 个音符", voice, len(vNotes))
		for i, n := range vNotes {
			if i < 5 {
				t.Logf("  [%d] note=%s duration=%.3fs time=%.3fs voice=%d dynamics=%s",
					i, n.note, n.duration, n.time, n.voice, n.dynamics)
			}
		}
		if len(vNotes) > 5 {
			t.Logf("  ... 共 %d 个音符", len(vNotes))
		}
	}

	if len(notes) == 0 {
		t.Error("parseABCForMIDI 应返回音符但返回了 0 个")
	}
}

// ==== 辅助函数 ====

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
