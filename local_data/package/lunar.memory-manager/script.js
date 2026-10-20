/**
 * 记忆库管理 - 页面逻辑（Zero-LTP：纯人类操作，无 AtoA 智能体）
 * 新版记忆库：集合是内容自动归入的语义分组，不再是查询表。
 * 职责：
 * - 分组管理：新增 / 删除 / 清空（/memory REST 接口）
 * - 记忆导入：全局导入，入库时自动归组；文本可带标签影响向量，图片按识别取向生成标签
 * - 浏览：按分组浏览，或查看全部条目
 * - 查询：只指定返回类型（文本/图像/音频），跨全部分组检索
 * - 条目管理：全局删除；文本条目改写内容（直改 documents_NNNN.json，不动 embeddings）
 * - 逐个渲染加载：卡片逐个创建渲染，完成一个再开始下一个，保证大数据量下的流畅体验
 */

// ===== DOM 引用 =====
const themeToggle = document.getElementById('themeToggle');
const themeIcon = document.getElementById('themeIcon');
const themeLabel = document.getElementById('themeLabel');
const refreshCollectionsBtn = document.getElementById('refreshCollectionsBtn');
const newCollectionInput = document.getElementById('newCollectionInput');
const addCollectionBtn = document.getElementById('addCollectionBtn');
const collectionList = document.getElementById('collectionList');
const importTarget = document.getElementById('importTarget');
const importToggleGroup = document.getElementById('importToggleGroup');
const textImportForm = document.getElementById('textImportForm');
const imageImportForm = document.getElementById('imageImportForm');
const roleSelect = document.getElementById('roleSelect');
const tagInput = document.getElementById('tagInput');
const textContentInput = document.getElementById('textContentInput');
const importTextBtn = document.getElementById('importTextBtn');
const orientationSelect = document.getElementById('orientationSelect');
const customOrientationRow = document.getElementById('customOrientationRow');
const customOrientationInput = document.getElementById('customOrientationInput');
const importImageBtn = document.getElementById('importImageBtn');
const importImagesBtn = document.getElementById('importImagesBtn');
const importProgress = document.getElementById('importProgress');
const progressFill = document.getElementById('progressFill');
const progressText = document.getElementById('progressText');
const listStats = document.getElementById('listStats');
const clearListBtn = document.getElementById('clearListBtn');
const queryInput = document.getElementById('queryInput');
const queryTypeSelect = document.getElementById('queryTypeSelect');
const topKInput = document.getElementById('topKInput');
const queryBtn = document.getElementById('queryBtn');
const backBrowseBtn = document.getElementById('backBrowseBtn');
const cardGrid = document.getElementById('cardGrid');
const cardEmpty = document.getElementById('cardEmpty');
const loadMoreBtn = document.getElementById('loadMoreBtn');
const listEndHint = document.getElementById('listEndHint');
const editModal = document.getElementById('editModal');
const editMeta = document.getElementById('editMeta');
const editTextWrap = document.getElementById('editTextWrap');
const editTextContent = document.getElementById('editTextContent');
const editCancelBtn = document.getElementById('editCancelBtn');
const editSaveBtn = document.getElementById('editSaveBtn');
const confirmModal = document.getElementById('confirmModal');
const confirmTitle = document.getElementById('confirmTitle');
const confirmMessage = document.getElementById('confirmMessage');
const confirmOkBtn = document.getElementById('confirmOkBtn');
const confirmCancelBtn = document.getElementById('confirmCancelBtn');
const imageFileInput = document.getElementById('imageFileInput');
const imageFilesInput = document.getElementById('imageFilesInput');

// ===== 常量 =====

/** 记忆库数据目录（相对于 local_data） */
const MEMORY_DIR = 'database/memory';
/** 分页每页条数 */
const PAGE_SIZE = 24;
/** 逐个渲染间隔（毫秒） */
const RENDER_STEP_MS = 30;

// ===== 深色/浅色模式切换 =====
const THEME_KEY = 'mmTheme';

function applyTheme(dark) {
    document.body.classList.toggle('dark-mode', dark);
    if (themeIcon) themeIcon.className = 'fas ' + (dark ? 'fa-sun' : 'fa-moon');
    if (themeLabel) themeLabel.textContent = dark ? '浅色' : '深色';
}
if (themeToggle) {
    themeToggle.addEventListener('click', () => {
        const dark = !document.body.classList.contains('dark-mode');
        applyTheme(dark);
        try { localStorage.setItem(THEME_KEY, dark ? 'dark' : 'light'); } catch (e) { }
    });
}
applyTheme(localStorage.getItem(THEME_KEY) === 'dark');

