// ============================================================
// 星月智能 · 知识库 — tables.js
// 表列表（sqlite_master）/ 表结构 / 内容预览 / 主视图页签
// ============================================================

// 表列表（原生 SQL 查询 sqlite_master）
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

// 右侧主视图页签切换（data-tab 约定；即时查询避免陈旧引用导致切不回去）
function switchSubView(name) {
    document.querySelectorAll('.tab').forEach(t => t.classList.toggle('active', t.dataset.tab === name));
    document.querySelectorAll('.tab-page').forEach(v => v.classList.toggle('active', v.dataset.tab === name));
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
