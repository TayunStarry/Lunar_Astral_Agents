// ==== 星月智能 · 文件整理 — 前端逻辑 ====

const WORKSPACE = 'package/file_organizer/workspace';
const PACKAGE_DIR = 'package/file_organizer';
const MODEL = 'system-multimodal';
const BATCH_SIZE = 20;

// 文件类型分类
const FILE_CATEGORIES = {
    text: ['.txt', '.md', '.log', '.csv', '.json', '.xml', '.yaml', '.yml', '.toml', '.ini', '.cfg', '.conf', '.html', '.css', '.js', '.ts', '.go', '.py', '.java', '.c', '.cpp', '.h', '.rs', '.rb', '.sh', '.bat', '.ps1'],
    image: ['.png', '.jpg', '.jpeg', '.webp', '.gif', '.svg', '.bmp', '.ico', '.tiff', '.tif', '.avif'],
    video: ['.mp4', '.webm', '.avi', '.mov', '.mkv', '.wmv', '.flv', '.m4v', '.mpg', '.mpeg'],
    audio: ['.mp3', '.wav', '.flac', '.aac', '.ogg', '.wma', '.m4a'],
    archive: ['.zip', '.rar', '.7z', '.tar', '.gz', '.bz2', '.xz']
};

// ==== DOM 元素 ====
const $ = (sel) => document.querySelector(sel);
const $$ = (sel) => document.querySelectorAll(sel);

// ==== 应用状态 ====
const state = {
    files: [],
    summaries: [],
    operations: [],
    isProcessing: false,
    darkMode: false
};

// ==== 工具函数 ====

function getFileCategory(filename) {
    const ext = '.' + filename.split('.').pop().toLowerCase();
    for (const [cat, exts] of Object.entries(FILE_CATEGORIES)) {
        if (exts.includes(ext)) return cat;
    }
    return 'other';
}

function getFileIcon(category) {
    const icons = {
        text: 'fa-file-lines',
        image: 'fa-image',
        video: 'fa-video',
        audio: 'fa-music',
        archive: 'fa-file-zipper',
        code: 'fa-code',
        other: 'fa-file'
    };
    return icons[category] || 'fa-file';
}

function formatSize(bytes) {
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
    if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
    return (bytes / (1024 * 1024 * 1024)).toFixed(2) + ' GB';
}

function showToast(message, type = 'info', duration = 3000) {
    const container = $('#toast-container');
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    const icons = { success: 'fa-check-circle', error: 'fa-exclamation-circle', info: 'fa-info-circle', warning: 'fa-exclamation-triangle' };
    toast.innerHTML = `<i class="fas ${icons[type] || icons.info}"></i> ${message}`;
    container.appendChild(toast);
    setTimeout(() => {
        toast.classList.add('fade-out');
        setTimeout(() => toast.remove(), 300);
    }, duration);
}

function setStatus(status, type = '') {
    const dot = $('.status-dot');
    const text = $('.status-text');
    dot.className = 'status-dot ' + type;
    text.textContent = status;
}

function setProgress(percent, status) {
    const section = $('#progress-section');
    const fill = $('#progress-fill');
    const pct = $('#progress-percent');
    const statusEl = $('#progress-status');
    if (percent > 0) {
        section.style.display = '';
        fill.style.width = percent + '%';
        pct.textContent = Math.round(percent) + '%';
        statusEl.textContent = status;
    } else {
        section.style.display = 'none';
    }
}

function sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
}

// ==== 文件操作 ====

async function uploadFile(file, relativePath) {
    const encodedPath = btoa(unescape(encodeURIComponent(relativePath)));
    const response = await fetch('/file/write', {
        method: 'POST',
        headers: {
            'X-File-Name': encodedPath,
            'X-Overwrite': 'true'
        },
        body: file
    });
    if (!response.ok) throw new Error(`上传失败: ${response.statusText}`);
    return response.json();
}

async function scanWorkspace() {
    const response = await fetch('/file/list/' + WORKSPACE, { method: 'POST' });
    if (!response.ok) throw new Error('扫描工作区失败');
    return response.json();
}

async function readFileContent(relativePath) {
    const response = await fetch('/file/read/' + relativePath);
    if (!response.ok) throw new Error('读取文件失败');
    const text = await response.text();
    return text.substring(0, 2048); // 截取前2048字符
}

