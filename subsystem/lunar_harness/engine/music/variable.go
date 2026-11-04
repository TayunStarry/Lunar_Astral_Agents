package music

// variable.go 音乐渲染链路的常量与词表参数。

const (
	// module 音乐链路日志模块名
	module = "Music"
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

// ==== 乐谱体检阈值（可枚举、可测试） ====

const (
	// reviewMinNotes 通过检查的最少音符数（16 小节作品的最低骨架）
	reviewMinNotes = 48
	// reviewMaxMalformedRatio 允许的无法解析事件占比
	reviewMaxMalformedRatio = 0.1
	// reviewMaxUnorderedRatio 允许的乱序起始时间占比
	reviewMaxUnorderedRatio = 0.02
	// reviewMaxOverlap 任一时刻允许同时发声的最大音符数（超过即为稠密音墙）
	reviewMaxOverlap = 24
	// reviewMinDistinctPitches 最少音高种类（避免单调单音）
	reviewMinDistinctPitches = 6
	// reviewMaxNotesPerSecond 音符密度上限（超过视为噪声化织体）
	reviewMaxNotesPerSecond = 24
	// reviewMinNotesPerSecond 音符密度下限（低于视为空洞）
	reviewMinNotesPerSecond = 1.0
	// reviewSpanShortRatio 实际时长低于期望时长的该比例即判定过短
	reviewSpanShortRatio = 0.45
	// reviewSpanLongRatio 实际时长高于期望时长的该比例即判定过长
	reviewSpanLongRatio = 2.0
	// reviewDiagnosisLimit 重写提示词的最大字符数
	reviewDiagnosisLimit = 512
)

// 乐谱体检问题码（可枚举）。
const (
	// issueNoNotes 没有可解析的音符
	issueNoNotes = "no_notes"
	// issueMalformed 无法解析的三连音事件过多
	issueMalformed = "malformed_triples"
	// issueUnordered 起始时间明显乱序（token 流错位）
	issueUnordered = "unordered_times"
	// issueTooShort 作品明显短于要求篇幅
	issueTooShort = "too_short"
	// issueTooLong 作品明显长于要求篇幅
	issueTooLong = "too_long"
	// issueDense 同时发声音符过多（稠密音墙）
	issueDense = "dense_texture"
	// issueNarrowPitch 音高种类过少
	issueNarrowPitch = "narrow_pitch"
	// issueTooDense 音符密度过高
	issueTooDense = "too_dense"
	// issueTooSparse 音符密度过低
	issueSparse = "too_sparse"
	// issueAudioSilent 音频近乎无声或时长过短
	issueAudioSilent = "audio_silent"
	// issueAudioClipped 音频削波比例过高
	issueAudioClipped = "audio_clipped"
)

// 渲染端常量（音量、混响、归一化）。
const (
	// renderFileNameVersion 渲染产物版本前缀：旧版未归一化音频不再复用
	renderFileNameVersion = "v2-"
	// renderGain fluidsynth 合成增益（低于默认 1.0，避免 16bit 写出即削波）
	renderGain = "0.5"
	// renderSampleRate 渲染采样率
	renderSampleRate = "44100"
	// renderReverbLevel 混响强度（0-1，降低以减轻糊音）
	renderReverbLevel = "0.6"
	// renderReverbRoom 混响房间大小（0-1）
	renderReverbRoom = "0.3"
	// renderTargetRMS 归一化目标 RMS（约 -17.7 dBFS）
	renderTargetRMS = 0.13
	// renderPeakCeiling 归一化后的峰值上限（留出余量避免削波）
	renderPeakCeiling = 0.95
	// audioSilenceTopDB 裁剪首尾静音的相对阈值（dB）
	audioSilenceTopDB = 30.0
	// audioMinSeconds 可听音频的最短时长（秒）
	audioMinSeconds = 0.2
	// audioMinRMS 可听音频的最低 RMS（0-1 满量程）
	audioMinRMS = 0.02
	// audioMaxClipRatio 允许的削波采样占比
	audioMaxClipRatio = 0.005
	// pcmFullScale PCM16 的满量程基准（归一化计算用）
	pcmFullScale = 32768.0
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
	// descriptionLimit 交给音乐模型的英文描述最大字符数（超出截断，防止说明性长文污染提示）
	descriptionLimit = 512
)

// gmCategoryNames GM 音色区间到中文分类名（用于展示谱的声部名）。
var gmCategoryNames = map[int]string{
	0: "钢琴", 8: "电钢琴", 16: "风琴", 24: "吉他", 32: "贝斯",
	40: "弦乐", 48: "合奏", 56: "铜管", 64: "簧管", 72: "木管",
	80: "合成器", 88: "氛围", 96: "音效", 104: "民族", 112: "打击乐",
	120: "音效",
}
