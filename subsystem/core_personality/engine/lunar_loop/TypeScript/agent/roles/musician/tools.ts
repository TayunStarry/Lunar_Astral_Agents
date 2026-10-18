import { ToolCall } from '../../../config/tool';

/**
 * 演奏者工具定义（三阶段依次提交）
 *
 *   1. submit_melody       生成完整主旋律（确立主题、调式、节拍与结构）
 *   2. submit_accompaniment 在主旋律基础上生成伴奏声部（小节数与主旋律严格对齐）
 *   3. submit_chords       最后生成和弦进行声部（低音支撑与和声骨架）
 */
export const musicTools: ToolCall[] = [
	{
		type: "function",
		function: {
			name: "submit_melody",
			description: "创作流程第一步：提交完整的主旋律声部。先规划风格、调式、节拍与段落结构，再写出贯穿全曲的主旋律。主旋律是全曲的根基，伴奏与和弦将在其基础上依次生成",
			parameters: {
				type: "object",
				properties: {
					"title": {
						type: "string",
						description: "音乐作品标题，应具有诗意与美感，与音乐风格匹配"
					},
					"instrument": {
						type: "string",
						description: "主旋律乐器，单个乐器名，如：钢琴(piano)、小提琴(violin)、长笛(flute)、萨克斯(sax)、竖琴(harp)、大提琴(cello)、8bit"
					},
					"tempo": {
						type: "number",
						description: "演奏速度（BPM）。抒情曲建议60-80，轻快曲建议90-120，激昂曲建议130-150。默认100"
					},
					"key": {
						type: "string",
						description: "调式，如 C、G、D、F、a、e、d"
					},
					"meter": {
						type: "string",
						description: "拍号，如 4/4、3/4、6/8。默认 4/4"
					},
					"structure": {
						type: "string",
						description: "段落结构，如：'前奏(4小节)-A段主旋律(8小节)-B段展开(8小节)-A'再现(8小节)-尾声(4小节)'"
					},
					"melody": {
						type: "string",
						description: "主旋律ABC片段：仅一行音符，用 | 分隔小节，以 |] 结尾。禁止包含 X:/T:/M:/L:/Q:/K: 等头部字段与 [V:N] 声部标记。应有起伏与乐句呼吸感（每4-8小节一个乐句）"
					}
				},
				required: [
					"title",
					"melody"
				]
			}
		}
	},
	{
		type: "function",
		function: {
			name: "submit_accompaniment",
			description: "创作流程第二步：在已提交的主旋律基础上生成伴奏声部。使用柱式和弦、分解和弦或阿尔贝蒂低音为旋律提供和声织体，小节数必须与主旋律完全一致",
			parameters: {
				type: "object",
				properties: {
					"instrument": {
						type: "string",
						description: "伴奏乐器，单个乐器名，如：钢琴(piano)、竖琴(harp)、吉他(guitar)、贝斯(bass)、氛围铺底(弦乐群)"
					},
					"accompaniment": {
						type: "string",
						description: "伴奏ABC片段：仅一行音符，用 | 分隔小节，小节数必须与主旋律完全一致，以 |] 结尾。禁止包含头部字段与 [V:N] 声部标记。和弦用方括号包裹同时发音的音符，如 [C,,E,,G,,]4 或分解和弦 C,,2 E,2 G,2 c2"
					}
				},
				required: [
					"accompaniment"
				]
			}
		}
	},
	{
		type: "function",
		function: {
			name: "submit_chords",
			description: "创作流程第三步（最后一步）：在主旋律与伴奏基础上生成和弦进行声部，提供低音支撑与和声骨架，完成整部作品。小节数必须与主旋律完全一致",
			parameters: {
				type: "object",
				properties: {
					"instrument": {
						type: "string",
						description: "和弦声部乐器，单个乐器名，如：贝斯(bass)、大提琴(cello)、钢琴(piano)、鼓组(打击乐)"
					},
					"chords": {
						type: "string",
						description: "和弦进行ABC片段：仅一行音符，用 | 分隔小节，小节数必须与主旋律完全一致，以 |] 结尾。禁止包含头部字段与 [V:N] 声部标记。使用根音、柱式和弦或分解和弦，明确体现和声进行（如 C-F-G-C）"
					}
				},
				required: [
					"chords"
				]
			}
		}
	}
];
