document.addEventListener('DOMContentLoaded', () => {
    initDragDrop();
    initFileInput();
});

/**
 * 初始化拖放功能
 */
function initDragDrop() {
    const dropZone = document.getElementById('drop-zone');

    // 防止浏览器默认行为（打开文件）
    ['dragenter', 'dragover', 'dragleave', 'drop'].forEach(eventName => {
        dropZone.addEventListener(eventName, (e) => {
            e.preventDefault();
            e.stopPropagation();
        });
        document.body.addEventListener(eventName, (e) => {
            e.preventDefault();
            e.stopPropagation();
        });
    });

    // 拖入高亮
    ['dragenter', 'dragover'].forEach(eventName => {
        dropZone.addEventListener(eventName, () => {
            dropZone.classList.add('drag-over');
        });
    });

    // 离开取消高亮
    ['dragleave', 'drop'].forEach(eventName => {
        dropZone.addEventListener(eventName, () => {
            dropZone.classList.remove('drag-over');
        });
    });

    // 处理文件投放
    dropZone.addEventListener('drop', (e) => {
        const files = e.dataTransfer.files;
        if (files.length > 0) {
            handleFile(files[0]);
        }
    });

    // 点击拖放区域也可选择文件
    dropZone.addEventListener('click', (e) => {
        // 如果点击的是按钮标签，不重复触发
        if (e.target.tagName === 'LABEL' || e.target.tagName === 'INPUT') {
            return;
        }
        document.getElementById('file-input').click();
    });
}

/**
 * 初始化文件选择按钮
 */
function initFileInput() {
    const fileInput = document.getElementById('file-input');
    fileInput.addEventListener('change', (e) => {
        const files = e.target.files;
        if (files.length > 0) {
            handleFile(files[0]);
        }
        // 重置输入以便可以重新选择同一个文件
        fileInput.value = '';
    });
}

/**
 * 处理选中的文件：上传并解析元数据
 */
async function handleFile(file) {
    // 验证文件扩展名
    if (!file.name.toLowerCase().endsWith('.gguf')) {
        showError('文件格式不支持，请选择 .gguf 格式的模型文件。');
        return;
    }

    // 显示加载状态
    showLoading(true);
    hideError();
    hideResult();

    try {
        const formData = new FormData();
        formData.append('file', file);

        const response = await fetch('/api/upload', {
            method: 'POST',
            body: formData
        });

        const data = await response.json();

        if (!data.success) {
            showError(data.error || '未知解析错误');
            return;
        }

        // 显示结果
        displayResult(data);
    } catch (err) {
        console.error('上传失败:', err);
        showError('文件上传或解析失败，请重试。' + (err.message || ''));
    } finally {
        showLoading(false);
    }
}

/**
 * 显示/隐藏加载状态
 */
function showLoading(show) {
    document.getElementById('loading').style.display = show ? 'block' : 'none';
}

/**
 * 显示错误信息
 */
function showError(message) {
    const errorEl = document.getElementById('error-msg');
    errorEl.textContent = '错误: ' + message;
    errorEl.style.display = 'block';
}

/**
 * 隐藏错误信息
 */
function hideError() {
    document.getElementById('error-msg').style.display = 'none';
}

/**
 * 隐藏结果区域
 */
function hideResult() {
    document.getElementById('result').style.display = 'none';
}

/**
 * 显示解析结果
 */
function displayResult(data) {
    const resultEl = document.getElementById('result');
    resultEl.style.display = 'block';

    // 渲染摘要卡片
    renderSummary(data.summary);

    // 渲染元数据表
    document.getElementById('metadata-count').textContent = data.count;
    renderMetadataTable(data.metadata);
    initFilter();

    // 滚动到结果区域
    resultEl.scrollIntoView({ behavior: 'smooth', block: 'start' });
}

/**
 * 渲染摘要卡片
 */
function renderSummary(summary) {
    const container = document.getElementById('summary-cards');
    container.innerHTML = '';

    // 按重要性排序的键
    const keyOrder = [
        '模型名称', '架构', '量化方式', '量化版本',
        '上下文长度', '嵌入维度', '层数',
        '注意力头数', 'KV头数', 'FFN维度', '词表大小'
    ];

    const orderedKeys = keyOrder.filter(k => summary[k]);
    // 添加不在预设顺序中的键
    for (const key of Object.keys(summary)) {
        if (!orderedKeys.includes(key)) {
            orderedKeys.push(key);
        }
    }

    for (const key of orderedKeys) {
        const value = summary[key];
        if (value === undefined || value === '') continue;

        const card = document.createElement('div');
        card.className = 'summary-card';
        card.innerHTML = `
            <div class="card-label">${key}</div>
            <div class="card-value">${escapeHTML(String(value))}</div>
        `;
        container.appendChild(card);
    }
}

/**
 * 渲染元数据表格
 */
function renderMetadataTable(metadata) {
    const tbody = document.getElementById('metadata-body');
    tbody.innerHTML = '';

    // 按键名字母排序
    const keys = Object.keys(metadata).sort();

    for (const key of keys) {
        const value = metadata[key];
        const tr = document.createElement('tr');
        tr.setAttribute('data-key', key.toLowerCase());
        tr.setAttribute('data-value', String(value).toLowerCase());

        const tdKey = document.createElement('td');
        tdKey.textContent = key;

        const tdValue = document.createElement('td');
        tdValue.textContent = value;

        // 根据值类型添加样式
        const numValue = Number(value);
        if (!isNaN(numValue) && value.trim() !== '') {
            tdValue.className = 'value-number';
        } else if (value === 'true' || value === 'false') {
            tdValue.className = 'value-bool';
        } else {
            tdValue.className = 'value-string';
        }

        tr.appendChild(tdKey);
        tr.appendChild(tdValue);
        tbody.appendChild(tr);
    }
}

/**
 * 初始化搜索过滤功能
 */
function initFilter() {
    // 检查是否已有搜索栏
    let filterBar = document.querySelector('.filter-bar');
    if (filterBar) {
        // 清空已有输入
        const input = filterBar.querySelector('.filter-input');
        if (input) input.value = '';
        return;
    }

    const metadataSection = document.querySelector('.metadata-section');

    filterBar = document.createElement('div');
    filterBar.className = 'filter-bar';
    filterBar.innerHTML = `
        <input type="text" class="filter-input" placeholder="输入关键字过滤元数据...">
        <span class="filter-count" id="filter-count"></span>
    `;

    // 插入到h2之后、table-wrapper之前
    const tableWrapper = metadataSection.querySelector('.table-wrapper');
    metadataSection.insertBefore(filterBar, tableWrapper);

    const filterInput = filterBar.querySelector('.filter-input');
    const filterCount = document.getElementById('filter-count');

    filterInput.addEventListener('input', () => {
        const query = filterInput.value.toLowerCase().trim();
        const rows = document.querySelectorAll('#metadata-body tr');
        let visibleCount = 0;

        rows.forEach(row => {
            const key = row.getAttribute('data-key') || '';
            const value = row.getAttribute('data-value') || '';
            if (query === '' || key.includes(query) || value.includes(query)) {
                row.style.display = '';
                visibleCount++;
            } else {
                row.style.display = 'none';
            }
        });

        filterCount.textContent = query ? `显示 ${visibleCount}/${rows.length}` : '';
    });
}

/**
 * HTML转义
 */
function escapeHTML(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}