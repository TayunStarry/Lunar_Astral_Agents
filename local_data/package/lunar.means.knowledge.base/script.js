// ============================================================
// 星月智能 · 智存库 — 主逻辑脚本 (Font Awesome 图标适配)
// ============================================================

// 全局变量
let selectedTable = null;
let tables = [];

// DOM元素
const elements = {
    // 表相关
    tableList: document.getElementById('table-list'),
    tableDetails: document.getElementById('table-details'),
    previewContent: document.getElementById('preview-content'),
    previewStatus: document.getElementById('preview-status'),
    refreshTablesBtn: document.getElementById('refresh-tables'),
    createTableBtn: document.getElementById('create-table'),
    refreshPreviewBtn: document.getElementById('refresh-preview'),

    // 标签页
    tabBtns: document.querySelectorAll('.tab-btn'),
    tabContents: document.querySelectorAll('.tab-content'),

    // 数据操作
    executeSelectBtn: document.getElementById('execute-select'),
    executeInsertBtn: document.getElementById('execute-insert'),
    executeUpdateBtn: document.getElementById('execute-update'),
    executeDeleteBtn: document.getElementById('execute-delete'),

    // 表单输入
    selectFilter: document.getElementById('select-filter'),
    selectOrder: document.getElementById('select-order'),
    selectLimit: document.getElementById('select-limit'),
    selectOffset: document.getElementById('select-offset'),
    insertData: document.getElementById('insert-data'),
    updateData: document.getElementById('update-data'),
    updateFilter: document.getElementById('update-filter'),
    deleteFilter: document.getElementById('delete-filter'),

    // 创建表对话框
    createTableDialog: document.getElementById('create-table-dialog'),
    newTableName: document.getElementById('new-table-name'),
    cancelCreateTableBtn: document.getElementById('cancel-create-table'),
    confirmCreateTableBtn: document.getElementById('confirm-create-table'),
    addColumnBtn: document.getElementById('add-column'),
    columnsContainer: document.getElementById('columns-container'),

    // 确认对话框
    confirmDialog: document.getElementById('confirm-dialog'),
    confirmMessage: document.getElementById('confirm-message'),
    cancelConfirmBtn: document.getElementById('cancel-confirm'),
    confirmActionBtn: document.getElementById('confirm-action'),

    // 高级查询操作符说明对话框
    operatorsDialog: document.getElementById('operators-dialog'),
    showOperatorsBtn: document.getElementById('show-operators'),
    closeOperatorsBtn: document.getElementById('close-operators')
};

// API基础URL
const API_BASE = '/knowledge/';

// ============================================================
// 初始化
// ============================================================
function init() {
    bindEvents();
    loadTables();

    const clearResultsBtn = document.getElementById('clear-results');
    if (clearResultsBtn) {
        clearResultsBtn.addEventListener('click', clearResults);
    }

    if (elements.refreshPreviewBtn) {
        elements.refreshPreviewBtn.addEventListener('click', refreshPreview);
    }

    updateTableCount();
}

// ============================================================
// 清除结果
// ============================================================
function clearResults() {
    elements.previewStatus.innerHTML = '';
    elements.previewContent.innerHTML = `
        <div class="empty-state">
            <div class="empty-icon"><i class="fa-solid fa-chart-line fa-2x"></i></div>
            <h3>数据预览</h3>
            <p>选择一个表后，将自动显示数据内容<br>执行操作后，结果也将显示在这里</p>
        </div>
    `;
}

// 刷新预览
async function refreshPreview() {
    if (!selectedTable) {
        showStatusMessage('info', 'ℹ️ 请先选择一个表');
        return;
    }
    await loadTablePreview(selectedTable);
    switchToPreviewPage();
}

