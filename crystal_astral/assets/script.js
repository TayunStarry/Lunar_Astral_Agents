let SYSTEM_PROMPT = '';
let pages = [];
let messages = [];
let pendingAttachments = [];
let configData = {};
let originalConfig = null;
let pendingConfigChanges = null;

const pageGrid = document.getElementById('pageGrid');
const crystalBtn = document.getElementById('crystalBtn');
const chatModal = document.getElementById('chatModal');
const previewModal = document.getElementById('previewModal');
const chatModalClose = document.getElementById('chatModalClose');
const chatMessages = document.getElementById('chatMessages');
const chatInput = document.getElementById('chatInput');
const sendBtn = document.getElementById('sendBtn');
const attachmentsPreview = document.getElementById('attachmentsPreview');
const dropOverlay = document.getElementById('dropOverlay');
const deleteModal = document.getElementById('deleteModal');
const deleteModalMessage = document.getElementById('deleteModalMessage');
const deleteCancelBtn = document.getElementById('deleteCancelBtn');
const deleteConfirmBtn = document.getElementById('deleteConfirmBtn');
const exportModal = document.getElementById('exportModal');
const exportPackageName = document.getElementById('exportPackageName');
const exportConfirmBtn = document.getElementById('exportConfirmBtn');
const exportCancelBtn = document.getElementById('exportCancelBtn');
const savePathGroup = document.getElementById('savePathGroup');
const callYuehuaBtn = document.getElementById('callYuehuaBtn');
const callYuehuaModal = document.getElementById('callYuehuaModal');
const callYuehuaModalClose = document.getElementById('callYuehuaModalClose');
const callYuehuaMessage = document.getElementById('callYuehuaMessage');
const callYuehuaStatus = document.getElementById('callYuehuaStatus');
const configBtn = document.getElementById('configBtn');
const configModal = document.getElementById('configModal');
const configModalClose = document.getElementById('configModalClose');
const configPages = document.getElementById('configPages');
const configPageIndicator = document.getElementById('configPageIndicator');
const configPrevBtn = document.getElementById('configPrevBtn');
const configNextBtn = document.getElementById('configNextBtn');
const configCancelBtn = document.getElementById('configCancelBtn');
const configSaveBtn = document.getElementById('configSaveBtn');

let currentPackageName = null;
const defaultSendBtnHTML = sendBtn.innerHTML;

const VALID_FILE_TYPES = [
    'image/png', 'image/jpeg', 'image/gif', 'image/webp',
    'text/plain', 'text/csv', 'text/html', 'text/xml', 'text/css', 'text/javascript',
    'application/json', 'application/xml', 'application/javascript', 'text/markdown'
];

const PACKAGE_FILE_EXTENSIONS = ['.ltpx', '.ltp2'];

// ===== Markdown 渲染配置 =====
function initMarked() {
    if (typeof marked !== 'undefined') {
        marked.setOptions({ breaks: true, gfm: true });
    }
}

