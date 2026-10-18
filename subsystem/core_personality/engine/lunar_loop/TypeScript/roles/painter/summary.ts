import type { ToolCallItem } from '../../config/model';
import { interactEvent } from '../../capabilities/ltp-event';
import { resolveImageRef } from '../../capabilities/image-tag';
import { normalizeStrength } from './frame';
import { normalizeGroupRoles, characterLabel } from './characters';
import type { PaintingDetail } from './types';

/** 从工具调用中提取绘画作品详情 */
export function collectPaintingDetail(
	toolCall: ToolCallItem,
	paintings: PaintingDetail[]
): void {
	try {
		const args = typeof toolCall.function.arguments === 'string'
			? JSON.parse(toolCall.function.arguments)
			: toolCall.function.arguments;
		const name = toolCall.function.name;
		if (name === 'portrait') {
			const character = String(args.character || '通用-女');
			paintings.push({
				toolName: name,
				promptSummary: `${characterLabel(character)}${args.chibi === true ? 'Q版' : '全尺寸'}立绘`,
				character: character,
				chibi: args.chibi === true,
				expression: args.expression || '',
				posture: args.posture || '',
				environment: args.environment || '',
			});
		}
		else if (name === 'group_photo') {
			const roles = normalizeGroupRoles(args.roles);
			paintings.push({
				toolName: name,
				promptSummary: `${args.chibi === true ? 'Q版' : ''}与${roles.join('、') || '他人'}的合照`,
				roles,
				chibi: args.chibi === true,
				planning: args.planning || '',
				environment: args.environment || '',
			});
		}
		else if (name === 'diffusion_generation' || name === 'image_to_image') {
			const prompt = args.prompt || '';
			/** 强度标注（图生图由绘制者指定，记入作品详情便于核对） */
			const strengthNote = name === 'image_to_image' && args.strength !== undefined
				? `（强度 ${normalizeStrength(args.strength, '图生图')}）`
				: '';
			paintings.push({
				toolName: name,
				promptSummary: (prompt.length > 100 ? prompt.slice(0, 97) + '...' : prompt) + strengthNote,
			});
		}
		else if (name === 'upscale_image') {
			const resolved = args.image_tag ? resolveImageRef(String(args.image_tag)) : null;
			paintings.push({
				toolName: name,
				promptSummary: resolved ? `${resolved.handle} 超分重建` : '最近作品超分重建',
			});
		}
	} catch {
		// 解析失败时跳过，不阻断流程
	}
}

/** 构建绘画作品摘要，使用月华话术格式 */
export function buildPaintingSummary(paintings: PaintingDetail[]): string {
	// 检查是否有作品
	if (paintings.length === 0) return '月华没有绘制任何作品';
	// 事件 -> 绘制画作前：推送作品详情，插件可改写后再汇总
	const feedback: PaintingDetail[] = interactEvent('draw_painting_before', { paintings }).return;
	// 插件可改写作品详情，如添加乐器、速度、结构等
	if (feedback && Array.isArray(feedback) && feedback.every(p => p.toolName && p.promptSummary)) {
		paintings = feedback;
	}
	const parts: string[] = [];
	for (const p of paintings) {
		if (p.toolName === 'portrait') {
			/** 角色 → 话术：月华说「自画像」，琉璃与他人说「立绘」 */
			let desc: string;
			if (p.character === '月华') desc = p.chibi ? '月华绘制了一幅Q版自画像' : '月华绘制了一幅全尺寸自画像';
			else if (p.character === '琉璃') desc = `月华为琉璃绘制了一幅${p.chibi ? 'Q版' : '全尺寸'}立绘`;
			else desc = `月华绘制了一幅${p.chibi ? 'Q版' : '全尺寸'}${characterLabel(p.character)}立绘`;
			if (p.expression) desc += `，展现了${p.expression}`;
			if (p.environment) desc += `，背景是${p.environment}`;
			parts.push(desc + '。');
		}
		else if (p.toolName === 'group_photo') {
			let desc = `月华绘制了一幅${p.chibi ? 'Q版' : ''}与${p.roles && p.roles.length > 0 ? p.roles.join('、') : '他人'}的合照`;
			if (p.planning) desc += `，${p.planning}`;
			if (p.environment) desc += `，背景是${p.environment}`;
			parts.push(desc + '。');
		}
		else if (p.toolName === 'image_to_image') parts.push(`月华参照参考图绘制了一幅图像：${p.promptSummary}。`);
		else if (p.toolName === 'upscale_image') parts.push(`月华为${p.promptSummary}提升了清晰度，图像已变得更精细。`);
		else parts.push(`月华绘制了一幅图像：${p.promptSummary}。`);
	}
	parts.push('图像已通过前端推送给用户。');
	return parts.join('\n');
}
