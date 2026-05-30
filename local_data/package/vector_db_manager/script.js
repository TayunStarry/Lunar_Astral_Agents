// ============================================================
// 星月智能 · 向量检索 — 主逻辑脚本 v2
// ============================================================

const API_MESSAGES = '/chromem/messages';
const API_STATS = '/chromem/stats';
const API_INIT = '/chromem/init';
const API_DOCS = '/chromem/documents';
const PAGE_SIZE = 15;

const dom = {
    hdrCount: document.getElementById('hdr-count'),
    hdrStatusDot: document.getElementById('hdr-status-dot'),
    hdrStatusLabel: document.getElementById('hdr-status-label'),
    refreshStats: document.getElementById('refresh-stats'),

    initCard: document.getElementById('init-card'),
    initBaseURL: document.getElementById('init-base-url'),
    initAPIKey: document.getElementById('init-api-key'),
    initModelName: document.getElementById('init-model-name'),
    btnInit: document.getElementById('btn-init'),

    searchInput: document.getElementById('search-input'),
    btnSearch: document.getElementById('btn-search'),
    btnShowAdd: document.getElementById('btn-show-add'),

    addPanel: document.getElementById('add-panel'),
    btnCloseAdd: document.getElementById('btn-close-add'),
    addRole: document.getElementById('add-role'),
    addContent: document.getElementById('add-content'),
    btnAdd: document.getElementById('btn-add'),

    docList: document.getElementById('doc-list'),
    docEmpty: document.getElementById('doc-empty'),
    pagination: document.getElementById('pagination'),
    btnPrev: document.getElementById('btn-prev'),
    btnNext: document.getElementById('btn-next'),
    pageInfo: document.getElementById('page-info'),

    toastContainer: document.getElementById('toast-container')
};

var state = {
    initialized: false,
    currentPage: 1,
    totalPages: 1,
    totalDocs: 0,
    searchMode: false,
    searchQuery: ''
};

function init() {
    bindEvents();
    loadStats();
}

function bindEvents() {
    dom.refreshStats.addEventListener('click', loadStats);
    dom.btnInit.addEventListener('click', handleInit);
    dom.btnSearch.addEventListener('click', handleSearch);
    dom.searchInput.addEventListener('keydown', function (e) {
        if (e.key === 'Enter') handleSearch();
    });
    dom.btnShowAdd.addEventListener('click', function () {
        dom.addPanel.style.display = 'block';
        dom.addContent.focus();
    });
    dom.btnCloseAdd.addEventListener('click', function () {
        dom.addPanel.style.display = 'none';
    });
    dom.btnAdd.addEventListener('click', handleAdd);
    dom.addContent.addEventListener('keydown', function (e) {
        if (e.ctrlKey && e.key === 'Enter') handleAdd();
    });
    dom.btnPrev.addEventListener('click', function () {
        if (state.currentPage > 1) {
            state.currentPage--;
            if (state.searchMode) {
                executeSearch(state.searchQuery);
            } else {
                loadDocuments();
            }
        }
    });
    dom.btnNext.addEventListener('click', function () {
        if (state.currentPage < state.totalPages) {
            state.currentPage++;
            if (state.searchMode) {
                executeSearch(state.searchQuery);
            } else {
                loadDocuments();
            }
        }
    });
}

// ========== 初始化 & 状态 ==========

async function loadStats() {
    try {
        var response = await fetch(API_STATS);
        if (!response.ok) throw new Error('HTTP ' + response.status);

        var result = await response.json();

        if (result.success && result.data) {
            var connected = result.data.initialized;
            state.initialized = connected;
            state.totalDocs = result.data.document_count || 0;
            dom.hdrCount.textContent = state.totalDocs;
            updateStatusIndicator(connected);

            if (connected) {
                dom.initCard.style.display = 'none';
                state.currentPage = 1;
                loadDocuments();
            } else {
                dom.initCard.style.display = 'block';
                dom.docList.innerHTML = '';
                dom.docEmpty.style.display = 'none';
                dom.pagination.style.display = 'none';
            }
        } else {
            state.initialized = false;
            dom.hdrCount.textContent = '--';
            updateStatusIndicator(false);
            dom.initCard.style.display = 'block';
        }
    } catch (error) {
        state.initialized = false;
        dom.hdrCount.textContent = '--';
        updateStatusIndicator(false);
        dom.initCard.style.display = 'block';
        showToast('无法连接到向量数据库服务', 'error');
    }
}

