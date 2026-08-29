/**
 * 图片转码模块
 * 参考 lunar.means.image.converter 扩展包实现，支持批量转换当前层级与转换选中图片
 */

/** 图片转码状态 */
const convertState = {
    mode: 'batch',       // batch 批量 / selected 选中
    sourceFormat: 'all', // all / png / jpeg / webp
    targetFormat: 'jpeg',
    deleteSource: true,  // 默认转换后删除源文件
    quality: 90
};

/**
 * 打开图片转码模态框
 * @param {FileManager} fileManager - 文件管理器实例
 */
function openConvertModal(fileManager) {
    // 统计当前层级可转码图片与选中图片
    const folderImages = fileManager.files.filter(f => !f.isDir && isConvertibleImage(f.name));
    const selectedImages = Array.from(fileManager.selectedFiles)
        .map(path => fileManager.files.find(f => f.path === path))
        .filter(f => f && !f.isDir && isConvertibleImage(f.name));

    // 有选中的图片时默认「转换选中图片」，否则默认「批量转换」
    convertState.mode = selectedImages.length > 0 ? 'selected' : 'batch';
    convertState.sourceFormat = 'all';
    convertState.targetFormat = 'jpeg';

    updateConvertModeTabs();
    updateConvertModeHint(folderImages, selectedImages);
    setConvertTabActive('convert-source-tabs', 'all');
    setConvertTabActive('convert-target-tabs', 'jpeg');

    const resultEl = document.getElementById('convert-result');
    resultEl.style.display = 'none';
    resultEl.innerHTML = '';

    document.getElementById('convert-modal').classList.add('show');
}

/**
 * 关闭图片转码模态框
 */
function closeConvertModal() {
    document.getElementById('convert-modal').classList.remove('show');
}

/**
 * 更新转换模式选项卡高亮
 */
function updateConvertModeTabs() {
    document.querySelectorAll('#convert-mode-tabs .convert-mode-tab').forEach(tab => {
        tab.classList.toggle('active', tab.dataset.mode === convertState.mode);
    });
}

/**
 * 更新模式提示文案
 * @param {Array<Object>} folderImages - 当前层级可转码图片列表
 * @param {Array<Object>} selectedImages - 选中的可转码图片列表
 */
function updateConvertModeHint(folderImages, selectedImages) {
    const hintEl = document.getElementById('convert-mode-hint');
    const sourceSection = document.getElementById('convert-source-section');
    if (convertState.mode === 'selected') {
        if (selectedImages.length > 0) {
            hintEl.innerHTML = `<i class="fas fa-check"></i> 将转换选中的 ${selectedImages.length} 张图片`;
        } else {
            hintEl.innerHTML = '<i class="fas fa-info-circle"></i> 未勾选图片文件，可先勾选文件卡片右上角的复选框，即可只转换指定的图片';
        }
        sourceSection.style.display = 'none';
    } else {
        const total = folderImages.length;
        hintEl.innerHTML = `<i class="fas fa-info-circle"></i> 将批量转换当前层级全部 ${total} 张可转码图片（也可以先勾选部分图片，只转换所选）`;
        sourceSection.style.display = '';
    }
}

/**
 * 切换格式选项卡高亮
 * @param {string} containerId - 选项卡容器 ID
 * @param {string} format - 选中格式
 */
function setConvertTabActive(containerId, format) {
    document.querySelectorAll(`#${containerId} .convert-tab`).forEach(tab => {
        tab.classList.toggle('active', tab.dataset.format === format);
    });
}

/**
 * 开始执行图片转码
 * @param {FileManager} fileManager - 文件管理器实例
 */
