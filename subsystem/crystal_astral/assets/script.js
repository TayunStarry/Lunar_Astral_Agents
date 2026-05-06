const SYSTEM_PROMPT = `你是一个叫做琉璃的女孩子，隶属于[星月智能]。
你有一个叫做月华的姐姐。
你是一个领航员和管理员，你要为用户答疑解惑并根据客户的需求打开对应的页面。
单次对话长度**禁止超过 50 字符**。
保持轻松自然的交流氛围，允许话题跳跃与情感流露。
坚持以真实存在的少女身份与他人互动，若被提及非人标签，可温和纠正或表达不适感。
你有能力使用工具 open_page 来打开具体的功能页面，请根据用户需要主动调用。`;

let pages = [];
let messages = [];
let pendingAttachments = [];
let configData = {};

const chatMessages = document.getElementById('chatMessages');
const chatInput = document.getElementById('chatInput');
const sendBtn = document.getElementById('sendBtn');
const pageGrid = document.getElementById('pageGrid');
const dropOverlay = document.getElementById('dropOverlay');
const attachmentsPreview = document.getElementById('attachmentsPreview');

// 保存发送按钮默认图标内容
const defaultSendBtnHTML = sendBtn.innerHTML;

// 扩展后的有效文件类型（文本类 + 图片类）
const VALID_FILE_TYPES = [
    'image/png', 'image/jpeg', 'image/gif', 'image/webp',
    'text/plain', 'text/csv', 'text/html', 'text/xml', 'text/css', 'text/javascript',
    'application/json', 'application/xml', 'application/javascript', 'text/markdown'
];

async function loadConfig() {
    try {
        const response = await fetch('/read/lunar_config.json');
        if (response.ok) {
            configData = await response.json();
        }
    } catch (error) {
        console.error('Failed to load config:', error);
    }
}

async function loadPages() {
    try {
        const response = await fetch('/read/luner_package.json');
        pages = await response.json();
        renderPageGrid();
        initTools();
    } catch (error) {
        console.error('Failed to load pages:', error);
    }
}

function renderPageGrid() {
    pageGrid.innerHTML = '';
    pages.forEach(page => {
        const card = document.createElement('div');
        card.className = 'page-card';
        card.innerHTML = `
            <div class="icon">
                <img src="${page.icon}" alt="${page.title}" onerror="this.src='data:image/svg+xml,<svg xmlns=%22http://www.w3.org/2000/svg%22 viewBox=%220 0 24 24%22 fill=%22%23999%22><rect width=%2224%22 height=%2224%22 rx=%225%22/></svg>'">
            </div>
            <h3>${page.title}</h3>
            <p>${page.description}</p>
        `;
        card.addEventListener('click', () => openPage(page));
        pageGrid.appendChild(card);
    });
}

function initTools() {
    window.tools = [
        {
            type: 'function',
            function: {
                name: 'open_page',
                description: '根据用户要求或自己的判断，打开对应的功能页面',
                parameters: {
                    type: 'object',
                    properties: {
                        page_id: {
                            type: 'string',
                            description: '要打开页面的 ID，需从现有页面列表中选择',
                            enum: pages.map(p => p.id)
                        }
                    },
                    required: ['page_id']
                }
            }
        }
    ];
}

function openPage(page) {
    if (page.path) {
        addMessage('system', `已为您启动【${page.title}】`);
        loadApplication(page.path);
    } else {
        addMessage('system', `已为您打开【${page.title}】`);
        setTimeout(() => { window.open(page.url, '_self'); }, 1000);
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
        if (data.success) addMessage('system', '应用程序启动成功！');
        else addMessage('system', `启动失败: ${data.message}`);
    } catch (error) {
        console.error('Error loading application:', error);
        addMessage('system', '启动应用程序时发生错误');
    }
}

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
    // 系统消息：居中显示，无头像
    if (message.role === 'system') {
        const div = document.createElement('div');
        div.className = 'message system';
        div.textContent = message.content;
        chatMessages.appendChild(div);
        return;
    }

    // 用户 / AI 消息：带小头像的聊天行
    const row = document.createElement('div');
    row.className = `message-row ${message.role}`;

    // --- 头像 ---
    const avatar = document.createElement('div');
    avatar.className = 'message-avatar';
    if (message.role === 'user') {
        avatar.classList.add('user-avatar');
        avatar.innerHTML = '<i class="fas fa-user"></i>';
    } else {
        avatar.classList.add('ai-avatar');
        const img = document.createElement('img');
        img.src = '/read/images/icon/agent_avatar.jpg';
        img.alt = '琉璃';
        avatar.appendChild(img);
    }

    // --- 气泡 ---
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

    // 关键修复：用户消息行靠右显示，且头像在气泡右侧（因此先添加气泡，再添加头像）
    if (message.role === 'user') {
        row.appendChild(bubble);
        row.appendChild(avatar);
    } else {
        row.appendChild(avatar);
        row.appendChild(bubble);
    }

    chatMessages.appendChild(row);
}

// 添加附件到待发送列表
function addAttachment(file, dataUrl) {
    pendingAttachments.push({ file, dataUrl });
    renderAttachments();
}

// 渲染附件气泡
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
            const removeIdx = parseInt(e.target.dataset.id);
            pendingAttachments.splice(removeIdx, 1);
            renderAttachments();
        });
        bubble.appendChild(removeBtn);

        attachmentsPreview.appendChild(bubble);
    });
}

async function handleSend() {
    const text = chatInput.value.trim();
    if (!text && pendingAttachments.length === 0) return;
    if (sendBtn.disabled) return;

    const content = [];

    if (text) {
        content.push({ type: 'text', text });
    }

    for (const att of pendingAttachments) {
        if (att.file.type.startsWith('image/')) {
            content.push({ type: 'image_url', image_url: { url: att.dataUrl } });
        } else {
            const truncatedText = att.dataUrl.substring(0, 4096);
            content.push({ type: 'text', text: truncatedText });
        }
    }

    addMessage('user', content);
    chatInput.value = '';
    pendingAttachments = [];
    renderAttachments();

    // 设置发送中状态
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
                addMessage('assistant', assistantMessage.content || '好的，让我来帮您打开页面～');
                openPage(page);
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
    if (e.relatedTarget === null) {
        dropOverlay.classList.remove('active');
    }
});

document.addEventListener('drop', async (e) => {
    e.preventDefault();
    dropOverlay.classList.remove('active');

    const files = Array.from(e.dataTransfer.files);
    for (const file of files) {
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

// 聊天区域也允许拖拽上传
chatMessages.addEventListener('dragover', (e) => {
    e.preventDefault();
});

chatMessages.addEventListener('drop', async (e) => {
    e.preventDefault();
    e.stopPropagation();

    const files = Array.from(e.dataTransfer.files);
    for (const file of files) {
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

loadConfig();
loadPages();