// ============================================================
// 星月智能 · 记忆库 — 主逻辑脚本 v4
// 双栏多集合架构：集合管理、文档操作、语义搜索
// ============================================================

var API = {
    INIT: '/memory/init',
    STATS: '/memory/stats',
    COLLECTIONS: '/memory/collections',
    collection: function (name) {
        var encoded = encodeURIComponent(name);
        return {
            BASE: '/memory/' + encoded,
            STATS: '/memory/' + encoded + '/stats',
            MESSAGES: '/memory/' + encoded + '/messages',
            DOCS: '/memory/' + encoded + '/documents',
            REBUILD: '/memory/' + encoded + '/rebuild',
            CLEAR: '/memory/' + encoded + '/clear'
        };
    }
};

var PAGE_SIZE = 12;
var MAX_DOCS = 100000; // 已取消分页：一次性拉取该集合全部文档的上限

var App = {
    initialized: false,
    currentCollection: '',
    collections: [],
    page: 1,
    totalPages: 1,
    totalDocs: 0,
    searchMode: false,
    searchQuery: '',
    documents: [],
    searchResults: [],
    imageBase64: null,       // 当前上传的图片 base64
    isImageCollection: false, // 当前集合是否为 image 类型
    batchFiles: [],           // 批量导入的图片文件列表
    batchRunning: false       // 批量导入是否正在运行
};

function $(id) {
    return document.getElementById(id);
}

var D = {
    // 侧栏通用
    syncWarn:       $('sync-warn'),
    btnRebuildSync: $('btn-rebuild-sync'),

    // Init
    initCard: $('init-card'),
    initColName: $('init-collection-name'),
    btnInit: $('btn-init'),

    // App body
    appBody: $('app-body'),

    // Left sidebar - collection stats
    statName: $('stat-name'),
    statModel: $('stat-model'),
    statDim: $('stat-dim'),
    statType: $('stat-type'),
    statCount: $('stat-count'),
    btnAddDoc:      $('btn-add-doc'),
    btnRebuild:     $('btn-rebuild'),
    btnBatchImport: $('btn-batch-import'),

    // Left sidebar - collection list
    collectionList: $('collection-list'),
    btnCreateCol: $('btn-create-collection'),

    // Left sidebar - footer
    btnClearCol: $('btn-clear-collection'),

    // Search
    searchInput: $('search-input'),
    btnSearch: $('btn-search'),
    btnClearSearch: $('btn-clear-search'),

    // Content area
    loadingState: $('loading-state'),
    emptyState: $('empty-state'),
    emptyColState: $('empty-collection-state'),
    errorState: $('error-state'),
    docList: $('doc-list'),

    // Pagination
    pagination: $('pagination'),
    btnPrev: $('btn-prev'),
    btnNext: $('btn-next'),
    pageInfo: $('page-info'),

    // Confirm modal
    modalOverlay: $('modal-overlay'),
    modalTitle: $('modal-title'),
    modalBody: $('modal-body'),
    modalConfirm: $('modal-confirm'),
    modalCancel: $('modal-cancel'),

    // Add doc modal
    modalAddDoc: $('modal-add-doc'),
    addRole: $('add-role'),
    addContent: $('add-content'),
    btnAddSubmit: $('btn-add-submit'),

    // Create collection modal
    modalCreateCol: $('modal-create-collection'),
    createColName: $('create-col-name'),
    createColType: $('create-col-type'),
    btnCreateSubmit: $('btn-create-submit'),

    // Add image modal
    modalAddImage: $('modal-add-image'),
    addImageFile: $('add-image-file'),
    imageUploadArea: $('image-upload-area'),
    imageUploadPlaceholder: $('image-upload-placeholder'),
    imageUploadPreview: $('image-upload-preview'),
    btnRemoveImage: $('btn-remove-image'),
    btnAddImageSubmit: $('btn-add-image-submit'),
    imageOrientation: $('image-orientation'),
    imageOrientationCustomGroup: $('image-orientation-custom-group'),
    imageOrientationCustom: $('image-orientation-custom'),

    // Batch import modal
    modalBatchImport:  $('modal-batch-import'),
    batchPath:         $('batch-path'),
    btnScanPath:       $('btn-scan-path'),
    batchFileList:     $('batch-file-list'),
    batchProgress:     $('batch-progress'),
    batchProgressFill: $('batch-progress-fill'),
    batchProgressText: $('batch-progress-text'),
    batchLog:          $('batch-log'),
    btnStartBatch:     $('btn-start-batch'),
    batchOrientation:  $('batch-orientation'),
    batchOrientationCustomGroup: $('batch-orientation-custom-group'),
    batchOrientationCustom: $('batch-orientation-custom'),

    // Toast
    toastContainer: $('toast-container')
};

// ========== 初始化 ==========

function init() {
    bindGlobalEvents();
    bindKeyboard();
    loadGlobalStats();
}

