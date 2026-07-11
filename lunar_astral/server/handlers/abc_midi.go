package handlers

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ==== 内置 ABC → MIDI 转换器 ====
// 当 abc2midi 外部工具不可用时，使用 Go 原生实现
// 支持基本的 ABC 记谱法：音符、时值、升降号、八度、多声部、力度记号

const (
	midiHeaderChunk = "MThd"
	midiTrackChunk  = "MTrk"
	ticksPerQuarter = 480 // 每四分音符的 tick 数
)

// MIDI 写入器
type midiWriter struct {
	trackData []byte
	lastTick  int
}

// noteToMIDI 将 ABC 音符名转换为 MIDI 音符号
// 格式: C4, C#4, Db3 等
func noteToMIDI(note string) int {
	notes := map[byte]int{'C': 0, 'D': 2, 'E': 4, 'F': 5, 'G': 7, 'A': 9, 'B': 11}

	if len(note) == 0 {
		return 60
	}

	base := notes[note[0]]
	if base == 0 && note[0] != 'C' {
		return 60
	}

	idx := 1
	// 升降号
	sharp := 0
	if idx < len(note) {
		if note[idx] == '#' {
			sharp = 1
			idx++
		} else if note[idx] == 'b' {
			sharp = -1
			idx++
		}
	}

	// 八度
	octave := 4
	if idx < len(note) {
		if o, err := strconv.Atoi(note[idx:]); err == nil {
			octave = o
		}
	}

	return (octave+1)*12 + base + sharp
}

// variableLengthQuantity 编码 MIDI 变长整数
func variableLengthQuantity(value int) []byte {
	if value < 0 {
		value = 0
	}
	var result []byte
	buf := value & 0x7F
	for {
		result = append([]byte{byte(buf)}, result...)
		if value >>= 7; value == 0 {
			break
		}
		buf = ((value & 0x7F) | 0x80)
	}
	if len(result) == 0 {
		return []byte{0}
	}
	return result
}

// writeDeltaTime 写入 delta time
func (w *midiWriter) writeDeltaTime(tick int) {
	delta := tick - w.lastTick
	if delta < 0 {
		delta = 0
	}
	w.trackData = append(w.trackData, variableLengthQuantity(delta)...)
	w.lastTick = tick
}

// writeNoteOn 写入 Note On 事件
func (w *midiWriter) writeNoteOn(tick, channel, note, velocity int) {
	w.writeDeltaTime(tick)
	w.trackData = append(w.trackData,
		byte(0x90|channel&0x0F),
		byte(note&0x7F),
		byte(velocity&0x7F),
	)
}

// writeNoteOff 写入 Note Off 事件
func (w *midiWriter) writeNoteOff(tick, channel, note int) {
	w.writeDeltaTime(tick)
	w.trackData = append(w.trackData,
		byte(0x80|channel&0x0F),
		byte(note&0x7F),
		byte(0),
	)
}

// writeProgramChange 写入乐器切换事件
func (w *midiWriter) writeProgramChange(tick, channel, program int) {
	w.writeDeltaTime(tick)
	w.trackData = append(w.trackData,
		byte(0xC0|channel&0x0F),
		byte(program&0x7F),
	)
}

// writeTempo 写入速度事件
func (w *midiWriter) writeTempo(tick int, bpm float64) {
	microsecondsPerBeat := int(60000000.0 / bpm)
	w.writeDeltaTime(tick)
	w.trackData = append(w.trackData,
		0xFF, 0x51, 0x03,
		byte((microsecondsPerBeat>>16)&0xFF),
		byte((microsecondsPerBeat>>8)&0xFF),
		byte(microsecondsPerBeat&0xFF),
	)
}

// writeEndOfTrack 写入轨道结束标记
func (w *midiWriter) writeEndOfTrack(tick int) {
	w.writeDeltaTime(tick)
	w.trackData = append(w.trackData, 0xFF, 0x2F, 0x00)
}

