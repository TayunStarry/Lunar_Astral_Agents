// api.js 资源库后端接口封装（只读接口 + 文件管理接口）。

const BASE = '/library/api';

// getJSON 统一 GET 请求。
async function getJSON(url) {
    const res = await fetch(url, { headers: { Accept: 'application/json' } });
    if (!res.ok) {
        throw new Error(await textOfError(res));
    }
    return res.json();
}

// postJSON 统一 POST 请求。
async function postJSON(url, body) {
    const res = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
    });
    const data = await res.json().catch(() => ({ ok: false, error: '响应解析失败' }));
    if (!res.ok || !data.ok) {
        throw new Error(data.error || `请求失败（HTTP ${res.status}）`);
    }
    return data;
}

async function textOfError(res) {
    try {
        const text = (await res.text()).trim();
        return text || `请求失败（HTTP ${res.status}）`;
    } catch {
        return `请求失败（HTTP ${res.status}）`;
    }
}

// fetchTree 递归清单。
export function fetchTree() {
    return getJSON(`${BASE}/tree`);
}

// fetchEdits 文件状态（支持增量游标与分页）。
// options: { since, offset, limit }；不传 limit 时后端返回全部。
export function fetchEdits(options = {}) {
    const query = new URLSearchParams();
    if (options.since) query.set('since', String(options.since));
    if (options.offset) query.set('offset', String(options.offset));
    if (options.limit) query.set('limit', String(options.limit));
    const suffix = query.toString();
    return getJSON(`${BASE}/edits${suffix ? `?${suffix}` : ''}`);
}

// fetchHistory 单文件历史记录分页（before 为上一页返回的游标）。
export function fetchHistory(path, before = 0, limit = 50) {
    const query = new URLSearchParams({ path, limit: String(limit) });
    if (before) query.set('before', String(before));
    return getJSON(`${BASE}/history?${query.toString()}`);
}

// fetchMeta 单文件详情。
export function fetchMeta(path) {
    return getJSON(`${BASE}/meta?path=${encodeURIComponent(path)}`);
}

// renameFile 同目录重命名。
export function renameFile(path, name) {
    return postJSON(`${BASE}/rename`, { path, name });
}

// moveFile 移动到目标目录。
export function moveFile(path, target) {
    return postJSON(`${BASE}/move`, { path, target });
}

// deleteFile 删除文件。
export function deleteFile(path) {
    return postJSON(`${BASE}/delete`, { path });
}

// fileURL 生成内容读取地址（逐段编码，保留路径分隔符）。
export function fileURL(path) {
    return '/file/read/' + path.split('/').map(encodeURIComponent).join('/');
}