function bindGlobalEvents() {
    // 侧栏同步重建
    D.btnRebuildSync.addEventListener('click', handleRebuildSync);

    // Init
    D.btnInit.addEventListener('click', handleInit);

    // Search
    D.searchInput.addEventListener('input', debounce(handleSearchInput, 350));
    D.searchInput.addEventListener('keydown', function (e) {
        if (e.key === 'Enter') commitSearch();
        if (e.key === 'Escape') { D.searchInput.value = ''; clearSearch(); }
    });
    D.btnSearch.addEventListener('click', commitSearch);
    D.btnClearSearch.addEventListener('click', clearSearch);

    // Add doc
    D.btnAddDoc.addEventListener('click', function () {
        if (App.isImageCollection) {
            showAddImageModal();
        } else {
            showAddDocModal();
        }
    });
    D.btnAddSubmit.addEventListener('click', handleAddDocument);
    D.addContent.addEventListener('keydown', function (e) {
        if (e.ctrlKey && e.key === 'Enter') handleAddDocument();
        if (e.key === 'Escape') closeModalById('modal-add-doc');
    });

    // Add image
    D.btnAddImageSubmit.addEventListener('click', handleAddImage);
    D.imageUploadArea.addEventListener('click', function () {
        D.addImageFile.click();
    });
    D.addImageFile.addEventListener('change', handleImageFileSelected);
    D.btnRemoveImage.addEventListener('click', function (e) {
        e.stopPropagation();
        clearImageUpload();
    });
    D.imageOrientation.addEventListener('change', function () {
        toggleOrientationCustomGroup(D.imageOrientation, D.imageOrientationCustomGroup);
    });
    // Drag and drop for image upload
    D.imageUploadArea.addEventListener('dragover', function (e) {
        e.preventDefault();
        D.imageUploadArea.classList.add('drag-over');
    });
    D.imageUploadArea.addEventListener('dragleave', function () {
        D.imageUploadArea.classList.remove('drag-over');
    });
    D.imageUploadArea.addEventListener('drop', function (e) {
        e.preventDefault();
        D.imageUploadArea.classList.remove('drag-over');
        if (e.dataTransfer.files && e.dataTransfer.files[0]) {
            processImageFile(e.dataTransfer.files[0]);
        }
    });

    // Batch import
    D.btnBatchImport.addEventListener('click', showBatchImportModal);
    D.btnScanPath.addEventListener('click', handleScanPath);
    D.btnStartBatch.addEventListener('click', handleStartBatch);
    D.batchOrientation.addEventListener('change', function () {
        toggleOrientationCustomGroup(D.batchOrientation, D.batchOrientationCustomGroup);
    });

    // Rebuild
    D.btnRebuild.addEventListener('click', handleRebuild);

    // Create collection
    D.btnCreateCol.addEventListener('click', showCreateCollectionModal);
    D.btnCreateSubmit.addEventListener('click', handleCreateCollection);

    // Clear collection
    D.btnClearCol.addEventListener('click', handleClearCollection);

    // Collection list - event delegation
    D.collectionList.addEventListener('click', function (e) {
        var delBtn = e.target.closest('.btn-delete-col');
        if (delBtn) {
            e.stopPropagation();
            var name = delBtn.getAttribute('data-col-name');
            if (name) confirmDeleteCollection(name);
            return;
        }

        var item = e.target.closest('.collection-item');
        if (item && !item.classList.contains('create')) {
            var name = item.getAttribute('data-col-name');
            if (name) selectCollection(name);
        }
    });

    // Pagination
    D.btnPrev.addEventListener('click', goPrevPage);
    D.btnNext.addEventListener('click', goNextPage);

    // Confirm modal
    D.modalCancel.addEventListener('click', closeConfirmModal);
    D.modalOverlay.addEventListener('click', function (e) {
        if (e.target === D.modalOverlay) closeConfirmModal();
    });

    // Doc list - event delegation
    D.docList.addEventListener('click', function (e) {
        var delBtn = e.target.closest('.btn-del');
        if (delBtn) {
            e.stopPropagation();
            var id = delBtn.getAttribute('data-delete-id');
            if (id) confirmDeleteDoc(id, delBtn);
        }
        var copyBtn = e.target.closest('.btn-copy');
        if (copyBtn) {
            e.stopPropagation();
            var content = copyBtn.getAttribute('data-content');
            if (content) copyToClipboard(content);
        }
    });

    // Modal close buttons
    document.querySelectorAll('.modal-close-btn').forEach(function (btn) {
        btn.addEventListener('click', function () {
            var modalId = btn.getAttribute('data-close');
            closeModalById(modalId);
        });
    });

    // Close modals on overlay click
    document.querySelectorAll('.modal-overlay').forEach(function (overlay) {
        overlay.addEventListener('click', function (e) {
            if (e.target === overlay) closeModalById(overlay.id);
        });
    });
}

function bindKeyboard() {
    document.addEventListener('keydown', function (e) {
        if (e.ctrlKey && e.key === 'k') {
            e.preventDefault();
            D.searchInput.focus();
        }
        if (e.key === 'Escape') {
            if (D.modalAddDoc.style.display === 'flex') closeModalById('modal-add-doc');
            if (D.modalAddImage.style.display === 'flex') closeModalById('modal-add-image');
            if (D.modalCreateCol.style.display === 'flex') closeModalById('modal-create-collection');
            if (D.modalBatchImport.style.display === 'flex') closeModalById('modal-batch-import');
            if (D.modalOverlay.style.display === 'flex') closeConfirmModal();
        }
    });
}

// ========== 全局统计 ==========

async function loadGlobalStats() {
    try {
        var resp = await fetch(API.STATS);
        if (!resp.ok) throw new Error('HTTP ' + resp.status);
        var result = await resp.json();

        if (result.success && result.data) {
            var data = result.data;
            App.initialized = data.initialized;

            if (data.initialized) {
                D.initCard.style.display = 'none';
                D.appBody.style.display = 'flex';
                loadCollections();
            } else {
                showInitCard();
            }
        } else {
            App.initialized = false;
            showInitCard();
        }
    } catch (err) {
        App.initialized = false;
        showInitCard();
        showToast('无法连接到记忆库服务', 'error');
    }
}

function showInitCard() {
    D.initCard.style.display = 'block';
    D.appBody.style.display = 'none';
    D.syncWarn.style.display = 'none';
}

async function refreshAll() {
    if (App.initialized) {
        await loadCollections();
        if (App.currentCollection) {
            loadCollectionStats(App.currentCollection);
            App.page = 1;
            App.searchMode = false;
            App.searchQuery = '';
            D.searchInput.value = '';
            D.btnClearSearch.style.display = 'none';
            loadDocuments();
        }
    } else {
        loadGlobalStats();
    }
}

// ========== 初始化 ==========