async function renderMarkdownContent(content) {
    if (!content) return '';
    if (typeof marked !== 'undefined') {
        return await marked.parse(content);
    }
    return escapeHtml(content).replace(/\n/g, '<br>');
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function highlightCodeInContainer(container) {
    if (typeof hljs === 'undefined') return;
    container.querySelectorAll('pre code').forEach(block => {
        if (block.parentElement.classList.contains('hljs')) return;
        try { hljs.highlightElement(block); } catch (e) { console.warn('代码高亮失败', e); }
    });
}

function addCodeCopyButtons(container) {
    container.querySelectorAll('pre').forEach(pre => {
        if (pre.querySelector('.code-copy-btn')) return;
        const wrapper = document.createElement('div');
        wrapper.className = 'code-block-wrapper';
        pre.parentNode.insertBefore(wrapper, pre);
        wrapper.appendChild(pre);

        const header = document.createElement('div');
        header.className = 'code-block-header';

        const code = pre.querySelector('code');
        const langClass = code ? Array.from(code.classList).find(c => c.startsWith('language-')) : '';
        const lang = langClass ? langClass.replace('language-', '') : '';
        const langLabel = document.createElement('span');
        langLabel.className = 'code-lang';
        langLabel.textContent = lang;
        header.appendChild(langLabel);

        const copyBtn = document.createElement('button');
        copyBtn.className = 'code-copy-btn';
        copyBtn.innerHTML = '<i class="fas fa-copy"></i>';
        copyBtn.addEventListener('click', () => {
            const text = code ? code.textContent : pre.textContent;
            navigator.clipboard.writeText(text).then(() => {
                copyBtn.innerHTML = '<i class="fas fa-check"></i>';
                setTimeout(() => { copyBtn.innerHTML = '<i class="fas fa-copy"></i>'; }, 2000);
            }).catch(() => {
                const ta = document.createElement('textarea');
                ta.value = text;
                ta.style.position = 'fixed';
                ta.style.top = '-9999px';
                document.body.appendChild(ta);
                ta.select();
                document.execCommand('copy');
                document.body.removeChild(ta);
                copyBtn.innerHTML = '<i class="fas fa-check"></i>';
                setTimeout(() => { copyBtn.innerHTML = '<i class="fas fa-copy"></i>'; }, 2000);
            });
        });
        header.appendChild(copyBtn);
        wrapper.insertBefore(header, pre);
    });
}

// ===== 配置管理：标签映射（本地化翻译） =====
// 标签映射已抽离为独立文件，由 local_data/config_labels.json 提供，
// 通过 /file/read/ 接口加载，避免随前端脚本一起编译、便于维护
let labelMap = {};

async function loadConfigLabels() {
    try {
        const response = await fetch('/file/read/config_labels.json');
        if (response.ok) {
            const data = await response.json();
            labelMap = data && typeof data === 'object' ? data : {};
        }
    } catch (error) {
        console.error('加载配置标签本地化文件失败，将显示原始配置键名:', error);
    }
}

function getTopLevelKeys() {
    return configData && typeof configData === 'object' ? Object.keys(configData) : [];
}

// ===== 配置工具函数 =====
function getLabel(key) { return labelMap[key] || key; }

function setValueByPath(path, value) {
    const parts = path.replace(/\[(\d+)\]/g, '.$1').split('.');
    let current = configData;
    for (let i = 0; i < parts.length - 1; i++) {
        if (!(parts[i] in current) || current[parts[i]] === null) {
            current[parts[i]] = {};
        }
        current = current[parts[i]];
    }
    current[parts[parts.length - 1]] = value;
}

function encodeFileName(filename) {
    return btoa(unescape(encodeURIComponent(filename)));
}

// ===== 初始化 =====
async function loadConfig() {
    try {
        const response = await fetch('/file/read/lunar_config.json');
        if (response.ok) {
            configData = await response.json();
            originalConfig = JSON.parse(JSON.stringify(configData));
        }
    } catch (error) { console.error('Failed to load config:', error); }
}

async function loadSystemPrompt() {
    try {
        const response = await fetch('/liuli_system_prompt.md');
        if (!response.ok) throw new Error('加载系统提示词失败');
        const raw = await response.text();
        SYSTEM_PROMPT = processSystemPrompt(raw);
    } catch (error) {
        console.error('Failed to load system prompt:', error);
        SYSTEM_PROMPT = '你是琉璃，星月智能的领航员。帮助用户定位功能页面，使用 open_page 工具。';
    }
}

function processSystemPrompt(raw) {
    let result = raw;
    if (configData?.current_address) {
        result = result.replace(/\{\{current-address\}\}/g, configData.current_address);
    }
    result = result.replace(/\{\{current-time\}\}/g, new Date().toLocaleString('zh-CN'));
    result = result.replace(/\{\{page-count\}\}/g, String(pages.length));
    return result;
}

async function loadPages() {
    try {
        const response = await fetch('/api/packages');
        pages = await response.json();
        renderPageGrid();
        initTools();
    } catch (error) { console.error('Failed to load pages:', error); }
}

// ===== 网格渲染 =====
const DEFAULT_ICON_COUNT = 12;

function getRandomDefaultIcon() {
    return `/default/icon (${Math.floor(Math.random() * DEFAULT_ICON_COUNT) + 1}).webp`;
}

function renderPageGrid() {
    pageGrid.innerHTML = '';
    pages.forEach(page => {
        const card = document.createElement('div');
        card.className = 'page-card';
        card.dataset.pageId = page.id;

        // 标签：有多个时随机显示一个
        const displayTag = (page.tags && page.tags.length > 0)
            ? page.tags[Math.floor(Math.random() * page.tags.length)]
            : null;
        const isLtpTag = displayTag && /^LTP[0-9A-Za-z]+$/.test(displayTag);
        const isGitTag = displayTag === 'Git';
        const isDeepSeekTag = displayTag === 'DeepSeek';
        const isDeepSeekDemoTag = displayTag === 'DeepSeek-Demo';

        card.innerHTML = `
            ${displayTag ? `<span class="card-tag${isLtpTag ? ' card-tag-ltp' : ''}${isGitTag ? ' card-tag-git' : ''}${isDeepSeekTag ? ' card-tag-deepseek' : ''}${isDeepSeekDemoTag ? ' card-tag-deepseek-demo' : ''}">${displayTag}</span>` : ''}
            <div class="icon">
                <img src="${page.icon || getRandomDefaultIcon()}" alt="${page.title}" onerror="this.onerror=null;this.src=getRandomDefaultIcon()">
            </div>
            <h3>${page.title}</h3>
            <p>${page.description}</p>
            <div class="card-actions">
                <button class="card-btn card-btn-export" title="导出包" data-action="export" data-package="${page.package_name || ''}">
                    <i class="fas fa-box"></i> 导出
                </button>
                <button class="card-btn card-btn-delete" title="删除包" data-action="delete" data-package="${page.package_name || ''}">
                    <i class="fas fa-trash-alt"></i> 删除
                </button>
            </div>
        `;

        card.addEventListener('click', (e) => {
            if (e.target.closest('.card-btn')) return;
            openPage(page);
        });

        pageGrid.appendChild(card);
    });

    document.querySelectorAll('.card-btn').forEach(btn => {
        btn.addEventListener('click', (e) => {
            e.stopPropagation();
            const action = btn.dataset.action;
            const pkgName = btn.dataset.package;
            if (action === 'delete') openDeleteModal(pkgName);
            else if (action === 'export') openExportModal(pkgName);
        });
    });
}

function initTools() {
    window.tools = [
        {
            type: 'function',
            function: {
                name: 'open_page',
                description: '根据用户要求或自己的判断，定位到对应的功能页面',
                parameters: {
                    type: 'object',
                    properties: {
                        page_id: { type: 'string', description: '要定位的页面 ID，需从现有页面列表中选择', enum: pages.map(p => p.id) }
                    },
                    required: ['page_id']
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'get_config',
                description: '获取当前系统配置的全部或指定部分内容',
                parameters: {
                    type: 'object',
                    properties: {
                        section: { type: 'string', description: '要获取的配置节名称，如 models、server、agent 等。不传则返回全部', enum: getTopLevelKeys() }
                    },
                    required: []
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'modify_config',
                description: '修改系统配置项，会自动弹出确认对话框让用户确认变更',
                parameters: {
                    type: 'object',
                    properties: {
                        changes: { type: 'object', description: '要修改的配置变更对象，键为配置路径，值为新值。支持嵌套对象' }
                    },
                    required: ['changes']
                }
            }
        }
    ];
}

// ===== 页面打开 =====
// 打开统一走覆盖层 iframe（与月华调用共用同一 iframe），不再整页跳转
function openPage(page) {
    if (page.tags && page.tags.includes('LTPX') && page.url && page.url.endsWith('.md')) {
        addMessage('system', `已为您打开工具文档【${page.title}】`);
        const viewerUrl = '/file/read/package/tool_viewer/index.html?url='
            + encodeURIComponent(page.url)
            + '&title=' + encodeURIComponent(page.title);
        openPageInFrame(viewerUrl, page.title);
        return;
    }

    if (page.path) {
        // 外部原生应用（exe/ps1/bat）无法在 iframe 中嵌入，保持独立启动
        addMessage('system', `已为您启动【${page.title}】`);
        loadApplication(page.path);
        return;
    }

    if (page.url && /^https?:\/\//i.test(page.url)) {
        // 外部链接（如 Git 仓库）受跨站限制无法可靠 iframe 嵌入，保持原窗口跳转
        addMessage('system', `已为您打开【${page.title}】`);
        window.open(page.url, '_self');
        return;
    }

    addMessage('system', `已为您打开【${page.title}】`);
    openPageInFrame(page.url, page.title, page.id);
}

async function loadApplication(path) {
    try {
        const response = await fetch('/load/application', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ path })
        });
        const data = await response.json();
        addMessage('system', data.success ? '应用程序启动成功！' : `启动失败: ${data.message}`);
    } catch (error) {
        console.error('Error loading application:', error);
        addMessage('system', '启动应用程序时发生错误');
    }
}

