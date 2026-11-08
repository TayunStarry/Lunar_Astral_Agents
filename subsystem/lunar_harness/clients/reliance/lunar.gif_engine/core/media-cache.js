// ==== media-cache.js — 动图字节缓存 ====
//
// 只缓存「压缩后的文件字节」并以 Blob URL 交给 <img>：
//   浏览器按需解码当前显示的一张，缓存里不会堆积每一帧的像素数据。
// 容量按设备档位控制，淘汰时吊销 Blob URL；正在显示的条目由外部 pin 住。

import { readGifDuration } from './gif-meta.js';

/** 读不到帧延迟时的兜底时长（APNG / WebP 等） */
const FALLBACK_DURATION = 3000;

/** 扩展名 → MIME（不依赖服务端返回的 Content-Type） */
const MIME_BY_EXT = {
    gif: 'image/gif',
    apng: 'image/apng',
    webp: 'image/webp',
    png: 'image/png',
};

export class GifMediaCache {
    /** 资源基址（含尾部斜杠） */
    #baseUrl;
    /** 名称 → 文件名 */
    #files;
    /** 容量上限（张） */
    #capacity;
    /** name → { url, duration }，Map 的插入顺序即 LRU 顺序 */
    #entries = new Map();
    /** name → Promise，合并并发请求 */
    #inflight = new Map();
    /** 正在显示的动画名（淘汰时跳过） */
    #pinned = '';

    /**
     * @param {string} baseUrl 资源基址
     * @param {Map<string, string>} files 动画名 → 文件名
     * @param {number} capacity 最多缓存的动画张数
     */
    constructor(baseUrl, files, capacity) {
        this.#baseUrl = baseUrl;
        this.#files = files instanceof Map ? files : new Map();
        this.#capacity = Math.max(1, Number(capacity) || 3);
    }

    /** 标记正在显示的动画，避免被 LRU 淘汰吊销 */
    pin(name) {
        this.#pinned = name || '';
    }

    /**
     * 取一张动图（命中缓存直接返回，未命中则下载并解析时长）。
     * @param {string} name 动画名
     * @returns {Promise<{url: string, duration: number}>}
     */
    async load(name) {
        const hit = this.#entries.get(name);
        if (hit) {
            // 命中后移到队尾，维持 LRU 顺序
            this.#entries.delete(name);
            this.#entries.set(name, hit);
            return hit;
        }
        const running = this.#inflight.get(name);
        if (running) return running;
        const task = this.#fetchEntry(name).finally(() => this.#inflight.delete(name));
        this.#inflight.set(name, task);
        return task;
    }

    /** 预取（失败静默）：供播放间隔期提前下载下一张 */
    prefetch(name) {
        if (!name || this.#entries.has(name)) return;
        this.load(name).catch(() => { /* 预取失败由正式播放时重试 */ });
    }

    /** 释放全部 Blob URL 与在途状态（切走引擎时调用） */
    dispose() {
        for (const entry of this.#entries.values()) {
            URL.revokeObjectURL(entry.url);
        }
        this.#entries.clear();
        this.#inflight.clear();
        this.#pinned = '';
    }

    /** 下载并解析一张动图 */
    async #fetchEntry(name) {
        const file = this.#files.get(name);
        if (!file) throw new Error(`动画不存在：${name}`);
        const resp = await fetch(this.#baseUrl + encodeURIComponent(file));
        if (!resp.ok) throw new Error(`动画读取失败：${file}（HTTP ${resp.status}）`);
        // 一次读入字节：解析时长与建 Blob 共用同一份，避免 blob() 后再 arrayBuffer() 整文件回读
        const buffer = await resp.arrayBuffer();
        const ext = file.slice(file.lastIndexOf('.') + 1).toLowerCase();
        const duration = ext === 'gif' ? readGifDuration(new Uint8Array(buffer)) : 0;
        const blob = new Blob([buffer], { type: MIME_BY_EXT[ext] || 'application/octet-stream' });
        const entry = {
            url: URL.createObjectURL(blob),
            duration: duration || FALLBACK_DURATION,
        };
        this.#entries.set(name, entry);
        this.#evict(name);
        return entry;
    }

    /** 超出容量时从最久未用的一端淘汰（保留 pin 住的与刚写入的一张） */
    #evict(latest) {
        while (this.#entries.size > this.#capacity) {
            let victim = '';
            for (const key of this.#entries.keys()) {
                if (key !== this.#pinned && key !== latest) {
                    victim = key;
                    break;
                }
            }
            if (!victim) return; // 全部被占用：宁可超出容量也不吊销正在使用的 URL
            URL.revokeObjectURL(this.#entries.get(victim).url);
            this.#entries.delete(victim);
        }
    }
}
