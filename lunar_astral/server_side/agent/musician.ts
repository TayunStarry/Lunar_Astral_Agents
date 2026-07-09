import { ToolCall, ToolCallItem } from '../index';
import { CreativeRoleBase } from './creative';

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

/** 音乐家角色 */
export class MusicianRole extends CreativeRoleBase<MusicPieceDetail> {
	/** 音乐创作工具定义 */
	private readonly musicTool: ToolCall[] = [
		{
			type: "function",
			function: {
				name: "compose_music",
				description: "创作音乐作品并生成ABC记谱法格式的乐谱。ABC记谱法是一种基于文本的音乐记谱格式，使用字母表示音符。请务必生成完整的、可直接播放的ABC乐谱，确保音符时值、小节线和调号正确。",
				parameters: {
					type: "object",
					properties: {
						"title": {
							type: "string",
							description: "音乐作品标题"
						},
						"instruments": {
							type: "string",
							description: "使用的乐器列表，多个乐器用逗号分隔，如'钢琴,小提琴'"
						},
						"tempo": {
							type: "number",
							description: "演奏速度（BPM），默认120"
						},
						"structure": {
							type: "string",
							description: "音乐段落结构描述，例如：'前奏(4小节)-主旋律(8小节)-副歌(8小节)-尾声(4小节)'"
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
							description: `ABC记谱法格式的完整乐谱。必须严格遵循ABC记谱法规范：

标题行格式:
X:1
T:作品标题
M:拍号
L:默认音符时值(如 1/8 表示八分音符)
Q:速度标记(如 1/4=120)
K:调号

音符与音高: 使用CDEFGAB表示音名，小写字母表示高八度，后面跟逗号表示低八度(如 C, D, E,)。升半音用^前缀(如 ^C)，降半音用_前缀(如 _B)。
时值: 数字后缀表示时值倍数，如 C2 表示两倍时值的C音，C/2 表示一半时值。
小节线: | 分隔小节，|| 表示双小节线，|] 表示结束。
休止符: z 表示休止符，时值规则同音符。

示例:
X:1
T:月光小夜曲
M:4/4
L:1/8
Q:1/4=100
K:C
C2 E2 G2 c'2 | e'2 d'2 c'2 G2 | E2 C2 D2 E2 | C8 |]`
						}
					},
					required: [
						"title",
						"abc_notation"
					]
				}
			}
		}
	]
	/** 音乐创作关键词模式 — 匹配用户明确要求创作音乐的意图 */
	private readonly musicKeywords = [
		/创作(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|乐谱|钢琴曲|古典乐|轻音乐)/,
		/生成(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|乐谱|钢琴曲|古典乐|轻音乐)/,
		/写(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|乐谱|钢琴曲|古典乐|轻音乐)/,
		/制作(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|乐谱|钢琴曲|古典乐|轻音乐)/,
		/编(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|乐谱|曲|钢琴曲|古典乐|轻音乐)/,
		/(?:帮我|给我|为我)(?:创作|生成|写|制作|编|做|弄|整)(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|乐谱|钢琴曲|古典乐|轻音乐)?/,
		/(?:做|弄|整)(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|乐谱|钢琴曲|古典乐|轻音乐)/,
		/来(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|乐谱|钢琴曲|古典乐|轻音乐)/,
		/作曲/,
		/编曲/,
		/谱写/,
		/演奏(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|钢琴曲|古典乐|轻音乐)/,
		/(?:弹|拉|吹)(?:一(?:首|段|曲))?.*(?:钢琴|小提琴|吉他|笛子|古筝|曲子|音乐|旋律)/,
	]
	/** 构造函数 */
	public constructor() {
		super(fileView('prompts/musicianRole.md')[0]);
	}
	/** 角色名称 */
	protected get roleName(): string { return '音乐家' }
	/** 检查未读消息是否匹配音乐创作关键词 */
	protected matchKeywords(texts: string[]): boolean {
		return texts.some(text => this.musicKeywords.some(keyword => keyword.test(text)));
	}
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
			console.error(`[音乐家] 工具调用参数解析失败:`, toolCall.function.arguments);
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
	/** 构建音乐作品摘要，供对话者使用 */
	protected buildSummary(pieces: MusicPieceDetail[]): string {
		const parts: string[] = [];
		parts.push('[音乐创作记录] 你（月华）刚刚完成了以下音乐作品创作：');
		for (let i = 0; i < pieces.length; i++) {
			const p = pieces[i];
			const detailLines: string[] = [];
			detailLines.push(`作品${i + 1}：《${p.title}》`);
			if (p.instruments) detailLines.push(`  - 乐器配置：${p.instruments}`);
			if (p.key) detailLines.push(`  - 调式：${p.key}${p.key === p.key.toLowerCase() ? '小调' : '大调'}`);
			if (p.tempo > 0) detailLines.push(`  - 速度：${p.tempo} BPM`);
			if (p.meter) detailLines.push(`  - 拍号：${p.meter}`);
			if (p.structure) detailLines.push(`  - 段落结构：${p.structure}`);
			detailLines.push(`  - 乐谱长度：${p.abcLength} 字符`);
			parts.push(detailLines.join('\n'));
		}
		parts.push('\n注意：请基于以上真实创作信息向用户介绍音乐作品，切勿编造不存在的曲名、乐器或结构。乐谱已通过音乐播放器推送给用户，可以引导用户查看和播放。');
		return parts.join('\n');
	}
	/**
	 * 处理音乐创作工具调用
	 *
	 * 接收 LLM 生成的 ABC 记谱法乐谱，验证格式后推送到前端进行渲染和播放。
	 * 会将 instruments 乐器参数以 ABC 标准 `%%instrument` 指令注入乐谱头部，
	 * 供前端 music_renderer.html 据此选择对应的 Tone.js 合成器音色。
	 */
	private handleComposeMusic(args: ComposeMusicParams): string {
		try {
			const title = args.title || '未命名作品';
			const abcNotation = args.abc_notation || '';
			const instruments = (args.instruments || '').trim();

			console.log(`[音乐家] 创作音乐: "${title}"`);
			if (instruments) console.log(`  乐器: ${instruments}`);
			if (args.tempo) console.log(`  速度: ${args.tempo} BPM`);
			if (args.structure) console.log(`  结构: ${args.structure}`);

			if (!abcNotation.trim()) {
				return '音乐创作失败：ABC记谱法乐谱为空';
			}

			// 注入乐器指令到 ABC 头部
			const enrichedAbc = this.injectInstrumentDirective(abcNotation, instruments);

			const hasX = /^X:\s*\d+/m.test(enrichedAbc);
			const hasT = /^T:\s*.+/m.test(enrichedAbc);
			const hasK = /^K:\s*.+/m.test(enrichedAbc);

			if (!hasX || !hasK) {
				console.warn('[音乐家] ABC乐谱缺少必要字段 (X:/K:)，尝试自动补充');
				let fixedAbc = enrichedAbc;
				if (!hasX) fixedAbc = 'X:1\n' + fixedAbc;
				if (!hasT) fixedAbc = fixedAbc.replace(/^(X:\s*\d+\n)/m, `$1T:${title}\n`);
				if (!hasK) fixedAbc = fixedAbc.replace(/^(T:.*\n)/m, `$1K:C\n`);
				const pushSuccess = pushContext('music', fixedAbc, '');
				if (!pushSuccess) {
					console.warn('[音乐家] 推送乐谱到前端失败');
				}
				return `音乐作品"${title}"创作成功（已自动补全格式）。乐谱已推送到前端进行渲染播放。`;
			}

			const pushSuccess = pushContext('music', enrichedAbc, '');
			if (!pushSuccess) {
				console.warn('[音乐家] 推送乐谱到前端失败');
			}

			console.log(`[音乐家] 乐谱推送成功，长度: ${enrichedAbc.length} 字符，乐器: ${instruments || '默认'}`);
			return `音乐作品"${title}"创作成功。乐谱已推送到前端进行渲染播放。`;
		}
		catch (error) {
			console.error('[音乐家] 音乐创作处理异常:', error);
			return `音乐创作异常: ${error}`;
		}
	}
	/**
	 * 将乐器列表以 ABC 标准 `%%instrument` 指令注入乐谱头部
	 *
	 * ABC 标准支持 `%%` 开头的指令行（stylesheet directive），
	 * 通常放在 X: 行之前或之后。前端解析时优先取第一个乐器作为主音色。
	 */
	private injectInstrumentDirective(abcNotation: string, instruments: string): string {
		if (!instruments) return abcNotation;
		const cleaned = instruments
			.replace(/，/g, ',')
			.split(',')
			.map(s => s.trim())
			.filter(Boolean)
			.join(',');
		if (!cleaned) return abcNotation;
		if (/^%%instrument/m.test(abcNotation)) return abcNotation;

		const directive = `%%instrument ${cleaned}\n`;
		const xMatch = abcNotation.match(/^X:\s*\d+/m);
		if (xMatch && xMatch.index !== undefined) {
			const before = abcNotation.substring(0, xMatch.index);
			const after = abcNotation.substring(xMatch.index);
			return before + directive + after;
		}
		return directive + abcNotation;
	}
}
