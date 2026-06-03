document.addEventListener('DOMContentLoaded', () => {
    initSelectZone();
});

/**
 * 初始化选择区域（仅点击触发）
 */
function initSelectZone() {
    const selectZone = document.getElementById('select-zone');

    selectZone.addEventListener('click', async () => {
        await selectAndAnalyzeFile();
    });
}

/**
 * 打开系统文件对话框选择文件，然后分析元数据
 */
async function selectAndAnalyzeFile() {
    hideError();
    hideResult();

    // 第一步：打开系统文件对话框获取文件路径
    showLoading(true, '正在打开文件选择对话框...');

    try {
        const dialogResp = await fetch('/api/open-file-dialog', {
            method: 'POST'
        });
        const dialogData = await dialogResp.json();

        if (dialogData.cancelled) {
            showLoading(false);
            return;
        }

        if (!dialogData.success) {
            showError(dialogData.error || '文件选择失败');
            showLoading(false);
            return;
        }

        const filePath = dialogData.filePath;
        const fileName = dialogData.fileName;

        // 第二步：将路径发给后端，后端直接读取本地文件
        showLoading(true, `正在解析: ${fileName}...`);

        const analyzeResp = await fetch('/api/analyze-path', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ filePath: filePath })
        });

        const data = await analyzeResp.json();

        if (!data.success) {
            showError(data.error || '解析失败');
            showLoading(false);
            return;
        }

        // 显示结果
        displayResult(data);
    } catch (err) {
        console.error('操作失败:', err);
        showError('操作失败，请重试。' + (err.message || ''));
    } finally {
        showLoading(false);
    }
}

/**
 * 显示/隐藏加载状态
 */
function showLoading(show, text) {
    const el = document.getElementById('loading');
    if (show) {
        if (text) {
            document.getElementById('loading-text').textContent = text;
        }
        el.style.display = 'block';
    } else {
        el.style.display = 'none';
    }
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

    // 显示文件路径
    document.getElementById('file-path').textContent = data.filePath || '';

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

    const labelMap = {
        'Model Name': '模型名称',
        'Architecture': '架构',
        'Quantization': '量化方式',
        'Quant Version': '量化版本',
        'Context Length': '上下文长度',
        'Embedding Dim': '嵌入维度',
        'Block Count': '层数',
        'Attention Heads': '注意力头数',
        'KV Heads': 'KV头数',
        'FFN Dim': 'FFN维度',
        'Vocab Size': '词表大小'
    };

    const keyOrder = [
        'Model Name', 'Architecture', 'Quantization', 'Quant Version',
        'Context Length', 'Embedding Dim', 'Block Count',
        'Attention Heads', 'KV Heads', 'FFN Dim', 'Vocab Size'
    ];

    const orderedKeys = keyOrder.filter(k => summary[k]);
    for (const key of Object.keys(summary)) {
        if (!orderedKeys.includes(key)) {
            orderedKeys.push(key);
        }
    }

    for (const key of orderedKeys) {
        const value = summary[key];
        if (value === undefined || value === '') continue;

        const label = labelMap[key] || key;

        const card = document.createElement('div');
        card.className = 'summary-card';
        card.innerHTML = `
            <div class="card-label">${label}</div>
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
 * 初始化搜索过滤
 */
function initFilter() {
    let filterBar = document.querySelector('.filter-bar');
    if (filterBar) {
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