// ===== Toast =====

function showToast(text, kind) {
    const toast = document.getElementById('toast');
    if (!toast) return;
    toast.textContent = text;
    toast.className = 'toast visible' + (kind ? ' ' + kind : '');
    clearTimeout(showToast._timer);
    showToast._timer = setTimeout(() => { toast.className = 'toast'; }, 2600);
}

// ===== 多媒体预览浮层（multimedia_preview 组件） =====

/**
 * 打开多媒体预览浮层（全局 previewImage，由 multimedia_preview/script.js 提供）
 * 组件不可用或调用异常时回退为新窗口打开，保证图片始终可查看
 * @param {string} url - 图片地址（/file/read/... 或 blob: 数据地址）
 * @param {string} name - 展示的文件名
 */
function openPreview(url, name) {
    const fallback = () => { try { window.open(url, '_blank'); } catch (e) { /* 忽略 */ } };
    try {
        if (typeof previewImage === 'function') {
            Promise.resolve(previewImage(url, name)).catch((e) => {
                console.warn('预览浮层打开失败，回退新窗口:', e);
                fallback();
            });
        } else {
            console.warn('预览组件未加载（previewImage 未定义），回退新窗口');
            fallback();
        }
    } catch (e) {
        console.warn('预览浮层调用异常，回退新窗口:', e);
        fallback();
    }
}

/**
 * 图片记忆以 base64（data URL）存储，直接作为预览路径会让浮层信息栏承载超长文本，
 * 故先转成 Blob URL 再交给预览组件
 * @param {string} dataUrl - data:image/...;base64,xxx
 * @returns {string} Blob URL（转换失败返回空串，由调用方回退原始数据）
 */
function base64ToBlobUrl(dataUrl) {
    try {
        const comma = String(dataUrl).indexOf(',');
        if (comma < 0) return '';
        const mime = (dataUrl.slice(0, comma).match(/data:([^;]+)/) || [])[1] || 'image/png';
        const binary = atob(dataUrl.slice(comma + 1));
        const bytes = new Uint8Array(binary.length);
        for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
        return URL.createObjectURL(new Blob([bytes], { type: mime }));
    } catch (e) {
        console.warn('图片数据转换 Blob 失败，回退原始数据:', e);
        return '';
    }
}

// ===== 确认弹窗 =====

let confirmResolver = null;

/** 显示确认弹窗，返回 Promise<boolean> */
function showConfirm(title, message) {
    confirmTitle.textContent = title;
    confirmMessage.textContent = message;
    confirmModal.classList.remove('hidden');
    return new Promise((resolve) => { confirmResolver = resolve; });
}

function settleConfirm(result) {
    confirmModal.classList.add('hidden');
    if (confirmResolver) { confirmResolver(result); confirmResolver = null; }
}

confirmOkBtn.addEventListener('click', () => settleConfirm(true));
confirmCancelBtn.addEventListener('click', () => settleConfirm(false));

// ===== 记忆库 REST 接口封装（响应统一为 {success, data?, error?}） =====

/** 调用 /memory 接口并解包响应 */
async function memoryAPI(path, options) {
    const resp = await fetch('/memory/' + path, options);
    let body = null;
    try { body = await resp.json(); } catch (e) { /* 非 JSON 响应 */ }
    if (!resp.ok || !body || body.success !== true) {
        throw new Error((body && body.error) || ('请求失败（HTTP ' + resp.status + '）'));
    }
    return body.data;
}

/** 列出所有分组 */
async function apiListCollections() {
    const data = await memoryAPI('collections');
    return (data && Array.isArray(data.collections)) ? data.collections : [];
}

/** 创建分组（后端会按分组名生成名称嵌入向量） */
async function apiCreateCollection(name) {
    return await memoryAPI('collections', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: name })
    });
}

/** 删除分组 */
async function apiDeleteCollection(name) {
    return await memoryAPI(encodeURIComponent(name), { method: 'DELETE' });
}

/** 清空分组 */
async function apiClearCollection(name) {
    return await memoryAPI(encodeURIComponent(name) + '/clear', { method: 'POST' });
}

/** 外部改写分片文件后，让后端从磁盘重新加载分组（不重算向量） */
async function apiReloadCollection(name) {
    return await memoryAPI(encodeURIComponent(name) + '/reload', { method: 'POST' });
}