function updateStatusIndicator(connected) {
    dom.hdrStatusDot.classList.toggle('connected', connected);
    dom.hdrStatusDot.classList.toggle('disconnected', !connected);
    dom.hdrStatusLabel.textContent = connected ? '已连接' : '未连接';
}

async function handleInit() {
    var baseURL = dom.initBaseURL.value.trim();
    var apiKey = dom.initAPIKey.value.trim();
    var modelName = dom.initModelName.value.trim();

    if (!baseURL) { showToast('API 地址不能为空', 'error'); return; }
    if (!modelName) { showToast('模型名称不能为空', 'error'); return; }

    dom.btnInit.disabled = true;
    dom.btnInit.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i> 初始化中...';

    try {
        var response = await fetch(API_INIT, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ base_url: baseURL, api_key: apiKey, model_name: modelName })
        });

        var result = await response.json();

        if (result.success) {
            showToast('向量数据库初始化成功', 'success');
            loadStats();
        } else {
            showToast('初始化失败: ' + (result.error || '未知错误'), 'error');
        }
    } catch (error) {
        showToast('初始化请求失败: ' + error.message, 'error');
    } finally {
        dom.btnInit.disabled = false;
        dom.btnInit.innerHTML = '<i class="fa-solid fa-bolt"></i> 初始化连接';
    }
}

// ========== 文档列表 ==========

async function loadDocuments() {
    if (!state.initialized) return;

    var offset = (state.currentPage - 1) * PAGE_SIZE;

    try {
        var response = await fetch(API_DOCS + '?offset=' + offset + '&limit=' + PAGE_SIZE);
        var result = await response.json();

        if (result.success && result.data) {
            var data = result.data;
            state.totalDocs = data.total || 0;
            dom.hdrCount.textContent = state.totalDocs;

            state.totalPages = Math.max(1, Math.ceil(state.totalDocs / PAGE_SIZE));
            if (state.currentPage > state.totalPages) {
                state.currentPage = state.totalPages;
                loadDocuments();
                return;
            }

            renderDocList(data.documents || [], false);
            renderPagination();
        }
    } catch (error) {
        showToast('加载文档列表失败: ' + error.message, 'error');
    }
}

function renderDocList(documents, isSearch) {
    dom.docList.innerHTML = '';

    if (documents.length === 0) {
        dom.docEmpty.style.display = 'block';
        dom.docList.style.display = 'none';
        dom.pagination.style.display = 'none';
        return;
    }

    dom.docEmpty.style.display = 'none';
    dom.docList.style.display = 'flex';

    var html = '';
    for (var i = 0; i < documents.length; i++) {
        var doc = documents[i];
        var cardClass = 'doc-card' + (isSearch ? ' search-result' : '');
        html += '<div class="' + cardClass + '" data-id="' + escapeHtml(doc.id) + '">' +
            '<span class="doc-role-badge ' + escapeHtml(doc.role) + '">' + escapeHtml(doc.role) + '</span>' +
            '<div class="doc-body">' +
            '<div class="doc-id">' + escapeHtml(doc.id) + '</div>' +
            '<div class="doc-content">' + escapeHtml(doc.content) + '</div>' +
            '</div>' +
            '<div class="doc-actions">' +
            '<button class="btn-del" title="删除此文档" data-delete-id="' + escapeHtml(doc.id) + '">' +
            '<i class="fa-solid fa-trash-can"></i>' +
            '</button>' +
            '</div>' +
            '</div>';
    }

    dom.docList.innerHTML = html;

    var delButtons = dom.docList.querySelectorAll('.btn-del');
    for (var j = 0; j < delButtons.length; j++) {
        delButtons[j].addEventListener('click', function (e) {
            e.stopPropagation();
            var btn = e.currentTarget;
            handleDeleteOne(btn.getAttribute('data-delete-id'), btn);
        });
    }
}

function renderPagination() {
    if (state.totalPages <= 1) {
        dom.pagination.style.display = 'none';
        return;
    }

    dom.pagination.style.display = 'flex';
    dom.pageInfo.textContent = '第 ' + state.currentPage + ' 页 / 共 ' + state.totalPages + ' 页';
    dom.btnPrev.disabled = state.currentPage <= 1;
    dom.btnNext.disabled = state.currentPage >= state.totalPages;
}

// ========== 搜索 ==========