async function resizeImage(relativePath) {
    // 先从服务器获取图片 Blob（/resize 端点需要服务端来源的 Blob）
    const fetchResponse = await fetch('/file/read/' + relativePath);
    if (!fetchResponse.ok) throw new Error('获取图片失败');
    const blob = await fetchResponse.blob();
    // 创建 FormData 并发送到 /resize
    const formData = new FormData();
    formData.append('image', blob);
    const resizeResponse = await fetch('/resize', {
        method: 'POST',
        body: formData
    });
    if (!resizeResponse.ok) throw new Error('图片缩放失败');
    const resizeData = await resizeResponse.json();
    // /resize 返回的 base64 可能已包含 data URI 前缀，统一去除
    let base64 = resizeData.base64 || resizeData.data || '';
    base64 = base64.replace(/^data:image\/\w+;base64,/, '');
    return base64;
}

async function extractKeyframes(relativePath) {
    // 先从服务器获取视频 Blob（/keyframe 端点需要服务端来源的 Blob）
    const fetchResponse = await fetch('/file/read/' + relativePath);
    if (!fetchResponse.ok) throw new Error('获取视频文件失败');
    const videoBlob = await fetchResponse.blob();
    const filename = relativePath.replace(/\\/g, '/').split('/').pop().trim();
    // 创建 FormData 并发送到 /keyframe
    const formData = new FormData();
    formData.append('video', videoBlob, filename);
    const extractResponse = await fetch('/keyframe', {
        method: 'POST',
        body: formData
    });
    if (!extractResponse.ok) throw new Error('关键帧提取失败');
    const data = await extractResponse.json();
    // 取前10帧
    return (data.keyFrames || []).slice(0, 10);
}

async function callAI(messages) {
    const response = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            model: MODEL,
            messages: messages,
            stream: false
        })
    });
    if (!response.ok) throw new Error(`AI 调用失败: ${response.statusText}`);
    const data = await response.json();
    // 处理 OpenAI 兼容响应格式
    if (data.choices && data.choices[0]) {
        return data.choices[0].message.content;
    }
    // 处理代理响应格式
    if (data.success && data.data && data.data.choices) {
        return data.data.choices[0].message.content;
    }
    throw new Error('AI 响应格式异常');
}

async function executeOrganize(operations) {
    const response = await fetch('/file/organize', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            base_path: WORKSPACE,
            operations: operations
        })
    });
    if (!response.ok) throw new Error('执行整理操作失败');
    return response.json();
}

// ==== 文件预处理 ====

async function preprocessFile(file) {
    const category = getFileCategory(file.name);
    const meta = {
        name: file.name,
        size: file.size,
        category: category,
        lastModified: new Date(file.lastModified).toISOString()
    };

    switch (category) {
        case 'text': {
            const content = await readFileContent(WORKSPACE + '/' + file.name);
            return { type: 'text', meta, content };
        }
        case 'image': {
            const result = await resizeImage(WORKSPACE + '/' + file.name);
            return { type: 'image', meta, base64: result };
        }
        case 'video': {
            const frames = await extractKeyframes(WORKSPACE + '/' + file.name);
            const framesData = frames.map(f => ({
                timestamp: f.timestamp,
                frameNum: f.frameNum,
                base64: f.data ? arrayBufferToBase64(f.data) : ''
            }));
            return { type: 'video', meta, frames: framesData };
        }
        case 'code': {
            const content = await readFileContent(WORKSPACE + '/' + file.name);
            return { type: 'code', meta, content };
        }
        default: {
            return { type: 'other', meta };
        }
    }
}

function arrayBufferToBase64(buffer) {
    if (typeof buffer === 'string') return buffer;
    const bytes = new Uint8Array(buffer);
    let binary = '';
    for (let i = 0; i < bytes.byteLength; i++) {
        binary += String.fromCharCode(bytes[i]);
    }
    return btoa(binary);
}

// ==== 构建 AI 消息 ====

function buildSummaryMessage(fileData) {
    const systemMsg = { role: 'system', content: SUMMARY_SYSTEM };
    let userContent;

    switch (fileData.type) {
        case 'text':
        case 'code':
            userContent = [
                { type: 'text', text: `文件名: ${fileData.meta.name}\n文件大小: ${formatSize(fileData.meta.size)}\n文件类型: ${fileData.meta.category}\n\n内容:\n${fileData.content}` }
            ];
            break;
        case 'image':
            userContent = [
                { type: 'text', text: `文件名: ${fileData.meta.name}\n文件大小: ${formatSize(fileData.meta.size)}\n请分析这张图片的内容。` },
                { type: 'image_url', image_url: { url: `data:image/jpeg;base64,${fileData.base64}` } }
            ];
            break;
        case 'video':
            userContent = [
                { type: 'text', text: `文件名: ${fileData.meta.name}\n文件大小: ${formatSize(fileData.meta.size)}\n以下是视频的关键帧截图，请分析视频内容。` }
            ];
            for (const frame of fileData.frames) {
                if (frame.base64) {
                    userContent.push({
                        type: 'image_url',
                        image_url: { url: `data:image/jpeg;base64,${frame.base64}` }
                    });
                }
            }
            break;
        default:
            userContent = [
                { type: 'text', text: `文件名: ${fileData.meta.name}\n文件大小: ${formatSize(fileData.meta.size)}\n文件类型: ${fileData.meta.category}\n创建时间: ${fileData.meta.lastModified}\n\n请根据文件元数据生成摘要。` }
            ];
            break;
    }

    return [systemMsg, { role: 'user', content: userContent }];
}

