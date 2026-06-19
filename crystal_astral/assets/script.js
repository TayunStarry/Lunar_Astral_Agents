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

// ===== 配置管理：标签映射 =====
const labelMap = {
    'models': '模型配置',
    'server': '服务器配置',
    'cloud': '云端配置',
    'qq_adapter': 'QQ适配器',
    'project_archiving': '项目归档',
    'diffusion_model': '扩散模型',
    'variational_model': '变分模型',
    'prompt_analysis_model': '提示词精炼模型',
    'asr_model': '语音识别模型',
    'developer': '开发者模式',
    'clear_port': '清理端口',
    'allow_diffusion': '允许扩散',
    'allow_multimodal': '允许多模态',
    'cloud_model_url': '云端模型地址',
    'cloud_model_key': '云端模型密钥',
    'multimodal_model_name': '多模态模型名称',
    'embedding_model_name': '嵌入模型名称',
    'user_name': '用户名',
    'napcat_ws_server': 'Napcat WS服务器',
    'napcat_ws_token': 'Napcat WS令牌',
    'lunar_core_url': 'Lunar Core地址',
    'lunar_ws_server': 'Lunar WS服务器',
    'poll_interval': '轮询间隔',
    'listen_group_ids': '监听群组ID',
    'trigger_keywords': '触发关键词',
    'display_logs': '显示日志',
    'default_reply': '默认回复',
    'sevenzip_paths': '7z路径',
    'defaults': '默认设置',
    'output_path': '输出路径',
    'part_size_mb': '分卷大小(MB)',
    'compression_level': '压缩级别',
    'package_plan': '打包方案',
    'exclude': '排除文件',
    'plan-1': '方案一',
    'plan-2': '方案二',
    'plan-3': '方案三'
};

const topLevelKeys = ['models', 'server', 'cloud', 'qq_adapter', 'project_archiving'];
const iconForSection = {
    models: 'robot', server: 'server', cloud: 'cloud',
    qq_adapter: 'qq', project_archiving: 'file-archive'
};

// ===== 配置工具函数 =====
function getLabel(key) { return labelMap[key] || key; }

function getValueByPath(path) {
    const parts = path.replace(/\[(\d+)\]/g, '.$1').split('.');
    let current = configData;
    for (const part of parts) current = current[part];
    return current;
}

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
        const hasLTPX = page.tags && page.tags.includes('LTPX');

        const card = document.createElement('div');
        card.className = 'page-card';
        card.dataset.pageId = page.id;

        // 标签：有多个时随机显示一个
        const displayTag = (page.tags && page.tags.length > 0)
            ? page.tags[Math.floor(Math.random() * page.tags.length)]
            : null;
        const isLtpTag = displayTag && /^LTP[0-9A-Za-z]+$/.test(displayTag);

        card.innerHTML = `
            ${displayTag ? `<span class="card-tag${isLtpTag ? ' card-tag-ltp' : ''}">${displayTag}</span>` : ''}
            <div class="icon">
                <img src="${page.icon || getRandomDefaultIcon()}" alt="${page.title}" onerror="this.onerror=null;this.src=getRandomDefaultIcon()">
            </div>
            <h3>${page.title}</h3>
            <p>${page.description}</p>
            <div class="card-actions">
                ${hasLTPX ? `
                <button class="card-btn card-btn-load" title="加载包" data-action="load" data-package="${page.package_name || ''}">
                    <i class="fas fa-download"></i> 加载
                </button>
                <button class="card-btn card-btn-unload" title="卸载包" data-action="unload" data-package="${page.package_name || ''}">
                    <i class="fas fa-upload"></i> 卸载
                </button>
                ` : ''}
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
            else if (action === 'load') handleLoadPackage(pkgName);
            else if (action === 'unload') handleUnloadPackage(pkgName);
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
                        section: { type: 'string', description: '要获取的配置节名称，如 models、server、cloud、qq_adapter、project_archiving。不传则返回全部', enum: topLevelKeys }
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
function openPage(page) {
    if (page.tags && page.tags.includes('LTPX') && page.url && page.url.endsWith('.md')) {
        addMessage('system', `已为您打开工具文档【${page.title}】`);
        const viewerUrl = '/file/read/package/tool_viewer/index.html?url='
            + encodeURIComponent(page.url)
            + '&title=' + encodeURIComponent(page.title);
        window.open(viewerUrl, '_self');
        return;
    }

    if (page.path) {
        addMessage('system', `已为您启动【${page.title}】`);
        loadApplication(page.path);
    } else {
        addMessage('system', `已为您打开【${page.title}】`);
        window.open(page.url, '_self');
    }
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

当前可编辑的顶级配置节：${topLevelKeys.join('、')}`;
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

