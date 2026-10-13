import { ToolCall } from '../../config/tool';
import { ToolCallItem } from '../../config/model';
import { CreativeRoleBase } from '../base/creative';
import { interactEvent } from '../capabilities/ltp-event';

/** 音乐创作详情记录（用于向对话者传递作品信息） */
interface MusicPieceDetail {
	/** 音乐作品标题 */
	title: string;
	/** 使用的乐器列表，多个乐器用逗号分隔 */
	instruments: string;
	/** 演奏速度（BPM） */
	tempo: number;
	/** 音乐段落结构描述 */
	structure: string;
	/** 调式，如 C大调、a小调 */
	key: string;
	/** 拍号，如 4/4、3/4 */
	meter: string;
	/** ABC记谱法格式的完整乐谱长度（小节） */
	abcLength: number;
}

/** 分阶段创作草稿：主旋律 → 伴奏 → 和弦进行 */
interface MusicDraft {
	/** 作品标题 */
	title: string;
	/** 段落结构描述 */
	structure: string;
	/** 调式，如 C、a */
	key: string;
	/** 拍号，如 4/4 */
	meter: string;
	/** 速度（BPM） */
	tempo: number;
	/** 主旋律声部（ABC 片段） */
	melody: string;
	/** 主旋律乐器 */
	melodyInstrument: string;
	/** 主旋律小节数 */
	melodyBars: number;
	/** 伴奏声部（ABC 片段） */
	accompaniment: string;
	/** 伴奏乐器 */
	accompanimentInstrument: string;
	/** 和弦进行声部（ABC 片段） */
	chords: string;
	/** 和弦声部乐器 */
	chordInstrument: string;
}

/**
 * 演奏者角色
 *
 * 创作流程分三个阶段依次提交：
 *   1. submit_melody       生成完整主旋律（确立主题、调式、节拍与结构）
 *   2. submit_accompaniment 在主旋律基础上生成伴奏声部（小节数与主旋律严格对齐）
 *   3. submit_chords       最后生成和弦进行声部（低音支撑与和声骨架）
 *
 * 三个声部由服务端校验小节数后自动拼装为多声部乐谱并推送前端。
 */
