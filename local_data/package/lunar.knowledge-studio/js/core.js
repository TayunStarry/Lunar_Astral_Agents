// ============================================================
// 星月智能 · 知识库 — core.js
// 全局状态 / DOM 引用 / SQL 执行通道 / 通用工具（Toast / 复制 / 确认框）
// 数据操作全部走原生 SQL（POST /knowledge/ {sql, params, db}）
// ============================================================

// 全局变量
let tables = [];
let selectedTable = null;
let currentDB = 'knowledge';  // 当前数据源：knowledge | 扫描到的 database 目录内 *.db（相对路径去 .db）
let aiResult = null;          // { sql, explanation }
let currentSQL = null;        // 当前审核中的待执行 SQL
let history = [];             // 操作历史
let historySeq = 0;

// DOM 元素
const $ = (id) => document.getElementById(id);
const elements = {
    tableSelect: $('table-select'),
    btnRefreshTables: $('btn-refresh-tables'),
    btnCreateTable: $('btn-create-table'),
    btnDropTable: $('btn-drop-table'),
    tableContextHint: $('table-context-hint'),
    tableCountBadge: $('table-count-badge'),
    structList: $('struct-list'),
    structCount: $('struct-count'),
    // 数据库探查
    btnScanDb: $('btn-scan-db'),
    dbScanResult: $('db-scan-result'),
    sqlInput: $('sql-input'),
    btnRun: $('btn-run'),
    detectHint: $('detect-hint'),
    reviewBox: $('review-box'),
    reviewEmpty: $('review-empty'),
    historyList: $('history-list'),
    historyCount: $('history-count'),
    btnClearHistory: $('btn-clear-history'),
    // 右侧主视图
    contentTableHint: $('content-table-hint'),
    contentRows: $('content-rows'),
    contentRowsCount: $('content-rows-count'),
    btnRefreshContent: $('btn-refresh-content'),
    // 创建表
    createTableDialog: $('create-table-dialog'),
    newTableName: $('new-table-name'),
    columnsContainer: $('columns-container'),
    cancelCreateTableBtn: $('cancel-create-table'),
    confirmCreateTableBtn: $('confirm-create-table'),
    addColumnBtn: $('add-column'),
    // 确认框
    confirmDialog: $('confirm-dialog'),
    confirmMessage: $('confirm-message'),
    cancelConfirmBtn: $('cancel-confirm'),
    confirmActionBtn: $('confirm-action'),
    // 单例 Toast
    toast: $('toast')
};

const API_BASE = '/knowledge/';
const AI_MODEL = 'system-multimodal';

// ============================================================
// 原生 SQL 执行通道（db 为当前数据源；返回 Promise<op>，op 为后端 OperationResult）
// ============================================================
async function runSQL(sql, params = []) {
    const response = await fetch(API_BASE, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ sql, params, db: currentDB })
    });
    if (!response.ok) throw new Error(`HTTP错误! 状态码: ${response.status}`);
    const result = await response.json();
    if (!result.success) {
        throw new Error(result.error || 'SQL 执行失败');
    }
    return (result.results && result.results[0]) || { success: true };
}

// ============================================================
// 通用工具
// ============================================================
function escapeHtml(str) {
    if (typeof str !== 'string') return str === null || str === undefined ? '' : String(str);
    return str
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;');
}

function formatTime(d) {
    const p = n => String(n).padStart(2, '0');
    return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

async function copyText(text) {
    try {
        await navigator.clipboard.writeText(text);
        showToast('已复制到剪贴板', 'success');
    } catch (err) {
        const ta = document.createElement('textarea');
        ta.value = text;
        ta.style.position = 'fixed';
        ta.style.opacity = '0';
        document.body.appendChild(ta);
        ta.select();
        document.execCommand('copy');
        document.body.removeChild(ta);
        showToast('已复制到剪贴板', 'success');
    }
}

function showToast(message, type) {
    const toast = elements.toast;
    if (!toast) return;
    type = type || 'info';
    const icons = { success: 'fa-circle-check', error: 'fa-circle-exclamation', info: 'fa-circle-info', warn: 'fa-triangle-exclamation' };
    toast.className = 'toast ' + (type === 'success' || type === 'error' || type === 'warn' ? type : 'info');
    toast.innerHTML = '<i class="fas ' + (icons[type] || icons.info) + '"></i><span>' + escapeHtml(message) + '</span>';
    // 触发重排后加 visible，保证连续调用也有过渡
    void toast.offsetWidth;
    toast.classList.add('visible');
    clearTimeout(showToast._timer);
    showToast._timer = setTimeout(() => toast.classList.remove('visible'), 2600);
}

// 确认框
function openConfirmDialog(onConfirm) {
    elements.confirmActionBtn.onclick = onConfirm;
    elements.confirmDialog.classList.add('show');
}
function hideConfirmDialog() {
    elements.confirmDialog.classList.remove('show');
}