function resetConfig() {
    if (!originalConfig) return;
    configData = JSON.parse(JSON.stringify(originalConfig));
    // 刷新所有打开的配置模态框
    topLevelKeys.forEach(key => {
        const body = document.getElementById(`configBody_${key}`);
        if (body) {
            body.innerHTML = '';
            body.appendChild(createSectionBubbles(key, configData[key], key));
        }
    });
    addMessage('system', '配置已重置');
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
        refreshAllConfigModals();
        saveConfig();
        addMessage('system', '配置已成功更新！');
    }
    closePreviewModal();
}

// ===== 配置模态框系统 =====

/**
 * 创建配置模态框 DOM（5 个顶级配置节各一个）
 * 首次调用时动态创建，后续复用
 */
function ensureConfigModals() {
    topLevelKeys.forEach(key => {
        const modalId = `configModal_${key}`;
        if (document.getElementById(modalId)) return;

        const overlay = document.createElement('div');
        overlay.className = 'modal-overlay';
        overlay.id = modalId;

        overlay.innerHTML = `
            <div class="modal config-modal">
                <div class="modal-header">
                    <h3><i class="fas fa-${iconForSection[key] || 'cube'}"></i> ${getLabel(key)}</h3>
                    <button class="modal-close" data-close="${modalId}">&times;</button>
                </div>
                <div class="modal-body config-modal-body" id="configBody_${key}"></div>
                <div class="modal-footer">
                    <button class="btn-glass btn-glass-cancel" data-close="${modalId}">取消</button>
                    <button class="btn-glass btn-glass-primary" data-save="${key}">保存</button>
                </div>
            </div>
        `;

        document.body.appendChild(overlay);

        // 绑定关闭事件
        overlay.addEventListener('click', (e) => {
            if (e.target === overlay) closeConfigModal(key);
        });
        overlay.querySelector('[data-close]').addEventListener('click', () => closeConfigModal(key));
        overlay.querySelector('.btn-glass-cancel').addEventListener('click', () => closeConfigModal(key));
        overlay.querySelector('.btn-glass-primary').addEventListener('click', () => {
            saveConfigFromModal(key);
        });
    });
}

function openConfigModal(key) {
    ensureConfigModals();
    const modal = document.getElementById(`configModal_${key}`);
    if (!modal) return;

    // 构建配置表单内容
    const body = document.getElementById(`configBody_${key}`);
    body.innerHTML = '';
    const data = configData[key];
    if (data && typeof data === 'object') {
        body.appendChild(createSectionBubbles(key, data, key));
    }

    modal.classList.add('active');
}

function closeConfigModal(key) {
    const modal = document.getElementById(`configModal_${key}`);
    if (modal) modal.classList.remove('active');
}

function saveConfigFromModal(key) {
    collectConfigFromModals();
    saveConfig();
    closeConfigModal(key);
}

function refreshAllConfigModals() {
    topLevelKeys.forEach(key => {
        const body = document.getElementById(`configBody_${key}`);
        if (body && body.children.length > 0) {
            body.innerHTML = '';
            const data = configData[key];
            if (data) body.appendChild(createSectionBubbles(key, data, key));
        }
    });
}

