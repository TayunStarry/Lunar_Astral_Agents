// ============================================================
// 星月智能 · 记忆库 — 主逻辑脚本 v3
// 前后端协调升级：持久化存储、智能同步、人机交互优化
// ============================================================

var API = {
    MESSAGES:   '/vector/lunar_messages/messages',
    STATS:      '/vector/lunar_messages/stats',
    INIT:       '/vector/init',
    COLLECTION: '/vector/lunar_messages',
    DOCS:       '/vector/lunar_messages/documents',
    REBUILD:    '/vector/lunar_messages/rebuild'
};

var PAGE_SIZE = 12;

var App = {
    initialized: false,
    page: 1,
    totalPages: 1,
    totalDocs: 0,
    entryCount: 0,
    syncing: false,
    searchMode: false,
    searchQuery: '',
    documents: [],
    searchResults: []
};

var Cache = {
    pendingDelete: null
};

function $(id) {
    return document.getElementById(id);
}

var D = {
    hdrCount:     $('hdr-count'),
    hdrEntry:     $('hdr-entry'),
    hdrDot:       $('hdr-status-dot'),
    hdrLabel:     $('hdr-status-label'),
    syncWarn:     $('sync-warn'),
    btnRefresh:   $('btn-refresh'),
    btnRebuild:   $('btn-rebuild'),

    initCard:     $('init-card'),
    initBase:     $('init-base-url'),
    initKey:      $('init-api-key'),
    initModel:    $('init-model-name'),
    btnInit:      $('btn-init'),

    searchInput:  $('search-input'),
    btnSearch:    $('btn-search'),
    btnClearSearch: $('btn-clear-search'),
    btnAdd:       $('btn-add'),

    addPanel:     $('add-panel'),
    addRole:      $('add-role'),
    addContent:   $('add-content'),
    btnAddSubmit: $('btn-add-submit'),
    btnAddCancel: $('btn-add-cancel'),

    contentArea:  $('content-area'),
    loadingState: $('loading-state'),
    emptyState:   $('empty-state'),
    errorState:   $('error-state'),
    docList:      $('doc-list'),

    pagination:   $('pagination'),
    btnPrev:      $('btn-prev'),
    btnNext:      $('btn-next'),
    pageInfo:     $('page-info'),

    modalOverlay: $('modal-overlay'),
    modalTitle:   $('modal-title'),
    modalBody:    $('modal-body'),
    modalConfirm: $('modal-confirm'),
    modalCancel:  $('modal-cancel'),

    toastContainer: $('toast-container')
};

function init() {
    bindGlobalEvents();
    bindKeyboard();
    loadStats();
}

function bindGlobalEvents() {
    D.btnRefresh.addEventListener('click', loadStats);
    D.btnRebuild.addEventListener('click', handleRebuild);
    D.btnInit.addEventListener('click', handleInit);

    D.searchInput.addEventListener('input', debounce(handleSearchInput, 350));
    D.searchInput.addEventListener('keydown', function (e) {
        if (e.key === 'Enter') commitSearch();
        if (e.key === 'Escape') { D.searchInput.value = ''; clearSearch(); }
    });
    D.btnSearch.addEventListener('click', commitSearch);
    D.btnClearSearch.addEventListener('click', clearSearch);

    D.btnAdd.addEventListener('click', showAddPanel);
    D.btnAddCancel.addEventListener('click', hideAddPanel);
    D.btnAddSubmit.addEventListener('click', handleAdd);
    D.addContent.addEventListener('keydown', function (e) {
        if (e.ctrlKey && e.key === 'Enter') handleAdd();
        if (e.key === 'Escape') hideAddPanel();
    });

    D.btnPrev.addEventListener('click', goPrevPage);
    D.btnNext.addEventListener('click', goNextPage);

    D.modalCancel.addEventListener('click', closeModal);
    D.modalOverlay.addEventListener('click', function (e) {
        if (e.target === D.modalOverlay) closeModal();
    });

    D.docList.addEventListener('click', function (e) {
        var btn = e.target.closest('.btn-del');
        if (btn) {
            e.stopPropagation();
            var id = btn.getAttribute('data-delete-id');
            if (id) confirmDelete(id, btn);
        }
        var copyBtn = e.target.closest('.btn-copy');
        if (copyBtn) {
            e.stopPropagation();
            var content = copyBtn.getAttribute('data-content');
            if (content) copyToClipboard(content);
        }
    });
}

