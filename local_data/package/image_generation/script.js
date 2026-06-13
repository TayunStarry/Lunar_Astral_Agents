// 全局变量
let currentTaskId = null;
let uploadedImagePath = null;
let currentImageBase64 = null;

// DOM元素引用
const elements = {
    prompt: document.getElementById('prompt'),
    negativePrompt: document.getElementById('negative-prompt'),
    initImage: document.getElementById('init-image'),
    uploadArea: document.getElementById('upload-area'),
    imagePreview: document.getElementById('image-preview'),
    uploadPlaceholder: document.getElementById('upload-placeholder'),
    clearImageBtn: document.getElementById('clear-image-btn'),
    widthSlider: document.getElementById('width'),
    widthValue: document.getElementById('width-value'),
    heightSlider: document.getElementById('height'),
    heightValue: document.getElementById('height-value'),
    strengthSlider: document.getElementById('strength'),
    strengthValue: document.getElementById('strength-value'),
    steps: document.getElementById('steps'),
    batchSize: document.getElementById('batch-size'),
    cfgScale: document.getElementById('cfg-scale'),
    seed: document.getElementById('seed'),
    allowSuperResolution: document.getElementById('allow-super-resolution'),
    optimizeBtn: document.getElementById('optimize-btn'),
    generateBtn: document.getElementById('generate-btn'),
    resetBtn: document.getElementById('reset-btn'),
    refreshBtn: document.getElementById('refresh-btn'),
    clearAllBtn: document.getElementById('clear-all-btn'),
    fileGrid: document.getElementById('file-grid'),
    taskStatus: document.getElementById('task-status'),
    taskMessage: document.getElementById('task-message'),
    toastContainer: document.getElementById('toast-container')
};

// ==== 初始化入口 ====
function initEventListeners() {
    // 滑块值更新
    elements.widthSlider.addEventListener('input', updateWidthValue);
    elements.heightSlider.addEventListener('input', updateHeightValue);
    elements.strengthSlider.addEventListener('input', updateStrengthValue);

    // 智能优化
    elements.optimizeBtn.addEventListener('click', optimizePromptAndParameters);

    // 参考图上传
    elements.uploadArea.addEventListener('click', () => elements.initImage.click());
    elements.uploadArea.addEventListener('dragover', (e) => {
        e.preventDefault();
        elements.uploadArea.style.borderColor = 'rgba(102,126,234,0.8)';
    });
    elements.uploadArea.addEventListener('dragleave', () => {
        elements.uploadArea.style.borderColor = 'rgba(255,255,255,0.5)';
    });
    elements.uploadArea.addEventListener('drop', handleDrop);

    elements.initImage.addEventListener('change', handleImageSelect);
    elements.clearImageBtn.addEventListener('click', clearReferenceImage);

    // 底部操作按钮
    elements.generateBtn.addEventListener('click', generateImage);
    elements.resetBtn.addEventListener('click', resetParameters);
    elements.refreshBtn.addEventListener('click', refreshPage);
    elements.clearAllBtn.addEventListener('click', clearAllFiles);

    // 超分按钮切换
    elements.allowSuperResolution.addEventListener('click', () => {
        const btn = elements.allowSuperResolution;
        const isActive = btn.classList.toggle('active');
        btn.innerHTML = isActive
            ? '<i class="fas fa-check-circle"></i> 已启用超分'
            : '<i class="fas fa-expand-arrows-alt"></i> 允许超分';
    });

    // 键盘快捷键
    document.addEventListener('keydown', handleKeyboardShortcuts);

    // 初始化显示值
    updateWidthValue();
    updateHeightValue();
    updateStrengthValue();

    // 加载默认数据
    loadDefaultPrompts();
    loadFileList();
}

// ==== 参数值更新 ====
function updateWidthValue() {
    elements.widthValue.textContent = elements.widthSlider.value;
}

