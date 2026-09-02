import { ToolCall, ToolCallItem, CreativeRoleBase } from '../../index';

/**
 * 音乐创作参数接口
 *
 * 用于 LLM 调用 compose_music 工具时传递音乐创作所需的完整参数。
 */
export interface ComposeMusicParams {
	/** 音乐作品标题 */
	title: string;
	/** 使用的乐器列表，多个乐器用逗号分隔 */
	instruments?: string;
	/** 演奏速度（BPM） */
	tempo?: number;
	/** 音乐段落结构描述 */
	structure?: string;
	/** 调式，如 C大调、a小调 */
	key?: string;
	/** 拍号，如 4/4、3/4 */
	meter?: string;
	/** ABC记谱法格式的完整乐谱 */
	abc_notation: string;
}

/** 音乐创作详情记录（用于向对话者传递作品信息） */
interface MusicPieceDetail {
	title: string;
	instruments: string;
	tempo: number;
	structure: string;
	key: string;
	meter: string;
	abcLength: number;
}

/** 演奏者角色 */
export class MusicianRole extends CreativeRoleBase<MusicPieceDetail> {
	/** 音乐创作允许更多思考轮次，确保乐谱完整性与表现力 */
	protected MAX_ITERATIONS = 5;
	/** 音乐创作工具定义 */
	private readonly musicTool: ToolCall[] = [
		{
			type: "function",
			function: {
				name: "compose_music",
				description: "创作音乐作品并生成ABC记谱法格式的乐谱，前端音乐播放器使用采样级音色库（真实乐器合成+LOFI效果链）播放。必须生成完整、可直接播放的ABC乐谱，包含和弦伴奏与多声部编配。",
				parameters: {
					type: "object",
					properties: {
					"title": {
						type: "string",
						description: "音乐作品标题"
					},
					"instruments": {
						type: "string",
						description: "使用的乐器列表，多个乐器用逗号分隔。优先使用琴类乐器：钢琴(piano)、复古电钢琴、竖琴(harp)、吉他(guitar)、大提琴(cello)、小提琴(violin)。也可以使用长笛(flute)、单簧管(clarinet)、双簧管(oboe)、小号(trumpet)、萨克斯(sax)、贝斯(bass/低音提琴)、合成器(8bit/电子)、鼓组(打击乐)、氛围铺底(弦乐群)。推荐组合：'钢琴'独奏、'钢琴,大提琴'二重奏、'竖琴,小提琴'、'钢琴,贝斯,鼓组'三重奏等。"
					},
					"tempo": {
						type: "number",
						description: "演奏速度（BPM）。抒情曲建议60-80，轻快曲建议90-120，激昂曲建议130-150。默认100"
					},
					"structure": {
						type: "string",
						description: "音乐段落结构，如：'前奏(4小节)-A段主旋律(8小节)-B段展开(8小节)-A'再现(8小节)-尾声(4小节)'"
					},
					"key": {
						type: "string",
						description: "调式，如 C、G、D、F、a、e、d"
					},
					"meter": {
						type: "string",
						description: "拍号，如 4/4、3/4、6/8"
					},
					"abc_notation": {
						type: "string",
						description: `ABC记谱法格式的完整乐谱。前端音乐播放器使用采样级音色库（温暖钢琴/复古电钢/清澈竖琴/尼龙吉他/大提琴/小提琴/长笛/单簧管/双簧管/小号/萨克斯/贝斯/8Bit/鼓组/氛围铺底）合成并经过LOFI混音效果链（混响/延迟/磁带饱和/压缩）处理。

=== 基础格式 ===
X:1
T:作品标题
M:拍号
L:默认音符时值(如 1/8)
Q:速度标记(如 1/4=100)
K:调号

=== 音符规则 ===
音名: C D E F G A B（大写=中低音区, 小写cdefgab=高八度, 加逗号=低八度如C,D,）
升降号: ^升半音(如^C)  _降半音(如_B)
时值: 数字后缀=倍数(C2=两倍)  /数字=分数(C/2=一半)
小节线: | 分隔  || 双线  |] 结束
休止符: z

=== 和弦伴奏（核心要求！必须包含！） ===
和弦用方括号包裹同时发音的音符，如 [CEG] 表示C大三和弦同时演奏。
和弦必须贯穿全曲，形成完整的伴奏织体：

1. 柱式和弦: [C,,E,,G,,]2 [C,,E,,G,,]2 | [F,,A,,C,]2 [G,,B,,D,]2 |
2. 分解和弦(琶音): C,,2 E,2 G,2 c2 | F,,2 A,2 C2 f2 |
3. 阿尔贝蒂低音: C,2 G,2 E,2 G,2 | F,2 C2 A,2 C2 |

=== 多声部记谱（推荐！多乐器时让每个乐器对应一个声部） ===
[V:1] = 旋律声部（主旋律乐器，如钢琴/小提琴/长笛/萨克斯）
[V:2] = 和弦伴奏声部（钢琴/竖琴/吉他，用柱式或分解和弦）
[V:3] = 低音声部（贝斯/大提琴，根音支撑，可选）
[V:4] = 鼓组声部（鼓/打击乐，节奏骨架，可选）
各声部小节对齐、同步演奏。声部越多，音乐层次越丰满。

=== 表情记号（使音乐富有表现力！） ===
力度: !pp!极弱 !p!弱 !mp!中弱 !mf!中强 !f!强 !ff!极强
运音法: .断奏 >重音 -保持

=== 完整示例：钢琴独奏（含和弦伴奏） ===
X:1
T:晨光曲
M:4/4
L:1/8
Q:1/4=90
K:C
!mp! [V:1] c2 e2 g2 e2 | f2 a2 g2 e2 | d2 f2 e2 d2 | c4 z4 |
!mf! [V:2] [C,,E,,G,,]4 | [F,,A,,C,]4 | [G,,B,,D,]4 | [C,,E,,G,,]4 |

=== 完整示例：钢琴+大提琴二重奏 ===
X:1
T:夜色温柔
M:4/4
L:1/8
Q:1/4=80
K:Am
!mp! [V:1] e2 a2 c'2 a2 | d2 f2 e2 d2 | c2 e2 d2 ^c2 | A4 z4 |
!p!   [V:2] [A,,2E,2A,2]2 | [D,,2A,,2D,2]2 | [E,,2B,,2E,2]2 | [A,,,2E,,2A,,2]2 |

关键原则:
- 必须包含和弦伴奏，不可只有单音旋律线
- 两个及以上乐器时，务必用 [V:N] 分为多个声部，各声部小节对齐、同步演奏
- 左手/第二声部使用和弦或分解和弦提供和声支撑
- 可选加入贝斯（低音根音）与鼓组（节奏骨架），让音乐更有层次
- 合理使用力度变化（开头mp、高潮f、结尾p）
- 旋律要有乐句呼吸感（每4-8小节一个乐句，句末用稍长时值或休止）`
					},
					},
					required: [
						"title",
						"abc_notation"
					]
				}
			}
		}
	]
	/** 构造函数 */
	public constructor() {
		super(fileView('prompts/musicianRole.md')[0]);
	}
	/** 角色名称 */
	protected get roleName(): string { return '演奏者' }
	/** 获取工具定义 */
	protected getToolDefinitions(): ToolCall[] { return this.musicTool }
	/** 执行音乐创作工具调用 */
	protected executeTool(toolCall: ToolCallItem): string {
		const funcName = toolCall.function.name;
		let args: Record<string, any> = {};
		try {
			args = typeof toolCall.function.arguments === 'string'
				? JSON.parse(toolCall.function.arguments)
				: toolCall.function.arguments;
		}
		catch (parseError) {
			console.error(`[演奏者] 工具调用参数解析失败:`, toolCall.function.arguments);
			return `工具调用参数解析失败，请确保传入合法的 JSON 字符串。错误: ${parseError}`;
		}
		switch (funcName) {
			case 'compose_music': return this.handleComposeMusic(args as ComposeMusicParams);
			default: return `未知工具: ${funcName}，可用工具为 compose_music`;
		}
	}
	/** 从工具调用中提取音乐作品详情 */
	protected collectDetail(toolCall: ToolCallItem, pieces: MusicPieceDetail[]): void {
		try {
			const args = typeof toolCall.function.arguments === 'string'
				? JSON.parse(toolCall.function.arguments)
				: toolCall.function.arguments;
			if (args.title) {
				pieces.push({
					title: args.title,
					instruments: args.instruments || '',
					tempo: args.tempo || 0,
					structure: args.structure || '',
					key: args.key || '',
					meter: args.meter || '',
					abcLength: (args.abc_notation || '').length,
				});
			}
		} catch {
			// 解析失败时跳过，不阻断流程
		}
	}
	/** 构建音乐作品摘要，使用月华话术格式 */
	protected buildSummary(pieces: MusicPieceDetail[]): string {
		if (pieces.length === 0) return '月华没有演奏任何作品';
		const parts: string[] = [];
		for (let i = 0; i < pieces.length; i++) {
			const p = pieces[i];
			let desc = `月华演奏了《${p.title}》`;
			const details: string[] = [];
			if (p.instruments) details.push(`使用${p.instruments}`);
			if (p.key) details.push(`${p.key}${p.key === p.key.toLowerCase() ? '小调' : '大调'}`);
			if (p.tempo > 0) details.push(`${p.tempo}BPM`);
			if (p.structure) details.push(`结构为${p.structure}`);
			if (details.length > 0) desc += `（${details.join('，')}）`;
			parts.push(desc + '。');
		}
		parts.push('乐谱已通过音乐播放器推送给用户，可以查看和播放。');
		return parts.join('\n');
	}
	/**
	 * 处理音乐创作工具调用
	 *
	 * 接收 LLM 生成的 ABC 记谱法乐谱，注入乐器指令后：
	 * 将 ABC 乐谱推送到前端，由音乐播放器进行乐谱可视化与 Tone.js 合成播放
	 */
	private handleComposeMusic(args: ComposeMusicParams): string {
		try {
			const title = args.title || '未命名作品';
			const abcNotation = args.abc_notation || '';
			const instruments = (args.instruments || '').trim();

			console.log(`[演奏者] 创作音乐: "${title}"`);
			if (instruments) console.log(`  乐器: ${instruments}`);
			if (args.tempo) console.log(`  速度: ${args.tempo} BPM`);
			if (args.structure) console.log(`  结构: ${args.structure}`);

			if (!abcNotation.trim()) {
				return '音乐创作失败：ABC记谱法乐谱为空';
			}

			// 注入乐器指令到 ABC 头部
			let enrichedAbc = this.injectInstrumentDirective(abcNotation, instruments);

			const hasX = /^X:\s*\d+/m.test(enrichedAbc);
			const hasT = /^T:\s*.+/m.test(enrichedAbc);
			const hasK = /^K:\s*.+/m.test(enrichedAbc);

			if (!hasX || !hasK) {
				console.warn('[演奏者] ABC乐谱缺少必要字段 (X:/K:)，尝试自动补充');
				if (!hasX) enrichedAbc = 'X:1\n' + enrichedAbc;
				if (!hasT) enrichedAbc = enrichedAbc.replace(/^(X:\s*\d+\n)/m, `$1T:${title}\n`);
				if (!hasK) enrichedAbc = enrichedAbc.replace(/^(T:.*\n)/m, `$1K:C\n`);
			}

			// 第一步：推送 ABC 乐谱到前端（用于乐谱可视化展示）
			const pushSuccess = pushContext('music', enrichedAbc, '');
			if (!pushSuccess) {
				console.warn('[演奏者] 推送乐谱到前端失败');
			}

			console.log(`[演奏者] 乐谱推送成功，长度: ${enrichedAbc.length} 字符，乐器: ${instruments || '默认'}`);
			return `音乐作品"${title}"创作成功。乐谱已推送到前端展示，可通过音乐播放器查看和播放。`;
		}
		catch (error) {
			console.error('[演奏者] 音乐创作处理异常:', error);
			return `音乐创作异常: ${error}`;
		}
	}
	/**
	 * 将乐器列表以 `%%voice N` 指令注入乐谱头部
	 *
	 * 每个乐器对应一个声部（voice），前端 music_renderer.html 会据此为每个声部
	 * 创建独立的 Tone.js 合成器，实现真正的多乐器并行演奏。
	 *
	 * 指令格式：`%%voice 1 钢琴`、`%%voice 2 小提琴` ...
	 */
	private injectInstrumentDirective(abcNotation: string, instruments: string): string {
		if (!instruments) return abcNotation;
		const list = instruments
			.replace(/，/g, ',')
			.split(',')
			.map(s => s.trim())
			.filter(Boolean);
		if (list.length === 0) return abcNotation;
		// 已包含 %%voice 指令则不重复注入
		if (/^%%voice\s+/m.test(abcNotation)) return abcNotation;

		// 构建 %%voice N 乐器名 指令行
		const directives: string[] = [];
		for (let i = 0; i < list.length; i++) {
			const inst = list[i];
			const voiceNum = i + 1;
			directives.push(`%%voice ${voiceNum} ${inst}`);
		}
		const directive = directives.join('\n') + '\n';

		const xMatch = abcNotation.match(/^X:\s*\d+/m);
		if (xMatch && xMatch.index !== undefined) {
			const before = abcNotation.substring(0, xMatch.index);
			const after = abcNotation.substring(xMatch.index);
			return before + directive + after;
		}
		return directive + abcNotation;
	}
}