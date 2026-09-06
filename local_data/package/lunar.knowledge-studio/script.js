// ============================================================
// 星月智能 · 知识库 — 统一助手主逻辑 v4
// 左侧：输入 SQL/自然语言 + 审核；右侧：操作历史预览
// 数据操作全部回归原生 SQL（POST /knowledge/ {sql, params}）
// ============================================================

// 全局变量
let tables = [];
let selectedTable = tables.length ? tables[0] : null;
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
    sqlInput: $('sql-input'),
    btnRun: $('btn-run'),
    detectHint: $('detect-hint'),
    reviewBox: $('review-box'),
    reviewEmpty: $('review-empty'),
    historyList: $('history-list'),
    historyCount: $('history-count'),
    btnClearHistory: $('btn-clear-history'),
    // 右侧主视图
    subTabs: document.querySelectorAll('.master-tab'),
    subViews: document.querySelectorAll('.master-page'),
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
    toastContainer: $('toast-container')
};

const API_BASE = '/knowledge/';
const AI_MODEL = 'system-multimodal';

// ============================================================
// 初始化
// ============================================================
function init() {
    bindEvents();
    refreshTables();
    updateDetectState();
}

// ============================================================
// 原生 SQL 执行通道（放弃结构化 JSON operations，回归原生 SQL）
// 返回 Promise<{success, error, op}>，op 为后端 OperationResult
// ============================================================
async function runSQL(sql, params = []) {
    const response = await fetch(API_BASE, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ sql, params })
    });
    if (!response.ok) throw new Error(`HTTP错误! 状态码: ${response.status}`);
    const result = await response.json();
    if (!result.success) {
        throw new Error(result.error || 'SQL 执行失败');
    }
    return (result.results && result.results[0]) || { success: true };
}

// ============================================================
// 表列表（原生 SQL 查询 sqlite_master）
// ============================================================
async function refreshTables() {
    try {
        const op = await runSQL(
            "SELECT name AS name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name"
        );
        tables = (op.rows || []).map(r => r.name).filter(Boolean);

        renderTableSelect();
        if (tables.includes(selectedTable)) {
            // 保持选择
            updateCurrentTable();
        } else if (tables.length > 0) {
            selectedTable = tables[0];
            updateCurrentTable();
        } else {
            selectedTable = null;
            clearCurrentTable();
        }
        elements.tableSelect.value = selectedTable || '';
        updateTableCount();
        updateContextHint();
    } catch (error) {
        showToast('加载表列表失败: ' + error.message, 'error');
    }
}

// 加载当前表的数据结构（左侧工具区）与内容显示（右侧主视图）
function updateCurrentTable() {
    loadStructurePanel(selectedTable);
    loadContentView(selectedTable);
}

// 无可用表时的空态清理
function clearCurrentTable() {
    elements.structCount.textContent = '未选择表';
    elements.structList.innerHTML = '<p class="message info">请先创建一张表</p>';
    elements.contentTableHint.textContent = '未选择表';
    elements.contentRows.innerHTML = '<p class="message info">选择一张表后将显示其前 100 条数据</p>';
    elements.contentRowsCount.textContent = '';
}

// 左侧工具区：当前表数据结构（字段名 + 类型）
async function loadStructurePanel(table) {
    if (!table) {
        elements.structCount.textContent = '未选择表';
        elements.structList.innerHTML = '<p class="message info">请选择一张表</p>';
        return;
    }
    try {
        const op = await runSQL(`PRAGMA table_info(\`${table.replace(/`/g, '')}\`)`);
        const rows = op.rows || [];
        elements.structCount.textContent = `${table} · ${rows.length} 字段`;
        if (!rows.length) {
            elements.structList.innerHTML = '<p class="message info">该表暂无字段</p>';
            return;
        }
        elements.structList.innerHTML = rows.map(c => `
            <span class="struct-chip">
                <span class="sc-name">${escapeHtml(c.name)}</span>
                <span class="sc-type">${escapeHtml(c.type || '')}</span>
                ${Number(c.pk) === 1 ? '<i class="sc-key fa-solid fa-key" title="主键"></i>' : ''}
            </span>
        `).join('');
    } catch (error) {
        elements.structCount.textContent = table;
        elements.structList.innerHTML = `<p class="message error">加载结构失败：${escapeHtml(error.message)}</p>`;
    }
}

