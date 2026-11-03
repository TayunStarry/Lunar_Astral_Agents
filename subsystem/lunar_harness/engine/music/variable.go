package music

// variable.go 音乐渲染链路的常量与词表参数。

import "time"

const (
	// module 音乐链路日志模块名
	module = "Music"
	// modelSwapWait 切换「主模型 ↔ 音乐模型」时等待对端释放显存的上限
	modelSwapWait = 60 * time.Second
	// modelLoadWait 音乐模型从触发加载到权重可用的等待上限
	modelLoadWait = 90 * time.Second
	// musicTokenBudget 单次演奏的 MIDI token 上限（AMT 每 3 个 token 一个音符）
	musicTokenBudget = 2046
)

const (
	// PPQ SMF 每四分音符的 tick 数
	PPQ = 480
	// defaultTempo 缺省速度
	defaultTempo = 100
	// defaultMeterNum 缺省拍号分子
	defaultMeterNum = 4
	// defaultMeterDen 缺省拍号分母
	defaultMeterDen = 4
	// defaultBars 缺省篇幅（仅用于描述与提示词，实际篇幅由模型 token 决定）
	defaultBars = 16
	// minBars 可接受的最少小节数
	minBars = 4
	// maxBars 描述中允许的最大小节数
	maxBars = 48
	// VoiceSourceModel 乐谱来自音乐模型
	VoiceSourceModel = "model"
)

// ==== AMT（Anticipatory Music Transformer）词表 ====

const (
	// amtTimeOffset 时间 token 的起始 id（10ms 为单位，绝对时间）
	amtTimeOffset = 0
	// amtDurOffset 时值 token 的起始 id（10ms 为单位）
	amtDurOffset = 10000
	// amtNoteOffset 音符 token 的起始 id（instr*128+pitch）
	amtNoteOffset = 11000
	// amtRestToken 休止 token id
	amtRestToken = 27512
	// amtMaxTime 时间 token 的上限（10000 = 100 秒）
	amtMaxTime = 10000
	// amtMaxDuration 时值 token 的上限（1000 = 10 秒）
	amtMaxDuration = 1000
	// amtDrumInstrument 鼓声部的乐器编号（对应 MIDI 通道 9）
	amtDrumInstrument = 128
	// amtNotesPerStep 每个音符占用的 token 数（时间 / 时值 / 音符）
	amtNotesPerStep = 3
	// amtMaxVoices 单个作品最多保留的声部数（MIDI 通道上限）
	amtMaxVoices = 16
	// amtPromptBOS 提示词末尾追加的自回归起始 token（GGUF 中的文本形式）
	amtPromptBOS = "<|midi_token_55026|>"
)

// gmCategoryNames GM 音色区间到中文分类名（用于展示谱的声部名）。
var gmCategoryNames = map[int]string{
	0: "钢琴", 8: "电钢琴", 16: "风琴", 24: "吉他", 32: "贝斯",
	40: "弦乐", 48: "合奏", 56: "铜管", 64: "簧管", 72: "木管",
	80: "合成器", 88: "氛围", 96: "音效", 104: "民族", 112: "打击乐",
	120: "音效",
}
