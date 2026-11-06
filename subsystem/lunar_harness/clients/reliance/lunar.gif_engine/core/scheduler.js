// ==== scheduler.js — 随机播放排程 ====
//
// 规则：一张播完（按 GIF 帧延迟求和）→ 随机 1–3 秒 → 下一张；
// 选下一张时排除刚播过的那张（目录只有一张时除外）。
// 外部（嵌入匹配 / action 指令）可直接 play(name) 打断排程，播完仍回到随机序列。

const DEFAULT_GAP_MIN = 1000;
const DEFAULT_GAP_MAX = 3000;
/** 单张加载失败后的重试间隔 */
const RETRY_DELAY = 600;
/** 连续失败达到该次数则停止并上报（避免目录整体不可读时无限重试） */
const MAX_FAILURES = 3;

export class GifScheduler {
    #names;
    #cache;
    #image;
    #onPlay;
    #onError;
    #gapMin;
    #gapMax;
    /** 下一张的定时器 */
    #timer = null;
    /** 播放令牌：外部打断时自增，作废在途的加载结果 */
    #token = 0;
    /** 上一次真正显示过的动画名 */
    #last = '';
    /** 已选定、等待间隔结束的下一张 */
    #pending = '';
    #stopped = true;
    #failures = 0;

    /**
     * @param {object} options
     * @param {string[]} options.names 动画名列表
     * @param {import('./media-cache.js').GifMediaCache} options.cache 字节缓存
     * @param {HTMLImageElement} options.image 显示动图的 img
     * @param {(name: string, media: {duration: number}) => void} [options.onPlay] 开始播放回调
     * @param {(err: Error) => void} [options.onError] 连续失败回调
     * @param {number} [options.gapMin] 间隔下限（毫秒）
     * @param {number} [options.gapMax] 间隔上限（毫秒）
     */
    constructor({ names, cache, image, onPlay, onError, gapMin, gapMax }) {
        this.#names = Array.isArray(names) ? names : [];
        this.#cache = cache;
        this.#image = image;
        this.#onPlay = onPlay;
        this.#onError = onError;
        this.#gapMin = Number.isFinite(gapMin) ? gapMin : DEFAULT_GAP_MIN;
        this.#gapMax = Number.isFinite(gapMax) ? gapMax : DEFAULT_GAP_MAX;
    }

    /** 开始随机播放（首个动画立即显示） */
    start() {
        if (this.#names.length === 0) return;
        this.#stopped = false;
        this.#queue(0);
    }

    /** 停止排程并作废在途加载（切走引擎 / 页面卸载时调用） */
    stop() {
        this.#stopped = true;
        this.#token++;
        this.#pending = '';
        if (this.#timer) {
            clearTimeout(this.#timer);
            this.#timer = null;
        }
    }

    /**
     * 播放指定动画（嵌入匹配 / action 指令）；名称不在库中时返回 false。
     * 播放完成后自动接回随机序列。
     */
    play(name) {
        if (!this.#names.includes(name)) return false;
        this.#stopped = false;
        this.#failures = 0;
        if (this.#timer) {
            clearTimeout(this.#timer);
            this.#timer = null;
        }
        this.#pending = '';
        this.#show(name);
        return true;
    }

    /** 选定下一张并预取，间隔结束后显示 */
    #queue(delay) {
        if (this.#stopped) return;
        if (this.#timer) clearTimeout(this.#timer);
        const next = this.#pick();
        if (!next) return;
        this.#pending = next;
        this.#cache.prefetch(next);
        this.#timer = setTimeout(() => {
            this.#timer = null;
            this.#show(this.#pending);
        }, Math.max(0, delay));
    }

    /** 加载并显示一张动图，随后排下一张 */
    async #show(name) {
        if (!name || this.#stopped) return;
        const token = ++this.#token;
        let media;
        try {
            media = await this.#cache.load(name);
        } catch (err) {
            if (token !== this.#token || this.#stopped) return;
            this.#last = name; // 失败也记为刚播过，避免立刻重复同一张
            this.#failures++;
            if (this.#failures >= MAX_FAILURES) {
                this.stop();
                if (this.#onError) this.#onError(err);
                return;
            }
            this.#queue(RETRY_DELAY);
            return;
        }
        if (token !== this.#token || this.#stopped) return;
        this.#failures = 0;
        this.#last = name;
        this.#pending = '';
        this.#cache.pin(name);
        this.#image.src = media.url;
        if (this.#onPlay) this.#onPlay(name, media);
        const gap = this.#gapMin + Math.random() * (this.#gapMax - this.#gapMin);
        this.#queue(media.duration + gap);
    }

    /** 随机挑一张（排除上一张；只有一张时退化为它本身） */
    #pick() {
        const total = this.#names.length;
        if (total === 0) return '';
        if (total === 1) return this.#names[0];
        for (let i = 0; i < 8; i++) {
            const candidate = this.#names[Math.floor(Math.random() * total)];
            if (candidate !== this.#last) return candidate;
        }
        const index = this.#names.indexOf(this.#last);
        return this.#names[(index + 1) % total];
    }
}
