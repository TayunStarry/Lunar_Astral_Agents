package console

// ==== 原始日志导出（数据导出端点与开发者调试快照共用） ====

import (
	"bytes"
	"encoding/json"
)

// BuildRawLogsJSONL 把「原始日志」缓冲序列化为 JSON Lines，返回文本与记录条数。
//
// 用 JSONL 而非单个 JSON 数组：日志是流式数据，逐行 JSON 可被 jq/grep 直接处理，
// 也便于只看末尾若干行（tail）而不必解析整个文件。
//
// 两个「为可读性服务」的取舍（与 server/data_dump.go 的导出端点一致）：
//  1. 正文一律经 ScrubLogText 再写出：导出给的是离线排查用的文本，终端控制序列
//     （`\u001b[…`、进度行的 `\r`）只会干扰阅读与 grep；
//  2. 关闭 encoding/json 的 HTML 转义：普通日志里 `->`、`=>`、`&&` 极常见，
//     转义后整份文件几乎没法用肉眼扫读。
func BuildRawLogsJSONL() (string, int) {
	lines := GetLogs(SourceRaw)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	/** 实际写出的记录数（清洗后为空的整行会被丢弃，不再等于 lines 长度） */
	written := 0
	for _, line := range lines {
		for _, text := range ScrubLogText(line.Text) {
			rec := map[string]any{
				"time":   line.Time.Format("2006-01-02 15:04:05.000"),
				"level":  line.Level,
				"source": line.Source,
				"module": line.Module,
				"sub":    line.Sub,
				"text":   text,
			}
			if err := enc.Encode(rec); err != nil {
				continue
			}
			written++
		}
	}
	return buf.String(), written
}