/** 分页获取指定分组内的文档（管理用途） */
async function apiGetDocuments(name, offset, limit) {
    return await memoryAPI(encodeURIComponent(name) + '/documents?offset=' + offset + '&limit=' + limit);
}

/** 分页获取全部记忆条目（跨全部分组） */
async function apiGetEntries(offset, limit) {
    return await memoryAPI('entries?offset=' + offset + '&limit=' + limit);
}

/** 添加记忆（文本/图片/音频统一端点，后端自动归组） */
async function apiAddMessage(payload) {
    return await memoryAPI('messages', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
    });
}

/** 按 ID 删除单条记忆（跨全部分组） */
async function apiDeleteMessage(id) {
    return await memoryAPI('messages', {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: id })
    });
}

/** 按指定返回类型跨全部分组语义查询（后端执行一次嵌入并按余弦相似度检索） */
async function apiQueryMessages(type, query, topK) {
    return await memoryAPI('messages?type=' + encodeURIComponent(type)
        + '&query=' + encodeURIComponent(query) + '&top_k=' + topK);
}

// ===== 文件接口封装（直改分片文件，用于条目内容改写） =====

/** 文件名编码（与琉璃文件接口约定一致：encodeURIComponent 后按字节 base64） */
function encodeFileName(filename) {
    const encodedParams = encodeURIComponent(filename);
    const decodedParams = encodedParams.replace(/%([0-9A-F]{2})/g, (_, p1) => String.fromCharCode(parseInt(p1, 16)));
    return btoa(decodedParams);
}

/** 读取文本文件（GET /file/read） */
async function fetchFileText(path) {
    const resp = await fetch('/file/read/' + path.split('/').map(encodeURIComponent).join('/'), { cache: 'no-store' });
    if (!resp.ok) throw new Error('读取文件失败：' + path + '（HTTP ' + resp.status + '）');
    return await resp.text();
}

/** 写入文本文件（POST /file/write，覆盖写） */
function writeFileToServer(path, content) {
    return new Promise((resolve, reject) => {
        const xhr = new XMLHttpRequest();
        xhr.addEventListener('load', () => {
            if (xhr.status === 200) resolve();
            else reject(new Error(xhr.responseText || '写入失败（HTTP ' + xhr.status + '）'));
        });
        xhr.addEventListener('error', () => reject(new Error('写入失败')));
        xhr.addEventListener('timeout', () => reject(new Error('写入超时')));
        xhr.open('POST', '/file/write');
        xhr.setRequestHeader('X-File-Name', encodeFileName(path));
        xhr.setRequestHeader('X-Overwrite', 'true');
        xhr.send(new Blob([content], { type: 'text/plain; charset=utf-8' }));
    });
}

/** 列出分组内的 documents 分片文件名（documents_NNNN.json） */
async function listSliceFiles(collection) {
    const resp = await fetch('/file/list/' + MEMORY_DIR + '/' + encodeURIComponent(collection), { cache: 'no-store' });
    if (!resp.ok) throw new Error('列出分片文件失败（HTTP ' + resp.status + '）');
    const files = await resp.json();
    return (Array.isArray(files) ? files : [])
        .filter(f => !f.isDir && /^documents_\d+\.json$/i.test(String(f.name)))
        .map(f => String(f.name))
        .sort();
}

/**
 * 改写记忆条目内容（仅修改数据字段，不重建嵌入向量）
 * @param {string} collection - 分组名
 * @param {string} entryId - 条目 ID
 * @param {Object} changes - 要覆盖的字段（仅文本条目，如 { content }）
 */
async function updateEntryContent(collection, entryId, changes) {
    const files = await listSliceFiles(collection);
    for (const file of files) {
        const relPath = MEMORY_DIR + '/' + collection + '/' + file;
        let arr;
        try {
            arr = JSON.parse(await fetchFileText(relPath));
        } catch (e) {
            continue; // 分片损坏时跳过
        }
        if (!Array.isArray(arr)) continue;
        const idx = arr.findIndex(e => e && e.id === entryId);
        if (idx < 0) continue;
        // 仅覆盖指定字段，其余字段（含标签、时间戳等）原样保留
        Object.assign(arr[idx], changes);
        // 紧凑单行 JSON 写回（与记忆库存储格式一致）
        await writeFileToServer(relPath, JSON.stringify(arr));
        // 通知后端从磁盘重载该分组，使改写立即生效（不重建向量）
        try {
            await apiReloadCollection(collection);
        } catch (e) {
            console.warn('分组重载失败（改写可能需重启后生效）:', e);
        }
        return file;
    }
    throw new Error('未在分片文件中找到该记忆条目：' + entryId);
}