function updateHeightValue() {
    elements.heightValue.textContent = elements.heightSlider.value;
}

function updateStrengthValue() {
    elements.strengthValue.textContent = parseFloat(1 - elements.strengthSlider.value).toFixed(2);
}

// ==== 提示词加载 ====
async function loadDefaultPrompts() {
    try {
        const positiveResponse = await fetch('positive_prompt.md');
        if (positiveResponse.ok) {
            const positiveText = await positiveResponse.text();
            const cleanPositive = positiveText.replace(/^\s*\/\/.*$/gm, '').trim();
            if (cleanPositive) {
                elements.prompt.value = cleanPositive;
            }
        }

        const negativeResponse = await fetch('negative_prompt.md');
        if (negativeResponse.ok) {
            const negativeText = await negativeResponse.text();
            const cleanNegative = negativeText.replace(/^\s*\/\/.*$/gm, '').trim();
            if (cleanNegative) {
                elements.negativePrompt.value = cleanNegative;
            }
        }
    } catch (error) {
        console.log('使用默认提示词:', error);
    }
}

// ==== 参考图片处理 ====
function clearReferenceImage() {
    if (confirm('确定要清除参考图片吗？')) {
        elements.initImage.value = '';
        elements.imagePreview.style.display = 'none';
        elements.imagePreview.src = '';
        elements.uploadPlaceholder.style.display = 'flex';
        elements.clearImageBtn.style.display = 'none';
        uploadedImagePath = null;
        currentImageBase64 = null;
        elements.uploadArea.style.borderColor = 'rgba(255,255,255,0.5)';
        showToast('参考图片已清除', 'info');
    }
}

async function handleDrop(e) {
    e.preventDefault();
    elements.uploadArea.style.borderColor = 'rgba(255,255,255,0.5)';
    const files = e.dataTransfer.files;
    if (files.length > 0) {
        const file = files[0];
        if (file.type.startsWith('image/')) {
            elements.initImage.files = e.dataTransfer.files;
            await handleImageSelect({ target: elements.initImage });
        } else {
            showToast('请选择图片文件', 'error');
        }
    }
}

async function handleImageSelect(e) {
    const file = e.target.files[0];
    if (!file) return;
    if (file.size > 10 * 1024 * 1024) {
        showToast('文件大小不能超过10MB', 'error');
        return;
    }
    if (!file.type.startsWith('image/')) {
        showToast('请选择图片文件', 'error');
        return;
    }

    const reader = new FileReader();
    reader.onload = async (event) => {
        elements.imagePreview.src = event.target.result;
        elements.imagePreview.style.display = 'block';
        elements.uploadPlaceholder.style.display = 'none';
        elements.clearImageBtn.style.display = 'inline-block';
        currentImageBase64 = event.target.result;
    };
    reader.readAsDataURL(file);
    await uploadImage(file);
}

async function uploadImage(file) {
    try {
        const fixedFileName = 'images/uploaded_image.' + (file.name.split('.').pop() || 'png');
        const base64FileName = btoa(fixedFileName);
        const arrayBuffer = await file.arrayBuffer();

        const response = await fetch('/file/write', {
            method: 'POST',
            headers: {
                'X-File-Name': base64FileName,
                'Content-Type': 'application/octet-stream',
                'X-Overwrite': 'true'
            },
            body: arrayBuffer
        });

        if (!response.ok) throw new Error('上传失败: ' + response.statusText);
        const result = await response.json();
        uploadedImagePath = result.filename;
        showToast('图片上传成功', 'success');
    } catch (error) {
        console.error('上传失败:', error);
        showToast(`图片上传失败: ${error.message}`, 'error');
    }
}

