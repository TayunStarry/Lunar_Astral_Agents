// dbview.js 数据库只读预览：表清单、表结构、分页行数据与右侧「数据库」操作标签。
// 所有请求都走 /library/api/db/*，后端以 mode=ro + _query_only=1 打开，前端不提供任何写入入口。

import { fetchDBTables, fetchDBSchema, fetchDBRows, runDBQuery, generateDBSQL } from './api.js';
import { kindOf, humanSize } from './model.js';
import { toast } from './dialog.js';

const PAGE_SIZES = [25, 50, 100, 200];
const EXPORT_PAGE_SIZE = 500;
const EXPORT_MAX_ROWS = 10000;

const state = {
    path: '',
    tables: [],
    table: '',
    columns: [],
    rows: [],
    rowColumns: [],
    total: 0,
    offset: 0,
    limit: 100,
    sort: '',
    order: 'asc',
    loading: false,
    error: '',
    token: 0,
    // 只读查询：输入框文本、模式、执行状态与结果。
    query: '',
    queryMode: 'sql',
    queryBusy: false,
    queryError: '',
    queryResult: null,
    queryActive: false,
};

let changeHandler = null;

// setDatabaseChangeHandler 数据变化后通知 app.js 重绘主显示区与右侧标签页。
export function setDatabaseChangeHandler(fn) {
    changeHandler = fn;
}

// isDatabaseFile 是否为可只读预览的数据库文件。
export function isDatabaseFile(entry) {
    return Boolean(entry && !entry.dir && kindOf(entry.name) === 'database');
}

// databasePath 当前已加载的数据库路径（用于判断是否需要重新加载）。
export function databasePath() {
    return state.path;
}

// resetDatabase 切换文件时清空状态，避免把上一个库的表结构带过来。
export function resetDatabase() {
    state.path = '';
    state.tables = [];
    state.table = '';
    state.columns = [];
    state.rows = [];
    state.rowColumns = [];
    state.total = 0;
    state.offset = 0;
    state.sort = '';
    state.order = 'asc';
    state.loading = false;
    state.error = '';
    state.token += 1;
    clearQuery();
}

// clearQuery 切换库时清空查询输入与结果，避免把上一个库的语句带过来。
function clearQuery() {
    state.query = '';
    state.queryMode = 'sql';
    state.queryBusy = false;
    state.queryError = '';
    state.queryResult = null;
    state.queryActive = false;
}

// openDatabase 首次打开某个库时加载表清单并选中第一张表；重复调用直接复用。
export async function openDatabase(entry) {
    if (!isDatabaseFile(entry)) return;
    if (state.path === entry.path && (state.loading || state.tables.length > 0 || state.error)) return;
    state.path = entry.path;
    state.tables = [];
    state.table = '';
    state.columns = [];
    state.rows = [];
    state.rowColumns = [];
    state.total = 0;
    state.offset = 0;
    state.sort = '';
    state.order = 'asc';
    state.error = '';
    state.loading = true;
    state.token += 1;
    clearQuery();
    const token = state.token;
    emit();
    try {
        const data = await fetchDBTables(entry.path);
        if (token !== state.token) return;
        state.tables = Array.isArray(data.tables) ? data.tables : [];
        if (state.tables.length === 0) {
            state.error = '该数据库没有可查看的表或视图';
            state.loading = false;
            emit();
            return;
        }
        state.loading = false;
        await loadTable(state.tables[0].name, { offset: 0, sort: '', order: 'asc' });
    } catch (err) {
        if (token !== state.token) return;
        state.error = err.message || String(err);
        state.loading = false;
        emit();
    }
}

// loadTable 同时拉取表结构与一页行数据。
async function loadTable(table, options = {}) {
    if (!state.path || !table) return;
    state.table = table;
    state.offset = options.offset || 0;
    state.sort = options.sort || '';
    state.order = options.order === 'desc' ? 'desc' : 'asc';
    state.loading = true;
    state.error = '';
    // 任何表浏览动作都退出查询结果视图，避免两个视图各自维护一份「当前表」。
    state.queryActive = false;
    state.queryError = '';
    state.token += 1;
    const token = state.token;
    emit();
    try {
        const [schema, rows] = await Promise.all([
            fetchDBSchema(state.path, table),
            fetchDBRows(state.path, table, {
                offset: state.offset,
                limit: state.limit,
                sort: state.sort,
                order: state.order,
            }),
        ]);
        if (token !== state.token) return;
        state.columns = Array.isArray(schema.columns) ? schema.columns : [];
        state.rowColumns = Array.isArray(rows.columns) ? rows.columns : state.columns.map((column) => column.name);
        state.rows = Array.isArray(rows.rows) ? rows.rows : [];
        state.total = Number(rows.total || 0);
        state.offset = Number(rows.offset || 0);
        state.limit = Number(rows.limit || state.limit);
        state.sort = rows.sort || state.sort;
        state.order = rows.order || state.order;
        state.loading = false;
        emit();
    } catch (err) {
        if (token !== state.token) return;
        state.error = err.message || String(err);
        state.rows = [];
        state.loading = false;
        emit();
    }
}