// ===== 分组管理 =====

/** 当前浏览的分组；空串表示“全部记忆” */
let currentCollection = '';
let collectionsCache = [];

/** 渲染“全部记忆”入口 */
function renderGlobalItem(totalCount) {
    const el = document.createElement('div');
    el.className = 'collection-item global-item' + (currentCollection === '' ? ' active' : '');
    el.innerHTML = '<span class="collection-icon"><i class="fas fa-globe"></i></span>'
        + '<span class="collection-name">全部记忆</span>'
        + '<span class="collection-count"></span>'
        + '<span class="collection-actions"></span>';
    el.querySelector('.collection-count').textContent = totalCount + ' 条';
    el.title = '跨全部分组浏览全部条目（新版记忆库的查询不再绑定分组）';
    el.addEventListener('click', selectAllMemories);
    collectionList.appendChild(el);
}

/** 刷新分组并渲染列表 */
async function renderCollections() {
    try {
        collectionsCache = await apiListCollections();
        collectionList.innerHTML = '';
        const totalCount = collectionsCache.reduce((sum, c) => sum + (Number(c.count) || 0), 0);
        renderGlobalItem(totalCount);
        if (collectionsCache.length === 0) {
            const empty = document.createElement('div');
            empty.className = 'list-empty';
            empty.textContent = '暂无分组。启动后会自动创建默认分组，也可手动新增。';
            collectionList.appendChild(empty);
            return;
        }
        collectionsCache.forEach(c => {
            const el = document.createElement('div');
            el.className = 'collection-item' + (c.name === currentCollection ? ' active' : '');
            el.innerHTML = '<span class="collection-icon"><i class="fas fa-layer-group"></i></span>'
                + '<span class="collection-name"></span>'
                + '<span class="collection-count"></span>'
                + '<span class="collection-actions">'
                + '<button class="icon-btn warn" data-act="clear" title="清空分组"><i class="fas fa-eraser"></i></button>'
                + '<button class="icon-btn" data-act="delete" title="删除分组"><i class="fas fa-trash"></i></button>'
                + '</span>';
            el.querySelector('.collection-name').textContent = c.name;
            el.querySelector('.collection-count').textContent = (c.count || 0) + ' 条';
            el.title = c.has_name_embedding === false
                ? '分组名称向量缺失，重新初始化记忆库后会自动补齐'
                : '内容按语义自动归入的分组；点击浏览该组条目';
            // 选择分组浏览
            el.addEventListener('click', (ev) => {
                if (ev.target.closest('.icon-btn')) return;
                selectCollection(c.name);
            });
            // 清空分组
            el.querySelector('[data-act="clear"]').addEventListener('click', async () => {
                const ok = await showConfirm('清空分组', '确定清空分组「' + c.name + '」中的所有 ' + (c.count || 0) + ' 条记忆吗？此操作不可恢复。');
                if (!ok) return;
                try {
                    await apiClearCollection(c.name);
                    showToast('分组已清空：' + c.name, 'success');
                    await renderCollections();
                    if (currentCollection === c.name || currentCollection === '') await loadEntries(true);
                } catch (e) { showToast(e.message, 'error'); }
            });
            // 删除分组
            el.querySelector('[data-act="delete"]').addEventListener('click', async () => {
                const ok = await showConfirm('删除分组', '确定删除分组「' + c.name + '」吗？分组目录及其中全部文档与向量将被移除，此操作不可恢复。');
                if (!ok) return;
                try {
                    await apiDeleteCollection(c.name);
                    showToast('分组已删除：' + c.name, 'success');
                    if (currentCollection === c.name) currentCollection = '';
                    await renderCollections();
                    await loadEntries(true);
                } catch (e) { showToast(e.message, 'error'); }
            });
            collectionList.appendChild(el);
        });
    } catch (e) {
        collectionList.innerHTML = '<div class="list-empty">分组加载失败：' + escapeText(e.message) + '</div>';
    }
}

/** 浏览全部记忆（跨全部分组） */
async function selectAllMemories() {
    currentCollection = '';
    importTarget.textContent = '自动归组';
    await renderCollections();
    await loadEntries(true);
}

/** 选择分组浏览 */
async function selectCollection(name) {
    currentCollection = name;
    importTarget.textContent = '自动归组';
    await renderCollections(); // 刷新选中态与条数
    await loadEntries(true);
}