// ============================================================
// 加载表列表
// ============================================================
async function loadTables() {
    try {
        showStatusMessage('info', '⏳ 正在加载表列表...');

        const response = await fetch(API_BASE, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ operations: [{ type: 'tables' }] })
        });

        if (!response.ok) throw new Error(`HTTP错误! 状态码: ${response.status}`);

        const result = await response.json();

        if (result.success && result.results.length > 0) {
            tables = result.results[0].tables || [];
            renderTableList();
            updateTableCount();

            if (tables.length > 0 && !selectedTable) {
                await selectTable(tables[0]);
            } else if (tables.length > 0 && selectedTable) {
                if (!tables.includes(selectedTable)) {
                    await selectTable(tables[0]);
                } else {
                    await loadTableStructure(selectedTable);
                    await loadTablePreview(selectedTable);
                }
            } else if (tables.length === 0) {
                selectedTable = null;
                elements.tableDetails.innerHTML = `
                    <div class="empty-state">
                        <div class="empty-icon"><i class="fa-solid fa-table fa-2x"></i></div>
                        <h3>暂无数据表</h3>
                        <p>点击"创建表"按钮来创建您的第一个表</p>
                    </div>`;
                clearResults();
                document.getElementById('current-table-stats').textContent = '未选择表';
            }

            showStatusMessage('success', '✅ 成功加载 ' + tables.length + ' 个表');
        } else {
            showStatusMessage('error', '❌ 加载表列表失败');
        }
    } catch (error) {
        showStatusMessage('error', '❌ 加载表列表时出错: ' + error.message);
    }
}

// 更新表数量
function updateTableCount() {
    const tableCountElement = document.getElementById('table-count');
    if (tableCountElement) {
        tableCountElement.textContent = tables.length;
    }
}

// 渲染表列表
function renderTableList() {
    elements.tableList.innerHTML = '';

    if (tables.length === 0) {
        elements.tableList.innerHTML =
            '<p style="text-align:center;color:var(--text-on-glass-secondary);padding:20px;font-size:0.85rem;">暂无表</p>';
        return;
    }

    tables.forEach(tableName => {
        const tableItem = document.createElement('div');
        tableItem.className = `table-item ${selectedTable === tableName ? 'selected' : ''}`;
        tableItem.dataset.tableName = tableName;

        tableItem.innerHTML = `
            <div style="display:flex;justify-content:space-between;align-items:center;">
                <div class="table-name">${tableName}</div>
                <button class="delete-table-btn" data-table="${tableName}"><i class="fa-solid fa-trash-can"></i> 删除</button>
            </div>
            <div class="table-stats">加载中...</div>
        `;

        tableItem.addEventListener('click', (e) => {
            if (!e.target.closest('.delete-table-btn')) {
                selectTable(tableName);
            }
        });

        const deleteBtn = tableItem.querySelector('.delete-table-btn');
        deleteBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            showDeleteTableConfirm(tableName);
        });

        elements.tableList.appendChild(tableItem);
        loadTableStats(tableName, tableItem);
    });
}

// 加载表统计信息
async function loadTableStats(tableName, tableItem) {
    try {
        const response = await fetch(API_BASE, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ operations: [{ type: 'count', table: tableName }] })
        });

        const result = await response.json();

        if (result.success && result.results.length > 0) {
            const count = result.results[0].count || 0;
            const statsElement = tableItem.querySelector('.table-stats');
            if (statsElement) {
                statsElement.textContent = `${count} 条记录`;
            }
        }
    } catch (error) {
        console.error('加载表统计信息失败:', error);
        const statsElement = tableItem.querySelector('.table-stats');
        if (statsElement) statsElement.textContent = '—';
    }
}

// ============================================================
// 选择表（默认查看数据）
// ============================================================
async function selectTable(tableName) {
    selectedTable = tableName;

    document.querySelectorAll('.table-item').forEach(item => {
        item.classList.remove('selected');
        if (item.dataset.tableName === tableName) {
            item.classList.add('selected');
        }
    });

    const statsElement = document.getElementById('current-table-stats');
    if (statsElement) {
        statsElement.textContent = `已选择: ${tableName}`;
    }

    await loadTableStructure(tableName);
    await loadTablePreview(tableName);
    switchToPreviewPage();
}