function emit() {
    if (changeHandler) changeHandler();
}

// ===== 主显示区：表清单 + 表结构 + 分页行数据 =====
export function renderDatabasePreview(container, entry) {
    container.replaceChildren();
    if (!entry) return;
    if (state.path !== entry.path) {
        container.append(placeholder('fa-database', '正在读取数据库…'));
        return;
    }
    const wrap = el('div', 'db-preview');
    wrap.dataset.keyScope = 'local';

    const head = el('div', 'db-head');
    const icon = el('span', 'db-head-icon');
    icon.innerHTML = '<i class="fas fa-database"></i>';
    const title = el('div', 'db-head-title');
    const name = el('div', 'db-name', entry.name);
    const sub = el('div', 'db-sub', `${humanSize(entry.size)} · ${state.total} 行 · 只读`);
    title.append(name, sub);
    head.append(icon, title);
    wrap.append(head);

    if (state.error) {
        wrap.append(placeholder('fa-triangle-exclamation', state.error));
        container.append(wrap);
        return;
    }

    wrap.append(renderTableChips());

    if (state.queryActive) {
        wrap.append(renderQueryResult());
        container.append(wrap);
        return;
    }

    wrap.append(renderSchema());

    if (state.loading) {
        wrap.append(placeholder('fa-spinner', '正在读取表数据…'));
    } else if (state.rows.length === 0) {
        wrap.append(placeholder('fa-table', '这张表没有数据'));
    } else {
        wrap.append(renderRowsTable());
    }
    wrap.append(renderPager());
    container.append(wrap);
}

// renderQueryResult 主显示区的只读查询结果：错误提示或表格。
function renderQueryResult() {
    const wrap = el('div', 'db-query-result');
    const head = el('div', 'db-query-result-head');
    const label = el('div', 'db-query-result-label');
    label.innerHTML = '<i class="fas fa-terminal"></i>';
    label.append(document.createTextNode(' 查询结果'));
    const back = el('button', 'btn-glass btn-sm', '返回表浏览');
    back.type = 'button';
    back.addEventListener('click', () => {
        state.queryActive = false;
        state.queryError = '';
        emit();
    });
    head.append(label, back);
    wrap.append(head);

    if (state.queryError) {
        wrap.append(placeholder('fa-triangle-exclamation', state.queryError));
        return wrap;
    }
    const result = state.queryResult;
    if (!result) {
        wrap.append(placeholder('fa-table', '查询没有返回结果'));
        return wrap;
    }
    const flags = result.truncated ? ` · 已截断至 ${result.rowCount} 行` : '';
    wrap.append(el('div', 'db-hint', `${result.rowCount} 行 · ${result.elapsed} ms${flags}`));
    if (result.columns.length === 0) {
        wrap.append(placeholder('fa-table', '这条语句没有返回列'));
        return wrap;
    }
    wrap.append(renderResultTable(result.columns, result.rows));
    return wrap;
}

// renderResultTable 查询结果的通用表格（列由语句决定，不提供排序与分页）。
function renderResultTable(columns, rows) {
    const wrap = el('div', 'db-table-wrap');
    const table = document.createElement('table');
    table.className = 'db-table';
    const thead = document.createElement('thead');
    const headRow = document.createElement('tr');
    headRow.append(el('th', 'db-rowno', '#'));
    columns.forEach((name) => headRow.append(el('th', '', name)));
    thead.append(headRow);
    table.append(thead);
    const tbody = document.createElement('tbody');
    rows.forEach((row, rowIndex) => {
        const tr = document.createElement('tr');
        tr.append(el('td', 'db-rowno', String(rowIndex + 1)));
        row.forEach((value) => {
            const td = document.createElement('td');
            td.className = 'db-cell';
            const text = displayValue(value);
            td.textContent = text;
            if (value === null) td.classList.add('is-null');
            td.title = '点击复制';
            td.addEventListener('click', () => void copyCell(text));
            tr.append(td);
        });
        tbody.append(tr);
    });
    table.append(tbody);
    wrap.append(table);
    return wrap;
}

