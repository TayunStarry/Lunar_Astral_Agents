import { ImageContent, TextContent } from '../../config/model';
import { GlobalConfig } from '../../config/global';
import { admitImageDataURI } from '../resource';
import { guessImageExt, handleFromVisionURL } from '../image-tag';
import { resourceAudioOrFallback } from './audio';
import { resourceVideoOrFallback } from './video';

/** 动态图入库失败时的兜底置换文本 */
const ANIMATED_FALLBACK_TEXT = '月华看不了这个动图呢';

/**
 * 处理未读消息中的媒体内容
 *
 * 入口语义（新）：
 *   · 动态图：只入库给句柄，不再自动理解；
 *   · 视频：只入库给句柄，不再自动理解；
 *   · 音频：仍走转写（语音内容无法用句柄二次取用，转写是唯一可用形态）；
 *   · 静态图：原样透传（入队赋签已改写为视觉读取形态）。
 * 视频与动态图的内容描述改由句柄系统按需生成（use_resource），
 * 避免在消息入口阻塞一次完整理解，也避免把用户没问到的内容塞进上下文。
 */
export async function LiteImageFile(): Promise<void> {
	// 遍历未读上下文数组中的每个消息
	for (let message of GlobalConfig.unreadContext) {
		// 跳过纯文本消息
		if (typeof message.content === 'string') continue;
		/** 新内容数组 */
		const newContent: Array<ImageContent | TextContent> = [];
		// 遍历消息内容中的每个项
		for (let item of message.content as any[]) {
			// 如果是文本项,直接添加到新内容数组
			if (item.type == 'text') {
				newContent.push(item);
				continue;
			}
			// 视频URL内容块（宿主统一时序队列下发）：收进资源目录后只留下句柄
			if (item.type === 'video_url') {
				newContent.push({ type: 'text', text: resourceVideoOrFallback(item.video_url.url) });
				continue;
			}
			// 音频URL内容块（宿主统一时序队列下发）：收进资源目录后以「句柄 + 转写文本」原位置换
			if (item.type === 'audio_url') {
				newContent.push({ type: 'text', text: await resourceAudioOrFallback(item.audio_url.url) });
				continue;
			}
			// 音频内容（内联 base64）：转写为文本后原位替换音频块
			if ('input_audio' in item) {
				const format = item.input_audio.format || 'wav';
				newContent.push({ type: 'text', text: await resourceAudioOrFallback(`data:audio/${format};base64,${item.input_audio.data}`) });
				continue;
			}
			if (!item.image_url) continue;
			/** 图片来源地址（入队时已改写为视觉读取 URL / data URI） */
			const imageSource = item.image_url.url;
			// 视频（扩展名匹配或内联 data:video URI）：收进资源目录后只留下句柄
			if (imageSource.toLowerCase().startsWith('data:video/') || GlobalConfig.videoFormatsExtensions.some(format => imageSource.toLowerCase().endsWith(format))) {
				newContent.push({ type: 'text', text: resourceVideoOrFallback(imageSource) });
				continue;
			}
			/** 是否为动态图（多帧 GIF / APNG / 动态 WebP；视觉端点对动图原样直通，判定依然有效） */
			const [isAnimated, animatedError] = isAnimatedImage(imageSource);
			if (animatedError) console.error('[动态图判定失败]:', animatedError.message);
			// 动态图：只保留句柄，内容描述由句柄系统按需生成
			if (!animatedError && isAnimated) {
				// 句柄优先取 URL 自带的 tag 参数（入队赋签写入）；缺失时现场入库兜底
				let handle = handleFromVisionURL(imageSource);
				if (!handle) {
					const admitted = admitImageDataURI(imageSource, `image${guessImageExt(imageSource)}`);
					handle = admitted?.handle || '';
				}
				newContent.push({ type: 'text', text: handle || ANIMATED_FALLBACK_TEXT });
				continue;
			}
			// 静态图：已是视觉读取形态（尺寸受约束、句柄烙在图上），原样进入多模态窗口
			newContent.push(item);
		}
		// 替换消息内容
		message.content = newContent;
	}
}