// ===== 搜索定位 =====
function locateAndHighlightCard(pageId, onHighlightEnd) {
    const card = document.querySelector(`.page-card[data-page-id="${pageId}"]`);
    if (!card) return;

    if (chatModal.classList.contains('active')) {
        fadeOutChatModal();
    }

    card.scrollIntoView({ behavior: 'smooth', block: 'center' });
    card.classList.remove('highlight');
    void card.offsetWidth;
    card.classList.add('highlight');

    // 监听高亮动画结束后执行回调（如打开页面）
    const onAnimEnd = () => {
        card.removeEventListener('animationend', onAnimEnd);
        card.classList.remove('highlight');
        if (onHighlightEnd) onHighlightEnd();
    };
    card.addEventListener('animationend', onAnimEnd);
}

function fadeOutChatModal() {
    chatModal.style.transition = 'opacity 0.3s ease';
    chatModal.style.opacity = '0';
    setTimeout(() => {
        chatModal.classList.remove('active');
        chatModal.style.opacity = '';
        chatModal.style.transition = '';
    }, 300);
}

// ===== 消息系统 =====
function addMessage(role, content) {
    const message = { role, content };
    if (role !== 'system') messages.push(message);
    if (messages.length > 20) {
        messages.shift();
        chatMessages.removeChild(chatMessages.firstChild);
    }
    renderMessage(message);
    chatMessages.scrollTop = chatMessages.scrollHeight;
}

function renderMessage(message) {
    if (message.role === 'system') {
        const div = document.createElement('div');
        div.className = 'message system';
        div.textContent = message.content;
        chatMessages.appendChild(div);
        return;
    }

    const row = document.createElement('div');
    row.className = `message-row ${message.role}`;

    const avatar = document.createElement('div');
    avatar.className = 'message-avatar';
    if (message.role === 'user') {
        avatar.classList.add('user-avatar');
        avatar.innerHTML = '<i class="fas fa-user"></i>';
    } else {
        avatar.classList.add('ai-avatar');
        const img = document.createElement('img');
        img.src = '/avatar.webp';
        img.alt = '琉璃';
        avatar.appendChild(img);
    }

    const bubble = document.createElement('div');
    bubble.className = `message-bubble ${message.role}`;

    if (message.role === 'assistant') {
        // 助手消息：Markdown 渲染
        const content = Array.isArray(message.content)
            ? message.content.filter(i => i.type === 'text').map(i => i.text).join('\n')
            : message.content;
        bubble.classList.add('markdown-content');
        renderMarkdownContent(content).then(html => {
            bubble.innerHTML = html;
            highlightCodeInContainer(bubble);
            addCodeCopyButtons(bubble);
        });
    } else if (Array.isArray(message.content)) {
        message.content.forEach(item => {
            if (item.type === 'text') {
                bubble.appendChild(document.createTextNode(item.text));
            } else if (item.type === 'image_url') {
                const img = document.createElement('img');
                img.src = item.image_url.url;
                img.alt = 'Uploaded image';
                img.addEventListener('click', () => window.open(img.src, '_blank'));
                bubble.appendChild(img);
            }
        });
    } else {
        bubble.textContent = message.content;
    }

    if (message.role === 'user') {
        row.appendChild(bubble);
        row.appendChild(avatar);
    } else {
        row.appendChild(avatar);
        row.appendChild(bubble);
    }

    chatMessages.appendChild(row);
}

// ===== 附件处理 =====
function addAttachment(file, dataUrl) {
    pendingAttachments.push({ file, dataUrl });
    renderAttachments();
}

function renderAttachments() {
    attachmentsPreview.innerHTML = '';
    pendingAttachments.forEach((att, idx) => {
        const bubble = document.createElement('div');
        bubble.className = 'attachment-bubble';
        bubble.title = att.file.name;

        if (att.file.type.startsWith('image/')) {
            const img = document.createElement('img');
            img.src = att.dataUrl;
            img.alt = '缩略图';
            bubble.appendChild(img);
        } else {
            const icon = document.createElement('i');
            icon.className = 'fas fa-file-alt';
            bubble.appendChild(icon);
        }

        const removeBtn = document.createElement('span');
        removeBtn.className = 'remove-btn';
        removeBtn.innerHTML = '&times;';
        removeBtn.dataset.id = idx;
        removeBtn.addEventListener('click', (e) => {
            pendingAttachments.splice(parseInt(e.target.dataset.id), 1);
            renderAttachments();
        });
        bubble.appendChild(removeBtn);
        attachmentsPreview.appendChild(bubble);
    });
}

