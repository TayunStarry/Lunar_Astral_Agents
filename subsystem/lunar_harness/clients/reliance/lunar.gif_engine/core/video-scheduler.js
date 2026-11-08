// ==== video-scheduler.js — WebM 随机播放排程 ====
//
// 与 GifScheduler 同一套规则，但由 <video> 的 ended 事件驱动，不估算时长：
//   播完 → 随机 1–3 秒 → 下一张；选下一张时排除刚播过的那张（只有一张除外）。
//   外部（嵌入匹配 / action 指令）可 play(name) 打断排程，播完仍回到随机序列。
// 内存：全页面只复用一个 <video>，每条只读取一次，不额外预取；
//       不按文件数量创建元素；停止时清空 src 释放解码器。

const DEFAULT_GAP_MIN = 1000;
const DEFAULT_GAP_MAX = 3000;
/** 单条加载失败后的重试间隔 */
const RETRY_DELAY = 600;
/** 连续失败达到该次数则上报（由引擎决定是否回退 GIF 资源） */
const MAX_FAILURES = 3;

export class VideoScheduler {
    #names;
    #baseUrl;
    #files;
    #video;
    #onPlay;
    #onFatal;
    #gapMin;
    #gapMax;
    /** 下一条的起播定时器 */
    #timer = null;
    /** 上一次真正播过的动画名 */
    #last = '';
    #pending = '';
    #stopped = true;
    #failures = 0;
    #bound = false;

    /**
     * @param {object} options
     * @param {string[]} options.names 动画名列表
     * @param {string} options.baseUrl 资源基址（含尾部斜杠）
     * @param {Map<string, string>} options.files 动画名 → 文件名
     * @param {HTMLVideoElement} options.video 复用的播放元素
     * @param {(name: string) => void} [options.onPlay] 开始播放回调
     * @param {(err: Error) => void} [options.onError] 连续失败回调
     * @param {number} [options.gapMin] 间隔下限（毫秒）
     * @param {number} [options.gapMax] 间隔上限（毫秒）
     */
    constructor({ names, baseUrl, files, video, onPlay, onError, gapMin, gapMax }) {
        this.#names = Array.isArray(names) ? names : [];
        this.#baseUrl = baseUrl;
        this.#files = files instanceof Map ? files : new Map();
        this.#video = video;
        this.#onPlay = onPlay;
        this.#onFatal = onError;
        this.#gapMin = Number.isFinite(gapMin) ? gapMin : DEFAULT_GAP_MIN;
        this.#gapMax = Number.isFinite(gapMax) ? gapMax : DEFAULT_GAP_MAX;
    }

    /** 开始随机播放（第一条立即播放） */
    start() {
        if (!this.#names.length || !this.#video) return;
        if (!this.#bound) {
            this.#video.addEventListener('ended', this.#onEnded);
            this.#video.addEventListener('error', this.#onError);
            this.#video.addEventListener('playing', this.#onPlaying);
            this.#bound = true;
        }
        this.#stopped = false;
        this.#queue(0);
    }

    /** 停止排程并释放解码资源（切走引擎 / 页面卸载 / 回退 GIF 时调用） */
    stop() {
        this.#stopped = true;
        this.#pending = '';
        if (this.#timer) {
            clearTimeout(this.#timer);
            this.#timer = null;
        }
        if (this.#video) {
            try { this.#video.pause(); } catch (e) { /* 忽略 */ }
            this.#video.removeAttribute('src');
            this.#video.load();
        }
    }

    /**
     * 播放指定动画（嵌入匹配 / action 指令）；名称不在库中时返回 false。
     * 播放完成后自动接回随机序列。
     */
    play(name) {
        if (!this.#names.includes(name)) return false;
        this.#stopped = false;
        if (this.#timer) {
            clearTimeout(this.#timer);
            this.#timer = null;
        }
        this.#pending = '';
        this.#show(name);
        return true;
    }

    /** 选定下一条，间隔结束后播放 */
    #queue(delay) {
        if (this.#stopped) return;
        if (this.#timer) clearTimeout(this.#timer);
        const next = this.#pick();
        if (!next) return;
        this.#pending = next;
        this.#timer = setTimeout(() => {
            this.#timer = null;
            this.#show(this.#pending);
        }, Math.max(0, delay));
    }

    /** 起播一条：不排下一张，等待 ended 事件 */
    #show(name) {
        if (!name || this.#stopped) return;
        this.#last = name;
        this.#pending = '';
        const url = this.#url(name);
        if (this.#video.getAttribute('src') !== url) this.#video.src = url;
        else this.#video.currentTime = 0;
        this.#video.muted = true;
        const playing = this.#video.play();
        if (playing && typeof playing.catch === 'function') {
            playing.catch(() => { this.#retryPlay(); });
        }
        if (this.#onPlay) this.#onPlay(name);
    }

    /** 自动播放被推迟（页面不可见等）时稍后重试一次，失败交给 error 事件 */
    #retryPlay() {
        setTimeout(() => {
            if (this.#stopped) return;
            const retry = this.#video.play();
            if (retry && typeof retry.catch === 'function') retry.catch(() => {});
        }, 400);
    }

    #onEnded = () => {
        if (this.#stopped) return;
        this.#failures = 0;
        const gap = this.#gapMin + Math.random() * (this.#gapMax - this.#gapMin);
        this.#queue(gap);
    };

    #onPlaying = () => {
        this.#failures = 0;
    };

    #onError = () => {
        if (this.#stopped) return;
        this.#failures++;
        // 记住失败项，避免立刻重试同一条
        this.#last = this.#pending || this.#last;
        if (this.#failures >= MAX_FAILURES) {
            this.stop();
            if (this.#onFatal) this.#onFatal(new Error('WebM 连续播放失败'));
            return;
        }
        this.#queue(RETRY_DELAY);
    };

    /** 随机挑一张（排除上一张；只有一张时退化为它本身） */
    #pick() {
        const total = this.#names.length;
        if (!total) return '';
        if (total === 1) return this.#names[0];
        for (let i = 0; i < 8; i++) {
            const candidate = this.#names[Math.floor(Math.random() * total)];
            if (candidate !== this.#last) return candidate;
        }
        const index = this.#names.indexOf(this.#last);
        return this.#names[(index + 1) % total];
    }

    /** 动画名 → 资源 URL（文件名按 URL 编码） */
    #url(name) {
        const file = this.#files.get(name) || '';
        return this.#baseUrl + encodeURIComponent(file);
    }
}