// GM 乐器程序号映射（General MIDI 标准乐器编号）
var gmInstrumentMap = map[string]int{
	"piano":    0, // Acoustic Grand Piano
	"钢琴":       0,
	"violin":   40, // Violin
	"小提琴":      40,
	"flute":    73, // Flute
	"长笛":       73,
	"cello":    42, // Cello
	"大提琴":      42,
	"guitar":   24, // Acoustic Guitar (nylon)
	"吉他":       24,
	"harp":     46, // Orchestral Harp
	"竖琴":       46,
	"clarinet": 71, // Clarinet
	"单簧管":      71,
	"oboe":     68, // Oboe
	"双簧管":      68,
	"trumpet":  56, // Trumpet
	"小号":       56,
	"synth":    80, // Synth Lead (square)
	"合成器":      80,
}

// getGMProgram 根据乐器名获取 GM 程序号
func getGMProgram(instrumentName string) int {
	// 精确匹配
	if prog, ok := gmInstrumentMap[instrumentName]; ok {
		return prog
	}
	// 英文关键词匹配
	keywordMap := map[string]int{
		"piano": 0, "keyboard": 0,
		"violin": 40, "fiddle": 40,
		"flute":  73,
		"cello":  42,
		"guitar": 24, "bass": 32,
		"harp":     46,
		"clarinet": 71,
		"oboe":     68,
		"trumpet":  56,
		"synth":    80,
	}
	lower := strings.ToLower(instrumentName)
	for kw, prog := range keywordMap {
		if strings.Contains(lower, kw) {
			return prog
		}
	}
	return 0 // 默认钢琴
}

// abcNote 表示一个解析后的 ABC 音符
type abcNote struct {
	note     string  // 音符名（如 C#4）
	duration float64 // 持续时间（秒）
	time     float64 // 开始时间（秒）
	voice    int     // 声部编号（从 1 开始）
	dynamics string  // 力度记号
}