function switchToPreviewPage() {
    const previewNavBtn = document.querySelector('.nav-btn[data-page="preview"]');
    if (previewNavBtn) {
        document.querySelectorAll('.nav-btn').forEach(b => b.classList.remove('active'));
        previewNavBtn.classList.add('active');
        document.querySelectorAll('.app-page').forEach(p => p.classList.remove('active'));
        const previewPage = document.querySelector('.app-page[data-page="preview"]');
        if (previewPage) previewPage.classList.add('active');
    }
}

// ============================================================
// 加载表结构
// ============================================================
async function loadTableStructure(tableName) {
    try {
        const response = await fetch(API_BASE, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ operations: [{ type: 'structure', table: tableName }] })
        });

        if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);

        const result = await response.json();

        if (result.success && result.results.length > 0) {
            const structure = result.results[0].structure || [];
            renderTableStructure(tableName, structure);
        } else {
            elements.tableDetails.innerHTML = '<p class="message error">加载表结构失败</p>';
        }
    } catch (error) {
        elements.tableDetails.innerHTML = `<p class="message error">加载表结构时出错: ${error.message}</p>`;
    }
}

// 渲染表结构 (使用 Font Awesome 图标)
function renderTableStructure(tableName, structure) {
    elements.tableDetails.innerHTML = `
        <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:0.8rem;flex-wrap:wrap;gap:0.5rem;">
            <h3 style="margin:0;"><i class="fa-solid fa-table"></i> ${tableName}</h3>
            <span style="font-size:0.8rem;color:var(--text-on-glass-secondary);background:rgba(255,255,255,0.25);padding:0.25rem 0.7rem;border-radius:999px;">
                ${structure.length} 个字段
            </span>
        </div>
        <div class="data-table-container">
            <table class="structure-table">
                <thead>
                    <tr>
                        <th>字段名</th>
                        <th>数据类型</th>
                        <th>是否为空</th>
                        <th>键类型</th>
                        <th>默认值</th>
                        <th>额外属性</th>
                    </tr>
                </thead>
                <tbody>
                    ${structure.map(col => `
                        <tr>
                            <td style="font-weight:600;">${col.field}</td>
                            <td>${col.type}</td>
                            <td>${col.null}</td>
                            <td>${col.key || '-'}</td>
                            <td>${col.default || '-'}</td>
                            <td>${col.extra || '-'}</td>
                        </tr>
                    `).join('')}
                </tbody>
            </table>
        </div>
    `;
}

// ============================================================
// 加载数据预览
// ============================================================
async function loadTablePreview(tableName) {
    try {
        if (!elements.previewContent) return;

        elements.previewContent.innerHTML = `
            <div style="display:flex;align-items:center;justify-content:center;padding:30px;gap:10px;">
                <div class="loading"></div>
                <span style="color:var(--text-on-glass-secondary);">加载数据预览...</span>
            </div>`;

        const response = await fetch(API_BASE, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                operations: [{
                    type: 'select',
                    table: tableName,
                    limit: 100
                }]
            })
        });

        if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);

        const result = await response.json();

        if (result.success && result.results.length > 0) {
            const selectResult = result.results[0];
            renderDataToPreview(selectResult, `✅ 预览成功，显示 ${selectResult.rows ? selectResult.rows.length : 0} 条记录`);
        } else {
            elements.previewContent.innerHTML = '<p class="message info">预览数据为空</p>';
            showStatusMessage('info', 'ℹ️ 该表暂无数据');
        }
    } catch (error) {
        console.error('加载表数据预览失败:', error);
        elements.previewContent.innerHTML = '<p class="message error">加载数据预览失败</p>';
        showStatusMessage('error', '❌ 加载预览失败: ' + error.message);
    }
}

