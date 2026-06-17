let SYSTEM_PROMPT = '';
let pages = [];
let messages = [];
let pendingAttachments = [];
let configData = {};

const pageGrid = document.getElementById('pageGrid');
const crystalBtn = document.getElementById('crystalBtn');
const chatModal = document.getElementById('chatModal');
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

let currentPackageName = null;
const defaultSendBtnHTML = sendBtn.innerHTML;

const VALID_FILE_TYPES = [
    'image/png', 'image/jpeg', 'image/gif', 'image/webp',
    'text/plain', 'text/csv', 'text/html', 'text/xml', 'text/css', 'text/javascript',
    'application/json', 'application/xml', 'application/javascript', 'text/markdown'
];

const PACKAGE_FILE_EXTENSIONS = ['.ltpx', '.ltp2'];

// ===== 初始化 =====
async function loadConfig() {
    try {
        const response = await fetch('/file/read/lunar_config.json');
        if (response.ok) configData = await response.json();
    } catch (error) { console.error('Failed to load config:', error); }
}

/**
 * 加载琉璃系统提示词
 * 从独立 Markdown 文件动态载入并执行占位符置换
 */
async function loadSystemPrompt() {
    try {
        const response = await fetch('/liuli_system_prompt.md');
        if (!response.ok) throw new Error('加载系统提示词失败');
        const raw = await response.text();
        SYSTEM_PROMPT = processSystemPrompt(raw);
    } catch (error) {
        console.error('Failed to load system prompt:', error);
        // 兜底提示词
        SYSTEM_PROMPT = '你是琉璃，星月智能的领航员。帮助用户定位功能页面，使用 open_page 工具。';
    }
}

/**
 * 占位符置换函数
 * 将系统提示词中的占位符替换为运行时值
 * 支持的占位符：
 *   {{current-address}} - 当前地址
 *   {{current-time}}    - 当前时间
 *   {{page-count}}      - 当前可用页面数量
 */
function processSystemPrompt(raw) {
    let result = raw;
    // 替换当前地址
    if (configData?.current_address) {
        result = result.replace(/\{\{current-address\}\}/g, configData.current_address);
    }
    // 替换当前时间
    result = result.replace(/\{\{current-time\}\}/g, new Date().toLocaleString('zh-CN'));
    // 替换页面数量
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
function renderPageGrid() {
    pageGrid.innerHTML = '';
    pages.forEach(page => {
        const hasLTPX = page.tags && page.tags.includes('LTPX');

        const card = document.createElement('div');
        card.className = 'page-card';
        card.dataset.pageId = page.id;
        card.innerHTML = `
            <div class="icon">
                <img src="${page.icon}" alt="${page.title}" onerror="this.src='data:image/svg+xml,<svg xmlns=%22http://www.w3.org/2000/svg%22 viewBox=%220 0 24 24%22 fill=%22%23999%22><rect width=%2224%22 height=%2224%22 rx=%225%22/></svg>'">
            </div>
            <h3>${page.title}</h3>
            <p>${page.description}</p>
            <button class="card-btn card-btn-export" title="导出包" data-action="export" data-package="${page.package_name || ''}">
                <i class="fas fa-box"></i>
            </button>
            <button class="card-btn card-btn-delete" title="删除包" data-action="delete" data-package="${page.package_name || ''}">
                <i class="fas fa-trash-alt"></i>
            </button>
            ${hasLTPX ? `
            <button class="card-btn card-btn-load" title="加载包" data-action="load" data-package="${page.package_name || ''}">
                <i class="fas fa-download"></i>
            </button>
            <button class="card-btn card-btn-unload" title="卸载包" data-action="unload" data-package="${page.package_name || ''}">
                <i class="fas fa-upload"></i>
            </button>
            ` : ''}
        `;

        card.addEventListener('click', (e) => {
            if (e.target.closest('.card-btn')) return;
            openPage(page);
        });

        pageGrid.appendChild(card);
    });

    // 绑定卡片按钮事件
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
    window.tools = [{
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
    }];
}

// ===== 页面打开（立即跳转，无延迟） =====
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

// ===== 搜索定位：跳转到卡片并高亮 =====
function locateAndHighlightCard(pageId) {
    const card = document.querySelector(`.page-card[data-page-id="${pageId}"]`);
    if (!card) return;

    // 如果聊天模态框处于交互状态（已打开），执行平滑渐隐
    if (chatModal.classList.contains('active')) {
        fadeOutChatModal();
    }

    // 滚动到卡片位置
    card.scrollIntoView({ behavior: 'smooth', block: 'center' });

    // 添加高亮动画
    card.classList.remove('highlight');
    void card.offsetWidth; // 触发重排
    card.classList.add('highlight');

    // 3秒后移除高亮
    setTimeout(() => card.classList.remove('highlight'), 3000);
}

/**
 * 平滑渐隐关闭聊天模态框
 * 0.3 秒渐隐动画后移除 active 状态
 */
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

    if (Array.isArray(message.content)) {
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

    const allMessages = [{ role: 'system', content: SYSTEM_PROMPT }, ...messages];

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
            const toolCall = assistantMessage.tool_calls[0];
            if (toolCall.function.name === 'open_page') {
                const args = JSON.parse(toolCall.function.arguments);
                const page = pages.find(p => p.id === args.page_id);
                if (!page) return;
                addMessage('assistant', assistantMessage.content || '好的，让我来帮您定位到这个应用～');
                // 定位到卡片并高亮，而非直接打开
                locateAndHighlightCard(args.page_id);
            }
        } else {
            addMessage('assistant', assistantMessage.content);
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

// ===== LTPX 工具加载/卸载 =====
async function handleLoadPackage(packageName) {
    if (!packageName) {
        addMessage('system', '无法获取包名信息');
        return;
    }
    addMessage('system', `正在加载工具包【${packageName}】...`);

    try {
        // 读取 metadata.json 获取工具定义
        const metaResp = await fetch(`/file/read/package/${packageName}/metadata.json`);
        if (!metaResp.ok) throw new Error('读取 metadata.json 失败');
        const metadata = await metaResp.json();

        // 读取 tool.js 获取工具实现
        const toolResp = await fetch(`/file/read/package/${packageName}/tool.js`);
        if (!toolResp.ok) throw new Error('读取 tool.js 失败');
        const toolJS = await toolResp.text();

        // 提取工具定义（取第一个工具的 function 定义）
        const toolDef = metadata.tools && metadata.tools.length > 0
            ? metadata.tools[0]
            : null;
        if (!toolDef) throw new Error('工具定义为空');

        // 发送加载请求到 lunar_astral (36789)
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
            addMessage('system', `工具包【${packageName}】加载成功`);
        } else {
            addMessage('system', `加载失败: ${result.message}`);
        }
    } catch (error) {
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

// ===== 启动 =====
async function initApp() {
    await loadConfig();
    await loadSystemPrompt();
    await loadPages();
}
initApp();