function buildOrganizeMessage(summaries) {
    const summaryText = JSON.stringify(summaries, null, 2);
    return [
        { role: 'system', content: ORGANIZE_SYSTEM },
        { role: 'user', content: `以下是文件摘要列表，请为每个文件生成整理操作方案：\n\n${summaryText}` }
    ];
}

// ==== 操作方案校验 ====

const COMMON_EXTENSIONS = new Set([
    '.png', '.jpg', '.jpeg', '.webp', '.gif', '.svg', '.bmp', '.ico', '.tiff', '.tif', '.avif',
    '.mp4', '.webm', '.avi', '.mov', '.mkv', '.wmv', '.flv', '.m4v', '.mpg', '.mpeg',
    '.txt', '.md', '.log', '.csv', '.json', '.xml', '.yaml', '.yml', '.toml', '.ini', '.cfg', '.conf',
    '.html', '.css', '.js', '.ts', '.go', '.py', '.java', '.c', '.cpp', '.h', '.rs', '.rb', '.sh', '.bat', '.ps1'
]);

function getExt(filename) {
    return '.' + filename.split('.').pop().toLowerCase();
}

function getFolder(op) {
    if (!op || !op.target) return null;
    const parts = op.target.replace(/\\/g, '/').split('/');
    return parts.length > 1 ? parts.slice(0, -1).join('/') : null;
}

/**
 * 收集所有需要校验的冲突，返回去重后的文件集合及其问题标记
 */
function collectAllConflicts(operations, limit = 50) {
    const flagged = {}; // { sourceFilename: { op, issues: ['ext_conflict', 'feature_conflict', 'unmoved'] } }

    // ---- 校验 1: 同非通用后缀名但不同文件夹 ----
    const extMap = {}; // { '.ext': { folder: [op, ...] } }
    for (const op of operations) {
        if (op.type !== 'move') continue;
        const ext = getExt(op.source);
        if (COMMON_EXTENSIONS.has(ext)) continue;
        const folder = getFolder(op);
        if (!folder) continue;
        if (!extMap[ext]) extMap[ext] = {};
        if (!extMap[ext][folder]) extMap[ext][folder] = [];
        extMap[ext][folder].push(op);
    }
    for (const [ext, folderMap] of Object.entries(extMap)) {
        if (Object.keys(folderMap).length > 1) {
            for (const ops of Object.values(folderMap)) {
                for (const op of ops) {
                    if (!flagged[op.source]) flagged[op.source] = { op: { ...op }, issues: [] };
                    flagged[op.source].issues.push('ext_conflict');
                }
            }
        }
    }

    // ---- 校验 2: 相同文件名特征但不同文件夹 ----
    const featMap = {}; // { 'feature': { folder: [op, ...] } }
    for (const op of operations) {
        if (op.type !== 'move') continue;
        const folder = getFolder(op);
        if (!folder) continue;
        const name = op.source.replace(/\.[^.]+$/, '');
        const features = new Set();
        if (name.length >= 4) features.add('pref:' + name.substring(0, 4));
        if (name.length >= 4) features.add('suff:' + name.substring(name.length - 4));
        for (const part of name.split(/[-_ .]+/)) {
            if (part.length >= 3) features.add(part.toLowerCase());
        }
        for (const feat of features) {
            if (!featMap[feat]) featMap[feat] = {};
            if (!featMap[feat][folder]) featMap[feat][folder] = [];
            featMap[feat][folder].push(op);
        }
    }
    for (const [, folderMap] of Object.entries(featMap)) {
        if (Object.keys(folderMap).length > 1) {
            for (const ops of Object.values(folderMap)) {
                for (const op of ops) {
                    if (!flagged[op.source]) flagged[op.source] = { op: { ...op }, issues: [] };
                    if (!flagged[op.source].issues.includes('feature_conflict')) {
                        flagged[op.source].issues.push('feature_conflict');
                    }
                }
            }
        }
    }

    // ---- 校验 3: 未被移动或重命名的文件 ----
    const handled = new Set();
    for (const op of operations) {
        if (op.type === 'move' || op.type === 'rename') handled.add(op.source);
    }
    for (const op of operations) {
        if (!handled.has(op.source) && op.type !== 'delete') {
            if (!flagged[op.source]) flagged[op.source] = { op: { ...op }, issues: [] };
            if (!flagged[op.source].issues.includes('unmoved')) {
                flagged[op.source].issues.push('unmoved');
            }
        }
    }

    // 限制数量
    const entries = Object.entries(flagged).slice(0, limit);
    return Object.fromEntries(entries);
}

