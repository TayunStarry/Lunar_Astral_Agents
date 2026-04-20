// 全局变量
let selectedTable = null;
let tables = [];

// DOM元素
const elements = {
    // 表相关
    tableList: document.getElementById('table-list'),
    tableDetails: document.getElementById('table-details'),
    tablePreview: document.getElementById('table-preview'),
    previewContent: document.getElementById('preview-content'),
    refreshTablesBtn: document.getElementById('refresh-tables'),
    createTableBtn: document.getElementById('create-table'),

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

    // 结果显示
    resultContent: document.getElementById('result-content'),

    // 创建表对话框
    createTableDialog: document.getElementById('create-table-dialog'),
    newTableName: document.getElementById('new-table-name'),
    cancelCreateTableBtn: document.getElementById('cancel-create-table'),
    confirmCreateTableBtn: document.getElementById('confirm-create-table'),
    addColumnBtn: document.getElementById('add-column'),
    columnsContainer: document.getElementById('columns-container'),
    generatedSql: document.getElementById('generated-sql'),

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
const API_BASE = '/database/';

// 初始化
function init() {
    bindEvents();
    loadTables();

    // 新增：绑定清除结果按钮
    const clearResultsBtn = document.getElementById('clear-results');
    if (clearResultsBtn) {
        clearResultsBtn.addEventListener('click', clearResults);
    }

    // 新增：绑定表数量更新
    updateTableCount();
}

// 新增：清除结果功能
function clearResults() {
    elements.resultContent.innerHTML = `
        <div class="empty-state">
            <div class="empty-icon">📈</div>
            <h3>结果已清除</h3>
            <p>执行操作后，新结果将显示在这里</p>
        </div>
    `;
}

// 修改：更新表列表时同时更新数量和预览
async function loadTables() {
    try {
        showLoading('加载表列表...');
        elements.resultContent.innerHTML = '<div style="display: flex; align-items: center; justify-content: center; padding: 20px;"><div class="loading"></div> <span style="margin-left: 10px;">加载表列表...</span></div>';

        const response = await fetch(API_BASE, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                operations: [{
                    type: 'tables'
                }]
            })
        });

        if (!response.ok) {
            throw new Error(`HTTP错误! 状态码: ${response.status}`);
        }

        const result = await response.json();

        if (result.success && result.results.length > 0) {
            tables = result.results[0].tables || [];
            renderTableList();
            updateTableCount();

            // 如果有表且没有选中的表，自动选中第一个
            if (tables.length > 0 && !selectedTable) {
                selectTable(tables[0]);
            } else if (tables.length > 0 && selectedTable) {
                // 如果当前有选中的表，重新加载其数据预览
                await loadTableStructure(selectedTable);
                await loadTablePreview(selectedTable);
            }

            showSuccess(`成功加载 ${tables.length} 个表`);
        } else {
            showError('加载表列表失败');
        }
    } catch (error) {
        showError(`加载表列表时出错: ${error.message}`);
    } finally {
        hideLoading();
    }
}

// 新增：更新表数量显示
function updateTableCount() {
    const tableCountElement = document.getElementById('table-count');
    if (tableCountElement) {
        tableCountElement.textContent = tables.length;
    }
}

// 修改：选择表时更新统计信息
async function selectTable(tableName) {
    selectedTable = tableName;

    // 更新UI
    document.querySelectorAll('.table-item').forEach(item => {
        item.classList.remove('selected');
        if (item.dataset.tableName === tableName) {
            item.classList.add('selected');
        }
    });

    // 更新当前表统计信息
    const statsElement = document.getElementById('current-table-stats');
    if (statsElement) {
        statsElement.textContent = `已选择: ${tableName}`;
    }

    // 加载表结构
    await loadTableStructure(tableName);
    // 加载表数据预览
    await loadTablePreview(tableName);
}

