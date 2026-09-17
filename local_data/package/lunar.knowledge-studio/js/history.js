// ============================================================
// 星月智能 · 知识库 — history.js
// 操作历史渲染（展开/折叠）/ 结果表格 HTML / 清空历史
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
                ${rec.db !== 'knowledge' ? '<span class="badge k-other"><i class="fa-solid fa-globe"></i> ' + escapeHtml(rec.db) + '</span>' : ''}
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

// 通用结果表格 HTML（列名从首行键推导，后端不返回 columns）
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