/**
 * 构建校验消息：完整操作数组 + 标记哪些有问题
 */
function buildValidationMessage(allOps, flagged, metaIndex) {
    // 构建完整操作列表，被标记的附上问题和元数据
    const opsWithAnnotation = allOps.map(op => {
        const flag = flagged[op.source];
        const meta = metaIndex[op.source] || {};
        const entry = {
            source: op.source,
            type: op.type,
            target: op.target || null,
            _meta: {
                size: meta.size ? formatSize(meta.size) : '?',
                ext: getExt(op.source),
                category: meta.category || getFileCategory(op.source)
            }
        };
        if (flag) {
            entry._ISSUES = flag.issues;
            entry._ACTION = '请检查并修正此条目的操作';
        }
        return entry;
    });

    const userContent = JSON.stringify({
        _instruction: '以下是完整的文件操作预览方案。带 _ISSUES 标记的条目需要你检查并修正。修正后返回完整的操作数组（包含所有条目，不只是被标记的）。',
        _format: '[{"type":"move|rename|delete|merge","source":"文件名","target":"目标路径"}]',
        operations: opsWithAnnotation
    }, null, 2);

    return [
        { role: 'system', content: VALIDATION_SYSTEM },
        { role: 'user', content: userContent }
    ];
}

/**
 * 执行校验：收集冲突 → 单次 AI 调用 → 合并结果
 */
async function runValidation(allOps) {
    // 构建元数据索引
    const metaIndex = {};
    for (const f of state.files) {
        metaIndex[f.name] = {
            size: f.size,
            category: getFileCategory(f.name),
            lastModified: f.lastModified || ''
        };
    }

    // 收集所有冲突
    const flagged = collectAllConflicts(allOps);
    const flaggedCount = Object.keys(flagged).length;

    if (flaggedCount === 0) {
        console.log('[校验] 未发现需要修正的条目，跳过');
        return allOps;
    }

    setProgress(78, `查漏补缺: 发现 ${flaggedCount} 条潜在问题，正在提交 AI 复核...`);
    console.log('[校验] 发现问题条目:', flaggedCount, Object.keys(flagged));

    try {
        const messages = buildValidationMessage(allOps, flagged, metaIndex);
        const response = await callAI(messages);
        const corrected = parseAIJSON(response);

        // 合并修正结果：用 AI 返回的条目覆盖对应条目
        const merged = [...allOps];
        for (const correctedOp of corrected) {
            const idx = merged.findIndex(o => o.source === correctedOp.source);
            if (idx >= 0) {
                merged[idx] = { ...merged[idx], ...correctedOp };
            }
        }

        // 统计变更
        let changed = 0;
        for (let i = 0; i < allOps.length; i++) {
            const before = JSON.stringify(allOps[i]);
            const after = JSON.stringify(merged[i]);
            if (before !== after) changed++;
        }

        setProgress(88, `查漏补缺完成: ${changed} 条操作已修正`);
        console.log(`[校验] 完成: ${changed} 条已修正，${flaggedCount - changed} 条保持原样`);
        return merged;

    } catch (err) {
        console.error('[校验] AI 复核失败，使用原始方案:', err);
        showToast('查漏补缺校验失败，已跳过', 'warning');
        return allOps;
    }
}

function parseAIJSON(response) {
    const cleaned = response.replace(/```json\s*|```\s*/g, '').trim();
    const match = cleaned.match(/\[[\s\S]*\]/);
    if (match) return JSON.parse(match[0]);
    return JSON.parse(cleaned);
}

// ==== 网格渲染 ====

function getPreviewURL(file) {
    const category = getFileCategory(file.name);
    // 图片和视频可通过 /file/read/ 端点直接作为 img src（已设置正确的 Content-Type）
    if (category === 'image') {
        return `/file/read/${WORKSPACE}/${encodeURIComponent(file.name)}`;
    }
    return null;
}