// ===== 发送消息 =====
async function handleSend() {
    const text = chatInput.value.trim();
    if (!text && pendingAttachments.length === 0) return;
    if (sendBtn.disabled) return;

    const content = [];
    if (text) content.push({ type: 'text', text });

    for (const att of pendingAttachments) {
        if (att.file.type.startsWith('image/')) {
            content.push({ type: 'image_url', image_url: { url: att.dataUrl } });
        } else {
            content.push({ type: 'text', text: att.dataUrl.substring(0, 4096) });
        }
    }

    addMessage('user', content);
    chatInput.value = '';
    pendingAttachments = [];
    renderAttachments();

    sendBtn.disabled = true;
    sendBtn.classList.add('loading');
    sendBtn.innerHTML = '';

    // 构建增强版系统提示词，附加当前配置信息
    const enhancedSystemPrompt = SYSTEM_PROMPT + '\n\n' + buildConfigContext();
    const allMessages = [{ role: 'system', content: enhancedSystemPrompt }, ...messages];

    try {
        const modelName = configData?.cloud?.multimodal_model_name || 'system-multimodal';
        const response = await fetch('/v1/chat/completions', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ model: modelName, messages: allMessages, tools: window.tools })
        });

        if (!response.ok) throw new Error('Network response was not ok');

        const data = await response.json();
        const assistantMessage = data.choices[0].message;

        if (assistantMessage.tool_calls && assistantMessage.tool_calls.length > 0) {
            for (const toolCall of assistantMessage.tool_calls) {
                await handleToolCall(toolCall);
            }
            if (assistantMessage.content) {
                addMessage('assistant', assistantMessage.content);
            }
        } else {
            const replyContent = assistantMessage.content || '';
            addMessage('assistant', replyContent);
            // 解析回复中的 JSON 配置变更
            parseConfigChangeFromReply(replyContent);
        }
    } catch (error) {
        console.error('Error:', error);
        addMessage('system', '请求失败，请稍后重试');
    } finally {
        sendBtn.disabled = false;
        sendBtn.classList.remove('loading');
        sendBtn.innerHTML = defaultSendBtnHTML;
    }
}

// ===== 构建配置上下文（附加到系统提示词） =====
function buildConfigContext() {
    if (!configData || Object.keys(configData).length === 0) return '';
    return `【当前系统配置信息】
以下是星月智能系统的当前配置，你可以根据用户需求建议或执行配置修改。

${JSON.stringify(configData, null, 2)}

【配置修改指南】
如果用户要求修改配置，你可以：
1. 使用 modify_config 工具直接提交配置变更
2. 或在回复中使用 \`\`\`json 代码块包含配置变更对象，系统会自动弹出确认对话框

配置变更 JSON 示例：
\`\`\`json
{
  "server": {
    "port": 8080
  }
}
\`\`\`

当前可编辑的顶级配置节：${getTopLevelKeys().join('、')}`;
}

// ===== AI 工具调用处理 =====
async function handleToolCall(toolCall) {
    const fnName = toolCall.function.name;
    const args = JSON.parse(toolCall.function.arguments);

    switch (fnName) {
        case 'open_page': {
            const page = pages.find(p => p.id === args.page_id);
            if (page) {
                locateAndHighlightCard(args.page_id, () => openPage(page));
            }
            break;
        }
        case 'get_config': {
            if (args.section) {
                const sectionData = configData[args.section];
                addMessage('system', `【${getLabel(args.section)}】配置已获取`);
                addMessage('assistant', `当前${getLabel(args.section)}配置如下：\n\`\`\`json\n${JSON.stringify(sectionData, null, 2)}\n\`\`\``);
            } else {
                addMessage('assistant', `当前全部配置如下：\n\`\`\`json\n${JSON.stringify(configData, null, 2)}\n\`\`\``);
            }
            break;
        }
        case 'modify_config': {
            if (args.changes) {
                const originalSnapshot = JSON.parse(JSON.stringify(configData));
                const mergedConfig = deepMerge(originalSnapshot, args.changes);
                showPreviewModal(originalSnapshot, args.changes, mergedConfig);
                addMessage('system', '已生成配置变更预览，请在弹窗中确认');
            }
            break;
        }
    }
}

// ===== 解析 AI 回复中的 JSON 配置变更 =====
function parseConfigChangeFromReply(content) {
    const jsonMatch = content.match(/```json([\s\S]*?)```/);
    if (!jsonMatch) return;
    try {
        const modifiedConfig = JSON.parse(jsonMatch[1].trim());
        const originalSnapshot = JSON.parse(JSON.stringify(configData));
        const mergedConfig = deepMerge(originalSnapshot, modifiedConfig);
        showPreviewModal(originalSnapshot, modifiedConfig, mergedConfig);
    } catch (e) {
        console.error('解析配置变更失败:', e);
    }
}

// ===== 配置深合并 =====
function deepMerge(target, source) {
    const result = { ...target };
    for (const key in source) {
        if (source[key] && typeof source[key] === 'object' && !Array.isArray(source[key])) {
            result[key] = key in target ? deepMerge(target[key], source[key]) : source[key];
        } else {
            result[key] = source[key];
        }
    }
    return result;
}

function getChangedConfig(original, modified) {
    const changes = {};
    for (const key in modified) {
        if (!(key in original)) {
            changes[key] = modified[key];
        } else if (typeof modified[key] === 'object' && typeof original[key] === 'object' && !Array.isArray(modified[key]) && !Array.isArray(original[key])) {
            const nestedChanges = getChangedConfig(original[key], modified[key]);
            if (Object.keys(nestedChanges).length > 0) changes[key] = nestedChanges;
        } else if (JSON.stringify(original[key]) !== JSON.stringify(modified[key])) {
            changes[key] = modified[key];
        }
    }
    return changes;
}

// ===== 配置保存 =====
async function saveConfig() {
    try {
        collectConfigFromModals();
        const jsonString = JSON.stringify(configData, null, '\t');
        const blob = new Blob([jsonString], { type: 'application/json' });
        const res = await fetch('/file/write', {
            method: 'POST',
            headers: {
                'X-File-Name': encodeFileName('lunar_config.json'),
                'X-Overwrite': 'true'
            },
            body: blob
        });
        if (!res.ok) throw new Error('保存失败');
        originalConfig = JSON.parse(JSON.stringify(configData));
        addMessage('system', '配置已保存成功！');
    } catch (e) {
        console.error(e);
        addMessage('system', '配置保存失败，请检查服务');
    }
}

