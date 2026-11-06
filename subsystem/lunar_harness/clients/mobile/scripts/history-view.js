/**
 * 移动端历史记录视图
 *
 * 数据来源与桌面端一致：思维链写入的 database/messages.json（桌面端 loadPersistedMessages 读取同一文件）。
 * 渲染只做移动端必要的展示层，媒体与播放能力复用共享组件：
 * - 音乐卡：window.LunarMusicPlayer（/reliance/music_player）
 * - 图片预览：window.previewImage（multimedia_preview）
 * - 语音重播：AudioQueue（./tts.js）
 * - Markdown：window.marked（/reliance/marked.min.js）
 */

import { AudioQueue } from './tts.js';

// 历史记录接口（与桌面端 MESSAGE_FILE_PATH 保持一致）
const HISTORY_ENDPOINT = '/file/read/database/messages.json';
// 首屏条数
const PAGE_SIZE = 20;
// 单次会话最多渲染的历史条数：低端手机兜底，避免无限向前加载
const MAX_RENDER = 120;
// 距顶部多远开始加载更早的消息
const LOAD_MORE_THRESHOLD = 48;

/** 去掉群聊/用户前缀与思考块，只保留可展示正文 */
function stripContentPrefixes(text) {
	return String(text || '')
		.replace(/^\[群聊:[^\]]*\]\s*/u, '')
		.replace(/^\[用户:\s*[^\]]*\]\s*[:：]\s*/u, '')
		.replace(/<think>[\s\S]*?<\/think>/gi, '')
		.trim();
}

function formatTime(timestamp) {
	if (!timestamp) return '';
	const date = new Date(timestamp);
	if (Number.isNaN(date.getTime())) return '';
	return date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
}

export class HistoryView {
	constructor({ scrollEl, listEl, statusEl }) {
		this.scrollEl = scrollEl;
		this.listEl = listEl;
		this.statusEl = statusEl;
		/** @type {Array<object>|null} 原始记录（时间正序） */
		this.records = null;
		/** 实时推送到达、可能尚未写入历史文件的消息 */
		this.live = [];
		/** 下一次向前加载的结束下标（不含） */
		this.pointer = 0;
		/** 允许向前加载到的下界（MAX_RENDER 保护） */
		this.floor = 0;
		this.loading = false;
		this.loadError = false;
		this.loadedAt = 0;
		this.rafPending = false;
		this.scrollEl?.addEventListener('scroll', () => this.scheduleScrollCheck(), { passive: true });
	}

	/** 进入历史视图：首次加载并渲染最新一页，之后仅在数据过期时静默刷新 */
	async open() {
		if (!this.records) {
			await this.load();
			if (this.loadError) {
				this.setStatus('历史记录读取失败');
				return;
			}
			this.renderLatest();
			return;
		}
		if (Date.now() - this.loadedAt > 15000) {
			const nearBottom = this.isNearBottom();
			await this.load();
			if (this.loadError) this.setStatus('历史记录读取失败');
			else if (nearBottom) this.renderLatest();
			else this.updateStatus();
		}
	}

	/** 读取历史文件（与桌面端同一接口） */
	async load() {
		this.setStatus('加载中…');
		try {
			const res = await fetch(HISTORY_ENDPOINT, { credentials: 'same-origin' });
			if (!res.ok) throw new Error('HTTP ' + res.status);
			const data = await res.json();
			this.records = Array.isArray(data) ? data.filter(item => item && typeof item === 'object') : [];
			this.loadError = false;
		} catch (error) {
			this.records = [];
			this.loadError = true;
		}
		this.loadedAt = Date.now();
	}

	/** 服务端记录 + 尚未落盘的实时记录（按时间戳去重，避免刷新后重复） */
	allRecords() {
		if (!this.records) return this.live.slice();
		const lastTimestamp = this.records.length
			? Number(this.records[this.records.length - 1].timestamp) || 0
			: 0;
		return this.records.concat(this.live.filter(item => (Number(item.timestamp) || 0) > lastTimestamp));
	}

