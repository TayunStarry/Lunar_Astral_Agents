/* ========== 全局配置 ========== */
const labelMap = {
    'models': '模型配置',
    'server': '服务器配置',
    'cloud': '云端配置',
    'qq_adapter': 'QQ适配器',
    'project_archiving': '项目归档',
    'diffusion_model': '扩散模型',
    'variational_model': '变分模型',
    'prompt_refine_model': '提示词精炼模型',
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

let configData = null;
let originalConfig = null;
let aiMessages = [];
let pendingConfigChanges = null;
const MAX_CONTEXT_MESSAGES = 15;

/* ========== 工具函数 ========== */
function getLabel(key) {
    return labelMap[key] || key;
}

function encodeFileName(filename) {
    return btoa(unescape(encodeURIComponent(filename)));
}

function getValueByPath(path) {
    const parts = path.replace(/\[(\d+)\]/g, '.$1').split('.');
    let current = configData;
    for (const part of parts) current = current[part];
    return current;
}

function setValueByPath(path, value) {
    const parts = path.replace(/\[(\d+)\]/g, '.$1').split('.');
    let current = configData;
    for (let i = 0; i < parts.length - 1; i++) current = current[parts[i]];
    current[parts[parts.length - 1]] = value;
}

/* ========== 数据加载与保存 ========== */
async function loadConfig() {
    try {
        const res = await fetch('/file/read/lunar_config.json');
        if (!res.ok) throw new Error('加载配置失败');
        configData = await res.json();
        originalConfig = JSON.parse(JSON.stringify(configData));
        buildAllPages();
    } catch (e) {
        console.error(e);
        alert('加载配置失败，请检查服务是否正常运行');
    }
}

async function saveConfig() {
    try {
        collectConfig();
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
        alert('配置已保存！');
    } catch (e) {
        console.error(e);
        alert('保存失败，请检查服务');
    }
}

function resetConfig() {
    if (!originalConfig) return;
    configData = JSON.parse(JSON.stringify(originalConfig));
    buildAllPages();
}

/* ========== 收集表单数据 ========== */
function collectConfig() {
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

/* ========== 页面构建 ========== */
const topLevelKeys = ['models', 'server', 'cloud', 'qq_adapter', 'project_archiving'];

function buildAllPages() {
    buildNavigation();
    buildPages();
    // 默认显示第一个页面
    switchPage(topLevelKeys[0]);
}

function buildNavigation() {
    const nav = document.getElementById('topNav');
    nav.innerHTML = '';
    topLevelKeys.forEach(key => {
        const btn = document.createElement('button');
        btn.className = 'nav-tab';
        btn.dataset.page = key;
        btn.innerHTML = `<i class="fas fa-${iconForSection(key)}"></i> ${getLabel(key)}`;
        btn.addEventListener('click', () => switchPage(key));
        nav.appendChild(btn);
    });
}

function iconForSection(key) {
    const icons = {
        models: 'robot', server: 'server', cloud: 'cloud',
        qq_adapter: 'qq', project_archiving: 'file-archive'
    };
    return icons[key] || 'cube';
}

function buildPages() {
    const container = document.getElementById('pageContainer');
    container.innerHTML = '';
    topLevelKeys.forEach(key => {
        const page = document.createElement('div');
        page.className = 'config-page';
        page.id = `page-${key}`;
        page.dataset.page = key;
        const data = configData[key];
        if (data && typeof data === 'object') {
            page.appendChild(createSectionBubbles(key, data, key));
        }
        container.appendChild(page);
    });
}

/* 为一组键值对生成气泡容器 */
function createSectionBubbles(sectionKey, data, basePath) {
    const wrap = document.createElement('div');
    wrap.className = 'bubble-grid';
    Object.keys(data).forEach(fieldKey => {
        const value = data[fieldKey];
        const path = `${basePath}.${fieldKey}`;
        wrap.appendChild(createBubble(fieldKey, value, path));
    });
    return wrap;
}

/* 根据类型创建对应气泡 */
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
        bubble.classList.add('bubble-primitive');
        bubble.appendChild(createPrimitiveContent(key, value, path));
    }
    return bubble;
}