/** 新增分组 */
async function handleAddCollection() {
    const name = newCollectionInput.value.trim();
    if (!name) { showToast('请输入分组名称', 'error'); return; }
    if (['init', 'stats', 'collections', 'messages', 'entries'].includes(name)) {
        showToast('该名称是保留字，不能用作分组名', 'error');
        return;
    }
    try {
        await apiCreateCollection(name);
        showToast('分组已创建：' + name, 'success');
        newCollectionInput.value = '';
        await renderCollections();
    } catch (e) { showToast(e.message, 'error'); }
}

addCollectionBtn.addEventListener('click', handleAddCollection);
newCollectionInput.addEventListener('keydown', (e) => { if (e.key === 'Enter') handleAddCollection(); });
refreshCollectionsBtn.addEventListener('click', async () => {
    await renderCollections();
    showToast('分组列表已刷新', 'success');
});

// ===== 记忆导入 =====

/** 当前导入模式：text | image */
let importMode = 'text';

importToggleGroup.addEventListener('click', (e) => {
    const btn = e.target.closest('.mode-btn');
    if (!btn) return;
    importMode = btn.dataset.mode;
    importToggleGroup.querySelectorAll('.mode-btn').forEach(b => b.classList.toggle('active', b === btn));
    textImportForm.classList.toggle('hidden', importMode !== 'text');
    imageImportForm.classList.toggle('hidden', importMode !== 'image');
});

// 识别取向切换：custom 时显示自定义输入
orientationSelect.addEventListener('change', () => {
    customOrientationRow.classList.toggle('hidden', orientationSelect.value !== 'custom');
});

/** 构造当前导入载荷的公共字段（图片） */
function imagePayload(base64) {
    const payload = {
        type: 'image',
        base64: base64,
        recognition_orientation: orientationSelect.value || 'auto'
    };
    if (payload.recognition_orientation === 'custom') {
        payload.recognition_custom = customOrientationInput.value.trim();
        if (!payload.recognition_custom) {
            throw new Error('自定义取向需要填写参考文本');
        }
    }
    return payload;
}

/** 导入文本记忆（标签只参与嵌入与归组，正文保持干净） */
importTextBtn.addEventListener('click', async () => {
    const content = textContentInput.value.trim();
    if (!content) { showToast('记忆内容不能为空', 'error'); return; }
    const tags = tagInput.value.split(/[,，]/).map(s => s.trim()).filter(Boolean);
    try {
        importTextBtn.disabled = true;
        await apiAddMessage({
            type: 'text',
            role: roleSelect.value,
            content: content,
            tags: tags
        });
        showToast('文本记忆导入成功（已自动归组）', 'success');
        textContentInput.value = '';
        await selectAllMemories();
    } catch (e) {
        showToast(e.message, 'error');
    } finally {
        importTextBtn.disabled = false;
    }
});

/** File 对象 → data URL base64 */
function readImageAsBase64(file) {
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(String(reader.result || ''));
        reader.onerror = () => reject(new Error('读取图片失败：' + file.name));
        reader.readAsDataURL(file);
    });
}

/** 导入单张图片 */
importImageBtn.addEventListener('click', () => {
    imageFileInput.value = '';
    imageFileInput.onchange = async () => {
        const file = imageFileInput.files && imageFileInput.files[0];
        if (!file) return;
        try {
            importImageBtn.disabled = true;
            const base64 = await readImageAsBase64(file);
            await apiAddMessage(imagePayload(base64));
            showToast('图片记忆导入成功（已自动归组）：' + file.name, 'success');
            await selectAllMemories();
        } catch (e) {
            showToast(e.message, 'error');
        } finally {
            importImageBtn.disabled = false;
        }
    };
    imageFileInput.click();
});

/** 批量导入图片（逐张顺序导入，进度条反馈） */
importImagesBtn.addEventListener('click', () => {
    imageFilesInput.value = '';
    imageFilesInput.onchange = async () => {
        const files = Array.from(imageFilesInput.files || []);
        if (files.length === 0) return;
        let ok = 0, fail = 0;
        importProgress.classList.remove('hidden');
        importImagesBtn.disabled = true;
        for (let i = 0; i < files.length; i++) {
            progressFill.style.width = Math.round(i / files.length * 100) + '%';
            progressText.textContent = (i + 1) + ' / ' + files.length;
            try {
                const base64 = await readImageAsBase64(files[i]);
                await apiAddMessage(imagePayload(base64));
                ok++;
            } catch (e) {
                fail++;
                console.warn('导入失败:', files[i].name, e);
            }
        }
        progressFill.style.width = '100%';
        progressText.textContent = files.length + ' / ' + files.length;
        setTimeout(() => importProgress.classList.add('hidden'), 1200);
        importImagesBtn.disabled = false;
        showToast('批量导入完成：成功 ' + ok + ' 张' + (fail ? '，失败 ' + fail + ' 张' : ''), fail ? 'error' : 'success');
        await selectAllMemories();
    };
    imageFilesInput.click();
});