// 绑定事件
function bindEvents() {
    // 刷新表列表
    elements.refreshTablesBtn.addEventListener('click', loadTables);

    // 创建表
    elements.createTableBtn.addEventListener('click', showCreateTableDialog);
    elements.cancelCreateTableBtn.addEventListener('click', hideCreateTableDialog);
    elements.confirmCreateTableBtn.addEventListener('click', createTable);
    elements.addColumnBtn.addEventListener('click', addColumn);

    // 标签页切换
    elements.tabBtns.forEach(btn => {
        btn.addEventListener('click', switchTab);
    });

    // 数据操作
    elements.executeSelectBtn.addEventListener('click', executeSelect);
    elements.executeInsertBtn.addEventListener('click', executeInsert);
    elements.executeUpdateBtn.addEventListener('click', executeUpdate);
    elements.executeDeleteBtn.addEventListener('click', showDeleteConfirm);

    // 确认对话框
    elements.cancelConfirmBtn.addEventListener('click', hideConfirmDialog);
    elements.confirmActionBtn.addEventListener('click', confirmDelete);

    // 高级查询操作符说明
    elements.showOperatorsBtn.addEventListener('click', showOperatorsDialog);
    elements.closeOperatorsBtn.addEventListener('click', hideOperatorsDialog);

    // 页面导航
    document.querySelectorAll('.nav-btn').forEach(btn => {
        btn.addEventListener('click', switchPage);
    });

    // 表名变化时更新SQL语句
    elements.newTableName.addEventListener('input', updateGeneratedSql);
}

// 渲染表列表
function renderTableList() {
    elements.tableList.innerHTML = '';

    if (tables.length === 0) {
        elements.tableList.innerHTML = '<p style="text-align: center; color: #999; padding: 20px;">暂无表</p>';
        return;
    }

    tables.forEach(tableName => {
        const tableItem = document.createElement('div');
        tableItem.className = `table-item ${selectedTable === tableName ? 'selected' : ''}`;
        tableItem.dataset.tableName = tableName;

        tableItem.innerHTML = `
            <div style="display: flex; justify-content: space-between; align-items: center;">
                <div class="table-name">${tableName}</div>
                <button class="delete-table-btn" data-table="${tableName}" style="background: #ff4757; color: white; border: none; border-radius: 4px; padding: 4px 8px; cursor: pointer; font-size: 0.8em;">删除</button>
            </div>
            <div class="table-stats">加载中...</div>
        `;

        tableItem.addEventListener('click', (e) => {
            if (!e.target.classList.contains('delete-table-btn')) {
                selectTable(tableName);
            }
        });

        const deleteBtn = tableItem.querySelector('.delete-table-btn');
        deleteBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            showDeleteTableConfirm(tableName);
        });

        elements.tableList.appendChild(tableItem);

        // 加载表统计信息
        loadTableStats(tableName, tableItem);
    });
}

// 加载表统计信息
async function loadTableStats(tableName, tableItem) {
    try {
        const response = await fetch(API_BASE, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                operations: [{
                    type: 'count',
                    table: tableName
                }]
            })
        });

        const result = await response.json();

        if (result.success && result.results.length > 0) {
            const count = result.results[0].count || 0;
            const statsElement = tableItem.querySelector('.table-stats');
            statsElement.textContent = `${count} 条记录`;
        }
    } catch (error) {
        console.error('加载表统计信息失败:', error);
    }
}

// 加载表数据预览
async function loadTablePreview(tableName) {
    try {
        // 检查必要的DOM元素是否存在
        if (!elements.previewContent || !elements.tablePreview) {
            console.warn('预览相关DOM元素不存在');
            return;
        }

        elements.previewContent.innerHTML = '<div style="display: flex; align-items: center; justify-content: center; padding: 20px;"><div class="loading"></div> <span style="margin-left: 10px;">加载数据预览...</span></div>';
        elements.tablePreview.style.display = 'block';

        const response = await fetch(API_BASE, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                operations: [{
                    type: 'select',
                    table: tableName,
                    limit: 10 // 只预览前10条数据
                }]
            })
        });

        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }

        const result = await response.json();

        if (result.success && result.results.length > 0) {
            const selectResult = result.results[0];
            renderTablePreview(selectResult);
        } else {
            elements.previewContent.innerHTML = '<p class="message info">预览数据为空</p>';
        }
    } catch (error) {
        console.error('加载表数据预览失败:', error);
        if (elements.previewContent) {
            elements.previewContent.innerHTML = '<p class="message error">加载数据预览失败</p>';
        }
    }
}

// 处理换行符，将\n转换为<br>
function handleNewlines(text) {
    if (typeof text === 'string') {
        return text.replace(/\n/g, '<br>');
    }
    return text;
}