// ==== Toast 提示 ====
function showToast(message, type = 'info', duration = 3000) {
    const toast = document.createElement('div');
    toast.className = `toast-glass toast-${type}`;

    const iconMap = {
        success: '<i class="fas fa-check-circle"></i>',
        error: '<i class="fas fa-times-circle"></i>',
        info: '<i class="fas fa-info-circle"></i>'
    };

    toast.innerHTML = `
        <span class="toast-icon">${iconMap[type]}</span>
        <span class="toast-message">${message}</span>
        <button class="toast-close" onclick="this.parentElement.remove()"><i class="fas fa-times"></i></button>
    `;

    elements.toastContainer.appendChild(toast);

    if (duration > 0) {
        setTimeout(() => {
            toast.classList.add('toast-exit');
            setTimeout(() => toast.remove(), 300);
        }, duration);
    }
}

// ==== 参数重置 ====
function resetParameters() {
    if (confirm('确定要重置所有参数吗？当前设置将会丢失。')) {
        loadDefaultPrompts();
        elements.initImage.value = '';
        elements.imagePreview.style.display = 'none';
        elements.imagePreview.src = '';
        elements.uploadPlaceholder.style.display = 'flex';
        elements.clearImageBtn.style.display = 'none';
        elements.widthSlider.value = 512;
        elements.heightSlider.value = 512;
        elements.strengthSlider.value = 0.85;
        elements.steps.value = 20;
        elements.batchSize.value = 1;
        elements.cfgScale.value = 1.0;
        elements.seed.value = 0;
        elements.allowSuperResolution.classList.remove('active');
        elements.allowSuperResolution.innerHTML = '<i class="fas fa-expand-arrows-alt"></i> 允许超分';

        updateWidthValue();
        updateHeightValue();
        updateStrengthValue();
        uploadedImagePath = null;
        currentImageBase64 = null;
        showToast('参数已重置', 'info');
    }
}

function refreshPage() {
    location.reload();
}

// ==== 图片生成 ====
async function generateImage() {
    try {
        const generateData = {
            prompt: elements.prompt.value.trim(),
            negative_prompt: elements.negativePrompt.value.trim(),
            batch_size: parseInt(elements.batchSize.value),
            width: parseInt(elements.widthSlider.value),
            height: parseInt(elements.heightSlider.value),
            strength: parseFloat(elements.strengthSlider.value),
            steps: parseInt(elements.steps.value),
            seed: elements.seed.value === '0' ? Date.now() % 1000000000 : parseInt(elements.seed.value),
            cfg_scale: parseFloat(elements.cfgScale.value),
            init_img: uploadedImagePath || null,
            allow_super_resolution: elements.allowSuperResolution.classList.contains('active')
        };

        if (!generateData.prompt) {
            showToast('请输入提示词', 'error');
            return;
        }
        if (generateData.batch_size < 1 || generateData.batch_size > 8) {
            showToast('生成数量必须在1-8之间', 'error');
            return;
        }

        elements.taskStatus.style.display = 'flex';
        elements.taskMessage.textContent = '提交生成任务...';
        elements.generateBtn.disabled = true;

        const response = await fetch('/generate', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(generateData)
        });

        if (!response.ok) {
            const error = await response.text();
            throw new Error(error);
        }

        const result = await response.json();
        currentTaskId = result.task_id;
        elements.taskMessage.textContent = `任务已排队 (位置: ${result.queue_pos})`;
        waitForTaskCompletion();
    } catch (error) {
        console.error('生成失败:', error);
        showToast(`生成失败: ${error.message}`, 'error');
        elements.taskStatus.style.display = 'none';
        elements.generateBtn.disabled = false;
    }
}