// 右侧主视图「内容显示」：前 100 条数据（表结构在左侧「数据源与表结构」显示）
async function loadContentView(table) {
    if (!table) {
        elements.contentTableHint.textContent = '未选择表';
        return;
    }
    elements.contentTableHint.textContent = `当前表：${table}`;

    // 数据内容
    try {
        const data = await runSQL(
            `SELECT * FROM \`${table.replace(/`/g, '')}\` LIMIT 100`
        );
        const rows = data.rows || [];
        elements.contentRowsCount.textContent = rows.length ? `共显示 ${rows.length} 行（前 100 条）` : '（空表）';
        if (!rows.length) {
            elements.contentRows.innerHTML = '<p class="message info">该表暂无数据</p>';
        } else {
            const cols = data.columns || (rows.length ? Object.keys(rows[0]) : []);
            elements.contentRows.innerHTML = renderResultTableHTML(cols, rows);
        }
    } catch (error) {
        elements.contentRowsCount.textContent = '';
        elements.contentRows.innerHTML = `<p class="message error">加载数据失败：${escapeHtml(error.message)}</p>`;
    }
}

// 右侧主视图页签切换
function switchSubView(name) {
    elements.subTabs.forEach(t => t.classList.toggle('active', t.dataset.view === name));
    elements.subViews.forEach(v => v.classList.toggle('active', v.dataset.view === name));
}

function renderTableSelect() {
    const options = ['<option value="">未选择表…</option>'];
    tables.forEach(t => {
        options.push(`<option value="${escapeHtml(t)}">${escapeHtml(t)}</option>`);
    });
    elements.tableSelect.innerHTML = options.join('');
    elements.tableSelect.value = selectedTable || '';
}

function updateTableCount() {
    elements.tableCountBadge.textContent = `${tables.length} 表`;
    // 同步右侧「操作历史」页签计数（纯数字徽标）
    elements.historyCount.textContent = `${history.length}`;
}

function updateContextHint() {
    elements.tableContextHint.textContent = selectedTable
        ? `将依据「${selectedTable}」表的字段结构生成`
        : '请选择一张表，以便 AI 依据其字段结构生成 SQL';
}

