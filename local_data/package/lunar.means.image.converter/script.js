// ==== 图片格式转换工具 ====

// DOM 元素引用
const elements = {
    folderPath: document.getElementById('folder-path'),
    browseBtn: document.getElementById('browse-btn'),
    sourceFormatTabs: document.getElementById('source-format-tabs'),
    targetFormatTabs: document.getElementById('target-format-tabs'),
    deleteSourceToggle: document.getElementById('delete-source-toggle'),
    batchConvertBtn: document.getElementById('batch-convert-btn'),
    refreshBtn: document.getElementById('refresh-btn'),
    fileGrid: document.getElementById('file-grid'),
    emptyState: document.getElementById('empty-state'),
    fileCount: document.getElementById('file-count'),
    gridTitle: document.getElementById('grid-title'),
    toastContainer: document.getElementById('toast-container'),
};

// 状态
let state = {
    currentFolder: '',
    sourceFormat: 'all',
    targetFormat: 'webp',
    deleteSource: true,
    files: [],
};

// 支持的图片格式
const SUPPORTED_FORMATS = ['png', 'jpeg', 'webp'];
const FORMAT_EXTENSIONS = { png: '.png', jpeg: '.jpg', webp: '.webp' };

// ==== 初始化 ====
function init() {
    bindEvents();
    // 尝试从 localStorage 恢复上次路径
    const savedPath = localStorage.getItem('image_converter_folder');
    if (savedPath) {
        elements.folderPath.value = savedPath;
        loadFiles(savedPath);
    }
}

// ==== 事件绑定 ====
function bindEvents() {
    // 浏览按钮
    elements.browseBtn.addEventListener('click', () => {
        const path = elements.folderPath.value.trim();
        if (!path) {
            showToast('请输入文件夹路径', 'info');
            return;
        }
        const resolvedPath = resolvePath(path);
        elements.folderPath.value = resolvedPath;
        state.currentFolder = resolvedPath;
        localStorage.setItem('image_converter_folder', resolvedPath);
        loadFiles(resolvedPath);
    });

    // 回车键浏览
    elements.folderPath.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') {
            elements.browseBtn.click();
        }
    });

    // 源格式选项卡
    elements.sourceFormatTabs.addEventListener('click', (e) => {
        const btn = e.target.closest('.tab-btn');
        if (!btn) return;
        setActiveTab(elements.sourceFormatTabs, btn);
        state.sourceFormat = btn.dataset.format;
        renderFiles();
    });

    // 目标格式选项卡
    elements.targetFormatTabs.addEventListener('click', (e) => {
        const btn = e.target.closest('.tab-btn');
        if (!btn) return;
        setActiveTab(elements.targetFormatTabs, btn);
        state.targetFormat = btn.dataset.format;
    });

    // 删除源文件开关
    elements.deleteSourceToggle.addEventListener('click', () => {
        state.deleteSource = !state.deleteSource;
        elements.deleteSourceToggle.classList.toggle('active', state.deleteSource);
    });

    // 批量转换
    elements.batchConvertBtn.addEventListener('click', () => {
        if (!state.currentFolder) {
            showToast('请先选择文件夹', 'info');
            return;
        }
        if (state.sourceFormat === state.targetFormat) {
            showToast('源格式和目标格式相同，无需转换', 'info');
            return;
        }
        batchConvert();
    });

    // 刷新
    elements.refreshBtn.addEventListener('click', () => {
        if (state.currentFolder) {
            loadFiles(state.currentFolder);
        }
    });
}

// ==== 路径解析 ====
function resolvePath(inputPath) {
    // 去除首尾空格和引号
    let path = inputPath.trim().replace(/^["']|["']$/g, '');
    // 将反斜杠统一为正斜杠用于判断，但保留原始路径格式
    const normalized = path.replace(/\\/g, '/');
    // 判断是否为文件路径（有扩展名且不是目录）
    const lastPart = normalized.split('/').pop();
    if (lastPart && lastPart.includes('.')) {
        // 可能是文件路径，取父目录
        const lastSlash = Math.max(normalized.lastIndexOf('/'), normalized.lastIndexOf('\\'));
        if (lastSlash > 0) {
            return path.substring(0, lastSlash);
        }
    }
    return path;
}

// ==== 设置选项卡激活状态 ====
function setActiveTab(container, activeBtn) {
    container.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
    activeBtn.classList.add('active');
}

// ==== 加载文件列表 ====
async function loadFiles(folder) {
    try {
        const response = await fetch('/convert/list', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ folder }),
        });

        const data = await response.json();
        if (!data.success) {
            showToast(data.error || '加载文件列表失败', 'error');
            state.files = [];
            renderFiles();
            return;
        }

        state.files = data.files || [];
        state.currentFolder = data.folder || folder;
        renderFiles();
        showToast(`已加载 ${state.files.length} 个图片文件`, 'info');
    } catch (err) {
        showToast('请求失败: ' + err.message, 'error');
        state.files = [];
        renderFiles();
    }
}

