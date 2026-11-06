// app.js 资源库前端主控：状态、渲染、键鼠交互与 WebSocket 实时刷新。

import { fetchTree, fetchEdits, fetchMeta, renameFile, moveFile, deleteFile } from './api.js';
import { buildTree, childrenOf, parentOf, nameOf, humanSize, kindOf, statusClass, isRunnable } from './model.js';
import { renderPreview, renderChanges, renderDetails } from './view.js';
import { toast, confirmDialog, promptDialog, folderDialog, helpDialog } from './dialog.js';
import { runInSandbox, hideSandbox } from './sandbox.js';

const THEME_KEY = 'libraryTheme';
const state = {
    entries: [],
    roots: [],
    edits: new Map(),
    version: 0,
    currentPath: '',
    selectedPath: '',
    expanded: new Set(),
    query: '',
    tab: 'preview',
    meta: new Map(),
    truncated: false,
};

const els = {};

// 窄窗（详情以抽屉形式覆盖显示）判定，与 styles.css 的断点保持一致
const narrowQuery = window.matchMedia('(max-width: 1120px)');

// ===== 初始化 =====
document.addEventListener('DOMContentLoaded', init);

async function init() {
    cacheElements();
    applyTheme(localStorage.getItem(THEME_KEY) === 'dark');
    bindEvents();
    await refreshAll();
    connectSocket();
    await autoRunFromQuery();
}

// autoRunFromQuery 当月华运行浏览器端 js / html 时，资源库窗口会自动弹出并带上
// ?run=<文件路径>；这里定位文件、打开预览并在 iframe 沙箱里执行。
async function autoRunFromQuery() {
    const runPath = new URLSearchParams(location.search).get('run');
    if (!runPath) return;
    const path = runPath.replace(/\\/g, '/');
    const entry = state.entries.find((item) => item.path === path && !item.dir);
    if (!entry) {
        toast(`待运行文件不在资源库中：${path}`, 'error');
        return;
    }
    navigate(parentOf(entry.path));
    selectFile(entry.path);
    closeDetails();
    try {
        await runInSandbox(entry);
        document.getElementById('sandbox-panel').classList.add('expanded');
        toast('已在资源库沙箱中运行', 'success');
    } catch (err) {
        toast(`运行失败：${err.message}`, 'error');
    }
}

function cacheElements() {
    els.tree = document.getElementById('tree');
    els.grid = document.getElementById('view-preview');
    els.changes = document.getElementById('view-changes');
    els.details = document.getElementById('details');
    els.detailsPanel = document.getElementById('details-panel');
    els.detailsBackdrop = document.getElementById('details-backdrop');
    els.crumbs = document.getElementById('crumbs');
    els.search = document.getElementById('search-input');
    els.entryCount = document.getElementById('entry-count');
    els.truncate = document.getElementById('truncate-hint');
    els.tabs = document.getElementById('view-tabs');
    els.summary = document.getElementById('content-summary');
    els.renameBtn = document.getElementById('rename-btn');
    els.moveBtn = document.getElementById('move-btn');
    els.deleteBtn = document.getElementById('delete-btn');
}

function bindEvents() {
    document.getElementById('refresh-btn').addEventListener('click', () => refreshAll(true));
    document.getElementById('theme-btn').addEventListener('click', toggleTheme);
    document.getElementById('help-btn').addEventListener('click', showHelp);
    document.getElementById('back-btn').addEventListener('click', goBack);
    els.renameBtn.addEventListener('click', doRename);
    els.moveBtn.addEventListener('click', doMove);
    els.deleteBtn.addEventListener('click', doDelete);
    document.getElementById('search-clear').addEventListener('click', clearSearch);
    document.getElementById('details-btn').addEventListener('click', toggleDetails);
    document.getElementById('details-close').addEventListener('click', closeDetails);
    document.getElementById('sandbox-close').addEventListener('click', hideSandbox);
    els.detailsBackdrop.addEventListener('click', closeDetails);
    narrowQuery.addEventListener('change', () => {
        if (!isNarrow()) closeDetails();
    });
    els.search.addEventListener('input', () => {
        state.query = els.search.value.trim().toLowerCase();
        document.getElementById('search-clear').classList.toggle('hidden', !state.query);
        if (state.query && state.selectedPath) {
            state.selectedPath = '';
            renderAll();
            return;
        }
        renderGrid();
        renderSummary();
    });
    document.querySelectorAll('.view-tab').forEach((tab) => {
        tab.addEventListener('click', () => {
            switchTab(tab.dataset.tab);
            renderContent();
        });
    });
    document.addEventListener('keydown', onKeyDown);
}

