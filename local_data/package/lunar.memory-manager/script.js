/**
 * 记忆库管理 - 页面逻辑（Zero-LTP：纯人类操作，无 AtoA 智能体）
 * 职责：
 * - 集合管理：新增 / 删除 / 清空（/memory REST 接口）
 * - 记忆导入：带标签文本与纯文本（标签并入内容参与向量化）、单张与批量图片（识别取向可选）
 * - 条目管理：删除（/memory DELETE）、改写内容（直改 documents_NNNN.json 分片，不动 embeddings，向量保持不变）
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
const editImageWrap = document.getElementById('editImageWrap');
const editImagePreview = document.getElementById('editImagePreview');
const editImagePickBtn = document.getElementById('editImagePickBtn');
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

/** 列出所有集合 */
async function apiListCollections() {
    const data = await memoryAPI('collections');
    return (data && Array.isArray(data.collections)) ? data.collections : [];
}

/** 创建集合 */
async function apiCreateCollection(name) {
    return await memoryAPI(encodeURIComponent(name), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({})
    });
}

/** 删除集合 */
async function apiDeleteCollection(name) {
    return await memoryAPI(encodeURIComponent(name), { method: 'DELETE' });
}

/** 清空集合 */
async function apiClearCollection(name) {
    return await memoryAPI(encodeURIComponent(name) + '/clear', { method: 'POST' });
}

/** 分页获取集合文档 */
async function apiGetDocuments(name, offset, limit) {
    return await memoryAPI(encodeURIComponent(name) + '/documents?offset=' + offset + '&limit=' + limit);
}

/** 添加记忆（文本或图片统一端点） */
async function apiAddMessage(name, payload) {
    return await memoryAPI(encodeURIComponent(name) + '/messages', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
    });
}

/** 删除单条记忆 */
async function apiDeleteMessage(name, id) {
    return await memoryAPI(encodeURIComponent(name) + '/messages', {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: id })
    });
}

/** 语义查询（后端执行一次嵌入并按余弦相似度检索） */
async function apiQueryMessages(name, query, topK) {
    return await memoryAPI(encodeURIComponent(name) + '/messages?query=' + encodeURIComponent(query) + '&top_k=' + topK);
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

/** 列出集合的 documents 分片文件名（documents_NNNN.json） */
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
 * @param {string} collection - 集合名
 * @param {string} entryId - 条目 ID
 * @param {Object} changes - 要覆盖的字段，如 { content } 或 { base64 }
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
        return file;
    }
    throw new Error('未在分片文件中找到该记忆条目：' + entryId);
}

// ===== 集合管理 =====

let currentCollection = '';
let collectionsCache = [];

/** 刷新集合并渲染列表 */
async function renderCollections() {
    try {
        collectionsCache = await apiListCollections();
        collectionList.innerHTML = '';
        if (collectionsCache.length === 0) {
            collectionList.innerHTML = '<div class="list-empty">暂无集合。</div>';
            return;
        }
        collectionsCache.forEach(c => {
            const el = document.createElement('div');
            el.className = 'collection-item' + (c.name === currentCollection ? ' active' : '');
            el.innerHTML = '<span class="collection-icon"><i class="fas fa-database"></i></span>'
                + '<span class="collection-name"></span>'
                + '<span class="collection-count"></span>'
                + '<span class="collection-actions">'
                + '<button class="icon-btn warn" data-act="clear" title="清空集合"><i class="fas fa-eraser"></i></button>'
                + '<button class="icon-btn" data-act="delete" title="删除集合"><i class="fas fa-trash"></i></button>'
                + '</span>';
            el.querySelector('.collection-name').textContent = c.name;
            el.querySelector('.collection-count').textContent = (c.count || 0) + ' 条';
            // 选择集合
            el.addEventListener('click', (ev) => {
                if (ev.target.closest('.icon-btn')) return;
                selectCollection(c.name);
            });
            // 清空集合
            el.querySelector('[data-act="clear"]').addEventListener('click', async () => {
                const ok = await showConfirm('清空集合', '确定清空集合「' + c.name + '」中的所有 ' + (c.count || 0) + ' 条记忆吗？此操作不可恢复。');
                if (!ok) return;
                try {
                    await apiClearCollection(c.name);
                    showToast('集合已清空：' + c.name, 'success');
                    await renderCollections();
                    if (currentCollection === c.name) await loadEntries(true);
                } catch (e) { showToast(e.message, 'error'); }
            });
            // 删除集合
            el.querySelector('[data-act="delete"]').addEventListener('click', async () => {
                const ok = await showConfirm('删除集合', '确定删除集合「' + c.name + '」吗？集合目录及全部文档与向量将被移除，此操作不可恢复。');
                if (!ok) return;
                try {
                    await apiDeleteCollection(c.name);
                    showToast('集合已删除：' + c.name, 'success');
                    if (currentCollection === c.name) {
                        currentCollection = '';
                        importTarget.textContent = '';
                        clearCardGrid();
                    }
                    await renderCollections();
                } catch (e) { showToast(e.message, 'error'); }
            });
            collectionList.appendChild(el);
        });
    } catch (e) {
        collectionList.innerHTML = '<div class="list-empty">集合加载失败：' + escapeText(e.message) + '</div>';
    }
}