// ============================================================
// 渲染数据到预览区
// ============================================================
function renderDataToPreview(result, statusMessage) {
    if (!elements.previewContent) return;

    if (statusMessage) {
        showStatusMessage('success', statusMessage);
    }

    if (!result.rows || result.rows.length === 0) {
        elements.previewContent.innerHTML = '<p class="message info">查询结果为空</p>';
        return;
    }

    const columns = result.columns || (result.rows.length > 0 ? Object.keys(result.rows[0]) : []);

    elements.previewContent.innerHTML = `
        <div class="data-table-container">
            <table class="data-table">
                <thead>
                    <tr>
                        ${columns.map(col => `<th>${col}</th>`).join('')}
                    </tr>
                </thead>
                <tbody>
                    ${result.rows.map(row => `
                        <tr>
                            ${columns.map(col => {
                                const val = row[col];
                                const display = val !== null && val !== undefined ? String(val) : '';
                                return `<td>${escapeHtml(display)}</td>`;
                            }).join('')}
                        </tr>
                    `).join('')}
                </tbody>
            </table>
        </div>
    `;
}

function escapeHtml(str) {
    if (typeof str !== 'string') return str;
    return str
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/\n/g, '<br>');
}

// ============================================================
// 状态消息
// ============================================================
function showStatusMessage(type, message) {
    if (!elements.previewStatus) return;
    const typeClass = type === 'success' ? 'success' : type === 'error' ? 'error' : 'info';
    elements.previewStatus.innerHTML = `<div class="message ${typeClass}">${message}</div>`;
    clearTimeout(elements._statusTimeout);
    if (type === 'success') {
        elements._statusTimeout = setTimeout(() => {
            if (elements.previewStatus) elements.previewStatus.innerHTML = '';
        }, 5000);
    }
}

// ============================================================
// 执行查询
// ============================================================
async function executeSelect() {
    if (!selectedTable) {
        showStatusMessage('error', '❌ 请先选择一个表');
        switchToPreviewPage();
        return;
    }

    try {
        showStatusMessage('info', '⏳ 正在执行查询...');

        let filter = {};
        let order = [];

        if (elements.selectFilter.value.trim()) {
            try {
                filter = JSON.parse(elements.selectFilter.value);
            } catch (e) {
                showStatusMessage('error', '❌ 过滤条件JSON格式错误');
                switchToPreviewPage();
                return;
            }
        }

        if (elements.selectOrder.value.trim()) {
            try {
                order = JSON.parse(elements.selectOrder.value);
            } catch (e) {
                showStatusMessage('error', '❌ 排序JSON格式错误');
                switchToPreviewPage();
                return;
            }
        }

        const limit = parseInt(elements.selectLimit.value) || 100;
        const offset = parseInt(elements.selectOffset.value) || 0;

        const response = await fetch(API_BASE, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                operations: [{
                    type: 'select',
                    table: selectedTable,
                    filter: filter,
                    order: order,
                    limit: limit,
                    offset: offset
                }]
            })
        });

        if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);

        const result = await response.json();

        if (result.success && result.results.length > 0) {
            const selectResult = result.results[0];
            renderDataToPreview(selectResult,
                `✅ 查询成功，找到 ${selectResult.rows ? selectResult.rows.length : 0} 条记录`);
        } else {
            showStatusMessage('error', `❌ 查询失败: ${result.error || '未知错误'}`);
            elements.previewContent.innerHTML = '<p class="message error">查询失败</p>';
        }
    } catch (error) {
        showStatusMessage('error', '❌ 查询时出错: ' + error.message);
        elements.previewContent.innerHTML = `<p class="message error">${error.message}</p>`;
    }
    switchToPreviewPage();
}

// ============================================================
// 执行插入
// ============================================================
async function executeInsert() {
    if (!selectedTable) {
        showStatusMessage('error', '❌ 请先选择一个表');
        switchToPreviewPage();
        return;
    }

    const dataStr = elements.insertData.value.trim();
    if (!dataStr) {
        showStatusMessage('error', '❌ 插入数据不能为空');
        switchToPreviewPage();
        return;
    }

    try {
        const data = JSON.parse(dataStr);
        showStatusMessage('info', '⏳ 正在执行插入...');

        const response = await fetch(API_BASE, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                operations: [{ type: 'insert', table: selectedTable, data: data }]
            })
        });

        if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);

        const result = await response.json();

        if (result.success) {
            showStatusMessage('success', '✅ 插入成功');
            elements.insertData.value = '';
            await loadTablePreview(selectedTable);
            refreshTableStatsInList();
        } else {
            showStatusMessage('error', '❌ 插入失败: ' + result.error);
        }
    } catch (error) {
        if (error instanceof SyntaxError) {
            showStatusMessage('error', '❌ 插入数据JSON格式错误');
        } else {
            showStatusMessage('error', '❌ 插入时出错: ' + error.message);
        }
    }
    switchToPreviewPage();
}