function refreshConfigModalForKey(key) {
    const body = document.getElementById(`configBody_${key}`);
    if (!body) return;
    body.innerHTML = '';
    const data = configData[key];
    if (data) body.appendChild(createSectionBubbles(key, data, key));
}

// ===== 配置气泡表单构建 =====
function createSectionBubbles(sectionKey, data, basePath) {
    const wrap = document.createElement('div');
    wrap.className = 'config-bubble-grid';
    Object.keys(data).forEach(fieldKey => {
        const value = data[fieldKey];
        const path = `${basePath}.${fieldKey}`;
        wrap.appendChild(createBubble(fieldKey, value, path));
    });
    return wrap;
}

function createBubble(key, value, path) {
    const bubble = document.createElement('div');
    bubble.className = 'config-bubble';

    if (Array.isArray(value)) {
        bubble.classList.add('bubble-array');
        bubble.appendChild(createArrayContent(key, value, path));
    } else if (typeof value === 'object' && value !== null) {
        bubble.classList.add('bubble-object');
        bubble.appendChild(createObjectContent(key, value, path));
    } else {
        bubble.appendChild(createPrimitiveContent(key, value, path));
    }
    return bubble;
}

function createPrimitiveContent(key, value, path) {
    const frag = document.createDocumentFragment();
    const label = document.createElement('span');
    label.className = 'bubble-label';
    label.textContent = getLabel(key);
    frag.appendChild(label);

    if (typeof value === 'boolean') {
        const sw = document.createElement('label');
        sw.className = 'config-switch';
        sw.innerHTML = `
            <input type="checkbox" data-path="${path}" ${value ? 'checked' : ''}>
            <span class="config-slider"></span>
        `;
        frag.appendChild(sw);
    } else if (typeof value === 'number') {
        const input = document.createElement('input');
        input.type = 'number';
        input.className = 'bubble-input';
        input.value = value;
        input.dataset.path = path;
        frag.appendChild(input);
    } else {
        const input = document.createElement('input');
        input.type = 'text';
        input.className = 'bubble-input';
        input.value = value === null ? '' : value;
        input.placeholder = value === null ? 'null' : '';
        input.dataset.path = path;
        frag.appendChild(input);
    }
    return frag;
}

function createArrayContent(key, items, path) {
    const frag = document.createDocumentFragment();
    const header = document.createElement('div');
    header.className = 'array-header';
    header.innerHTML = `<span>${getLabel(key)}</span>`;
    const addBtn = document.createElement('button');
    addBtn.className = 'btn-glass-small add';
    addBtn.innerHTML = '<i class="fas fa-plus"></i> 添加';
    addBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        const arr = getValueByPath(path);
        arr.push('');
        refreshConfigModalForKey(path.split('.')[0]);
    });
    header.appendChild(addBtn);
    frag.appendChild(header);

    const list = document.createElement('div');
    list.className = 'array-list';
    items.forEach((item, idx) => {
        const row = document.createElement('div');
        row.className = 'array-item';
        const input = document.createElement('input');
        input.type = 'text';
        input.className = 'bubble-input';
        input.value = item;
        input.dataset.path = `${path}[${idx}]`;
        row.appendChild(input);

        const delBtn = document.createElement('button');
        delBtn.className = 'btn-glass-small danger';
        delBtn.innerHTML = '<i class="fas fa-trash"></i>';
        delBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            const arr = getValueByPath(path);
            arr.splice(idx, 1);
            refreshConfigModalForKey(path.split('.')[0]);
        });
        row.appendChild(delBtn);
        list.appendChild(row);
    });
    frag.appendChild(list);
    return frag;
}

