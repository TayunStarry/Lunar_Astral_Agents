// app.js 资源库前端主控：状态、渲染、键鼠交互与 WebSocket 实时刷新。

import { fetchTree, fetchEdits, fetchMeta, renameFile, moveFile, deleteFile, resolveHandle } from './api.js';
import { childrenOf, parentOf, nameOf, humanSize, kindOf, statusClass, isRunnable, rootLabel } from './model.js';
import { renderPreview, renderChanges, renderDetails } from './view.js';
import {
    isDatabaseFile, openDatabase, renderDatabasePreview, renderDatabasePanel,
    resetDatabase, databasePath, setDatabaseChangeHandler,
} from './dbview.js';
import { isArchiveFile, openArchive, renderArchivePreview, resetArchive } from './archiveview.js';
import { prepareRequirement, submitRequirement, bindRequirementPush, setRequirementObserver } from './requirement.js';
import {
    prepareStoryboard, submitStoryboard, bindStoryboardPush,
    setStoryboardObserver,
    confirmStoryboard, pauseStoryboard, resumeStoryboard, cancelStoryboard,
} from './storyboard.js';
import { connectSocket, onPush, onSocketStatus } from './socket.js';
import { toast, confirmDialog, promptDialog, folderDialog, helpDialog, updateRunStatus, dismissRunStatus, clearRunStatus } from './dialog.js';
import { runInSandbox, hideSandbox } from './sandbox.js';

const THEME_KEY = 'libraryTheme';
const TASK_STATUS_LABELS = {
    queued: '排队中',
    researching: '资料准备',
    planning: '规划中',
    reviewing: '待审查',
    revising: '分镜修订',
    painting: '绘制中',
    writing: '生成文档',
    paused: '已暂停',
    canceled: '已终止',
    executing: '执行中',
    completed: '已完成',
    failed: '失败',
};
const state = {
    entries: [],
    roots: [],
    edits: new Map(),
    version: 0,
    currentPath: '',
    selectedPath: '',
    query: '',
    tab: 'preview',
    detailsTab: 'detail',
    meta: new Map(),
    truncated: false,
};

const els = {};

// 创作产出自动跳转：只对本次会话提交的任务生效，并记录提交时的选中路径作为基线。
const creationBaselines = new Map();
const jumpedTasks = new Set();
// 演绎产出自动跳转：同样只对本次会话提交的任务生效。
const storyboardBaselines = new Map();
const storyboardJumped = new Set();

// 演绎进度：只保留当前任务引用（暂停 / 终止按钮需要任务标识与状态）。
// 阶段与百分比一律写入运行态浮窗，页面内不再出现任何进度文本。
let storyboardActiveTask = null;

// 分镜审查面板：只在任务进入审查态时出现，编辑内容以本地状态为准。
const storyboardReview = {
    taskId: '',
    title: '',
    shots: [],
    busy: false,
    mode: 'read',
    root: null,
    // statuses 只读模式下已同步到徽标的镜头状态，避免每帧重复查找 DOM。
    statuses: [],
};

// 窄窗（详情以抽屉形式覆盖显示）判定，与 styles.css 的断点保持一致
const narrowQuery = window.matchMedia('(max-width: 1120px)');

// ===== 初始化 =====
document.addEventListener('DOMContentLoaded', init);

async function init() {
    cacheElements();
    applyTheme(localStorage.getItem(THEME_KEY) === 'dark');
    bindEvents();
    setRequirementObserver(onRequirementEvent);
    setStoryboardObserver(onStoryboardEvent);
    // 任务推送与连接状态统一由 socket.js 分发，页面不再自己持有 WebSocket。
    bindRequirementPush();
    bindStoryboardPush();
    onPush('library_sync', () => scheduleSync(150));
    onSocketStatus(onRealtimeStatus);
    // 数据库视图的数据变化（选表 / 翻页 / 排序）回写主显示区与右侧标签页。
    setDatabaseChangeHandler(() => {
        const entry = currentEntry();
        if (!entry || !isDatabaseFile(entry)) return;
        if (state.tab === 'preview') renderDatabasePreview(els.grid, entry);
        renderDatabasePanel(els.dbPane, entry);
    });
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
    els.grid = document.getElementById('view-preview');
    els.changes = document.getElementById('view-changes');
    els.details = document.getElementById('details');
    els.detailsPanel = document.getElementById('details-panel');
    els.detailsBackdrop = document.getElementById('details-backdrop');
    els.crumbs = document.getElementById('crumbs');
    els.backBtn = document.getElementById('back-btn');
    els.search = document.getElementById('search-input');
    els.tabs = document.getElementById('view-tabs');
    els.summary = document.getElementById('content-summary');
    els.reqForm = document.getElementById('requirement-form');
    els.reqHandle = document.getElementById('requirement-handle');
    els.reqText = document.getElementById('requirement-text');
    els.reqPick = document.getElementById('requirement-pick');
    els.reqSubmit = document.getElementById('requirement-submit');
    els.storyForm = document.getElementById('storyboard-form');
    els.storyText = document.getElementById('storyboard-text');
    els.storyHandle = document.getElementById('storyboard-handle');
    els.storyPick = document.getElementById('storyboard-pick');
    els.storyTitle = document.getElementById('storyboard-title');
    els.storyStyle = document.getElementById('storyboard-style');
    els.storyRatio = document.getElementById('storyboard-ratio');
    els.storySubmit = document.getElementById('storyboard-submit');
    els.storyPause = document.getElementById('storyboard-pause');
    els.storyCancel = document.getElementById('storyboard-cancel');
    els.storyReviewOpen = document.getElementById('storyboard-review-open');
    els.storyReview = document.getElementById('storyboard-review');
    els.dbTab = document.getElementById('details-tab-database');
    els.dbPane = document.getElementById('database-pane');
}