function renderTableChips() {
    const row = el('div', 'db-chips');
    state.tables.forEach((table) => {
        const chip = el('button', 'db-chip', table.name);
        chip.type = 'button';
        chip.title = table.type === 'view' ? '视图' : '表';
        if (table.name === state.table) chip.classList.add('active');
        chip.addEventListener('click', () => {
            if (table.name === state.table) return;
            void loadTable(table.name, { offset: 0, sort: '', order: 'asc' });
        });
        row.append(chip);
    });
    return row;
}

function renderSchema() {
    const details = document.createElement('details');
    details.className = 'db-schema';
    const summary = document.createElement('summary');
    summary.textContent = `表结构 · ${state.table || '—'}（${state.columns.length} 列）`;
    details.append(summary);
    const list = el('div', 'db-schema-list');
    state.columns.forEach((column) => {
        const item = el('div', 'db-schema-item');
        const colName = el('code', 'db-col-name', column.name);
        const type = el('span', 'db-col-type', column.type || '未声明类型');
        item.append(colName, type);
        if (column.pk) item.append(el('span', 'db-col-tag pk', '主键'));
        if (column.not_null) item.append(el('span', 'db-col-tag', '非空'));
        if (column.default) item.append(el('span', 'db-col-default', `默认 ${column.default}`));
        list.append(item);
    });
    details.append(list);
    return details;
}

function renderRowsTable() {
    const wrap = el('div', 'db-table-wrap');
    const table = document.createElement('table');
    table.className = 'db-table';
    const thead = document.createElement('thead');
    const headRow = document.createElement('tr');
    headRow.append(el('th', 'db-rowno', '#'));
    state.rowColumns.forEach((name) => {
        const th = document.createElement('th');
        th.className = 'db-sortable';
        th.title = '点击按该列排序';
        const label = el('span', '', name);
        th.append(label);
        if (state.sort === name) {
            const mark = el('i', `fas ${state.order === 'desc' ? 'fa-arrow-down' : 'fa-arrow-up'} db-sort-mark`);
            th.append(mark);
        }
        th.addEventListener('click', () => {
            const order = state.sort === name && state.order === 'asc' ? 'desc' : 'asc';
            void loadTable(state.table, { offset: 0, sort: name, order });
        });
        headRow.append(th);
    });
    thead.append(headRow);
    table.append(thead);
    const tbody = document.createElement('tbody');
    state.rows.forEach((row, rowIndex) => {
        const tr = document.createElement('tr');
        tr.append(el('td', 'db-rowno', String(state.offset + rowIndex + 1)));
        row.forEach((value) => {
            const td = document.createElement('td');
            td.className = 'db-cell';
            const text = displayValue(value);
            td.textContent = text;
            if (value === null) td.classList.add('is-null');
            td.title = '点击复制';
            td.addEventListener('click', () => void copyCell(text));
            tr.append(td);
        });
        tbody.append(tr);
    });
    table.append(tbody);
    wrap.append(table);
    return wrap;
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
    bar.append(prev, el('span', 'db-page-text', `第 ${page} / ${pageCount} 页 · 共 ${state.total} 行`), next);
    return bar;
}

function goPage(delta) {
    const pageCount = Math.max(1, Math.ceil(state.total / state.limit));
    const next = Math.max(0, Math.min(pageCount - 1, Math.floor(state.offset / state.limit) + delta));
    void loadTable(state.table, { offset: next * state.limit, sort: state.sort, order: state.order });
}

