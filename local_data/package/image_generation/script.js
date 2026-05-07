// 全局变量
let currentTaskId = null;
let uploadedImagePath = null;

// 预设数据（保持不变）
const presets = {
    "anime": {
        name: "动漫人物",
        prompt: "可爱, 萌系风格, 白发绿眼少女, 动漫画风, 卡通形象, 高清",
        negative_prompt: "模糊, 低质量, 变形, 畸形, 水印, 文字, 现实",
        width: 512,
        height: 768,
        steps: 20,
        cfg_scale: 1.0,
        strength: 0.75,
        batch_size: 1
    },
    "landscape": {
        name: "风景画",
        prompt: "壮丽的山脉, 日出, 云海, 湖泊, 极简主义, 油画风格",
        negative_prompt: "人物, 建筑, 文字, 现代元素, 模糊",
        width: 1024,
        height: 512,
        steps: 30,
        cfg_scale: 1.5,
        strength: 0.6,
        batch_size: 1
    },
    "sci-fi": {
        name: "科幻世界",
        prompt: "未来的城市, 机器人, 太空站, 科幻元素, 赛博朋克, 高科技建筑",
        negative_prompt: "模糊, 低质量, 变形, 畸形, 水印, 文字, 现实, 古代, 自然风景, 手绘",
        width: 768,
        height: 768,
        steps: 35,
        cfg_scale: 1.8,
        strength: 0.7,
        batch_size: 2
    },
    "portrait": {
        name: "肖像画",
        prompt: "专业肖像, 精致的面部特征, 戏剧性灯光, 高质量, 8k",
        negative_prompt: "模糊, 低质量, 变形, 畸形, 水印, 文字, 卡通, 动漫",
        width: 512,
        height: 768,
        steps: 28,
        cfg_scale: 1.3,
        strength: 0.65,
        batch_size: 1
    },
    "fantasy": {
        name: "奇幻场景",
        prompt: "巨龙, 魔法城堡, 精灵, 幻想世界, 史诗级场景, 概念艺术",
        negative_prompt: "现代, 科技, 现实, 照片, 低质量, 模糊",
        width: 768,
        height: 512,
        steps: 32,
        cfg_scale: 1.6,
        strength: 0.7,
        batch_size: 2
    },
    "architecture": {
        name: "建筑设计",
        prompt: "现代建筑, 极简主义设计, 玻璃幕墙, 几何形状, 建筑设计图",
        negative_prompt: "杂乱, 破旧, 模糊, 低质量, 人物, 动物",
        width: 768,
        height: 512,
        steps: 30,
        cfg_scale: 1.4,
        strength: 0.6,
        batch_size: 1
    },
    "cyberpunk": {
        name: "赛博朋克",
        prompt: "霓虹灯, 雨夜街道, 高科技低生活, 未来都市, 亚洲风格城市景观",
        negative_prompt: "白天, 自然光, 田园, 古典, 模糊, 低质量",
        width: 768,
        height: 768,
        steps: 35,
        cfg_scale: 1.7,
        strength: 0.75,
        batch_size: 2
    },
    "watercolor": {
        name: "水彩插画",
        prompt: "水彩画风格, 柔和色彩, 艺术感, 手绘质感, 梦幻效果",
        negative_prompt: "照片写实, 3D渲染, 数字感, 生硬边缘, 模糊",
        width: 768,
        height: 768,
        steps: 30,
        cfg_scale: 1.3,
        strength: 0.65,
        batch_size: 1
    }
};

// DOM元素（ID与原HTML保持一致）
const elements = {
    prompt: document.getElementById('prompt'),
    negativePrompt: document.getElementById('negative-prompt'),
    initImage: document.getElementById('init-image'),
    uploadArea: document.getElementById('upload-area'),
    imagePreview: document.getElementById('image-preview'),
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
    presetSelect: document.getElementById('preset-select'),
    generateBtn: document.getElementById('generate-btn'),
    resetBtn: document.getElementById('reset-btn'),
    refreshBtn: document.getElementById('refresh-btn'),
    clearAllBtn: document.getElementById('clear-all-btn'),
    fileGrid: document.getElementById('file-grid'),
    taskStatus: document.getElementById('task-status'),
    taskMessage: document.getElementById('task-message'),
    statusBar: document.getElementById('status-bar'),
    statusMessage: document.getElementById('status-message'),
    taskProgressFill: document.getElementById('task-progress-fill')
};