function getStatusInfo(file) {
    if (file.opStatus === 'processing') return { cls: 'processing', icon: 'fa-circle-notch fa-spin', text: '处理中' };
    if (file.opStatus === 'pending') return { cls: 'pending', icon: 'fa-clock', text: '待操作' };
    if (file.opStatus === 'done') return { cls: 'done', icon: 'fa-check-circle', text: '已完成' };
    if (file.opStatus === 'error') return { cls: 'error', icon: 'fa-exclamation-circle', text: '失败' };
    return { cls: 'idle', icon: 'fa-circle', text: '就绪' };
}

function renderFileGrid(files) {
    const grid = $('#file-grid');
    const count = $('#file-count');

    if (!files || files.length === 0) {
        grid.innerHTML = `
            <div class="empty-state">
                <div class="empty-visual"><i class="fas fa-cloud-upload-alt"></i></div>
                <h3>拖拽文件到此处开始整理</h3>
                <p>支持连续拖入多个文件或多次拖入，文件将自动保存到工作区</p>
            </div>`;
        count.textContent = '0 个文件';
        return;
    }

    count.textContent = `${files.length} 个文件`;

    grid.innerHTML = files.map((f, i) => {
        const category = getFileCategory(f.name);
        const icon = getFileIcon(category);
        const size = f.size ? formatSize(f.size) : '';
        const previewURL = getPreviewURL(f);
        const status = getStatusInfo(f);
        const categoryLabels = { text: '文本', image: '图片', video: '视频', audio: '音频', archive: '压缩包', code: '代码', other: '其他' };

        return `
            <div class="file-card" data-index="${i}">
                <div class="file-card-preview">
                    ${previewURL
                        ? `<img src="${previewURL}" alt="${f.name}" loading="lazy" onerror="this.style.display='none';this.nextElementSibling.style.display='';">`
                        : ''}
                    <i class="fas ${icon} preview-icon" style="${previewURL ? 'display:none' : ''}"></i>
                    <div class="preview-overlay">${categoryLabels[category] || category}</div>
                </div>
                <div class="file-card-header">
                    <div class="file-card-icon ${category}"><i class="fas ${icon}"></i></div>
                    <span class="file-card-name" title="${f.name}">${f.name}</span>
                </div>
                <div class="file-card-body">
                    <div class="file-card-meta">
                        <span><i class="fas fa-weight-hanging"></i> ${size}</span>
                    </div>
                    ${f.summary ? `<div class="file-card-summary">${f.summary}</div>` : ''}
                </div>
                <div class="file-card-status ${status.cls}">
                    <i class="fas ${status.icon} status-icon"></i>
                    <span>${status.text}</span>
                </div>
            </div>`;
    }).join('');
}

function updateFileCard(index, updates) {
    if (state.files[index]) {
        Object.assign(state.files[index], updates);
    }
    renderFileGrid(state.files);
}

// ==== 审阅模态框 ====

function showReviewModal(operations) {
    const modal = $('#review-modal');
    const body = $('#review-body');
    const stats = $('#review-stats');

    const counts = { move: 0, rename: 0, merge: 0, delete: 0 };
    operations.forEach(op => { if (counts[op.type] !== undefined) counts[op.type]++; });

    stats.innerHTML = `
        <div class="stat-chip total"><i class="fas fa-list"></i> 总计 <span class="stat-count">${operations.length}</span></div>
        <div class="stat-chip move"><i class="fas fa-arrow-right"></i> 移动 <span class="stat-count">${counts.move}</span></div>
        <div class="stat-chip rename"><i class="fas fa-pen"></i> 重命名 <span class="stat-count">${counts.rename}</span></div>
        <div class="stat-chip merge"><i class="fas fa-object-group"></i> 合并 <span class="stat-count">${counts.merge}</span></div>
        <div class="stat-chip delete"><i class="fas fa-trash"></i> 删除 <span class="stat-count">${counts.delete}</span></div>
    `;

    body.innerHTML = operations.map((op, i) => {
        const typeLabels = { move: '移动', rename: '重命名', merge: '合并', delete: '删除' };
        const typeIcons = { move: 'fa-arrow-right', rename: 'fa-pen', merge: 'fa-object-group', delete: 'fa-trash' };
        return `
            <div class="review-op">
                <div class="review-op-index">${i + 1}</div>
                <div class="review-op-content">
                    <span class="review-op-type ${op.type}">${typeLabels[op.type] || op.type}</span>
                    <div class="review-op-paths">
                        <div class="review-op-source">${op.source}</div>
                        ${op.target ? `<div class="review-op-arrow"><i class="fas ${typeIcons[op.type]}"></i> ${op.target}</div>` : ''}
                    </div>
                </div>
            </div>`;
    }).join('');

    modal.style.display = '';
    state.operations = operations;
}

