/**
 * 星月智能 · 共享消息内容块
 *
 * 只负责「一条消息内部的内容块」DOM 构建：文本、图片、视频、音频、
 * 语音转写、语音重播胶囊、音乐卡。
 *
 * 设计边界：
 * - 不绑定任何端内全局状态（消息列表、筛选、引用、发送状态都不在这里）
 * - 不判断运行端，播放 / 预览 / 音乐卡等能力由调用方通过参数注入
 * - 外层消息壳（桌面气泡 + 操作按钮、移动简化气泡 + 发送状态）仍由两端各自实现
 */

import { renderMarkdownSync } from './markdown.js';

const DEFAULT_PREVIEW = (src, label) => {
	if (typeof globalThis.previewImage === 'function') globalThis.previewImage(src, label);
};

/** 图片块：图片网格 + 可点击预览 + 左下角标签 */
export function buildImageBlock({ src, label = '图片', onPreview = DEFAULT_PREVIEW } = {}) {
	const grid = document.createElement('div');
	grid.className = 'image-grid';
	const container = document.createElement('div');
	container.className = 'labeled-image-container';
	container.style.setProperty('--image-label', `'${label}'`);
	const img = document.createElement('img');
	img.src = src;
	img.alt = label;
	img.loading = 'lazy';
	img.addEventListener('click', () => onPreview(src, label));
	container.appendChild(img);
	grid.appendChild(container);
	return grid;
}

/** 视频块 */
export function buildVideoBlock({ src, className = 'video-container' } = {}) {
	const container = document.createElement('div');
	container.className = className;
	const video = document.createElement('video');
	video.src = src;
	video.controls = true;
	video.playsInline = true;
	container.appendChild(video);
	return container;
}

/** 音频文件块（原生播放器） */
export function buildAudioBlock({ src, className = 'message-audio-player', preload } = {}) {
	const audio = document.createElement('audio');
	audio.className = className;
	audio.controls = true;
	audio.src = src;
	if (preload) audio.preload = preload;
	return audio;
}

/** 语音转写块：展示句柄与转写文本，替代已取消的音频落盘播放 */
export function buildVoiceTranscriptBlock(att = {}) {
	const wrap = document.createElement('div');
	wrap.className = 'voice-transcript';
	const head = document.createElement('div');
	head.className = 'voice-transcript-head';
	head.innerHTML = '<i class="fas fa-microphone"></i> ';
	head.appendChild(document.createTextNode(att.label || '语音'));
	if (att.handle) head.title = att.handle;
	const text = document.createElement('div');
	text.className = 'voice-transcript-text';
	text.textContent = att.text || '（这段语音没有可用的转写文本）';
	wrap.appendChild(head);
	wrap.appendChild(text);
	return wrap;
}

/** 普通文件块：展示来源方声明的文件名与大小 */
export function buildFileAttachmentBlock(att = {}) {
	const wrap = document.createElement('div');
	wrap.className = 'file-attachment';
	const icon = document.createElement('i');
	icon.className = 'fas fa-file';
	wrap.appendChild(icon);
	const name = document.createElement('span');
	name.className = 'file-attachment-name';
	name.textContent = att.label || '文件';
	if (att.handle) name.title = att.handle;
	wrap.appendChild(name);
	if (att.size) {
		const size = document.createElement('span');
		size.className = 'file-attachment-size';
		size.textContent = att.size;
		wrap.appendChild(size);
	}
	return wrap;
}

/**
 * 单个附件块
 *
 * @param {{type:'image'|'video'|'audio'|'file', src?:string, label?:string, size?:string, text?:string, handle?:string}} att
 * @param {object} [options] 透传给图片 / 视频 / 音频构建器
 * @returns {HTMLElement|null}
 */
export function buildAttachmentBlock(att, options = {}) {
	if (!att) return null;
	if (att.type === 'file') {
		return buildFileAttachmentBlock(att);
	}
	if (att.type === 'image') {
		return buildImageBlock({ src: att.src, label: att.label || '图片', onPreview: options.onPreview });
	}
	if (att.type === 'video') {
		return buildVideoBlock({ src: att.src, className: options.videoClass });
	}
	if (att.type === 'audio') {
		// 音频不再持久化：历史消息只有悬空句柄 + 转写文本，不渲染播放器
		if (!att.src) return buildVoiceTranscriptBlock(att);
		return buildAudioBlock({ src: att.src, className: options.audioClass, preload: options.audioPreload });
	}
	return null;
}