function collectConfigFromModals() {
    document.querySelectorAll('[data-path]').forEach(el => {
        const path = el.dataset.path;
        if (!path) return;
        if (el.type === 'checkbox') {
            setValueByPath(path, el.checked);
        } else if (el.type === 'number') {
            setValueByPath(path, parseFloat(el.value));
        } else if (el.tagName === 'INPUT' && el.type === 'text') {
            const val = el.value.trim() === '' ? null : el.value;
            setValueByPath(path, val);
        }
    });
}

// ===== 配置预览模态框 =====
function showPreviewModal(original, modified, merged) {
    // 隐藏聊天模态框，确保预览弹窗不被遮挡
    if (chatModal.classList.contains('active')) {
        chatModal.classList.remove('active');
    }
    pendingConfigChanges = { original, modified, merged };
    const changed = getChangedConfig(original, modified);
    document.getElementById('originalConfigPreview').textContent = JSON.stringify(original, null, 2);
    document.getElementById('changedConfigPreview').textContent = JSON.stringify(changed, null, 2);
    document.getElementById('mergedConfigPreview').textContent = JSON.stringify(merged, null, 2);
    previewModal.classList.add('active');
}

function closePreviewModal() {
    previewModal.classList.remove('active');
    pendingConfigChanges = null;
}

function applyConfigChanges() {
    if (pendingConfigChanges) {
        configData = JSON.parse(JSON.stringify(pendingConfigChanges.merged));
        originalConfig = JSON.parse(JSON.stringify(configData));
        saveConfig();
        addMessage('system', '配置已成功更新！');
    }
    closePreviewModal();
}

// ===== 配置编辑模态框（自动分页） =====
const CONFIG_BUBBLE_MIN_WIDTH = 240; // 单个气泡最小宽度
const CONFIG_BUBBLE_ROW_HEIGHT = 120; // 单个气泡行高
const CONFIG_BUBBLE_GAP = 12;
let configPageSize = 8; // 单页条数，运行时根据可用空间动态计算
let configEntries = [];
let configCurrentPage = 0;
let configTotalPages = 1;

// 将配置递归展开为扁平的叶子项列表
function flattenConfigEntries(data, basePath = '') {
    const result = [];
    Object.keys(data).forEach(key => {
        const value = data[key];
        const path = basePath ? `${basePath}.${key}` : key;

        if (Array.isArray(value)) {
            value.forEach((item, idx) => {
                const itemPath = `${path}[${idx}]`;
                if (item !== null && typeof item === 'object') {
                    result.push(...flattenConfigEntries(item, itemPath));
                } else {
                    result.push({ path: itemPath, key, index: idx, value: item });
                }
            });
        } else if (value !== null && typeof value === 'object') {
            result.push(...flattenConfigEntries(value, path));
        } else {
            result.push({ path, key, value });
        }
    });
    return result;
}

function entryLabel(entry) {
    const base = getLabel(entry.key);
    return entry.index !== undefined ? `${base}[${entry.index}]` : base;
}

function createFlatEntryBubble(entry) {
    const bubble = document.createElement('div');
    bubble.className = 'config-bubble';

    const label = document.createElement('span');
    label.className = 'bubble-label';
    label.textContent = entryLabel(entry);
    bubble.appendChild(label);

    const pathHint = document.createElement('span');
    pathHint.className = 'bubble-path';
    pathHint.textContent = entry.path;
    bubble.appendChild(pathHint);

    if (typeof entry.value === 'boolean') {
        const sw = document.createElement('label');
        sw.className = 'config-switch';
        sw.innerHTML = `<input type="checkbox" data-path="${entry.path}" ${entry.value ? 'checked' : ''}><span class="config-slider"></span>`;
        bubble.appendChild(sw);
    } else if (typeof entry.value === 'number') {
        const input = document.createElement('input');
        input.type = 'number';
        input.className = 'bubble-input';
        input.value = entry.value;
        input.dataset.path = entry.path;
        bubble.appendChild(input);
    } else {
        const input = document.createElement('input');
        input.type = 'text';
        input.className = 'bubble-input';
        input.value = entry.value === null ? '' : entry.value;
        input.placeholder = entry.value === null ? 'null' : '';
        input.dataset.path = entry.path;
        bubble.appendChild(input);
    }
    return bubble;
}

// 根据配置容器可用宽高，动态计算单页可容纳的条数
function computeConfigPageSize() {
    const w = configPages.clientWidth;
    const h = configPages.clientHeight;
    if (!w || !h) return configPageSize;

    const cols = Math.max(1, Math.floor((w + CONFIG_BUBBLE_GAP) / (CONFIG_BUBBLE_MIN_WIDTH + CONFIG_BUBBLE_GAP)));
    const rows = Math.max(1, Math.floor((h + CONFIG_BUBBLE_GAP) / (CONFIG_BUBBLE_ROW_HEIGHT + CONFIG_BUBBLE_GAP)));
    return Math.max(1, cols * rows);
}

function renderConfigPage() {
    const start = configCurrentPage * configPageSize;
    const pageEntries = configEntries.slice(start, start + configPageSize);

    configPages.innerHTML = '';
    const grid = document.createElement('div');
    grid.className = 'config-bubble-grid';
    pageEntries.forEach(entry => grid.appendChild(createFlatEntryBubble(entry)));
    configPages.appendChild(grid);

    configPageIndicator.textContent = `第 ${configCurrentPage + 1} / ${configTotalPages} 页`;
    configPrevBtn.disabled = configCurrentPage <= 0;
    configNextBtn.disabled = configCurrentPage >= configTotalPages - 1;
}