async function handleInit() {
    var colName = D.initColName.value.trim() || 'lunar_messages';

    if (!colName) { showToast('集合名称不能为空', 'error'); return; }

    D.btnInit.disabled = true;
    showBtnLoading(D.btnInit, true, '初始化中...');

    try {
        var initResp = await fetch(API.INIT, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({})
        });
        var initResult = await initResp.json();

        if (!initResult.success) {
            showToast('初始化失败: ' + (initResult.error || '未知错误'), 'error');
            return;
        }

        var colResp = await fetch(API.collection(colName).BASE, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({})
        });
        var colResult = await colResp.json();

        if (colResult.success) {
            showToast('记忆库初始化成功', 'success');
            loadGlobalStats();
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

// ========== 集合管理 ==========

async function loadCollections() {
    try {
        var resp = await fetch(API.COLLECTIONS);
        var result = await resp.json();

        if (result.success && result.data) {
            var collections = result.data.collections || [];
            App.collections = collections;

            if (collections.length > 0) {
                var found = false;
                if (App.currentCollection) {
                    for (var i = 0; i < collections.length; i++) {
                        if (collections[i].name === App.currentCollection) {
                            found = true;
                            break;
                        }
                    }
                }
                if (!found) {
                    var hasLunar = false;
                    for (var i = 0; i < collections.length; i++) {
                        if (collections[i].name === 'lunar_messages') {
                            hasLunar = true;
                            App.currentCollection = 'lunar_messages';
                            break;
                        }
                    }
                    if (!hasLunar) {
                        App.currentCollection = collections[0].name;
                    }
                }
                selectCollection(App.currentCollection, true);
                // 首次自动选择后加载文档内容
                loadCollectionStats(App.currentCollection);
                loadDocuments();
            } else {
                App.currentCollection = '';
                updateCollectionStats(null);
                renderCollectionList();
                showEmptyCollectionState();
            }
        } else {
            showToast('加载集合列表失败', 'error');
        }
    } catch (err) {
        showToast('加载集合列表失败: ' + err.message, 'error');
    }
}

function selectCollection(name, skipReload) {
    App.currentCollection = name;
    App.page = 1;
    App.searchMode = false;
    App.searchQuery = '';
    D.searchInput.value = '';
    D.btnClearSearch.style.display = 'none';

    updateCurrentCollectionUI(name);
    renderCollectionList();

    if (!skipReload) {
        loadCollectionStats(name);
        loadDocuments();
    }
}

function updateCurrentCollectionUI(name) {
    var col = findCollection(name);
    updateCollectionStats(col);
    updateSyncWarning(col);
}

function findCollection(name) {
    for (var i = 0; i < App.collections.length; i++) {
        if (App.collections[i].name === name) return App.collections[i];
    }
    return null;
}

function updateCollectionStats(col) {
    if (col) {
        D.statName.textContent = col.name;
        D.statModel.textContent = col.embedding_model || col.model;
        D.statDim.textContent = col.dimension;
        D.statType.textContent = col.type === 'image' ? '图片记忆' : '文本文档';
        D.statCount.textContent = col.count;
        App.isImageCollection = col.type === 'image';
        updateAddButton();
    } else {
        D.statName.textContent = '--';
        D.statModel.textContent = '--';
        D.statDim.textContent = '--';
        D.statType.textContent = '--';
        D.statCount.textContent = '--';
        App.isImageCollection = false;
        updateAddButton();
    }
}

function updateAddButton() {
    if (App.isImageCollection) {
        D.btnAddDoc.innerHTML = '<i class="fa-solid fa-image"></i> 新增';
        D.btnAddDoc.title = '向当前图片集合添加图片';
        D.btnBatchImport.style.display = 'inline-flex';
    } else {
        D.btnAddDoc.innerHTML = '<i class="fa-solid fa-plus"></i> 添加文档';
        D.btnAddDoc.title = '向当前集合添加文档';
        D.btnBatchImport.style.display = 'none';
    }
}

function updateSyncWarning(col) {
    if (col && col.count > 0) {
        // Only show if we detect mismatch (we'll rely on /stats endpoint)
        D.syncWarn.style.display = 'none';
        D.btnRebuildSync.style.display = 'none';
    } else {
        D.syncWarn.style.display = 'none';
    }
}

async function loadCollectionStats(name) {
    try {
        var resp = await fetch(API.collection(name).STATS);
        var result = await resp.json();

        if (result.success && result.data) {
            var data = result.data;
            var col = findCollection(name);
            if (col) {
                col.count = data.document_count || 0;
                updateCollectionStats(col);
            }
            if (data.sync_mismatch) {
                D.syncWarn.style.display = 'flex';
                D.btnRebuildSync.style.display = 'inline-flex';
            } else {
                D.syncWarn.style.display = 'none';
                D.btnRebuildSync.style.display = 'none';
            }
        }
    } catch (err) {
        // Silently fail
    }
}

function renderCollectionList() {
    var html = '';
    for (var i = 0; i < App.collections.length; i++) {
        var col = App.collections[i];
        var isActive = col.name === App.currentCollection;
        var typeIcon = col.type === 'image' ? 'fa-image' : 'fa-folder';
        var typeCls = col.type === 'image' ? ' image-type' : '';
        html += '<div class="collection-item' + (isActive ? ' active' : '') + typeCls + '" data-col-name="' + esc(col.name) + '">' +
            '<span class="col-icon"><i class="fa-solid ' + (isActive ? 'fa-folder-open' : typeIcon) + '"></i></span>' +
            '<span class="col-name" title="' + esc(col.name) + '">' + esc(col.name) + '</span>' +
            '<span class="col-count">' + col.count + '</span>' +
            '<button class="btn-delete-col" data-col-name="' + esc(col.name) + '" title="删除集合">' +
            '<i class="fa-solid fa-trash-can"></i>' +
            '</button>' +
            '</div>';
    }

    D.collectionList.innerHTML = html;
}

// ========== 创建集合 ==========

function showCreateCollectionModal() {
    if (!App.initialized) {
        showToast('记忆库未初始化，请先完成初始化', 'error');
        return;
    }
    D.createColName.value = '';
    D.modalCreateCol.style.display = 'flex';
    setTimeout(function () { D.createColName.focus(); }, 100);
}

async function handleCreateCollection() {
    var name = D.createColName.value.trim();
    var type = D.createColType.value;

    if (!name) { showToast('集合名称不能为空', 'error'); return; }

    // Validate name
    if (!/^[a-zA-Z0-9_-]+$/.test(name)) {
        showToast('集合名称仅允许字母、数字、下划线、连字符', 'error');
        return;
    }

    var reserved = ['init', 'stats', 'collections'];
    if (reserved.indexOf(name) !== -1) {
        showToast('集合名称不能使用保留字: ' + name, 'error');
        return;
    }

    D.btnCreateSubmit.disabled = true;
    showBtnLoading(D.btnCreateSubmit, true, '创建中...');

    try {
        var resp = await fetch(API.collection(name).BASE, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ collection_type: type })
        });
        var result = await resp.json();

        if (result.success) {
            showToast('集合 ' + name + ' (' + type + ') 创建成功', 'success');
            closeModalById('modal-create-collection');
            App.currentCollection = name;
            loadCollections();
        } else {
            showToast('创建失败: ' + (result.error || '未知错误'), 'error');
        }
    } catch (err) {
        showToast('创建请求失败: ' + err.message, 'error');
    } finally {
        D.btnCreateSubmit.disabled = false;
        showBtnLoading(D.btnCreateSubmit, false, '创建');
    }
}