// 初始化事件监听器
function initEventListeners() {
    elements.widthSlider.addEventListener('input', updateWidthValue);
    elements.heightSlider.addEventListener('input', updateHeightValue);
    elements.strengthSlider.addEventListener('input', updateStrengthValue);
    elements.presetSelect.addEventListener('change', loadPreset);

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

    elements.generateBtn.addEventListener('click', generateImage);
    elements.resetBtn.addEventListener('click', resetParameters);
    elements.refreshBtn.addEventListener('click', refreshFileList);
    elements.clearAllBtn.addEventListener('click', clearAllFiles);

    updateWidthValue();
    updateHeightValue();
    updateStrengthValue();

    loadDefaultPrompts();
    loadFileList();

    document.addEventListener('keydown', handleKeyboardShortcuts);
}

function loadPreset() {
    const presetKey = elements.presetSelect.value;
    if (!presetKey) return;
    const preset = presets[presetKey];
    if (!preset) return;

    elements.prompt.value = preset.prompt;
    elements.negativePrompt.value = preset.negative_prompt;
    elements.widthSlider.value = preset.width;
    elements.heightSlider.value = preset.height;
    elements.steps.value = preset.steps;
    elements.cfgScale.value = preset.cfg_scale;
    elements.strengthSlider.value = preset.strength;
    elements.batchSize.value = preset.batch_size;

    updateWidthValue();
    updateHeightValue();
    updateStrengthValue();
    showStatus(`已加载预设: ${preset.name}`, 'success', 2000);
}

async function loadDefaultPrompts() {
    try {
        const positiveResponse = await fetch('positive_prompt.md');
        if (positiveResponse.ok) {
            const positiveText = await positiveResponse.text();
            const cleanPositive = positiveText.replace(/^\s*\/\/.*$/gm, '').trim();
            elements.prompt.value = cleanPositive || presets.anime.prompt;
        } else {
            elements.prompt.value = presets.anime.prompt;
        }

        const negativeResponse = await fetch('negative_prompt.md');
        if (negativeResponse.ok) {
            const negativeText = await negativeResponse.text();
            const cleanNegative = negativeText.replace(/^\s*\/\/.*$/gm, '').trim();
            elements.negativePrompt.value = cleanNegative || presets.anime.negative_prompt;
        } else {
            elements.negativePrompt.value = presets.anime.negative_prompt;
        }
    } catch (error) {
        console.log('使用预设提示词:', error);
        elements.prompt.value = presets.anime.prompt;
        elements.negativePrompt.value = presets.anime.negative_prompt;
    }
}

function clearReferenceImage() {
    if (confirm('确定要清除参考图片吗？')) {
        elements.initImage.value = '';
        elements.imagePreview.style.display = 'none';
        elements.imagePreview.src = '';
        elements.clearImageBtn.style.display = 'none';
        uploadedImagePath = null;
        elements.uploadArea.style.borderColor = 'rgba(255,255,255,0.5)';
        showStatus('参考图片已清除', 'info');
    }
}

function updateWidthValue() {
    elements.widthValue.textContent = elements.widthSlider.value;
}

function updateHeightValue() {
    elements.heightValue.textContent = elements.heightSlider.value;
}

function updateStrengthValue() {
    elements.strengthValue.textContent = parseFloat(1 - elements.strengthSlider.value).toFixed(2);
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
            showStatus('请选择图片文件', 'error');
        }
    }
}

async function handleImageSelect(e) {
    const file = e.target.files[0];
    if (!file) return;
    if (file.size > 10 * 1024 * 1024) {
        showStatus('文件大小不能超过10MB', 'error');
        return;
    }
    if (!file.type.startsWith('image/')) {
        showStatus('请选择图片文件', 'error');
        return;
    }

    const reader = new FileReader();
    reader.onload = (event) => {
        elements.imagePreview.src = event.target.result;
        elements.imagePreview.style.display = 'block';
        elements.clearImageBtn.style.display = 'inline-block';
    };
    reader.readAsDataURL(file);
    await uploadImage(file);
}

