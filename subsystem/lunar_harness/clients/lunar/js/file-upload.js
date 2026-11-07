// ============================================================
//  星月智能 · 消息终端 — 文件分类 / 上传 / 读取（共享层适配）
//  分类、哈希、保存统一走 /shared/core/files.js，
//  这里保留桌面端专属的句柄文件名缓存。
// ============================================================

// ---------- 文件上传辅助 ----------
function getFileCategory(file) {
    return window.LunarShared.files.getFileCategory(file);
}

// 保存文件到服务器，返回可访问的 fileUrl
async function saveFile(file) {
    const { url } = await window.LunarShared.files.writeHashedFile(file, {
        overwrite: true,
        hashLength: 16
    });
    return `${window.location.origin}${url}`;
}

function fileToRawBase64(file) {
    return window.LunarShared.utils.fileToRawBase64(file);
}

function getAudioFormat(file) {
    return window.LunarShared.files.getAudioFormat(file);
}

// 将字节数格式化为可读的文件大小文本（B/KB/MB/GB）
function formatFileSize(bytes) {
    return window.LunarShared.utils.formatFileSize(bytes);
}

// ---------- 句柄文件名解析 ----------
/** 句柄 → 文件名缓存：历史消息按需向后端解析 */
const resourceNameCache = new Map();

/** 去掉句柄锚点（@D.003|§4 → @D.003），用于缓存键与解析接口 */
function baseResourceHandle(handle) {
    return String(handle || '').trim().replace(/\|\S+$/, '');
}

/** 解析句柄对应文件名：缓存命中直接返回，否则查询资源目录 */
async function resolveResourceName(handle) {
    const key = baseResourceHandle(handle);
    if (!key) return '';
    if (resourceNameCache.has(key)) return resourceNameCache.get(key);
    try {
        const res = await fetch('/file/resource/resolve?handle=' + encodeURIComponent(key));
        if (!res.ok) return '';
        const data = await res.json();
        const resource = data.resource || {};
        const name = String(resource.name || resource.title || '');
        if (name) resourceNameCache.set(key, name);
        return name;
    } catch (e) {
        return '';
    }
}

function readFileAsDataUrl(file) {
    return window.LunarShared.utils.readFileAsDataUrl(file);
}
