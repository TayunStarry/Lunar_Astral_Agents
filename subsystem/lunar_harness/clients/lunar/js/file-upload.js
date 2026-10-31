// ============================================================
//  星月智能 · 消息终端 — 文件分类 / 上传 / 读取
// ============================================================

// ---------- 文件上传辅助 ----------
function getFileCategory(file) {
    if (file.type.startsWith('image/')) return 'image';
    if (file.type.startsWith('video/')) return 'video';
    if (file.type.startsWith('audio/')) return 'audio';
    const ext = file.name.slice(file.name.lastIndexOf('.') + 1).toLowerCase();
    const textExts = ['txt', 'md', 'json', 'xml', 'yaml', 'yml', 'toml', 'csv', 'html', 'htm', 'css', 'js', 'ts', 'jsx', 'tsx', 'go', 'rs', 'java', 'c', 'cpp', 'cxx', 'h', 'hpp', 'cs', 'py', 'rb', 'sh', 'ps1', 'bat', 'log'];
    if (textExts.includes(ext)) return 'text';
    return 'other';
}

async function calculateFileHash(file) {
    return new Promise((resolve) => {
        const reader = new FileReader();
        reader.onload = async (e) => {
            try {
                const arrayBuffer = e.target.result;
                const hashBuffer = await crypto.subtle.digest('SHA-256', arrayBuffer);
                const hashArray = Array.from(new Uint8Array(hashBuffer));
                const fullHash = hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
                resolve(fullHash.substring(0, 16));
            } catch {
                resolve(encodeFilePath(file.name).slice(-16));
            }
        };
        reader.onerror = () => resolve(encodeFilePath(file.name).slice(-16));
        reader.readAsArrayBuffer(file);
    });
}

// 保存文件到服务器，返回可访问的 fileUrl
async function saveFile(file) {
    const category = getFileCategory(file);
    const prefix = (category === 'image' || category === 'video' || category === 'audio') ? 'multimedia/' : 'documents/';
    const fileHash = await calculateFileHash(file);
    const ext = file.name.slice(file.name.lastIndexOf('.')).toLowerCase() || '.bin';
    const newFileName = `${fileHash}${ext}`;
    const res = await fetch('/file/write', {
        method: 'POST',
        headers: {
            'X-File-Name': encodeFilePath(prefix + newFileName),
            'X-Overwrite': 'true'
        },
        body: file
    });
    if (!res.ok) throw new Error('文件上传失败');
    const result = await res.json();
    // 二进制会被服务端归位到 unknown/，读取地址必须用实际落点路径
    const rel = String(result.rel_path || result.filename || '');
    return `${window.location.origin}/file/read/${rel}`;
}

function fileToRawBase64(file) {
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => {
            const result = reader.result;
            if (typeof result !== 'string' || !result.startsWith('data:')) {
                reject(new Error('读取文件失败'));
                return;
            }
            resolve(result.slice(result.indexOf(',') + 1));
        };
        reader.onerror = () => reject(reader.error || new Error('FileReader error'));
        reader.readAsDataURL(file);
    });
}

function getAudioFormat(file) {
    const ext = file.name.slice(file.name.lastIndexOf('.') + 1).toLowerCase();
    // 音频不再落盘：任何格式都以内联音频下发，由服务端 ffmpeg 转码后交给 audio_transcriber
    return ext || 'wav';
}

// 将字节数格式化为可读的文件大小文本（B/KB/MB/GB）
function formatFileSize(bytes) {
    if (!Number.isFinite(bytes) || bytes < 0) return '未知';
    const units = ['B', 'KB', 'MB', 'GB'];
    let value = bytes;
    let unit = 0;
    while (value >= 1024 && unit < units.length - 1) {
        value /= 1024;
        unit++;
    }
    return `${value >= 100 || unit === 0 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`;
}

// ---------- 文档入库与句柄 ----------
/** 句柄 → 文件名缓存：本地上传时即知名字，历史消息按需向后端解析 */
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

/**
 * 上传文档并按原文件名入库，返回 { handle, name }
 *
 * 入库由 Go 侧资源目录完成（同一文件名稳定复用同一句柄），
 * 前端只把规范句柄随消息下发，不再传输正文或文件名引用。
 */
async function admitDocumentFile(file) {
    // 与 Go 侧 safeDocumentName 保持一致的裸文件名规整，避免非法路径字符导致落盘失败
    const fileName = String(file.name || 'file').replace(/[\\/:*?"<>|\u0000-\u001f]/g, '_').trim() || 'file';
    const path = 'documents/' + fileName;
    const saved = await fetch('/file/write', {
        method: 'POST',
        headers: {
            'X-File-Name': encodeFilePath(path),
            'X-Overwrite': 'true'
        },
        body: file
    });
    if (!saved.ok) throw new Error('文件上传失败');
    // 服务端会把不可读二进制归位到 unknown/，入库必须按实际落点路径
    const savedInfo = await saved.json().catch(() => ({}));
    const storedPath = String(savedInfo.rel_path || path);
    const admitted = await fetch('/file/resource/admit', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: storedPath, name: fileName })
    });
    if (!admitted.ok) throw new Error('资源入库失败');
    const data = await admitted.json();
    const resource = data.resource || {};
    const handle = String(resource.handle || '');
    if (!handle) throw new Error('资源入库未返回句柄');
    const name = String(resource.name || fileName);
    resourceNameCache.set(handle, name);
    return { handle: handle, name: name };
}

function readFileAsDataUrl(file) {
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(reader.result);
        reader.onerror = reject;
        reader.readAsDataURL(file);
    });
}