// ============================================================
// 输入识别：SQL 还是自然语言
// ============================================================
function isLikelySQL(text) {
    const t = (text || '').trim();
    if (t.length < 4) return false;
    // 去掉注释前缀后判断首关键字
    const clean = t.replace(/^\/\*[\s\S]*?\*\//, '').trim().toUpperCase();
    if (/^(SELECT|INSERT|UPDATE|DELETE|CREATE|DROP|ALTER|TRUNCATE|WITH|PRAGMA|EXPLAIN|REPLACE|BEGIN|COMMIT|ROLLBACK)\b/.test(clean)) {
        return true;
    }
    // 多行且含明显 SQL 关键字
    if (t.includes('\n') && /\b(SELECT|INSERT|UPDATE|DELETE|CREATE|DROP|ALTER|FROM|WHERE|INTO|VALUES|SET)\b/i.test(t)) {
        return true;
    }
    return false;
}

function updateDetectState() {
    const text = elements.sqlInput.value;
    const isSQL = isLikelySQL(text);
    const btn = elements.btnRun;
    const hint = elements.detectHint;

    if (isSQL) {
        btn.className = 'btn btn-success btn-run';
        btn.innerHTML = '<i class="fa-solid fa-bolt"></i> 执行操作';
        hint.innerHTML = '<i class="fa-solid fa-code"></i> 识别为 <b>SQL 语句</b>，点击可直接审核并执行';
    } else {
        btn.className = 'btn btn-primary btn-run';
        btn.innerHTML = '<i class="fa-solid fa-wand-magic-sparkles"></i> 生成 SQL';
        hint.innerHTML = '<i class="fa-solid fa-circle-info"></i> 输入内容自动识别 SQL / 自然语言';
    }
}

// ============================================================
// 主入口：根据识别结果 生成SQL 或 执行操作
// ============================================================
async function handleRun() {
    const text = elements.sqlInput.value;
    if (!text.trim()) {
        showToast('请先输入 SQL 或自然语言需求', 'info');
        elements.sqlInput.focus();
        return;
    }

    if (isLikelySQL(text)) {
        prepareDirectSQL(text.trim());
    } else {
        await generateSql();
    }
}

// 直接 SQL：填入审核框
function prepareDirectSQL(sqlText) {
    const cls = classifySql(sqlText);
    currentSQL = sqlText;
    renderReview({
        from: 'direct',
        sql: sqlText,
        explanation: `原生 SQL 语句（${cls.label}操作），确认后将被直接执行到 SQLite 数据库。`,
        cls
    });
}

// ============================================================
// AI 生成 SQL
// ============================================================
async function generateSql() {
    const prompt = elements.sqlInput.value.trim();
    if (!prompt) { showToast('请先输入需求描述', 'info'); elements.sqlInput.focus(); return; }

    // 加载态
    elements.reviewBox.innerHTML = `
        <div class="ai-card">
            <div class="ai-loading">
                <div class="spinner"></div>
                <span>正在调用模型，依据当前表结构生成 SQL...</span>
            </div>
        </div>
    `;

    const schemaContext = await buildSchemaContext();

    const systemPrompt = `你是 SQLite 数据库的 SQL 专家。请根据用户的需求和数据表结构，生成一条 SQLite 可执行的 SQL 语句。
要求：
1. 只能使用提供的表与字段名，列名/表名使用反引号包裹。
2. 只返回一条 SQL。INSERT/UPDATE/DELETE 属于写入操作，SELECT 属于查询。
3. 对于写入类需求，确保字段名严格匹配表结构。
4. 返回值必须是合法的 JSON：{"sql":"SQL语句","explanation":"用一两句中文说明这条 SQL 的含义与作用"}。
5. 不要输出任何 JSON 之外的文字。

用户需求：${prompt}

数据表结构：
${schemaContext}`;

    try {
        const content = await callAI([
            { role: 'system', content: systemPrompt },
            { role: 'user', content: prompt }
        ]);

        const parsed = parseAIResult(content);
        if (!parsed || !parsed.sql) {
            throw new Error('模型未返回有效的 SQL');
        }

        aiResult = parsed;
        currentSQL = parsed.sql;
        renderReview({ from: 'ai', sql: parsed.sql, explanation: parsed.explanation, cls: classifySql(parsed.sql) });
    } catch (error) {
        elements.reviewBox.innerHTML = `
            <div class="ai-card">
                <div class="ai-error"><i class="fa-solid fa-circle-exclamation"></i><div>生成 SQL 失败：${escapeHtml(error.message)}</div></div>
            </div>
        `;
    }
}

// 解析 AI 返回（兼容代码块包裹的 JSON）
function parseAIResult(content) {
    if (!content) return null;
    let text = content.trim();
    const fence = text.match(/```(?:json)?\s*([\s\S]*?)```/i);
    if (fence) text = fence[1].trim();
    try {
        return JSON.parse(text);
    } catch (e) {
        const start = text.indexOf('{');
        const end = text.lastIndexOf('}');
        if (start >= 0 && end > start) {
            try { return JSON.parse(text.slice(start, end + 1)); } catch (_) { /* fallthrough */ }
        }
        return null;
    }
}

// ============================================================
// 审核框渲染
// ============================================================
function renderReview({ from, sql, explanation, cls }) {
    elements.reviewBox.innerHTML = `
        <div class="ai-card">
            <div class="ai-meta">
                <span class="ai-badge"><span class="dot"></span> ${from === 'ai' ? 'AI 生成结果' : '原生 SQL'}</span>
                <span class="ai-table-tag"><i class="fa-solid fa-table"></i> ${escapeHtml(selectedTable || '未选择表')}</span>
            </div>
            <div class="ai-body">
                <div>
                    <div class="ai-block-title">SQL 语句</div>
                    <div class="sql-box">
                        <button class="sql-copy" title="复制 SQL"><i class="fa-solid fa-copy"></i></button>
                        <pre>${escapeHtml(sql)}</pre>
                    </div>
                </div>
                <div>
                    <div class="ai-block-title">含义说明</div>
                    <div class="ai-explain">${escapeHtml(explanation || '（未提供说明）')}</div>
                </div>
            </div>
            <div class="ai-foot">
                ${from === 'ai'
                    ? '<button class="btn btn-ghost btn-sm btn-regenerate"><i class="fa-solid fa-rotate-right"></i> 重新生成</button>'
                    : ''}
                <span class="op-type-label"><i class="fa-solid ${cls.icon}"></i> ${cls.label}操作</span>
                <button class="btn btn-light btn-sm" id="ai-copy-result"><i class="fa-solid fa-copy"></i> 复制</button>
                <button class="btn btn-primary btn-sm" id="ai-execute"><i class="fa-solid fa-check"></i> 确认执行</button>
            </div>
        </div>
    `;

    const box = elements.reviewBox;
    box.querySelector('.sql-copy').addEventListener('click', () => copyText(sql));
    box.querySelector('#ai-copy-result').addEventListener('click', () => copyText(sql));
    const regen = box.querySelector('.btn-regenerate');
    if (regen) regen.addEventListener('click', generateSql);
    box.querySelector('#ai-execute').addEventListener('click', () => confirmExecuteSql());
}

// 确认执行（带确认对话框）
function confirmExecuteSql() {
    if (!currentSQL) return;
    const cls = classifySql(currentSQL);
    elements.confirmMessage.innerHTML =
        `确定要执行这条<strong>${escapeHtml(cls.label)}</strong>SQL 吗？${escapeHtml(cls.isWrite ? '此操作不可恢复！' : '')}<br><br>` +
        `<code style="word-break:break-all;font-size:0.8rem;">${escapeHtml(currentSQL)}</code>`;
    elements.confirmActionBtn.className = cls.isWrite ? 'btn btn-danger' : 'btn btn-primary';
    elements.confirmActionBtn.innerHTML = `<i class="fa-solid fa-check"></i> 确认执行`;
    openConfirmDialog(() => executeSql());
}

// ============================================================
// 执行 SQL → 记录操作历史
// ============================================================
async function executeSql() {
    const sql = currentSQL;
    if (!sql) { hideConfirmDialog(); return; }

    hideConfirmDialog();
    renderReviewLoading();

    const cls = classifySql(sql);
    const rec = {
        seq: ++historySeq,
        kind: cls.kind,
        label: cls.label,
        icon: cls.icon,
        isWrite: cls.isWrite,
        sql,
        time: new Date(),
        timeText: formatTime(new Date()),
        success: false,
        error: '',
        affectedRows: 0,
        lastInsertId: null,
        columns: [],
        rows: [],
        expanded: false
    };

    try {
        const op = await runSQL(sql);
        rec.success = true;
        rec.affectedRows = op.affected_rows || 0;
        rec.lastInsertId = op.last_insert_id != null ? op.last_insert_id : null;
        if (op.rows) {
            // 后端原生 SQL 返回 rows 而不含 columns，列名从首行的键推导
            const cols = op.columns || (op.rows.length ? Object.keys(op.rows[0]) : []);
            rec.columns = cols;
            rec.rows = op.rows;
            rec.expanded = true; // 查询结果自动展开
        }
        history.unshift(rec);

        if (cls.isWrite) {
            // 写入后刷新表列表 / 统计
            await refreshTables();
            showToast(`执行成功，影响 ${rec.affectedRows} 行${rec.lastInsertId != null ? ' · ID=' + rec.lastInsertId : ''}`, 'success');
            renderReviewResult(rec);
        } else {
            showToast(`查询成功，返回 ${rec.rows.length} 条记录`, 'success');
            renderReviewResult(rec);
        }
    } catch (error) {
        rec.success = false;
        rec.error = error.message;
        history.unshift(rec);
        showToast('SQL 执行失败: ' + error.message, 'error');
        renderReviewError(error.message);
    }

    renderHistory();
    updateTableCount();
}

function renderReviewLoading() {
    elements.reviewBox.innerHTML = `
        <div class="ai-card">
            <div class="ai-loading"><div class="spinner"></div><span>正在执行 SQL...</span></div>
        </div>
    `;
}

function renderReviewResult(rec) {
    let body;
    if (rec.columns && rec.rows) {
        body = renderResultTableHTML(rec.columns, rec.rows);
    } else {
        body = `
            <div class="ai-exec-ok">
                <i class="fa-solid fa-circle-check"></i>
                <div>
                    <b>执行完成</b>
                    <span>影响 ${rec.affectedRows} 行${rec.lastInsertId != null ? ' · 新记录 ID=' + rec.lastInsertId : ''}</span>
                </div>
            </div>
        `;
    }
    elements.reviewBox.innerHTML = `
        <div class="ai-card">
            <div class="ai-meta">
                <span class="ai-badge success"><span class="dot"></span> 执行结果</span>
                <span class="op-type-label"><i class="fa-solid ${rec.icon}"></i> ${escapeHtml(rec.label)}操作</span>
            </div>
            <div class="ai-body">
                <div class="ai-block-title">返回数据</div>
                <div class="result-wrap">${body}</div>
            </div>
            <div class="ai-foot">
                <button class="btn btn-light btn-sm" id="ai-copy-sql"><i class="fa-solid fa-copy"></i> 复制 SQL</button>
                <button class="btn btn-light btn-sm" id="ai-close-result"><i class="fa-solid fa-xmark"></i> 关闭</button>
            </div>
        </div>
    `;
    elements.reviewBox.querySelector('#ai-copy-sql').addEventListener('click', () => copyText(rec.sql));
    elements.reviewBox.querySelector('#ai-close-result').addEventListener('click', () => resetReviewEmpty());
}

function renderReviewError(msg) {
    elements.reviewBox.innerHTML = `
        <div class="ai-card">
            <div class="ai-error"><i class="fa-solid fa-circle-exclamation"></i><div>SQL 执行失败：${escapeHtml(msg)}</div></div>
        </div>
    `;
}

function resetReviewEmpty() {
    elements.reviewBox.innerHTML = `
        <div class="empty-mini">
            <i class="fa-solid fa-list-check"></i>
            <p>输入 SQL 点击「执行操作」，或输入自然语言点击「生成 SQL」后，SQL 与含义将在这里展示，经你确认后再执行。</p>
        </div>
    `;
}

// ============================================================
// SQL 分类
// ============================================================
function classifySql(sql) {
    const up = (sql || '').trim().replace(/^\/\*[\s\S]*?\*\//, '').toUpperCase();
    if (/^\s*SELECT/.test(up) || /^\s*PRAGMA/.test(up) || /^\s*EXPLAIN/.test(up) || /^\s*WITH/.test(up)) {
        return { kind: 'query', label: '查询', icon: 'fa-magnifying-glass', isWrite: false };
    }
    if (/^\s*INSERT|^\s*REPLACE/.test(up)) return { kind: 'insert', label: '插入', icon: 'fa-plus', isWrite: true };
    if (/^\s*UPDATE/.test(up)) return { kind: 'update', label: '更新', icon: 'fa-pen-to-square', isWrite: true };
    if (/^\s*DELETE/.test(up)) return { kind: 'delete', label: '删除', icon: 'fa-trash-can', isWrite: true };
    if (/^\s*CREATE/.test(up)) return { kind: 'create', label: '建表', icon: 'fa-table', isWrite: true };
    if (/^\s*DROP/.test(up)) return { kind: 'drop', label: '删表', icon: 'fa-table', isWrite: true };
    if (/^\s*ALTER/.test(up) || /^\s*TRUNCATE/.test(up)) return { kind: 'alter', label: '变更', icon: 'fa-arrows-rotate', isWrite: true };
    return { kind: 'other', label: '执行', icon: 'fa-terminal', isWrite: !/^\s*SELECT|^\s*PRAGMA|^\s*EXPLAIN|^\s*WITH/.test(up) };
}

// ============================================================
// 操作历史渲染
// ============================================================
const kindMeta = {
    create:  { label: '创建', icon: 'fa-table', cls: 'k-create' },
    insert:  { label: '插入', icon: 'fa-plus', cls: 'k-insert' },
    update:  { label: '变更', icon: 'fa-pen-to-square', cls: 'k-update' },
    delete:  { label: '删除', icon: 'fa-trash-can', cls: 'k-delete' },
    drop:    { label: '删表', icon: 'fa-trash-can', cls: 'k-drop' },
    alter:   { label: '变更', icon: 'fa-arrows-rotate', cls: 'k-update' },
    query:   { label: '查询', icon: 'fa-magnifying-glass', cls: 'k-query' },
    other:   { label: '执行', icon: 'fa-terminal', cls: 'k-other' }
};

function renderHistory() {
    if (!history.length) {
        elements.historyList.innerHTML = `
            <div class="empty-state">
                <div class="empty-icon"><i class="fa-solid fa-chart-line"></i></div>
                <h3>暂无操作</h3>
                <p>在左侧执行 SQL 后，<br>这里会展示创建 / 变更 / 删除 / 查询的记录与结果预览</p>
            </div>
        `;
        elements.historyCount.textContent = '0';
        return;
    }

    elements.historyList.innerHTML = history.map(rec => {
        const meta = kindMeta[rec.kind] || kindMeta.other;
        const statusCls = rec.success ? 's-ok' : 's-err';
        const statusIcon = rec.success ? 'fa-circle-check' : 'fa-circle-xmark';
        const summary = rec.success
            ? (rec.kind === 'query' ? `返回 ${rec.rows.length} 条` : (rec.affectedRows ? `影响 ${rec.affectedRows} 行` : `成功${rec.lastInsertId != null ? ' · ID=' + rec.lastInsertId : ''}`))
            : '失败';
        const hasRows = rec.success && rec.rows && rec.rows.length > 0;

        return `
            <div class="history-item ${statusCls}" data-seq="${rec.seq}">
                <div class="hi-head" data-toggle="${rec.seq}">
                    <span class="badge ${meta.cls}"><i class="fa-solid ${meta.icon}"></i> ${meta.label}</span>
                    <div class="hi-sql">${escapeHtml(rec.sql)}</div>
                    <span class="hi-summary"><i class="fa-solid ${statusIcon}"></i> ${escapeHtml(summary)}</span>
                    <span class="hi-time">${escapeHtml(rec.timeText)}</span>
                    ${hasRows ? '<i class="fa-solid fa-chevron-down hi-caret"></i>' : ''}
                </div>
                ${rec.error ? `<div class="hi-error">${escapeHtml(rec.error)}</div>` : ''}
                ${hasRows ? `
                    <div class="hi-detail ${rec.expanded ? 'open' : ''}">
                        ${renderResultTableHTML(rec.columns, rec.rows)}
                    </div>
                ` : ''}
            </div>
        `;
    }).join('');

    // 绑定展开 / 折叠
    elements.historyList.querySelectorAll('[data-toggle]').forEach(el => {
        el.addEventListener('click', () => {
            const item = el.closest('.history-item');
            const rec = history.find(h => h.seq === Number(el.dataset.toggle));
            if (!rec) return;
            const detail = item.querySelector('.hi-detail');
            if (!detail) return;
            const open = detail.classList.toggle('open');
            rec.expanded = open;
            const caret = item.querySelector('.hi-caret');
            if (caret) caret.classList.toggle('open', open);
        });
    });

    elements.historyCount.textContent = `${history.length}`;
}

function renderResultTableHTML(columns, rows) {
    if (!columns || !columns.length) columns = rows.length ? Object.keys(rows[0]) : [];
    if (!rows || !rows.length) return '<p class="message info">查询结果为空</p>';
    return `
        <div class="table-wrap">
            <table class="data-table">
                <thead><tr>${columns.map(c => `<th>${escapeHtml(c)}</th>`).join('')}</tr></thead>
                <tbody>
                    ${rows.map(row => `<tr>${columns.map(c => {
                        const v = row[c];
                        return `<td>${v !== null && v !== undefined ? escapeHtml(String(v)) : ''}</td>`;
                    }).join('')}</tr>`).join('')}
                </tbody>
            </table>
        </div>
    `;
}

function clearHistory() {
    if (!history.length) return;
    history = [];
    historySeq = 0;
    renderHistory();
    showToast('操作历史已清空', 'success');
}

// ============================================================
// 创建表（原生 SQL：拼 CREATE TABLE 语句）
// ============================================================
function showCreateTableDialog() {
    elements.newTableName.value = '';
    elements.columnsContainer.innerHTML = '';
    addColumn(true);
    elements.createTableDialog.classList.add('show');
}

function hideCreateTableDialog() {
    elements.createTableDialog.classList.remove('show');
}

function addColumn(isDefault = false) {
    const count = elements.columnsContainer.children.length + 1;
    const item = document.createElement('div');
    item.className = 'column-item';
    item.innerHTML = `
        <div class="column-header">
            <span>列 ${count}</span>
            <button class="remove-column btn btn-ghost btn-sm"><i class="fa-solid fa-trash-can"></i> 删除</button>
        </div>
        <div class="column-fields">
            <div class="form-row">
                <div class="form-group">
                    <label class="field-label">列名</label>
                    <input type="text" class="column-name input" value="${isDefault ? 'id' : ''}" placeholder="column_name">
                </div>
                <div class="form-group">
                    <label class="field-label">数据类型</label>
                    <select class="column-type input">
                        <option value="INTEGER" ${isDefault ? 'selected' : ''}>INTEGER</option>
                        <option value="TEXT">TEXT</option>
                        <option value="REAL">REAL</option>
                        <option value="BLOB">BLOB</option>
                        <option value="BOOLEAN">BOOLEAN</option>
                        <option value="TIMESTAMP">TIMESTAMP</option>
                    </select>
                </div>
            </div>
            <div class="form-row">
                <div class="checkbox-group"><input type="checkbox" class="column-primary" ${isDefault ? 'checked' : ''}><label>主键</label></div>
                <div class="checkbox-group"><input type="checkbox" class="column-autoinc" ${isDefault ? 'checked' : ''}><label>自增</label></div>
                <div class="checkbox-group"><input type="checkbox" class="column-notnull" ${isDefault ? 'checked' : ''}><label>非空</label></div>
            </div>
            <div class="form-group">
                <label class="field-label">默认值</label>
                <input type="text" class="column-default input" placeholder="可选">
            </div>
        </div>
    `;
    elements.columnsContainer.appendChild(item);
    item.querySelector('.remove-column').addEventListener('click', () => {
        if (elements.columnsContainer.children.length <= 1) {
            showToast('至少需要保留一列', 'error');
            return;
        }
        item.remove();
        renumberColumns();
    });
}

function renumberColumns() {
    const cols = elements.columnsContainer.children;
    for (let i = 0; i < cols.length; i++) {
        const span = cols[i].querySelector('.column-header span');
        if (span) span.textContent = `列 ${i + 1}`;
    }
}

async function createTable() {
    const tableName = elements.newTableName.value.trim();
    if (!tableName) { showToast('表名不能为空', 'error'); return; }

    const cols = elements.columnsContainer.children;
    const defs = [];
    for (let i = 0; i < cols.length; i++) {
        const item = cols[i];
        const name = item.querySelector('.column-name').value.trim();
        const type = item.querySelector('.column-type').value;
        if (!name) continue;
        let def = `\`${name}\` ${type}`;
        const primary = item.querySelector('.column-primary').checked;
        const autoinc = item.querySelector('.column-autoinc').checked;
        const notnull = item.querySelector('.column-notnull').checked;
        if (primary) {
            def += ' PRIMARY KEY';
            if (autoinc) def += ' AUTOINCREMENT';
        }
        if (notnull) def += ' NOT NULL';
        const dflt = item.querySelector('.column-default').value.trim();
        if (dflt) {
            const isNum = !isNaN(Number(dflt));
            def += ` DEFAULT ${isNum ? dflt : `'${dflt.replace(/'/g, "''")}'`}`;
        }
        defs.push(def);
    }

    if (!defs.length) { showToast('至少需要定义一个列', 'error'); return; }

    const createSQL = `CREATE TABLE IF NOT EXISTS \`${tableName.replace(/`/g, '')}\` (${defs.join(', ')})`;

    try {
        hideCreateTableDialog();
        currentSQL = createSQL;
        await executeSql(); // 复用执行与历史记录
        showToast(`表「${tableName}」创建成功`, 'success');
    } catch (error) {
        hideCreateTableDialog();
        showToast('创建表失败: ' + error.message, 'error');
        renderReviewError(error.message);
    }
}

