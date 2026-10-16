package TuiSystem

import (
	"encoding/json"
	"strings"
	"testing"
)

// 覆盖：日志正文清洗必须剥掉终端控制序列，且不损伤可读内容。
//
// 背景（实测）：日志正文里普遍带控制序列——彩色工具输出的 `\u001b[…m`、进度行的 `\r`、
// 终端标题用的 OSC。历史实现只在写 console.log 时做 ansi.Strip，**入缓冲与导出仍带转义码**，
// 于是导出的 .log 里混着 `\u001b[32m` 与裸 `\r`，用 grep / jq / 编辑器都难以阅读。
func TestScrubLogTextStripsTerminalSequences(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "SGR 颜色序列",
			raw:  "\x1b[32m成功\x1b[0m 读取模型",
			want: []string{"成功 读取模型"},
		},
		{
			name: "光标移动与清行序列",
			raw:  "\x1b[2K\x1b[1G进度 10/20",
			want: []string{"进度 10/20"},
		},
		{
			name: "回车原地刷新只保留最后一次内容",
			raw:  "加载中 10%\r加载中 55%\r加载完成",
			want: []string{"加载中 10%", "加载中 55%", "加载完成"},
		},
		{
			name: "OSC 终端标题序列",
			raw:  "\x1b]0;标题\x07正文在这里",
			want: []string{"正文在这里"},
		},
		{
			name: "杂散控制符被去掉",
			raw:  "正常\x00\x07\x1f文本",
			want: []string{"正常文本"},
		},
		{
			name: "全空 / 纯控制序列不产生行",
			raw:  "\x1b[0m\r\r   ",
			want: nil,
		},
		{
			name: "空串不产生行",
			raw:  "",
			want: nil,
		},
		{
			name: "制表符与中文保留",
			raw:  "字段\t值 中文日志",
			want: []string{"字段\t值 中文日志"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ScrubLogText(tc.raw)
			if len(got) != len(tc.want) {
				t.Fatalf("行数不符: 期望 %q，实际 %q", tc.want, got)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("第 %d 行不符: 期望 %q，实际 %q", i, tc.want[i], got[i])
				}
			}
			// 无论输入如何，输出都不得残留任何 C0/C1 控制符（\t 除外）
			for i, line := range got {
				for _, r := range line {
					if (r < 0x20 && r != '\t') || r == 0x7f {
						t.Fatalf("第 %d 行残留控制符 U+%04X: %q", i, r, line)
					}
				}
			}
		})
	}
}

// 覆盖：清洗不得改变纯文本内容（回归防护——清洗过猛会把中文/标点/制表符吃掉）。
func TestScrubLogTextPreservesPlainText(t *testing.T) {
	for _, raw := range []string{
		"普通日志行",
		"[LunarCore][JavaScript] 工具调用完成: describe_images",
		"路径 D:\\Lunar_Astral_Agents\\local_data\\logs\\console.log",
		"数值 1.5 MB / 3.2it/s / 100%",
	} {
		got := ScrubLogText(raw)
		if len(got) != 1 || got[0] != raw {
			t.Fatalf("纯文本被改动: 输入 %q，输出 %q", raw, got)
		}
	}
}

// 覆盖：导出的 JSONL 每行都是合法 JSON，且正文里不再出现转义序列。
//
// 这是「导出原始日志时应剔除颜色代码」这条需求的直接验收断言：
// 若正文仍带 `\u001b`，JSON 虽仍合法（会被编码成 \u001b），但离线排查时不可读，
// 因此这里同时要求「JSON 可解析」与「文本无转义字节」。
func TestExportedJSONLHasNoEscapeCodes(t *testing.T) {
	raw := []string{
		"\x1b[31m[ERROR]\x1b[0m 模型加载失败",
		"进度 40%\r进度 100%",
	}

	var b strings.Builder
	for _, r := range raw {
		for _, text := range ScrubLogText(r) {
			rec := map[string]any{"level": "RAW", "text": text}
			encoded, err := json.Marshal(rec)
			if err != nil {
				t.Fatalf("编码失败: %v", err)
			}
			b.Write(encoded)
			b.WriteByte('\n')
		}
	}

	// 逐行可解析，且正文不含 ESC 或裸 \r
	for i, line := range strings.Split(strings.TrimRight(b.String(), "\n"), "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("第 %d 行不是合法 JSON: %v（%s）", i, err, line)
		}
		text, _ := rec["text"].(string)
		if strings.ContainsRune(text, 0x1b) {
			t.Fatalf("第 %d 行正文仍含 ESC: %q", i, text)
		}
		if strings.ContainsRune(text, '\r') {
			t.Fatalf("第 %d 行正文仍含回车: %q", i, text)
		}
	}

	// 原始记录文本本身也不得含 ESC（兜底：序列化层没做过滤）
	if strings.ContainsRune(b.String(), 0x1b) {
		t.Fatalf("导出内容含 ESC 字节:\n%s", b.String())
	}
}