// ==== 渲染文件网格 ====
function renderFiles() {
    const { files, sourceFormat } = state;

    // 过滤文件
    let filteredFiles = files;
    if (sourceFormat !== 'all') {
        filteredFiles = files.filter(f => f.format === sourceFormat);
    }

    // 更新计数
    elements.fileCount.textContent = `${filteredFiles.length} 个文件`;

    // 空状态
    if (filteredFiles.length === 0) {
        elements.emptyState.style.display = '';
        // 移除所有卡片
        elements.fileGrid.querySelectorAll('.file-card').forEach(c => c.remove());
        return;
    }

    elements.emptyState.style.display = 'none';

    // 渲染卡片
    elements.fileGrid.innerHTML = '';
    filteredFiles.forEach(file => {
        const card = createFileCard(file);
        elements.fileGrid.appendChild(card);
    });
}

// ==== 创建文件卡片 ====
function createFileCard(file) {
    const card = document.createElement('div');
    card.className = 'file-card';
    card.dataset.path = file.path;

    // 预览图 URL（通过预览接口，支持任意路径）
    const previewUrl = `/file/preview?path=${encodeURIComponent(file.path)}`;

    card.innerHTML = `
        <img class="file-card-preview" src="${previewUrl}" alt="${file.name}" loading="lazy"
             onerror="this.src='data:image/svg+xml,<svg xmlns=%22http://www.w3.org/2000/svg%22 viewBox=%220 0 100 100%22><rect fill=%22%23ddd%22 width=%22100%22 height=%22100%22/><text x=%2250%22 y=%2255%22 text-anchor=%22middle%22 fill=%22%23999%22 font-size=%2212%22>无预览</text></svg>'">
        <div class="file-card-info">
            <span class="file-card-name" title="${file.name}">${file.name}</span>
            <span class="file-card-format">${file.format.toUpperCase()}</span>
            <div class="file-card-actions">
                <button class="btn-glass btn-glass-success convert-single-btn" title="转换为 ${state.targetFormat.toUpperCase()}">
                    <i class="fas fa-exchange-alt"></i> 转换
                </button>
            </div>
        </div>
    `;

    // 绑定单独转换按钮
    card.querySelector('.convert-single-btn').addEventListener('click', (e) => {
        e.stopPropagation();
        if (file.format === state.targetFormat) {
            showToast('源格式与目标格式相同，无需转换', 'info');
            return;
        }
        convertSingle(file.path);
    });

    return card;
}

// ==== 单张转换 ====
async function convertSingle(filePath) {
    const btn = document.querySelector(`.file-card[data-path="${CSS.escape(filePath)}"] .convert-single-btn`);
    if (btn) {
        btn.disabled = true;
        btn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> 转换中...';
    }

    try {
        const response = await fetch('/convert/image', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                path: filePath,
                target_format: state.targetFormat,
                delete_source: state.deleteSource,
                quality: 90,
            }),
        });

        const data = await response.json();
        if (data.success) {
            showToast('转换成功', 'success');
            // 重新加载文件列表
            if (state.currentFolder) {
                loadFiles(state.currentFolder);
            }
        } else {
            showToast(data.error || '转换失败', 'error');
            if (btn) {
                btn.disabled = false;
                btn.innerHTML = '<i class="fas fa-exchange-alt"></i> 转换';
            }
        }
    } catch (err) {
        showToast('请求失败: ' + err.message, 'error');
        if (btn) {
            btn.disabled = false;
            btn.innerHTML = '<i class="fas fa-exchange-alt"></i> 转换';
        }
    }
}

// ==== 批量转换 ====
async function batchConvert() {
    elements.batchConvertBtn.disabled = true;
    elements.batchConvertBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> 转换中...';

    try {
        const response = await fetch('/convert/batch', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                folder: state.currentFolder,
                source_format: state.sourceFormat,
                target_format: state.targetFormat,
                delete_source: state.deleteSource,
                quality: 90,
            }),
        });

        const data = await response.json();
        if (data.success) {
            const msg = `批量转换完成: 成功 ${data.success_count} 个, 失败 ${data.fail_count} 个`;
            showToast(msg, data.fail_count > 0 ? 'info' : 'success');
            // 重新加载
            loadFiles(state.currentFolder);
        } else {
            showToast(data.error || '批量转换失败', 'error');
        }
    } catch (err) {
        showToast('请求失败: ' + err.message, 'error');
    } finally {
        elements.batchConvertBtn.disabled = false;
        elements.batchConvertBtn.innerHTML = '<i class="fas fa-layer-group"></i> 批量转换';
    }
}

// ==== Toast 消息 ====
function showToast(message, type = 'info') {
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;

    const icons = {
        success: 'fa-check-circle',
        error: 'fa-exclamation-circle',
        info: 'fa-info-circle',
    };

    toast.innerHTML = `<i class="fas ${icons[type] || icons.info}"></i> ${message}`;
    elements.toastContainer.appendChild(toast);

    // 3秒后自动消失
    setTimeout(() => {
        toast.classList.add('fade-out');
        toast.addEventListener('animationend', () => toast.remove());
    }, 3000);
}

// ==== 启动 ====
document.addEventListener('DOMContentLoaded', init);