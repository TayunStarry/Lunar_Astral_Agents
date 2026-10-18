import { getPromptFromKnowledge } from '../../io/knowledge';
import { admitMedia, existingSummary, mediaName, storeMediaSummary } from './admission';
import { mediaRoleContext } from './registry';
import { understandMediaSegments, useFrameSequence } from './understanding';

/** 视频理解失败时的兜底置换文本 */
export const VIDEO_FALLBACK_TEXT = '月华看不了这个视频呢';

/**
 * 理解视频并返回理解文本（含缓存读写）
 *
 * 媒体化失败时抛出异常；理解文本为空时返回空字符串，由调用方决定兜底提示。
 * 缓存命中时直接返回缓存的理解文本。
 */
export async function understandVideo(videoUrl: string): Promise<string> {
	// 缓存检查：如果已处理过该视频，直接返回缓存结果
	const cachedPrompt = getPromptFromKnowledge(videoUrl);
	if (cachedPrompt) {
		console.log('[感知者] 命中视频缓存，直接返回');
		return cachedPrompt;
	}
	// 序列帧模式：本地抽帧为图片序列（base64），不依赖服务端读取 file:// 视频文件
	if (useFrameSequence()) {
		console.log('[感知者] 开始将视频抽取为图片序列帧...');
		const [groups, framesError] = videoFrames(videoUrl);
		if (!groups || groups.length === 0 || framesError) {
			console.error('[感知者] 视频抽帧失败:', framesError);
			throw new Error('视频抽帧失败');
		}
		/** 采样帧总数（仅用于日志） */
		const frameCount = groups.reduce((sum, group) => sum + group.frames.length, 0);
		console.log(`[感知者] 视频抽帧完成，共 ${groups.length} 组 / ${frameCount} 帧`);
		// 第二步：交给感知者理解并返回理解文本
		return understandMediaSegments(() => mediaRoleContext().perceiverRole.watchFrames(groups), videoUrl);
	}
	// 第一步：视频本地化到 llama-server 媒体目录（长视频自动分段），获取 file:// 引用
	console.log('[感知者] 开始将视频写入媒体目录...');
	const [segments, mediaError] = videoMedia(videoUrl);
	if (!segments || segments.length === 0 || mediaError) {
		console.error('[感知者] 视频媒体化失败:', mediaError);
		throw new Error('视频媒体化失败');
	}
	/** 媒体片段 file:// 引用列表（llama-server 端 ffmpeg 自动抽帧） */
	const mediaUrls = segments.map(segment => `file://${segment.file}`);
	console.log(`[感知者] 视频媒体化完成，共 ${mediaUrls.length} 个片段`);
	// 第二步：交给感知者理解并返回理解文本
	return understandMediaSegments(() => mediaRoleContext().perceiverRole.watchVideo(mediaUrls), videoUrl);
}

/**
 * 视频资源化：本地化 → 登记 → 理解 → 写回结论 → 产出带句柄文本
 *
 * @param originalSource 用户/渠道给出的原始视频来源（理解链路的输入）
 * @param name 建议文件名
 * @param origin 来源语义
 * @param admitSource 登记进资源目录的来源（本地化产物路径；缺省用原始来源）
 * @returns 形如 `@V.001【视频理解】…` 的文本；失败返回空串由调用方兜底
 */
export async function resourceVideo(
	originalSource: string,
	name: string,
	origin: 'client-upload' | 'channel',
	admitSource?: string
): Promise<string> {
	const media = admitMedia(admitSource || originalSource, name, origin, 'V');
	if (!media) return '';
	const cached = existingSummary(media);
	if (cached) return `${media.handle}【视频理解】${cached}`;
	try {
		const summary = await understandVideo(originalSource);
		if (!summary || !summary.trim()) return `${media.handle}`;
		storeMediaSummary(media, summary);
		return `${media.handle}【视频理解】${summary}`;
	}
	catch (error) {
		console.error('[感知者] 视频理解失败（句柄已保留）:', error);
		return `${media.handle}${VIDEO_FALLBACK_TEXT}`;
	}
}

/** 理解视频并返回理解文本；失败或理解为空时返回兜底/默认提示（调用方原位置换） */
export async function understandVideoOrFallback(videoUrl: string): Promise<string> {
	try {
		/** 视频理解文本 */
		const summary = await understandVideo(videoUrl);
		if (summary.trim().length > 0) return summary;
		// 理解为空时以默认应答兜底
		return mediaRoleContext().randomDefaultMessage();
	}
	catch (error) {
		console.error('[感知者] 视频理解失败，以兜底文本置换:', error);
		return VIDEO_FALLBACK_TEXT;
	}
}

/**
 * 视频资源化包装：收进资源目录后返回「句柄 + 理解文本」
 *
 * 资源化失败（目录不可用等）时退化为纯理解文本，保证对话链路不中断。
 *
 * @param videoSource 视频来源（本地路径 / 服务地址 / 远程 URL / data URI）
 * @returns 置换用文本
 */
export async function resourceVideoOrFallback(videoSource: string): Promise<string> {
	try {
		const text = await resourceVideo(videoSource, mediaName(videoSource, '.mp4'), videoSource.startsWith('data:') ? 'client-upload' : 'channel');
		if (text) return text;
	}
	catch (error) {
		console.error('[感知者] 视频资源化失败，退化为纯理解文本:', error);
	}
	return understandVideoOrFallback(videoSource);
}