function hideReviewModal() {
    $('#review-modal').style.display = 'none';
}

// ==== 主流程 ====

async function startOrganize() {
    if (state.isProcessing) return;
    if (state.files.length === 0) {
        showToast('请先拖入需要整理的文件', 'warning');
        return;
    }

    state.isProcessing = true;
    setStatus('处理中', 'processing');
    $('#start-btn').disabled = true;

    try {
        // 阶段 1: 生成摘要（优先复用缓存）
        const total = state.files.length;

        // 加载缓存
        const cached = await loadSummariesFromCache();
        const cachedMap = {};
        if (cached && Array.isArray(cached)) {
            for (const s of cached) cachedMap[s.name] = s;
        }

        // 分离缓存命中与需要新生成的文件
        const cachedFiles = []; // { index, summary }
        const newFiles = [];    // { index, file }

        for (let i = 0; i < total; i++) {
            const file = state.files[i];
            if (cachedMap[file.name]) {
                cachedFiles.push({ index: i, summary: cachedMap[file.name].summary });
            } else {
                newFiles.push({ index: i, file });
            }
        }

        state.summaries = [];
        setProgress(5, cachedFiles.length > 0
            ? `已加载 ${cachedFiles.length} 条缓存摘要，${newFiles.length} 个文件需要分析`
            : '正在生成文件摘要...');

        // 先显示缓存摘要
        for (let i = 0; i < total; i++) {
            updateFileCard(i, { summary: '', opStatus: 'idle' });
        }
        for (const { index, summary } of cachedFiles) {
            updateFileCard(index, { summary, opStatus: 'idle' });
            state.summaries.push({
                name: state.files[index].name,
                size: state.files[index].size,
                category: getFileCategory(state.files[index].name),
                summary
            });
        }

        // 为新文件逐个生成摘要
        for (let j = 0; j < newFiles.length; j++) {
            const { index, file } = newFiles[j];
            const progress = 5 + ((j + 1) / Math.max(newFiles.length, 1)) * 35;
            setProgress(progress, `正在分析: ${file.name} (${j + 1}/${newFiles.length})`);

            updateFileCard(index, { opStatus: 'processing' });

            try {
                const fileData = await preprocessFile(file);
                const messages = buildSummaryMessage(fileData);
                const summary = await callAI(messages);

                state.summaries.push({
                    name: file.name,
                    size: file.size,
                    category: getFileCategory(file.name),
                    summary: summary.trim()
                });

                updateFileCard(index, { summary: summary.trim(), opStatus: 'pending' });
            } catch (err) {
                console.error(`摘要生成失败: ${file.name}`, err);
                state.summaries.push({
                    name: file.name,
                    size: file.size,
                    category: getFileCategory(file.name),
                    summary: `[摘要生成失败: ${err.message}]`
                });
                updateFileCard(index, { summary: '摘要生成失败', opStatus: 'error' });
            }
        }

        // 保存摘要 JSON（含缓存 + 新生成的）
        await saveSummaries(state.summaries);
        setProgress(40, '摘要生成完成，正在保存...');

        // 阶段 2: 生成操作方案（20个一批）
        const allOps = [];
        const batches = [];
        for (let i = 0; i < state.summaries.length; i += BATCH_SIZE) {
            batches.push(state.summaries.slice(i, i + BATCH_SIZE));
        }

        for (let b = 0; b < batches.length; b++) {
            const progress = 40 + (b / batches.length) * 35;
            setProgress(progress, `正在生成整理方案: 批次 ${b + 1}/${batches.length}`);

            const messages = buildOrganizeMessage(batches[b]);
            const response = await callAI(messages);

            // 解析 AI 返回的 JSON
            let batchOps = [];
            try {
                // 尝试提取 JSON 数组
                const jsonStr = response.replace(/```json\s*|```\s*/g, '').trim();
                const match = jsonStr.match(/\[[\s\S]*\]/);
                if (match) {
                    batchOps = JSON.parse(match[0]);
                } else {
                    batchOps = JSON.parse(jsonStr);
                }
            } catch (err) {
                console.error('解析操作方案失败:', err, response);
                showToast(`批次 ${b + 1} 操作方案解析失败，已跳过`, 'error');
                continue;
            }

            allOps.push(...batchOps);
        }

        // 阶段 2.5: 查漏补缺校验
        setProgress(75, '正在执行查漏补缺校验...');
        const validatedOps = await runValidation(allOps);

        // 阶段 3: 展示审阅模态框
        setProgress(90, '整理方案已生成，请审阅');
        showReviewModal(validatedOps);

    } catch (err) {
        console.error('整理流程失败:', err);
        showToast('整理流程失败: ' + err.message, 'error');
        setStatus('错误', 'error');
    } finally {
        state.isProcessing = false;
        $('#start-btn').disabled = false;
        setProgress(0, '');
    }
}

