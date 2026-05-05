const labelMap = {
    'models': '模型配置',
    'server': '服务器配置',
    'cloud': '云端配置',
    'qq_adapter': 'QQ适配器',
    'project_archiving': '项目归档',
    'multimodal_model': '多模态模型',
    'mmproj_model': '多模态投影模型',
    'embedding_model': '嵌入模型',
    'diffusion_model': '扩散模型',
    'variational_model': '变分模型',
    'prompt_refine_model': '提示词精炼模型',
    'tts_url': 'TTS服务地址',
    'developer': '开发者模式',
    'clear_port': '清理端口',
    'allow_diffusion': '允许扩散',
    'allow_multimodal': '允许多模态',
    'multimodalModelUrl': '多模态模型地址',
    'multimodalModelName': '多模态模型名称',
    'multimodalModelKey': '多模态模型密钥',
    'userName': '用户名',
    'embeddingModelUrl': '嵌入模型地址',
    'embeddingModelName': '嵌入模型名称',
    'embeddingModelKey': '嵌入模型密钥',
    'napcat_ws_server': 'Napcat WS服务器',
    'napcat_ws_token': 'Napcat WS令牌',
    'lunar_core_url': 'Lunar Core地址',
    'lunar_ws_server': 'Lunar WS服务器',
    'poll_interval': '轮询间隔',
    'listen_group_ids': '监听群组ID',
    'trigger_keywords': '触发关键词',
    'display_logs': '显示日志',
    'default_reply': '默认回复',
    'package_levels': '打包层级',
    'sevenzip_paths': '7z路径',
    'defaults': '默认设置',
    'output_path': '输出路径',
    'part_size_mb': '分卷大小(MB)',
    'compression_level': '压缩级别',
    'package_level': '打包层级',
    'name': '名称',
    'description': '描述',
    'sources': '源文件'
};

let configData = null;
let originalConfig = null;
let aiMessages = [];
let pendingConfigChanges = null;
const MAX_CONTEXT_MESSAGES = 15;

const cardIcons = {
    'models': '<i class="fas fa-robot"></i>',
    'server': '<i class="fas fa-server"></i>',
    'cloud': '<i class="fas fa-cloud"></i>',
    'qq_adapter': '<i class="fab fa-qq"></i>',
    'project_archiving': '<i class="fas fa-box"></i>'
};

function getLabel(key) {
    return labelMap[key] || key;
}

function encodeFileName(filename) {
    return btoa(unescape(encodeURIComponent(filename)));
}

async function loadConfig() {
    try {
        const response = await fetch('/read/lunar_config.json');
        if (!response.ok) throw new Error('加载配置失败');
        configData = await response.json();
        originalConfig = JSON.parse(JSON.stringify(configData));
        renderAllPages();
    } catch (error) {
        console.error('加载配置失败:', error);
        alert('加载配置失败，请检查服务是否正常运行');
    }
}

async function saveConfig() {
    try {
        collectConfig();
        const jsonString = JSON.stringify(configData, null, '\t');
        const blob = new Blob([jsonString], { type: 'application/json' });
        
        const response = await fetch('/save', {
            method: 'POST',
            headers: {
                'X-File-Name': encodeFileName('lunar_config.json'),
                'X-Overwrite': 'true'
            },
            body: blob
        });
        
        if (!response.ok) throw new Error('保存配置失败');
        originalConfig = JSON.parse(JSON.stringify(configData));
        alert('保存成功！');
    } catch (error) {
        console.error('保存配置失败:', error);
        alert('保存配置失败，请检查服务是否正常运行');
    }
}

function resetConfig() {
    if (originalConfig) {
        configData = JSON.parse(JSON.stringify(originalConfig));
        renderAllPages();
    }
}

function renderAllPages() {
    renderBasicPage();
    renderQQPage();
    renderArchivePage();
}

function renderBasicPage() {
    const grid = document.getElementById('grid-basic');
    grid.innerHTML = '';
    
    const sections = ['models', 'server', 'cloud'];
    
    sections.forEach(key => {
        const card = createCard(key);
        grid.appendChild(card);
        renderSection(card.querySelector('.config-fields'), configData[key], key);
    });
}

