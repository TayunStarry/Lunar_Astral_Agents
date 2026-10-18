import { MessageContent, PostMessage } from '../../config/model';
import { composeImagePlaceholder, handleFromVisionURL } from '../../capabilities/image-tag';

/**
 * 增量降级：窗内多模态图片超过上限时，从最老的消息开始摘图留签
 *
 * @param messages 对话者上下文（原位修改）
 * @param window 多模态窗口上限
 * @param describe 句柄 + 来源 → 占位描述
 */
export function demoteOverflowImagesIn(
	messages: PostMessage[],
	window: number,
	describe: (tag: string, source: string) => string
): void {
	/** 窗内多模态图片计数 */
	const countImages = (): number => messages.reduce(
		(sum, message) => typeof message.content === 'string'
			? sum
			: sum + message.content.filter((c: any) => c.type === 'image_url').length,
		0
	);
	for (let i = 0; i < messages.length && countImages() > window; i++) {
		const message = messages[i];
		// 纯文本消息与无图消息跳过
		if (typeof message.content === 'string') continue;
		if (!message.content.some((c: any) => c.type === 'image_url')) continue;
		demoteMessageImagesIn(message, describe);
	}
}

/**
 * 将单条消息中的图片降级为占位文本：图片项 → 一条「[@I.00x]（描述）」文本
 *
 * 句柄直接从图片项自身的视觉读取 URL（tag 参数）解析 —— 句柄随图走，
 * 无需任何相邻文本配对，连续多图也不会错位。
 */
export function demoteMessageImagesIn(
	message: PostMessage,
	describe: (tag: string, source: string) => string
): void {
	if (typeof message.content === 'string') return;
	const items = message.content as MessageContent[];
	const rebuilt: MessageContent[] = [];
	for (const item of items) {
		const content: any = item;
		if (content.type !== 'image_url') {
			rebuilt.push(item);
			continue;
		}
		// 相邻排布已废除：句柄从 URL 自带的 tag 参数解析
		const tag = handleFromVisionURL(content.image_url?.url);
		rebuilt.push({
			type: 'text', text: tag
				? composeImagePlaceholder(tag, describe(tag, content.image_url?.url || ''))
				: '（一张未能登记句柄的图片，已超出窗口）'
		});
	}
	message.content = rebuilt;
}