// ===== 记忆条目列表（逐个渲染 + 分页加载） =====

let loadedCount = 0;  // 当前视图已加载条数
let totalCount = 0;   // 当前视图总条数
let rendering = false; // 渲染中标记（防并发）

function clearCardGrid() {
    cardGrid.innerHTML = '<div class="list-empty" id="cardEmpty">选择左侧分组浏览，或直接查询全部记忆。</div>';
    loadedCount = 0;
    totalCount = 0;
    listStats.textContent = '';
    loadMoreBtn.classList.add('hidden');
    listEndHint.textContent = '';
}

function escapeText(text) {
    const div = document.createElement('div');
    div.textContent = String(text);
    return div.innerHTML;
}

/** 等待指定毫秒（逐个渲染节流） */
function sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
}

/** 创建统一尺寸的记忆卡片 */
function createCard(entry) {
    const entryType = entry.type || ((entry.url || entry.base64) ? 'image' : 'text');
    const isImage = entryType === 'image' && !!(entry.url || entry.base64);
    const isAudio = entryType === 'audio' && !!(entry.url || entry.base64);
    const card = document.createElement('div');
    card.className = 'memory-card ' + (isImage ? 'image-card' : (isAudio ? 'audio-card' : 'text-card'));
    card.dataset.id = entry.id || '';

    // 头部：类型图标 + role 徽标 + 改写（仅文本）/删除按钮
    const role = entry.role || entryType;
    const timeText = entry.timestamp
        ? new Date(entry.timestamp * 1000).toLocaleString('zh-CN', { hour12: false })
        : '早前记录';
    const typeIcon = isImage ? 'fa-image' : (isAudio ? 'fa-volume-high' : 'fa-font');
    const canEdit = entryType === 'text' && entry.source !== 'action';
    const editBtn = canEdit
        ? '<button class="icon-btn warn" data-act="edit" title="改写文本内容（不重算向量）"><i class="fas fa-pen"></i></button>'
        : '';
    const head = document.createElement('div');
    head.className = 'card-head';
    head.innerHTML = '<i class="fas ' + typeIcon + ' card-type-icon"></i>'
        + '<span class="card-role"><span class="role-badge"></span></span>'
        + '<span class="card-actions">'
        + editBtn
        + '<button class="icon-btn" data-act="del" title="删除条目"><i class="fas fa-trash"></i></button>'
        + '</span>';
    head.querySelector('.role-badge').textContent = role;
    card.appendChild(head);

    // 主体：图片显示区 / 音频播放区 / 文本显示区（固定高度，超长滚动，保留换行）
    const body = document.createElement('div');
    body.className = 'card-body';
    if (isImage) {
        const img = document.createElement('img');
        img.className = 'card-image';
        img.alt = '图片记忆';
        img.loading = 'lazy';
        img.src = entry.url || entry.base64;
        body.appendChild(img);
        // 点击图片区经多媒体预览浮层缩放查看（头部改写/删除按钮不受影响）
        body.classList.add('card-body-zoomable');
        body.addEventListener('click', () => {
            // 新数据只存 URL；历史数据仍为 base64 data URI
            if (entry.url) {
                openPreview(entry.url, '记忆图片 ' + String(entry.id || '').slice(0, 8));
                return;
            }
            const blobUrl = base64ToBlobUrl(entry.base64);
            openPreview(blobUrl || entry.base64, '记忆图片 ' + String(entry.id || '').slice(0, 8));
            // 延迟回收 Blob（浮层已取到数据后释放，避免内存驻留）
            if (blobUrl) setTimeout(() => URL.revokeObjectURL(blobUrl), 120000);
        });
    } else if (isAudio) {
        const audioWrap = document.createElement('div');
        audioWrap.className = 'card-audio';
        const audio = document.createElement('audio');
        audio.controls = true;
        audio.preload = 'none';
        audio.src = entry.url || entry.base64;
        audioWrap.appendChild(audio);
        if (entry.content) {
            const desc = document.createElement('div');
            desc.className = 'card-audio-desc';
            desc.textContent = entry.content;
            audioWrap.appendChild(desc);
        }
        body.appendChild(audioWrap);
    } else {
        const text = document.createElement('div');
        text.className = 'card-text';
        text.textContent = entry.content || '';
        body.appendChild(text);
    }
    // 匹配度徽标：悬浮于主体左上角（查询结果带有效 similarity 时显示）
    if (typeof entry.similarity === 'number' && entry.similarity > 0) {
        const sim = document.createElement('span');
        const pct = entry.similarity * 100;
        sim.className = 'card-similarity ' + (pct >= 80 ? 'sim-high' : (pct >= 50 ? 'sim-mid' : 'sim-low'));
        sim.textContent = '匹配 ' + pct.toFixed(1) + '%';
        sim.title = '余弦相似度：' + entry.similarity;
        body.appendChild(sim);
    }
    card.appendChild(body);

    // 底部：ID + 所属分组 + 入库时间
    const foot = document.createElement('div');
    foot.className = 'card-foot';
    foot.innerHTML = '<span class="foot-id"></span><span class="foot-time"></span>';
    foot.querySelector('.foot-id').textContent = entry.collection
        ? entry.collection + ' · ' + String(entry.id || '').slice(0, 8)
        : (entry.id || '');
    foot.title = (entry.id || '') + (entry.collection ? ' · 分组：' + entry.collection : '') + (entry.timestamp ? ' · ' + timeText : '');
    foot.querySelector('.foot-time').textContent = timeText;
    card.appendChild(foot);

    // 改写按钮（仅文本条目：新版图片/音频以实体 URL 存储，不支持直改内容）
    const editButton = head.querySelector('[data-act="edit"]');
    if (editButton) {
        editButton.addEventListener('click', () => openEditModal(entry));
    }
    // 删除按钮：全局按 ID 删除，不再依赖当前分组
    head.querySelector('[data-act="del"]').addEventListener('click', async () => {
        const ok = await showConfirm('删除记忆', '确定删除该条记忆吗？\nID: ' + (entry.id || ''));
        if (!ok) return;
        try {
            await apiDeleteMessage(entry.id);
            card.remove();
            loadedCount = Math.max(0, loadedCount - 1);
            totalCount = Math.max(0, totalCount - 1);
            listStats.textContent = '已加载 ' + loadedCount + ' / 共 ' + totalCount + ' 条';
            if (loadedCount === 0) {
                cardGrid.innerHTML = '<div class="list-empty" id="cardEmpty">暂无记忆。</div>';
            }
            showToast('记忆已删除', 'success');
            await renderCollections();
        } catch (e) { showToast(e.message, 'error'); }
    });

    return card;
}