export class MusicianRole extends CreativeRoleBase<MusicPieceDetail> {
	/** 分阶段创作每阶段一次成功提交即可完成，预留一轮修正机会 */
	protected MAX_ITERATIONS = 4;
	/** 主旋律允许的最少小节数（保障音乐展开充分） */
	private readonly MIN_MELODY_BARS = 12;
	/** 默认速度（BPM） */
	private readonly DEFAULT_TEMPO = 100;
	/** 分阶段创作草稿 */
	private draft: MusicDraft | null = null;
	/** 拼装完成的作品详情（在 collectDetail 中取走） */
	private assembledDetail: MusicPieceDetail | null = null;
	/** 音乐创作工具定义 */
	private readonly musicTool: ToolCall[] = [
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
		// 作品已完成时拦截重复提交
		if (this.draft?.chords) return '作品已创作完成，无需重复提交任何声部';
		switch (funcName) {
			case 'submit_melody': return this.handleSubmitMelody(args);
			case 'submit_accompaniment': return this.handleSubmitAccompaniment(args);
			case 'submit_chords': return this.handleSubmitChords(args);
			default: return `未知工具: ${funcName}，请按流程依次调用 submit_melody → submit_accompaniment → submit_chords`;
		}
	}
	/** 从工具调用中提取音乐作品详情（仅在三阶段全部完成时产出） */
	protected collectDetail(toolCall: ToolCallItem, pieces: MusicPieceDetail[]): void {
		if (toolCall.function.name !== 'submit_chords' || !this.assembledDetail) return;
		pieces.push(this.assembledDetail);
		this.assembledDetail = null;
	}
	/** 构建音乐作品摘要，使用月华话术格式 */
	protected buildSummary(pieces: MusicPieceDetail[]): string {
		// 检查是否有作品
		if (pieces.length === 0) return '月华没有演奏任何作品';
		// 事件 -> 演奏音乐前：推送作品详情，插件可改写后再汇总
		const feedback: MusicPieceDetail[] = interactEvent('play_music_before', { pieces }).return;
		// 插件可改写作品详情，如添加乐器、速度、结构等
		if (feedback && Array.isArray(feedback) && feedback.every(p => p.title && p.instruments && p.tempo && p.structure && p.key && p.meter && p.abcLength !== undefined)) {
			pieces = feedback;
		}
		/** 汇总音乐作品详情 */
		const parts: string[] = [];
		// 汇总每个作品的详情
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
	/** 清理声部片段：剥离头部字段、[V:N] 声部标记与多余空白，合并为单行 */
	private sanitizeFragment(fragment: string): string {
		return fragment
			.replace(/^[A-Za-z]:.*$/gm, '')
			.replace(/\[[Vv]:\d+\]/g, '')
			.replace(/\s+/g, ' ')
			.trim();
	}
	/** 统计声部片段的小节数（忽略头部字段、声部标记与力度记号） */
	private countBars(fragment: string): number {
		const body = fragment
			.replace(/^[A-Za-z]:.*$/gm, '')
			.replace(/\[[Vv]:\d+\]/g, '')
			.replace(/![^!|]*!/g, '')
			.replace(/\|\]|:\||\|\|/g, '|');
		return body.split('|').filter(segment => /[A-Ga-gz]/.test(segment)).length;
	}
	/** 处理第一步：提交主旋律 */
	private handleSubmitMelody(args: Record<string, any>): string {
		try {
			const title = (args.title || '').trim();
			const melody = this.sanitizeFragment(args.melody || '');
			if (!title) return '主旋律提交失败：作品标题不能为空';
			if (!melody) return '主旋律提交失败：旋律内容不能为空';
			const bars = this.countBars(melody);
			if (bars < this.MIN_MELODY_BARS) {
				return `主旋律提交失败：当前仅 ${bars} 小节，音乐展开不足。请扩展至至少 ${this.MIN_MELODY_BARS} 小节（建议 16 小节以上），保持完整的段落结构后重新提交`;
			}
			this.draft = {
				title: title,
				structure: args.structure || '',
				key: args.key || 'C',
				meter: args.meter || '4/4',
				tempo: args.tempo || this.DEFAULT_TEMPO,
				melody: melody,
				melodyInstrument: args.instrument || '钢琴',
				melodyBars: bars,
				accompaniment: '',
				accompanimentInstrument: '',
				chords: '',
				chordInstrument: '',
			};
			console.log(`[演奏者] 主旋律已提交: "${title}"，${bars} 小节，${this.draft.key} 调，${this.draft.tempo}BPM`);
			return `主旋律提交成功：共 ${bars} 小节。下一步请在此旋律基础上调用 submit_accompaniment 生成伴奏声部，小节数必须恰好为 ${bars} 小节并逐小节对齐`;
		}
		catch (error) {
			console.error('[演奏者] 主旋律提交处理异常:', error);
			return `主旋律提交异常: ${error}`;
		}
	}
	/** 处理第二步：提交伴奏声部 */
	private handleSubmitAccompaniment(args: Record<string, any>): string {
		try {
			if (!this.draft) return '伴奏提交失败：尚未提交主旋律，请先调用 submit_melody';
			const accompaniment = this.sanitizeFragment(args.accompaniment || '');
			if (!accompaniment) return '伴奏提交失败：伴奏内容不能为空';
			const bars = this.countBars(accompaniment);
			if (bars !== this.draft.melodyBars) {
				return `伴奏提交失败：伴奏为 ${bars} 小节，与主旋律 ${this.draft.melodyBars} 小节不一致。请逐小节对齐主旋律后重新提交完整的伴奏声部`;
			}
			this.draft.accompaniment = accompaniment;
			this.draft.accompanimentInstrument = args.instrument || this.draft.melodyInstrument;
			console.log(`[演奏者] 伴奏已提交: ${bars} 小节，乐器: ${this.draft.accompanimentInstrument}`);
			return `伴奏提交成功：${bars} 小节，与主旋律对齐。最后一步请调用 submit_chords 生成和弦进行声部，小节数必须恰好为 ${bars} 小节`;
		}
		catch (error) {
			console.error('[演奏者] 伴奏提交处理异常:', error);
			return `伴奏提交异常: ${error}`;
		}
	}
	/** 处理第三步：提交和弦进行并拼装完整乐谱 */
	private handleSubmitChords(args: Record<string, any>): string {
		try {
			if (!this.draft) return '和弦提交失败：尚未提交主旋律，请先调用 submit_melody';
			if (!this.draft.accompaniment) return '和弦提交失败：尚未提交伴奏声部，请先调用 submit_accompaniment';
			const chords = this.sanitizeFragment(args.chords || '');
			if (!chords) return '和弦提交失败：和弦进行内容不能为空';
			const bars = this.countBars(chords);
			if (bars !== this.draft.melodyBars) {
				return `和弦提交失败：和弦进行为 ${bars} 小节，与主旋律 ${this.draft.melodyBars} 小节不一致。请逐小节对齐后重新提交完整的和弦声部`;
			}
			this.draft.chords = chords;
			this.draft.chordInstrument = args.instrument || '贝斯';
			// 拼装完整乐谱并推送前端
			const detail = this.assembleAndPush();
			console.log(`[演奏者] 和弦已提交: ${bars} 小节，乐器: ${this.draft.chordInstrument}，作品《${detail.title}》拼装完成`);
			this.assembledDetail = detail;
			this.draft = null;
			return `和弦进行提交成功，作品《${detail.title}》创作完成：主旋律、伴奏与和弦进行三声部已对齐拼装（共 ${detail.abcLength} 小节）。乐谱已推送到前端，可通过音乐播放器查看和播放。`;
		}
		catch (error) {
			console.error('[演奏者] 和弦提交处理异常:', error);
			return `和弦提交异常: ${error}`;
		}
	}
	/** 将三阶段草稿拼装为多声部完整乐谱并推送前端，返回作品详情 */
	private assembleAndPush(): MusicPieceDetail {
		const d = this.draft!;
		/** 乐器列表：主旋律 → 伴奏 → 和弦，与声部编号一一对应 */
		const instruments = [d.melodyInstrument, d.accompanimentInstrument, d.chordInstrument]
			.map(inst => inst.trim())
			.filter(Boolean)
			.join(',');
		/** 拼装乐谱：头部字段 + 三个对齐的声部 */
		let enrichedAbc = [
			`X:1`,
			`T:${d.title}`,
			`M:${d.meter}`,
			`L:1/8`,
			`Q:1/4=${d.tempo}`,
			`K:${d.key}`,
			`[V:1] ${d.melody}`,
			`[V:2] ${d.accompaniment}`,
			`[V:3] ${d.chords}`,
			''
		].join('\n');
		// 注入 %%voice N 乐器指令，前端为每个声部创建独立合成器
		enrichedAbc = this.injectInstrumentDirective(enrichedAbc, instruments);
		// 推送 ABC 乐谱到前端（用于乐谱可视化展示与合成播放）
		const pushSuccess = pushContext('music', enrichedAbc, '');
		if (!pushSuccess) {
			console.warn('[演奏者] 推送乐谱到前端失败');
		}
		return {
			title: d.title,
			instruments: instruments,
			tempo: d.tempo,
			structure: d.structure,
			key: d.key,
			meter: d.meter,
			abcLength: d.melodyBars,
		};
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