function waitForTaskCompletion() {
    if (!currentTaskId) return;
    try {
        const eventSource = new EventSource(`/generate/wait?task_id=${currentTaskId}`);

        eventSource.onmessage = function (event) {
            try {
                const data = JSON.parse(event.data);
                if (data.status === 'completed') {
                    elements.taskMessage.textContent = '生成完成！';
                    showToast('图像生成完成！', 'success');
                    setTimeout(() => {
                        elements.taskStatus.style.display = 'none';
                        elements.generateBtn.disabled = false;
                    }, 2000);
                    currentTaskId = null;
                    loadFileList();
                    eventSource.close();
                } else if (data.status === 'failed') {
                    elements.taskMessage.textContent = '生成失败';
                    showToast(`生成失败: ${data.error}`, 'error');
                    elements.taskStatus.style.display = 'none';
                    elements.generateBtn.disabled = false;
                    currentTaskId = null;
                    eventSource.close();
                }
            } catch (error) {
                console.error('处理消息失败:', error);
                showToast('处理消息失败，请刷新页面重试', 'error');
                elements.taskStatus.style.display = 'none';
                elements.generateBtn.disabled = false;
                currentTaskId = null;
                eventSource.close();
            }
        };

        eventSource.onerror = function (error) {
            console.error('EventSource 错误:', error);
            showToast('连接失败，请刷新页面重试', 'error');
            elements.taskStatus.style.display = 'none';
            elements.generateBtn.disabled = false;
            currentTaskId = null;
            eventSource.close();
        };
    } catch (error) {
        console.error('创建 EventSource 失败:', error);
        showToast('创建连接失败，请刷新页面重试', 'error');
        elements.taskStatus.style.display = 'none';
        elements.generateBtn.disabled = false;
        currentTaskId = null;
    }
}

// ==== 文件列表管理 ====
async function loadFileList() {
    try {
        elements.fileGrid.innerHTML = '<div class="loading"><div class="spinner"></div></div>';
        let allFiles = await getAllFilesRecursive('images/generated');

        if (allFiles.length === 0) {
            elements.fileGrid.innerHTML = `
                <div class="empty-state">
                    <div class="empty-state-icon"><i class="fas fa-folder-open"></i></div>
                    <p>还没有生成任何文件</p>
                    <p style="font-size: 0.9em; margin-top: 10px; color: #888;">点击"开始生成"按钮创建第一张图片</p>
                </div>`;
            return;
        }

        if (allFiles.length < 8) {
            const missingCount = 8 - allFiles.length;
            for (let i = 0; i < missingCount; i++) {
                const imageUrl = `images/placeholder/unknown_file_icon-0${Math.floor(Math.random() * 4)}.webp`;
                allFiles.push({ name: '*.png', path: imageUrl, size: 0, lastModified: new Date().toISOString(), isDir: false });
            }
        }

        allFiles.sort((a, b) => new Date(b.lastModified) - new Date(a.lastModified));

        const filesHTML = allFiles.map(file => {
            const isImage = isImageFile(file.name);
            const iconHTML = getFileIcon(file.name);
            const sizeFormatted = formatFileSize(file.size);
            const dateFormatted = formatDate(file.lastModified);
            const path = file.path.replace(/\\/g, '/');
            const relativePath = file.path.replace(/^images\/generated[\\/]/, '');
            const displayPath = relativePath || file.name;
            const pathParts = displayPath.split(/[\\/]/);
            const previewContent = isImage
                ? `<img src="/file/read/${path}" alt="${file.name}" onerror="this.onerror=null; this.src='/file/read/images/placeholder/video_file_icon-0${Math.floor(Math.random() * 5)}.png'" onclick="previewImage('/file/read/${path}', '${file.name}')">`
                : `<div style="font-size: 48px; color: var(--primary-color); opacity: 0.3;">${iconHTML}</div>`;

            return `
                <div class="file-card">
                    <div class="file-card-header">
                        <div class="file-icon">${iconHTML}</div>
                        <div class="file-name" title="${displayPath}">
                            <div style="font-size: 0.85em; color: var(--text-light); margin-bottom: 2px;">${pathParts.length > 1 ? pathParts.slice(0, -1).join('/') + '/' : ''}</div>
                            <div style="font-weight: bold;">${pathParts[pathParts.length - 1]}</div>
                        </div>
                    </div>
                    <div class="file-preview">${previewContent}</div>
                    <div class="file-meta">
                        <span>${sizeFormatted}</span>
                        <span>${dateFormatted}</span>
                    </div>
                    <div class="file-actions">
                        <button class="file-btn file-btn-primary" onclick="downloadFile('${path}', '${file.name}')">下载</button>
                        <button class="file-btn file-btn-danger" onclick="deleteFile('${path}')" ${path.startsWith('images/placeholder') ? 'disabled' : ''}>删除</button>
                    </div>
                </div>`;
        }).join('');

        elements.fileGrid.innerHTML = filesHTML;
    } catch (error) {
        console.error('加载文件列表失败:', error);
        elements.fileGrid.innerHTML = `
            <div class="empty-state">
                <div style="color: #dc3545; font-size: 36px;"><i class="fas fa-exclamation-circle"></i></div>
                <p style="color: #dc3545;">加载文件列表失败</p>
                <p style="font-size: 0.9em; margin-top: 10px; color: #666;">${error.message}</p>
                <button onclick="loadFileList()" class="btn-glass btn-glass-secondary" style="margin-top: 15px;">重试</button>
            </div>`;
    }
}