// ========== 删除集合 ==========

function confirmDeleteCollection(name) {
    showConfirmModal(
        '确认删除集合',
        '<div class="modal-delete-body">' +
        '<p>确定要删除集合 <strong>' + esc(name) + '</strong> 吗？</p>' +
        '<p>此操作将<strong>永久删除</strong>该集合的所有文档，<strong>不可恢复</strong>。</p>' +
        '<div class="modal-delete-info">' +
        '<span class="modal-delete-id">' + esc(name) + '</span>' +
        '</div>' +
        '</div>',
        function () {
            closeConfirmModal();
            executeDeleteCollection(name);
        },
        '确认删除',
        'btn-danger'
    );
}

async function executeDeleteCollection(name) {
    try {
        var resp = await fetch(API.collection(name).BASE, {
            method: 'DELETE'
        });
        var result = await resp.json();

        if (result.success) {
            showToast('集合 ' + name + ' 已删除', 'success');

            if (App.currentCollection === name) {
                App.currentCollection = '';
                App.documents = [];
            }

            loadCollections();
        } else {
            showToast('删除失败: ' + (result.error || '未知错误'), 'error');
        }
    } catch (err) {
        showToast('删除请求失败: ' + err.message, 'error');
    }
}

// ========== 清空集合 ==========

function handleClearCollection() {
    if (!App.currentCollection) {
        showToast('请先选择一个集合', 'error');
        return;
    }

    showConfirmModal(
        '确认清空集合',
        '<div class="modal-delete-body">' +
        '<p>确定要清空集合 <strong>' + esc(App.currentCollection) + '</strong> 吗？</p>' +
        '<p>此操作将<strong>永久删除</strong>该集合中的所有文档，<strong>不可恢复</strong>。</p>' +
        '<p class="modal-hint">集合元数据（模型、维度）将被保留。</p>' +
        '</div>',
        function () {
            closeConfirmModal();
            executeClearCollection();
        },
        '确认清空',
        'btn-danger'
    );
}

async function executeClearCollection() {
    var name = App.currentCollection;
    try {
        var resp = await fetch(API.collection(name).CLEAR, {
            method: 'POST'
        });
        var result = await resp.json();

        if (result.success) {
            showToast('集合 ' + name + ' 已清空', 'success');
            App.page = 1;
            App.searchMode = false;
            App.searchQuery = '';
            D.searchInput.value = '';
            D.btnClearSearch.style.display = 'none';
            loadCollections();
        } else {
            showToast('清空失败: ' + (result.error || '未知错误'), 'error');
        }
    } catch (err) {
        showToast('清空请求失败: ' + err.message, 'error');
    }
}

// ========== 重建索引 ==========

function handleRebuild() {
    if (!App.currentCollection) {
        showToast('请先选择一个集合', 'error');
        return;
    }

    showConfirmModal(
        '重建索引',
        '<p>将检测并清理维度不符或向量缺失的文档记录。此操作用于修复向量数据异常。</p>' +
        '<p class="modal-hint">提示：如果集合中有大量文档，此操作可能需要一些时间。</p>',
        function () {
            closeConfirmModal();
            executeRebuild();
        },
        '确认重建',
        'btn-warn'
    );
}

function handleRebuildSync() {
    if (!App.currentCollection) return;
    handleRebuild();
}