// 渲染表数据预览
function renderTablePreview(result) {
    if (!elements.previewContent) {
        console.warn('预览内容DOM元素不存在');
        return;
    }

    if (!result.rows || result.rows.length === 0) {
        elements.previewContent.innerHTML = '<p class="message info">预览数据为空</p>';
        return;
    }

    // 获取列名，优先使用result.columns，如果不存在则从第一行数据中提取
    const columns = result.columns || (result.rows.length > 0 ? Object.keys(result.rows[0]) : []);

    let html = '<div class="preview-table-container"><table class="preview-table"><thead><tr>';

    // 渲染表头
    columns.forEach(column => {
        html += `<th>${column}</th>`;
    });

    html += '</tr></thead><tbody>';

    // 渲染数据行
    result.rows.forEach(row => {
        html += '<tr>';
        columns.forEach(column => {
            const value = row[column] !== null && row[column] !== undefined ? row[column] : '';
            html += `<td>${handleNewlines(value)}</td>`;
        });
        html += '</tr>';
    });

    html += '</tbody></table></div>';

    elements.previewContent.innerHTML = `
        <div class="message success">预览成功，显示前 ${result.rows.length} 条记录</div>
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
                            ${columns.map(col => `<td>${handleNewlines(row[col] || '')}</td>`).join('')}
                        </tr>
                    `).join('')}
                </tbody>
            </table>
        </div>
    `;
}

// 显示高级查询操作符说明对话框
function showOperatorsDialog() {
    elements.operatorsDialog.classList.add('show');
}

// 隐藏高级查询操作符说明对话框
function hideOperatorsDialog() {
    elements.operatorsDialog.classList.remove('show');
}

// 加载表结构
async function loadTableStructure(tableName) {
    try {
        showLoading('加载表结构...');

        const response = await fetch(API_BASE, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                operations: [{
                    type: 'structure',
                    table: tableName
                }]
            })
        });

        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }

        const result = await response.json();

        if (result.success && result.results.length > 0) {
            const structure = result.results[0].structure || [];
            renderTableStructure(tableName, structure);
        } else {
            showError('加载表结构失败');
        }
    } catch (error) {
        showError(`加载表结构时出错: ${error.message}`);
    } finally {
        hideLoading();
    }
}

// 渲染表结构
function renderTableStructure(tableName, structure) {
    elements.tableDetails.innerHTML = `
        <div style="display: flex; justify-content: space-between; align-items: center;">
            <h3>${tableName}</h3>
            <div style="font-size: 0.9em; color: #666;">
                ${structure.length} 个字段
            </div>
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
                    ${structure.map((column, index) => `
                        <tr ${index % 2 === 0 ? 'style="background-color: #f9f9f9;"' : ''}>
                            <td style="font-weight: 500;">${column.field}</td>
                            <td>${column.type}</td>
                            <td>${column.null}</td>
                            <td>${column.key || '-'}</td>
                            <td>${column.default || '-'}</td>
                            <td>${column.extra || '-'}</td>
                        </tr>
                    `).join('')}
                </tbody>
            </table>
        </div>
        <div style="margin-top: 10px; font-size: 0.9em; color: #666;">
            <p><strong>表结构说明：</strong></p>
            <ul>
                <li><strong>字段名：</strong>表中的列名</li>
                <li><strong>数据类型：</strong>字段的数据类型</li>
                <li><strong>是否为空：</strong>字段是否允许为空</li>
                <li><strong>键类型：</strong>字段的键类型，如PRI（主键）</li>
                <li><strong>默认值：</strong>字段的默认值</li>
                <li><strong>额外属性：</strong>字段的额外属性，如AUTOINCREMENT</li>
            </ul>
        </div>
    `;
}

// 显示创建表对话框
function showCreateTableDialog() {
    elements.newTableName.value = '';
    elements.generatedSql.value = '';

    // 清空列容器，只保留默认列
    elements.columnsContainer.innerHTML = '';
    addColumn(true); // 添加默认列

    elements.createTableDialog.classList.add('show');
    updateGeneratedSql();
}

// 隐藏创建表对话框
function hideCreateTableDialog() {
    elements.createTableDialog.classList.remove('show');
}

// 添加列
function addColumn(isDefault = false) {
    const columns = elements.columnsContainer.children;
    const columnIndex = columns.length + 1;

    const columnItem = document.createElement('div');
    columnItem.className = 'column-item';

    columnItem.innerHTML = `
        <div class="column-header">
            <span>列 ${columnIndex}</span>
            <button class="remove-column btn-danger">删除</button>
        </div>
        <div class="column-fields">
            <div class="form-row">
                <div class="form-group">
                    <label>列名</label>
                    <input type="text" class="column-name" value="${isDefault ? 'id' : ''}">
                </div>
                <div class="form-group">
                    <label>数据类型</label>
                    <select class="column-type">
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
                <input type="text" class="column-default">
            </div>
        </div>
    `;

    elements.columnsContainer.appendChild(columnItem);

    // 绑定删除列事件
    const removeBtn = columnItem.querySelector('.remove-column');
    removeBtn.addEventListener('click', function () {
        removeColumn(columnItem);
    });

    // 绑定列属性变化事件
    const columnFields = columnItem.querySelectorAll('input, select');
    columnFields.forEach(field => {
        field.addEventListener('input', updateGeneratedSql);
        field.addEventListener('change', updateGeneratedSql);
    });

    updateGeneratedSql();
}

// 删除列
function removeColumn(columnItem) {
    if (elements.columnsContainer.children.length <= 1) {
        showError('至少需要保留一列');
        return;
    }

    columnItem.remove();

    // 更新列序号
    const columns = elements.columnsContainer.children;
    for (let i = 0; i < columns.length; i++) {
        const header = columns[i].querySelector('.column-header span');
        header.textContent = `列 ${i + 1}`;
    }

    updateGeneratedSql();
}

// 更新生成的SQL语句
function updateGeneratedSql() {
    const tableName = elements.newTableName.value.trim() || 'table_name';
    const columns = elements.columnsContainer.children;

    let columnsSql = [];

    for (let i = 0; i < columns.length; i++) {
        const columnItem = columns[i];
        const name = columnItem.querySelector('.column-name').value.trim();
        const type = columnItem.querySelector('.column-type').value;
        const isPrimary = columnItem.querySelector('.column-primary').checked;
        const isAutoinc = columnItem.querySelector('.column-autoinc').checked;
        const isNotNull = columnItem.querySelector('.column-notnull').checked;
        const defaultValue = columnItem.querySelector('.column-default').value.trim();

        if (!name) continue;

        let columnSql = `${name} ${type}`;

        if (isNotNull) {
            columnSql += ' NOT NULL';
        }

        if (defaultValue) {
            if (type === 'TEXT' || type === 'TIMESTAMP') {
                columnSql += ` DEFAULT '${defaultValue}'`;
            } else {
                columnSql += ` DEFAULT ${defaultValue}`;
            }
        }

        if (isPrimary) {
            columnSql += ' PRIMARY KEY';
            if (isAutoinc) {
                columnSql += ' AUTOINCREMENT';
            }
        }

        columnsSql.push(columnSql);
    }

    if (columnsSql.length === 0) {
        elements.generatedSql.value = '';
        return;
    }

    const createSql = `CREATE TABLE ${tableName} (\n    ${columnsSql.join(',\n    ')}\n)`;
    elements.generatedSql.value = createSql;
}

// 创建表
async function createTable() {
    const tableName = elements.newTableName.value.trim();

    if (!tableName) {
        showError('表名不能为空');
        return;
    }

    // 构建表定义
    const columns = elements.columnsContainer.children;
    const columnDefs = [];

    for (let i = 0; i < columns.length; i++) {
        const columnItem = columns[i];
        const name = columnItem.querySelector('.column-name').value.trim();
        const type = columnItem.querySelector('.column-type').value;
        const isPrimary = columnItem.querySelector('.column-primary').checked;
        const isAutoinc = columnItem.querySelector('.column-autoinc').checked;
        const isNotNull = columnItem.querySelector('.column-notnull').checked;
        const defaultValue = columnItem.querySelector('.column-default').value.trim();

        if (!name) continue;

        const columnDef = {
            name: name,
            type: type,
            primary_key: isPrimary,
            auto_increment: isAutoinc,
            not_null: isNotNull
        };

        if (defaultValue) {
            columnDef.default = type === 'TEXT' || type === 'TIMESTAMP' ? defaultValue : isNaN(defaultValue) ? defaultValue : Number(defaultValue);
        }

        columnDefs.push(columnDef);
    }

    if (columnDefs.length === 0) {
        showError('至少需要定义一个列');
        return;
    }

    try {
        showLoading('创建表...');

        const response = await fetch(API_BASE, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                operations: [{
                    type: 'create',
                    table: tableName,
                    definition: {
                        columns: columnDefs
                    }
                }]
            })
        });

        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }

        const result = await response.json();

        if (result.success) {
            showSuccess('表创建成功');
            hideCreateTableDialog();
            loadTables();
        } else {
            showError(`创建表失败: ${result.error}`);
        }
    } catch (error) {
        showError(`创建表时出错: ${error.message}`);
    } finally {
        hideLoading();
    }
}

// 切换标签页
// 切换页面
function switchPage(e) {
    // 找到实际的导航按钮元素（可能点击的是子元素）
    let navBtn = e.target;
    while (navBtn && !navBtn.dataset.page) {
        navBtn = navBtn.parentElement;
        if (!navBtn) return;
    }

    const pageName = navBtn.dataset.page;
    if (!pageName) {
        console.warn('导航按钮缺少data-page属性');
        return;
    }

    // 更新导航按钮状态
    document.querySelectorAll('.nav-btn').forEach(btn => {
        btn.classList.remove('active');
    });
    navBtn.classList.add('active');

    // 更新页面内容
    document.querySelectorAll('.app-page').forEach(page => {
        page.classList.remove('active');
    });
    const page = document.querySelector(`.app-page[data-page="${pageName}"]`);
    if (page) {
        page.classList.add('active');
    } else {
        console.warn(`找不到页面元素: ${pageName}`);
    }
}

// 切换标签页
function switchTab(e) {
    // 找到实际的标签按钮元素（可能点击的是子元素）
    let tabBtn = e.target;
    while (tabBtn && !tabBtn.dataset.tab) {
        tabBtn = tabBtn.parentElement;
        if (!tabBtn) return;
    }

    const tabName = tabBtn.dataset.tab;
    if (!tabName) {
        console.warn('标签按钮缺少data-tab属性');
        return;
    }

    // 更新标签按钮状态
    elements.tabBtns.forEach(btn => {
        btn.classList.remove('active');
    });
    tabBtn.classList.add('active');

    // 更新标签内容
    elements.tabContents.forEach(content => {
        content.classList.remove('active');
    });
    const tabContent = document.getElementById(`${tabName}-content`);
    if (tabContent) {
        tabContent.classList.add('active');
    } else {
        console.warn(`找不到标签内容元素: ${tabName}-content`);
    }
}

// 执行查询
async function executeSelect() {
    if (!selectedTable) {
        showError('请先选择一个表');
        return;
    }

    try {
        showLoading('执行查询...');

        let filter = {};
        let order = [];

        // 解析过滤条件
        if (elements.selectFilter.value.trim()) {
            try {
                filter = JSON.parse(elements.selectFilter.value);
            } catch (e) {
                showError('过滤条件JSON格式错误');
                return;
            }
        }

        // 解析排序
        if (elements.selectOrder.value.trim()) {
            try {
                order = JSON.parse(elements.selectOrder.value);
            } catch (e) {
                showError('排序JSON格式错误');
                return;
            }
        }

        const limit = parseInt(elements.selectLimit.value) || 100;
        const offset = parseInt(elements.selectOffset.value) || 0;

        const response = await fetch(API_BASE, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
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

        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }

        const result = await response.json();

        if (result.success && result.results.length > 0) {
            const selectResult = result.results[0];
            renderSelectResult(selectResult);
        } else {
            showError(`查询失败: ${result.error}`);
        }
    } catch (error) {
        showError(`查询时出错: ${error.message}`);
    } finally {
        hideLoading();
    }
}

// 渲染查询结果
function renderSelectResult(result) {
    if (!result.rows || result.rows.length === 0) {
        elements.resultContent.innerHTML = '<p class="message info">查询结果为空</p>';
        return;
    }

    const columns = Object.keys(result.rows[0]);

    elements.resultContent.innerHTML = `
        <div class="message success">查询成功，找到 ${result.rows.length} 条记录</div>
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
                            ${columns.map(col => `<td>${handleNewlines(row[col] || '')}</td>`).join('')}
                        </tr>
                    `).join('')}
                </tbody>
            </table>
        </div>
    `;
}