/** 逐个渲染卡片：完成一个再渲染下一个 */
async function renderCardsSequentially(entries) {
    for (const entry of entries) {
        cardGrid.appendChild(createCard(entry));
        await sleep(RENDER_STEP_MS);
    }
}

/** 加载记忆条目（reset=true 时从第一页开始；空 currentCollection 表示全部条目） */
async function loadEntries(reset) {
    if (rendering) return;
    rendering = true;
    loadMoreBtn.disabled = true;
    backBrowseBtn.classList.add('hidden');
    try {
        if (reset) {
            // 保留空态提示直到首批渲染
            cardGrid.innerHTML = '<div class="list-empty" id="cardEmpty">正在加载…</div>';
            loadedCount = 0;
        }
        const empty = cardGrid.querySelector('.list-empty');
        const offset = loadedCount;
        const data = currentCollection
            ? await apiGetDocuments(currentCollection, offset, PAGE_SIZE)
            : await apiGetEntries(offset, PAGE_SIZE);
        const docs = currentCollection
            ? ((data && Array.isArray(data.documents)) ? data.documents : [])
            : ((data && Array.isArray(data.entries)) ? data.entries : []);
        totalCount = (data && typeof data.total === 'number') ? data.total : loadedCount + docs.length;

        if (reset && empty) empty.remove();
        if (reset && docs.length === 0) {
            cardGrid.innerHTML = '<div class="list-empty" id="cardEmpty">'
                + (currentCollection ? '该分组暂无记忆。' : '暂无记忆。') + '</div>';
        } else {
            // 逐个渲染，完成一个再开始下一个
            await renderCardsSequentially(docs);
        }
        loadedCount += docs.length;
        listStats.textContent = (currentCollection ? '分组「' + currentCollection + '」 ' : '全部记忆 ')
            + '已加载 ' + loadedCount + ' / 共 ' + totalCount + ' 条';
        const hasMore = loadedCount < totalCount;
        loadMoreBtn.classList.toggle('hidden', !hasMore);
        listEndHint.textContent = hasMore ? '' : (loadedCount > 0 ? '已全部加载' : '');
    } catch (e) {
        showToast(e.message, 'error');
        const empty = cardGrid.querySelector('.list-empty');
        if (empty) empty.textContent = '加载失败：' + e.message;
    } finally {
        rendering = false;
        loadMoreBtn.disabled = false;
    }
}

