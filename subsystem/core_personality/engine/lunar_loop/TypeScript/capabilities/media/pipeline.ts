import { ImageContent, TextContent } from '../../config/model';
import { GlobalConfig } from '../../config/global';
import { admitImageDataURI } from '../resource';
import { guessImageExt, handleFromVisionURL } from '../image-tag';
import { replaceAnimatedImage } from './animated';
import { resourceAudioOrFallback } from './audio';
import { resourceVideoOrFallback } from './video';

/**
 * 处理图片文件
 *
 * 静态图在推入未读数组时已由入队赋签机制改写为视觉读取形态（约束尺寸 + 句柄叠印副本，
 * 或 base64 形态的同一变换产物），本函数只负责会动的与听得着的媒体：
 *   · 动态图：感知者理解后原位置换为「句柄 + 理解文本」（不占多模态窗口）
 *   · 视频 / 音频：理解或转写后原位置换为「句柄 + 理解/转写文本」
 * 静态图原样透传，保证理解结果与消息内其他内容保持宿主队列的到达时序。
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
			// 视频URL内容块（宿主统一时序队列下发）：收进资源目录后以「句柄 + 理解文本」原位置换
			if (item.type === 'video_url') {
				newContent.push({ type: 'text', text: await resourceVideoOrFallback(item.video_url.url) });
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
			// 视频（扩展名匹配或内联 data:video URI）：收进资源目录后以「句柄 + 理解文本」原位置换
			if (imageSource.toLowerCase().startsWith('data:video/') || GlobalConfig.videoFormatsExtensions.some(format => imageSource.toLowerCase().endsWith(format))) {
				newContent.push({ type: 'text', text: await resourceVideoOrFallback(imageSource) });
				continue;
			}
			/** 是否为动态图（多帧 GIF / APNG / 动态 WebP；视觉端点对动图原样直通，判定依然有效） */
			const [isAnimated, animatedError] = isAnimatedImage(imageSource);
			if (animatedError) console.error('[动态图判定失败]:', animatedError.message);
			// 动态图：置换为「句柄 + 理解文本」
			if (!animatedError && isAnimated) {
				// 句柄优先取 URL 自带的 tag 参数（入队赋签写入）；缺失时现场入库兜底
				let handle = handleFromVisionURL(imageSource);
				if (!handle) {
					const admitted = admitImageDataURI(imageSource, `image${guessImageExt(imageSource)}`);
					handle = admitted?.handle || '';
				}
				newContent.push({ type: 'text', text: await replaceAnimatedImage(imageSource, handle) });
				continue;
			}
			// 静态图：已是视觉读取形态（尺寸受约束、句柄烙在图上），原样进入多模态窗口
			newContent.push(item);
		}
		// 替换消息内容
		message.content = newContent;
	}
}
