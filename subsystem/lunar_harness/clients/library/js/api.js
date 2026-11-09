// api.js 资源库后端接口封装（只读接口 + 文件管理接口）。

const BASE = '/library/api';

// getJSON 统一 GET 请求。
async function getJSON(url) {
    const res = await fetch(url, { headers: { Accept: 'application/json' } });
    if (!res.ok) {
        throw new Error(await textOfError(res));
    }
    const data = await res.json();
    if (data && data.ok === false) {
        throw new Error(data.error || `请求失败（HTTP ${res.status}）`);
    }
    return data;
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
        if (!text) return `请求失败（HTTP ${res.status}）`;
        // 后端错误响应多为 {ok:false,error}，优先取可读原因。
        try {
            const data = JSON.parse(text);
            if (data && data.error) return data.error;
        } catch {
            // 非 JSON，按原文返回。
        }
        return text;
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

// fetchDBTables 只读：数据库表 / 视图清单。
export function fetchDBTables(path) {
    return getJSON(`${BASE}/db/tables?path=${encodeURIComponent(path)}`);
}

// fetchDBSchema 只读：单表列结构。
export function fetchDBSchema(path, table) {
    return getJSON(`${BASE}/db/schema?path=${encodeURIComponent(path)}&table=${encodeURIComponent(table)}`);
}

// fetchDBRows 只读：单表分页行数据。
// options: { offset, limit, sort, order }
export function fetchDBRows(path, table, options = {}) {
    const query = new URLSearchParams({ path, table });
    if (options.offset) query.set('offset', String(options.offset));
    if (options.limit) query.set('limit', String(options.limit));
    if (options.sort) query.set('sort', options.sort);
    if (options.order) query.set('order', options.order);
    return getJSON(`${BASE}/db/rows?${query.toString()}`);
}

// runDBQuery 只读：执行一条 SELECT / PRAGMA 查询 / EXPLAIN，返回表格数据。
export function runDBQuery(path, sql) {
    return postJSON(`${BASE}/db/query`, { path, sql });
}

// generateDBSQL 只读：把自然语言需求翻译成一条只读 SQL（不执行）。
export function generateDBSQL(path, question) {
    return postJSON(`${BASE}/db/ai-sql`, { path, question });
}

// fetchArchive 只读：压缩包指定目录的条目分页。
export function fetchArchive(path, dir = '', offset = 0, limit = 200) {
    const query = new URLSearchParams({ path, limit: String(limit) });
    if (dir) query.set('dir', dir);
    if (offset) query.set('offset', String(offset));
    return getJSON(`${BASE}/archive?${query.toString()}`);
}

// resolveHandle 按资源句柄解析资源目录条目（blob_path 相对 LocalDir）。
export async function resolveHandle(handle) {
    const res = await fetch(`/file/resource/resolve?handle=${encodeURIComponent(handle)}`, {
        headers: { Accept: 'application/json' },
    });
    const data = await res.json().catch(() => null);
    if (!res.ok || !data || data.success === false || !data.resource) {
        throw new Error((data && data.error) || `句柄解析失败（HTTP ${res.status}）`);
    }
    return data.resource;
}

// fileURL 生成内容读取地址（逐段编码，保留路径分隔符）。
export function fileURL(path) {
    return '/file/read/' + path.split('/').map(encodeURIComponent).join('/');
}
