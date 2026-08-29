/**
 * GGUF 模型预览模块
 * 参考 lunar.means.gguf.viewer 扩展包实现，展示 GGUF 模型元数据
 */

/** GGUF 摘要字段中文映射 */
const GGUF_LABEL_MAP = {
    'Model Name': '模型名称',
    'Architecture': '架构',
    'Quantization': '量化方式',
    'Quant Version': '量化版本',
    'Context Length': '上下文长度',
    'Embedding Dim': '嵌入维度',
    'Block Count': '层数',
    'Attention Heads': '注意力头数',
    'KV Heads': 'KV 头数',
    'FFN Dim': 'FFN 维度',
    'Vocab Size': '词表大小'
};

/** GGUF 摘要字段展示顺序 */
const GGUF_KEY_ORDER = [
    'Model Name', 'Architecture', 'Quantization', 'Quant Version',
    'Context Length', 'Embedding Dim', 'Block Count',
    'Attention Heads', 'KV Heads', 'FFN Dim', 'Vocab Size'
];

/**
 * 展示 GGUF 模型预览模态框
 * @param {Object} file - 文件对象（path 为相对 LocalDir 的路径）
 */
async function showGGUFModal(file) {
    const modal = document.getElementById('gguf-modal');
    const pathEl = document.getElementById('gguf-file-path');
    const cardsEl = document.getElementById('gguf-summary-cards');
    const bodyEl = document.getElementById('gguf-metadata-body');

    document.getElementById('gguf-search-input').value = '';
    document.getElementById('gguf-metadata-count').textContent = '';
    pathEl.textContent = file.path;
    cardsEl.innerHTML = '<div class="gguf-loading"><i class="fas fa-spinner fa-spin"></i> 正在解析模型元数据...</div>';
    bodyEl.innerHTML = '';
    modal.classList.add('show');

    try {
        const response = await fetch('/gguf/metadata', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ filePath: file.path })
        });
        const data = await response.json();

        if (!data.success) {
            cardsEl.innerHTML = `<div class="gguf-error"><i class="fas fa-exclamation-triangle"></i> ${mediaEscapeHTML(data.error || '解析失败，请检查文件格式')}</div>`;
            return;
        }

        pathEl.textContent = data.filePath || file.path;
        renderGGUFSummary(cardsEl, data.summary);
        document.getElementById('gguf-metadata-count').textContent = `（${data.count} 项）`;
        renderGGUFMetadataTable(bodyEl, data.metadata);
    } catch (err) {
        cardsEl.innerHTML = '<div class="gguf-error"><i class="fas fa-exclamation-triangle"></i> 网络请求失败，请检查服务是否正常运行</div>';
    }
}

/**
 * 关闭 GGUF 预览模态框
 */
function closeGGUFModal() {
    document.getElementById('gguf-modal').classList.remove('show');
}

/**
 * 渲染 GGUF 摘要卡片
 * @param {HTMLElement} container - 卡片容器
 * @param {Object} summary - 摘要字段映射
 */
function renderGGUFSummary(container, summary) {
    container.innerHTML = '';
    if (!summary) {
        container.innerHTML = '<div class="gguf-error"><i class="fas fa-inbox"></i> 无摘要信息</div>';
        return;
    }

    const orderedKeys = GGUF_KEY_ORDER.filter(k => summary[k]);
    for (const key of Object.keys(summary)) {
        if (!orderedKeys.includes(key)) orderedKeys.push(key);
    }

    for (const key of orderedKeys) {
        const value = summary[key];
        if (value === undefined || value === '') continue;
        const label = GGUF_LABEL_MAP[key] || key;
        const card = document.createElement('div');
        card.className = 'gguf-summary-card';
        card.innerHTML = `
            <div class="gguf-summary-label">${mediaEscapeHTML(label)}</div>
            <div class="gguf-summary-value">${mediaEscapeHTML(String(value))}</div>
        `;
        container.appendChild(card);
    }
}

/**
 * 渲染 GGUF 元数据表格
 * @param {HTMLElement} tbody - 表格主体
 * @param {Object} metadata - 元数据键值映射
 */
function renderGGUFMetadataTable(tbody, metadata) {
    tbody.innerHTML = '';
    const keys = Object.keys(metadata || {}).sort();
    if (keys.length === 0) {
        tbody.innerHTML = '<tr><td colspan="2" class="gguf-empty"><i class="fas fa-inbox"></i> 暂无元数据</td></tr>';
        return;
    }
    for (const key of keys) {
        const value = metadata[key];
        const tr = document.createElement('tr');
        tr.setAttribute('data-key', key.toLowerCase());
        tr.setAttribute('data-value', String(value).toLowerCase());
        const tdKey = document.createElement('td');
        tdKey.textContent = key;
        const tdValue = document.createElement('td');
        tdValue.textContent = value;
        const numValue = Number(value);
        if (!isNaN(numValue) && String(value).trim() !== '') {
            tdValue.className = 'gguf-value-number';
        } else if (value === 'true' || value === 'false') {
            tdValue.className = 'gguf-value-bool';
        } else {
            tdValue.className = 'gguf-value-string';
        }
        tr.appendChild(tdKey);
        tr.appendChild(tdValue);
        tbody.appendChild(tr);
    }
}