/* 基础类型字段：标签 + 控件 */
function createPrimitiveContent(key, value, path) {
    const frag = document.createDocumentFragment();
    const label = document.createElement('span');
    label.className = 'bubble-label';
    label.textContent = getLabel(key);
    frag.appendChild(label);

    if (typeof value === 'boolean') {
        const sw = document.createElement('label');
        sw.className = 'switch';
        sw.innerHTML = `
            <input type="checkbox" data-path="${path}" ${value ? 'checked' : ''}>
            <span class="slider"></span>
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

/* 数组字段：可增减列表 */
function createArrayContent(key, items, path) {
    const frag = document.createDocumentFragment();
    const header = document.createElement('div');
    header.className = 'array-header';
    header.innerHTML = `<span>${getLabel(key)}</span>`;
    const addBtn = document.createElement('button');
    addBtn.className = 'btn btn-small';
    addBtn.innerHTML = '<i class="fas fa-plus"></i> 添加';
    addBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        const arr = getValueByPath(path);
        arr.push('');
        refreshPageForPath(path);
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
        delBtn.className = 'btn btn-small';
        delBtn.innerHTML = '<i class="fas fa-trash"></i>';
        delBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            const arr = getValueByPath(path);
            arr.splice(idx, 1);
            refreshPageForPath(path);
        });
        row.appendChild(delBtn);
        list.appendChild(row);
    });
    frag.appendChild(list);
    return frag;
}

/* 对象字段：嵌套气泡区域 */
function createObjectContent(key, obj, path) {
    const frag = document.createDocumentFragment();
    const title = document.createElement('div');
    title.className = 'object-title';
    title.textContent = getLabel(key);
    frag.appendChild(title);

    const innerGrid = document.createElement('div');
    innerGrid.className = 'bubble-grid nested';
    Object.keys(obj).forEach(subKey => {
        const subValue = obj[subKey];
        const subPath = `${path}.${subKey}`;
        innerGrid.appendChild(createBubble(subKey, subValue, subPath));
    });
    frag.appendChild(innerGrid);
    return frag;
}

/* 刷新当前显示的页面（保持导航激活状态） */
function refreshPageForPath(path) {
    const topKey = path.split('.')[0];
    const pageEl = document.getElementById(`page-${topKey}`);
    if (!pageEl) return;
    const data = configData[topKey];
    pageEl.innerHTML = '';
    pageEl.appendChild(createSectionBubbles(topKey, data, topKey));
}

function refreshAllPages() {
    topLevelKeys.forEach(key => {
        const pageEl = document.getElementById(`page-${key}`);
        if (pageEl) {
            pageEl.innerHTML = '';
            const data = configData[key];
            if (data) pageEl.appendChild(createSectionBubbles(key, data, key));
        }
    });
}

/* 页面切换（平滑过渡） */
function switchPage(pageKey) {
    // 更新导航激活状态
    document.querySelectorAll('.nav-tab').forEach(btn => {
        btn.classList.toggle('active', btn.dataset.page === pageKey);
    });
    // 切换页面显示
    document.querySelectorAll('.config-page').forEach(page => {
        if (page.dataset.page === pageKey) {
            page.classList.add('active');
        } else {
            page.classList.remove('active');
        }
    });
}

/* ========== AI 对话（保留原有逻辑） ========== */
function addUserMessage(content) {
    const container = document.getElementById('aiMessages');
    const div = document.createElement('div');
    div.className = 'message user-message';
    div.innerHTML = `<div class="message-avatar">你</div><div class="message-content">${marked.parse(escapeHtml(content))}</div>`;
    container.appendChild(div);
    container.scrollTop = container.scrollHeight;
}

function addAiMessage(content) {
    const container = document.getElementById('aiMessages');
    const div = document.createElement('div');
    div.className = 'message ai-message';
    div.innerHTML = `<div class="message-avatar"><img src="/file/read/images/icon/agent_avatar.Webp" alt="Agent Avatar"></div><div class="message-content">${marked.parse(content)}</div>`;
    container.appendChild(div);
    container.scrollTop = container.scrollHeight;
}

function addTypingIndicator() {
    const container = document.getElementById('aiMessages');
    const div = document.createElement('div');
    div.className = 'message ai-message';
    div.id = 'typingIndicator';
    div.innerHTML = `<div class="message-avatar"><img src="/file/read/images/icon/agent_avatar.Webp" alt="Agent Avatar"></div><div class="message-content">正在思考...<span class="typing-dots"></span></div>`;
    container.appendChild(div);
    container.scrollTop = container.scrollHeight;
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
    aiMessages.push({ role: 'user', content });

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
        const modelName = configData?.cloud?.multimodal_model_name || 'system-multimodal';
        const res = await fetch(apiUrl, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${configData?.cloud?.cloud_model_key || ''}` },
            body: JSON.stringify({ model: modelName, messages, stream: false })
        });
        if (!res.ok) throw new Error('AI请求失败');
        const result = await res.json();
        const aiContent = result.choices[0].message.content;
        removeTypingIndicator();
        addAiMessage(aiContent);
        aiMessages.push({ role: 'assistant', content: aiContent });
        if (aiMessages.length > MAX_CONTEXT_MESSAGES) aiMessages = aiMessages.slice(-MAX_CONTEXT_MESSAGES);

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
        console.error(error);
        addAiMessage('抱歉，我现在无法连接到服务器，请稍后再试哦～<i class="fas fa-sad-tear"></i>');
    }
}

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

function showPreviewModal(original, modified, merged) {
    pendingConfigChanges = { original, modified, merged };
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
        originalConfig = JSON.parse(JSON.stringify(configData));
        refreshAllPages();
        addAiMessage('好的！配置已经成功更新啦～<i class="fas fa-sparkles"></i>');
    }
    closePreviewModal();
}

/* ========== 事件绑定 ========== */
document.addEventListener('DOMContentLoaded', () => {
    loadConfig().then(() => {
        document.getElementById('saveBtn').addEventListener('click', saveConfig);
        document.getElementById('resetBtn').addEventListener('click', resetConfig);
        document.getElementById('aiSendBtn').addEventListener('click', sendToAI);
        document.getElementById('aiInput').addEventListener('keydown', (e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                sendToAI();
            }
        });
        document.getElementById('closePreview').addEventListener('click', closePreviewModal);
        document.getElementById('cancelChanges').addEventListener('click', closePreviewModal);
        document.getElementById('applyChanges').addEventListener('click', applyConfigChanges);
        document.getElementById('previewModal').addEventListener('click', (e) => {
            if (e.target.id === 'previewModal') closePreviewModal();
        });
    });
});