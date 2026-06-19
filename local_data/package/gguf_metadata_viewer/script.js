// ===== 全局 DOM 引用 =====
const elements = {
    modelPath: document.getElementById('modelPath'),
    analyzeBtn: document.getElementById('analyzeBtn'),
    validationFeedback: document.getElementById('validationFeedback'),
    loadingPanel: document.getElementById('loadingPanel'),
    loadingText: document.getElementById('loadingText'),
    errorPanel: document.getElementById('errorPanel'),
    resultPanel: document.getElementById('resultPanel'),
    displayFilePath: document.getElementById('displayFilePath'),
    summaryCards: document.getElementById('summaryCards'),
    metadataCount: document.getElementById('metadataCount'),
    metadataBody: document.getElementById('metadataBody'),
    searchBar: document.getElementById('searchBar'),
    searchInput: document.getElementById('searchInput'),
    clearSearchBtn: document.getElementById('clearSearchBtn'),
    searchCount: document.getElementById('searchCount'),
    themeToggle: document.getElementById('themeToggle'),
    themeIcon: document.getElementById('themeToggle').querySelector('i'),
    themeLabel: document.getElementById('themeLabel'),
};

// 缓存解析后的元数据用于搜索
let cachedMetadata = null;

// ===== 初始化 =====
document.addEventListener('DOMContentLoaded', () => {
    initTheme();
    bindEvents();
});

function bindEvents() {
    // 主题切换
    elements.themeToggle.addEventListener('click', toggleTheme);

    // 解析按钮点击
    elements.analyzeBtn.addEventListener('click', handleAnalyze);

    // 回车键触发解析
    elements.modelPath.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') {
            e.preventDefault();
            handleAnalyze();
        }
    });

    // 路径输入时实时校验
    elements.modelPath.addEventListener('input', handlePathValidation);

    // 搜索输入
    elements.searchInput.addEventListener('input', handleSearch);
    elements.clearSearchBtn.addEventListener('click', clearSearch);
    elements.searchInput.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') {
            clearSearch();
        }
    });
}

// ===== 主题切换 =====

const THEME_KEY = 'gguf-viewer-theme';

function initTheme() {
    const saved = localStorage.getItem(THEME_KEY);
    if (saved === 'dark') {
        document.body.classList.add('dark-mode');
        updateThemeUI(true);
    } else {
        document.body.classList.remove('dark-mode');
        updateThemeUI(false);
    }
}

function toggleTheme() {
    const isDark = document.body.classList.toggle('dark-mode');
    localStorage.setItem(THEME_KEY, isDark ? 'dark' : 'light');
    updateThemeUI(isDark);
}

function updateThemeUI(isDark) {
    if (isDark) {
        elements.themeIcon.className = 'fas fa-sun';
        elements.themeLabel.textContent = '浅色模式';
    } else {
        elements.themeIcon.className = 'fas fa-moon';
        elements.themeLabel.textContent = '深色模式';
    }
}

// ===== 功能1：模型路径输入 + 功能2：文件类型校验 =====

/**
 * 实时校验输入路径
 */
function handlePathValidation() {
    const path = elements.modelPath.value.trim();
    const feedback = elements.validationFeedback;

    if (!path) {
        feedback.style.display = 'none';
        return;
    }

    const ext = path.substring(path.lastIndexOf('.')).toLowerCase();

    if (ext === '.gguf') {
        feedback.style.display = 'block';
        feedback.className = 'feedback-area success';
        feedback.innerHTML = '<i class="fas fa-check-circle"></i> 有效的 GGUF 文件路径';
    } else if (ext && ['.gguf'].indexOf(ext) === -1) {
        feedback.style.display = 'block';
        feedback.className = 'feedback-area error';
        feedback.innerHTML = `<i class="fas fa-exclamation-circle"></i> 文件扩展名 "${ext}" 无效，仅支持 .gguf 格式`;
    } else {
        feedback.style.display = 'none';
    }
}

/**
 * 处理解析请求
 */
async function handleAnalyze() {
    const path = elements.modelPath.value.trim();

    // 先隐藏之前的错误和结果
    hideError();
    hideResult();

    // 空值校验
    if (!path) {
        showToast('请输入模型文件路径', 'error');
        showError('请输入 GGUF 模型文件的完整路径');
        return;
    }

    // 文件类型校验
    const ext = path.substring(path.lastIndexOf('.')).toLowerCase();
    if (ext !== '.gguf') {
        showToast('仅支持 .gguf 格式的模型文件', 'error');
        showError(`文件扩展名 "${ext}" 无效，仅支持 .gguf 格式的模型文件`);
        return;
    }

    // 显示加载状态
    showLoading(true, `正在解析: ${getFileName(path)}...`);
    elements.validationFeedback.style.display = 'none';

    try {
        const response = await fetch('/gguf/metadata', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ filePath: path })
        });

        const data = await response.json();

        if (!data.success) {
            const errorMsg = data.error || '解析失败，请检查文件路径和格式';
            showToast(errorMsg, 'error');
            showError(errorMsg);
            showLoading(false);
            return;
        }

        // 解析成功
        showToast(`解析成功，共 ${data.count} 项元数据`, 'success');
        displayResult(data);
    } catch (err) {
        const errorMsg = '网络请求失败，请检查服务是否正常运行';
        showToast(errorMsg, 'error');
        showError(errorMsg + (err.message ? ` (${err.message})` : ''));
    } finally {
        showLoading(false);
    }
}