// 执行插入
async function executeInsert() {
    if (!selectedTable) {
        showError('请先选择一个表');
        return;
    }

    const dataStr = elements.insertData.value.trim();
    if (!dataStr) {
        showError('插入数据不能为空');
        return;
    }

    try {
        const data = JSON.parse(dataStr);
        showLoading('执行插入...');

        const response = await fetch(API_BASE, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                operations: [{
                    type: 'insert',
                    table: selectedTable,
                    data: data
                }]
            })
        });

        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }

        const result = await response.json();

        if (result.success) {
            showSuccess('插入成功');
            // 清空表单
            elements.insertData.value = '';
        } else {
            showError(`插入失败: ${result.error}`);
        }
    } catch (error) {
        if (error instanceof SyntaxError) {
            showError('插入数据JSON格式错误');
        } else {
            showError(`插入时出错: ${error.message}`);
        }
    } finally {
        hideLoading();
    }
}

// 执行更新
async function executeUpdate() {
    if (!selectedTable) {
        showError('请先选择一个表');
        return;
    }

    const dataStr = elements.updateData.value.trim();
    const filterStr = elements.updateFilter.value.trim();

    if (!dataStr) {
        showError('更新数据不能为空');
        return;
    }

    try {
        const data = JSON.parse(dataStr);
        let filter = {};

        if (filterStr) {
            filter = JSON.parse(filterStr);
        }

        showLoading('执行更新...');

        const response = await fetch(API_BASE, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                operations: [{
                    type: 'update',
                    table: selectedTable,
                    data: data,
                    filter: filter
                }]
            })
        });

        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }

        const result = await response.json();

        if (result.success) {
            showSuccess('更新成功');
        } else {
            showError(`更新失败: ${result.error}`);
        }
    } catch (error) {
        if (error instanceof SyntaxError) {
            showError('JSON格式错误');
        } else {
            showError(`更新时出错: ${error.message}`);
        }
    } finally {
        hideLoading();
    }
}