function renderQQPage() {
    const container = document.getElementById('grid-qq');
    container.innerHTML = '';
    
    const card = createCard('qq_adapter');
    container.appendChild(card);
    renderSection(card.querySelector('.config-fields'), configData['qq_adapter'], 'qq_adapter');
}

function renderArchivePage() {
    const container = document.getElementById('grid-archive');
    container.innerHTML = '';
    
    const card = createCard('project_archiving');
    card.classList.add('full-width');
    container.appendChild(card);
    renderSection(card.querySelector('.config-fields'), configData['project_archiving'], 'project_archiving');
}

function createCard(sectionKey) {
    const card = document.createElement('div');
    card.className = 'config-card';
    card.innerHTML = `
        <h3>${cardIcons[sectionKey] || '📋'} ${getLabel(sectionKey)}</h3>
        <div class="config-fields" data-section="${sectionKey}">
        </div>
    `;
    return card;
}

function renderSection(container, data, path) {
    if (!data) return;
    
    Object.keys(data).forEach(key => {
        const value = data[key];
        const fieldPath = path ? `${path}.${key}` : key;
        
        if (typeof value === 'boolean') {
            container.appendChild(createSwitchField(key, value, fieldPath));
        } else if (typeof value === 'number') {
            container.appendChild(createNumberField(key, value, fieldPath));
        } else if (typeof value === 'string' || value === null) {
            container.appendChild(createTextField(key, value, fieldPath));
        } else if (Array.isArray(value)) {
            container.appendChild(createArrayField(key, value, fieldPath));
        } else if (typeof value === 'object') {
            container.appendChild(createObjectField(key, value, fieldPath));
        }
    });
}

function createSwitchField(key, value, path) {
    const div = document.createElement('div');
    div.className = 'field-group';
    div.innerHTML = `
        <div class="field-switch">
            <label class="switch">
                <input type="checkbox" data-path="${path}" ${value ? 'checked' : ''}>
                <span class="slider"></span>
            </label>
            <span class="field-label">${getLabel(key)}</span>
        </div>
    `;
    return div;
}

function createNumberField(key, value, path) {
    const div = document.createElement('div');
    div.className = 'field-group';
    div.innerHTML = `
        <label class="field-label">${getLabel(key)}</label>
        <input type="number" class="field-input" data-path="${path}" value="${value}">
    `;
    return div;
}

function createTextField(key, value, path) {
    const div = document.createElement('div');
    div.className = 'field-group';
    div.innerHTML = `
        <label class="field-label">${getLabel(key)}</label>
        <input type="text" class="field-input" data-path="${path}" value="${value === null ? '' : value}" placeholder="null">
    `;
    return div;
}

function createArrayField(key, value, path) {
    const div = document.createElement('div');
    div.className = 'field-group';
    div.innerHTML = `
        <label class="field-label">${getLabel(key)}</label>
        <div class="field-array" data-path="${path}">
        </div>
    `;
    
    const arrayContainer = div.querySelector('.field-array');
    value.forEach((item, index) => {
        const itemDiv = document.createElement('div');
        itemDiv.className = 'array-item';
        itemDiv.innerHTML = `
            <input type="text" class="field-input" data-path="${path}[${index}]" value="${item}">
            <button class="btn btn-small" onclick="removeArrayItem(this, '${path}', ${index})">删除</button>
        `;
        arrayContainer.appendChild(itemDiv);
    });
    
    const addBtn = document.createElement('button');
    addBtn.className = 'btn btn-small';
    addBtn.textContent = '+ 添加';
    addBtn.onclick = () => addArrayItem(path);
    arrayContainer.appendChild(addBtn);
    
    return div;
}

function addArrayItem(path) {
    const arr = getValueByPath(path);
    arr.push('');
    renderAllPages();
}

function removeArrayItem(btn, path, index) {
    const arr = getValueByPath(path);
    arr.splice(index, 1);
    renderAllPages();
}

function createObjectField(key, value, path) {
    const div = document.createElement('div');
    div.className = 'sub-section';
    div.innerHTML = `
        <div class="sub-section-title">${getLabel(key)}</div>
        <div class="sub-fields"></div>
    `;
    
    const subFields = div.querySelector('.sub-fields');
    
    if (key === 'package_levels') {
        Object.keys(value).forEach(levelKey => {
            const levelData = value[levelKey];
            const nestedCard = document.createElement('div');
            nestedCard.className = 'nested-card';
            nestedCard.innerHTML = `<div class="nested-card-title">层级 ${levelKey}: ${levelData.name}</div>`;
            renderSection(nestedCard, levelData, `${path}.${levelKey}`);
            subFields.appendChild(nestedCard);
        });
    } else {
        renderSection(subFields, value, path);
    }
    
    return div;
}