function bindKeyboard() {
    document.addEventListener('keydown', function (e) {
        if (e.ctrlKey && e.key === 'k') {
            e.preventDefault();
            D.searchInput.focus();
        }
        if (e.key === 'Escape') {
            if (D.addPanel.style.display === 'block') hideAddPanel();
            if (D.modalOverlay.style.display === 'flex') closeModal();
        }
    });
}

// ========== 状态与统计 ==========

function setLoading(loading) {
    if (loading) {
        D.loadingState.style.display = 'flex';
        D.emptyState.style.display = 'none';
        D.errorState.style.display = 'none';
        D.docList.style.display = 'none';
    } else {
        D.loadingState.style.display = 'none';
    }
}

function setEmpty() {
    D.emptyState.style.display = 'flex';
    D.docList.style.display = 'none';
    D.errorState.style.display = 'none';
    D.pagination.style.display = 'none';
}

function setError(msg) {
    D.errorState.style.display = 'flex';
    D.errorState.querySelector('.error-msg').textContent = msg;
    D.docList.style.display = 'none';
    D.emptyState.style.display = 'none';
    D.pagination.style.display = 'none';
}

function updateSyncWarning(mismatch) {
    if (mismatch) {
        D.syncWarn.style.display = 'flex';
        D.btnRebuild.style.display = 'inline-flex';
    } else {
        D.syncWarn.style.display = 'none';
        D.btnRebuild.style.display = 'none';
    }
}

async function loadStats() {
    try {
        var resp = await fetch(API.STATS);
        if (!resp.ok) throw new Error('HTTP ' + resp.status);
        var result = await resp.json();

        if (result.success && result.data) {
            var data = result.data;
            App.initialized = data.initialized;
            App.totalDocs = data.document_count || 0;
            App.entryCount = data.entry_count || 0;
            D.hdrCount.textContent = App.totalDocs;
            D.hdrEntry.textContent = App.entryCount;
            updateStatusDot(data.initialized);
            updateSyncWarning(data.sync_mismatch);

            if (data.initialized) {
                D.initCard.style.display = 'none';
                App.page = 1;
                App.searchMode = false;
                App.searchQuery = '';
                D.searchInput.value = '';
                D.btnClearSearch.style.display = 'none';
                loadDocuments();
            } else {
                showInitCard();
            }
        } else {
            App.initialized = false;
            updateStatusDot(false);
            showInitCard();
        }
    } catch (err) {
        App.initialized = false;
        updateStatusDot(false);
        showInitCard();
        showToast('无法连接到向量数据库服务', 'error');
    }
}

function updateStatusDot(ok) {
    D.hdrDot.className = 'status-dot ' + (ok ? 'connected' : 'disconnected');
    D.hdrLabel.textContent = ok ? '已连接' : '未连接';
}

function showInitCard() {
    D.initCard.style.display = 'block';
    D.docList.innerHTML = '';
    setEmpty();
    D.syncWarn.style.display = 'none';
    D.btnRebuild.style.display = 'none';
}

// ========== 初始化 ==========

async function handleInit() {
    var base = D.initBase.value.trim();
    var key = D.initKey.value.trim();
    var model = D.initModel.value.trim();

    if (!base) { showToast('API 地址不能为空', 'error'); return; }
    if (!model) { showToast('模型名称不能为空', 'error'); return; }

    D.btnInit.disabled = true;
    showBtnLoading(D.btnInit, true);

    try {
        // 第一步：实例初始化（只传 base_url 和 api_key，不含 model_name）
        var initResp = await fetch(API.INIT, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ base_url: base, api_key: key })
        });
        var initResult = await initResp.json();

        if (!initResult.success) {
            showToast('初始化失败: ' + (initResult.error || '未知错误'), 'error');
            return;
        }

        // 第二步：创建集合（传 model_name）
        var colResp = await fetch(API.COLLECTION, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ model_name: model })
        });
        var colResult = await colResp.json();

        if (colResult.success) {
            showToast('向量数据库初始化成功', 'success');
            loadStats();
        } else {
            showToast('集合创建失败: ' + (colResult.error || '未知错误'), 'error');
        }
    } catch (err) {
        showToast('初始化请求失败: ' + err.message, 'error');
    } finally {
        D.btnInit.disabled = false;
        showBtnLoading(D.btnInit, false);
    }
}

// ========== 文档列表 ==========