// ===== 右侧「数据库」标签 =====
export function renderDatabasePanel(container, entry) {
    container.replaceChildren();
    container.dataset.keyScope = 'local';
    if (!entry || state.path !== entry.path) {
        container.append(placeholder('fa-database', '正在读取数据库…'));
        return;
    }
    const pane = el('div', 'database-pane');
    const pathLine = el('div', 'db-panel-path', entry.path);
    pane.append(pathLine);

    if (state.error) {
        pane.append(placeholder('fa-triangle-exclamation', state.error));
        container.append(pane);
        return;
    }

    // 表的选择只保留主显示区的 chips（状态共用 state.table），这里不再重复一份。

    // 每页行数
    pane.append(field('每页行数', selectOf(
        PAGE_SIZES.map((size) => ({ value: String(size), label: `${size} 行` })),
        String(state.limit),
        (value) => {
            state.limit = Number(value) || 100;
            void loadTable(state.table, { offset: 0, sort: state.sort, order: state.order });
        },
    )));

    // 排序列 + 方向
    const sortField = field('排序列', selectOf(
        [{ value: '', label: '不排序' }, ...state.rowColumns.map((name) => ({ value: name, label: name }))],
        state.sort,
        (value) => void loadTable(state.table, { offset: 0, sort: value, order: state.order }),
    ));
    const direction = el('button', 'btn-glass btn-sm db-order', state.order === 'desc' ? '降序' : '升序');
    direction.type = 'button';
    direction.disabled = !state.sort;
    direction.addEventListener('click', () => {
        if (!state.sort) return;
        const order = state.order === 'asc' ? 'desc' : 'asc';
        void loadTable(state.table, { offset: 0, sort: state.sort, order });
    });
    sortField.append(direction);
    pane.append(sortField);

    // 分页
    const pager = renderPager();
    pane.append(pager);

    // 导出 CSV
    const exportBtn = el('button', 'btn-glass btn-sm', '导出当前查询为 CSV');
    exportBtn.type = 'button';
    exportBtn.disabled = state.loading || !state.table;
    exportBtn.addEventListener('click', () => void exportCSV(entry));
    pane.append(exportBtn);

    pane.append(el('div', 'db-hint', '只读：点击单元格可复制值；导出最多 10000 行，直接下载到本机。'));
    pane.append(renderQueryEditor());
    container.append(pane);
}

// ===== 只读查询输入区（SQL / 自然语言） =====
function renderQueryEditor() {
    const section = el('div', 'db-query');
    const head = el('div', 'db-query-head');
    const tabs = el('div', 'db-query-tabs');
    tabs.append(
        queryModeButton('sql', 'SQL'),
        queryModeButton('nl', '自然语言'),
    );
    const submit = el('button', 'btn-glass btn-sm primary db-query-submit', state.queryMode === 'nl' ? '生成 SQL' : '执行');
    submit.type = 'button';
    submit.disabled = state.queryBusy;
    submit.addEventListener('click', () => void submitQuery());
    head.append(tabs, submit);

    const input = document.createElement('textarea');
    input.className = 'db-query-input';
    input.value = state.query;
    input.disabled = state.queryBusy;
    input.spellcheck = false;
    input.placeholder = state.queryMode === 'nl'
        ? '用自然语言描述想查询的内容，例如：列出最近 20 条文档记录'
        : '输入只读 SQL，例如：SELECT * FROM doc_sections LIMIT 20';
    // Enter 换行，Ctrl+Enter 提交，避免与多行输入冲突。
    input.addEventListener('input', () => {
        state.query = input.value;
    });
    input.addEventListener('keydown', (event) => {
        if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
            event.preventDefault();
            void submitQuery();
        }
    });

    const foot = el('div', 'db-query-foot');
    const tips = state.queryMode === 'nl' ? 'Ctrl+Enter 生成 · Enter 换行' : 'Ctrl+Enter 执行 · Enter 换行';
    foot.append(el('span', 'db-hint', state.queryBusy ? '处理中…' : tips));
    if (state.queryError && !state.queryActive) {
        foot.append(el('span', 'db-query-error', state.queryError));
    }

    section.append(head, input, foot);
    return section;
}

function queryModeButton(mode, label) {
    const button = el('button', 'db-query-tab', label);
    button.type = 'button';
    button.disabled = state.queryBusy;
    if (state.queryMode === mode) button.classList.add('active');
    button.addEventListener('click', () => {
        if (state.queryMode === mode || state.queryBusy) return;
        state.queryMode = mode;
        state.queryError = '';
        emit();
        focusQueryInput();
    });
    return button;
}

function focusQueryInput() {
    requestAnimationFrame(() => {
        const input = document.querySelector('#database-pane .db-query-input');
        if (!input) return;
        input.focus();
        input.setSelectionRange(input.value.length, input.value.length);
    });
}

// submitQuery 按当前模式提交：SQL 直接执行，自然语言先由智能体生成 SQL 再回填。
async function submitQuery() {
    if (state.queryBusy || !state.path) return;
    const text = state.query.trim();
    if (!text) {
        toast(state.queryMode === 'nl' ? '请先描述想查询的内容' : '请先输入 SQL', 'error');
        return;
    }
    if (state.queryMode === 'nl') {
        await generateQuerySQL(text);
        return;
    }
    await executeQuery(text);
}

