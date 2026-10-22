import { admitMedia, mediaName } from './admission';

// ===== 视频入库（只发句柄，不再自动解析） =====
//
// 视频进入未读消息时只做「收进资源目录 + 给句柄」：内容理解改由句柄系统按需完成
// （Go 侧描述者现场观看、写回描述）。这样消息入口不再阻塞在一次完整的视频理解上，
// 也不会把没被问到的视频理解结果塞进上下文。

/** 视频入库失败时的兜底置换文本 */
export const VIDEO_FALLBACK_TEXT = '月华看不了这个视频呢';

/**
 * 视频资源化：收进资源目录后返回裸句柄
 *
 * @param originalSource 用户/渠道给出的原始视频来源
 * @param name 建议文件名
 * @param origin 来源语义
 * @returns 形如 `@V.001` 的句柄；入库失败返回空串
 */
export function resourceVideo(
	originalSource: string,
	name: string,
	origin: 'client-upload' | 'channel'
): string {
	const media = admitMedia(originalSource, name, origin, 'V');
	return media ? media.handle : '';
}

/**
 * 视频资源化包装：收进资源目录后返回裸句柄
 *
 * 资源化失败（目录不可用等）时退化为兜底提示，保证对话链路不中断。
 *
 * @param videoSource 视频来源（本地路径 / 服务地址 / 远程 URL / data URI）
 * @returns 置换用文本
 */
export function resourceVideoOrFallback(videoSource: string): string {
	try {
		const handle = resourceVideo(videoSource, mediaName(videoSource, '.mp4'), videoSource.startsWith('data:') ? 'client-upload' : 'channel');
		if (handle) return handle;
	}
	catch (error) {
		console.error('[视频] 资源化失败:', error);
	}
	return VIDEO_FALLBACK_TEXT;
}