async function uploadImage(file) {
    try {
        showStatus('上传图片中...', 'info');
        const fixedFileName = 'uploaded_image.' + (file.name.split('.').pop() || 'png');
        const base64FileName = btoa(fixedFileName);
        const arrayBuffer = await file.arrayBuffer();

        const response = await fetch('/save', {
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
        showStatus('图片上传成功', 'success');
    } catch (error) {
        console.error('上传失败:', error);
        showStatus(`图片上传失败: ${error.message}`, 'error');
    }
}

let statusTimeout = null;

function showStatus(message, type = 'info', duration = 3000) {
    elements.statusMessage.textContent = message;
    // 使用新玻璃类名（与原JS兼容）
    elements.statusBar.className = `status-bar-glass status-${type}`;
    elements.statusBar.style.display = 'flex';

    if (statusTimeout) clearTimeout(statusTimeout);
    if (duration > 0) {
        statusTimeout = setTimeout(() => {
            elements.statusBar.style.display = 'none';
        }, duration);
    }
}

function resetParameters() {
    if (confirm('确定要重置所有参数吗？当前设置将会丢失。')) {
        elements.presetSelect.value = '';
        loadDefaultPrompts();
        elements.prompt.value = presets.anime.prompt;
        elements.negativePrompt.value = presets.anime.negative_prompt;
        elements.initImage.value = '';
        elements.imagePreview.style.display = 'none';
        elements.widthSlider.value = presets.anime.width;
        elements.heightSlider.value = presets.anime.height;
        elements.strengthSlider.value = presets.anime.strength;
        elements.steps.value = presets.anime.steps;
        elements.batchSize.value = presets.anime.batch_size;
        elements.cfgScale.value = presets.anime.cfg_scale;
        elements.seed.value = 0;

        updateWidthValue();
        updateHeightValue();
        updateStrengthValue();
        uploadedImagePath = null;
        showStatus('参数已重置', 'info');
    }
}

function refreshFileList() {
    // 可选音效
    const audio = new Audio('/read/audios/prompt-tone.mp3');
    audio.volume = 1.0;
    audio.play().catch(() => {});
    elements.refreshBtn.classList.add('spin');
    loadFileList();
    setTimeout(() => {
        elements.refreshBtn.classList.remove('spin');
    }, 100);
}

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
            init_img: uploadedImagePath || null
        };

        if (!generateData.prompt) {
            showStatus('请输入提示词', 'error');
            return;
        }
        if (generateData.batch_size < 1 || generateData.batch_size > 8) {
            showStatus('生成数量必须在1-8之间', 'error');
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
        showStatus('生成任务已提交，请等待处理', 'info', 2000);
        waitForTaskCompletion();
    } catch (error) {
        console.error('生成失败:', error);
        showStatus(`生成失败: ${error.message}`, 'error');
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
                    showStatus('图像生成完成！', 'success');
                    setTimeout(() => {
                        elements.taskStatus.style.display = 'none';
                        elements.generateBtn.disabled = false;
                    }, 2000);
                    currentTaskId = null;
                    refreshFileList();
                    eventSource.close();
                } else if (data.status === 'failed') {
                    elements.taskMessage.textContent = '生成失败';
                    showStatus(`生成失败: ${data.error}`, 'error');
                    elements.taskStatus.style.display = 'none';
                    elements.generateBtn.disabled = false;
                    currentTaskId = null;
                    eventSource.close();
                }
            } catch (error) {
                console.error('处理消息失败:', error);
                showStatus('处理消息失败，请刷新页面重试', 'error');
                elements.taskStatus.style.display = 'none';
                elements.generateBtn.disabled = false;
                currentTaskId = null;
                eventSource.close();
            }
        };

        eventSource.onerror = function (error) {
            console.error('EventSource 错误:', error);
            showStatus('连接失败，请刷新页面重试', 'error');
            elements.taskStatus.style.display = 'none';
            elements.generateBtn.disabled = false;
            currentTaskId = null;
            eventSource.close();
        };
    } catch (error) {
        console.error('创建 EventSource 失败:', error);
        showStatus('创建连接失败，请刷新页面重试', 'error');
        elements.taskStatus.style.display = 'none';
        elements.generateBtn.disabled = false;
        currentTaskId = null;
    }
}