async function getAllFilesRecursive(dirPath) {
    try {
        const response = await fetch(`/file/list/${dirPath}`);
        if (!response.ok) throw new Error(`获取目录 ${dirPath} 失败: ${response.status}`);
        const items = await response.json();
        const allFiles = [];
        for (const item of items) {
            if (item.isDir) {
                const subDirFiles = await getAllFilesRecursive(item.path);
                allFiles.push(...subDirFiles);
            } else {
                allFiles.push(item);
            }
        }
        return allFiles;
    } catch (error) {
        console.error(`递归获取文件失败 (${dirPath}):`, error);
        return [];
    }
}

function getFileIcon(filename) {
    const ext = filename.split('.').pop().toLowerCase();
    const iconMap = {
        'png': '<i class="fas fa-image"></i>', 'jpg': '<i class="fas fa-image"></i>',
        'jpeg': '<i class="fas fa-image"></i>', 'gif': '<i class="fas fa-image"></i>',
        'bmp': '<i class="fas fa-image"></i>', 'webp': '<i class="fas fa-image"></i>',
        'mp4': '<i class="fas fa-film"></i>', 'webm': '<i class="fas fa-film"></i>',
        'ogg': '<i class="fas fa-music"></i>', 'mov': '<i class="fas fa-film"></i>',
        'avi': '<i class="fas fa-film"></i>', 'mkv': '<i class="fas fa-film"></i>',
        'flv': '<i class="fas fa-film"></i>', 'wmv': '<i class="fas fa-film"></i>',
        'm4v': '<i class="fas fa-film"></i>', 'mp3': '<i class="fas fa-music"></i>',
        'wav': '<i class="fas fa-music"></i>', 'flac': '<i class="fas fa-music"></i>',
        'aac': '<i class="fas fa-music"></i>', 'pdf': '<i class="fas fa-file-pdf"></i>',
        'txt': '<i class="fas fa-file-alt"></i>', 'json': '<i class="fas fa-file-code"></i>',
        'default': '<i class="fas fa-file"></i>'
    };
    return iconMap[ext] || iconMap.default;
}

function isImageFile(filename) {
    const imageExts = ['png', 'jpg', 'jpeg', 'gif', 'bmp', 'webp', 'mp4', 'webm', 'ogg', 'mov', 'avi', 'mkv', 'flv', 'wmv', 'm4v'];
    const ext = filename.split('.').pop().toLowerCase();
    return imageExts.includes(ext);
}

function formatFileSize(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

function formatDate(dateString) {
    const date = new Date(dateString);
    const now = new Date();
    const diffMs = now - date;
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);

    if (diffMins < 1) return '刚刚';
    if (diffMins < 60) return `${diffMins}分钟前`;
    if (diffHours < 24) return `${diffHours}小时前`;
    if (diffDays < 7) return `${diffDays}天前`;

    return date.toLocaleDateString('zh-CN', {
        month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit'
    });
}