function gotoConfigPage(page) {
    // 先收集当前页编辑，再按最新内容重新分页
    collectConfigFromModals();
    configEntries = flattenConfigEntries(configData || {});
    configTotalPages = Math.max(1, Math.ceil(configEntries.length / configPageSize));
    configCurrentPage = Math.max(0, Math.min(page, configTotalPages - 1));
    renderConfigPage();
}

function openConfigModal() {
    configModal.classList.add('active');
    // 等待布局完成后测量可用空间，确定单页条数
    requestAnimationFrame(() => {
        requestAnimationFrame(() => {
            configPageSize = computeConfigPageSize();
            configEntries = flattenConfigEntries(configData || {});
            configTotalPages = Math.max(1, Math.ceil(configEntries.length / configPageSize));
            configCurrentPage = 0;
            renderConfigPage();
        });
    });
}

function closeConfigModal() {
    configModal.classList.remove('active');
}

// 模态框尺寸变化时，重新计算单页条数并重排
const configResizeObserver = new ResizeObserver(() => {
    if (!configModal.classList.contains('active')) return;
    const newSize = computeConfigPageSize();
    if (newSize !== configPageSize) {
        collectConfigFromModals();
        configEntries = flattenConfigEntries(configData || {});
        configPageSize = newSize;
        configTotalPages = Math.max(1, Math.ceil(configEntries.length / configPageSize));
        configCurrentPage = Math.min(configCurrentPage, configTotalPages - 1);
        renderConfigPage();
    }
});
configResizeObserver.observe(configPages);

// ===== 安装扩展包 =====
async function installPackage(file) {
    addMessage('system', `正在安装扩展包【${file.name}】...`);

    const formData = new FormData();
    formData.append('package_file', file);

    try {
        const response = await fetch('/file/package/install', {
            method: 'POST',
            body: formData
        });
        const data = await response.json();

        if (data.success) {
            addMessage('system', `扩展包安装成功！【${data.package_title}】`);
            setTimeout(() => loadPages(), 500);
        } else {
            addMessage('system', `安装失败: ${data.message}`);
        }
    } catch (error) {
        console.error('Error installing package:', error);
        addMessage('system', '安装扩展包时发生网络错误');
    }
}

function isPackageFile(file) {
    const name = file.name.toLowerCase();
    return PACKAGE_FILE_EXTENSIONS.some(ext => name.endsWith(ext));
}

// ===== 事件绑定 =====

// 水晶按钮 → 打开聊天模态框
crystalBtn.addEventListener('click', () => {
    chatModal.style.opacity = '';
    chatModal.style.transition = '';
    chatModal.classList.add('active');
    setTimeout(() => chatInput.focus(), 100);
});

// 关闭聊天模态框
chatModalClose.addEventListener('click', () => {
    chatModal.classList.remove('active');
});

chatModal.addEventListener('click', (e) => {
    if (e.target === chatModal) chatModal.classList.remove('active');
});

// 聊天输入
chatInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && !e.ctrlKey) {
        e.preventDefault();
        handleSend();
    } else if (e.key === 'Enter' && e.ctrlKey) {
        e.preventDefault();
        chatInput.value += '\n';
    }
});

sendBtn.addEventListener('click', handleSend);

// 查看配置按钮 → 打开自动分页配置模态框
configBtn.addEventListener('click', openConfigModal);

// 配置模态框：关闭 / 分页 / 保存
configModalClose.addEventListener('click', closeConfigModal);
configCancelBtn.addEventListener('click', closeConfigModal);
configModal.addEventListener('click', (e) => {
    if (e.target === configModal) closeConfigModal();
});
configPrevBtn.addEventListener('click', () => gotoConfigPage(configCurrentPage - 1));
configNextBtn.addEventListener('click', () => gotoConfigPage(configCurrentPage + 1));
configSaveBtn.addEventListener('click', async () => {
    await saveConfig();
    closeConfigModal();
});

// 全局拖拽上传
document.addEventListener('dragover', (e) => {
    e.preventDefault();
    dropOverlay.classList.add('active');
});

document.addEventListener('dragleave', (e) => {
    if (e.relatedTarget === null) dropOverlay.classList.remove('active');
});

document.addEventListener('drop', async (e) => {
    e.preventDefault();
    dropOverlay.classList.remove('active');

    const files = Array.from(e.dataTransfer.files);
    for (const file of files) {
        if (isPackageFile(file)) {
            await installPackage(file);
            continue;
        }
        if (!VALID_FILE_TYPES.includes(file.type)) {
            addMessage('system', `不支持的文件类型: ${file.name}`);
            continue;
        }
        if (file.type.startsWith('image/')) {
            const reader = new FileReader();
            reader.onload = (e) => { addAttachment(file, e.target.result); };
            reader.readAsDataURL(file);
        } else {
            const text = await file.text();
            addAttachment(file, text);
        }
    }
});

// ===== 删除模态框 =====
function openDeleteModal(packageName) {
    if (!packageName) { addMessage('system', '无法获取包名信息'); return; }
    currentPackageName = packageName;
    deleteModalMessage.textContent = `确定要删除扩展包【${packageName}】吗？此操作不可撤销，所有文件将被永久删除。`;
    deleteModal.classList.add('active');
}

function closeDeleteModal() {
    deleteModal.classList.remove('active');
    currentPackageName = null;
}

deleteCancelBtn.addEventListener('click', closeDeleteModal);
deleteModal.addEventListener('click', (e) => {
    if (e.target === deleteModal) closeDeleteModal();
});

deleteConfirmBtn.addEventListener('click', async () => {
    if (!currentPackageName) return;
    deleteConfirmBtn.disabled = true;
    deleteConfirmBtn.textContent = '删除中...';

    try {
        const response = await fetch('/file/package/delete', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ package_name: currentPackageName })
        });
        const data = await response.json();

        if (data.success) {
            addMessage('system', `扩展包【${currentPackageName}】已删除`);
            closeDeleteModal();
            setTimeout(() => loadPages(), 500);
        } else {
            addMessage('system', `删除失败: ${data.message}`);
        }
    } catch (error) {
        console.error('Error deleting package:', error);
        addMessage('system', '删除扩展包时发生网络错误');
    } finally {
        deleteConfirmBtn.disabled = false;
        deleteConfirmBtn.textContent = '确认删除';
    }
});