// ============================================================
// 删除当前表
// ============================================================
function confirmDropTable() {
    if (!selectedTable) { showToast('请先选择一张表', 'info'); return; }
    const table = selectedTable;
    elements.confirmMessage.innerHTML = `确定要删除整个表 <strong>${escapeHtml(table)}</strong> 吗？此操作不可恢复！`;
    elements.confirmActionBtn.className = 'btn btn-danger';
    elements.confirmActionBtn.innerHTML = `<i class="fa-solid fa-trash-can"></i> 确认删除`;
    openConfirmDialog(async () => {
        const dropSQL = `DROP TABLE IF EXISTS \`${table.replace(/`/g, '')}\``;
        currentSQL = dropSQL;
        await executeSql();
        if (selectedTable === table) {
            selectedTable = null;
            elements.tableSelect.value = '';
            updateContextHint();
        }
    });
}

// ============================================================
// 模型调用 / 工具函数
// ============================================================
async function buildSchemaContext() {
    if (tables.length === 0) return '';
    const lines = [];
    for (const t of tables) {
        try {
            const op = await runSQL(`PRAGMA table_info(\`${t.replace(/`/g, '')}\`)`);
            const fields = (op.rows || []).map(c => `${c.name} ${c.type}`).join(', ');
            lines.push(`表 ${t}(${fields})`);
        } catch (e) { /* skip */ }
    }
    return lines.join('\n');
}