// ============================================================
// 执行更新
// ============================================================
async function executeUpdate() {
    if (!selectedTable) {
        showStatusMessage('error', '❌ 请先选择一个表');
        switchToPreviewPage();
        return;
    }

    const dataStr = elements.updateData.value.trim();
    const filterStr = elements.updateFilter.value.trim();

    if (!dataStr) {
        showStatusMessage('error', '❌ 更新数据不能为空');
        switchToPreviewPage();
        return;
    }

    try {
        const data = JSON.parse(dataStr);
        let filter = {};
        if (filterStr) filter = JSON.parse(filterStr);

        showStatusMessage('info', '⏳ 正在执行更新...');

        const response = await fetch(API_BASE, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                operations: [{ type: 'update', table: selectedTable, data: data, filter: filter }]
            })
        });

        if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);

        const result = await response.json();

        if (result.success) {
            showStatusMessage('success', '✅ 更新成功');
            await loadTablePreview(selectedTable);
            refreshTableStatsInList();
        } else {
            showStatusMessage('error', '❌ 更新失败: ' + result.error);
        }
    } catch (error) {
        if (error instanceof SyntaxError) {
            showStatusMessage('error', '❌ JSON格式错误');
        } else {
            showStatusMessage('error', '❌ 更新时出错: ' + error.message);
        }
    }
    switchToPreviewPage();
}

// ============================================================
// 删除确认
// ============================================================
function showDeleteConfirm() {
    if (!selectedTable) {
        showStatusMessage('error', '❌ 请先选择一个表');
        switchToPreviewPage();
        return;
    }

    const filterStr = elements.deleteFilter.value.trim();
    if (!filterStr) {
        showStatusMessage('error', '❌ 过滤条件不能为空');
        switchToPreviewPage();
        return;
    }

    elements.confirmMessage.textContent = `确定要删除 ${selectedTable} 表中符合条件的记录吗？此操作不可恢复！`;
    elements.confirmDialog.classList.add('show');
    elements.confirmActionBtn.onclick = confirmDelete;
}

function showDeleteTableConfirm(tableName) {
    elements.confirmMessage.textContent = `确定要删除整个「${tableName}」表吗？此操作不可恢复！`;
    elements.confirmDialog.classList.add('show');
    elements.confirmActionBtn.onclick = () => confirmDeleteTable(tableName);
}

function hideConfirmDialog() {
    elements.confirmDialog.classList.remove('show');
}

async function confirmDelete() {
    const filterStr = elements.deleteFilter.value.trim();

    try {
        const filter = JSON.parse(filterStr);
        showStatusMessage('info', '⏳ 正在执行删除...');

        const response = await fetch(API_BASE, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                operations: [{ type: 'delete', table: selectedTable, filter: filter }]
            })
        });

        if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);

        const result = await response.json();

        if (result.success) {
            showStatusMessage('success', '✅ 删除成功');
            hideConfirmDialog();
            await loadTablePreview(selectedTable);
            refreshTableStatsInList();
        } else {
            showStatusMessage('error', '❌ 删除失败: ' + result.error);
            hideConfirmDialog();
        }
    } catch (error) {
        if (error instanceof SyntaxError) {
            showStatusMessage('error', '❌ 过滤条件JSON格式错误');
        } else {
            showStatusMessage('error', '❌ 删除时出错: ' + error.message);
        }
        hideConfirmDialog();
    }
    switchToPreviewPage();
}

