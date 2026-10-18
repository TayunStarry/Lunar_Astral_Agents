import { interactEvent } from '../../capabilities/ltp-event';
import type { MusicPieceDetail } from './types';

/** 构建音乐作品摘要，使用月华话术格式 */
export function buildMusicSummary(pieces: MusicPieceDetail[]): string {
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
	for (const p of pieces) {
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