async function loadSummariesFromCache() {
    try {
        const res = await fetch('/file/read/package/file_organizer/summaries.json');
        if (!res.ok) return null;
        return await res.json();
    } catch {
        return null;
    }
}

async function saveSummaries(summaries) {
    const jsonStr = JSON.stringify(summaries, null, 2);
    const blob = new Blob([jsonStr], { type: 'application/json' });
    const encodedPath = btoa(unescape(encodeURIComponent(PACKAGE_DIR + '/summaries.json')));
    await fetch('/file/write', {
        method: 'POST',
        headers: {
            'X-File-Name': encodedPath,
            'X-Overwrite': 'true'
        },
        body: blob
    });
}

async function executeConfirmedOperations() {
    if (state.operations.length === 0) {
        hideReviewModal();
        return;
    }

    hideReviewModal();
    setStatus('执行中', 'processing');
    setProgress(80, '正在执行整理操作...');

    try {
        const result = await executeOrganize(state.operations);
        setProgress(100, '整理完成');

        if (result.success) {
            showToast(`整理完成！成功执行 ${result.success_count} 个操作`, 'success');
            // 文件已移动，清除过期摘要缓存
            try { await fetch('/file/delete/package/file_organizer/summaries.json', { method: 'DELETE' }); } catch {}
        } else {
            showToast(`整理完成，${result.success_count} 成功，${result.fail_count} 失败`, 'warning');
        }

        // 重新扫描
        await refreshFiles();
        setStatus('就绪');

    } catch (err) {
        console.error('执行整理操作失败:', err);
        showToast('执行失败: ' + err.message, 'error');
        setStatus('错误', 'error');
    } finally {
        setProgress(0, '');
    }
}

// ==== 文件拖拽处理 ====

async function handleDrop(files) {
    if (state.isProcessing) {
        showToast('正在处理中，请等待完成后再添加文件', 'warning');
        return;
    }

    setStatus('上传中', 'processing');
    let uploaded = 0;
    let failed = 0;

    for (const file of files) {
        try {
            const relativePath = WORKSPACE + '/' + file.name;
            await uploadFile(file, relativePath);
            uploaded++;
        } catch (err) {
            console.error(`上传失败: ${file.name}`, err);
            failed++;
        }
    }

    if (uploaded > 0) {
        showToast(`成功上传 ${uploaded} 个文件` + (failed > 0 ? `，${failed} 个失败` : ''), 'success');
        await refreshFiles();
    } else if (failed > 0) {
        showToast('所有文件上传失败', 'error');
    }

    setStatus('就绪');
}

async function refreshFiles() {
    try {
        const fileList = await scanWorkspace();
        // 过滤掉目录
        const files = (fileList || []).filter(f => !f.isDir);
        state.files = files;
        renderFileGrid(files);
        $('#start-btn').disabled = files.length === 0;
        return files;
    } catch (err) {
        console.error('扫描工作区失败:', err);
        showToast('扫描工作区失败', 'error');
        return [];
    }
}

// ==== 事件绑定 ====

