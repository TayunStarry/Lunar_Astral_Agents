// ==== library.js — 动画库枚举 ====
//
// 动画名 = 文件名（去扩展名）。目录清单直接复用宿主的文件列表接口
// POST /file/list/<相对路径>，与 /file/read/ 同根（local_data），不额外维护清单文件。
// 图像格式按 GIF > APNG > WebP > PNG 取其一；视频格式（WebM）按独占目录枚举。

/** 允许的动图格式（按优先级排列） */
const IMAGE_EXTENSIONS = ['gif', 'apng', 'webp', 'png'];
/** 允许的视频格式 */
const VIDEO_EXTENSIONS = ['webm'];

/** 读不到目录清单时的空结果 */
const EMPTY = [];

/**
 * 规整资源基址（补齐尾部斜杠）。
 * @param {string} raw 形如 /file/read/models/gif/ 或 http 地址
 * @param {string} [fallback] 未指定时使用的默认基址
 * @returns {string}
 */
export function resolveAssetBase(raw, fallback = '/file/read/models/gif/') {
    const base = String(raw || '').trim() || fallback;
    return base.endsWith('/') ? base : `${base}/`;
}

/**
 * 枚举目录下的动画。
 * @param {string} baseUrl 资源基址
 * @param {'image'|'video'} [kind] 资源类型
 * @returns {Promise<Array<{name: string, file: string, ext: string, size: number}>>}
 */
export async function discoverAnimations(baseUrl, kind = 'image') {
    const resp = await fetch(`/file/list/${toListPath(baseUrl)}`, { method: 'POST' });
    if (!resp.ok) throw new Error(`动画目录读取失败：HTTP ${resp.status}`);
    const list = await resp.json();
    const extensions = kind === 'video' ? VIDEO_EXTENSIONS : IMAGE_EXTENSIONS;
    return normalize(Array.isArray(list) ? list : EMPTY, extensions);
}

/** /file/read/xxx/ → xxx（与文件列表接口同一相对路径口径） */
function toListPath(baseUrl) {
    const readPrefix = '/file/read/';
    const absolute = baseUrl.match(/^https?:\/\/[^/]+(\/.*)$/);
    let path = absolute ? absolute[1] : baseUrl;
    if (path.startsWith(readPrefix)) path = path.slice(readPrefix.length);
    return path.replace(/^\/+|\/+$/g, '');
}

/** 过滤目录项、去掉扩展名、同名多格式择优、按名字排序 */
function normalize(list, extensions) {
    const picked = new Map();
    for (const item of list) {
        if (!item || item.isDir) continue;
        const file = String(item.name || '');
        const dot = file.lastIndexOf('.');
        if (dot <= 0) continue;
        const ext = file.slice(dot + 1).toLowerCase();
        const rank = extensions.indexOf(ext);
        if (rank < 0) continue;
        const name = file.slice(0, dot);
        const prev = picked.get(name);
        if (prev && prev.rank <= rank) continue;
        picked.set(name, {
            name,
            file,
            ext,
            rank,
            size: Number(item.size) || 0,
        });
    }
    return [...picked.values()]
        .sort((a, b) => a.name.localeCompare(b.name))
        .map(({ rank, ...entry }) => entry);
}
