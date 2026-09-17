// ============================================================
// 星月智能 · 知识库 — execute.js
// SQL 执行与历史落账 / 创建表对话框 / 删除当前表
// ============================================================

// 执行 SQL → 记录操作历史
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
        db: currentDB,
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
            <button class="remove-column btn btn-sm"><i class="fas fa-trash-can"></i> 删除</button>
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
        await executeSql(); // 复用执行与历史记录（执行后停留在审核页查看结果）
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