// ===== 数据加载 =====
async function refreshAll(notify = false) {
    try {
        const [tree, edits] = await Promise.all([fetchTree(), loadAllEdits(0)]);
        state.roots = tree.roots || [];
        state.entries = tree.entries || [];
        state.truncated = Boolean(tree.truncated);
        state.edits = new Map((edits.files || []).map((file) => [file.path, file]));
        state.version = edits.version || 0;
        state.meta.clear();
        if (!state.currentPath || !state.roots.includes(state.currentPath.split('/')[0])) {
            state.currentPath = state.roots[0] || '';
        }
        if (state.selectedPath && !state.entries.some((entry) => entry.path === state.selectedPath)) {
            state.selectedPath = '';
        }
        state.roots.forEach((root) => state.expanded.add(root));
        renderAll();
        if (notify) toast('已刷新', 'success');
    } catch (err) {
        toast(`加载失败：${err.message}`, 'error');
    }
}

// loadAllEdits 按页拉取变更状态：since 为 0 时全量，大于 0 时增量。
// 返回 reset=true 表示库已重建或游标失效，调用方应重新做一次全量加载。
async function loadAllEdits(since = 0) {
    const pageSize = 200;
    let offset = 0;
    let version = 0;
    let files = [];
    for (;;) {
        const page = await fetchEdits({ since, offset, limit: pageSize });
        if (page.reset) return { version: page.version || 0, files: [], reset: true };
        const batch = page.files || [];
        // 只锁定第一页的版本：分页期间若有新变更，下一轮同步会从该版本继续补齐。
        if (offset === 0) version = page.version || 0;
        files = files.concat(batch);
        if (!page.has_more || batch.length === 0) break;
        offset += page.limit || pageSize;
    }
    return { version, files, reset: false };
}

// syncEdits 收到同步信号后回库拉取增量；结构变化再补一次清单刷新。
async function syncEdits() {
    if (syncRunning) {
        scheduleSync(100);
        return;
    }
    syncRunning = true;
    try {
        const result = await loadAllEdits(state.version);
        if (result.reset) {
            await refreshAll();
            return;
        }
        if (result.files.length === 0) {
            state.version = Math.max(state.version, result.version);
            return;
        }
        let structureChanged = false;
        result.files.forEach((file) => {
            state.edits.set(file.path, file);
            if (needsTreeRefresh(file)) structureChanged = true;
        });
        state.version = Math.max(state.version, result.version);
        if (structureChanged) {
            await refreshAll();
            return;
        }
        renderGrid();
        renderSummary();
        if (state.selectedPath) {
            renderContent();
            renderDetailsPane();
        }
    } catch (err) {
        toast(`同步变更失败：${err.message}`, 'error');
    } finally {
        syncRunning = false;
    }
}

// ===== 渲染 =====
function renderAll() {
    renderTree();
    renderContent();
    renderDetailsPane();
    updateActions();
    els.entryCount.textContent = String(state.entries.length);
    els.truncate.classList.toggle('hidden', !state.truncated);
    renderCrumbs();
    renderSummary();
}

// 当前目录下的直接子项（并按「文件夹优先 + 名称」排序）
function currentItems() {
    const items = childrenOf(state.entries, state.currentPath);
    return items.sort((a, b) => (a.dir === b.dir
        ? a.name.localeCompare(b.name, 'zh-Hans-CN')
        : a.dir ? -1 : 1));
}

// 搜索过滤后的可见条目
function visibleItems() {
    const items = currentItems();
    return state.query
        ? items.filter((item) => item.name.toLowerCase().includes(state.query))
        : items;
}