async function downloadFile(path, filename) {
    try {
        showToast('开始下载...', 'info');
        const link = document.createElement('a');
        link.href = `/file/download/${path}`;
        link.download = filename;
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
        showToast('下载链接已打开', 'info');
    } catch (error) {
        console.error('下载失败:', error);
        showToast(`下载失败: ${error.message}`, 'error');
        window.open(`/file/download/${path}`, '_blank');
    }
}

async function deleteFile(path) {
    if (!confirm(`确定要删除文件吗？\n${path.replace(/^images\/generated[\\/]/, '')}`)) return;
    try {
        const response = await fetch(`/file/delete/${path}`, { method: 'DELETE' });
        if (response.ok) {
            showToast('文件删除成功', 'success');
            loadFileList();
        } else {
            const errorText = await response.text();
            throw new Error(errorText);
        }
    } catch (error) {
        console.error('删除失败:', error);
        showToast(`删除失败: ${error.message}`, 'error');
    }
}

async function clearAllFiles() {
    if (!confirm('确定要删除所有生成的文件吗？\n此操作不可恢复！')) return;
    try {
        showToast('清空所有文件中...', 'info');
        const response = await fetch('/file/delete/images/generated', { method: 'DELETE' });
        if (response.ok) {
            showToast('所有文件已清空', 'success');
            loadFileList();
        } else {
            const errorText = await response.text();
            throw new Error(errorText);
        }
    } catch (error) {
        console.error('清空失败:', error);
        showToast(`清空失败: ${error.message}`, 'error');
    }
}

// ==== 图片预览 ====
function previewImage(src, name) {
    const overlay = document.createElement('div');
    overlay.style.cssText = `
        position: fixed; top: 0; left: 0; right: 0; bottom: 0;
        background: rgba(0,0,0,0.8); z-index: 2000;
        display: flex; align-items: center; justify-content: center;
        cursor: pointer;
    `;
    overlay.innerHTML = `
        <img src="${src}" alt="${name}" style="max-width: 90vw; max-height: 90vh; object-fit: contain; border-radius: 8px;">
        <button style="position: absolute; top: 20px; right: 20px; background: rgba(255,255,255,0.2); border: none; color: white; font-size: 24px; cursor: pointer; width: 40px; height: 40px; border-radius: 50%;">×</button>
    `;
    overlay.addEventListener('click', (e) => {
        if (e.target === overlay || e.target.tagName === 'BUTTON') {
            overlay.remove();
        }
    });
    document.body.appendChild(overlay);
}

// ==== 键盘快捷键 ====
function handleKeyboardShortcuts(e) {
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
        e.preventDefault();
        elements.generateBtn.click();
    }
    if ((e.ctrlKey || e.metaKey) && e.key === 'r' && !e.shiftKey) {
        e.preventDefault();
        refreshPage();
    }
    if (e.key === 'Escape') {
        const toasts = document.querySelectorAll('.toast-glass');
        toasts.forEach(toast => {
            toast.classList.add('toast-exit');
            setTimeout(() => toast.remove(), 300);
        });
    }
}

// ==== 智能优化 ====
async function optimizePromptAndParameters() {
    const prompt = elements.prompt.value.trim();
    if (!prompt) {
        showToast('请先输入提示词', 'error');
        return;
    }

    elements.optimizeBtn.disabled = true;
    elements.optimizeBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> 优化中...';

    try {
        const messages = buildOptimizationMessages(prompt);
        const result = await callMultimodalModel(messages);
        applyOptimizationResult(result);
        showToast('优化完成！', 'success');
    } catch (error) {
        console.error('优化失败:', error);
        showToast(`优化失败: ${error.message}`, 'error');
    } finally {
        elements.optimizeBtn.disabled = false;
        elements.optimizeBtn.innerHTML = '<i class="fas fa-wand-magic-sparkles"></i> 智能优化';
    }
}