// generateQuerySQL 自然语言 → SQL：结果回填输入框并切到 SQL 模式，由用户确认后再执行。
async function generateQuerySQL(question) {
    state.queryBusy = true;
    state.queryError = '';
    state.queryActive = false;
    emit();
    try {
        const data = await generateDBSQL(state.path, question);
        state.query = String(data.sql || '').trim();
        state.queryMode = 'sql';
        toast('已生成 SQL，可编辑后 Ctrl+Enter 执行', 'success');
    } catch (err) {
        state.queryError = err.message || String(err);
        toast(`生成失败：${state.queryError}`, 'error');
    } finally {
        state.queryBusy = false;
        emit();
        focusQueryInput();
    }
}

// executeQuery 执行只读 SQL，结果在主显示区以表格展示。
async function executeQuery(sql) {
    state.queryBusy = true;
    state.queryError = '';
    emit();
    try {
        const data = await runDBQuery(state.path, sql);
        state.queryResult = {
            columns: Array.isArray(data.columns) ? data.columns : [],
            rows: Array.isArray(data.rows) ? data.rows : [],
            rowCount: Number(data.row_count || 0),
            truncated: Boolean(data.truncated),
            elapsed: Number(data.elapsed_ms || 0),
        };
        state.queryActive = true;
    } catch (err) {
        state.queryResult = null;
        state.queryActive = true;
        state.queryError = err.message || String(err);
    } finally {
        state.queryBusy = false;
        emit();
        focusQueryInput();
    }
}

function field(labelText, control) {
    const wrap = el('label', 'db-field');
    wrap.append(el('span', 'db-field-label', labelText), control);
    return wrap;
}

function selectOf(options, current, onChange) {
    const select = document.createElement('select');
    select.className = 'db-select';
    options.forEach((option) => {
        const item = document.createElement('option');
        item.value = option.value;
        item.textContent = option.label;
        if (option.value === current) item.selected = true;
        select.append(item);
    });
    select.addEventListener('change', () => onChange(select.value));
    return select;
}

// ===== CSV 导出 =====
async function exportCSV(entry) {
    if (!state.path || !state.table) return;
    const lines = [csvLine(state.rowColumns)];
    let offset = 0;
    try {
        while (offset < state.total && offset < EXPORT_MAX_ROWS) {
            const page = await fetchDBRows(state.path, state.table, {
                offset,
                limit: EXPORT_PAGE_SIZE,
                sort: state.sort,
                order: state.order,
            });
            const rows = Array.isArray(page.rows) ? page.rows : [];
            if (rows.length === 0) break;
            rows.forEach((row) => lines.push(csvLine(row.map(csvValue))));
            offset += rows.length;
        }
        const base = (entry.name || 'database').replace(/\.(db|sqlite|sqlite3)$/i, '');
        downloadText(`${base}-${state.table}.csv`, lines.join('\r\n'));
        const capped = state.total > EXPORT_MAX_ROWS ? `（已截断至 ${EXPORT_MAX_ROWS} 行）` : '';
        toast(`已导出 ${Math.min(offset, EXPORT_MAX_ROWS)} 行${capped}`, 'success');
    } catch (err) {
        toast(`导出失败：${err.message}`, 'error');
    }
}

function csvLine(values) {
    return values.map((value) => {
        const text = value === null || value === undefined ? '' : String(value);
        return /[",\r\n]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text;
    }).join(',');
}

function csvValue(value) {
    if (value === null || value === undefined) return '';
    return value;
}

function downloadText(filename, text) {
    // BOM 让 Excel 正确识别 UTF-8 中文。
    const blob = new Blob([`\uFEFF${text}`], { type: 'text/csv;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = filename;
    document.body.append(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 0);
}

// ===== 复制 =====
async function copyCell(text) {
    try {
        if (navigator.clipboard && window.isSecureContext) {
            await navigator.clipboard.writeText(text);
        } else {
            fallbackCopy(text);
        }
        toast('已复制单元格值', 'success');
    } catch {
        toast('复制失败', 'error');
    }
}

function fallbackCopy(text) {
    const area = document.createElement('textarea');
    area.value = text;
    area.style.position = 'fixed';
    area.style.opacity = '0';
    document.body.append(area);
    area.select();
    document.execCommand('copy');
    area.remove();
}

// ===== 工具 =====
function displayValue(value) {
    if (value === null || value === undefined) return 'NULL';
    if (typeof value === 'object') return JSON.stringify(value);
    return String(value);
}

function placeholder(icon, text) {
    const wrap = el('div', 'empty-state db-empty');
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
