import type { MusicDraft } from './types';

/** 清理声部片段：剥离头部字段、[V:N] 声部标记与多余空白，合并为单行 */
export function sanitizeFragment(fragment: string): string {
	return fragment
		.replace(/^[A-Za-z]:.*$/gm, '')
		.replace(/\[[Vv]:\d+\]/g, '')
		.replace(/\s+/g, ' ')
		.trim();
}

/** 统计声部片段的小节数（忽略头部字段、声部标记与力度记号） */
export function countBars(fragment: string): number {
	const body = fragment
		.replace(/^[A-Za-z]:.*$/gm, '')
		.replace(/\[[Vv]:\d+\]/g, '')
		.replace(/![^!|]*!/g, '')
		.replace(/\|\]|:\||\|\|/g, '|');
	return body.split('|').filter(segment => /[A-Ga-gz]/.test(segment)).length;
}

/**
 * 将三阶段草稿拼装为多声部完整乐谱
 *
 * @param draft 已对齐的三声部草稿
 * @returns 拼装后的 ABC 乐谱与乐器清单（逗号分隔）
 */
export function assembleScore(draft: MusicDraft): { abc: string; instruments: string } {
	/** 乐器列表：主旋律 → 伴奏 → 和弦，与声部编号一一对应 */
	const instruments = [draft.melodyInstrument, draft.accompanimentInstrument, draft.chordInstrument]
		.map(inst => inst.trim())
		.filter(Boolean)
		.join(',');
	/** 拼装乐谱：头部字段 + 三个对齐的声部 */
	const abc = [
		`X:1`,
		`T:${draft.title}`,
		`M:${draft.meter}`,
		`L:1/8`,
		`Q:1/4=${draft.tempo}`,
		`K:${draft.key}`,
		`[V:1] ${draft.melody}`,
		`[V:2] ${draft.accompaniment}`,
		`[V:3] ${draft.chords}`,
		''
	].join('\n');
	return { abc: injectInstrumentDirective(abc, instruments), instruments };
}

/**
 * 将乐器列表以 `%%voice N` 指令注入乐谱头部
 *
 * 每个乐器对应一个声部（voice），前端 music_renderer.html 会据此为每个声部
 * 创建独立的 Tone.js 合成器，实现真正的多乐器并行演奏。
 *
 * 指令格式：`%%voice 1 钢琴`、`%%voice 2 小提琴` ...
 */
export function injectInstrumentDirective(abcNotation: string, instruments: string): string {
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
	const directives = list.map((inst, index) => `%%voice ${index + 1} ${inst}`);
	const directive = directives.join('\n') + '\n';

	const xMatch = abcNotation.match(/^X:\s*\d+/m);
	if (xMatch && xMatch.index !== undefined) {
		const before = abcNotation.substring(0, xMatch.index);
		const after = abcNotation.substring(xMatch.index);
		return before + directive + after;
	}
	return directive + abcNotation;
}