async function loadDocuments() {
    if (!App.initialized) return;

    setLoading(true);
    var offset = (App.page - 1) * PAGE_SIZE;

    try {
        var resp = await fetch(API.DOCS + '?offset=' + offset + '&limit=' + PAGE_SIZE);
        var result = await resp.json();

        if (result.success && result.data) {
            var data = result.data;
            App.entryCount = data.total || 0;
            D.hdrEntry.textContent = App.entryCount;

            App.totalPages = Math.max(1, Math.ceil(App.entryCount / PAGE_SIZE));
            if (App.page > App.totalPages) {
                App.page = App.totalPages;
                loadDocuments();
                return;
            }

            App.documents = data.documents || [];
            renderList(App.documents, false);
            renderPagination();
        } else {
            setError(result.error || '加载失败');
        }
    } catch (err) {
        setError('加载文档列表失败: ' + err.message);
        showToast('加载文档列表失败', 'error');
    } finally {
        setLoading(false);
    }
}

function renderList(docs, isSearch) {
    if (docs.length === 0) {
        setEmpty();
        D.docList.innerHTML = '';
        return;
    }

    D.emptyState.style.display = 'none';
    D.errorState.style.display = 'none';
    D.docList.style.display = 'flex';

    var html = '';
    for (var i = 0; i < docs.length; i++) {
        html += renderCard(docs[i], isSearch);
    }

    D.docList.innerHTML = html;
}

function renderCard(doc, isSearch) {
    var hasValidId = doc.id && typeof doc.id === 'string' && doc.id.trim() !== '';
    var cardCls = 'doc-card' + (isSearch ? ' search-result' : '');
    var contentPreview = truncateContent(doc.content, 200);
    var simBadge = '';
    if (isSearch && typeof doc.similarity === 'number') {
        var simPercent = (doc.similarity * 100).toFixed(1);
        var simCls = simPercent > 80 ? 'sim-high' : simPercent > 50 ? 'sim-mid' : 'sim-low';
        simBadge = '<span class="sim-badge ' + simCls + '" title="余弦相似度">' + simPercent + '%</span>';
    }

    return '<div class="' + cardCls + '" data-doc-id="' + esc(doc.id) + '">' +
        '<span class="doc-role-badge ' + esc(doc.role) + '">' + esc(doc.role) + '</span>' +
        simBadge +
        '<div class="doc-body">' +
            '<div class="doc-id-row">' +
                '<code class="doc-id-tag">' + esc(doc.id) + '</code>' +
                '<div class="doc-actions-inline">' +
                    '<button class="btn-icon-sm btn-copy" title="复制内容" data-content="' + escAttr(doc.content) + '">' +
                        '<i class="fa-solid fa-copy"></i>' +
                    '</button>' +
                    (hasValidId
                        ? '<button class="btn-icon-sm btn-del" title="删除此文档" data-delete-id="' + esc(doc.id) + '">' +
                            '<i class="fa-solid fa-trash-can"></i>' +
                          '</button>'
                        : '') +
                '</div>' +
            '</div>' +
            '<div class="doc-content">' + esc(doc.content) + '</div>' +
            (doc.content.length > 200
                ? '<button class="btn-expand" onclick="this.previousElementSibling.classList.toggle(\'expanded\'); this.textContent = this.previousElementSibling.classList.contains(\'expanded\') ? \'收起\' : \'展开全部\';" type="button">展开全部</button>'
                : '') +
        '</div>' +
    '</div>';
}

function renderPagination() {
    if (App.totalPages <= 1) {
        D.pagination.style.display = 'none';
        return;
    }

    D.pagination.style.display = 'flex';
    D.pageInfo.textContent = '第 ' + App.page + ' 页 / 共 ' + App.totalPages + ' 页';
    D.btnPrev.disabled = App.page <= 1;
    D.btnNext.disabled = App.page >= App.totalPages;
}

function goPrevPage() {
    if (App.page > 1) {
        App.page--;
        App.searchMode ? executeSearch(App.searchQuery) : loadDocuments();
    }
}

function goNextPage() {
    if (App.page < App.totalPages) {
        App.page++;
        App.searchMode ? executeSearch(App.searchQuery) : loadDocuments();
    }
}

// ========== 搜索 ==========

function handleSearchInput() {
    var val = D.searchInput.value.trim();
    D.btnClearSearch.style.display = val ? 'inline-flex' : 'none';

    if (!val) {
        clearSearch();
        return;
    }

    if (val.length < 2) return;

    App.searchMode = true;
    App.searchQuery = val;
    App.page = 1;
    executeSearch(val);
}

function commitSearch() {
    var val = D.searchInput.value.trim();
    D.btnClearSearch.style.display = val ? 'inline-flex' : 'none';

    if (!val) {
        clearSearch();
        return;
    }

    App.searchMode = true;
    App.searchQuery = val;
    App.page = 1;
    executeSearch(val);
}

