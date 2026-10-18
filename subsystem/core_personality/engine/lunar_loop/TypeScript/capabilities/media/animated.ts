import { getPromptFromKnowledge } from '../../io/knowledge';
import { resolveAdmittedMedia, storeMediaSummary } from './admission';
import { mediaRoleContext } from './registry';
import { understandMediaSegments, useFrameSequence } from './understanding';

/**
 * 理解动态图（感知者智能体）
 *
 * 动态图先编码为慢放视频再走与视频相同的理解链路（file 模式下写媒体目录并由服务端抽帧，
 * 序列帧模式下本地抽帧为 base64 图片序列），时序与动作过程由视频采样点承载。
 * 返回理解文本；转码失败时抛出异常，缓存命中时直接返回缓存文本。
 *
 * @param imageSource 动态图数据或地址（字节 / HTTP(S) URL / data:image URI）
 * @param cacheKey 知识库缓存键（URL 来源可缓存，data URI 为一次性数据不缓存）
 */
export async function understandAnimatedImage(imageSource: any, cacheKey: string): Promise<string> {
	// 缓存检查：同一张动态图不重复做转码与理解
	const cachedPrompt = cacheKey ? getPromptFromKnowledge(cacheKey) : '';
	if (cachedPrompt) {
		console.log('[感知者] 命中动态图缓存，直接返回');
		return cachedPrompt;
	}
	// 第一步：动态图编码为慢放视频（补足抽帧采样点数）
	console.log('[感知者] 检测到动态图，开始编码为视频...');
	// 序列帧模式：慢放转码后抽帧为图片序列（base64），不依赖服务端读取 file:// 视频文件
	if (useFrameSequence()) {
		const [groups, framesError] = animatedImageToFrames(imageSource);
		if (!groups || groups.length === 0 || framesError) {
			console.error('[感知者] 动态图抽帧失败:', framesError);
			throw new Error('动态图抽帧失败');
		}
		/** 采样帧总数（仅用于日志） */
		const frameCount = groups.reduce((sum, group) => sum + group.frames.length, 0);
		console.log(`[感知者] 动态图抽帧完成，共 ${groups.length} 组 / ${frameCount} 帧`);
		// 第二步：交给感知者理解并返回理解文本
		return understandMediaSegments(() => mediaRoleContext().perceiverRole.watchFrames(groups), cacheKey);
	}
	const [segments, mediaError] = animatedImageToVideo(imageSource);
	if (!segments || segments.length === 0 || mediaError) {
		console.error('[感知者] 动态图转视频失败:', mediaError);
		throw new Error('动态图转视频失败');
	}
	/** 媒体片段 file:// 引用列表 */
	const mediaUrls = segments.map(segment => `file://${segment.file}`);
	console.log(`[感知者] 动态图转视频完成，共 ${mediaUrls.length} 个片段`);
	// 第二步：交给感知者理解并返回理解文本
	return understandMediaSegments(() => mediaRoleContext().perceiverRole.watchVideo(mediaUrls), cacheKey);
}

/**
 * 动态图原位置换：感知者理解 → 结论写回资源目录 → 产出「句柄 + 理解文本」
 *
 * @param imageSource 动态图数据或地址
 * @param handle 入队赋签已分配的句柄（可空：结论写回跳过，仅返回理解文本）
 * @returns 置换用文本（理解为空时以默认应答兜底）
 */
export async function replaceAnimatedImage(imageSource: string, handle: string): Promise<string> {
	const media = resolveAdmittedMedia(handle);
	/** 理解文本（内联 data URI 为一次性上传数据不缓存；URL 来源以原地址为缓存键） */
	let summary = '';
	try {
		summary = await understandAnimatedImage(imageSource, imageSource.startsWith('data:') ? '' : imageSource);
	}
	catch (error) {
		console.error('[感知者] 动态图理解失败，以兜底文本置换:', error);
	}
	if (media && summary.trim().length > 0) storeMediaSummary(media, summary);
	// 句柄优先，理解文本紧随其后（理解为空时以默认应答兜底）
	const body = summary.trim().length > 0 ? `【动态图理解】${summary}` : mediaRoleContext().randomDefaultMessage();
	return media ? `${media.handle}${body}` : body;
}