function buildOptimizationMessages(prompt) {
    const currentParams = {
        width: parseInt(elements.widthSlider.value),
        height: parseInt(elements.heightSlider.value),
        steps: parseInt(elements.steps.value),
        cfg_scale: parseFloat(elements.cfgScale.value),
        strength: parseFloat(elements.strengthSlider.value),
        batch_size: parseInt(elements.batchSize.value),
        seed: elements.seed.value === '0' ? null : parseInt(elements.seed.value),
        allow_super_resolution: elements.allowSuperResolution.classList.contains('active')
    };

    let userMessage = `请优化以下提示词和参数设置：

正面提示词：
${prompt}

当前参数：
- 宽度: ${currentParams.width}
- 高度: ${currentParams.height}
- 迭代步数: ${currentParams.steps}
- 提示词权重: ${currentParams.cfg_scale}
- 噪声强度: ${currentParams.strength}
- 生成数量: ${currentParams.batch_size}
- 允许超分: ${currentParams.allow_super_resolution ? '是' : '否'}
${currentParams.seed ? `- 随机种子: ${currentParams.seed}` : ''}`;

    if (currentImageBase64) {
        userMessage += `

参考图片：已提供（见附件）`;
    }

    userMessage += `

请使用图像生成参数优化工具来优化这些设置，确保参数组合合理有效。`;

    const messages = [
        {
            role: 'system',
            content: `你是一个专业的AI图像生成专家。请帮助用户优化他们的提示词和生成参数。

当用户提供提示词和参数时，你必须使用"image_generation_parameters"工具来返回优化后的结果。

优化原则：
1. 正面提示词：增强细节描述，保持风格一致性，适当添加质量标签
2. 负面提示词：补充常见的质量问题，确保覆盖全面
3. 参数调整：根据提示词复杂度调整步数和CFG，风景建议更高分辨率，人物建议适当降低强度
4. 所有参数必须在有效范围内：width/height(256-2048), steps(1-100), cfg_scale(0.1-3.0), strength(0.1-1.0), batch_size(1-8)

请始终使用工具返回结果，不要仅做文字描述。`
        },
        {
            role: 'user',
            content: userMessage
        }
    ];

    if (currentImageBase64) {
        messages[1] = {
            role: 'user',
            content: [
                {
                    type: 'text',
                    text: userMessage
                },
                {
                    type: 'image_url',
                    image_url: {
                        url: currentImageBase64
                    }
                }
            ]
        };
    }

    return messages;
}