// renderSummary 顶部标签页显隐与右侧摘要（目录统计 / 选中文件大小）
function renderSummary() {
    const selected = Boolean(state.selectedPath);
    els.tabs.classList.toggle('hidden', !selected);
    if (selected) {
        const entry = state.entries.find((item) => item.path === state.selectedPath);
        els.summary.textContent = entry
            ? (entry.dir ? '文件夹' : humanSize(entry.size))
            : '';
        return;
    }
    const items = currentItems();
    const visible = state.query
        ? items.filter((item) => item.name.toLowerCase().includes(state.query))
        : items;
    const folders = visible.filter((item) => item.dir).length;
    const files = visible.length - folders;
    els.summary.textContent = state.query
        ? `匹配 ${visible.length} 项`
        : `共 ${visible.length} 项 · ${folders} 个文件夹 · ${files} 个文件`;
}

function renderTree() {
    const tree = buildTree(state.entries.filter((entry) => entry.dir), state.roots);
    els.tree.replaceChildren();
    tree.forEach((node) => els.tree.append(treeRow(node, 0)));
}

function treeRow(node, depth) {
    const wrap = document.createElement('div');
    wrap.className = 'tree-root';
    const row = document.createElement('button');
    row.className = 'tree-node root-node';
    row.style.paddingLeft = `${8 + depth * 14}px`;
    row.dataset.path = node.path;
    const twisty = document.createElement('span');
    twisty.className = 'twisty';
    twisty.innerHTML = node.children.length
        ? `<i class="fas fa-chevron-${state.expanded.has(node.path) ? 'down' : 'right'}"></i>`
        : '';
    const icon = document.createElement('i');
    icon.className = `fas ${state.expanded.has(node.path) ? 'fa-folder-open' : 'fa-folder'} root-icon`;
    const name = document.createElement('span');
    name.className = 'name';
    name.textContent = node.name;
    row.append(twisty, icon, name);
    // 仅当前所在层级做选中标记，祖先节点不再叠加第二个选择框
    if (node.path === state.currentPath) {
        row.classList.add('active');
    }
    row.addEventListener('click', (event) => {
        if (event.target.closest('.twisty')) {
            toggleExpand(node.path);
            return;
        }
        navigate(node.path);
    });
    wrap.append(row);
    if (state.expanded.has(node.path)) {
        node.children.forEach((child) => wrap.append(treeRow(child, depth + 1)));
    }
    return wrap;
}

function toggleExpand(path) {
    if (state.expanded.has(path)) state.expanded.delete(path);
    else state.expanded.add(path);
    renderTree();
}

function renderContent() {
    const showChanges = Boolean(state.selectedPath) && state.tab === 'changes';
    els.grid.classList.toggle('active', !showChanges);
    els.changes.classList.toggle('active', showChanges);
    if (!state.selectedPath) {
        renderGrid();
        return;
    }
    const entry = state.entries.find((item) => item.path === state.selectedPath);
    if (!entry) {
        els.grid.replaceChildren(emptyNode('fa-triangle-exclamation', '文件已不存在'));
        return;
    }
    if (showChanges) {
        renderChanges(els.changes, state.edits.get(entry.path));
    } else {
        renderPreview(els.grid, entry, state.edits.get(entry.path));
    }
}

function renderGrid() {
    els.grid.classList.add('active');
    const visible = visibleItems();
    els.grid.replaceChildren();
    if (visible.length === 0) {
        els.grid.append(emptyNode('fa-folder-open', state.query ? '没有匹配的文件' : '这个目录是空的'));
        return;
    }
    const grid = document.createElement('div');
    grid.className = 'file-grid';
    visible.forEach((entry) => grid.append(fileCard(entry)));
    els.grid.append(grid);
}