// parseABCForMIDI 解析 ABC 乐谱，提取音符和元信息
func parseABCForMIDI(abcNotation string) (notes []abcNote, bpm float64, voiceInstruments map[int]string, voicePrograms map[int]int) {
	bpm = 120
	voiceInstruments = map[int]string{1: "钢琴"}
	voicePrograms = map[int]int{1: 0}

	// 解析 BPM — Q:1/4=120 格式
	if tempoMatch := findMatch(abcNotation, `Q:\s*(\d+)/(\d+)\s*=\s*(\d+)`); tempoMatch != nil && len(tempoMatch) >= 3 {
		if beatUnit, err := strconv.Atoi(tempoMatch[0]); err == nil {
			if beats, err := strconv.Atoi(tempoMatch[1]); err == nil {
				if tempo, err := strconv.Atoi(tempoMatch[2]); err == nil {
					bpm = float64(tempo) * float64(beatUnit) / float64(beats)
				}
			}
		}
	}
	// Q:120 简单格式
	if simpleQ := findMatch(abcNotation, `Q:\s*(\d+)\s*$`); simpleQ != nil && len(simpleQ) >= 1 {
		if t, err := strconv.Atoi(simpleQ[0]); err == nil && bpm == 120 {
			bpm = float64(t)
		}
	}

	// 解析 %%prog N 程序号（GM 乐器程序号）
	for _, line := range strings.Split(abcNotation, "\n") {
		if m := findMatchString(line, `^%%prog\s+(\d+)\s+(\d+)`); m != nil && len(m) >= 2 {
			voiceID, _ := strconv.Atoi(m[0])
			prog, _ := strconv.Atoi(m[1])
			voicePrograms[voiceID] = prog
		}
	}

	// 解析 %%voice N 乐器名
	for _, line := range strings.Split(abcNotation, "\n") {
		if m := findMatchString(line, `^%%voice\s+(\d+)\s+(.+)`); m != nil && len(m) >= 2 {
			voiceID, _ := strconv.Atoi(m[0])
			voiceInstruments[voiceID] = strings.TrimSpace(m[1])
		}
	}

	// 解析 %%instrument（兼容旧版）
	instMatch := findMatch(abcNotation, `%%instrument\s+(.+)$`)
	if instMatch != nil && len(voiceInstruments) <= 1 {
		instruments := strings.Split(strings.ReplaceAll(instMatch[1], "，", ","), ",")
		for i, inst := range instruments {
			voiceInstruments[i+1] = strings.TrimSpace(inst)
		}
	}

	// 解析 L: 默认音符时值
	defaultLenNum := 1
	defaultLenDen := 8
	if lMatch := findMatch(abcNotation, `L:\s*(\d+)/(\d+)`); lMatch != nil && len(lMatch) >= 2 {
		if n, err := strconv.Atoi(lMatch[0]); err == nil {
			if d, err := strconv.Atoi(lMatch[1]); err == nil && d > 0 {
				defaultLenNum = n
				defaultLenDen = d
			}
		}
	}

	// 解析 M: 拍号（用于时值校验）
	meterNum := 4
	meterDen := 4
	if mMatch := findMatch(abcNotation, `M:\s*(\d+)/(\d+)`); mMatch != nil && len(mMatch) >= 2 {
		if n, err := strconv.Atoi(mMatch[0]); err == nil {
			if d, err := strconv.Atoi(mMatch[1]); err == nil && d > 0 {
				meterNum = n
				meterDen = d
			}
		}
	}
	_ = meterNum
	_ = meterDen

	// 提取音乐主体（K: 行之后）
	// 注意：Go regexp 默认模式下 '.' 不匹配 '\n'、'$' 只匹配文本末尾，
	// 因此 "K:.*$" 在多行文本中无法匹配。改用 "K:" 定位调号行即可。
	keyIdx := indexOfPattern(abcNotation, `K:`)
	if keyIdx < 0 {
		return
	}
	musicBody := abcNotation[keyIdx:]
	if newlineIdx := strings.Index(musicBody, "\n"); newlineIdx >= 0 {
		musicBody = musicBody[newlineIdx+1:]
	}

	// 逐行解析
	voiceTimes := map[int]float64{1: 0}
	currentVoice := 1
	currentDynamics := "mf" // 力度记号持续生效，直到遇到新的力度标记
	secondsPerBeat := 60.0 / bpm

	for _, line := range strings.Split(musicBody, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "%") {
			continue
		}

		// 先提取力度记号（如 !mp!、!f! 等，可能出现在行首）
		// 力度记号持续生效直到下一个力度记号
		for {
			if dm := findMatchString(line, `^!(pp|p|mp|mf|f|ff)!`); dm != nil && len(dm) >= 1 {
				currentDynamics = dm[0]
				prefix := "!" + dm[0] + "!"
				line = strings.TrimSpace(line[len(prefix):])
			} else {
				break
			}
		}

		// 处理 [V:N] 标记（力度记号已剥离，现在 [V:N] 可能在行首）
		if vm := findMatchString(line, `^\[V:(\d+)\]`); vm != nil && len(vm) >= 1 {
			currentVoice, _ = strconv.Atoi(vm[0])
			if _, exists := voiceTimes[currentVoice]; !exists {
				voiceTimes[currentVoice] = 0
			}
			// 去掉 [V:N] 前缀（使用正则匹配完整文本的长度）
			vPrefix := fmt.Sprintf("[V:%s]", vm[0])
			line = strings.TrimSpace(line[len(vPrefix):])
			if line == "" {
				continue
			}
		}

		// 解析和弦 [CEG] 和单音符
		// 先拆分成 token：和弦块或单音符序列
		tokens := tokenizeABCLine(line)
		for _, token := range tokens {
			if strings.HasPrefix(token, "[") && strings.Contains(token, "]") {
				// 和弦：[CEG] 或 [C,,E,,G,,]2
				chordContent := findMatchString(token, `\[([^\]]+)\]`)
				durStr := findMatchString(token, `\]([\/\d]*)`)
				if chordContent != nil {
					// 解析和弦内每个音符
					chordNotes := parseChordNotes(chordContent[0], currentVoice, currentDynamics)
					// 解析和弦时值
					chordDuration := calcDuration(durStr, defaultLenNum, defaultLenDen, secondsPerBeat)
					// 所有和弦音符同时发声，共享时值
					for i := range chordNotes {
						chordNotes[i].time = voiceTimes[currentVoice]
						chordNotes[i].duration = chordDuration
					}
					notes = append(notes, chordNotes...)
					voiceTimes[currentVoice] += chordDuration
				}
			} else {
				// 单音符序列
				noteRegex := `([\^=_]*)([a-gA-Gz])([,']*)(\d*)(\/?\d*)`
				for _, m := range findAllMatches(token, noteRegex) {
					// findAllMatches 返回 m[1:]，索引0=组1(accidental), 1=组2(noteName), ...
					accidental := m[0]
					noteName := m[1]
					octave := m[2]
					durationNum := m[3]
					durationFrac := m[4]

					duration := calcDurationFromParts(durationNum, durationFrac, defaultLenNum, defaultLenDen, secondsPerBeat)

					// 休止符
					if noteName == "z" {
						voiceTimes[currentVoice] += duration
						continue
					}

					fullNote := buildNoteName(accidental, noteName, octave)
					time := voiceTimes[currentVoice]
					notes = append(notes, abcNote{
						note:     fullNote,
						duration: duration,
						time:     time,
						voice:    currentVoice,
						dynamics: currentDynamics,
					})
					voiceTimes[currentVoice] = time + duration
				}
			}
		}
	}

	return
}

