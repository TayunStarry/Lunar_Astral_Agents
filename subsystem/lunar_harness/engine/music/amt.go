package music

// amt.go MIDI-LLM 的 AMT 词表解码。
//
// MIDI-LLM 用 Anticipatory Music Transformer 词表：每个音符由三个 token 组成
// （绝对时间、时值、乐器*128+音高），时间与时值均以 10ms 为单位。
// 解码后按乐器分出声部，直接写入标准 MIDI，交给 fluidsynth 渲染。

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ComposePrompt 构造 MIDI-LLM 的完整输入提示。
// 模型要求系统描述以固定句式开头，并在末尾追加 AMT 自回归起始 token。
func ComposePrompt(description string) string {
	return "You are a world-class composer. Please compose some music according to the following description: " +
		strings.TrimSpace(description) + " " + amtPromptBOS
}

// DecodeAMT 把 AMT token 解码为按乐器分组的可演奏乐谱。
func DecodeAMT(ids []int, brief Brief) (*parsedScore, error) {
	normalized := normalizeBrief(brief)
	score := &parsedScore{
		Title:    normalized.Title,
		Tempo:    normalized.Tempo,
		MeterNum: defaultMeterNum,
		MeterDen: defaultMeterDen,
	}
	score.MeterNum, score.MeterDen = meterParts(normalized.Meter)

	groups := make(map[int][]noteEvent)
	firstTick := make(map[int]int)
	notes, _ := scanNotes(ids)
	for _, parsed := range notes {
		duration := parsed.Duration
		if duration < 1 {
			duration = 1
		}
		instrument := parsed.Instrument
		tick := parsed.Time * normalized.Tempo * PPQ / 6000
		ticks := duration * normalized.Tempo * PPQ / 6000
		if ticks < 1 {
			ticks = 1
		}
		velocity := 88
		if instrument == amtDrumInstrument {
			velocity = 80
		}
		groups[instrument] = append(groups[instrument], noteEvent{
			Tick: tick, Duration: ticks, Note: parsed.Pitch, Velocity: velocity,
		})
		if _, ok := firstTick[instrument]; !ok {
			firstTick[instrument] = tick
		}
	}
	if len(groups) == 0 {
		return nil, fmt.Errorf("乐谱中没有任何可演奏音符")
	}

	instruments := make([]int, 0, len(groups))
	for instrument := range groups {
		instruments = append(instruments, instrument)
	}
	sort.Slice(instruments, func(i, j int) bool {
		if firstTick[instruments[i]] != firstTick[instruments[j]] {
			return firstTick[instruments[i]] < firstTick[instruments[j]]
		}
		return instruments[i] < instruments[j]
	})
	if len(instruments) > amtMaxVoices {
		instruments = instruments[:amtMaxVoices]
	}

	channel := 0
	for _, instrument := range instruments {
		notes := groups[instrument]
		sort.Slice(notes, func(i, j int) bool {
			if notes[i].Tick != notes[j].Tick {
				return notes[i].Tick < notes[j].Tick
			}
			return notes[i].Note < notes[j].Note
		})
		voice := midiVoice{
			Name:    gmVoiceName(instrument),
			Program: instrument,
			Notes:   notes,
		}
		if instrument == amtDrumInstrument {
			// GM 鼓组固定使用通道 9
			voice.Channel = 9
			voice.Program = 0
		} else {
			if channel == 9 {
				channel++
			}
			if channel > 15 {
				channel = 15
			}
			voice.Channel = channel
			channel++
		}
		score.Voices = append(score.Voices, voice)
	}
	return score, nil
}

// normalizeBrief 补齐音乐描述的缺省值并夹紧到安全范围。
func normalizeBrief(brief Brief) Brief {
	if strings.TrimSpace(brief.Title) == "" {
		brief.Title = "无题"
	}
	if brief.Tempo <= 0 {
		brief.Tempo = defaultTempo
	}
	if brief.Tempo > 220 {
		brief.Tempo = 220
	}
	if brief.Bars <= 0 {
		brief.Bars = defaultBars
	}
	if brief.Bars < minBars {
		brief.Bars = minBars
	}
	if brief.Bars > maxBars {
		brief.Bars = maxBars
	}
	if brief.Meter == "" || !strings.Contains(brief.Meter, "/") {
		brief.Meter = "4/4"
	}
	if strings.TrimSpace(brief.Key) == "" {
		brief.Key = "C"
	}
	if len(brief.Instruments) == 0 {
		brief.Instruments = []string{"钢琴"}
	}
	if strings.TrimSpace(brief.Prompt) == "" {
		brief.Prompt = BriefDescription(brief)
	}
	return brief
}

// BriefDescription 返回交给 MIDI-LLM 的英文描述：优先用主模型给出的 prompt，
// 缺失时由结构化字段拼接（异常保护，不作为常规路径）。
func BriefDescription(brief Brief) string {
	if value := strings.TrimSpace(brief.Prompt); value != "" {
		return clampDescription(value)
	}
	style := strings.TrimSpace(brief.Style)
	if style == "" {
		style = "instrumental"
	}
	mood := strings.TrimSpace(brief.Mood)
	if mood == "" {
		mood = "expressive"
	}
	return clampDescription(fmt.Sprintf("A %s %s piece for %s, about %d bars, %d BPM, %s meter.",
		style, mood, strings.Join(brief.Instruments, " and "), brief.Bars, brief.Tempo, brief.Meter))
}

// clampDescription 把模型给出的描述压成单行并限制长度，避免换行或超长说明混入模型输入。
func clampDescription(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if runes := []rune(value); len(runes) > descriptionLimit {
		value = string(runes[:descriptionLimit])
	}
	return value
}

// meterParts 解析拍号，缺失时回退 4/4。
func meterParts(meter string) (int, int) {
	parts := strings.SplitN(strings.TrimSpace(meter), "/", 2)
	if len(parts) != 2 {
		return defaultMeterNum, defaultMeterDen
	}
	num, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	den, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || num <= 0 || den <= 0 {
		return defaultMeterNum, defaultMeterDen
	}
	return num, den
}

// gmVoiceName 按 GM 音色区间给出中文声部名。
func gmVoiceName(program int) string {
	if program == amtDrumInstrument {
		return "鼓组"
	}
	for start := 120; start >= 0; start -= 8 {
		if program >= start {
			if name, ok := gmCategoryNames[start]; ok {
				return name
			}
		}
	}
	return "音色" + strconv.Itoa(program)
}

// ScoreFingerprint 乐谱指纹（用于 WAV 文件名去重）。
func ScoreFingerprint(score *parsedScore) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "%d|%d|%d|%d|", score.Tempo, score.MeterNum, score.MeterDen, len(score.Voices))
	for _, voice := range score.Voices {
		fmt.Fprintf(&builder, "%d:%d:%d;", voice.Channel, voice.Program, len(voice.Notes))
		for _, note := range voice.Notes {
			fmt.Fprintf(&builder, "%d,%d,%d,", note.Tick, note.Duration, note.Note)
		}
	}
	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:])[:16]
}