// ===== 导出模态框 =====
function openExportModal(packageName) {
    if (!packageName) { addMessage('system', '无法获取包名信息'); return; }
    currentPackageName = packageName;
    exportPackageName.value = packageName;
    document.querySelector('input[name="exportAction"][value="download"]').checked = true;
    savePathGroup.style.display = 'none';
    exportModal.classList.add('active');
}

function closeExportModal() {
    exportModal.classList.remove('active');
    currentPackageName = null;
}

exportCancelBtn.addEventListener('click', closeExportModal);
exportModal.addEventListener('click', (e) => {
    if (e.target === exportModal) closeExportModal();
});

document.querySelectorAll('input[name="exportAction"]').forEach(radio => {
    radio.addEventListener('change', () => {
        savePathGroup.style.display = radio.value === 'save' ? 'block' : 'none';
    });
});

exportConfirmBtn.addEventListener('click', async () => {
    const packageName = exportPackageName.value.trim();
    if (!packageName) { addMessage('system', '请输入包名'); return; }

    const action = document.querySelector('input[name="exportAction"]:checked').value;
    const savePath = document.getElementById('exportSavePath').value.trim();

    exportConfirmBtn.disabled = true;
    exportConfirmBtn.textContent = '导出中...';

    try {
        const body = { package_name: packageName, action };
        if (action === 'save') body.save_path = savePath || undefined;

        const response = await fetch('/file/package/export', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body)
        });

        if (action === 'download') {
            if (!response.ok) {
                const data = await response.json();
                addMessage('system', `导出失败: ${data.message}`);
                return;
            }
            const blob = await response.blob();
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = packageName + '.ltpx';
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);
            addMessage('system', `扩展包【${packageName}.ltpx】已开始下载`);
            closeExportModal();
        } else {
            const data = await response.json();
            if (data.success) {
                addMessage('system', data.message);
                closeExportModal();
            } else {
                addMessage('system', `导出失败: ${data.message}`);
            }
        }
    } catch (error) {
        console.error('Error exporting package:', error);
        addMessage('system', '导出扩展包时发生网络错误');
    } finally {
        exportConfirmBtn.disabled = false;
        exportConfirmBtn.textContent = '确认导出';
    }
});

// ===== 预览模态框事件 =====
document.getElementById('previewCloseBtn').addEventListener('click', closePreviewModal);
document.getElementById('cancelChangesBtn').addEventListener('click', closePreviewModal);
document.getElementById('applyChangesBtn').addEventListener('click', applyConfigChanges);
previewModal.addEventListener('click', (e) => {
    if (e.target === previewModal) closePreviewModal();
});

// ===== 包执行覆盖层（LTPX AtoA：包页面在 iframe 中执行，琉璃仅中转展示与回执） =====
const ltpxOverlay = document.getElementById('ltpxOverlay');
const ltpxFrame = document.getElementById('ltpxFrame');
const ltpxFrameTitle = document.getElementById('ltpxFrameTitle');
const ltpxFrameCloseBtn = document.getElementById('ltpxFrameCloseBtn');
let activeLTPXCall = null;   // 当前等待回执的 ltpx_call（含 request_id/tool/arguments）
let ltpxFrameReady = false;  // iframe 当前文档是否已加载完成（就绪后再投递指令）
let ltpxFrameApp = '';       // iframe 当前已加载的包 ID（同一包重复调用时直接投递，避免重新加载）

// 统一打开页面到覆盖层 iframe：用户点击应用图标与月华调用共用同一个 iframe。
// 手动打开时置空待定回执，并记录来源包 ID（月华随后调用同包工具时可直接复用已加载页面）。
function openPageInFrame(url, title, appId) {
    if (!url) return;
    ltpxFrameTitle.innerHTML = '<i class="fas fa-cube"></i> ' + (title || '页面');
    ltpxFrameReady = false;
    activeLTPXCall = null;       // 手动打开不等待任何回执
    ltpxFrameApp = appId || '';  // 记录来源包 ID，便于月华同包调用复用已加载页面
    ltpxFrame.src = url;
    ltpxOverlay.classList.add('active');
}

// 收到月华的 ltpx_call：打开对应包页面并投递执行指令
function openLTPXPackage(msg) {
    activeLTPXCall = msg;
    ltpxFrameTitle.innerHTML = '<i class="fas fa-cube"></i> ' + (msg.app_id || '包') + ' 执行中...';
    if (ltpxFrameApp === msg.app_id && ltpxFrameReady) {
        // 同一包页面已就绪：直接投递执行，无需重新加载（相同 src 不会触发 load 事件）
        deliverLTPXRun();
    } else {
        ltpxFrameReady = false;
        ltpxFrameApp = msg.app_id;
        ltpxFrame.src = '/file/read/package/' + encodeURIComponent(msg.app_id) + '/index.html';
    }
    ltpxOverlay.classList.add('active');
}

// 向包页面投递执行指令（仅在 iframe 加载完成后执行一次）
function deliverLTPXRun() {
    if (!activeLTPXCall || !ltpxFrameReady) return;
    const win = ltpxFrame.contentWindow;
    if (!win) return;
    win.postMessage({
        type: 'ltpx_run',
        request_id: activeLTPXCall.request_id,
        tool: activeLTPXCall.tool,
        arguments: activeLTPXCall.arguments
    }, '*');
}

// iframe 加载完成后投递（保证包内部监听器已注册）
ltpxFrame.addEventListener('load', () => {
    ltpxFrameReady = true;
    deliverLTPXRun();
});

// 关闭覆盖层（若包尚未回执，琉璃端会因超时向月华返回错误）
function closeLTPXOverlay() {
    ltpxOverlay.classList.remove('active');
    activeLTPXCall = null;
}
ltpxFrameCloseBtn.addEventListener('click', closeLTPXOverlay);
ltpxOverlay.addEventListener('click', (e) => { if (e.target === ltpxOverlay) closeLTPXOverlay(); });