// tokenizeABCLine 将 ABC 行拆分为和弦块和单音符段
func tokenizeABCLine(line string) []string {
	var tokens []string
	i := 0
	for i < len(line) {
		// 跳过空白和分隔符
		if line[i] == ' ' || line[i] == '\t' || line[i] == '|' {
			i++
			continue
		}
		// 力度记号跳过（不影响 token 划分）
		if line[i] == '!' && i+1 < len(line) {
			end := strings.Index(line[i+1:], "!")
			if end >= 0 {
				i += end + 2
				continue
			}
		}
		// 和弦块
		if line[i] == '[' {
			// 找到对应的 ]
			depth := 1
			j := i + 1
			for j < len(line) && depth > 0 {
				if line[j] == '[' {
					depth++
				} else if line[j] == ']' {
					depth--
				}
				j++
			}
			// 包含时值后缀
			for j < len(line) && (line[j] >= '0' && line[j] <= '9' || line[j] == '/') {
				j++
			}
			tokens = append(tokens, line[i:j])
			i = j
			continue
		}
		// 单音符段（到下一个 | 或 [ 或空格为止）
		j := i + 1
		for j < len(line) && line[j] != '|' && line[j] != '[' && line[j] != ' ' && line[j] != '\t' {
			j++
		}
		if j > i {
			tokens = append(tokens, line[i:j])
		}
		i = j
	}
	return tokens
}

// parseChordNotes 解析和弦内的音符列表
func parseChordNotes(chordStr string, voice int, dynamics string) []abcNote {
	var result []abcNote
	noteRegex := `([\^=_]*)([a-gA-G])([,']*)(\d*)`
	for _, m := range findAllMatches(chordStr, noteRegex) {
		// findAllMatches 返回 m[1:]，索引0=组1(accidental), 1=组2(noteName), ...
		accidental := m[0]
		noteName := m[1]
		octave := m[2]
		fullNote := buildNoteName(accidental, noteName, octave)
		result = append(result, abcNote{
			note:     fullNote,
			duration: 0,
			time:     0,
			voice:    voice,
			dynamics: dynamics,
		})
	}
	return result
}

