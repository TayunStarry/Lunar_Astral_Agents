/**
 * ZIP 压缩包预览 / 解压模块
 * 支持查看压缩包摘要与条目列表，并执行解压
 */

/**
 * 格式化字节大小
 * @param {number} bytes - 字节数
 * @returns {string} 人类可读大小
 */
function zipFormatSize(bytes) {
    if (!bytes && bytes !== 0) return '-';
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
    if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(2) + ' MB';
    return (bytes / (1024 * 1024 * 1024)).toFixed(2) + ' GB';
}

/**
 * 展示 ZIP 压缩包预览模态框
 * @param {Object} file - ZIP 文件对象
 * @param {Object} fileManager - 文件管理器实例
 */
async function showZipModal(file, fileManager) {
    const modal = document.getElementById('zip-modal');
    const filePathEl = document.getElementById('zip-file-path');
    const summaryEl = document.getElementById('zip-summary-cards');
    const entryBody = document.getElementById('zip-entry-body');
    const entryCount = document.getElementById('zip-entry-count');
    const resultEl = document.getElementById('zip-extract-result');
    const pathInput = document.getElementById('zip-extract-path');
    const hintEl = document.getElementById('zip-extract-hint');

    filePathEl.textContent = file.path || file.name;
    summaryEl.innerHTML = '<div class="zip-loading"><i class="fas fa-spinner fa-spin"></i> 正在读取压缩包信息...</div>';
    entryBody.innerHTML = '';
    entryCount.textContent = '';
    resultEl.innerHTML = '';
    hintEl.innerHTML = '';

    // 默认解压路径：当前路径/压缩包名/
    const baseName = (file.name || '').replace(/\.zip$/i, '');
    const dirParts = [];
    if (fileManager.currentPath) dirParts.push(fileManager.currentPath);
    dirParts.push(baseName);
    pathInput.value = dirParts.join('/') + '/';

    modal.classList.add('show');

    try {
        const response = await fetch('/file/archive/metadata', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ path: file.path })
        });
        if (!response.ok) throw new Error('读取失败');
        const data = await response.json();
        if (!data.success) throw new Error(data.error || '读取失败');

        // 摘要卡片
        const dirCount = data.entries.filter(e => e.isDir).length;
        const fileCount = data.file_count - dirCount;
        const cards = [
            { label: '压缩包大小', value: zipFormatSize(data.zip_size) },
            { label: '解压后大小', value: zipFormatSize(data.total_size) },
            { label: '文件数', value: fileCount },
            { label: '文件夹数', value: dirCount }
        ];
        summaryEl.innerHTML = cards.map(c =>
            `<div class="zip-summary-card"><div class="zip-summary-label">${c.label}</div><div class="zip-summary-value">${c.value}</div></div>`
        ).join('');

        entryCount.textContent = `（${data.file_count} 个条目）`;

        // 条目列表
        if (data.entries.length === 0) {
            entryBody.innerHTML = '<tr><td colspan="4" class="zip-empty">压缩包为空</td></tr>';
        } else {
            entryBody.innerHTML = data.entries.map(entry => {
                const icon = entry.isDir
                    ? '<i class="fas fa-folder" style="color: var(--accent);"></i>'
                    : '<i class="fas fa-file" style="color: var(--text-muted);"></i>';
                const type = entry.isDir ? '文件夹' : '文件';
                return `<tr>
                    <td class="zip-entry-name">${icon} <span>${mediaEscapeHTML(entry.name)}</span></td>
                    <td>${type}</td>
                    <td>${zipFormatSize(entry.size)}</td>
                    <td>${zipFormatSize(entry.compressed)}</td>
                </tr>`;
            }).join('');
        }

        hintEl.innerHTML = `<i class="fas fa-info-circle"></i> 将解压到：${mediaEscapeHTML(pathInput.value)}`;
    } catch (error) {
        summaryEl.innerHTML = `<div class="zip-error"><i class="fas fa-exclamation-triangle"></i> ${mediaEscapeHTML(error.message || '读取压缩包信息失败')}</div>`;
        console.error('读取 ZIP 元数据失败:', error);
    }
}

/**
 * 关闭 ZIP 预览模态框
 */
function closeZipModal() {
    document.getElementById('zip-modal').classList.remove('show');
    document.getElementById('zip-extract-result').innerHTML = '';
}

/**
 * 执行 ZIP 解压
 * @param {Object} fileManager - 文件管理器实例
 */
async function startZipExtract(fileManager) {
    const resultEl = document.getElementById('zip-extract-result');
    const filePath = document.getElementById('zip-file-path').textContent;
    const targetDir = document.getElementById('zip-extract-path').value.trim();

    if (!targetDir) {
        resultEl.innerHTML = '<div class="zip-error"><i class="fas fa-exclamation-triangle"></i> 请输入解压路径</div>';
        return;
    }

    resultEl.innerHTML = '<div class="zip-progress"><i class="fas fa-spinner fa-spin"></i> 正在解压...</div>';

    try {
        const response = await fetch('/file/archive/extract', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ path: filePath, target_dir: targetDir })
        });
        if (!response.ok) throw new Error('解压失败');
        const data = await response.json();
        if (!data.success) throw new Error(data.error || '解压失败');

        resultEl.innerHTML = `<div class="zip-success"><i class="fas fa-check-circle"></i> 已解压 ${data.file_count} 个文件到 ${mediaEscapeHTML(data.target_dir)}</div>`;
        showToast(`解压完成：${data.file_count} 个文件`, 'success');

        // 解压完成后刷新文件列表并关闭模态框
        setTimeout(async () => {
            await fileManager.loadFiles();
            fileManager.updateStats();
            closeZipModal();
        }, 800);
    } catch (error) {
        resultEl.innerHTML = `<div class="zip-error"><i class="fas fa-exclamation-triangle"></i> ${mediaEscapeHTML(error.message || '解压失败')}</div>`;
        console.error('解压失败:', error);
    }
}