// ===== 功能4：关键词查询 =====

/**
 * 处理搜索过滤
 */
function handleSearch() {
    const query = elements.searchInput.value.trim().toLowerCase();
    const rows = elements.metadataBody.querySelectorAll('tr[data-key]');
    let visibleCount = 0;

    if (!query) {
        // 显示所有行
        rows.forEach(row => {
            row.style.display = '';
            visibleCount++;
        });
        elements.clearSearchBtn.style.display = 'none';
        elements.searchCount.textContent = '';
    } else {
        rows.forEach(row => {
            const key = row.getAttribute('data-key') || '';
            const value = row.getAttribute('data-value') || '';
            if (key.includes(query) || value.includes(query)) {
                row.style.display = '';
                visibleCount++;
            } else {
                row.style.display = 'none';
            }
        });
        elements.clearSearchBtn.style.display = 'inline-flex';
        elements.searchCount.textContent = `显示 ${visibleCount}/${rows.length} 项`;
    }

    // 处理无结果状态
    const noResultRow = elements.metadataBody.querySelector('.no-result-row');
    if (visibleCount === 0 && query) {
        if (!noResultRow) {
            const tr = document.createElement('tr');
            tr.className = 'no-result-row';
            tr.innerHTML = `<td colspan="2" class="no-result">
                <i class="fas fa-search"></i> 未找到匹配 "${escapeHTML(query)}" 的元数据项
            </td>`;
            elements.metadataBody.appendChild(tr);
        }
    } else if (noResultRow) {
        noResultRow.remove();
    }
}

/**
 * 清除搜索
 */
function clearSearch() {
    elements.searchInput.value = '';
    handleSearch();
    elements.searchInput.focus();
}

// ===== 结果渲染 =====

/**
 * 显示解析结果
 */
function displayResult(data) {
    const resultPanel = elements.resultPanel;
    resultPanel.style.display = 'block';

    // 显示文件路径
    elements.displayFilePath.textContent = data.filePath || '';

    // 渲染摘要卡片
    renderSummary(data.summary);

    // 渲染元数据表格
    elements.metadataCount.textContent = `${data.count} 项`;
    renderMetadataTable(data.metadata);

    // 缓存元数据
    cachedMetadata = data.metadata;

    // 显示搜索栏
    elements.searchBar.style.display = 'block';
    elements.searchInput.value = '';
    elements.clearSearchBtn.style.display = 'none';
    elements.searchCount.textContent = '';

    // 滚动到结果区域
    resultPanel.scrollIntoView({ behavior: 'smooth', block: 'start' });
}

/**
 * 渲染摘要卡片
 */
function renderSummary(summary) {
    const container = elements.summaryCards;
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
        'KV Heads': 'KV 头数',
        'FFN Dim': 'FFN 维度',
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
            <div class="card-label">${escapeHTML(label)}</div>
            <div class="card-value">${escapeHTML(String(value))}</div>
        `;
        container.appendChild(card);
    }
}

/**
 * 渲染元数据表格
 */
function renderMetadataTable(metadata) {
    const tbody = elements.metadataBody;
    tbody.innerHTML = '';

    const keys = Object.keys(metadata).sort();

    if (keys.length === 0) {
        tbody.innerHTML = `
            <tr class="empty-row">
                <td colspan="2">
                    <div class="empty-state">
                        <i class="fas fa-inbox"></i>
                        <p>暂无元数据</p>
                    </div>
                </td>
            </tr>`;
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

        // 值类型着色
        const numValue = Number(value);
        if (!isNaN(numValue) && String(value).trim() !== '') {
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

// ===== UI 辅助函数 =====

function showLoading(show, text) {
    if (show) {
        elements.loadingPanel.style.display = 'block';
        if (text) {
            elements.loadingText.textContent = text;
        }
    } else {
        elements.loadingPanel.style.display = 'none';
    }
}

function showError(message) {
    const el = elements.errorPanel;
    el.innerHTML = `<i class="fas fa-exclamation-triangle"></i> ${escapeHTML(message)}`;
    el.style.display = 'block';
}

function hideError() {
    elements.errorPanel.style.display = 'none';
}

function hideResult() {
    elements.resultPanel.style.display = 'none';
    elements.searchBar.style.display = 'none';
    elements.searchInput.value = '';
    elements.clearSearchBtn.style.display = 'none';
    elements.searchCount.textContent = '';
    cachedMetadata = null;
}

function getFileName(path) {
    const sep = path.includes('\\') ? '\\' : '/';
    return path.split(sep).pop() || path;
}

function escapeHTML(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

/**
 * Toast 提示
 */
function showToast(message, type) {
    // 移除已有 toast
    const existing = document.querySelector('.toast');
    if (existing) existing.remove();

    const toast = document.createElement('div');
    toast.className = `toast ${type || ''}`;
    toast.textContent = message;
    document.body.appendChild(toast);

    // 触发显示
    requestAnimationFrame(() => {
        toast.classList.add('visible');
    });

    // 自动消失
    setTimeout(() => {
        toast.classList.remove('visible');
        setTimeout(() => toast.remove(), 300);
    }, 3000);
}