/**
 * 文本块
 *
 * @param {string} content 原始正文
 * @param {{className?:string, isPlain?:boolean, processThink?:boolean, render?:boolean}} [options]
 *        render=false 时只创建容器，交给调用方后续填充（桌面端异步渲染 Markdown）
 */
export function buildTextBlock(content, { className = 'markdown-content', isPlain = false, processThink = true, render = true } = {}) {
	const div = document.createElement('div');
	div.className = className;
	if (!render) return div;
	const raw = String(content ?? '');
	if (isPlain || typeof window.marked === 'undefined') {
		div.classList.add('is-plain');
		div.textContent = raw;
		return div;
	}
	try {
		div.innerHTML = renderMarkdownSync(raw, { processThink });
	} catch (error) {
		div.classList.add('is-plain');
		div.textContent = raw;
	}
	return div;
}

/** 读取语音音频的真实时长（秒）；拿不到返回 0，界面据此隐藏时长 */
export function probeVoiceDuration(src) {
	return new Promise(resolve => {
		if (!src) return resolve(0);
		const url = src.startsWith('data:') ? src : 'data:audio/wav;base64,' + src;
		const probe = new Audio();
		let settled = false;
		const finish = (value) => {
			if (settled) return;
			settled = true;
			probe.onloadedmetadata = null;
			probe.onerror = null;
			probe.src = '';
			resolve(value);
		};
		probe.preload = 'metadata';
		probe.onloadedmetadata = () => finish(Number.isFinite(probe.duration) ? probe.duration : 0);
		probe.onerror = () => finish(0);
		probe.src = url;
	});
}

export function formatVoiceDuration(seconds) {
	const total = Math.max(0, Math.round(seconds));
	const minutes = Math.floor(total / 60);
	const rest = total % 60;
	return `${minutes}:${String(rest).padStart(2, '0')}`;
}

/**
 * 语音重播胶囊：播放图标 + 装饰波形 + 文案 + 真实时长
 *
 * @param {{audio:string, onReplay:(audio:string)=>void, className?:string, label?:string}} options
 */
export function buildAudioReplay({ audio, onReplay, className = 'audio-replay-btn', label = '重播语音' } = {}) {
	const btn = document.createElement('button');
	btn.type = 'button';
	btn.className = className;
	btn.title = label;
	btn.innerHTML = `
        <span class="voice-icon"><i class="fas fa-play"></i></span>
        <span class="voice-wave" aria-hidden="true">${'<i></i>'.repeat(12)}</span>
        <span class="voice-label"></span>
        <span class="voice-dur" hidden></span>
    `;
	btn.querySelector('.voice-label').textContent = label;
	const durText = btn.querySelector('.voice-dur');
	probeVoiceDuration(audio).then(seconds => {
		if (seconds > 0) {
			durText.textContent = formatVoiceDuration(seconds);
			durText.hidden = false;
		}
	});
	btn.addEventListener('click', () => {
		onReplay?.(audio);
		btn.classList.add('replaying');
		setTimeout(() => btn.classList.remove('replaying'), 600);
	});
	return btn;
}

/**
 * 音乐卡片：后端推送的音频 + 共享播放器
 *
 * 播放器的折叠 / 关闭等桌面端专属行为通过 createOptions 注入，
 * 不在这里判断运行端。
 *
 * @param {object} msg 消息记录
 * @param {{player?:object, createOptions?:object, collapseWhenDismissed?:boolean}} [options]
 * @returns {HTMLElement|null}
 */
export function buildMusicCard(msg = {}, {
	player = globalThis.LunarMusicPlayer,
	createOptions = {},
	collapseWhenDismissed = false
} = {}) {
	if (!player || typeof player.createCard !== 'function') return null;
	const audio = String(msg.musicAudio || msg.audio || '');
	const card = player.createCard({
		title: msg.musicTitle || '月华演奏曲',
		summary: msg.musicSummary || '',
		duration: Number(msg.musicDuration) || 0,
		audio,
		...createOptions
	});
	// 历史消息重渲染时保持上次的收起状态，避免「关了又出现」
	if (collapseWhenDismissed && typeof player.isDismissed === 'function' && player.isDismissed(audio)) {
		card.setCollapsed?.(true);
	}
	return card;
}