function getValueByPath(path) {
    const parts = path.replace(/\[(\d+)\]/g, '.$1').split('.');
    let current = configData;
    for (let i = 0; i < parts.length; i++) {
        current = current[parts[i]];
    }
    return current;
}

function setValueByPath(path, value) {
    const parts = path.replace(/\[(\d+)\]/g, '.$1').split('.');
    let current = configData;
    for (let i = 0; i < parts.length - 1; i++) {
        current = current[parts[i]];
    }
    current[parts[parts.length - 1]] = value;
}

function collectConfig() {
    document.querySelectorAll('[data-path]').forEach(input => {
        const path = input.dataset.path;
        if (!path) return;
        
        if (input.type === 'checkbox') {
            setValueByPath(path, input.checked);
        } else if (input.type === 'number') {
            setValueByPath(path, parseFloat(input.value));
        } else if (input.tagName === 'INPUT' && input.type === 'text') {
            const value = input.value === '' ? null : input.value;
            setValueByPath(path, value);
        }
    });
}

function addUserMessage(content) {
    const messagesDiv = document.getElementById('aiMessages');
    const div = document.createElement('div');
    div.className = 'message user-message';
    div.innerHTML = `
        <div class="message-avatar">你</div>
        <div class="message-content">${marked.parse(escapeHtml(content))}</div>
    `;
    messagesDiv.appendChild(div);
    messagesDiv.scrollTop = messagesDiv.scrollHeight;
}

function addAiMessage(content) {
    const messagesDiv = document.getElementById('aiMessages');
    const div = document.createElement('div');
    div.className = 'message ai-message';
    div.innerHTML = `
        <div class="message-avatar"><img src="/icon/agent_avatar.jpg" alt="Agent Avatar"></div>
        <div class="message-content">${marked.parse(content)}</div>
    `;
    messagesDiv.appendChild(div);
    messagesDiv.scrollTop = messagesDiv.scrollHeight;
}

function addTypingIndicator() {
    const messagesDiv = document.getElementById('aiMessages');
    const div = document.createElement('div');
    div.className = 'message ai-message';
    div.id = 'typingIndicator';
    div.innerHTML = `
        <div class="message-avatar"><img src="/icon/agent_avatar.jpg" alt="Agent Avatar"></div>
        <div class="message-content">正在思考...<span class="typing-dots"></span></div>
    `;
    messagesDiv.appendChild(div);
    messagesDiv.scrollTop = messagesDiv.scrollHeight;
}

function removeTypingIndicator() {
    const indicator = document.getElementById('typingIndicator');
    if (indicator) indicator.remove();
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

async function sendToAI() {
    const input = document.getElementById('aiInput');
    const content = input.value.trim();
    if (!content) return;
    
    addUserMessage(content);
    input.value = '';
    addTypingIndicator();
    
    aiMessages.push({ role: 'user', content: content });
    
    if (aiMessages.length > MAX_CONTEXT_MESSAGES) {
        aiMessages = aiMessages.slice(-MAX_CONTEXT_MESSAGES);
    }
    
    try {
        const apiUrl = `${window.location.origin}/v1/chat/completions`;
        
        const systemPrompt = `你叫琉璃，一个温柔可爱、善解人意的少女助手。你的姐姐是月华。你的任务是辅助用户完成配置文件的定义和修改。
你需要根据用户的问题，结合以下配置文件内容给出建议，并且可以建议如何修改配置。

当前配置文件内容：
${JSON.stringify(configData, null, 2)}

如果用户要求修改配置，请在回复中包含一个JSON格式的配置变更，用\`\`\`json\`\`\`和\`\`\`包裹起来。例如：
\`\`\`json
{
  "server": {
    "port": 8080
  }
}
\`\`\`

请用友好、活泼的语气回复，就像和朋友聊天一样。`;
        
        const messages = [
            { role: 'system', content: systemPrompt },
            ...aiMessages
        ];
        
        const response = await fetch(apiUrl, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                model: 'system-multimodal',
                messages: messages,
                stream: false
            })
        });
        
        if (!response.ok) throw new Error('AI请求失败');
        
        const result = await response.json();
        let aiContent = result.choices[0].message.content;
        
        removeTypingIndicator();
        addAiMessage(aiContent);
        aiMessages.push({ role: 'assistant', content: aiContent });
        
        if (aiMessages.length > MAX_CONTEXT_MESSAGES) {
            aiMessages = aiMessages.slice(-MAX_CONTEXT_MESSAGES);
        }
        
        const jsonMatch = aiContent.match(/```json([\s\S]*?)```/);
        if (jsonMatch) {
            try {
                const modifiedConfig = JSON.parse(jsonMatch[1].trim());
                const originalSnapshot = JSON.parse(JSON.stringify(configData));
                const mergedConfig = deepMerge(originalSnapshot, modifiedConfig);
                showPreviewModal(originalSnapshot, modifiedConfig, mergedConfig);
            } catch (e) {
                console.error('解析配置变更失败:', e);
            }
        }
        
    } catch (error) {
        removeTypingIndicator();
        console.error('AI请求失败:', error);
        addAiMessage('抱歉，我现在无法连接到服务器，请稍后再试哦～<i class="fas fa-sad-tear"></i>');
    }
}