function clearSearch() {
    App.searchMode = false;
    App.searchQuery = '';
    App.page = 1;
    D.searchInput.value = '';
    D.btnClearSearch.style.display = 'none';
    App.searchResults = [];
    loadDocuments();
}

async function executeSearch(query) {
    if (!App.initialized) return;

    setLoading(true);
    var topK = PAGE_SIZE;

    try {
        var url = API.MESSAGES + '?query=' + encodeURIComponent(query) + '&top_k=' + topK;
        var resp = await fetch(url);
        var result = await resp.json();

        if (result.success && result.data) {
            var data = result.data;
            App.searchResults = data.results || [];
            App.totalDocs = data.total_found || App.searchResults.length;
            D.hdrCount.textContent = App.totalDocs;

            App.totalPages = Math.max(1, Math.ceil(App.totalDocs / PAGE_SIZE));
            renderList(App.searchResults, true);
            renderPagination();
            showToast('搜索完成，找到 ' + App.totalDocs + ' 条结果', 'info');
        } else {
            setError(result.error || '搜索失败');
            showToast('搜索失败: ' + (result.error || '未知错误'), 'error');
        }
    } catch (err) {
        setError('搜索请求失败: ' + err.message);
        showToast('搜索请求失败', 'error');
    } finally {
        setLoading(false);
    }
}

// ========== 添加 ==========

function showAddPanel() {
    if (!App.initialized) {
        showToast('向量数据库未初始化，请先完成初始化', 'error');
        return;
    }
    D.addPanel.style.display = 'block';
    D.addContent.focus();
}

function hideAddPanel() {
    D.addPanel.style.display = 'none';
    D.addContent.value = '';
}

async function handleAdd() {
    if (!App.initialized) {
        showToast('向量数据库未初始化，请先完成初始化', 'error');
        return;
    }

    var role = D.addRole.value;
    var content = D.addContent.value.trim();

    if (!content) { showToast('文档内容不能为空', 'error'); return; }

    D.btnAddSubmit.disabled = true;
    showBtnLoading(D.btnAddSubmit, true, '添加中...');

    try {
        var resp = await fetch(API.MESSAGES, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ role: role, content: content })
        });
        var result = await resp.json();

        if (result.success) {
            showToast('文档添加成功', 'success');
            hideAddPanel();
            App.page = 1;
            App.searchMode = false;
            App.searchQuery = '';
            D.searchInput.value = '';
            D.btnClearSearch.style.display = 'none';
            loadStats();
        } else {
            showToast('添加失败: ' + (result.error || '未知错误'), 'error');
        }
    } catch (err) {
        showToast('网络请求失败: ' + err.message, 'error');
    } finally {
        D.btnAddSubmit.disabled = false;
        showBtnLoading(D.btnAddSubmit, false, '添加到向量库');
    }
}

// ========== 删除 ==========

function confirmDelete(id, button) {
    var docEl = button.closest('.doc-card');
    var roleBadge = docEl ? docEl.querySelector('.doc-role-badge') : null;
    var role = roleBadge ? roleBadge.textContent : '未知';

    showModal(
        '确认删除',
        '<div class="modal-delete-body">' +
            '<p>确定要删除以下文档吗？此操作<strong>不可恢复</strong>。</p>' +
            '<div class="modal-delete-info">' +
                '<span class="modal-delete-id">' + esc(id) + '</span>' +
                '<span class="doc-role-badge ' + esc(role) + '">' + esc(role) + '</span>' +
            '</div>' +
        '</div>',
        function () {
            closeModal();
            executeDelete(id, button);
        }
    );
}

async function executeDelete(id, button) {
    if (!id || typeof id !== 'string' || id.trim() === '') {
        showToast('删除失败：文档ID无效', 'error');
        return;
    }

    if (button) {
        button.disabled = true;
        button.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i>';
    }

    try {
        var resp = await fetch(API.MESSAGES, {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ id: id })
        });
        var result = await resp.json();

        if (result.success) {
            showToast('文档 ' + id + ' 删除成功', 'success');

            var card = button ? button.closest('.doc-card') : null;
            if (card) {
                card.style.opacity = '0';
                card.style.transform = 'scale(0.95)';
                card.style.transition = 'all 0.25s ease';
                setTimeout(function () {
                    if (card.parentNode) card.parentNode.removeChild(card);
                    checkEmptyState();
                }, 260);
            }

            loadStats();
        } else {
            showToast('删除失败: ' + (result.error || '未知错误'), 'error');
            if (button) {
                button.disabled = false;
                button.innerHTML = '<i class="fa-solid fa-trash-can"></i>';
            }
        }
    } catch (err) {
        showToast('网络请求失败: ' + err.message, 'error');
        if (button) {
            button.disabled = false;
            button.innerHTML = '<i class="fa-solid fa-trash-can"></i>';
        }
    }
}