async function callAI(messages) {
    const response = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ model: AI_MODEL, messages, stream: false, temperature: 0.2 })
    });
    if (!response.ok) throw new Error('API 请求失败 (' + response.status + ')');
    const data = await response.json();
    if (!data.choices?.[0]?.message?.content) throw new Error('API 返回格式异常');
    return data.choices[0].message.content;
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
    if (!elements.toastContainer) return;
    type = type || 'info';
    const icons = { success: 'fa-circle-check', error: 'fa-circle-exclamation', info: 'fa-circle-info', warn: 'fa-triangle-exclamation' };
    const toast = document.createElement('div');
    toast.className = 'toast ' + type;
    toast.innerHTML = '<i class="fa-solid ' + (icons[type] || icons.info) + '"></i><span>' + escapeHtml(message) + '</span>';
    elements.toastContainer.appendChild(toast);
    requestAnimationFrame(() => toast.classList.add('toast-in'));
    setTimeout(() => {
        toast.classList.add('toast-out');
        toast.addEventListener('transitionend', () => toast.remove());
    }, 3500);
}

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

// 确认框
function openConfirmDialog(onConfirm) {
    elements.confirmActionBtn.onclick = onConfirm;
    elements.confirmDialog.classList.add('show');
}
function hideConfirmDialog() {
    elements.confirmDialog.classList.remove('show');
}