// buildNoteName 构建完整音符名（音名 + 升降号 + 八度）
func buildNoteName(accidental, noteName, octave string) string {
	fullNote := strings.ToUpper(string(noteName[0]))
	if accidental == "^" {
		fullNote += "#"
	} else if accidental == "_" {
		fullNote += "b"
	}

	octNum := 4
	if noteName[0] >= 'a' && noteName[0] <= 'g' {
		octNum = 5
	}
	apostrophes := 0
	commas := 0
	for _, r := range octave {
		if r == '\'' {
			apostrophes++
		} else if r == ',' {
			commas++
		}
	}
	octNum += apostrophes - commas
	fullNote += strconv.Itoa(octNum)
	return fullNote
}

// calcDuration 从时值字符串计算持续时间（秒）
func calcDuration(durStr []string, defaultNum, defaultDen int, secondsPerBeat float64) float64 {
	if durStr == nil || len(durStr) == 0 || durStr[0] == "" {
		// 默认时值 = L: 定义
		defaultDuration := float64(defaultNum) / float64(defaultDen)
		beatsPerNote := defaultDuration / 0.25 // 一拍=1/4
		return beatsPerNote * secondsPerBeat
	}
	return calcDurationFromParts("", durStr[0], defaultNum, defaultDen, secondsPerBeat)
}

// calcDurationFromParts 从时值组成部分计算持续时间（秒）
func calcDurationFromParts(numStr, fracStr string, defaultNum, defaultDen int, secondsPerBeat float64) float64 {
	defaultDuration := float64(defaultNum) / float64(defaultDen)

	// 基础时值 = L: 定义
	duration := defaultDuration

	if numStr != "" {
		if n, err := strconv.Atoi(numStr); err == nil && n > 0 {
			// C2 → 时值 = defaultDuration * 2
			duration = defaultDuration * float64(n)
		}
	}

	if fracStr != "" && strings.HasPrefix(fracStr, "/") {
		divStr := fracStr[1:]
		if divStr == "" {
			// C/ → 默认除以2
			duration = defaultDuration / 2.0
		} else if f, err := strconv.Atoi(divStr); err == nil && f > 0 {
			duration = defaultDuration / float64(f)
		}
	}

	// 转换为秒：一个四分音符 = 1拍 = secondsPerBeat
	beatsPerNote := duration / 0.25
	return beatsPerNote * secondsPerBeat
}

// convertABCToMIDI 将 ABC 文件转换为 MIDI 文件
func convertABCToMIDI(abcPath, midiPath string) error {
	abcData, err := os.ReadFile(abcPath)
	if err != nil {
		return fmt.Errorf("读取 ABC 文件失败: %w", err)
	}

	notes, bpm, voiceInstruments, voicePrograms := parseABCForMIDI(string(abcData))
	if len(notes) == 0 {
		return fmt.Errorf("ABC 乐谱中未找到有效音符")
	}

	return writeMIDIFile(midiPath, notes, bpm, voiceInstruments, voicePrograms)
}

