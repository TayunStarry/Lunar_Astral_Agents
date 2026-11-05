package console

// transcript.go 实况轮次导出的兼容入口。
// 导出结构与实现位于 logbuf 子包（server / debugsnap 直接依赖子包）。

import (
	"LunarSubsystem/LunarHarness/engine/console/logbuf"
)

// TranscriptTurn 一轮模型调用的导出结构
type TranscriptTurn = logbuf.TranscriptTurn

// TranscriptMeta 导出的元信息：说明这份数据的完整性与来源
type TranscriptMeta = logbuf.TranscriptMeta

// TranscriptDump 实况导出的完整载荷
type TranscriptDump = logbuf.TranscriptDump

// BuildTranscript 汇总当前实况轮次为可序列化结构。
func BuildTranscript() TranscriptDump { return logbuf.BuildTranscript() }

// BuildTranscriptJSON 把实况导出序列化为 JSON 文本（供导出端点落盘）。
func BuildTranscriptJSON() (string, error) { return logbuf.BuildTranscriptJSON() }
