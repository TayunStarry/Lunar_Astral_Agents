// ============================================================
// 星月智能 · 知识库 — review.js
// 主入口分流 / SQL 分类 / 审核框渲染 / 确认执行
// ============================================================

// 主入口：根据识别结果 生成SQL 或 执行操作
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
        switchSubView('review');
    }
}

// SQL 分类（驱动徽标 / 图标 / 读写判定）
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

// 审核框渲染
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
                    ? '<button class="btn btn-sm btn-regenerate"><i class="fas fa-rotate-right"></i> 重新生成</button>'
                    : ''}
                <span class="op-type-label"><i class="fas ${cls.icon}"></i> ${cls.label}操作</span>
                <button class="btn btn-sm" id="ai-copy-result"><i class="fas fa-copy"></i> 复制</button>
                <button class="btn btn-primary btn-sm" id="ai-execute"><i class="fas fa-check"></i> 确认执行</button>
            </div>
        </div>
    `;
    switchSubView('review');

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
    elements.confirmActionBtn.innerHTML = `<i class="fas fa-check"></i> 确认执行`;
    openConfirmDialog(() => executeSql());
}

// 审核框：执行中加载态
function renderReviewLoading() {
    elements.reviewBox.innerHTML = `
        <div class="ai-card">
            <div class="ai-loading"><div class="spinner"></div><span>正在执行 SQL...</span></div>
        </div>
    `;
    switchSubView('review');
}

// 审核框：执行结果
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
                <span class="op-type-label"><i class="fas ${rec.icon}"></i> ${escapeHtml(rec.label)}操作</span>
            </div>
            <div class="ai-body">
                <div class="ai-block-title">返回数据</div>
                <div class="result-wrap">${body}</div>
            </div>
            <div class="ai-foot">
                <button class="btn btn-sm" id="ai-copy-sql"><i class="fas fa-copy"></i> 复制 SQL</button>
                <button class="btn btn-sm" id="ai-close-result"><i class="fas fa-xmark"></i> 关闭</button>
            </div>
        </div>
    `;
    elements.reviewBox.querySelector('#ai-copy-sql').addEventListener('click', () => copyText(rec.sql));
    elements.reviewBox.querySelector('#ai-close-result').addEventListener('click', () => resetReviewEmpty());
}

// 审核框：执行失败
function renderReviewError(msg) {
    elements.reviewBox.innerHTML = `
        <div class="ai-card">
            <div class="ai-error"><i class="fas fa-circle-exclamation"></i><div>SQL 执行失败：${escapeHtml(msg)}</div></div>
        </div>
    `;
    switchSubView('review');
}

// 审核框：回到空态
function resetReviewEmpty() {
    elements.reviewBox.innerHTML = `
        <div class="empty-state" id="review-empty">
            <div class="empty-icon"><i class="fas fa-list-check"></i></div>
            <h3>等待审核</h3>
            <p>输入 SQL 点击「执行操作」，或输入自然语言点击「生成 SQL」后，<br>SQL 与含义将在这里展示，经你确认后再执行。</p>
        </div>
    `;
}