async function confirmDeleteTable(tableName) {
    try {
        showStatusMessage('info', '⏳ 正在删除表...');

        const response = await fetch(API_BASE, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ operations: [{ type: 'drop', table: tableName }] })
        });

        if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);

        const result = await response.json();

        if (result.success) {
            showStatusMessage('success', `✅ 表「${tableName}」删除成功`);
            hideConfirmDialog();
            if (selectedTable === tableName) {
                selectedTable = null;
                elements.tableDetails.innerHTML = `
                    <div class="empty-state">
                        <div class="empty-icon"><i class="fa-solid fa-table fa-2x"></i></div>
                        <h3>请选择一个表查看详细信息</h3>
                        <p>从左侧表列表中选择一个表</p>
                    </div>`;
                document.getElementById('current-table-stats').textContent = '未选择表';
                clearResults();
            }
            await loadTables();
        } else {
            showStatusMessage('error', '❌ 删除表失败: ' + result.error);
            hideConfirmDialog();
        }
    } catch (error) {
        showStatusMessage('error', '❌ 删除表时出错: ' + error.message);
        hideConfirmDialog();
    }
}

function refreshTableStatsInList() {
    const items = elements.tableList.querySelectorAll('.table-item');
    items.forEach(item => {
        const tableName = item.dataset.tableName;
        if (tableName) loadTableStats(tableName, item);
    });
}

// ============================================================
// 创建表对话框
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
    const columns = elements.columnsContainer.children;
    const columnIndex = columns.length + 1;

    const columnItem = document.createElement('div');
    columnItem.className = 'column-item';

    columnItem.innerHTML = `
        <div class="column-header">
            <span>列 ${columnIndex}</span>
            <button class="remove-column btn-glass btn-glass-danger btn-xs"><i class="fa-solid fa-trash-can"></i> 删除</button>
        </div>
        <div class="column-fields">
            <div class="form-row">
                <div class="form-group">
                    <label>列名</label>
                    <input type="text" class="column-name form-input glass-input" value="${isDefault ? 'id' : ''}">
                </div>
                <div class="form-group">
                    <label>数据类型</label>
                    <select class="column-type form-input glass-input">
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
                <div class="form-group checkbox-group">
                    <input type="checkbox" class="column-primary" ${isDefault ? 'checked' : ''}>
                    <label>主键</label>
                </div>
                <div class="form-group checkbox-group">
                    <input type="checkbox" class="column-autoinc" ${isDefault ? 'checked' : ''}>
                    <label>自增</label>
                </div>
                <div class="form-group checkbox-group">
                    <input type="checkbox" class="column-notnull" ${isDefault ? 'checked' : ''}>
                    <label>非空</label>
                </div>
            </div>
            <div class="form-group">
                <label>默认值</label>
                <input type="text" class="column-default form-input glass-input">
            </div>
        </div>
    `;

    elements.columnsContainer.appendChild(columnItem);

    const removeBtn = columnItem.querySelector('.remove-column');
    removeBtn.addEventListener('click', () => removeColumn(columnItem));
}

function removeColumn(columnItem) {
    if (elements.columnsContainer.children.length <= 1) {
        showStatusMessage('error', '❌ 至少需要保留一列');
        return;
    }
    columnItem.remove();

    const columns = elements.columnsContainer.children;
    for (let i = 0; i < columns.length; i++) {
        const header = columns[i].querySelector('.column-header span');
        if (header) header.textContent = `列 ${i + 1}`;
    }
}