function fileCard(entry) {
    const card = document.createElement('button');
    card.className = 'file-card';
    card.dataset.path = entry.path;
    card.title = entry.path;
    if (entry.path === state.selectedPath) card.classList.add('active');
    const icon = document.createElement('span');
    icon.className = `file-card-icon${entry.dir ? ' is-folder' : ''}`;
    icon.innerHTML = `<i class="fas ${iconOf(entry)}"></i>`;
    const meta = document.createElement('span');
    meta.className = 'file-card-meta';
    const name = document.createElement('span');
    name.className = 'file-card-name';
    name.title = entry.name;
    const sub = document.createElement('span');
    sub.className = 'file-card-sub';
    if (entry.dir) {
        sub.append(textSpan('文件夹'));
    } else {
        const fileState = state.edits.get(entry.path);
        const status = statusClass(fileState && fileState.status);
        if (status) {
            name.classList.add(status);
            const dot = document.createElement('span');
            dot.className = `status-dot${status === 'status-ok' ? ' ok' : status === 'status-error' ? ' error' : ''}`;
            sub.append(dot, textSpan(statusLabel(fileState.status)));
        } else {
            sub.append(textSpan(humanSize(entry.size)));
        }
    }
    name.textContent = entry.name;
    meta.append(name, sub);
    card.append(icon, meta);
    card.addEventListener('click', () => (entry.dir ? navigate(entry.path) : selectFile(entry.path)));
    return card;
}

// statusLabel AI 操作状态的可读文案
function statusLabel(status) {
    if (status === 'ok') return '运行正常';
    if (status === 'error') return '运行异常';
    if (status === 'active') return '最近操作';
    return '';
}

function textSpan(text) {
    const span = document.createElement('span');
    span.textContent = text;
    return span;
}

function renderCrumbs() {
    els.crumbs.replaceChildren();
    if (!state.currentPath) return;
    const parts = state.currentPath.split('/');
    parts.forEach((part, index) => {
        const path = parts.slice(0, index + 1).join('/');
        const btn = document.createElement('button');
        btn.className = 'crumb';
        if (index === parts.length - 1) btn.classList.add('current');
        btn.textContent = part;
        btn.addEventListener('click', () => navigate(path));
        els.crumbs.append(btn);
        if (index < parts.length - 1) {
            const sep = document.createElement('span');
            sep.className = 'crumb sep';
            sep.textContent = '/';
            els.crumbs.append(sep);
        }
    });
    if (state.selectedPath) {
        const sep = document.createElement('span');
        sep.className = 'crumb sep';
        sep.textContent = '›';
        const current = document.createElement('span');
        current.className = 'crumb current';
        current.textContent = nameOf(state.selectedPath);
        els.crumbs.append(sep, current);
    }
}

async function renderDetailsPane() {
    if (!state.selectedPath) {
        renderDetails(els.details, null, null, null, null);
        return;
    }
    const entry = state.entries.find((item) => item.path === state.selectedPath);
    if (!entry) return;
    let meta = state.meta.get(entry.path);
    if (!meta) {
        try {
            const res = await fetchMeta(entry.path);
            meta = res.meta;
            state.meta.set(entry.path, meta);
        } catch {
            meta = null;
        }
    }
    renderDetails(els.details, entry, meta, state.edits.get(entry.path), runFile);
}

// ===== 详情抽屉（窄窗为覆盖式，宽窗为常驻栏） =====
function isNarrow() {
    return narrowQuery.matches;
}

function openDetails() {
    if (!isNarrow()) return;
    els.detailsPanel.classList.add('open');
    els.detailsBackdrop.classList.remove('hidden');
}

function closeDetails() {
    els.detailsPanel.classList.remove('open');
    els.detailsBackdrop.classList.add('hidden');
}

function toggleDetails() {
    if (els.detailsPanel.classList.contains('open')) closeDetails();
    else openDetails();
}

// ===== 导航 =====
function navigate(path) {
    state.currentPath = path;
    state.selectedPath = '';
    state.tab = 'preview';
    switchTab('preview');
    closeDetails();
    renderAll();
}

function selectFile(path) {
    state.selectedPath = path;
    state.tab = 'preview';
    switchTab('preview');
    renderAll();
    openDetails();
}