function bindEvents() {
    // 主题切换
    $('#theme-toggle').addEventListener('click', () => {
        state.darkMode = !state.darkMode;
        document.body.classList.toggle('dark-mode', state.darkMode);
        const icon = $('#theme-toggle i');
        icon.className = state.darkMode ? 'fas fa-sun' : 'fas fa-moon';
    });

    // 开始整理
    $('#start-btn').addEventListener('click', startOrganize);

    // 刷新按钮
    $('#refresh-btn').addEventListener('click', async () => {
        await refreshFiles();
        showToast('文件列表已刷新', 'info');
    });

    // 清除工作区
    $('#clear-workspace-btn').addEventListener('click', async () => {
        if (!confirm('确定要清除工作区所有文件吗？此操作不可撤销。')) return;
        try {
            setStatus('处理中', 'processing');
            await fetch('/file/delete/' + WORKSPACE, { method: 'DELETE' });
            // 同时清除摘要缓存文件
            try { await fetch('/file/delete/package/file_organizer/summaries.json', { method: 'DELETE' }); } catch {}
            state.files = [];
            state.summaries = [];
            state.operations = [];
            renderFileGrid();
            setStatus('就绪');
            setProgress(0, '');
            showToast('工作区已清除', 'info');
        } catch (err) {
            console.error('清除工作区失败:', err);
            showToast('清除失败: ' + err.message, 'error');
            setStatus('就绪');
        }
    });

    // 打包工作区为 ZIP
    $('#zip-workspace-btn').addEventListener('click', async () => {
        if (state.files.length === 0) {
            showToast('工作区没有文件可打包', 'warning');
            return;
        }
        try {
            setStatus('打包中...', 'processing');
            const formData = new FormData();
            formData.append('zip_name', 'workspace_' + new Date().toISOString().slice(0, 10) + '.zip');
            for (const f of state.files) {
                const res = await fetch('/file/read/' + WORKSPACE + '/' + f.name);
                if (!res.ok) continue;
                const blob = await res.blob();
                formData.append('files', blob, f.name);
            }
            const response = await fetch('/file/archive', { method: 'POST', body: formData });
            if (!response.ok) throw new Error('打包失败');
            const blob = await response.blob();
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = 'workspace_backup.zip';
            a.click();
            URL.revokeObjectURL(url);
            setStatus('就绪');
            showToast('工作区打包完成，下载已开始', 'success');
        } catch (err) {
            console.error('打包失败:', err);
            showToast('打包失败: ' + err.message, 'error');
            setStatus('就绪');
        }
    });

    // 导出操作日志
    $('#export-log-btn').addEventListener('click', () => {
        const log = {
            export_time: new Date().toISOString(),
            workspace: WORKSPACE,
            file_count: state.files.length,
            files: state.files.map(f => ({
                name: f.name,
                size: f.size,
                category: getFileCategory(f.name),
                lastModified: f.lastModified || ''
            })),
            summaries: state.summaries.map(s => ({
                file: s.file,
                summary: s.summary,
                generated_at: s.generatedAt || ''
            })),
            operations: state.operations.map(op => ({
                type: op.type,
                source: op.source,
                target: op.target || null
            })),
            operation_count: state.operations.length
        };
        const blob = new Blob([JSON.stringify(log, null, 2)], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = 'organize_log_' + new Date().toISOString().slice(0, 19).replace(/:/g, '-') + '.json';
        a.click();
        URL.revokeObjectURL(url);
        showToast('操作日志已导出', 'success');
    });

    // 审阅模态框
    $('#review-cancel').addEventListener('click', () => {
        hideReviewModal();
        state.operations = [];
        setProgress(0, '');
        setStatus('就绪');
        showToast('已取消整理操作', 'info');
    });

    $('#review-execute').addEventListener('click', executeConfirmedOperations);

    // 点击遮罩层关闭模态框
    $('#review-modal').addEventListener('click', (e) => {
        if (e.target === $('#review-modal')) {
            hideReviewModal();
            state.operations = [];
            setProgress(0, '');
            setStatus('就绪');
        }
    });

    // 全局拖拽事件
    const dropZone = document.body;

    dropZone.addEventListener('dragover', (e) => {
        e.preventDefault();
        e.stopPropagation();
        dropZone.classList.add('drag-over');
    });

    dropZone.addEventListener('dragleave', (e) => {
        e.preventDefault();
        e.stopPropagation();
        if (e.target === dropZone) {
            dropZone.classList.remove('drag-over');
        }
    });

    dropZone.addEventListener('drop', (e) => {
        e.preventDefault();
        e.stopPropagation();
        dropZone.classList.remove('drag-over');

        const files = Array.from(e.dataTransfer.files);
        if (files.length > 0) {
            handleDrop(files);
        }
    });

    // 文件网格也支持拖拽
    const fileGrid = $('#file-grid');
    fileGrid.addEventListener('dragover', (e) => {
        e.preventDefault();
        e.stopPropagation();
        fileGrid.classList.add('drag-over');
    });

    fileGrid.addEventListener('dragleave', (e) => {
        e.preventDefault();
        e.stopPropagation();
        fileGrid.classList.remove('drag-over');
    });

    fileGrid.addEventListener('drop', (e) => {
        e.preventDefault();
        e.stopPropagation();
        fileGrid.classList.remove('drag-over');

        const files = Array.from(e.dataTransfer.files);
        if (files.length > 0) {
            handleDrop(files);
        }
    });

    // 移除路径栏（不再需要文件夹输入）
    const pathBar = document.querySelector('.path-bar');
    if (pathBar) {
        // 保留开始整理和刷新按钮，移除路径输入
        const pathInputGroup = pathBar.querySelector('.path-input-group');
        if (pathInputGroup) pathInputGroup.style.display = 'none';
    }
}

// ==== 初始化 ====

async function init() {
    bindEvents();
    await refreshFiles();
    setStatus('就绪');
}

document.addEventListener('DOMContentLoaded', init);