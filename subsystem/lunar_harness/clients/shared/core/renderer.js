/**
 * 星月智能 · 桌面端 / 移动端共享角色渲染器
 *
 * 读取 config/general.json 的 system.character_renderer，
 * 按 bedrock / live2d / gif 选择 iframe 地址；非法或缺省回退 bedrock。
 */
import { readGeneralConfig } from './api.js';

export const RENDERERS = ['bedrock', 'live2d', 'gif'];

/** 读取配置中的渲染器；失败或缺省返回 bedrock */
export async function loadRendererChoice() {
	try {
		const config = await readGeneralConfig();
		const value = config && config.system ? config.system.character_renderer : null;
		return RENDERERS.includes(value) ? value : 'bedrock';
	} catch {
		return 'bedrock';
	}
}

/** 按渲染器选择 iframe 地址（WsBridge 未传 ws 参数时自动走同源代理） */
export function applyRendererFrame(frame, renderer = 'bedrock') {
	if (!frame) return;
	const sources = {
		live2d: frame.dataset.srcLive2d,
		gif: frame.dataset.srcGif
	};
	const src = sources[renderer] || frame.dataset.src;
	if (!src) return;
	frame.src = `${src}&ws=${window.location.host}`;
}

/** 一次性完成「读配置 + 应用 iframe」，返回最终渲染器 */
export async function setupRendererFrame(frameOrId, renderer) {
	const frame = typeof frameOrId === 'string' ? document.getElementById(frameOrId) : frameOrId;
	const choice = renderer || await loadRendererChoice();
	applyRendererFrame(frame, choice);
	return choice;
}
