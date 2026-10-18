import { ToolCall } from '../../config/tool';
import { ToolCallItem } from '../../config/model';
import { CreativeRoleBase } from '../base/creative';
import { musicTools } from './musician/tools';
import { MusicDraft, MusicPieceDetail } from './musician/types';
import { assembleScore, countBars, sanitizeFragment } from './musician/abc';
import { buildMusicSummary } from './musician/summary';

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
	/** 智能体角色标签（TUI 实况屏显示） */
	protected roleTag: string = "演奏者";
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

	/** 构造函数 */
	public constructor() {
		super(fileView('prompts/musicianRole.md')[0]);
	}

	/** 角色名称 */
	protected get roleName(): string { return '演奏者' }

	/** 获取工具定义 */
	protected getToolDefinitions(): ToolCall[] { return musicTools }

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
		return buildMusicSummary(pieces);
	}

	/** 处理第一步：提交主旋律 */
	private handleSubmitMelody(args: Record<string, any>): string {
		try {
			const title = (args.title || '').trim();
			const melody = sanitizeFragment(args.melody || '');
			if (!title) return '主旋律提交失败：作品标题不能为空';
			if (!melody) return '主旋律提交失败：旋律内容不能为空';
			const bars = countBars(melody);
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
			const accompaniment = sanitizeFragment(args.accompaniment || '');
			if (!accompaniment) return '伴奏提交失败：伴奏内容不能为空';
			const bars = countBars(accompaniment);
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
			const chords = sanitizeFragment(args.chords || '');
			if (!chords) return '和弦提交失败：和弦进行内容不能为空';
			const bars = countBars(chords);
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
		const draft = this.draft!;
		const { abc, instruments } = assembleScore(draft);
		// 推送 ABC 乐谱到前端（用于乐谱可视化展示与合成播放）
		if (!pushContext('music', abc, '')) {
			console.warn('[演奏者] 推送乐谱到前端失败');
		}
		return {
			title: draft.title,
			instruments: instruments,
			tempo: draft.tempo,
			structure: draft.structure,
			key: draft.key,
			meter: draft.meter,
			abcLength: draft.melodyBars,
		};
	}
}