function checkEmptyState() {
    var remaining = D.docList.querySelectorAll('.doc-card');
    if (remaining.length === 0) {
        setEmpty();
    }
}

// ========== 重建 ==========

async function handleRebuild() {
    if (!App.initialized) return;

    showModal(
        '重建索引',
        '<p>将检测并清理维度不符或向量缺失的文档记录。此操作用于修复向量数据异常。</p>' +
        '<p class="modal-hint">提示：如果集合中有大量文档，此操作可能需要一些时间。</p>',
        async function () {
            closeModal();
            await executeRebuild();
        }
    );
}

async function executeRebuild() {
    showBtnLoading(D.btnRebuild, true, '重建中...');
    D.btnRebuild.disabled = true;

    try {
        var resp = await fetch(API.REBUILD, { method: 'POST' });
        var result = await resp.json();

        if (result.success) {
            var count = result.data ? result.data.rebuilt : 0;
            showToast('索引重建完成，共 ' + count + ' 条文档', 'success');
            loadStats();
        } else {
            showToast('重建失败: ' + (result.error || '未知错误'), 'error');
        }
    } catch (err) {
        showToast('重建请求失败: ' + err.message, 'error');
    } finally {
        D.btnRebuild.disabled = false;
        showBtnLoading(D.btnRebuild, false, '重建索引');
    }
}

// ========== Modal ==========

function showModal(title, bodyHTML, onConfirm) {
    D.modalTitle.textContent = title;
    D.modalBody.innerHTML = bodyHTML;
    D.modalOverlay.style.display = 'flex';

    var oldConfirm = D.modalConfirm;
    var newConfirm = oldConfirm.cloneNode(true);
    oldConfirm.parentNode.replaceChild(newConfirm, oldConfirm);
    D.modalConfirm = newConfirm;

    D.modalConfirm.addEventListener('click', function () {
        if (onConfirm) onConfirm();
    });

    D.modalConfirm.focus();
}

function closeModal() {
    D.modalOverlay.style.display = 'none';
    D.modalBody.innerHTML = '';
}

// ========== Toast ==========

function showToast(message, type) {
    type = type || 'info';
    var icons = { success: 'fa-circle-check', error: 'fa-circle-exclamation', info: 'fa-circle-info', warn: 'fa-triangle-exclamation' };

    var toast = document.createElement('div');
    toast.className = 'toast ' + type;
    toast.innerHTML = '<i class="fa-solid ' + (icons[type] || icons.info) + '"></i><span>' + esc(message) + '</span>';
    D.toastContainer.appendChild(toast);

    requestAnimationFrame(function () {
        toast.classList.add('toast-visible');
    });

    setTimeout(function () {
        toast.classList.add('toast-out');
        toast.addEventListener('transitionend', function () {
            if (toast.parentNode) toast.parentNode.removeChild(toast);
        });
    }, 3500);
}

// ========== 工具函数 ==========

function esc(str) {
    if (typeof str !== 'string') return '';
    var div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

function escAttr(str) {
    if (typeof str !== 'string') return '';
    return str.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/'/g, '&#39;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function truncateContent(content, maxLen) {
    if (!content) return '';
    return content.length > maxLen ? content.substring(0, maxLen) : content;
}

function debounce(fn, delay) {
    var timer = null;
    return function () {
        var ctx = this, args = arguments;
        if (timer) clearTimeout(timer);
        timer = setTimeout(function () { fn.apply(ctx, args); }, delay);
    };
}

function showBtnLoading(btn, loading, text) {
    if (!btn) return;
    if (loading) {
        btn.dataset.originalText = btn.innerHTML;
        btn.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i> ' + (text || '处理中...');
    } else {
        btn.innerHTML = btn.dataset.originalText || text || btn.innerHTML;
    }
}

async function copyToClipboard(text) {
    try {
        await navigator.clipboard.writeText(text);
        showToast('内容已复制到剪贴板', 'success');
    } catch (err) {
        var ta = document.createElement('textarea');
        ta.value = text;
        ta.style.position = 'fixed';
        ta.style.opacity = '0';
        document.body.appendChild(ta);
        ta.select();
        document.execCommand('copy');
        document.body.removeChild(ta);
        showToast('内容已复制到剪贴板', 'success');
    }
}

init();