// ============================================================
// 事件绑定
// ============================================================
function bindEvents() {
    // 表控制
    elements.btnRefreshTables.addEventListener('click', () => refreshTables());
    elements.btnCreateTable.addEventListener('click', showCreateTableDialog);
    elements.btnDropTable.addEventListener('click', confirmDropTable);
    elements.tableSelect.addEventListener('change', () => {
        selectedTable = elements.tableSelect.value || null;
        updateContextHint();
        if (selectedTable) {
            updateCurrentTable();
        } else {
            clearCurrentTable();
        }
    });

    // 输入识别
    elements.sqlInput.addEventListener('input', updateDetectState);
    elements.sqlInput.addEventListener('keydown', (e) => {
        if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') { e.preventDefault(); handleRun(); }
    });

    // 主操作
    elements.btnRun.addEventListener('click', handleRun);

    // 历史
    elements.btnClearHistory.addEventListener('click', clearHistory);

    // 右侧主视图：页签切换 + 内容刷新
    elements.subTabs.forEach(tab => tab.addEventListener('click', () => switchSubView(tab.dataset.view)));
    elements.btnRefreshContent.addEventListener('click', () => {
        if (!selectedTable) { showToast('请先选择一张表', 'info'); return; }
        loadContentView(selectedTable);
    });

    // 创建表对话框
    elements.cancelCreateTableBtn.addEventListener('click', hideCreateTableDialog);
    elements.confirmCreateTableBtn.addEventListener('click', createTable);
    elements.addColumnBtn.addEventListener('click', () => addColumn(false));

    // 确认框
    elements.cancelConfirmBtn.addEventListener('click', hideConfirmDialog);

    // 对话框遮罩 / 关闭
    document.querySelectorAll('.dialog').forEach(dialog => {
        dialog.addEventListener('click', function (e) {
            if (e.target === this) this.classList.remove('show');
        });
    });
    document.querySelectorAll('[data-close]').forEach(el => {
        el.addEventListener('click', () => {
            const dlg = document.getElementById(el.getAttribute('data-close'));
            if (dlg) dlg.classList.remove('show');
        });
    });
}

// ============================================================
// 启动
// ============================================================
init();