// writeMIDIFile 生成标准 MIDI 文件
func writeMIDIFile(path string, notes []abcNote, bpm float64, voiceInstruments map[int]string, voicePrograms map[int]int) error {
	// 将音符按声部分组
	voiceNotes := map[int][]abcNote{}
	for _, n := range notes {
		voiceNotes[n.voice] = append(voiceNotes[n.voice], n)
	}

	// 计算总 tick 数
	maxTick := 0
	for _, n := range notes {
		endTick := int((n.time + n.duration) * float64(ticksPerQuarter) * bpm / 60.0)
		if endTick > maxTick {
			maxTick = endTick
		}
	}

	// 创建轨道
	w := &midiWriter{}

	// 写入速度
	w.writeTempo(0, bpm)

	// 写入每个声部
	channel := 0
	for voiceID, vNotes := range voiceNotes {
		if channel >= 16 {
			channel = 15 // 最多 16 个 MIDI 通道
		}

		// 设置乐器（优先使用 %%prog 指定的 GM 程序号）
		program := voicePrograms[voiceID]
		if program == 0 {
			instrumentName := voiceInstruments[voiceID]
			program = getGMProgram(instrumentName)
		}
		w.writeProgramChange(0, channel, program)

		// 力度映射
		velocityMap := map[string]int{
			"pp": 30, "p": 50, "mp": 65, "mf": 80, "f": 100, "ff": 120,
		}

		// 写入音符
		for _, n := range vNotes {
			midiNote := noteToMIDI(n.note)
			if midiNote < 0 || midiNote > 127 {
				continue
			}

			startTick := int(n.time * float64(ticksPerQuarter) * bpm / 60.0)
			durationTicks := int(n.duration * float64(ticksPerQuarter) * bpm / 60.0)
			if durationTicks < 1 {
				durationTicks = 1
			}
			endTick := startTick + durationTicks

			velocity := velocityMap[n.dynamics]
			if velocity == 0 {
				velocity = 80
			}

			w.writeNoteOn(startTick, channel, midiNote, velocity)
			w.writeNoteOff(endTick, channel, midiNote)
		}

		channel++
	}

	// 结束轨道
	w.writeEndOfTrack(maxTick + ticksPerQuarter)

	// 组装 MIDI 文件
	var fileData []byte

	// Header chunk: MThd
	headerChunk := []byte(midiHeaderChunk)
	headerData := []byte{
		0x00, 0x00, 0x00, 0x06, // chunk length
		0x00, 0x00, // format 0 (single track)
		0x00, 0x01, // 1 track
		byte(ticksPerQuarter >> 8), byte(ticksPerQuarter & 0xFF),
	}
	fileData = append(fileData, headerChunk...)
	fileData = append(fileData, headerData...)

	// Track chunk: MTrk
	trackChunk := []byte(midiTrackChunk)
	trackLen := len(w.trackData)
	fileData = append(fileData, trackChunk...)
	fileData = append(fileData,
		byte((trackLen>>24)&0xFF),
		byte((trackLen>>16)&0xFF),
		byte((trackLen>>8)&0xFF),
		byte(trackLen&0xFF),
	)
	fileData = append(fileData, w.trackData...)

	return os.WriteFile(path, fileData, 0644)
}

// ==== 正则辅助函数 ====

func findMatch(text, pattern string) []string {
	re := compileRegex(pattern)
	if re == nil {
		return nil
	}
	match := re.FindStringSubmatch(text)
	if len(match) > 0 {
		return match[1:]
	}
	return nil
}

func findMatchString(text, pattern string) []string {
	return findMatch(text, pattern)
}

func findAllMatches(text, pattern string) [][]string {
	re := compileRegex(pattern)
	if re == nil {
		return nil
	}
	matches := re.FindAllStringSubmatch(text, -1)
	var result [][]string
	for _, m := range matches {
		if len(m) > 1 {
			result = append(result, m[1:])
		}
	}
	return result
}

func indexOfPattern(text, pattern string) int {
	re := compileRegex(pattern)
	if re == nil {
		return -1
	}
	loc := re.FindStringIndex(text)
	if loc != nil {
		return loc[0]
	}
	return -1
}

func splitByVoice(text string) []string {
	re := compileRegex(`(?=\[V:\d+\])`)
	if re == nil {
		return []string{text}
	}
	indices := re.FindAllStringIndex(text, -1)
	if len(indices) == 0 {
		return []string{text}
	}
	var result []string
	prev := 0
	for _, loc := range indices {
		if loc[0] > prev {
			result = append(result, text[prev:loc[0]])
		}
		prev = loc[0]
	}
	if prev < len(text) {
		result = append(result, text[prev:])
	}
	return result
}