loadMoreBtn.addEventListener('click', () => loadEntries(false));
clearListBtn.addEventListener('click', clearCardGrid);

// ===== 语义查询 =====

/** 执行语义查询：输入文本 → 后端跨全部分组嵌入检索 → 展示指定类型、带匹配度的命中结果 */
async function handleQuery() {
    const query = queryInput.value.trim();
    if (!query) { showToast('请输入查询文本', 'error'); return; }
    const type = queryTypeSelect.value || 'text';
    let topK = parseInt(topKInput.value, 10);
    if (!isFinite(topK)) topK = 10;
    topK = Math.max(1, Math.min(100, topK));
    try {
        queryBtn.disabled = true;
        const data = await apiQueryMessages(type, query, topK);
        const results = (data && Array.isArray(data.results)) ? data.results : [];
        // 查询结果占满列表区（退出分页浏览态），移除空态占位避免首格留白
        clearCardGrid();
        const emptyHint = cardGrid.querySelector('.list-empty');
        if (emptyHint) emptyHint.remove();
        if (results.length === 0) {
            cardGrid.innerHTML = '<div class="list-empty" id="cardEmpty">未命中任何记忆。</div>';
        } else {
            await renderCardsSequentially(results);
        }
        loadedCount = results.length;
        totalCount = results.length;
        const typeLabel = { text: '文本', image: '图像', audio: '音频' }[type] || type;
        listStats.textContent = '在全部 ' + collectionsCache.length + ' 个分组中检索 ' + typeLabel
            + '，命中 ' + results.length + ' 条（TopK ' + topK + '）';
        listEndHint.textContent = '查询模式 · 点击「返回浏览」恢复全部浏览';
        backBrowseBtn.classList.remove('hidden');
        showToast('查询完成：命中 ' + results.length + ' 条', 'success');
    } catch (e) {
        showToast(e.message, 'error');
    } finally {
        queryBtn.disabled = false;
    }
}

queryBtn.addEventListener('click', handleQuery);
queryInput.addEventListener('keydown', (e) => { if (e.key === 'Enter') handleQuery(); });

/** 返回分页浏览态 */
backBrowseBtn.addEventListener('click', () => {
    backBrowseBtn.classList.add('hidden');
    loadEntries(true);
});

// ===== 条目改写弹窗（仅文本条目） =====

let pendingEdit = null; // { entry }

function openEditModal(entry) {
    pendingEdit = { entry: entry };
    editMeta.textContent = 'ID: ' + (entry.id || '')
        + (entry.collection ? ' · 分组：' + entry.collection : '')
        + (entry.timestamp ? ' · ' + new Date(entry.timestamp * 1000).toLocaleString('zh-CN', { hour12: false }) : '');
    editTextWrap.classList.remove('hidden');
    editTextContent.value = entry.content || '';
    editModal.classList.remove('hidden');
}

editCancelBtn.addEventListener('click', () => {
    pendingEdit = null;
    editModal.classList.add('hidden');
});

editSaveBtn.addEventListener('click', async () => {
    if (!pendingEdit) return;
    const entry = pendingEdit.entry;
    try {
        editSaveBtn.disabled = true;
        const content = editTextContent.value.trim();
        if (!content) { showToast('内容不能为空', 'error'); return; }
        if (!entry.collection) {
            showToast('缺少条目所属分组，无法定位分片文件', 'error');
            return;
        }
        // 直改分片文件：仅覆盖内容字段，嵌入向量与标签保持不变
        await updateEntryContent(entry.collection, entry.id, { content: content });
        showToast('改写已保存（向量未变更）', 'success');
        pendingEdit = null;
        editModal.classList.add('hidden');
        await loadEntries(true);
    } catch (e) {
        showToast(e.message, 'error');
    } finally {
        editSaveBtn.disabled = false;
    }
});

// ===== 初始化 =====
// 默认进入“全部记忆”视图；分组列表与全局条目一起加载
selectAllMemories();