// 显示删除确认对话框（记录）
function showDeleteConfirm() {
    if (!selectedTable) {
        showError('请先选择一个表');
        return;
    }

    const filterStr = elements.deleteFilter.value.trim();
    if (!filterStr) {
        showError('过滤条件不能为空');
        return;
    }

    elements.confirmMessage.textContent = `确定要删除 ${selectedTable} 表中符合条件的记录吗？`;
    elements.confirmDialog.classList.add('show');
    elements.confirmActionBtn.onclick = confirmDelete;
}

// 显示删除表确认对话框
function showDeleteTableConfirm(tableName) {
    elements.confirmMessage.textContent = `确定要删除 ${tableName} 表吗？此操作不可恢复！`;
    elements.confirmDialog.classList.add('show');
    elements.confirmActionBtn.onclick = () => confirmDeleteTable(tableName);
}

// 隐藏确认对话框
function hideConfirmDialog() {
    elements.confirmDialog.classList.remove('show');
}

// 确认删除记录
async function confirmDelete() {
    const filterStr = elements.deleteFilter.value.trim();

    try {
        const filter = JSON.parse(filterStr);
        showLoading('执行删除...');

        const response = await fetch(API_BASE, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                operations: [{
                    type: 'delete',
                    table: selectedTable,
                    filter: filter
                }]
            })
        });

        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }

        const result = await response.json();

        if (result.success) {
            showSuccess('删除成功');
            hideConfirmDialog();
        } else {
            showError(`删除失败: ${result.error}`);
        }
    } catch (error) {
        if (error instanceof SyntaxError) {
            showError('过滤条件JSON格式错误');
        } else {
            showError(`删除时出错: ${error.message}`);
        }
    } finally {
        hideLoading();
    }
}

