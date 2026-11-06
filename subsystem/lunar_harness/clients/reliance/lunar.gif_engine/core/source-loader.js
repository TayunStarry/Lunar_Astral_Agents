// ==== source-loader.js — 资源集选择与播放器装配 ====
//
// 首选 WebM（VP9 alpha），WebKit 不渲染 WebM alpha 或 WebM 目录为空时回退 GIF。
// 两套资源的播放器都暴露 start / stop / play(name)，上层无需区分。
// 目录联接：local_data/models/webm → assets/webm
//           local_data/models/gif  → assets/preview（回退，保留旧资源）

import { discoverAnimations, resolveAssetBase } from './library.js';
import { GifMediaCache } from './media-cache.js';
import { GifScheduler } from './scheduler.js';
import { VideoScheduler } from './video-scheduler.js';

const VIDEO_BASE = '/file/read/models/webm/';
const IMAGE_BASE = '/file/read/models/gif/';

/** WebM alpha 能力探测：Safari / iOS 不解 alpha，直接走 GIF */
export function canPlayAlphaWebm() {
    const ua = navigator.userAgent || '';
    const isIOS = /iPad|iPhone|iPod/.test(ua)
        || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1);
    const isSafari = /Safari/i.test(ua) && !/Chrome|Chromium|Edg|OPR|CriOS|FxiOS|Android/i.test(ua);
    if (isIOS || isSafari) return false;
    const probe = document.createElement('video');
    return !!probe.canPlayType('video/webm; codecs="vp9"');
}

/** 解析两套资源基址（?video= / ?assets= 可覆盖） */
export function resolveBases(params) {
    return {
        video: resolveAssetBase(params.get('video'), VIDEO_BASE),
        image: resolveAssetBase(params.get('assets'), IMAGE_BASE),
    };
}

/**
 * 枚举资源并装配播放器。
 * @param {object} options
 * @param {'video'|'image'} options.type 资源类型
 * @param {{video: string, image: string}} options.bases 两套基址
 * @param {HTMLVideoElement} options.video 播放元素
 * @param {HTMLVideoElement} [options.prefetchEl] 预取元素
 * @param {HTMLImageElement} options.image 动图元素
 * @param {number} options.capacity GIF 字节缓存容量
 * @param {(err: Error) => void} options.onFatal WebM 连续失败
 * @param {(err: Error) => void} options.onImageError GIF 加载失败
 * @returns {Promise<{mode: string, library: Array, names: string[], files: Map, cache: GifMediaCache|null, scheduler: object}>}
 */
export async function loadSource({ type, bases, video, prefetchEl, image, capacity, onFatal, onImageError }) {
    const isVideo = type === 'video';
    const library = await discoverAnimations(isVideo ? bases.video : bases.image, isVideo ? 'video' : 'image');
    if (library.length === 0) throw new Error(isVideo ? 'WebM 目录为空' : 'GIF 回退目录为空');
    const names = library.map(item => item.name);
    const files = new Map(library.map(item => [item.name, item.file]));

    if (isVideo) {
        if (image) image.style.display = 'none';
        if (video) video.style.display = 'block';
        const scheduler = new VideoScheduler({
            names,
            baseUrl: bases.video,
            files,
            video,
            prefetchEl,
            onError: onFatal,
        });
        scheduler.start();
        return { mode: 'webm', library, names, files, cache: null, scheduler };
    }

    if (video) {
        video.style.display = 'none';
        video.removeAttribute('src');
    }
    if (image) image.style.display = 'block';
    const cache = new GifMediaCache(bases.image, files, capacity);
    const scheduler = new GifScheduler({
        names,
        cache,
        image,
        onError: onImageError,
    });
    scheduler.start();
    return { mode: 'gif', library, names, files, cache, scheduler };
}