	/**
	 * 实时消息到达：先进入列表，再交给调用方播报。
	 * 这里不依赖任何音频事件；历史视图未打开时只留在内存，打开时随首屏渲染。
	 */
	appendLive(record) {
		if (!record) return;
		this.live.push(record);
		// 长驻页面兜底：实时记录只保留最近 50 条，避免 base64 音频长期驻留内存
		if (this.live.length > 50) this.live.splice(0, this.live.length - 50);
		if (!this.records || !this.listEl) return;
		const lastTimestamp = this.records.length
			? Number(this.records[this.records.length - 1].timestamp) || 0
			: 0;
		// 已被服务端记录覆盖的消息，重新加载时会再次出现，这里不重复追加
		if ((Number(record.timestamp) || 0) <= lastTimestamp) return;
		const stickToBottom = this.isNearBottom();
		this.listEl.appendChild(this.buildItem(record));
		if (stickToBottom) this.scrollEl.scrollTop = this.scrollEl.scrollHeight;
	}

	/** 渲染最新一页，并滚到底部 */
	renderLatest() {
		if (!this.listEl) return;
		this.listEl.innerHTML = '';
		const all = this.allRecords();
		const total = all.length;
		this.floor = Math.max(0, total - MAX_RENDER);
		this.pointer = Math.max(this.floor, total - PAGE_SIZE);
		if (total === 0) {
			this.setStatus('还没有历史记录');
			return;
		}
		const fragment = document.createDocumentFragment();
		for (let index = this.pointer; index < total; index++) {
			fragment.appendChild(this.buildItem(all[index]));
		}
		this.listEl.appendChild(fragment);
		this.updateStatus();
		requestAnimationFrame(() => {
			this.scrollEl.scrollTop = this.scrollEl.scrollHeight;
		});
	}

	/** 向前追加更早的一页，保持当前阅读位置不跳动 */
	loadMore() {
		if (this.loading || !this.listEl || this.pointer <= this.floor) return;
		this.loading = true;
		this.setStatus('加载中…');
		const start = Math.max(this.floor, this.pointer - PAGE_SIZE);
		const all = this.allRecords();
		const fragment = document.createDocumentFragment();
		for (let index = start; index < this.pointer; index++) {
			fragment.appendChild(this.buildItem(all[index]));
		}
		const previousHeight = this.scrollEl.scrollHeight;
		this.listEl.prepend(fragment);
		this.pointer = start;
		// 追加后补偿滚动距离，避免视口跳到顶部
		this.scrollEl.scrollTop += this.scrollEl.scrollHeight - previousHeight;
		this.loading = false;
		this.updateStatus();
	}

	scheduleScrollCheck() {
		if (this.rafPending) return;
		this.rafPending = true;
		requestAnimationFrame(() => {
			this.rafPending = false;
			if (this.scrollEl.scrollTop <= LOAD_MORE_THRESHOLD) this.loadMore();
		});
	}

	isNearBottom() {
		if (!this.scrollEl) return true;
		const distance = this.scrollEl.scrollHeight - this.scrollEl.scrollTop - this.scrollEl.clientHeight;
		return distance < 32;
	}

	updateStatus() {
		if (this.pointer > this.floor) this.setStatus('上滑加载更早的消息');
		else this.setStatus('没有更早的消息了');
	}

	setStatus(text) {
		if (this.statusEl) this.statusEl.textContent = text || '';
	}