function goBack() {
    if (state.selectedPath) {
        state.selectedPath = '';
        if (isNarrow()) closeDetails();
        renderAll();
        return;
    }
    const parent = parentOf(state.currentPath);
    if (parent) navigate(parent);
}

function switchTab(tab) {
    state.tab = tab;
    document.querySelectorAll('.view-tab').forEach((el) => el.classList.toggle('active', el.dataset.tab === tab));
}

function updateActions() {
    const hasFile = Boolean(state.selectedPath);
    els.renameBtn.disabled = !hasFile;
    els.moveBtn.disabled = !hasFile;
    els.deleteBtn.disabled = !hasFile;
}

// ===== 文件管理 =====
async function doRename() {
    if (!state.selectedPath) return;
    const name = await promptDialog('重命名', '新的文件名', nameOf(state.selectedPath), '重命名');
    if (!name) return;
    try {
        const res = await renameFile(state.selectedPath, name);
        state.selectedPath = res.path;
        await refreshAll();
        toast('已重命名', 'success');
    } catch (err) {
        toast(`重命名失败：${err.message}`, 'error');
    }
}

async function doMove() {
    if (!state.selectedPath) return;
    const dirs = [state.roots, ...state.entries.filter((entry) => entry.dir).map((entry) => entry.path)]
        .reduce((list, item) => list.concat(item), []);
    const target = await folderDialog(dirs, state.currentPath);
    if (!target) return;
    try {
        const res = await moveFile(state.selectedPath, target);
        state.selectedPath = res.path;
        state.currentPath = target;
        await refreshAll();
        toast('已移动', 'success');
    } catch (err) {
        toast(`移动失败：${err.message}`, 'error');
    }
}

async function doDelete() {
    if (!state.selectedPath) return;
    const ok = await confirmDialog('删除文件', `确定删除「${nameOf(state.selectedPath)}」吗？此操作不可撤销。`, '删除');
    if (!ok) return;
    try {
        await deleteFile(state.selectedPath);
        state.selectedPath = '';
        closeDetails();
        await refreshAll();
        toast('已删除', 'success');
    } catch (err) {
        toast(`删除失败：${err.message}`, 'error');
    }
}

async function runFile(entry) {
    try {
        await runInSandbox(entry);
        toast('已在 iframe 沙箱中运行', 'success');
    } catch (err) {
        toast(err.message, 'error');
    }
}

// ===== 键鼠 =====
function onKeyDown(event) {
    if (document.getElementById('modal-root').firstChild) return;
    const typing = ['INPUT', 'TEXTAREA'].includes(document.activeElement && document.activeElement.tagName);
    switch (event.key) {
        case 'F1':
            event.preventDefault();
            showHelp();
            return;
        case 'F5':
            event.preventDefault();
            refreshAll(true);
            return;
        case 'Escape':
            hideSandbox();
            if (isNarrow() && els.detailsPanel.classList.contains('open')) {
                closeDetails();
                return;
            }
            if (!typing) goBack();
            return;
        case 'F2':
            if (!typing) doRename();
            return;
        case 'Delete':
            if (!typing) doDelete();
            return;
        case 'Enter':
            if (!typing) openSelected();
            return;
        default:
            break;
    }
    if (event.ctrlKey && event.key.toLowerCase() === 'f') {
        event.preventDefault();
        els.search.focus();
        return;
    }
    if (event.ctrlKey && event.key.toLowerCase() === 'm' && !typing) {
        event.preventDefault();
        doMove();
        return;
    }
    if (typing) return;
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
        event.preventDefault();
        moveSelection(event.key === 'ArrowDown' ? 1 : -1);
    }
    if (event.key === 'ArrowRight' && state.selectedPath) {
        const entry = state.entries.find((item) => item.path === state.selectedPath);
        if (entry && entry.dir) navigate(entry.path);
    }
    if (event.key === 'ArrowLeft') {
        goBack();
    }
}

function openSelected() {
    if (state.selectedPath) return;
    const cards = els.grid.querySelectorAll('.file-card');
    if (cards.length) cards[0].click();
}