async function createTable() {
    const tableName = elements.newTableName.value.trim();

    if (!tableName) {
        showStatusMessage('error', '❌ 表名不能为空');
        return;
    }

    const columns = elements.columnsContainer.children;
    const columnDefs = [];

    for (let i = 0; i < columns.length; i++) {
        const columnItem = columns[i];
        const name = columnItem.querySelector('.column-name')?.value.trim();
        const type = columnItem.querySelector('.column-type')?.value;
        const isPrimary = columnItem.querySelector('.column-primary')?.checked;
        const isAutoinc = columnItem.querySelector('.column-autoinc')?.checked;
        const isNotNull = columnItem.querySelector('.column-notnull')?.checked;
        const defaultValue = columnItem.querySelector('.column-default')?.value.trim();

        if (!name) continue;

        const columnDef = {
            name: name,
            type: type,
            primary_key: isPrimary,
            auto_increment: isAutoinc,
            not_null: isNotNull
        };

        if (defaultValue) {
            columnDef.default = (type === 'TEXT' || type === 'TIMESTAMP') ?
                defaultValue :
                isNaN(defaultValue) ? defaultValue : Number(defaultValue);
        }

        columnDefs.push(columnDef);
    }

    if (columnDefs.length === 0) {
        showStatusMessage('error', '❌ 至少需要定义一个列');
        return;
    }

    try {
        showStatusMessage('info', '⏳ 正在创建表...');

        const response = await fetch(API_BASE, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                operations: [{
                    type: 'create',
                    table: tableName,
                    definition: { columns: columnDefs }
                }]
            })
        });

        if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);

        const result = await response.json();

        if (result.success) {
            showStatusMessage('success', `✅ 表「${tableName}」创建成功`);
            hideCreateTableDialog();
            await loadTables();
        } else {
            showStatusMessage('error', '❌ 创建表失败: ' + result.error);
        }
    } catch (error) {
        showStatusMessage('error', '❌ 创建表时出错: ' + error.message);
    }
}

// ============================================================
// 操作符说明对话框
// ============================================================
function showOperatorsDialog() {
    elements.operatorsDialog.classList.add('show');
}

function hideOperatorsDialog() {
    elements.operatorsDialog.classList.remove('show');
}

// ============================================================
// 页面与标签页切换
// ============================================================
function switchPage(e) {
    let navBtn = e.target;
    while (navBtn && !navBtn.dataset.page) {
        navBtn = navBtn.parentElement;
        if (!navBtn) return;
    }

    const pageName = navBtn.dataset.page;
    if (!pageName) return;

    document.querySelectorAll('.nav-btn').forEach(btn => btn.classList.remove('active'));
    navBtn.classList.add('active');

    document.querySelectorAll('.app-page').forEach(page => page.classList.remove('active'));
    const page = document.querySelector(`.app-page[data-page="${pageName}"]`);
    if (page) page.classList.add('active');
}

function switchTab(e) {
    let tabBtn = e.target;
    while (tabBtn && !tabBtn.dataset.tab) {
        tabBtn = tabBtn.parentElement;
        if (!tabBtn) return;
    }

    const tabName = tabBtn.dataset.tab;
    if (!tabName) return;

    elements.tabBtns.forEach(btn => btn.classList.remove('active'));
    tabBtn.classList.add('active');

    elements.tabContents.forEach(content => content.classList.remove('active'));
    const tabContent = document.getElementById(`${tabName}-content`);
    if (tabContent) tabContent.classList.add('active');
}

// ============================================================
// 事件绑定
// ============================================================
function bindEvents() {
    elements.refreshTablesBtn.addEventListener('click', loadTables);

    elements.createTableBtn.addEventListener('click', showCreateTableDialog);
    elements.cancelCreateTableBtn.addEventListener('click', hideCreateTableDialog);
    elements.confirmCreateTableBtn.addEventListener('click', createTable);
    elements.addColumnBtn.addEventListener('click', () => addColumn(false));

    elements.tabBtns.forEach(btn => btn.addEventListener('click', switchTab));

    elements.executeSelectBtn.addEventListener('click', executeSelect);
    elements.executeInsertBtn.addEventListener('click', executeInsert);
    elements.executeUpdateBtn.addEventListener('click', executeUpdate);
    elements.executeDeleteBtn.addEventListener('click', showDeleteConfirm);

    elements.cancelConfirmBtn.addEventListener('click', hideConfirmDialog);

    elements.showOperatorsBtn.addEventListener('click', showOperatorsDialog);
    elements.closeOperatorsBtn.addEventListener('click', hideOperatorsDialog);

    document.querySelectorAll('.nav-btn').forEach(btn => btn.addEventListener('click', switchPage));

    document.querySelectorAll('.dialog').forEach(dialog => {
        dialog.addEventListener('click', function (e) {
            if (e.target === this) {
                this.classList.remove('show');
            }
        });
    });
}

// ============================================================
// 启动
// ============================================================
init();