// 包执行完毕通过 window.parent.postMessage 回传，主窗口代为上报 /ltpx/result
window.addEventListener('message', (event) => {
    if (!event.data || typeof event.data !== 'object') return;
    if (event.data.type !== 'ltpx_result') return;
    const { request_id, success, text, error, keep_open } = event.data;
    fetch('/ltpx/result', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            request_id: request_id,
            success: !!success,
            text: text || '',
            error: error || '',
            keep_open: !!keep_open
        })
    }).catch(e => console.warn('上报 LTPX 执行结果失败:', e));
    if (activeLTPXCall && activeLTPXCall.request_id === request_id) {
        if (keep_open) {
            // 包要求保持页面展示（如文件管理器执行后停留在目标路径/选中状态），不自动关闭
            activeLTPXCall = null; // 等待状态已结束，覆盖层保留供用户查看/手动关闭，后续调用可复用该页面
            ltpxFrameTitle.innerHTML = '<i class="fas fa-check-circle"></i> ' + (ltpxFrameApp || '包') + ' 执行完成';
        } else {
            closeLTPXOverlay();
        }
    }
});

// ===== WebSocket 客户端（连接琉璃 /ws，接收文件管理器等 LTPX 调用广播） =====
let ws = null;
let wsRetry = 0;
const WS_MAX_RETRY = 5;
const WS_RETRY_INTERVAL = 3000;

function establishWebSocket() {
    try {
        const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        ws = new WebSocket(proto + '//' + window.location.host + '/ws');
    } catch (e) {
        console.error('WebSocket 创建失败:', e);
        return;
    }

    ws.onopen = () => {
        wsRetry = 0;
    };

    ws.onmessage = (event) => {
        let msg;
        try { msg = JSON.parse(event.data); } catch (e) { return; }
        if (!msg || typeof msg !== 'object') return;

        if (msg.type === 'ltpx_call') {
            // 月华调用琉璃工具：打开对应包页面并投递执行
            openLTPXPackage(msg);
        }
    };

    ws.onclose = () => {
        if (wsRetry < WS_MAX_RETRY) {
            wsRetry++;
            setTimeout(establishWebSocket, WS_RETRY_INTERVAL);
        }
    };

    ws.onerror = () => { try { ws.close(); } catch (e) { } };
}

// ===== 呼叫月华 =====
const YUEHUA_WAKEUP_AUDIO = '/file/read/audios/start_lunar.wav';
const YUEHUA_CALLING_AUDIO = '/file/read/audios/call_lunar.wav';
const YUEHUA_WAKEUP_TEXT = '月华姐姐~~ 起床啦!';
const YUEHUA_CALLING_TEXT = '琉璃: 月华姐姐，有人在找你';

async function handleCallYuehua() {
    if (callYuehuaBtn.classList.contains('loading')) return;

    callYuehuaBtn.classList.add('loading');
    callYuehuaMessage.textContent = '';
    callYuehuaStatus.textContent = '正在检测月华服务状态...';

    try {
        // 检测端口36789可用性
        const checkResp = await fetch('/lunar/check', { method: 'GET' });
        const checkData = await checkResp.json();

        if (!checkData.available) {
            // 端口不可用（月华未启动）：唤醒月华
            callYuehuaStatus.textContent = '月华服务未启动，正在唤醒月华...';
            const startResp = await fetch('/lunar/start', { method: 'POST' });
            const startData = await startResp.json();

            if (!startData.success) {
                callYuehuaMessage.textContent = '唤醒月华失败了... ' + (startData.message || '');
                callYuehuaStatus.textContent = '';
                return;
            }

            // 播放起床音频 + 显示起床对话
            playYuehuaAudio(YUEHUA_WAKEUP_AUDIO);
            callYuehuaMessage.textContent = YUEHUA_WAKEUP_TEXT;
            callYuehuaStatus.textContent = '月华已唤醒！';
        } else {
            // 端口可用（月华已启动）：推送消息 + 条件重建webView
            callYuehuaStatus.textContent = '正在呼叫月华...';
            const msgResp = await fetch('/write/message', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    messages: [{ role: 'user', content: YUEHUA_CALLING_TEXT }]
                })
            });
            const msgData = await msgResp.json();

            if (!msgData.success) {
                callYuehuaMessage.textContent = '消息发送失败了...';
                callYuehuaStatus.textContent = '';
                return;
            }

            // 播放呼叫音频 + 显示呼叫对话
            playYuehuaAudio(YUEHUA_CALLING_AUDIO);
            callYuehuaMessage.textContent = YUEHUA_CALLING_TEXT;
            callYuehuaStatus.textContent = '已通知月华！';
        }

        // 打开模态框
        callYuehuaModal.classList.add('active');

    } catch (error) {
        console.error('呼叫月华失败:', error);
        callYuehuaMessage.textContent = '呼叫月华时发生错误，请稍后重试';
        callYuehuaStatus.textContent = '';
    } finally {
        callYuehuaBtn.classList.remove('loading');
    }
}

function playYuehuaAudio(src) {
    try {
        const audio = new Audio(src);
        audio.play().catch(e => console.warn('音频播放失败:', e));
    } catch (e) {
        console.warn('音频播放异常:', e);
    }
}

// 呼叫月华按钮事件
callYuehuaBtn.addEventListener('click', handleCallYuehua);

// 关闭呼叫月华模态框
callYuehuaModalClose.addEventListener('click', () => {
    callYuehuaModal.classList.remove('active');
});

callYuehuaModal.addEventListener('click', (e) => {
    if (e.target === callYuehuaModal) callYuehuaModal.classList.remove('active');
});

// ===== 启动 =====
async function initApp() {
    await loadConfigLabels();
    await loadConfig();
    await loadSystemPrompt();
    await loadPages();
    initMarked();
    establishWebSocket();
}
initApp();