function bindEvents() {
    document.getElementById('refresh-btn').addEventListener('click', () => refreshAll(true));
    document.getElementById('theme-btn').addEventListener('click', toggleTheme);
    document.getElementById('help-btn').addEventListener('click', showHelp);
    els.backBtn.addEventListener('click', goBack);
    document.getElementById('search-clear').addEventListener('click', clearSearch);
    document.getElementById('details-btn').addEventListener('click', toggleDetails);
    document.getElementById('details-close').addEventListener('click', closeDetails);
    document.getElementById('sandbox-close').addEventListener('click', hideSandbox);
    document.getElementById('run-status-close').addEventListener('click', dismissRunStatus);
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
    document.querySelectorAll('.details-tab').forEach((tab) => {
        tab.addEventListener('click', () => switchDetailsTab(tab.dataset.detailsTab));
    });
    els.reqForm.addEventListener('submit', onSubmitRequirement);
    els.reqPick.addEventListener('click', () => pickRequirementHandle(true));
    els.reqText.addEventListener('keydown', (event) => {
        if (event.key === 'Enter' && !event.shiftKey) {
            event.preventDefault();
            els.reqForm.requestSubmit();
        }
    });
    els.storyForm.addEventListener('submit', onSubmitStoryboard);
    els.storyPick.addEventListener('click', () => pickStoryboardHandle(true));
    els.storyPause.addEventListener('click', onStoryboardPause);
    els.storyCancel.addEventListener('click', onStoryboardCancel);
    els.storyReviewOpen.addEventListener('click', () => openStoryboardReview());
    els.storyText.addEventListener('keydown', (event) => {
        if (event.key === 'Enter' && !event.shiftKey) {
            event.preventDefault();
            els.storyForm.requestSubmit();
        }
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
        // 空路径代表虚拟根：直接展示所有存储根下的句柄根目录。
        // 存储根本身不再作为可停留层级，因此停在其中时回退到虚拟根。
        if (!state.currentPath || state.roots.includes(state.currentPath)
            || !isValidTopLevel(state.currentPath.split('/')[0])) {
            state.currentPath = '';
        }
        if (state.selectedPath && !state.entries.some((entry) => entry.path === state.selectedPath)) {
            state.selectedPath = '';
        }
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
    renderContent();
    updateDatabaseTab();
    renderDetailsPane();
    // 虚拟根且未选中文件时没有上级可回退，禁用返回按钮避免出现无反应层级。
    els.backBtn.disabled = !state.currentPath && !state.selectedPath;
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
        ? `匹配 ${visible.length} 项${state.truncated ? ' · 仅展示前 2 万项' : ''}`
        : `共 ${visible.length} 项 · ${folders} 个文件夹 · ${files} 个文件${state.truncated ? ' · 仅展示前 2 万项' : ''}`;
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
        return;
    }
    if (isDatabaseFile(entry)) {
        renderDatabasePreview(els.grid, entry);
        if (databasePath() !== entry.path) void openDatabase(entry);
        return;
    }
    if (isArchiveFile(entry)) {
        renderArchivePreview(els.grid, entry);
        void openArchive(entry);
        return;
    }
    renderPreview(els.grid, entry, state.edits.get(entry.path));
}

// isValidTopLevel 虚拟根下的首段路径是否可停留：4 个存储根，或 database 这类顶层目录条目。
function isValidTopLevel(segment) {
    return state.roots.includes(segment)
        || state.entries.some((entry) => entry.dir && entry.path === segment);
}

// currentEntry 当前选中的条目（不存在时返回 null）。
function currentEntry() {
    return state.entries.find((item) => item.path === state.selectedPath) || null;
}

// updateDatabaseTab 「数据库」标签只在当前预览为数据库文件时出现；
// 不可用时若正停在该标签，回退到详情页。
function updateDatabaseTab() {
    const entry = currentEntry();
    const available = Boolean(entry && !entry.dir && isDatabaseFile(entry));
    els.dbTab.classList.toggle('hidden', !available);
    if (!available) {
        if (state.detailsTab === 'database') switchDetailsTab('detail');
        return;
    }
    if (state.detailsTab === 'database') renderDatabasePanel(els.dbPane, entry);
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
        sub.append(textSpan(state.currentPath === '' ? rootLabel(entry.root) : '文件夹'));
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
    const home = document.createElement(state.currentPath ? 'button' : 'span');
    home.className = 'crumb';
    home.textContent = '全部资源';
    if (state.currentPath) {
        home.addEventListener('click', () => navigate(''));
    } else {
        home.classList.add('current');
    }
    els.crumbs.append(home);
    // parts[0] 是存储根时从层级中移除；database 是普通顶层目录，需保留自身面包屑。
    const parts = state.currentPath ? state.currentPath.split('/') : [];
    const rootSegment = parts.length > 0 && state.roots.includes(parts[0]);
    const visible = rootSegment ? parts.slice(1) : parts;
    visible.forEach((part, index) => {
        appendCrumbSep('/');
        const path = parts.slice(0, index + (rootSegment ? 2 : 1)).join('/');
        const btn = document.createElement('button');
        btn.className = 'crumb';
        if (index === visible.length - 1) btn.classList.add('current');
        btn.textContent = part;
        btn.addEventListener('click', () => navigate(path));
        els.crumbs.append(btn);
    });
    if (state.selectedPath) {
        appendCrumbSep('›');
        const current = document.createElement('span');
        current.className = 'crumb current';
        current.textContent = nameOf(state.selectedPath);
        els.crumbs.append(current);
    }
}

function appendCrumbSep(text) {
    const sep = document.createElement('span');
    sep.className = 'crumb sep';
    sep.textContent = text;
    els.crumbs.append(sep);
}

async function renderDetailsPane() {
    if (!state.selectedPath) {
        renderDetails(els.details, null, null, null, null);
        return;
    }
    const entry = state.entries.find((item) => item.path === state.selectedPath);
    if (!entry) return;
    // database 等非句柄存储根没有资源元数据，跳过请求避免每次渲染都打一次 4xx。
    const metaEligible = state.roots.includes(entry.path.split('/')[0]);
    let meta = metaEligible ? state.meta.get(entry.path) : null;
    if (metaEligible && !meta) {
        try {
            const res = await fetchMeta(entry.path);
            meta = res.meta;
            state.meta.set(entry.path, meta);
        } catch {
            meta = null;
        }
    }
    renderDetails(els.details, entry, meta, state.edits.get(entry.path), runFile, {
        onRename: doRename,
        onMove: doMove,
        onDelete: doDelete,
    });
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
    // 存储根不再是可停留层级，直接回落到虚拟根。
    state.currentPath = state.roots.includes(path) ? '' : path;
    state.selectedPath = '';
    state.tab = 'preview';
    resetDatabase();
    resetArchive();
    switchTab('preview');
    closeDetails();
    renderAll();
}

function selectFile(path) {
    const next = state.entries.find((item) => item.path === path) || null;
    // 离开数据库 / 压缩包时释放对应状态，避免下一个文件沿用旧的表或目录层级。
    if (!isDatabaseFile(next)) resetDatabase();
    if (!isArchiveFile(next)) resetArchive();
    state.selectedPath = path;
    state.tab = 'preview';
    switchTab('preview');
    renderAll();
    openDetails();
}

function goBack() {
    if (state.selectedPath) {
        state.selectedPath = '';
        resetDatabase();
        resetArchive();
        if (isNarrow()) closeDetails();
        renderAll();
        return;
    }
    if (!state.currentPath) return;
    const parent = parentOf(state.currentPath);
    navigate(!parent || state.roots.includes(parent) ? '' : parent);
}

function switchTab(tab) {
    state.tab = tab;
    document.querySelectorAll('.view-tab').forEach((el) => el.classList.toggle('active', el.dataset.tab === tab));
}

// switchDetailsTab 右侧「详情 / 数据库 / 创作 / 演绎」互斥切换。
// 只改显示状态，不重建内容：选中文件与两条链路的任务跟踪都保持原状。
function switchDetailsTab(tab) {
    const wanted = tab === 'create' || tab === 'database' || tab === 'storyboard' ? tab : 'detail';
    // 数据库标签仅在当前文件可用时允许停留；否则回退详情。
    state.detailsTab = wanted === 'database' && els.dbTab.classList.contains('hidden') ? 'detail' : wanted;
    document.querySelectorAll('.details-tab').forEach((el) => {
        const active = el.dataset.detailsTab === state.detailsTab;
        el.classList.toggle('active', active);
        el.setAttribute('aria-selected', String(active));
    });
    document.querySelectorAll('.details-pane').forEach((el) => {
        el.classList.toggle('active', el.dataset.detailsPane === state.detailsTab);
    });
    if (state.detailsTab === 'database') {
        const entry = currentEntry();
        if (entry) renderDatabasePanel(els.dbPane, entry);
        return;
    }
    if (state.detailsTab === 'create' && !els.reqHandle.value) {
        void pickRequirementHandle(false);
    }
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
    // database 目录不在 4 个句柄存储根内，不作为移动目标，避免必然失败的操作。
    const dirs = [state.roots, ...state.entries
        .filter((entry) => entry.dir && entry.root !== 'database')
        .map((entry) => entry.path)]
        .reduce((list, item) => list.concat(item), []);
    const target = await folderDialog(dirs, state.currentPath);
    if (!target) return;
    try {
        const res = await moveFile(state.selectedPath, target);
        state.selectedPath = res.path;
        state.currentPath = state.roots.includes(target) ? '' : target;
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

// ===== 需求输入 =====
// pickRequirementHandle 把当前选中文件的资源句柄带入输入框。
async function pickRequirementHandle(notify) {
    if (!state.selectedPath) {
        if (notify) toast('请先在主显示区选择一个文件', 'error');
        return;
    }
    const entry = state.entries.find((item) => item.path === state.selectedPath);
    if (!entry || entry.dir) {
        if (notify) toast('目录没有资源句柄', 'error');
        return;
    }
    const known = handleOf(entry);
    if (known) {
        els.reqHandle.value = known;
        toast(`已带入 ${known}`, 'success');
        return;
    }
    toast('正在读取资源句柄…');
    try {
        const res = await fetchMeta(entry.path);
        state.meta.set(entry.path, res.meta);
        const handle = handleOf(entry);
        if (handle) {
            els.reqHandle.value = handle;
            toast(`已带入 ${handle}`, 'success');
        } else {
            toast('该文件尚未登记资源句柄，可手动填写');
        }
    } catch (err) {
        toast(`读取句柄失败，可手动填写：${err.message}`, 'error');
    }
}

function handleOf(entry) {
    const meta = state.meta.get(entry.path);
    const fileState = state.edits.get(entry.path);
    return (meta && meta.handle) || (fileState && fileState.handle) || '';
}

async function onSubmitRequirement(event) {
    event.preventDefault();
    if (els.reqSubmit.disabled) return;
    const handle = els.reqHandle.value.trim();
    const description = els.reqText.value.trim();
    try {
        prepareRequirement(handle, description);
    } catch (err) {
        toast(err.message, 'error');
        return;
    }
    els.reqSubmit.disabled = true;
    updateRunStatus({
        key: 'creation:submit',
        title: '创作任务',
        stage: '正在提交…',
        progress: 0,
        detail: '',
        tone: '',
        terminal: false,
    });
    // 记录提交瞬间的选中项，用于判断任务完成时是否适合自动跳转。
    const baseline = state.selectedPath;
    try {
        const task = await submitRequirement({ handle, description });
        if (task && task.taskId) creationBaselines.set(task.taskId, baseline);
        renderCreationTask(task);
        void maybeJumpToOutput(task);
    } catch (err) {
        els.reqSubmit.disabled = false;
        setRequirementButtonLabel('重试');
        toast(`提交失败：${err.message || err}`, 'error');
        clearRunStatus();
    }
}

// renderCreationTask 把一次创作任务快照写进运行态浮窗。
function renderCreationTask(task) {
    if (!task) return;
    els.reqSubmit.disabled = false;
    setRequirementButtonLabel(task.status === 'failed' ? '重试' : '提交需求');
    updateRunStatus(runStatusPayload(task, '创作任务'));
}

// runStatusPayload 把任务视图映射成浮窗内容：
// 状态与阶段进标题行，失败原因 / 完成结论 / 产出去说明行。
function runStatusPayload(task, title) {
    const label = TASK_STATUS_LABELS[task.status] || task.status || '已受理';
    const stage = String(task.stage || '').trim();
    let detail = '';
    if (task.status === 'failed') {
        detail = task.error || task.summary || '';
    } else if (task.status === 'canceled') {
        detail = task.summary || '';
    } else if (task.status === 'completed') {
        detail = task.summary || '';
        const handles = (task.handles || [])
            .map((item) => (typeof item === 'string' ? item : item && item.handle))
            .filter(Boolean);
        if (handles.length) {
            const produced = `产出 ${handles.join(' ')}`;
            detail = detail ? `${detail} · ${produced}` : produced;
        }
    } else if (task.event === 'long_running' && task.summary) {
        detail = task.summary;
    }
    return {
        key: task.taskId,
        title,
        stage: stage ? `${label} · ${stage}` : label,
        progress: task.progress,
        detail,
        tone: task.status === 'failed' ? 'error' : task.status === 'completed' ? 'success' : '',
        terminal: task.status === 'completed' || task.status === 'failed' || task.status === 'canceled',
    };
}

// onRequirementEvent 接收 requirement.js 的推送/轮询结果。
function onRequirementEvent(event) {
    if (!event || event.kind !== 'task' || !event.task) return;
    try {
        renderCreationTask(event.task);
    } catch (err) {
        console.error('[library] 创作进度渲染失败', err);
    }
    if (event.task.status === 'completed') void maybeJumpToOutput(event.task);
}

// maybeJumpToOutput 创作完成且产出新文件时自动选中并预览。
// 仅在任务属于本次会话提交、且用户仍停留在提交前的选中项时跳转，避免打断浏览。
async function maybeJumpToOutput(task) {
    if (!task || task.status !== 'completed' || !task.taskId) return;
    if (!creationBaselines.has(task.taskId) || jumpedTasks.has(task.taskId)) return;
    const baseline = creationBaselines.get(task.taskId) || '';
    if ((state.selectedPath || '') !== baseline) {
        creationBaselines.delete(task.taskId);
        return;
    }
    const handles = (task.handles || [])
        .map((item) => (typeof item === 'string' ? item : item && item.handle))
        .filter(Boolean);
    if (handles.length === 0) return;
    jumpedTasks.add(task.taskId);
    creationBaselines.delete(task.taskId);
    try {
        const resource = await resolveHandle(handles[0]);
        const target = String((resource && (resource.blob_path || resource.path)) || '').replace(/\\/g, '/');
        if (!target) return;
        if (!state.entries.some((entry) => entry.path === target && !entry.dir)) {
            await refreshAll();
        }
        const entry = state.entries.find((item) => item.path === target && !item.dir);
        if (!entry) return;
        navigate(parentOf(entry.path));
        selectFile(entry.path);
        toast(`已定位创作产出：${entry.name}`, 'success');
    } catch {
        // 句柄解析失败时保持当前浏览位置，不打断用户。
    }
}

function setRequirementButtonLabel(label) {
    const span = els.reqSubmit.querySelector('span');
    if (span) span.textContent = label;
}

// ===== 演绎输入 =====
// pickStoryboardHandle 把当前选中文件的资源句柄带入演绎输入框。
async function pickStoryboardHandle(notify) {
    if (!state.selectedPath) {
        if (notify) toast('请先在主显示区选择一个文件', 'error');
        return;
    }
    const entry = state.entries.find((item) => item.path === state.selectedPath);
    if (!entry || entry.dir) {
        if (notify) toast('目录没有资源句柄', 'error');
        return;
    }
    const known = handleOf(entry);
    if (known) {
        els.storyHandle.value = known;
        toast(`已带入 ${known}`, 'success');
        return;
    }
    toast('正在读取资源句柄…');
    try {
        const res = await fetchMeta(entry.path);
        state.meta.set(entry.path, res.meta);
        const handle = handleOf(entry);
        if (handle) {
            els.storyHandle.value = handle;
            toast(`已带入 ${handle}`, 'success');
        } else {
            toast('该文件尚未登记资源句柄，可手动填写');
        }
    } catch (err) {
        toast(`读取句柄失败，可手动填写：${err.message}`, 'error');
    }
}

async function onSubmitStoryboard(event) {
    event.preventDefault();
    if (els.storySubmit.disabled) return;
    const input = {
        text: els.storyText.value.trim(),
        handle: els.storyHandle.value.trim(),
        title: els.storyTitle.value.trim(),
        style: els.storyStyle.value.trim(),
        ratio: els.storyRatio.value.trim(),
    };
    try {
        prepareStoryboard(input);
    } catch (err) {
        toast(err.message, 'error');
        return;
    }
    els.storySubmit.disabled = true;
    updateRunStatus({
        key: 'storyboard:submit',
        title: '演绎任务',
        stage: '正在提交…',
        progress: 0,
        detail: '',
        tone: '',
        terminal: false,
    });
    // 新任务开始：清掉上一个任务的审查面板。
    storyboardActiveTask = null;
    resetStoryboardReview();
    // 记录提交瞬间的选中项，用于判断任务完成时是否适合自动跳转。
    const baseline = state.selectedPath;
    try {
        const task = await submitStoryboard(input);
        if (task && task.taskId) storyboardBaselines.set(task.taskId, baseline);
        applyStoryboardTask(task);
        void maybeJumpToStoryboardDoc(task);
    } catch (err) {
        els.storySubmit.disabled = false;
        setStoryboardButtonLabel('重试');
        toast(`提交失败：${err.message || err}`, 'error');
        clearRunStatus();
    }
}

// applyStoryboardTask 应用一次任务更新：运行态浮窗 + 控制按钮 + 审查面板。
//
// 这里刻意不向页面插入任何进度文本块：旧实现把阶段、过程文本与逐镜列表
// 全部写进右侧表单，内容一变就撑开／挤压表单，页面排版随之跳动。
// 现在进度只写进常驻浮窗，任何量级的推送都不改变页面结构。
function applyStoryboardTask(task) {
    if (!task || !task.taskId || typeof task.status !== 'string') return;
    if (!els.storySubmit) return;
    const view = { ...task, shots: Array.isArray(task.shots) ? task.shots : [] };
    storyboardActiveTask = view;
    els.storySubmit.disabled = false;
    setStoryboardButtonLabel(view.status === 'failed' ? '重试' : '开始演绎');
    updateRunStatus(runStatusPayload(view, '演绎任务'));
    renderStoryboardControls(view);
    try {
        renderStoryboardReviewTask(view);
    } catch (err) {
        console.error('[library] 分镜审查渲染失败', err);
    }
}

// onStoryboardEvent 推送入口：一帧最多渲染一次，帧内多次推送只保留最新状态。
// 推送合并已经在后端完成（120ms 一次），这里再做一层帧级合并。
let pendingStoryboardTask = null;
let storyboardRenderFrame = 0;

function onStoryboardEvent(event) {
    if (!event || event.kind !== 'task' || !event.task) return;
    pendingStoryboardTask = event.task;
    if (storyboardRenderFrame) return;
    storyboardRenderFrame = requestAnimationFrame(flushStoryboardRender);
}

function flushStoryboardRender() {
    storyboardRenderFrame = 0;
    const task = pendingStoryboardTask;
    pendingStoryboardTask = null;
    if (!task) return;
    // 渲染失败只让这一帧失效：页面其余部分与后续推送照常工作。
    try {
        applyStoryboardTask(task);
    } catch (err) {
        console.error('[library] 演绎进度渲染失败', err);
    }
    if (task.status === 'completed') void maybeJumpToStoryboardDoc(task);
}

// maybeJumpToStoryboardDoc 演绎完成且产出文档时，自动打开分镜文档。
async function maybeJumpToStoryboardDoc(task) {
    if (!task || task.status !== 'completed' || !task.taskId || !task.docPath) return;
    if (!storyboardBaselines.has(task.taskId) || storyboardJumped.has(task.taskId)) return;
    const baseline = storyboardBaselines.get(task.taskId) || '';
    if ((state.selectedPath || '') !== baseline) {
        storyboardBaselines.delete(task.taskId);
        return;
    }
    storyboardJumped.add(task.taskId);
    storyboardBaselines.delete(task.taskId);
    const target = String(task.docPath).replace(/\\/g, '/');
    try {
        if (!state.entries.some((entry) => entry.path === target && !entry.dir)) {
            await refreshAll();
        }
        const entry = state.entries.find((item) => item.path === target && !item.dir);
        if (!entry) return;
        navigate(parentOf(entry.path));
        selectFile(entry.path);
        toast(`已打开分镜文档：${entry.name}`, 'success');
    } catch {
        // 文档尚未进入资源库时保持当前浏览位置，不打断用户。
    }
}

// ===== 演绎任务控制与审查 =====
const REVIEW_STATUS_LABELS = {
    pending: '待确认',
    planned: '待绘制',
    drawing: '绘制中',
    painted: '已完成',
    failed: '失败',
};
let reviewKeySeq = 0;

// renderStoryboardControls 按任务状态切换暂停 / 终止 / 审查入口。
function renderStoryboardControls(task) {
    const status = task.status || '';
    const terminal = status === 'completed' || status === 'failed' || status === 'canceled';
    const canPause = status === 'researching' || status === 'planning' ||
        status === 'revising' || status === 'painting' || status === 'paused';
    els.storyPause.classList.toggle('hidden', !canPause);
    setStoryboardButtonLabel(status === 'paused' ? '继续' : '暂停', els.storyPause);
    els.storyCancel.classList.toggle('hidden', terminal);
    const hasShots = (task.shots || []).length > 0;
    els.storyReviewOpen.classList.toggle('hidden', !hasShots);
    setStoryboardButtonLabel(status === 'reviewing' ? '打开审查' : '查看分镜', els.storyReviewOpen);
}

async function onStoryboardPause() {
    const task = storyboardActiveTask;
    if (!task || !task.taskId) return;
    const paused = task.status === 'paused';
    els.storyPause.disabled = true;
    try {
        if (paused) {
            await resumeStoryboard(task.taskId);
            toast('已继续演绎', 'success');
        } else {
            await pauseStoryboard(task.taskId);
            toast('已暂停，继续时从下一镜接着画', 'success');
        }
    } catch (err) {
        toast(`操作失败：${err.message}`, 'error');
    } finally {
        els.storyPause.disabled = false;
    }
}

async function onStoryboardCancel() {
    const task = storyboardActiveTask;
    if (!task || !task.taskId) return;
    const ok = await confirmDialog(
        '终止演绎',
        '终止后已产出的镜头会保留，未完成的镜头不再继续。确定终止吗？',
        '终止',
    );
    if (!ok) return;
    els.storyCancel.disabled = true;
    try {
        await cancelStoryboard(task.taskId, '用户终止了演绎任务');
        toast('已终止演绎', 'success');
    } catch (err) {
        toast(`终止失败：${err.message}`, 'error');
    } finally {
        els.storyCancel.disabled = false;
    }
}

// renderStoryboardReviewTask 维护主预览区审查面板：审查态可编辑，确认后转为只读进度。
function renderStoryboardReviewTask(task) {
    const shots = task.shots || [];
    const active = task.status === 'reviewing' || task.status === 'revising' ||
        task.status === 'painting' || task.status === 'paused' ||
        task.status === 'writing' || task.status === 'completed' ||
        task.status === 'failed' || task.status === 'canceled';
    if (!active || !shots.length) {
        closeStoryboardReview();
        return;
    }
    const editing = task.status === 'reviewing';
    if (storyboardReview.taskId !== task.taskId) {
        storyboardReview.taskId = task.taskId;
        storyboardReview.title = task.title || '';
        storyboardReview.mode = editing ? 'edit' : 'read';
        storyboardReview.shots = shots.map(toReviewShot);
        buildStoryboardReview();
        if (editing) openStoryboardReview();
        return;
    }
    if (editing && storyboardReview.mode !== 'edit') {
        storyboardReview.mode = 'edit';
        storyboardReview.shots = shots.map(toReviewShot);
        buildStoryboardReview();
        openStoryboardReview();
        return;
    }
    if (!editing && storyboardReview.mode === 'edit') {
        storyboardReview.mode = 'read';
        storyboardReview.shots = shots.map(toReviewShot);
        buildStoryboardReview();
        return;
    }
    if (!editing) updateReviewBadges(shots);
}

// toReviewShot 把服务端镜头转成审查面板的本地可编辑结构。
function toReviewShot(shot) {
    reviewKeySeq += 1;
    return {
        key: `shot-${reviewKeySeq}`,
        index: shot.index || 0,
        act: shot.act || '',
        scene: shot.scene || '',
        narration: shot.narration || '',
        prompt: shot.prompt || '',
        ratio: shot.ratio || '',
        status: shot.status || 'pending',
    };
}

function openStoryboardReview() {
    if (!storyboardReview.taskId || storyboardReview.shots.length === 0) return;
    els.storyReview.classList.remove('hidden');
}

function closeStoryboardReview() {
    els.storyReview.classList.add('hidden');
}

function resetStoryboardReview() {
    storyboardReview.taskId = '';
    storyboardReview.title = '';
    storyboardReview.shots = [];
    storyboardReview.busy = false;
    storyboardReview.mode = 'read';
    storyboardReview.statuses = [];
    closeStoryboardReview();
}

// buildStoryboardReview 重建审查面板 DOM（仅在结构变化或模式切换时调用）。
function buildStoryboardReview() {
    const root = els.storyReview;
    root.replaceChildren();
    const editing = storyboardReview.mode === 'edit';
    const header = document.createElement('div');
    header.className = 'sb-review-head';
    const heading = document.createElement('div');
    heading.className = 'sb-review-title';
    heading.append(makeIcon('fas fa-list-check'));
    heading.append(document.createTextNode(` 分镜审查（${storyboardReview.shots.length} 镜）`));
    const hint = document.createElement('span');
    hint.className = 'sb-review-hint';
    hint.textContent = editing ? '确认前不会开始绘制，可直接改写、删除或新增分镜' : '绘制进度';
    const close = document.createElement('button');
    close.type = 'button';
    close.className = 'icon-btn';
    close.dataset.action = 'close';
    close.setAttribute('aria-label', '关闭审查');
    close.append(makeIcon('fas fa-xmark'));
    close.addEventListener('click', closeStoryboardReview);
    header.append(heading, hint, close);

    const list = document.createElement('div');
    list.className = 'sb-review-list';
    storyboardReview.statuses = [];
    storyboardReview.shots.forEach((shot, index) => {
        storyboardReview.statuses.push(shot.status || 'pending');
        list.append(buildReviewShotCard(shot, index, editing));
    });
    list.addEventListener('input', onReviewShotInput);
    list.addEventListener('click', onReviewShotClick);

    root.append(header, list);

    if (editing) {
        const foot = document.createElement('div');
        foot.className = 'sb-review-foot';
        const add = document.createElement('button');
        add.type = 'button';
        add.className = 'btn-glass btn-sm';
        add.dataset.action = 'add';
        add.append(makeIcon('fas fa-plus'));
        add.append(document.createTextNode(' 新增分镜'));
        add.addEventListener('click', onReviewAddShot);
        const count = document.createElement('span');
        count.className = 'sb-review-count';
        count.textContent = `共 ${storyboardReview.shots.length} 镜`;
        const confirm = document.createElement('button');
        confirm.type = 'button';
        confirm.className = 'btn-glass primary btn-sm';
        confirm.dataset.action = 'confirm';
        confirm.append(makeIcon('fas fa-play'));
        confirm.append(document.createTextNode(' 确认并开始绘制'));
        confirm.addEventListener('click', onStoryboardConfirm);
        foot.append(add, count, confirm);
        root.append(foot);
    }
    root.classList.remove('hidden');
}

function buildReviewShotCard(shot, index, editing) {
    const card = document.createElement('article');
    card.className = 'sb-review-shot';
    card.dataset.key = shot.key;

    const head = document.createElement('div');
    head.className = 'sb-review-shot-head';
    const no = document.createElement('span');
    no.className = 'sb-review-index';
    no.textContent = `第 ${index + 1} 镜`;
    const badge = document.createElement('span');
    badge.className = `sb-review-badge is-${shot.status || 'pending'}`;
    badge.dataset.role = 'status';
    badge.textContent = REVIEW_STATUS_LABELS[shot.status] || '待确认';
    head.append(no, badge);
    if (editing) {
        const del = document.createElement('button');
        del.type = 'button';
        del.className = 'sb-review-delete';
        del.dataset.action = 'delete';
        del.textContent = '删除';
        head.append(del);
    }

    const grid = document.createElement('div');
    grid.className = 'sb-review-fields';
    grid.append(
        reviewInput('act', '幕名', shot.act, editing),
        reviewInput('scene', '镜次', shot.scene, editing),
        reviewInput('ratio', '画幅（可选）', shot.ratio, editing),
        reviewTextArea('narration', '本镜叙事', shot.narration, editing),
        reviewTextArea('prompt', '画面描述（留空则按叙事自动生成）', shot.prompt, editing),
    );
    card.append(head, grid);
    return card;
}

function reviewInput(field, placeholder, value, editing) {
    const input = document.createElement('input');
    input.type = 'text';
    input.className = 'sb-review-input';
    input.dataset.field = field;
    input.placeholder = placeholder;
    input.value = value || '';
    input.disabled = !editing;
    return input;
}

function reviewTextArea(field, placeholder, value, editing) {
    const area = document.createElement('textarea');
    area.className = `sb-review-input sb-review-textarea ${field === 'prompt' ? 'is-prompt' : ''}`;
    area.dataset.field = field;
    area.placeholder = placeholder;
    area.value = value || '';
    area.rows = field === 'prompt' ? 3 : 2;
    area.disabled = !editing;
    return area;
}

function onReviewShotInput(event) {
    const field = event.target.closest('[data-field]');
    if (!field) return;
    const card = field.closest('.sb-review-shot');
    if (!card) return;
    const shot = storyboardReview.shots.find((item) => item.key === card.dataset.key);
    if (shot) shot[field.dataset.field] = field.value;
}

function onReviewShotClick(event) {
    const button = event.target.closest('button[data-action="delete"]');
    if (!button) return;
    const card = button.closest('.sb-review-shot');
    if (!card) return;
    storyboardReview.shots = storyboardReview.shots.filter((item) => item.key !== card.dataset.key);
    buildStoryboardReview();
}

function onReviewAddShot() {
    reviewKeySeq += 1;
    storyboardReview.shots.push({
        key: `shot-${reviewKeySeq}`,
        index: 0,
        act: '',
        scene: '',
        narration: '',
        prompt: '',
        ratio: '',
        status: 'pending',
    });
    buildStoryboardReview();
    const cards = els.storyReview.querySelectorAll('.sb-review-shot');
    const last = cards[cards.length - 1];
    if (last) {
        const narration = last.querySelector('[data-field="narration"]');
        if (narration) narration.focus();
    }
}

async function onStoryboardConfirm() {
    if (storyboardReview.busy || storyboardReview.mode !== 'edit') return;
    const shots = storyboardReview.shots;
    if (shots.length === 0) {
        toast('至少保留一个分镜', 'error');
        return;
    }
    const meaningful = shots.some((shot) => (shot.narration || '').trim() || (shot.prompt || '').trim());
    if (!meaningful) {
        toast('请至少给一个分镜填写叙事或画面描述', 'error');
        return;
    }
    storyboardReview.busy = true;
    try {
        await confirmStoryboard(storyboardReview.taskId, shots, storyboardReview.title);
        toast('已确认分镜，开始逐镜绘制', 'success');
    } catch (err) {
        toast(`确认失败：${err.message}`, 'error');
    } finally {
        storyboardReview.busy = false;
    }
}

// updateReviewBadges 只读模式下把服务端镜头状态同步到徽标。
// 只写真正变化的徽标：逐镜绘制阶段每帧都会推送，全量重写会让渲染层反复重排。
function updateReviewBadges(shots) {
    const cards = els.storyReview.querySelectorAll('.sb-review-shot');
    cards.forEach((card, index) => {
        const shot = shots[index];
        if (!shot) return;
        const status = shot.status || 'pending';
        if (storyboardReview.statuses[index] === status) return;
        storyboardReview.statuses[index] = status;
        const badge = card.querySelector('[data-role="status"]');
        if (!badge) return;
        badge.className = `sb-review-badge is-${status}`;
        badge.textContent = REVIEW_STATUS_LABELS[status] || '待确认';
    });
}

function makeIcon(className) {
    const icon = document.createElement('i');
    icon.className = className;
    return icon;
}

// setStoryboardButtonLabel 更新按钮文案；节点缺失时静默跳过，不影响其余渲染。
function setStoryboardButtonLabel(label, button = els.storySubmit) {
    const span = button && button.querySelector('span');
    if (span) span.textContent = label;
}

// ===== 键鼠 =====
// 键盘语义（2026-11 重排）：
//   ← / →  在当前目录的文件间循环切换预览（不进入文件夹）
//   ↑      返回上级目录（虚拟根时清空选中）
//   ↓      进入选中的文件夹；未选中时进入第一项文件夹
//   Enter  未选中时打开第一项；Esc 保持原有的关闭/返回语义
// 输入框、文本域、下拉框、可编辑区域、音视频控件内的方向键不被拦截；
// 数据库 / 压缩包预览内部（data-key-scope="local"）也不拦截，避免与表格操作冲突。
function onKeyDown(event) {
    if (document.getElementById('modal-root').firstChild) return;
    const target = document.activeElement;
    const typing = isTypingTarget(target);
    const local = inLocalKeyScope(target);
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
            if (!typing && !local) doRename();
            return;
        case 'Delete':
            if (!typing && !local) doDelete();
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
    if (typing || local) return;
    if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
        event.preventDefault();
        cycleFile(event.key === 'ArrowRight' ? 1 : -1);
        return;
    }
    if (event.key === 'ArrowUp') {
        event.preventDefault();
        goUpDir();
        return;
    }
    if (event.key === 'ArrowDown') {
        event.preventDefault();
        enterSelectedDir();
    }
}

// isTypingTarget 焦点在输入控件、可编辑区域或媒体控件时，方向键必须保留原生行为。
function isTypingTarget(target) {
    if (!target) return false;
    if (['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)) return true;
    if (target.isContentEditable) return true;
    return target.tagName === 'VIDEO' || target.tagName === 'AUDIO';
}

// inLocalKeyScope 数据库表格 / 压缩包列表内部自行消费按键，全局方向键不再介入。
function inLocalKeyScope(target) {
    return Boolean(target && target.closest && target.closest('[data-key-scope="local"]'));
}

// cycleFile 在当前目录的可见文件（排除文件夹）之间循环切换预览。
function cycleFile(delta) {
    const files = visibleItems().filter((entry) => !entry.dir);
    if (files.length === 0) return;
    const current = files.findIndex((entry) => entry.path === state.selectedPath);
    const next = current < 0
        ? (delta > 0 ? 0 : files.length - 1)
        : (current + delta + files.length) % files.length;
    selectFile(files[next].path);
}

// goUpDir 返回上级目录；已在虚拟根时只清空选中。
function goUpDir() {
    if (!state.currentPath) {
        if (state.selectedPath) {
            state.selectedPath = '';
            resetDatabase();
            resetArchive();
            renderAll();
        }
        return;
    }
    const parent = parentOf(state.currentPath);
    navigate(!parent || state.roots.includes(parent) ? '' : parent);
}

// enterSelectedDir 下钻选中的文件夹；未选中时进入当前目录的第一项文件夹。
function enterSelectedDir() {
    const entry = currentEntry();
    if (entry && entry.dir) {
        navigate(entry.path);
        return;
    }
    if (!entry) {
        const firstDir = visibleItems().find((item) => item.dir);
        if (firstDir) navigate(firstDir.path);
    }
}

function openSelected() {
    if (state.selectedPath) return;
    const cards = els.grid.querySelectorAll('.file-card');
    if (cards.length) cards[0].click();
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
        ['← / →', '在当前目录的文件间循环切换预览'],
        ['↑', '返回上级目录（已无上级时清空选中）'],
        ['↓', '进入选中的文件夹（未选中时进入第一项）'],
        ['Enter', '未选中时打开第一项'],
        ['Esc', '关闭沙箱 / 详情抽屉，或退出预览返回上级'],
        ['F2', '重命名选中文件'],
        ['Delete', '删除选中文件'],
        ['Ctrl+M', '移动选中文件'],
        ['Ctrl+F', '定位搜索框'],
        ['F1', '显示本帮助'],
        ['F5', '刷新清单与状态'],
        ['鼠标', '单击文件预览、单击文件夹进入；Esc 关闭弹窗或沙箱'],
    ]);
}

// ===== 实时同步 =====
// 连接、重连、命令回执匹配与推送分发全部收敛在 socket.js；
// 这里只保留「文件变更信号 → 回库拉取」的合并节流。
let syncTimer = null;
let syncRunning = false;
let realtimeWasOpen = false;
let realtimeHadConnection = false;

// 结构类操作会改变目录内容，必须重新拉取清单；内容类操作只更新状态与预览。
const STRUCTURE_OPS = new Set(['create', 'delete', 'move', 'rename', 'copy']);

// onRealtimeStatus 实时通道状态变化：断开与恢复都必须可见，不能静默。
function onRealtimeStatus(state) {
    if (state === 'open') {
        // 首次连接与断线重连都回库同步一次，补上断开期间漏掉的变更。
        scheduleSync(0);
        if (realtimeWasOpen) return;
        realtimeWasOpen = true;
        if (realtimeHadConnection) toast('实时连接已恢复', 'success');
        realtimeHadConnection = true;
        return;
    }
    if (realtimeWasOpen) {
        toast('实时连接已断开，正在重连；期间任务进度可能延迟', 'error');
    }
    realtimeWasOpen = false;
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
        case 'database': return 'fa-database';
        case 'archive': return 'fa-file-zipper';
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
