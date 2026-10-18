// ===== 对话缓存（database/messages.json） =====
//
// 缓存文件的唯一写入方：思维链。前端只在页面刷新时读取并渲染，不再写盘/删档。
//
// 文件内容是前端可直接渲染的消息列表（最多 48 条），同时充当"近期记忆"：
//   - 月华启动时把缓存载回对话者上下文；
//   - 每轮思考结束、以及每次向前端推送文本/语音/图片时落盘；
//   - 写盘用 /file/read/ 便于前端查看原图；载入思维链时统一换成
//     /file/read-multimodal/ 视觉副本；
//   - 月华自己输出的图片只给前端看，不载回上下文。

import type { CachedAttachment, CachedMessage } from './conversation/types';
import { fileNameOf, localPathToReadURL, saveBase64Media, stripUserPrefix, toDisplaySource } from './conversation/codec';
import { appendCachedMessage, persistConversation } from './conversation/store';

export { initConversationCache, MAX_CACHE_MESSAGES, persistConversation } from './conversation/store';

/** 生成消息 ID（时间戳 + 随机后缀） */
function newId(): string {
	return Date.now().toString(36) + Math.random().toString(36).slice(2, 8);
}

/** 从用户消息内容构造展示记录 */
function buildUserRecord(content: any): CachedMessage | null {
	const attachments: CachedAttachment[] = [];
	const texts: string[] = [];
	let hasImage = false;
	let hasVoice = false;

	const pushAttachment = (type: string, source: string, label?: string) => {
		const src = toDisplaySource(source);
		if (!src) return;
		attachments.push({ type, src, label: label || fileNameOf(src) });
	};

	if (typeof content === 'string') texts.push(stripUserPrefix(content));
	else if (Array.isArray(content)) {
		for (const item of content as any[]) {
			if (!item) continue;
			if (item.type === 'text') {
				const text = stripUserPrefix(String(item.text || ''));
				// 前端为媒体附加的占位符不进入展示正文
				if (!text || text === '[视频]' || text === '[语音]') continue;
				texts.push(text);
			}
			else if (item.type === 'image_url') {
				pushAttachment('image', item.image_url?.url || '');
				hasImage = true;
			}
			else if (item.type === 'video_url') {
				pushAttachment('video', item.video_url?.url || '');
				hasImage = true;
			}
			else if (item.type === 'input_audio') {
				const src = saveBase64Media(item.input_audio?.data || '', `audio/${item.input_audio?.format || 'wav'}`, 'voice');
				if (src) {
					attachments.push({ type: 'audio', src, label: '语音' });
					hasVoice = true;
				}
			}
			else if (item.type === 'audio_url') {
				pushAttachment('audio', item.audio_url?.url || '', '语音');
				hasVoice = true;
			}
		}
	}

	const text = texts.join('\n').trim();
	if (!text && !attachments.length) return null;
	const categories: string[] = [];
	if (text) categories.push('text');
	if (hasImage) categories.push('image');
	if (hasVoice) categories.push('voice');
	if (/@[DIVAT]\.\d{3,}/.test(text)) categories.push('file');
	return {
		id: newId(),
		role: 'user',
		categories: categories.length ? categories : ['text'],
		content: text,
		attachments: attachments.length ? attachments : undefined,
		timestamp: Date.now(),
	};
}

/** 用户消息入缓存（在图片赋签之后调用，图片来源已改写为视觉读取地址） */
export function appendUserMessage(content: any): void {
	const record = buildUserRecord(content);
	if (!record) return;
	appendCachedMessage(record);
	persistConversation();
}

/** 记录一条助手文本/语音/乐谱/动作消息 */
function recordAssistantContext(type: string, content: string, audio: string): void {
	if (type === 'music_audio') return; // 乐谱音频回执不产生气泡
	const base: CachedMessage = {
		id: newId(),
		role: 'assistant',
		categories: ['text'],
		content: String(content || ''),
		timestamp: Date.now(),
	};
	if (type === 'music') {
		base.categories = ['music'];
		base.content = '';
		base.abcNotation = String(content || '');
	}
	else if (type === 'action') {
		base.categories = ['action'];
		base.actionType = type;
	}
	else if (type === 'text') {
		if (audio) {
			base.categories = ['text', 'voice'];
			base.audio = audio;
		}
		const notice = /^\[已收录\]\s*(@[DIVAT]\.\d{3,})\s*(.*)$/.exec(base.content.trim());
		if (notice) {
			base.categories = ['text', 'file'];
			base.fileRef = notice[1];
			base.fileLabel = notice[2] || notice[1];
		}
	}
	else {
		base.categories = [type];
	}
	appendCachedMessage(base);
}

/**
 * 推送助手上下文并写入缓存
 *
 * @param type 消息类型（text / action / music / music_audio）
 * @param content 正文
 * @param audio 语音 base64（无则空串）
 * @param auto 是否快速应答
 */
export function pushAssistantContext(type: string, content: string, audio: string, auto?: boolean): boolean {
	const ok = pushContext(type, content, audio, auto);
	recordAssistantContext(type, content, audio);
	persistConversation();
	return ok;
}

/**
 * 推送助手图片并写入缓存
 *
 * @param images 图片 base64 列表
 * @param isSticker 是否表情包
 * @param sourcePaths 可选：图片已落盘的本地路径（生成/超分结果），优先转成 /file/read/ 地址
 */
export function pushAssistantImage(images: string[], isSticker?: boolean, sourcePaths?: string[]): boolean {
	const ok = pushImage(images, isSticker);
	(images || []).forEach((image, index) => {
		if (!image) return;
		const hint = sourcePaths && sourcePaths[index] ? localPathToReadURL(sourcePaths[index]) : '';
		const src = hint || saveBase64Media(image, 'image/jpeg', isSticker ? 'sticker' : 'image');
		if (!src) return;
		appendCachedMessage({
			id: newId(),
			role: 'assistant',
			categories: ['image'],
			content: '',
			imageSrc: src,
			imageLabel: isSticker ? '表情包' : '图片',
			timestamp: Date.now(),
		});
	});
	persistConversation();
	return ok;
}