function moveSelection(delta) {
    const cards = [...els.grid.querySelectorAll('.file-card')];
    if (cards.length === 0) return;
    const current = cards.findIndex((card) => card.dataset.path === state.selectedPath);
    const next = Math.max(0, Math.min(cards.length - 1, (current < 0 ? 0 : current) + delta));
    cards[next].click();
}

function clearSearch() {
    els.search.value = '';
    state.query = '';
    document.getElementById('search-clear').classList.add('hidden');
    renderGrid();
    renderSummary();
}

function showHelp() {
    helpDialog([
        ['↑ / ↓', '在当前目录中切换选中文件'],
        ['Enter', '打开选中项（文件夹进入、文件预览）'],
        ['← / Esc', '返回上级或退出预览'],
        ['→', '进入选中的文件夹'],
        ['F2', '重命名选中文件'],
        ['Delete', '删除选中文件'],
        ['Ctrl+M', '移动选中文件'],
        ['Ctrl+F', '定位搜索框'],
        ['F1', '显示本帮助'],
        ['F5', '刷新清单与状态'],
        ['鼠标', '单击选择、双击文件夹进入；Esc 关闭弹窗或沙箱'],
    ]);
}

// ===== WebSocket =====
let socket = null;
let attempts = 0;
let syncTimer = null;
let syncRunning = false;

// 结构类操作会改变目录内容，必须重新拉取清单；内容类操作只更新状态与预览。
const STRUCTURE_OPS = new Set(['create', 'delete', 'move', 'rename', 'copy']);

function connectSocket() {
    const scheme = location.protocol === 'https:' ? 'wss' : 'ws';
    socket = new WebSocket(`${scheme}://${location.host}/ws?channel=library`);
    socket.addEventListener('open', () => {
        attempts = 0;
        // 首次连接与断线重连都回库同步一次，补上断开期间漏掉的变更。
        scheduleSync(0);
    });
    socket.addEventListener('message', onSocketMessage);
    socket.addEventListener('close', () => {
        if (attempts >= 5) return;
        attempts += 1;
        setTimeout(connectSocket, 3000);
    });
}

function onSocketMessage(event) {
    let payload = null;
    try {
        payload = JSON.parse(event.data);
    } catch {
        return;
    }
    if (!payload || !payload.type) return;
    // 后端只发同步信号，完整状态由前端回库拉取。
    if (payload.type === 'library_sync') {
        scheduleSync(150);
    }
}

// needsTreeRefresh 判断一次操作是否可能改变目录结构。
// 结构类操作必然改变清单；write 目标不在当前清单中时同样意味着新建了文件（或其父目录）。
function needsTreeRefresh(file) {
    if (STRUCTURE_OPS.has(file.op)) return true;
    if (file.op !== 'write') return false;
    return !state.entries.some((entry) => entry.path === file.path);
}

// scheduleSync 合并短时间内的多次同步信号，只回库拉取一次。
// 采用节流而不是防抖：持续的操作流不会把刷新无限推迟。
function scheduleSync(delay = 150) {
    if (syncTimer) return;
    syncTimer = setTimeout(() => {
        syncTimer = null;
        void syncEdits();
    }, delay);
}

// ===== 主题与工具 =====
function toggleTheme() {
    const dark = !document.body.classList.contains('dark-mode');
    applyTheme(dark);
    localStorage.setItem(THEME_KEY, dark ? 'dark' : 'light');
}

function applyTheme(dark) {
    document.body.classList.toggle('dark-mode', dark);
}

function iconOf(entry) {
    if (entry.dir) return 'fa-folder';
    switch (kindOf(entry.name)) {
        case 'image': return 'fa-image';
        case 'video': return 'fa-film';
        case 'audio': return 'fa-music';
        case 'text': return isRunnable(entry.name) ? 'fa-code' : 'fa-file-lines';
        default: return 'fa-file';
    }
}

function emptyNode(icon, text) {
    const wrap = document.createElement('div');
    wrap.className = 'empty-state';
    wrap.innerHTML = `<i class="fas ${icon}"></i>`;
    const p = document.createElement('p');
    p.textContent = text;
    wrap.append(p);
    return wrap;
}