function handleSearch() {
    var query = dom.searchInput.value.trim();

    if (!query) {
        state.searchMode = false;
        state.searchQuery = '';
        state.currentPage = 1;
        loadDocuments();
        return;
    }

    state.searchMode = true;
    state.searchQuery = query;
    state.currentPage = 1;
    executeSearch(query);
}

async function executeSearch(query) {
    if (!state.initialized) return;

    var topK = PAGE_SIZE;

    dom.btnSearch.disabled = true;
    dom.btnSearch.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i>';

    try {
        var url = API_MESSAGES + '?query=' + encodeURIComponent(query) + '&top_k=' + topK;
        var response = await fetch(url);
        var result = await response.json();

        if (result.success && result.data) {
            var data = result.data;
            var results = data.results || [];
            state.totalDocs = data.total_found || results.length;
            dom.hdrCount.textContent = state.totalDocs;

            state.totalPages = Math.max(1, Math.ceil(state.totalDocs / PAGE_SIZE));
            renderDocList(results, true);
            renderPagination();
            showToast('搜索完成，找到 ' + state.totalDocs + ' 条结果', 'info');
        } else {
            showToast('搜索失败: ' + (result.error || '未知错误'), 'error');
        }
    } catch (error) {
        showToast('搜索请求失败: ' + error.message, 'error');
    } finally {
        dom.btnSearch.disabled = false;
        dom.btnSearch.innerHTML = '搜索';
    }
}

// ========== 添加文档 ==========

async function handleAdd() {
    var role = dom.addRole.value;
    var content = dom.addContent.value.trim();

    if (!content) { showToast('文档内容不能为空', 'error'); return; }

    dom.btnAdd.disabled = true;
    dom.btnAdd.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i> 添加中...';

    try {
        var response = await fetch(API_MESSAGES, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ role: role, content: content })
        });

        var result = await response.json();

        if (result.success) {
            showToast('文档添加成功', 'success');
            dom.addContent.value = '';
            dom.addPanel.style.display = 'none';
            state.currentPage = 1;
            loadDocuments();
        } else {
            showToast('添加失败: ' + (result.error || '未知错误'), 'error');
        }
    } catch (error) {
        showToast('网络请求失败: ' + error.message, 'error');
    } finally {
        dom.btnAdd.disabled = false;
        dom.btnAdd.innerHTML = '<i class="fa-solid fa-cloud-arrow-up"></i> 添加到向量库';
    }
}

// ========== 删除文档 ==========

async function handleDeleteOne(id, button) {
    if (!confirm('确定要删除文档 ' + id + ' 吗？此操作不可恢复。')) return;

    button.disabled = true;
    button.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i>';

    try {
        var response = await fetch(API_MESSAGES, {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ id: id })
        });

        var result = await response.json();

        if (result.success) {
            showToast('文档 ' + id + ' 删除成功', 'success');

            var card = button.closest('.doc-card');
            if (card) {
                card.style.opacity = '0';
                card.style.transform = 'scale(0.95)';
                card.style.transition = 'all 0.25s ease';
                setTimeout(function () {
                    if (card.parentNode) card.parentNode.removeChild(card);
                    var remaining = dom.docList.querySelectorAll('.doc-card');
                    if (remaining.length === 0) {
                        dom.docEmpty.style.display = 'block';
                        dom.docList.style.display = 'none';
                        dom.pagination.style.display = 'none';
                    }
                }, 250);
            }

            loadStats();
        } else {
            showToast('删除失败: ' + (result.error || '未知错误'), 'error');
            button.disabled = false;
            button.innerHTML = '<i class="fa-solid fa-trash-can"></i>';
        }
    } catch (error) {
        showToast('网络请求失败: ' + error.message, 'error');
        button.disabled = false;
        button.innerHTML = '<i class="fa-solid fa-trash-can"></i>';
    }
}

// ========== 工具函数 ==========

function showToast(message, type) {
    type = type || 'info';
    var icons = { success: 'fa-circle-check', error: 'fa-circle-exclamation', info: 'fa-circle-info' };

    var toast = document.createElement('div');
    toast.className = 'toast ' + type;
    toast.innerHTML = '<i class="fa-solid ' + (icons[type] || icons.info) + '"></i>' + escapeHtml(message);
    dom.toastContainer.appendChild(toast);

    setTimeout(function () {
        toast.classList.add('toast-out');
        toast.addEventListener('animationend', function () {
            if (toast.parentNode) toast.parentNode.removeChild(toast);
        });
    }, 3500);
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

init();