	/** 单条记录：发送者 + 时间 + 正文 + 媒体 + 音乐/语音 */
	buildItem(msg) {
		const role = msg.role === 'user' ? 'user' : 'assistant';
		const item = document.createElement('div');
		item.className = `history-item is-${role}`;

		const meta = document.createElement('div');
		meta.className = 'history-meta';
		const sender = document.createElement('span');
		sender.className = 'history-sender';
		sender.textContent = msg.sender || (role === 'user' ? '我' : '月华');
		meta.appendChild(sender);
		const time = document.createElement('span');
		time.className = 'history-time';
		time.textContent = formatTime(msg.timestamp);
		meta.appendChild(time);
		item.appendChild(meta);

		const hasMusic = Array.isArray(msg.categories) && msg.categories.includes('music');
		const text = stripContentPrefixes(msg.content);

		if (text) item.appendChild(this.buildText(text, role));
		else if (hasMusic) item.appendChild(this.buildText(`为你放一首《${msg.musicTitle || '月华演奏曲'}》`, 'assistant'));

		const media = this.buildMedia(msg);
		if (media) item.appendChild(media);

		if (hasMusic && window.LunarMusicPlayer) {
			item.appendChild(window.LunarMusicPlayer.createCard({
				title: msg.musicTitle || '月华演奏曲',
				summary: msg.musicSummary || '',
				duration: Number(msg.musicDuration) || 0,
				audio: String(msg.musicAudio || msg.audio || '')
			}));
		}

		// 语音重播：音乐消息的 audio 属于演奏音频，已由音乐卡承载
		if (msg.audio && !hasMusic) item.appendChild(this.buildVoice(msg.audio));
		return item;
	}

	buildText(raw, role) {
		const div = document.createElement('div');
		div.className = 'history-text';
		// 用户原文按纯文本展示；助手正文复用桌面端同一份 marked 渲染
		if (role === 'user' || !window.marked) {
			div.classList.add('is-plain');
			div.textContent = raw;
			return div;
		}
		try {
			const html = window.marked.parse(raw);
			if (typeof html === 'string') div.innerHTML = html;
			else {
				div.classList.add('is-plain');
				div.textContent = raw;
			}
		} catch (error) {
			div.classList.add('is-plain');
			div.textContent = raw;
		}
		return div;
	}

	/** 图片 / 视频 / 附件块 */
	buildMedia(msg) {
		const wrap = document.createElement('div');
		wrap.className = 'history-media';
		if (msg.imageSrc) wrap.appendChild(this.buildImage(msg.imageSrc, msg.imageLabel || '图片'));
		if (msg.videoSrc) wrap.appendChild(this.buildVideo(msg.videoSrc));
		(msg.attachments || []).forEach(att => {
			if (!att) return;
			if (att.type === 'image') wrap.appendChild(this.buildImage(att.src, att.label || '图片'));
			else if (att.type === 'video') wrap.appendChild(this.buildVideo(att.src));
			else if (att.type === 'audio') {
				if (att.src) wrap.appendChild(this.buildAudio(att.src));
				else wrap.appendChild(this.buildTranscript(att));
			}
		});
		return wrap.childElementCount ? wrap : null;
	}

	buildImage(src, label) {
		const img = document.createElement('img');
		img.src = src;
		img.alt = label || '图片';
		img.loading = 'lazy';
		img.addEventListener('click', () => {
			if (typeof window.previewImage === 'function') window.previewImage(src, label || '图片');
		});
		return img;
	}

	buildVideo(src) {
		const video = document.createElement('video');
		video.src = src;
		video.controls = true;
		video.playsInline = true;
		video.preload = 'metadata';
		return video;
	}

	buildAudio(src) {
		const audio = document.createElement('audio');
		audio.src = src;
		audio.controls = true;
		audio.preload = 'metadata';
		return audio;
	}

	buildTranscript(att) {
		const box = document.createElement('div');
		box.className = 'history-transcript';
		box.textContent = att.text || '（这段语音没有可用的转写文本）';
		return box;
	}

	buildVoice(audio) {
		const btn = document.createElement('button');
		btn.type = 'button';
		btn.className = 'history-voice';
		btn.title = '重播语音';
		btn.innerHTML = '<i class="fas fa-play"></i><span>重播语音</span>';
		btn.addEventListener('click', () => AudioQueue.enqueue(audio));
		return btn;
	}
}
