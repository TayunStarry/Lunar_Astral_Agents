// ============================================================
// 星月智能 · 知识库 — main.js
// 事件绑定与启动
// ============================================================

function bindEvents() {
    // 数据库探查（重新扫描）
    elements.btnScanDb.addEventListener('click', scanDatabases);

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

    // 右侧主视图：页签切换（即时查询，避免陈旧引用）+ 内容刷新
    document.querySelectorAll('.tab').forEach(tab => {
        tab.addEventListener('click', () => switchSubView(tab.dataset.tab));
    });
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
function init() {
    bindEvents();
    refreshTables();
    updateDetectState();
    switchSubView('content');   // 默认显示「内容显示」
    scanDatabases();            // 数据库探查自动执行（按钮仅用于重新扫描）
}

init();