// 确认删除表
async function confirmDeleteTable(tableName) {
    try {
        showLoading('删除表...');

        const response = await fetch(API_BASE, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                operations: [{
                    type: 'drop',
                    table: tableName
                }]
            })
        });

        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }

        const result = await response.json();

        if (result.success) {
            showSuccess('表删除成功');
            hideConfirmDialog();
            // 重新加载表列表
            loadTables();
            // 如果删除的是当前选中的表，清空选中状态
            if (selectedTable === tableName) {
                selectedTable = null;
                elements.tableDetails.innerHTML = '<p>请选择一个表查看详细信息</p>';
            }
        } else {
            showError(`删除表失败: ${result.error}`);
        }
    } catch (error) {
        showError(`删除表时出错: ${error.message}`);
    } finally {
        hideLoading();
    }
}

// 显示加载状态
function showLoading(message) {
    elements.resultContent.innerHTML = `<div style="display: flex; align-items: center; justify-content: center; padding: 40px;"><div class="loading"></div> <span style="margin-left: 10px;">${message}</span></div>`;
}

// 隐藏加载状态
function hideLoading() {
    // 加载状态会被后续的结果或错误消息替换
}

// 显示成功消息
function showSuccess(message) {
    elements.resultContent.innerHTML = `<div class="message success">${message}</div>`;
}

// 显示错误消息
function showError(message) {
    elements.resultContent.innerHTML = `<div class="message error">${message}</div>`;
}

// 显示结果
function showResult(result) {
    elements.resultContent.innerHTML = `<pre>${JSON.stringify(result, null, 2)}</pre>`;
}

// 初始化应用
init();