async function executeRebuild() {
    var name = App.currentCollection;
    showBtnLoading(D.btnRebuild, true, '重建中...');
    D.btnRebuild.disabled = true;

    try {
        var resp = await fetch(API.collection(name).REBUILD, {
            method: 'POST'
        });
        var result = await resp.json();

        if (result.success) {
            var count = result.data ? result.data.rebuilt : 0;
            showToast('索引重建完成，共 ' + count + ' 条文档', 'success');
            refreshAll();
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

// ========== 文档列表 ==========

// 加载态：
// - 文本集合：显示横向骨架条；
// - 图片集合：自身已逐张渐进加载（每张图的 shimmer 占位 + 淡入），无需再叠加加载占位网格。
function setLoading(loading) {
    if (loading) {
        if (App.isImageCollection) {
            D.loadingState.style.display = 'none';
        } else {
            D.loadingState.innerHTML = '<div class="skeleton-card"></div>'.repeat(4);
            D.loadingState.style.display = 'flex';
        }
        D.emptyState.style.display = 'none';
        D.emptyColState.style.display = 'none';
        D.errorState.style.display = 'none';
        D.docList.style.display = 'none';
    } else {
        D.loadingState.style.display = 'none';
    }
}

function setEmpty() {
    D.emptyState.style.display = 'flex';
    D.emptyColState.style.display = 'none';
    D.docList.style.display = 'none';
    D.errorState.style.display = 'none';
    D.pagination.style.display = 'none';
}

function showEmptyCollectionState() {
    D.emptyColState.style.display = 'flex';
    D.emptyState.style.display = 'none';
    D.docList.style.display = 'none';
    D.errorState.style.display = 'none';
    D.pagination.style.display = 'none';
}

function setError(msg) {
    D.errorState.style.display = 'flex';
    D.errorState.querySelector('.error-msg').textContent = msg;
    D.docList.style.display = 'none';
    D.emptyState.style.display = 'none';
    D.emptyColState.style.display = 'none';
    D.pagination.style.display = 'none';
}

// ========== 加载文档（已取消分页：一次性拉取当前集合全部文档） ==========
// 服务端单次返回有上限（当前 max=100），这里循环分片拉取并拼接，
// 向量查询 top_k 为 50。若服务端将来放宽上限，循环会在首轮拉全后自动结束。
async function loadDocuments() {
    if (!App.initialized || !App.currentCollection) {
        showEmptyCollectionState();
        return;
    }

    setLoading(true);

    try {
        var all = [];
        var total = 0;
        var offset = 0;
        var pageLimit = 100; // 单次请求上限，与服务端保持一致

        while (true) {
            var resp = await fetch(API.collection(App.currentCollection).DOCS + '?offset=' + offset + '&limit=' + pageLimit);
            var result = await resp.json();

            if (!result.success || !result.data) {
                setError(result.error || '加载失败');
                return;
            }

            total = result.data.total || 0;
            var docs = result.data.documents || [];
            all = all.concat(docs);
            offset += docs.length;

            // 已拉全（本次不足一页 或 已达总数）
            if (docs.length === 0 || all.length >= total) break;
        }

        App.totalDocs = total;

        // Update collection count in sidebar
        var col = findCollection(App.currentCollection);
        if (col) {
            col.count = App.totalDocs;
            updateCollectionStats(col);
            renderCollectionList();
        }

        App.documents = all;
        renderList(all, false);
        renderPagination();
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
    D.emptyColState.style.display = 'none';
    D.errorState.style.display = 'none';
    D.docList.style.display = 'flex';

    var html = '';
    for (var i = 0; i < docs.length; i++) {
        html += renderCard(docs[i], isSearch);
    }

    D.docList.innerHTML = html;

    // 渲染完成后，为图片卡片填充尺寸徽标（宽 × 高）
    initImageSizeBadges();

    // 图片渐进式加载：每 10ms 给一张图片赋值 src，逐张显示，避免一次性解码大量大图导致卡顿
    loadImagesSequentially();
}

// ========== 图片渐进式加载（10ms/张） ==========
function loadImagesSequentially() {
    var imgs = D.docList.querySelectorAll('img.image-preview-thumb[data-src]');
    var i = 0;

    function next() {
        if (i >= imgs.length) return;
        var img = imgs[i++];
        var src = img.getAttribute('data-src');
        img.addEventListener('load', function () { img.classList.add('loaded'); });
        img.removeAttribute('data-src');
        img.src = src;
        setTimeout(next, 10);
    }
    next();
}

// ========== 图片尺寸徽标 ==========
// 图片卡片右上角不再显示「图片」文字，改为显示实际图片尺寸。
function initImageSizeBadges() {
    var cards = D.docList.querySelectorAll('.doc-card.image-card');
    for (var i = 0; i < cards.length; i++) {
        (function (card) {
            var img = card.querySelector('img.image-preview-thumb');
            var badge = card.querySelector('.img-size-badge');
            if (!img || !badge) return;

            var apply = function () {
                if (img.naturalWidth > 0 && img.naturalHeight > 0) {
                    badge.textContent = img.naturalWidth + ' × ' + img.naturalHeight;
                }
            };

            if (img.complete && img.naturalWidth > 0) {
                apply();
            } else {
                img.addEventListener('load', apply);
                img.addEventListener('error', function () {
                    badge.textContent = '';
                });
            }
        })(cards[i]);
    }
}

function renderCard(doc, isSearch) {
    var hasValidId = doc.id && typeof doc.id === 'string' && doc.id.trim() !== '';
    var cardCls = 'doc-card' + (isSearch ? ' search-result' : '');
    var simBadge = '';
    if (isSearch && typeof doc.similarity === 'number') {
        var simPercent = (doc.similarity * 100).toFixed(1);
        var simCls = simPercent > 80 ? 'sim-high' : simPercent > 50 ? 'sim-mid' : 'sim-low';
        simBadge = '<span class="sim-badge ' + simCls + '" title="余弦相似度">' + simPercent + '%</span>';
    }

    // Handle image document cards
	    if (App.isImageCollection || doc.role === 'image' || doc.image) {
	        var imageSrc = doc.image || '';
	        var imgSimBadge = '';
	        if (isSearch && typeof doc.similarity === 'number') {
	            var simPercent = (doc.similarity * 100).toFixed(1);
	            var simCls = simPercent > 80 ? 'sim-high' : simPercent > 50 ? 'sim-mid' : 'sim-low';
	            imgSimBadge = '<span class="sim-badge ' + simCls + '" title="累加余弦相似度">' + simPercent + '%</span>';
	        }
        return '<div class="' + cardCls + ' image-card" data-doc-id="' + esc(doc.id) + '">' +
            '<span class="img-size-badge" title="图片尺寸"></span>' +
            imgSimBadge +
            '<div class="doc-body">' +
            '<div class="doc-id-row">' +
            '<code class="doc-id-tag">' + esc(doc.id) + '</code>' +
            '<div class="doc-actions-inline">' +
            (hasValidId
                ? '<button class="btn-icon-sm btn-del" title="删除此图片" data-delete-id="' + esc(doc.id) + '">' +
                '<i class="fa-solid fa-trash-can"></i>' +
                '</button>'
                : '') +
            '</div>' +
            '</div>' +
            (imageSrc
                ? '<div class="image-preview-container"><img data-src="' + escAttr(imageSrc) + '" class="image-preview-thumb" alt="图片" decoding="async"></div>'
                : '<div class="image-preview-container"><span class="image-placeholder"><i class="fa-solid fa-image"></i> 图片数据将通过搜索加载</span></div>') +
            '</div>' +
            '</div>';
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

// ========== 分页（已取消，改为一次性加载全部） ==========
function renderPagination() {
    D.pagination.style.display = 'none';
}

function goPrevPage() { /* 分页已取消 */ }

function goNextPage() { /* 分页已取消 */ }

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
    if (!App.initialized || !App.currentCollection) return;

    setLoading(true);
    // 检索时主内容区只展示 Top-N 条最相关的命中结果
    var topK = 15;

    try {
        // v2: 统一使用 /messages 端点（text 和 image 共用）
        var url = API.collection(App.currentCollection).MESSAGES + '?query=' + encodeURIComponent(query) + '&top_k=' + topK;
        var resp = await fetch(url);
        var result = await resp.json();

        if (result.success && result.data) {
            var data = result.data;
            App.searchResults = data.results || [];
            App.totalDocs = data.total_found || App.searchResults.length;

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

// ========== 添加文档 ==========

function showAddDocModal() {
    if (!App.initialized) {
        showToast('记忆库未初始化，请先完成初始化', 'error');
        return;
    }
    if (!App.currentCollection) {
        showToast('请先选择一个集合', 'error');
        return;
    }
    D.addRole.value = 'user';
    D.addContent.value = '';
    D.modalAddDoc.style.display = 'flex';
    setTimeout(function () { D.addContent.focus(); }, 100);
}

async function handleAddDocument() {
    if (!App.initialized || !App.currentCollection) {
        showToast('请先选择一个集合', 'error');
        return;
    }

    var role = D.addRole.value;
    var content = D.addContent.value.trim();

    if (!content) { showToast('文档内容不能为空', 'error'); return; }

    D.btnAddSubmit.disabled = true;
    showBtnLoading(D.btnAddSubmit, true, '添加中...');

    try {
        var resp = await fetch(API.collection(App.currentCollection).MESSAGES, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ role: role, content: content })
        });
        var result = await resp.json();

        if (result.success) {
            showToast('文档添加成功', 'success');
            closeModalById('modal-add-doc');
            App.page = 1;
            App.searchMode = false;
            App.searchQuery = '';
            D.searchInput.value = '';
            D.btnClearSearch.style.display = 'none';
            loadCollectionStats(App.currentCollection);
            loadCollections();
        } else {
            showToast('添加失败: ' + (result.error || '未知错误'), 'error');
        }
    } catch (err) {
        showToast('网络请求失败: ' + err.message, 'error');
    } finally {
        D.btnAddSubmit.disabled = false;
        showBtnLoading(D.btnAddSubmit, false, '添加到记忆库');
    }
}

// ========== 删除文档 ==========

function confirmDeleteDoc(id, button) {
    var docEl = button.closest('.doc-card');
    var roleBadge = docEl ? docEl.querySelector('.doc-role-badge') : null;
    var role = roleBadge ? roleBadge.textContent : '未知';

    showConfirmModal(
        '确认删除',
        '<div class="modal-delete-body">' +
        '<p>确定要删除以下文档吗？此操作<strong>不可恢复</strong>。</p>' +
        '<div class="modal-delete-info">' +
        '<span class="modal-delete-id">' + esc(id) + '</span>' +
        '<span class="doc-role-badge ' + esc(role) + '">' + esc(role) + '</span>' +
        '</div>' +
        '</div>',
        function () {
            closeConfirmModal();
            executeDeleteDoc(id, button);
        }
    );
}

async function executeDeleteDoc(id, button) {
    if (!id || typeof id !== 'string' || id.trim() === '') {
        showToast('删除失败：文档ID无效', 'error');
        return;
    }

    if (button) {
        button.disabled = true;
        button.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i>';
    }

    try {
        var resp = await fetch(API.collection(App.currentCollection).MESSAGES, {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ id: id })
        });
        var result = await resp.json();

        if (result.success) {
            showToast('文档删除成功', 'success');

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

            loadCollectionStats(App.currentCollection);
            loadCollections();
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

// ========== 确认弹窗（通用） ==========

function showConfirmModal(title, bodyHTML, onConfirm, confirmText, confirmClass) {
    D.modalTitle.textContent = title;
    D.modalBody.innerHTML = bodyHTML;
    D.modalOverlay.style.display = 'flex';

    D.modalConfirm.textContent = confirmText || '确认';
    D.modalConfirm.className = confirmClass || 'btn-danger';

    var oldConfirm = D.modalConfirm;
    var newConfirm = oldConfirm.cloneNode(true);
    oldConfirm.parentNode.replaceChild(newConfirm, oldConfirm);
    D.modalConfirm = newConfirm;

    D.modalConfirm.addEventListener('click', function () {
        if (onConfirm) onConfirm();
    });

    D.modalConfirm.focus();
}

function closeConfirmModal() {
    D.modalOverlay.style.display = 'none';
    D.modalBody.innerHTML = '';
}

// ========== 模态框工具 ==========

function closeModalById(modalId) {
    var modal = document.getElementById(modalId);
    if (modal) modal.style.display = 'none';
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

// ========== 图片上传与添加 v2 ==========

// toggleOrientationCustomGroup 切换自定义取向输入框的显隐
function toggleOrientationCustomGroup(selectEl, groupEl) {
    if (!selectEl || !groupEl) return;
    groupEl.style.display = selectEl.value === 'custom' ? 'block' : 'none';
}

// getImageOrientation 读取识别取向与自定义参考文本
function getImageOrientation(selectEl, customEl) {
    var orientation = (selectEl && selectEl.value) ? selectEl.value : 'auto';
    var custom = '';
    if (orientation === 'custom' && customEl) {
        custom = customEl.value.trim();
    }
    return { orientation: orientation, custom: custom };
}

function showAddImageModal() {
    if (!App.initialized) {
        showToast('记忆库未初始化，请先完成初始化', 'error');
        return;
    }
    if (!App.currentCollection) {
        showToast('请先选择一个集合', 'error');
        return;
    }
    clearImageUpload();
    D.imageOrientation.value = 'auto';
    D.imageOrientationCustom.value = '';
    toggleOrientationCustomGroup(D.imageOrientation, D.imageOrientationCustomGroup);
    D.modalAddImage.style.display = 'flex';
}

function clearImageUpload() {
    App.imageBase64 = null;
    D.addImageFile.value = '';
    D.imageUploadPlaceholder.style.display = 'flex';
    D.imageUploadPreview.style.display = 'none';
    D.btnRemoveImage.style.display = 'none';
    D.imageUploadArea.classList.remove('has-image');
}

function handleImageFileSelected(e) {
    if (e.target.files && e.target.files[0]) {
        processImageFile(e.target.files[0]);
    }
}

function processImageFile(file) {
    if (!file.type.startsWith('image/')) {
        showToast('请选择图片文件', 'error');
        return;
    }

    if (file.size > 10 * 1024 * 1024) {
        showToast('图片大小不能超过 10MB', 'error');
        return;
    }

    var reader = new FileReader();
    reader.onload = function (e) {
        App.imageBase64 = e.target.result;
        D.imageUploadPreview.src = e.target.result;
        D.imageUploadPlaceholder.style.display = 'none';
        D.imageUploadPreview.style.display = 'block';
        D.btnRemoveImage.style.display = 'flex';
        D.imageUploadArea.classList.add('has-image');
    };
    reader.onerror = function () {
        showToast('图片读取失败', 'error');
    };
    reader.readAsDataURL(file);
}

async function handleAddImage() {
    if (!App.initialized || !App.currentCollection) {
        showToast('请先选择一个集合', 'error');
        return;
    }

    if (!App.imageBase64) {
        showToast('请先选择一张图片', 'error');
        return;
    }

    D.btnAddImageSubmit.disabled = true;
    showBtnLoading(D.btnAddImageSubmit, true, '添加中...');

    var orientation = getImageOrientation(D.imageOrientation, D.imageOrientationCustom);

    try {
        // v2: 统一使用 /messages 端点，LLM 自动生成标签
        var resp = await fetch(API.collection(App.currentCollection).MESSAGES, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                image: App.imageBase64,
                recognition_orientation: orientation.orientation,
                recognition_custom: orientation.custom
            })
        });
        var result = await resp.json();

        if (result.success) {
            showToast('图片添加成功', 'success');
            closeModalById('modal-add-image');
            App.page = 1;
            App.searchMode = false;
            App.searchQuery = '';
            D.searchInput.value = '';
            D.btnClearSearch.style.display = 'none';
            loadCollectionStats(App.currentCollection);
            loadCollections();
        } else {
            showToast('添加失败: ' + (result.error || '未知错误'), 'error');
        }
    } catch (err) {
        showToast('网络请求失败: ' + err.message, 'error');
    } finally {
        D.btnAddImageSubmit.disabled = false;
        showBtnLoading(D.btnAddImageSubmit, false, '添加到记忆库');
    }
}

// ========== 批量导入 ==========

function showBatchImportModal() {
    if (!App.initialized) {
        showToast('记忆库未初始化，请先完成初始化', 'error');
        return;
    }
    if (!App.currentCollection) {
        showToast('请先选择一个集合', 'error');
        return;
    }
    if (!App.isImageCollection) {
        showToast('批量导入仅支持图片类型集合，请先选择图片集合', 'warn');
        return;
    }

    D.batchPath.value = '';
    D.batchOrientation.value = 'auto';
    D.batchOrientationCustom.value = '';
    toggleOrientationCustomGroup(D.batchOrientation, D.batchOrientationCustomGroup);
    D.batchFileList.innerHTML = '<div class="batch-empty-hint"><i class="fa-solid fa-arrow-up"></i> 输入路径后点击扫描，将列出目录中的图片文件</div>';
    D.batchProgress.style.display = 'none';
    D.batchLog.style.display = 'none';
    D.batchLog.innerHTML = '';
    D.btnStartBatch.disabled = true;
    App.batchFiles = [];
    App.batchRunning = false;
    D.modalBatchImport.style.display = 'flex';
}

async function handleScanPath() {
    var path = D.batchPath.value.trim();
    if (!path) {
        showToast('请输入目录路径', 'error');
        return;
    }

    D.btnScanPath.disabled = true;
    showBtnLoading(D.btnScanPath, true, '扫描中...');

    try {
        // 通过 /file/list/{path} 端点扫描目录（路径拼接，非查询参数）
        var resp = await fetch('/file/list/' + path);
        if (!resp.ok) throw new Error('HTTP ' + resp.status);
        var files = await resp.json();

        // 返回格式为直接数组 [{name, path, type, ...}, ...]
        if (Array.isArray(files)) {
            // 筛选图片文件
            var imageExts = ['.jpg', '.jpeg', '.png', '.gif', '.webp', '.bmp', '.svg'];
            App.batchFiles = files.filter(function (f) {
                var name = (f.name || '').toLowerCase();
                for (var i = 0; i < imageExts.length; i++) {
                    if (name.endsWith(imageExts[i])) return true;
                }
                return false;
            }).map(function (f) {
                return { name: f.name, path: f.path };
            });

            renderBatchFileList();
            D.btnStartBatch.disabled = App.batchFiles.length === 0;
            showToast('扫描完成，找到 ' + App.batchFiles.length + ' 张图片', 'success');
        } else {
            D.batchFileList.innerHTML = '<div class="batch-empty-hint"><i class="fa-solid fa-circle-exclamation"></i> 目录不存在或无法访问</div>';
            App.batchFiles = [];
            D.btnStartBatch.disabled = true;
        }
    } catch (err) {
        D.batchFileList.innerHTML = '<div class="batch-empty-hint"><i class="fa-solid fa-circle-exclamation"></i> 扫描失败: ' + esc(err.message) + '</div>';
        App.batchFiles = [];
        D.btnStartBatch.disabled = true;
    } finally {
        D.btnScanPath.disabled = false;
        showBtnLoading(D.btnScanPath, false, '扫描');
    }
}

function renderBatchFileList() {
    if (App.batchFiles.length === 0) {
        D.batchFileList.innerHTML = '<div class="batch-empty-hint"><i class="fa-solid fa-inbox"></i> 目录中没有图片文件</div>';
        return;
    }

    var html = '<div class="batch-file-count">找到 <strong>' + App.batchFiles.length + '</strong> 张图片</div>';
    var maxShow = 20;
    for (var i = 0; i < Math.min(App.batchFiles.length, maxShow); i++) {
        var f = App.batchFiles[i];
        html += '<div class="batch-file-item"><i class="fa-solid fa-image"></i> <span>' + esc(f.name) + '</span>' +
            '<span class="batch-file-status" id="batch-status-' + i + '">等待</span></div>';
    }
    if (App.batchFiles.length > maxShow) {
        html += '<div class="batch-file-item batch-file-more">... 还有 ' + (App.batchFiles.length - maxShow) + ' 个文件</div>';
    }
    D.batchFileList.innerHTML = html;
}

async function handleStartBatch() {
    if (App.batchFiles.length === 0 || App.batchRunning) return;

    App.batchRunning = true;
    D.btnStartBatch.disabled = true;
    D.batchProgress.style.display = 'flex';
    D.batchLog.style.display = 'block';
    D.batchLog.innerHTML = '';
    updateBatchProgress(0, App.batchFiles.length);

    var successCount = 0;
    var failCount = 0;
    var total = App.batchFiles.length;
    var orientation = getImageOrientation(D.batchOrientation, D.batchOrientationCustom);

    for (var i = 0; i < total; i++) {
        if (!App.batchRunning) break;

        var f = App.batchFiles[i];
        updateFileStatus(i, '读取中...');

        try {
            // 1. 读取图片文件为 base64
            var base64 = await readFileAsBase64(f.path);
            if (!base64) {
                updateFileStatus(i, '读取失败');
                appendBatchLog('✗', f.name, '文件读取失败');
                failCount++;
                updateBatchProgress(i + 1, total);
                continue;
            }

            // 2. 直接添加到记忆库（v2: LLM 自动生成标签）
            updateFileStatus(i, '上传中...');
            var addResp = await fetch(API.collection(App.currentCollection).MESSAGES, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    image: base64,
                    recognition_orientation: orientation.orientation,
                    recognition_custom: orientation.custom
                })
            });
            var addResult = await addResp.json();

            if (addResult.success) {
                updateFileStatus(i, '✓ 成功');
                appendBatchLog('✓', f.name, '导入成功');
                successCount++;
            } else {
                updateFileStatus(i, '添加失败');
                appendBatchLog('✗', f.name, (addResult.error || '添加失败'));
                failCount++;
            }
        } catch (err) {
            updateFileStatus(i, '错误');
            appendBatchLog('✗', f.name, err.message);
            failCount++;
        }

        updateBatchProgress(i + 1, total);
    }

    // 完成
    App.batchRunning = false;
    D.btnStartBatch.disabled = false;
    appendBatchLog('', '', '--- 批量导入完成：成功 ' + successCount + '，失败 ' + failCount + ' ---');
    showToast('批量导入完成: 成功 ' + successCount + '/失败 ' + failCount, successCount > 0 ? 'success' : 'error');

    // 刷新集合
    loadCollectionStats(App.currentCollection);
    loadCollections();
}

// readFileAsBase64 通过 /file/read/{path} 端点读取文件并转为 base64
async function readFileAsBase64(filePath) {
    var resp = await fetch('/file/read/' + filePath);
    if (!resp.ok) throw new Error('HTTP ' + resp.status);

    var blob = await resp.blob();
    return new Promise(function (resolve, reject) {
        var reader = new FileReader();
        reader.onload = function () { resolve(reader.result); };
        reader.onerror = function () { reject(new Error('base64 转换失败')); };
        reader.readAsDataURL(blob);
    });
}

function updateFileStatus(index, status) {
    var el = document.getElementById('batch-status-' + index);
    if (el) el.textContent = status;
}

function updateBatchProgress(current, total) {
    var pct = total > 0 ? Math.round((current / total) * 100) : 0;
    D.batchProgressFill.style.width = pct + '%';
    D.batchProgressText.textContent = current + '/' + total;
}

function appendBatchLog(icon, name, msg) {
    var line = document.createElement('div');
    line.className = 'batch-log-line';
    if (icon) {
        line.innerHTML = '<span class="batch-log-icon">' + icon + '</span> ' +
            '<span class="batch-log-name">' + esc(name) + '</span> ' +
            '<span class="batch-log-msg">' + esc(msg) + '</span>';
    } else {
        line.innerHTML = '<span class="batch-log-msg batch-log-summary">' + esc(msg) + '</span>';
    }
    D.batchLog.appendChild(line);
    D.batchLog.scrollTop = D.batchLog.scrollHeight;
}

init();