async function loadFileList() {
    try {
        elements.fileGrid.innerHTML = '<div class="loading"><div class="spinner"></div></div>';
        let allFiles = await getAllFilesRecursive('images/generated');

        if (allFiles.length === 0) {
            elements.fileGrid.innerHTML = `
                <div class="empty-state">
                    <div class="empty-state-icon">📂</div>
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
            const fileIcon = getFileIcon(file.name);
            const sizeFormatted = formatFileSize(file.size);
            const dateFormatted = formatDate(file.lastModified);
            const path = file.path.replace(/\\/g, '/');
            const relativePath = file.path.replace(/^images\/generated[\\/]/, '');
            const displayPath = relativePath || file.name;
            const pathParts = displayPath.split(/[\\/]/);
            const previewContent = isImage
                ? `<img src="/read/${path}" alt="${file.name}" onerror="this.onerror=null; this.src='/read/images/placeholder/video_file_icon-0${Math.floor(Math.random() * 5)}.png'" onclick="previewImage('/read/${path}', '${file.name}')">`
                : `<div style="font-size: 48px; color: var(--primary-color); opacity: 0.3;">${fileIcon}</div>`;

            return `
                <div class="file-card">
                    <div class="file-card-header">
                        <div class="file-icon">${fileIcon}</div>
                        <div class="file-name" title="${displayPath}">
                            <div style="font-size: 0.9em; color: var(--text-light); margin-bottom: 2px;">${pathParts.length > 1 ? pathParts.slice(0, -1).join('/') + '/' : ''}</div>
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
                <div style="color: #dc3545; font-size: 36px;">❌</div>
                <p style="color: #dc3545;">加载文件列表失败</p>
                <p style="font-size: 0.9em; margin-top: 10px; color: #666;">${error.message}</p>
                <button onclick="loadFileList()" class="btn-glass btn-glass-secondary" style="margin-top: 15px;">重试</button>
            </div>`;
    }
}
// 递归获取 images/generated 目录下的所有文件
async function getAllFilesRecursive(dirPath) {
    try {
        const response = await fetch(`/file_list/${dirPath}`);
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
        'png': '🖼️', 'jpg': '🖼️', 'jpeg': '🖼️', 'gif': '🖼️', 'bmp': '🖼️', 'webp': '🖼️',
        'mp4': '🎬', 'webm': '🎬', 'ogg': '🎵', 'mov': '🎬', 'avi': '🎬', 'mkv': '🎬',
        'flv': '🎬', 'wmv': '🎬', 'm4v': '🎬', 'mp3': '🎵', 'wav': '🎵', 'flac': '🎵',
        'aac': '🎵', 'pdf': '📄', 'txt': '📝', 'json': '📋', 'default': '📄'
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
        showStatus('开始下载...', 'info');
        const link = document.createElement('a');
        link.href = `/download/${path}`;
        link.download = filename;
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
        showStatus('下载链接已打开', 'info');
    } catch (error) {
        console.error('下载失败:', error);
        showStatus(`下载失败: ${error.message}`, 'error');
        window.open(`/download/${path}`, '_blank');
    }
}

async function deleteFile(path) {
    if (!confirm(`确定要删除文件吗？\n${path.replace(/^images\/generated[\\/]/, '')}`)) return;
    try {
        showStatus('删除文件中...', 'info');
        const response = await fetch(`/delete/${path}`, { method: 'DELETE' });
        if (response.ok) {
            showStatus('文件删除成功', 'success');
            refreshFileList();
        } else {
            const errorText = await response.text();
            throw new Error(errorText);
        }
    } catch (error) {
        console.error('删除失败:', error);
        showStatus(`删除失败: ${error.message}`, 'error');
    }
}

async function clearAllFiles() {
    if (!confirm('⚠️ 确定要删除所有生成的文件吗？\n此操作不可恢复！')) return;
    try {
        showStatus('清空所有文件中...', 'info');
        const response = await fetch('/delete/images/generated', { method: 'DELETE' });
        if (response.ok) {
            showStatus('所有文件已清空', 'success');
            refreshFileList();
        } else {
            const errorText = await response.text();
            throw new Error(errorText);
        }
    } catch (error) {
        console.error('清空失败:', error);
        showStatus(`清空失败: ${error.message}`, 'error');
    }
}

function handleKeyboardShortcuts(e) {
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
        e.preventDefault();
        elements.generateBtn.click();
    }
    if ((e.ctrlKey || e.metaKey) && e.key === 'r' && !e.shiftKey) {
        e.preventDefault();
        refreshFileList();
    }
    if (e.key === 'Escape') {
        const modal = document.querySelector('[style*="position: fixed"]');
        if (modal) modal.remove();
    }
}

document.addEventListener('DOMContentLoaded', initEventListeners);