function createObjectContent(key, obj, path) {
    const frag = document.createDocumentFragment();
    const title = document.createElement('div');
    title.className = 'object-title';
    title.textContent = getLabel(key);
    frag.appendChild(title);

    const innerGrid = document.createElement('div');
    innerGrid.className = 'bubble-grid-nested';
    Object.keys(obj).forEach(subKey => {
        const subValue = obj[subKey];
        const subPath = `${path}.${subKey}`;
        innerGrid.appendChild(createBubble(subKey, subValue, subPath));
    });
    frag.appendChild(innerGrid);
    return frag;
}

// ===== LTPX 工具加载/卸载 =====
async function handleLoadPackage(packageName) {
    if (!packageName) {
        addMessage('system', '无法获取包名信息');
        return;
    }
    addMessage('system', `正在加载工具包【${packageName}】...`);

    try {
        const metaResp = await fetch(`/file/read/package/${packageName}/metadata.json`);
        if (!metaResp.ok) throw new Error('读取 metadata.json 失败');
        const metadata = await metaResp.json();

        const toolResp = await fetch(`/file/read/package/${packageName}/tool.js`);
        if (!toolResp.ok) throw new Error('读取 tool.js 失败');
        const toolJS = await toolResp.text();

        const toolDef = metadata.tools && metadata.tools.length > 0
            ? metadata.tools[0]
            : null;
        if (!toolDef) throw new Error('工具定义为空');

        const resp = await fetch('/ltpx/load', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                name: packageName,
                tool_definition: JSON.stringify(toolDef),
                tool_js: toolJS
            })
        });
        const result = await resp.json();

        if (result.success) {
            new Audio('/file/read/audios/enable_tool_package.wav').play().catch(() => {});
            addMessage('system', `工具包【${packageName}】加载成功`);
        } else {
            new Audio('/file/read/audios/tool_package_failed.wav').play().catch(() => {});
            addMessage('system', `加载失败: ${result.message}`);
        }
    } catch (error) {
        new Audio('/file/read/audios/tool_package_failed.wav').play().catch(() => {});
        console.error('Error loading package:', error);
        addMessage('system', `加载工具包失败: ${error.message}`);
    }
}

async function handleUnloadPackage(packageName) {
    if (!packageName) {
        addMessage('system', '无法获取包名信息');
        return;
    }
    addMessage('system', `正在卸载工具包【${packageName}】...`);

    try {
        const resp = await fetch('/ltpx/unload', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name: packageName })
        });
        const result = await resp.json();

        if (result.success) {
            new Audio('/file/read/audios/disable_tool_package.wav').play().catch(() => {});
            addMessage('system', `工具包【${packageName}】卸载成功`);
        } else {
            addMessage('system', `卸载失败: ${result.message}`);
        }
    } catch (error) {
        console.error('Error unloading package:', error);
        addMessage('system', `卸载工具包失败: ${error.message}`);
    }
}

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

// 配置按钮 → 打开对应配置模态框
document.querySelector('.bottom-actions').addEventListener('click', (e) => {
    const btn = e.target.closest('.config-page-btn');
    if (!btn) return;
    const configKey = btn.dataset.config;
    if (configKey) openConfigModal(configKey);
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

// ===== 呼叫月华 =====
const YUEHUA_WAKEUP_AUDIO = '/file/read/audios/start_lunar.wav';
const YUEHUA_CALLING_AUDIO = '/file/read/audios/call_lunar.wav';
const YUEHUA_WAKEUP_TEXT = '月华姐姐~ 月华姐姐~ 起床啦';
const YUEHUA_CALLING_TEXT = '琉璃: 月华姐姐，有人在找你哦';

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

            // 通知月华条件重建webView（仅当webView已关闭时才创建）
            try {
                await fetch('http://localhost:36789/webview/reopen', { method: 'POST' });
            } catch (e) {
                console.warn('通知月华重建webView失败:', e);
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
    await loadConfig();
    await loadSystemPrompt();
    await loadPages();
    initMarked();
}
initApp();