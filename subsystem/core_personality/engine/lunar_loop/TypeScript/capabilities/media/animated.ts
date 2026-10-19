import { getPromptFromKnowledge } from '../../io/knowledge';
import { resolveAdmittedMedia, storeMediaSummary } from './admission';
import { mediaRoleContext } from './registry';
import { understandMediaSegments } from './understanding';

/**
 * 理解动态图（感知者智能体）
 *
 * 动态图直接解码为帧序列（尺寸受限、至多 64 帧），由感知者按序列帧理解
 * 画面内容与帧间动作变化。不再走「动态图转慢放视频」的升格链路。
 *
 * @param imageSource 动态图数据或地址（字节 / HTTP(S) URL / data:image URI）
 * @param cacheKey 知识库缓存键（URL 来源可缓存，data URI 为一次性数据不缓存）
 */
export async function understandAnimatedImage(imageSource: any, cacheKey: string): Promise<string> {
	// 缓存检查：同一张动态图不重复做抽帧与理解
	const cachedPrompt = cacheKey ? getPromptFromKnowledge(cacheKey) : '';
	if (cachedPrompt) {
		console.log('[感知者] 命中动态图缓存，直接返回');
		return cachedPrompt;
	}
	console.log('[感知者] 检测到动态图，开始解码帧序列...');
	const [groups, framesError] = animatedImageToFrames(imageSource);
	if (!groups || groups.length === 0 || framesError) {
		console.error('[感知者] 动态图抽帧失败:', framesError);
		throw new Error('动态图抽帧失败');
	}
	/** 采样帧总数（仅用于日志） */
	const frameCount = groups.reduce((sum, group) => sum + group.frames.length, 0);
	console.log(`[感知者] 动态图抽帧完成，共 ${groups.length} 组 / ${frameCount} 帧`);
	return understandMediaSegments(() => mediaRoleContext().perceiverRole.watchFrames(groups), cacheKey);
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