function deepMerge(target, source) {
    const result = { ...target };
    
    for (const key in source) {
        if (source[key] && typeof source[key] === 'object' && !Array.isArray(source[key])) {
            if (!(key in target)) {
                result[key] = source[key];
            } else {
                result[key] = deepMerge(target[key], source[key]);
            }
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
            if (Object.keys(nestedChanges).length > 0) {
                changes[key] = nestedChanges;
            }
        } else if (JSON.stringify(original[key]) !== JSON.stringify(modified[key])) {
            changes[key] = modified[key];
        }
    }
    
    return changes;
}

function showPreviewModal(original, modified, merged) {
    pendingConfigChanges = {
        original: original,
        modified: modified,
        merged: merged
    };
    
    const changed = getChangedConfig(original, modified);
    
    document.getElementById('originalConfigPreview').textContent = JSON.stringify(original, null, 2);
    document.getElementById('changedConfigPreview').textContent = JSON.stringify(changed, null, 2);
    document.getElementById('mergedConfigPreview').textContent = JSON.stringify(merged, null, 2);
    
    document.getElementById('previewModal').classList.add('active');
}

function closePreviewModal() {
    document.getElementById('previewModal').classList.remove('active');
    pendingConfigChanges = null;
}

function applyConfigChanges() {
    if (pendingConfigChanges) {
        configData = JSON.parse(JSON.stringify(pendingConfigChanges.merged));
        renderAllPages();
        addAiMessage('好的！配置已经成功更新啦～<i class="fas fa-sparkles"></i>');
    }
    closePreviewModal();
}

function switchPage(pageName) {
    document.querySelectorAll('.page-tab').forEach(tab => {
        tab.classList.toggle('active', tab.dataset.page === pageName);
    });
    
    document.querySelectorAll('.page-panel').forEach(panel => {
        panel.classList.toggle('active', panel.id === `panel-${pageName}`);
    });
}

document.addEventListener('DOMContentLoaded', () => {
    loadConfig();
    
    document.getElementById('saveBtn').addEventListener('click', saveConfig);
    document.getElementById('resetBtn').addEventListener('click', resetConfig);
    document.getElementById('aiSendBtn').addEventListener('click', sendToAI);
    
    document.getElementById('aiInput').addEventListener('keydown', (e) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            sendToAI();
        }
    });
    
    document.querySelectorAll('.page-tab').forEach(tab => {
        tab.addEventListener('click', () => {
            switchPage(tab.dataset.page);
        });
    });
    
    document.getElementById('closePreview').addEventListener('click', closePreviewModal);
    document.getElementById('cancelChanges').addEventListener('click', closePreviewModal);
    document.getElementById('applyChanges').addEventListener('click', applyConfigChanges);
    
    document.getElementById('previewModal').addEventListener('click', (e) => {
        if (e.target.id === 'previewModal') {
            closePreviewModal();
        }
    });
});