/** 选择当前集合并加载条目 */
async function selectCollection(name) {
    currentCollection = name;
    importTarget.textContent = '→ ' + name;
    await renderCollections(); // 刷新选中态与条数
    await loadEntries(true);
}

/** 新增集合 */
async function handleAddCollection() {
    const name = newCollectionInput.value.trim();
    if (!name) { showToast('请输入集合名称', 'error'); return; }
    if (name === 'init' || name === 'stats' || name === 'collections') {
        showToast('该名称是保留字，不能用作集合名', 'error');
        return;
    }
    try {
        await apiCreateCollection(name);
        showToast('集合已创建：' + name, 'success');
        newCollectionInput.value = '';
        await renderCollections();
    } catch (e) { showToast(e.message, 'error'); }
}

addCollectionBtn.addEventListener('click', handleAddCollection);
newCollectionInput.addEventListener('keydown', (e) => { if (e.key === 'Enter') handleAddCollection(); });
refreshCollectionsBtn.addEventListener('click', () => { renderCollections(); showToast('集合列表已刷新', 'success'); });

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

/** 导入文本记忆（带标签时标签并入内容参与向量化） */
importTextBtn.addEventListener('click', async () => {
    if (!currentCollection) { showToast('请先在左侧选择一个集合', 'error'); return; }
    const content = textContentInput.value.trim();
    if (!content) { showToast('记忆内容不能为空', 'error'); return; }
    const tags = tagInput.value.trim();
    const fullContent = tags ? '[标签] ' + tags.split(/[,，]/).map(s => s.trim()).filter(Boolean).join('、') + '\n' + content : content;
    try {
        importTextBtn.disabled = true;
        await apiAddMessage(currentCollection, { role: roleSelect.value, content: fullContent });
        showToast('文本记忆导入成功', 'success');
        textContentInput.value = '';
        await loadEntries(true);
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
    if (!currentCollection) { showToast('请先在左侧选择一个集合', 'error'); return; }
    imageFileInput.value = '';
    imageFileInput.onchange = async () => {
        const file = imageFileInput.files && imageFileInput.files[0];
        if (!file) return;
        try {
            importImageBtn.disabled = true;
            const base64 = await readImageAsBase64(file);
            await apiAddMessage(currentCollection, imagePayload(base64));
            showToast('图片记忆导入成功：' + file.name, 'success');
            await loadEntries(true);
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
    if (!currentCollection) { showToast('请先在左侧选择一个集合', 'error'); return; }
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
                await apiAddMessage(currentCollection, imagePayload(base64));
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
        await loadEntries(true);
    };
    imageFilesInput.click();
});

// ===== 记忆条目列表（逐个渲染 + 分页加载） =====

let loadedCount = 0;  // 已加载条数
let totalCount = 0;   // 集合总条数
let rendering = false; // 渲染中标记（防并发）

function clearCardGrid() {
    cardGrid.innerHTML = '<div class="list-empty" id="cardEmpty">请先在左侧选择一个集合。</div>';
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
    const isImage = !!(entry.base64);
    const card = document.createElement('div');
    card.className = 'memory-card ' + (isImage ? 'image-card' : 'text-card');
    card.dataset.id = entry.id || '';

    // 头部：类型图标 + role 徽标 + 改写/删除按钮
    const role = entry.role || (isImage ? 'image' : '-');
    const timeText = entry.timestamp
        ? new Date(entry.timestamp * 1000).toLocaleString('zh-CN', { hour12: false })
        : '早前记录';
    const head = document.createElement('div');
    head.className = 'card-head';
    head.innerHTML = '<i class="fas ' + (isImage ? 'fa-image' : 'fa-font') + ' card-type-icon"></i>'
        + '<span class="card-role"><span class="role-badge"></span></span>'
        + '<span class="card-actions">'
        + '<button class="icon-btn warn" data-act="edit" title="改写内容"><i class="fas fa-pen"></i></button>'
        + '<button class="icon-btn" data-act="del" title="删除条目"><i class="fas fa-trash"></i></button>'
        + '</span>';
    head.querySelector('.role-badge').textContent = role;
    card.appendChild(head);

    // 主体：图片显示区 / 文本显示区（固定高度，超长滚动，保留换行）
    const body = document.createElement('div');
    body.className = 'card-body';
    if (isImage) {
        const img = document.createElement('img');
        img.className = 'card-image';
        img.alt = '图片记忆';
        img.loading = 'lazy';
        img.src = entry.base64;
        body.appendChild(img);
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

    // 底部：ID + 入库时间
    const foot = document.createElement('div');
    foot.className = 'card-foot';
    foot.innerHTML = '<span class="foot-id"></span><span class="foot-time"></span>';
    foot.querySelector('.foot-id').textContent = entry.id || '';
    foot.title = (entry.id || '') + (entry.timestamp ? ' · ' + timeText : '');
    foot.querySelector('.foot-time').textContent = timeText;
    card.appendChild(foot);

    // 改写按钮
    head.querySelector('[data-act="edit"]').addEventListener('click', () => openEditModal(entry));
    // 删除按钮
    head.querySelector('[data-act="del"]').addEventListener('click', async () => {
        const ok = await showConfirm('删除记忆', '确定删除该条记忆吗？\nID: ' + (entry.id || '')); 
        if (!ok) return;
        try {
            await apiDeleteMessage(currentCollection, entry.id);
            card.remove();
            loadedCount = Math.max(0, loadedCount - 1);
            totalCount = Math.max(0, totalCount - 1);
            listStats.textContent = '已加载 ' + loadedCount + ' / 共 ' + totalCount + ' 条';
            showToast('记忆已删除', 'success');
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

/** 加载记忆条目（reset=true 时从第一页开始） */
async function loadEntries(reset) {
    if (!currentCollection) return;
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
        const data = await apiGetDocuments(currentCollection, offset, PAGE_SIZE);
        const docs = (data && Array.isArray(data.documents)) ? data.documents : [];
        totalCount = (data && typeof data.total === 'number') ? data.total : loadedCount + docs.length;

        if (reset && empty) empty.remove();
        if (reset && docs.length === 0) {
            cardGrid.innerHTML = '<div class="list-empty" id="cardEmpty">该集合暂无记忆。</div>';
        } else {
            // 逐个渲染，完成一个再开始下一个
            await renderCardsSequentially(docs);
        }
        loadedCount += docs.length;
        listStats.textContent = '已加载 ' + loadedCount + ' / 共 ' + totalCount + ' 条';
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

/** 执行语义查询：输入文本 → 后端嵌入一次 → 展示带匹配度的命中结果 */
async function handleQuery() {
    if (!currentCollection) { showToast('请先在左侧选择一个集合', 'error'); return; }
    const query = queryInput.value.trim();
    if (!query) { showToast('请输入查询文本', 'error'); return; }
    let topK = parseInt(topKInput.value, 10);
    if (!isFinite(topK)) topK = 10;
    topK = Math.max(1, Math.min(100, topK));
    try {
        queryBtn.disabled = true;
        const data = await apiQueryMessages(currentCollection, query, topK);
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
        listStats.textContent = '查询命中 ' + results.length + ' 条（TopK ' + topK + '）';
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

// ===== 条目改写弹窗 =====

let pendingEdit = null; // { entry, newBase64 }

function openEditModal(entry) {
    pendingEdit = { entry: entry, newBase64: '' };
    const isImage = !!(entry.base64);
    editMeta.textContent = 'ID: ' + (entry.id || '') + (entry.timestamp ? ' · ' + new Date(entry.timestamp * 1000).toLocaleString('zh-CN', { hour12: false }) : '');
    editTextWrap.classList.toggle('hidden', isImage);
    editImageWrap.classList.toggle('hidden', !isImage);
    if (isImage) {
        editImagePreview.src = entry.base64;
        pendingEdit.newBase64 = '';
    } else {
        editTextContent.value = entry.content || '';
    }
    editModal.classList.remove('hidden');
}

editImagePickBtn.addEventListener('click', () => {
    imageFileInput.value = '';
    imageFileInput.onchange = () => {
        const file = imageFileInput.files && imageFileInput.files[0];
        if (!file || !pendingEdit) return;
        const reader = new FileReader();
        reader.onload = () => {
            pendingEdit.newBase64 = String(reader.result || '');
            editImagePreview.src = pendingEdit.newBase64;
        };
        reader.onerror = () => showToast('读取图片失败', 'error');
        reader.readAsDataURL(file);
    };
    imageFileInput.click();
});

editCancelBtn.addEventListener('click', () => {
    pendingEdit = null;
    editModal.classList.add('hidden');
});

editSaveBtn.addEventListener('click', async () => {
    if (!pendingEdit) return;
    const entry = pendingEdit.entry;
    try {
        editSaveBtn.disabled = true;
        const isImage = !!(entry.base64);
        const changes = {};
        if (isImage) {
            if (!pendingEdit.newBase64) { showToast('未选择新图片', 'error'); return; }
            changes.base64 = pendingEdit.newBase64;
        } else {
            const content = editTextContent.value.trim();
            if (!content) { showToast('内容不能为空', 'error'); return; }
            changes.content = content;
        }
        // 直改分片文件：仅覆盖内容字段，嵌入向量与标签保持不变
        await updateEntryContent(currentCollection, entry.id, changes);
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
renderCollections();
