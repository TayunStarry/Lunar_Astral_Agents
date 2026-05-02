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

const chatMessages = document.getElementById('chatMessages');
const chatInput = document.getElementById('chatInput');
const sendBtn = document.getElementById('sendBtn');
const pageGrid = document.getElementById('pageGrid');
const dropOverlay = document.getElementById('dropOverlay');
const attachmentsPreview = document.getElementById('attachmentsPreview');

async function loadPages() {
    try {
        const response = await fetch('./pages.json');
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
    addMessage('system', `已为您打开【${page.title}】`);
    setTimeout(() => {
        window.open(page.url, '_self');
    }, 1000);
}

function addMessage(role, content) {
    const message = { role, content };
    messages.push(message);

    if (messages.length > 10) {
        messages.shift();
        chatMessages.removeChild(chatMessages.firstChild);
    }

    renderMessage(message);
    chatMessages.scrollTop = chatMessages.scrollHeight;
}

function renderMessage(message) {
    const div = document.createElement('div');
    div.className = `message ${message.role}`;

    if (Array.isArray(message.content)) {
        message.content.forEach(item => {
            if (item.type === 'text') {
                div.appendChild(document.createTextNode(item.text));
            } else if (item.type === 'image_url') {
                const img = document.createElement('img');
                img.src = item.image_url.url;
                img.alt = 'Uploaded image';
                img.addEventListener('click', () => window.open(img.src, '_blank'));
                div.appendChild(img);
            }
        });
    } else {
        div.textContent = message.content;
    }

    chatMessages.appendChild(div);
}

function addAttachment(file, dataUrl) {
    const bubble = document.createElement('div');
    bubble.className = 'attachment-bubble';

    let icon = '📄';
    if (file.type.startsWith('image/')) {
        icon = '🖼️';
    }

    bubble.innerHTML = `
        <span>${icon}</span>
        <span title="${file.name}">${file.name.length > 15 ? file.name.substring(0, 12) + '...' : file.name}</span>
        <span class="remove-btn" data-id="${pendingAttachments.length}">✕</span>
    `;

    pendingAttachments.push({ file, dataUrl });
    attachmentsPreview.appendChild(bubble);

    bubble.querySelector('.remove-btn').addEventListener('click', (e) => {
        const idx = parseInt(e.target.dataset.id);
        pendingAttachments.splice(idx, 1);
        renderAttachments();
    });
}

function renderAttachments() {
    attachmentsPreview.innerHTML = '';
    pendingAttachments.forEach((att, idx) => {
        const bubble = document.createElement('div');
        bubble.className = 'attachment-bubble';

        let icon = '📄';
        if (att.file.type.startsWith('image/')) {
            icon = '🖼️';
        }

        bubble.innerHTML = `
            <span>${icon}</span>
            <span title="${att.file.name}">${file.name.length > 15 ? att.file.name.substring(0, 12) + '...' : att.file.name}</span>
            <span class="remove-btn" data-id="${idx}">✕</span>
        `;

        bubble.querySelector('.remove-btn').addEventListener('click', (e) => {
            const removeIdx = parseInt(e.target.dataset.id);
            pendingAttachments.splice(removeIdx, 1);
            renderAttachments();
        });

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
            content.push({ type: 'text', text: att.dataUrl });
        }
    }

    addMessage('user', content);
    chatInput.value = '';
    pendingAttachments = [];
    renderAttachments();

    sendBtn.disabled = true;
    sendBtn.classList.add('loading');
    sendBtn.textContent = '';

    const allMessages = [{ role: 'system', content: SYSTEM_PROMPT }, ...messages];

    try {
        const response = await fetch('/v1/chat/completions', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                model: 'system-multimodal',
                messages: allMessages,
                tools: window.tools
            })
        });

        if (!response.ok) {
            throw new Error('Network response was not ok');
        }

        const data = await response.json();
        const assistantMessage = data.choices[0].message;

        if (assistantMessage.tool_calls && assistantMessage.tool_calls.length > 0) {
            const toolCall = assistantMessage.tool_calls[0];
            if (toolCall.function.name === 'open_page') {
                const args = JSON.parse(toolCall.function.arguments);
                const page = pages.find(p => p.id === args.page_id);
                if (page) {
                    addMessage('assistant', assistantMessage.content || '好的，让我来帮您打开页面～');
                    openPage(page);
                }
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
        sendBtn.textContent = '发送';
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
    const validTypes = ['text/plain', 'image/png', 'image/jpeg', 'image/gif', 'image/webp'];

    for (const file of files) {
        if (!validTypes.includes(file.type)) {
            addMessage('system', `不支持的文件类型: ${file.name}`);
            continue;
        }

        if (file.type.startsWith('image/')) {
            const reader = new FileReader();
            reader.onload = (e) => {
                addAttachment(file, e.target.result);
            };
            reader.readAsDataURL(file);
        } else {
            const text = await file.text();
            addAttachment(file, text);
        }
    }
});

chatMessages.addEventListener('dragover', (e) => {
    e.preventDefault();
});

chatMessages.addEventListener('drop', async (e) => {
    e.preventDefault();
    e.stopPropagation();

    const files = Array.from(e.dataTransfer.files);
    const validTypes = ['text/plain', 'image/png', 'image/jpeg', 'image/gif', 'image/webp'];

    for (const file of files) {
        if (!validTypes.includes(file.type)) {
            addMessage('system', `不支持的文件类型: ${file.name}`);
            continue;
        }

        if (file.type.startsWith('image/')) {
            const reader = new FileReader();
            reader.onload = (e) => {
                addAttachment(file, e.target.result);
            };
            reader.readAsDataURL(file);
        } else {
            const text = await file.text();
            addAttachment(file, text);
        }
    }
});

loadPages();