async function startImageConvert(fileManager) {
    const targetFormat = convertState.targetFormat;
    const deleteSource = document.getElementById('convert-delete-source').checked;
    const quality = parseInt(document.getElementById('convert-quality').value, 10) || 90;
    const resultEl = document.getElementById('convert-result');

    const folderImages = fileManager.files.filter(f => !f.isDir && isConvertibleImage(f.name));
    const selectedImages = Array.from(fileManager.selectedFiles)
        .map(path => fileManager.files.find(f => f.path === path))
        .filter(f => f && !f.isDir && isConvertibleImage(f.name));

    // 批量模式需要源格式；选中模式逐张转换无需源格式过滤
    const sourceFormat = convertState.mode === 'batch' ? convertState.sourceFormat : 'all';

    resultEl.style.display = 'block';
    resultEl.innerHTML = '<div class="convert-progress"><i class="fas fa-spinner fa-spin"></i> 正在转换，请稍候...</div>';

    try {
        if (convertState.mode === 'batch') {
            await batchImageConvert(fileManager, folderImages, sourceFormat, targetFormat, deleteSource, quality, resultEl);
        } else {
            if (selectedImages.length === 0) {
                resultEl.innerHTML = '<div class="convert-error"><i class="fas fa-info-circle"></i> 未勾选任何图片文件，请先勾选要转换的图片</div>';
                return;
            }
            await selectedImageConvert(selectedImages, targetFormat, deleteSource, quality, resultEl);
        }

        await fileManager.loadFiles();
        fileManager.updateStats();
    } catch (err) {
        resultEl.innerHTML = `<div class="convert-error"><i class="fas fa-exclamation-triangle"></i> 转换流程失败: ${mediaEscapeHTML(err.message || '未知错误')}</div>`;
    }
}

/**
 * 批量转换：POST /convert/batch
 * @param {FileManager} fileManager - 文件管理器实例
 */
async function batchImageConvert(fileManager, folderImages, sourceFormat, targetFormat, deleteSource, quality, resultEl) {
    const response = await fetch('/convert/batch', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            folder: fileManager.currentPath || '.',
            source_format: sourceFormat,
            target_format: targetFormat,
            delete_source: deleteSource,
            quality: quality
        })
    });
    if (!response.ok) throw new Error('批量转换请求失败');
    const data = await response.json();
    if (!data.success) throw new Error(data.error || '批量转换失败');

    let html = `<div class="convert-summary"><i class="fas fa-check-circle"></i> 批量转换完成：成功 ${data.success_count}，失败 ${data.fail_count}，共 ${data.total} 项</div>`;
    const failed = (data.results || []).filter(r => !r.success);
    if (failed.length > 0) {
        html += '<div class="convert-fail-list">';
        failed.forEach(r => {
            html += `<div class="convert-fail-item"><i class="fas fa-times-circle"></i> ${mediaEscapeHTML(r.path.split(/[\\/]/).pop())} — ${mediaEscapeHTML(r.error || '未知错误')}</div>`;
        });
        html += '</div>';
    }
    resultEl.innerHTML = html;
}

/**
 * 逐个转换选中的图片：POST /convert/image
 * @param {Array<Object>} selectedImages - 选中的可转码图片
 */
async function selectedImageConvert(selectedImages, targetFormat, deleteSource, quality, resultEl) {
    const results = [];
    const targetExt = '.' + (targetFormat === 'jpeg' ? 'jpg' : targetFormat);

    for (let i = 0; i < selectedImages.length; i++) {
        const file = selectedImages[i];
        const ext = file.name.toLowerCase().slice(file.name.lastIndexOf('.'));
        resultEl.innerHTML = `<div class="convert-progress"><i class="fas fa-spinner fa-spin"></i> 正在转换 (${i + 1}/${selectedImages.length}): ${mediaEscapeHTML(file.name)}</div>`;

        // 源格式与目标格式相同则跳过
        if (ext === targetExt) {
            results.push({ name: file.name, success: false, error: '源格式与目标格式相同，无需转换' });
            continue;
        }

        try {
            const response = await fetch('/convert/image', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    path: file.path,
                    target_format: targetFormat,
                    delete_source: deleteSource,
                    quality: quality
                })
            });
            const data = await response.json();
            if (!data.success) {
                results.push({ name: file.name, success: false, error: data.error || '转换失败' });
            } else {
                results.push({ name: file.name, success: true });
            }
        } catch (err) {
            results.push({ name: file.name, success: false, error: err.message || '网络请求失败' });
        }
    }

    const successCount = results.filter(r => r.success).length;
    let html = `<div class="convert-summary"><i class="fas fa-check-circle"></i> 选中图片转换完成：成功 ${successCount}，失败 ${results.length - successCount}</div>`;
    const failed = results.filter(r => !r.success);
    if (failed.length > 0) {
        html += '<div class="convert-fail-list">';
        failed.forEach(r => {
            html += `<div class="convert-fail-item"><i class="fas fa-times-circle"></i> ${mediaEscapeHTML(r.name)} — ${mediaEscapeHTML(r.error || '未知错误')}</div>`;
        });
        html += '</div>';
    }
    resultEl.innerHTML = html;
}
