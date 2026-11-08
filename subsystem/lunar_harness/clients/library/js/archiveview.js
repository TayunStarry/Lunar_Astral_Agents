// archiveview.js 压缩包只读预览：目录层级浏览 + 条目分页。
// 后端只读中央目录 / 顺序扫描元数据，不解压落盘，也不执行包内任何内容。

import { fetchArchive } from './api.js';
import { kindOf, humanSize, formatTime } from './model.js';

const state = {
    path: '',
    dir: '',
    entries: [],
    total: 0,
    offset: 0,
    limit: 200,
    format: '',
    truncated: false,
    loading: false,
    error: '',
    token: 0,
};

let host = null;
let activeEntry = null;

// isArchiveFile 是否为支持预览的压缩包。
export function isArchiveFile(entry) {
    return Boolean(entry && !entry.dir && kindOf(entry.name) === 'archive');
}

// resetArchive 切换文件时清空目录浏览状态。
export function resetArchive() {
    state.path = '';
    state.dir = '';
    state.entries = [];
    state.total = 0;
    state.offset = 0;
    state.format = '';
    state.truncated = false;
    state.loading = false;
    state.error = '';
    state.token += 1;
    host = null;
    activeEntry = null;
}

// openArchive 首次打开压缩包时加载根目录条目；重复调用直接复用。
export async function openArchive(entry) {
    if (!isArchiveFile(entry)) return;
    if (state.path === entry.path && (state.loading || state.entries.length > 0 || state.error)) return;
    state.path = entry.path;
    state.dir = '';
    state.entries = [];
    state.total = 0;
    state.offset = 0;
    state.format = '';
    state.truncated = false;
    state.error = '';
    await load();
}

// renderArchivePreview 绑定预览容器并立即绘制当前状态。
export function renderArchivePreview(container, entry) {
    host = container;
    activeEntry = entry;
    paint();
}

async function load() {
    if (!state.path) return;
    state.loading = true;
    state.error = '';
    state.token += 1;
    const token = state.token;
    paint();
    try {
        const data = await fetchArchive(state.path, state.dir, state.offset, state.limit);
        if (token !== state.token) return;
        state.entries = Array.isArray(data.entries) ? data.entries : [];
        state.total = Number(data.total || 0);
        state.offset = Number(data.offset || 0);
        state.limit = Number(data.limit || state.limit);
        state.format = data.format || state.format;
        state.truncated = Boolean(data.truncated);
        state.loading = false;
        paint();
    } catch (err) {
        if (token !== state.token) return;
        state.error = err.message || String(err);
        state.entries = [];
        state.loading = false;
        paint();
    }
}

function paint() {
    if (!host || !activeEntry) return;
    host.replaceChildren();
    const wrap = el('div', 'archive-preview');
    wrap.dataset.keyScope = 'local';

    const head = el('div', 'archive-head');
    const icon = el('span', 'archive-head-icon');
    icon.innerHTML = '<i class="fas fa-file-zipper"></i>';
    const title = el('div', 'archive-head-title');
    const name = el('div', 'archive-name', activeEntry.name);
    const sub = el('div', 'archive-sub', `${(state.format || '压缩包').toUpperCase()} · 只读元数据 · 不解压`);
    title.append(name, sub);
    head.append(icon, title, renderBreadcrumb());
    wrap.append(head);

    if (state.error) {
        wrap.append(placeholder('fa-triangle-exclamation', state.error));
        host.append(wrap);
        return;
    }
    if (state.loading) {
        wrap.append(placeholder('fa-spinner', '正在读取压缩包条目…'));
        host.append(wrap);
        return;
    }
    if (state.entries.length === 0) {
        wrap.append(placeholder('fa-folder-open', '该目录没有条目'));
        host.append(wrap);
        return;
    }

    const table = el('div', 'archive-table');
    const headRow = el('div', 'archive-row archive-row-head');
    headRow.append(
        el('span', 'archive-name-cell', '路径'),
        el('span', 'archive-type-cell', '类型'),
        el('span', 'archive-size-cell', '大小'),
        el('span', 'archive-time-cell', '修改时间'),
    );
    table.append(headRow);
    state.entries.forEach((entry) => table.append(renderRow(entry)));
    wrap.append(table);
    wrap.append(renderPager());
    if (state.truncated) {
        wrap.append(el('div', 'archive-note', '条目过多，仅扫描了前 50000 条'));
    }
    host.append(wrap);
}

function renderBreadcrumb() {
    const nav = el('div', 'archive-crumbs');
    const root = el('button', 'archive-crumb', '根目录');
    root.type = 'button';
    root.disabled = state.dir === '';
    root.addEventListener('click', () => goDir(''));
    nav.append(root);
    if (state.dir) {
        const segments = state.dir.split('/');
        segments.forEach((segment, index) => {
            nav.append(el('span', 'archive-sep', '/'));
            const target = segments.slice(0, index + 1).join('/');
            const crumb = el('button', 'archive-crumb', segment);
            crumb.type = 'button';
            crumb.disabled = index === segments.length - 1;
            crumb.addEventListener('click', () => goDir(target));
            nav.append(crumb);
        });
    }
    return nav;
}

function renderRow(entry) {
    const row = el('div', `archive-row ${entry.type === 'dir' ? 'is-dir' : 'is-file'}`);
    const nameCell = el('span', 'archive-name-cell');
    const icon = document.createElement('i');
    icon.className = entry.type === 'dir' ? 'fas fa-folder' : 'fas fa-file';
    nameCell.append(icon, el('span', '', entry.name));
    row.append(
        nameCell,
        el('span', 'archive-type-cell', entry.type === 'dir' ? '目录' : '文件'),
        el('span', 'archive-size-cell', entry.type === 'dir' ? '—' : humanSize(entry.size)),
        el('span', 'archive-time-cell', formatTime(entry.modified)),
    );
    if (entry.type === 'dir') {
        row.title = '进入该目录';
        row.addEventListener('click', () => goDir(entry.path));
    } else {
        row.title = '压缩包内文件仅展示元数据，不解压预览';
    }
    return row;
}

function renderPager() {
    const bar = el('div', 'db-pager');
    const pageCount = Math.max(1, Math.ceil(state.total / state.limit));
    const page = Math.floor(state.offset / state.limit) + 1;
    const prev = el('button', 'btn-glass btn-sm', '上一页');
    prev.type = 'button';
    prev.disabled = state.offset <= 0 || state.loading;
    prev.addEventListener('click', () => goPage(-1));
    const next = el('button', 'btn-glass btn-sm', '下一页');
    next.type = 'button';
    next.disabled = page >= pageCount || state.loading;
    next.addEventListener('click', () => goPage(1));
    bar.append(prev, el('span', 'db-page-text', `第 ${page} / ${pageCount} 页 · 共 ${state.total} 项`), next);
    return bar;
}

function goDir(dir) {
    state.dir = dir;
    state.offset = 0;
    void load();
}

function goPage(delta) {
    const pageCount = Math.max(1, Math.ceil(state.total / state.limit));
    const next = Math.max(0, Math.min(pageCount - 1, Math.floor(state.offset / state.limit) + delta));
    state.offset = next * state.limit;
    void load();
}

function placeholder(icon, text) {
    const wrap = el('div', 'empty-state archive-empty');
    const i = document.createElement('i');
    i.className = `fas ${icon}`;
    wrap.append(i, el('p', '', text));
    return wrap;
}

function el(tag, className, text) {
    const node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined && text !== null) node.textContent = text;
    return node;
}