async function callMultimodalModel(messages) {
    const API_URL = '/v1/chat/completions';

    const requestBody = {
        model: 'system-multimodal',
        messages: messages,
        tools: [
            {
                type: 'function',
                function: {
                    name: 'image_generation_parameters',
                    description: '优化图像生成提示词和参数设置',
                    parameters: {
                        type: 'object',
                        properties: {
                            optimized_prompt: {
                                type: 'string',
                                description: '优化后的正面提示词'
                            },
                            optimized_negative_prompt: {
                                type: 'string',
                                description: '优化后的负面提示词'
                            },
                            width: {
                                type: 'integer',
                                description: '图像宽度 (256-2048)',
                                minimum: 256,
                                maximum: 2048
                            },
                            height: {
                                type: 'integer',
                                description: '图像高度 (256-2048)',
                                minimum: 256,
                                maximum: 2048
                            },
                            steps: {
                                type: 'integer',
                                description: '迭代步数 (1-100)',
                                minimum: 1,
                                maximum: 100
                            },
                            cfg_scale: {
                                type: 'number',
                                description: '提示词权重 (0.1-3.0)',
                                minimum: 0.1,
                                maximum: 3.0
                            },
                            strength: {
                                type: 'number',
                                description: '噪声强度/重绘幅度 (0.1-1.0)',
                                minimum: 0.1,
                                maximum: 1.0
                            },
                            batch_size: {
                                type: 'integer',
                                description: '生成数量 (1-8)',
                                minimum: 1,
                                maximum: 8
                            },
                            seed: {
                                type: 'integer',
                                description: '随机种子 (可选，不提供则随机)',
                                minimum: 0
                            },
                            allow_super_resolution: {
                                type: 'boolean',
                                description: '是否启用超分 (默认 false)'
                            }
                        },
                        required: ['optimized_prompt', 'optimized_negative_prompt', 'width', 'height', 'steps', 'cfg_scale', 'strength', 'batch_size']
                    }
                }
            }
        ],
        tool_choice: {
            type: 'function',
            function: {
                name: 'image_generation_parameters'
            }
        },
        temperature: 0.7
    };

    const response = await fetch(API_URL, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json'
        },
        body: JSON.stringify(requestBody)
    });

    if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`API调用失败: ${response.status} - ${errorText}`);
    }

    const result = await response.json();

    if (!result.choices || !result.choices[0]) {
        throw new Error('无效的API响应格式');
    }

    const message = result.choices[0].message;

    if (message.tool_calls && message.tool_calls[0]) {
        const toolCall = message.tool_calls[0];
        if (toolCall.function && toolCall.function.arguments) {
            try {
                const args = JSON.parse(toolCall.function.arguments);
                return {
                    success: true,
                    data: args
                };
            } catch (parseError) {
                throw new Error('解析工具参数失败: ' + parseError.message);
            }
        }
    }

    if (message.content) {
        try {
            const jsonMatch = message.content.match(/\{[\s\S]*\}/);
            if (jsonMatch) {
                const args = JSON.parse(jsonMatch[0]);
                return {
                    success: true,
                    data: args
                };
            }
        } catch (parseError) {
            console.log('尝试直接解析content:', parseError);
        }
    }

    throw new Error('未获得有效的优化结果');
}

function applyOptimizationResult(result) {
    if (!result.success || !result.data) {
        throw new Error('优化结果格式无效');
    }

    const data = result.data;

    if (data.optimized_prompt) {
        elements.prompt.value = data.optimized_prompt;
    }

    if (data.optimized_negative_prompt) {
        elements.negativePrompt.value = data.optimized_negative_prompt;
    }

    if (data.width && data.width >= 256 && data.width <= 2048) {
        elements.widthSlider.value = data.width;
        updateWidthValue();
    }

    if (data.height && data.height >= 256 && data.height <= 2048) {
        elements.heightSlider.value = data.height;
        updateHeightValue();
    }

    if (data.steps && data.steps >= 1 && data.steps <= 100) {
        elements.steps.value = data.steps;
    }

    if (data.cfg_scale && data.cfg_scale >= 0.1 && data.cfg_scale <= 3.0) {
        elements.cfgScale.value = data.cfg_scale;
    }

    if (data.strength && data.strength >= 0.1 && data.strength <= 1.0) {
        elements.strengthSlider.value = data.strength;
        updateStrengthValue();
    }

    if (data.batch_size && data.batch_size >= 1 && data.batch_size <= 8) {
        elements.batchSize.value = data.batch_size;
    }

    if (data.seed && data.seed >= 0) {
        elements.seed.value = data.seed;
    }

    if (data.allow_super_resolution !== undefined) {
        if (data.allow_super_resolution) {
            elements.allowSuperResolution.classList.add('active');
            elements.allowSuperResolution.innerHTML = '<i class="fas fa-check-circle"></i> 已启用超分';
        } else {
            elements.allowSuperResolution.classList.remove('active');
            elements.allowSuperResolution.innerHTML = '<i class="fas fa-expand-arrows-alt"></i> 允许超分';
        }
    }
}

// ==== 启动 ====
document.addEventListener('DOMContentLoaded', initEventListeners);