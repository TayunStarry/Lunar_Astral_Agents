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
        const response = await fetch('/system_prompt.md');
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
        await loadLayout();
        const changed = buildLayout();
        renderPageGrid();
        if (changed) await persistLayout();
    } catch (error) { console.error('Failed to load pages:', error); }
}

// ===== 网格渲染：预设网格布局页（应用图标分配到空格位置，可拖动摆放，位置 JSON 持久化） =====
const DEFAULT_ICON_COUNT = 8;

function getRandomDefaultIcon() {
    return `/default/icon (${Math.floor(Math.random() * DEFAULT_ICON_COUNT) + 1}).webp`;
}

// 标签 → 卡片角标色彩修饰类（不同标签不同配色，无对应样式的标签回退默认紫色）
function getTagModifierClass(tag) {
    switch (tag) {
        case 'Zero-LTP': return 'card-tag-zero-ltp';
        case 'Node-LTP': return 'card-tag-node-ltp';
        case 'Mini-LTP': return 'card-tag-mini-ltp';
        case 'Self-LTP': return 'card-tag-self-ltp';
        case 'Git': return 'card-tag-git';
        case 'DeepSeek': return 'card-tag-deepseek';
        case 'DS-Demo': return 'card-tag-deepseek-demo';
        default: return '';
    }
}

/* 布局持久化：扁平数组 layout，数组下标即网格槽位，元素为页面 id 或 null（null 表示空格）。
   通过 /file/read + /file/write 读写到本地 JSON（desktop_layout.json）。 */
const GRID_LAYOUT_FILE = 'desktop_layout.json';
let layout = [];

// 读取 CSS 定义的响应式列数（--grid-cols）
function getGridCols() {
    const v = parseInt(getComputedStyle(pageGrid).getPropertyValue('--grid-cols'), 10);
    return Number.isFinite(v) && v > 0 ? v : 6;
}

async function loadLayout() {
    try {
        const resp = await fetch('/file/read/' + encodeURIComponent(GRID_LAYOUT_FILE));
        if (resp.ok) {
            const data = await resp.json();
            if (Array.isArray(data)) layout = data;
        }
    } catch (e) { console.warn('读取桌面布局失败，使用默认排布:', e); }
    if (!Array.isArray(layout)) layout = [];
}

async function persistLayout() {
    try {
        const blob = new Blob([JSON.stringify(layout)], { type: 'application/json' });
        await fetch('/file/write', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'X-File-Name': encodeFileName(GRID_LAYOUT_FILE),
                'X-Overwrite': 'true'
            },
            body: blob
        });
    } catch (e) { console.error('保存桌面布局失败:', e); }
}

/* 系统加载或新增应用时，图标从前到后填充到空位；已删除应用对应的槽位置为空格。
   返回布局是否发生变化（供调用方决定是否持久化）。 */
function buildLayout() {
    const validIds = new Set(pages.map(p => p.id));
    // 保留下仍然存在的 id 与空格，删除已不存在的 id
    const next = layout.map(id => (id && validIds.has(id)) ? id : null);
    const placed = new Set(next.filter(Boolean));
    const newIds = pages.map(p => p.id).filter(id => !placed.has(id));
    // 新应用从前到后填充第一个空位，无空位则追加到末尾
    for (const id of newIds) {
        const nullIdx = next.indexOf(null);
        if (nullIdx === -1) next.push(id);
        else next[nullIdx] = id;
    }
    let changed = next.length !== layout.length;
    if (!changed) {
        for (let i = 0; i < next.length; i++) {
            if ((next[i] ?? null) !== (layout[i] ?? null)) { changed = true; break; }
        }
    }
    layout = next;
    return changed;
}

function renderPageGrid() {
    const pageById = new Map(pages.map(p => [p.id, p]));
    const cols = getGridCols();
    // 补齐最后一行的空格，保持预设网格满格可放置
    const rem = layout.length % cols;
    if (rem !== 0) for (let i = 0; i < cols - rem; i++) layout.push(null);

    pageGrid.innerHTML = '';
    layout.forEach((id, index) => {
        const slot = document.createElement('div');
        slot.className = 'grid-slot';
        slot.dataset.index = index;
        if (id) {
            const page = pageById.get(id);
            if (page) {
                slot.classList.add('grid-slot-filled');
                slot.appendChild(buildPageCard(page));
            } else {
                slot.classList.add('grid-slot-empty');
            }
        } else {
            slot.classList.add('grid-slot-empty');
        }
        pageGrid.appendChild(slot);
    });
}

const SELF_LTP_TAG = 'Self-LTP';

function buildPageCard(page) {
    const card = document.createElement('div');
    card.className = 'page-card';
    card.dataset.pageId = page.id;
    card.draggable = true;

    const isSelfLTP = !!(page.tags && page.tags.includes(SELF_LTP_TAG));
    // 未选中时，右上角显示一个随机标签（按标签类型着色）
    const displayTag = (page.tags && page.tags.length > 0)
        ? page.tags[Math.floor(Math.random() * page.tags.length)]
        : null;
    const tagModifier = displayTag ? getTagModifierClass(displayTag) : '';

    // 只显示应用图标与应用名称（description 不展示在前端，仅供 AI 理解项目）
    card.innerHTML = `
        ${displayTag ? `<span class="card-tag${tagModifier ? ' ' + tagModifier : ''}">${displayTag}</span>` : ''}
        <div class="icon">
            <img src="${page.icon || getRandomDefaultIcon()}" alt="${page.title}" onerror="this.onerror=null;this.src=getRandomDefaultIcon()">
        </div>
        <h3>${page.title}</h3>
        <button class="card-gear" title="设置"><i class="fas fa-cog"></i></button>
        <div class="card-menu">
            <button class="card-menu-item" data-action="export"><i class="fas fa-box"></i> 导出</button>
            <button class="card-menu-item" data-action="delete"><i class="fas fa-trash-alt"></i> 删除</button>
            <button class="card-menu-item" data-action="archive"><i class="fas fa-archive"></i> 归档</button>
            <button class="card-menu-item card-menu-assistant" data-action="assistant">
                <i class="fas fa-robot"></i> ${isSelfLTP ? '手动' : '助理'}
            </button>
        </div>
    `;

    // 单击 = 选中（持续闪烁）；已选中状态下再次单击 = 进入页面
    card.addEventListener('click', (e) => {
        if (e.target.closest('.card-gear') || e.target.closest('.card-menu')) return;
        if (card.classList.contains('selected')) {
            openPage(page);
        } else {
            selectCard(card);
        }
    });
    // 齿轮：切换管理菜单
    card.querySelector('.card-gear').addEventListener('click', (e) => {
        e.stopPropagation();
        const menu = card.querySelector('.card-menu');
        const willOpen = !menu.classList.contains('active');
        closeAllCardMenus();
        if (willOpen) menu.classList.add('active');
    });
    // 菜单项处理
    card.querySelectorAll('.card-menu-item').forEach(item => {
        item.addEventListener('click', (e) => {
            e.stopPropagation();
            handleCardMenuAction(item.dataset.action, page, card);
        });
    });
    return card;
}

// 当前选中的卡片（单实例，点击选中后显示齿轮）
let selectedCard = null;

function selectCard(card) {
    if (selectedCard && selectedCard !== card) {
        selectedCard.classList.remove('selected');
        selectedCard.querySelector('.card-menu')?.classList.remove('active');
    }
    selectedCard = card;
    // 通过 .selected 触发持续的慢速高亮闪烁（隐藏标签、显示设置按钮）
    card.classList.remove('selected');
    void card.offsetWidth;
    card.classList.add('selected');
}

function closeAllCardMenus() {
    pageGrid.querySelectorAll('.page-card.selected').forEach(c => {
        c.querySelector('.card-menu')?.classList.remove('active');
    });
}

async function handleCardMenuAction(action, page, card) {
    const pkgName = page.package_name;
    switch (action) {
        case 'export':
            openExportModal(pkgName);
            break;
        case 'delete':
            openDeleteModal(pkgName);
            break;
        case 'archive':
            await archivePackage(pkgName);
            break;
        case 'assistant':
            await toggleSelfLTP(page, card);
            break;
    }
}

// 归档：先执行一次导出备份，导出成功后再删除当前包
async function archivePackage(pkgName) {
    if (!pkgName) { addMessage('system', '无法获取包名信息'); return; }
    addMessage('system', `正在归档【${pkgName}】：先导出备份，再删除原包...`);
    try {
        const resp = await fetch('/file/package/export', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ package_name: pkgName, action: 'save' })
        });
        const data = await resp.json();
        if (!data.success) {
            addMessage('system', `归档导出失败: ${data.message || ''}`);
            return;
        }
        const delResp = await fetch('/file/package/delete', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ package_name: pkgName })
        });
        const delData = await delResp.json();
        if (delData.success) {
            addMessage('system', '归档完成：已备份到 ' + (data.save_path || pkgName + '.ltpx') + ' 并删除原包');
        } else {
            addMessage('system', `备份成功但删除失败: ${delData.message || ''}`);
        }
        setTimeout(() => loadPages(), 400);
    } catch (e) {
        console.error('归档失败:', e);
        addMessage('system', '归档时发生网络错误');
    }
}

// 助理/手动：切换包的 Self-LTP 标签，以启用/停用界面自动操作助理
async function toggleSelfLTP(page, card) {
    const pkgName = page.package_name;
    if (!pkgName) { addMessage('system', '无法获取包名信息'); return; }
    try {
        const readResp = await fetch('/file/read/package/' + encodeURIComponent(pkgName) + '/metadata.json');
        if (!readResp.ok) throw new Error('读取包元信息失败');
        const meta = await readResp.json();
        const tags = Array.isArray(meta.tags) ? meta.tags.slice() : [];
        const has = tags.includes(SELF_LTP_TAG);
        if (has) {
            meta.tags = tags.filter(t => t !== SELF_LTP_TAG);
        } else {
            meta.tags = tags.concat(SELF_LTP_TAG);
        }
        const blob = new Blob([JSON.stringify(meta, null, 2)], { type: 'application/json' });
        const writeResp = await fetch('/file/write', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'X-File-Name': encodeFileName('package/' + pkgName + '/metadata.json'),
                'X-Overwrite': 'true'
            },
            body: blob
        });
        if (!writeResp.ok) throw new Error('写入包元信息失败');
        addMessage('system', has
            ? `已取消【${page.title}】的 Self-LTP 界面自动操作助理`
            : `已为【${page.title}】启用 Self-LTP 界面自动操作助理`);
        setTimeout(() => loadPages(), 300);
    } catch (e) {
        console.error('切换 Self-LTP 标签失败:', e);
        addMessage('system', '切换 Self-LTP 标签失败: ' + (e.message || ''));
    }
}

// ===== 网格拖动摆放 =====
let gridDragSrc = null;   // 拖动中的源槽位下标

function setupGridDrag() {
    pageGrid.addEventListener('dragstart', (e) => {
        const slot = e.target.closest('.grid-slot');
        const card = e.target.closest('.page-card');
        if (!slot || !card) return;
        gridDragSrc = parseInt(slot.dataset.index, 10);
        e.dataTransfer.effectAllowed = 'move';
        try { e.dataTransfer.setData('application/x-liuli-grid', String(gridDragSrc)); } catch (err) { }
        e.dataTransfer.setData('text/plain', String(gridDragSrc));
        requestAnimationFrame(() => card.classList.add('dragging'));
    });

    pageGrid.addEventListener('dragover', (e) => {
        const types = e.dataTransfer ? e.dataTransfer.types : [];
        const isGridDrag = Array.prototype.indexOf.call(types, 'application/x-liuli-grid') !== -1
            || typeof gridDragSrc === 'number';
        if (!isGridDrag) { e.preventDefault(); return; }
        const slot = e.target.closest('.grid-slot');
        if (!slot) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
        clearDragState();
        slot.classList.add('drag-over');
    });

    pageGrid.addEventListener('drop', (e) => {
        const slot = e.target.closest('.grid-slot');
        if (typeof gridDragSrc !== 'number' || !slot) return;
        e.preventDefault();
        e.stopPropagation();
        const dstIdx = parseInt(slot.dataset.index, 10);
        if (gridDragSrc === dstIdx) { gridDragSrc = null; clearDragState(); return; }
        // 拖到空格 → 图标移动到空位；拖到另一应用图标 → 两图标互换位置
        const srcId = layout[gridDragSrc] ?? null;
        const dstId = layout[dstIdx] ?? null;
        layout[gridDragSrc] = dstId;
        layout[dstIdx] = srcId;
        gridDragSrc = null;
        clearDragState();
        persistLayout().then(() => renderPageGrid());
    });

    pageGrid.addEventListener('dragend', () => {
        gridDragSrc = null;
        clearDragState();
    });
}

function clearDragState() {
    pageGrid.querySelectorAll('.page-card.dragging, .grid-slot.drag-over')
        .forEach(el => el.classList.remove('dragging', 'drag-over'));
}

// 构建结构化操作指令（附加到系统提示词）：不再依赖范式函数调用（避免多模态模型后端卡死），
// 改由琉璃在需要执行操作时输出一个 JSON 动作对象，前端解析后执行。
function buildActionInstruction() {
    const pageList = pages
        .map(p => ({ id: p.id, title: p.title }))
        .sort((a, b) => (a.title || '').localeCompare(b.title || ''));
    return `【可打开的应用列表】
当用户想打开某个应用/功能页面时，从以下列表中选择最匹配的 id。
${JSON.stringify(pageList, null, 2)}

【结构化操作输出规则】
你需要用一个 JSON 对象来表达"要执行的操作"，并且只把 JSON 放在单独的代码块中(\`\`\`json ... \`\`\`)，不要夹杂其它文字。支持的动作：
1. 打开应用：
\`\`\`json
{"action": "open_page", "page_id": "上面列表中的某个 id"}
\`\`\`
2. 查看配置（不传 section 则查看全部）：
\`\`\`json
{"action": "get_config", "section": "server"}
\`\`\`
3. 修改配置（系统会弹出确认框）：
\`\`\`json
{"action": "modify_config", "changes": {"server": {"port": 8080}}}
\`\`\`
4. 不需要执行任何操作、只是普通聊天回答时，直接正常回复文字即可，不要输出 JSON。

请注意：如果用户只是闲聊或询问，请正常用文字回复；只有当用户明确要求打开应用、查看或修改配置时才输出对应的 JSON 动作。`;
}

// ===== 页面打开 =====
// 打开统一走覆盖层 iframe（与月华调用共用同一 iframe），不再整页跳转
function openPage(page) {
    if (page.tags && page.url && page.url.endsWith('.md')) {
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
// 琉璃选中的卡片与用户点击选中一致：滚动到卡片并进入持续高亮闪烁的选中态。
// 用户在高亮闪烁状态下再次点击卡片即进入页面。
function locateAndHighlightCard(pageId) {
    const card = document.querySelector(`.page-card[data-page-id="${pageId}"]`);
    if (!card) return;

    if (chatModal.classList.contains('active')) {
        fadeOutChatModal();
    }

    card.scrollIntoView({ behavior: 'smooth', block: 'center' });
    selectCard(card);
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

    // 构建增强版系统提示词，附加当前配置信息与结构化操作指令
    const enhancedSystemPrompt = SYSTEM_PROMPT + '\n\n' + buildConfigContext() + '\n\n' + buildActionInstruction();
    const allMessages = [{ role: 'system', content: enhancedSystemPrompt }, ...messages];

    try {
        const modelName = configData?.agent?.multimodal_model || 'system-multimodal';
        const response = await fetch('/v1/chat/completions', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ model: modelName, messages: allMessages })
        });

        if (!response.ok) throw new Error('Network response was not ok');

        const data = await response.json();
        const assistantMessage = data.choices[0].message;
        const replyContent = assistantMessage.content || '';

        // 解析结构化操作对象；若命中则执行动作（不把 JSON 原样展示给用户），否则当作普通回复
        const action = parseStructuredAction(replyContent);
        if (action) {
            await executeStructuredAction(action);
        } else {
            addMessage('assistant', replyContent);
            // 兼容旧格式：解析回复中的 JSON 代码块配置变更
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
如果用户要求修改配置，你可以输出结构化动作：
\`\`\`json
{"action": "modify_config", "changes": {"server": {"port": 8080}}}
\`\`\`
系统会自动弹出确认对话框；也可以在普通回复中用 \`\`\`json 代码块包含配置变更对象，系统同样会弹出确认框。

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

// ===== 结构化操作：解析与执行（替代范式函数调用，规避多模态模型后端卡死） =====
function parseStructuredAction(content) {
    if (!content || typeof content !== 'string') return null;
    let text = content;
    const fence = text.match(/```json\s*([\s\S]*?)```/);
    if (fence) text = fence[1].trim();
    const m = text.match(/\{[\s\S]*\}/);
    if (!m) return null;
    try {
        const obj = JSON.parse(m[0]);
        if (obj && typeof obj.action === 'string') return obj;
    } catch (e) { /* 不是合法 JSON 动作，按普通回复处理 */ }
    return null;
}

async function executeStructuredAction(action) {
    switch (action.action) {
        case 'open_page': {
            const page = pages.find(p => p.id === action.page_id);
            if (page) {
                // 选中该应用（持续闪烁），由用户在高亮状态下再次点击进入页面
                locateAndHighlightCard(action.page_id);
            } else {
                addMessage('system', '抱歉，没有找到对应的应用页面');
            }
            break;
        }
        case 'get_config': {
            if (action.section) {
                const sectionData = configData ? configData[action.section] : undefined;
                addMessage('system', `【${getLabel(action.section)}】配置已获取`);
                addMessage('assistant', `当前${getLabel(action.section)}配置如下：\n\`\`\`json\n${JSON.stringify(sectionData ?? null, null, 2)}\n\`\`\``);
            } else {
                addMessage('assistant', `当前全部配置如下：\n\`\`\`json\n${JSON.stringify(configData ?? {}, null, 2)}\n\`\`\``);
            }
            break;
        }
        case 'modify_config': {
            if (action.changes) {
                const originalSnapshot = JSON.parse(JSON.stringify(configData));
                const mergedConfig = deepMerge(originalSnapshot, action.changes);
                showPreviewModal(originalSnapshot, action.changes, mergedConfig);
                addMessage('system', '已生成配置变更预览，请在弹窗中确认');
            }
            break;
        }
        default:
            // 未知动作：当作普通文字展示
            addMessage('assistant', JSON.stringify(action));
            break;
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

// 全局拖拽上传（仅文件拖入时显示覆盖层；应用图标内部的拖动摆放不触发）
document.addEventListener('dragover', (e) => {
    const types = e.dataTransfer ? e.dataTransfer.types : [];
    const isFiles = Array.prototype.indexOf.call(types, 'Files') !== -1;
    if (!isFiles) return;
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

// 将包 ID 解析为包目录名：LTPX 广播携带的是 metadata.id，而资源按目录名（package_name）定位
function resolvePackageDir(appId) {
    const p = pages.find(x => x.id === appId);
    return (p && p.package_name) || appId;
}

// 判断包是否为 Mini-LTP（带 Mini-LTP 标签）
function isMiniLTPPage(appId) {
    const p = pages.find(x => x.id === appId);
    return !!(p && p.tags && p.tags.includes('Mini-LTP'));
}

// 判断包是否为 Self-LTP（带 Self-LTP 标签，自主驾驶，不接入 AtoA）
function isSelfLTPPage(appId) {
    const p = pages.find(x => x.id === appId);
    return !!(p && p.tags && p.tags.includes('Self-LTP'));
}

// 收到月华的 ltpx_call：打开对应包页面并投递执行指令
function openLTPXPackage(msg) {
    activeLTPXCall = msg;
    ltpxFrameTitle.innerHTML = '<i class="fas fa-cube"></i> ' + (msg.app_id || '包') + ' 执行中...';
    // 同一包页面已就绪（已加载 + agent 已注入）时直接投递，不重新加载，保持页面状态
    // （Mini-LTP 与普通包一致；仅首次或换包时才加载/注入）
    if (ltpxFrameApp === msg.app_id && ltpxFrameReady) {
        deliverLTPXRun();
        ltpxOverlay.classList.add('active');
        return;
    }
    // 首次加载该包 or 切换到其它包：加载页面
    ltpxFrameReady = false;
    ltpxFrameApp = msg.app_id;
    const base = '/file/read/package/' + encodeURIComponent(resolvePackageDir(msg.app_id)) + '/index.html';
    ltpxFrame.src = isMiniLTPPage(msg.app_id) ? base + '?t=' + Date.now() : base;
    ltpxOverlay.classList.add('active');
}

// 向 iframe 注入一段 JS 源码（url 提供的脚本）；注入成功后才回调。
// 供共享模块与智能体按序注入：先注 /shared-input.js，再注智能体脚本。
function injectFrameJS(win, url, injectedKey, label, callback) {
    try {
        fetch(url).then(r => (r.ok ? r.text() : '')).then(code => {
            try {
                if (code && win && win.document && !win.document[injectedKey]) {
                    const s = win.document.createElement('script');
                    s.textContent = code;
                    win.document.head.appendChild(s); // 同步执行
                    win.document[injectedKey] = true;
                }
            } catch (e) { console.warn((label || '') + ' 注入异常:', e); }
            callback();
        }).catch(e => { console.warn((label || '') + ' 加载失败:', e); callback(); });
    } catch (e) { console.warn((label || '') + ' 注入异常:', e); callback(); }
}

// 向 iframe 注入通用页面操作智能体（仅 Mini-LTP 包）：先注共享键鼠模块，再注智能体
function injectFrameAgent(win, callback) {
    injectFrameJS(win, '/shared-input.js', '__sharedInputInjected', 'shared-input',
        () => injectFrameJS(win, '/mini-ltp-agent.js', '__miniLTPAgentInjected', 'Mini-LTP 智能体', callback));
}

// 向 iframe 注入自主页面操作智能体（仅 Self-LTP 包）：先注共享键鼠模块，再注智能体
function injectSelfLTPAgent(win, callback) {
    injectFrameJS(win, '/shared-input.js', '__sharedInputInjected', 'shared-input',
        () => injectFrameJS(win, '/self-ltp-agent.js', '__selfLTPAgentInjected', 'Self-LTP 智能体', callback));
}

// iframe 加载完成后统一处理：置就绪 →（Mini-LTP / Self-LTP）注入 agent → 投递待定指令
function handleFrameLoad(appId) {
    const win = ltpxFrame.contentWindow;
    const finish = () => { ltpxFrameReady = true; deliverLTPXRun(); };
    if (isMiniLTPPage(appId)) injectFrameAgent(win, finish);
    else if (isSelfLTPPage(appId)) injectSelfLTPAgent(win, finish);
    else finish();
}
ltpxFrame.onload = () => handleFrameLoad(ltpxFrameApp);

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

// ===== 创建模块 =====
const createModuleBtn = document.getElementById('createModuleBtn');
const createModuleModal = document.getElementById('createModuleModal');
const createModuleModalClose = document.getElementById('createModuleModalClose');
const createModuleCancelBtn = document.getElementById('createModuleCancelBtn');
const createModuleSubmitBtn = document.getElementById('createModuleSubmitBtn');
const moduleUrlInput = document.getElementById('moduleUrlInput');
const moduleZipInput = document.getElementById('moduleZipInput');
const moduleIdInput = document.getElementById('moduleIdInput');
const moduleTitleInput = document.getElementById('moduleTitleInput');
const moduleDescInput = document.getElementById('moduleDescInput');
const moduleMiniLtp = document.getElementById('moduleMiniLtp');
const moduleAiGenBtn = document.getElementById('moduleAiGenBtn');
const iconStickerQuery = document.getElementById('iconStickerQuery');
const iconStickerSearchBtn = document.getElementById('iconStickerSearchBtn');
const iconStickerResults = document.getElementById('iconStickerResults');
const iconStickerStatus = document.getElementById('iconStickerStatus');
const iconManualInput = document.getElementById('iconManualInput');

let moduleSource = 'url';           // 当前来源：url / zip
let selectedStickerData = null;     // 选中的 sticker base64 dataURL

function openCreateModuleModal() {
    createModuleModal.classList.add('active');
    setModuleSource('url');
    selectedStickerData = null;
    iconStickerResults.innerHTML = '';
    iconStickerStatus.textContent = '';
    setTimeout(() => moduleUrlInput.focus(), 100);
}

function closeCreateModuleModal() {
    createModuleModal.classList.remove('active');
}

function setModuleSource(src) {
    moduleSource = src;
    document.querySelectorAll('.source-tab').forEach(t => {
        t.classList.toggle('active', t.dataset.source === src);
    });
    document.getElementById('sourceUrlGroup').style.display = src === 'url' ? 'block' : 'none';
    document.getElementById('sourceZipGroup').style.display = src === 'zip' ? 'block' : 'none';
}

// 来源切换
document.querySelectorAll('.source-tab').forEach(tab => {
    tab.addEventListener('click', () => setModuleSource(tab.dataset.source));
});

// 图标方式切换（留空 / 记忆库 stickers / 手动指定）
document.querySelectorAll('input[name="moduleIconMode"]').forEach(radio => {
    radio.addEventListener('change', () => {
        const mode = document.querySelector('input[name="moduleIconMode"]:checked').value;
        document.getElementById('iconStickerGroup').style.display = mode === 'sticker' ? 'block' : 'none';
        document.getElementById('iconManualGroup').style.display = mode === 'manual' ? 'block' : 'none';
    });
});

// 打开 / 关闭
createModuleBtn.addEventListener('click', openCreateModuleModal);
createModuleModalClose.addEventListener('click', closeCreateModuleModal);
createModuleCancelBtn.addEventListener('click', closeCreateModuleModal);
createModuleModal.addEventListener('click', (e) => {
    if (e.target === createModuleModal) closeCreateModuleModal();
});

// ===== AI 自动生成 id / title / description（AI 服务可用时） =====
// 先调用 /api/module/inspect 提取项目真实内容（README/title/文件清单），再交给 AI 生成
async function inspectModuleProject() {
    const url = moduleUrlInput.value.trim();
    const zipFile = (moduleZipInput.files && moduleZipInput.files[0]) || null;
    if (!url && !zipFile) return null;
    try {
        if (zipFile) {
            const fd = new FormData();
            if (url) fd.append('url', url);
            fd.append('zip_file', zipFile);
            const resp = await fetch('/api/module/inspect', { method: 'POST', body: fd });
            return await resp.json();
        }
        const resp = await fetch('/api/module/inspect', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ url })
        });
        return await resp.json();
    } catch (e) {
        console.warn('检查项目内容失败:', e);
        return null;
    }
}

function buildInspectPrompt(project) {
    const lines = [];
    if (!project) return '';
    if (project.name) lines.push('项目名：' + project.name);
    if (project.url) lines.push('来源：' + project.url);
    (project.fields || []).forEach(f => {
        if (f.key === 'title' && f.text) lines.push('页面标题：' + f.text);
        else if (f.key === 'README' && f.text) lines.push('README 摘要：\n' + f.text);
        else if (f.key === 'filenames' && f.text) lines.push('文件清单：\n' + f.text);
        else if (f.key === 'url' && f.text) lines.push('来源 URL：' + f.text);
    });
    return lines.join('\n\n');
}

async function aiGenerateModuleInfo() {
    const url = moduleUrlInput.value.trim();
    const zipName = (moduleZipInput.files && moduleZipInput.files[0]) ? moduleZipInput.files[0].name : '';
    if (!url && !zipName) {
        addMessage('system', '请先填写 URL/路径或选择 ZIP 文件，再使用 AI 生成');
        return;
    }
    moduleAiGenBtn.disabled = true;
    moduleAiGenBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> 分析并生成中...';
    try {
        // 1) 提取项目真实内容
        const project = await inspectModuleProject();
        const projectText = buildInspectPrompt(project);
        const modelName = configData?.cloud?.multimodal_model_name || 'system-multimodal';
        const prompt = '请根据以下 HTML 项目信息，生成合理的模块元信息。只返回 JSON，不要任何额外文字、解释或 markdown 代码块标记。\n'
            + '【项目信息】\n' + (projectText || (url || zipName))
            + '\n\n【要求】JSON 格式：{"id": "deepseek.xxx", "title": "简洁中文标题", "description": "一句话准确描述项目核心功能（依据 README 与页面标题）", "tool_name": "英文小写下划线单词，描述这个工具是什么（不是应用名称）"}。'
            + '\nid 只允许小写字母、数字、点和短横线，以 deepseek. 开头；title 要精炼准确（可用原英文名做副标题，如「中文名 · English Name」）；description 必须基于给出的项目内容概括，不要臆造；'
            + '\ntool_name 必须用最准确的词语形容这个工具的类型与用途（如飞行模拟器→fpv_flight_simulator、音乐编辑器→lofi_music_editor、文件管理器→file_manager、天气查询→weather_news_query），只允许小写字母、数字和下划线，禁止使用应用的中文名或品牌名。';
        const response = await fetch('/v1/chat/completions', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ model: modelName, messages: [{ role: 'user', content: prompt }] })
        });
        if (!response.ok) throw new Error('AI 服务不可用');
        const data = await response.json();
        const content = data.choices && data.choices[0] && data.choices[0].message
            ? data.choices[0].message.content : '';
        const jsonMatch = content.match(/\{[\s\S]*\}/);
        if (!jsonMatch) throw new Error('AI 返回格式无效');
        const info = JSON.parse(jsonMatch[0]);
        if (info.id) moduleIdInput.value = info.id;
        if (info.title) moduleTitleInput.value = info.title;
        if (info.description) moduleDescInput.value = info.description;
        if (info.tool_name) moduleToolNameInput.value = info.tool_name;
        addMessage('system', '已通过 AI 生成模块信息，请确认后创建');
    } catch (e) {
        console.error('AI 生成模块信息失败:', e);
        addMessage('system', 'AI 生成失败：' + (e.message || 'AI 服务不可用'));
    } finally {
        moduleAiGenBtn.disabled = false;
        moduleAiGenBtn.innerHTML = '<i class="fas fa-magic"></i> AI 生成';
    }
}
moduleAiGenBtn.addEventListener('click', aiGenerateModuleInfo);

// ===== 记忆库 stickers 搜索（图标） =====
async function searchIconStickers() {
    const q = iconStickerQuery.value.trim();
    if (!q) {
        iconStickerStatus.textContent = '请输入要匹配的图标描述';
        return;
    }
    iconStickerSearchBtn.disabled = true;
    iconStickerStatus.textContent = '正在搜索记忆库 stickers...';
    iconStickerResults.innerHTML = '';
    try {
        const resp = await fetch('/memory/stickers/messages?query=' + encodeURIComponent(q) + '&top_k=12');
        const payload = await resp.json();
        // 记忆库统一返回 { data: { results: [...] }, success }:结果嵌套在 data 中
        const data = payload ? (payload.data || payload) : null;
        const results = (data && data.results) || [];
        if (!results.length) {
            iconStickerStatus.textContent = '记忆库 stickers 中未找到匹配图片，可换个描述试试';
            return;
        }
        iconStickerStatus.textContent = '找到 ' + results.length + ' 张，点击选择：';
        results.forEach(r => {
            if (!r.image) return;
            const btn = document.createElement('button');
            btn.type = 'button';
            btn.className = 'sticker-option';
            const img = document.createElement('img');
            img.src = r.image;
            img.alt = r.content || 'sticker';
            btn.appendChild(img);
            btn.addEventListener('click', () => {
                selectedStickerData = r.image;
                document.querySelectorAll('.sticker-option').forEach(o => o.classList.remove('selected'));
                btn.classList.add('selected');
                iconStickerStatus.textContent = '已选择一张 sticker 作为图标';
            });
            iconStickerResults.appendChild(btn);
        });
    } catch (e) {
        console.error('搜索 stickers 失败:', e);
        iconStickerStatus.textContent = '搜索失败（记忆库可能未初始化）';
    } finally {
        iconStickerSearchBtn.disabled = false;
    }
}
iconStickerSearchBtn.addEventListener('click', searchIconStickers);
iconStickerQuery.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') searchIconStickers();
});

// ===== 提交创建 =====
function collectModuleBase() {
    return {
        package_name: '',
        id: moduleIdInput.value.trim(),
        title: moduleTitleInput.value.trim(),
        description: moduleDescInput.value.trim(),
        tool_name: moduleToolNameInput.value.trim(),
        icon: '',
        mini_ltp: moduleMiniLtp.checked,
        tags: []
    };
}

async function submitCreateModule() {
    const title = moduleTitleInput.value.trim();
    if (!title) {
        addMessage('system', '请填写模块标题');
        return;
    }
    const iconMode = document.querySelector('input[name="moduleIconMode"]:checked').value;
    const base = collectModuleBase();
    if (iconMode === 'sticker' && selectedStickerData) base.icon = selectedStickerData;
    else if (iconMode === 'manual') base.icon = iconManualInput.value.trim();

    createModuleSubmitBtn.disabled = true;
    createModuleSubmitBtn.textContent = '创建中...';
    try {
        let data;
        if (moduleSource === 'zip') {
            const file = moduleZipInput.files && moduleZipInput.files[0];
            if (!file) {
                addMessage('system', '请选择 ZIP 文件');
                return;
            }
            const fd = new FormData();
            fd.append('data', JSON.stringify(base));
            fd.append('zip_file', file);
            const resp = await fetch('/api/module/create', { method: 'POST', body: fd });
            data = await resp.json();
        } else {
            const urlOrPath = moduleUrlInput.value.trim();
            if (!urlOrPath) {
                addMessage('system', '请填写 URL 或本地路径');
                return;
            }
            const body = { ...base };
            if (/^https?:\/\//i.test(urlOrPath)) {
                body.url = urlOrPath;
            } else if (/\.(exe|ps1|bat|cmd|lnk)$/i.test(urlOrPath)) {
                body.path = urlOrPath;
            } else {
                body.url = urlOrPath;
            }
            const resp = await fetch('/api/module/create', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body)
            });
            data = await resp.json();
        }
        if (data && data.success) {
            addMessage('system', data.message + '（已刷新应用列表）');
            closeCreateModuleModal();
            setTimeout(() => loadPages(), 400);
        } else {
            addMessage('system', '创建失败：' + ((data && data.message) || '未知错误'));
        }
    } catch (e) {
        console.error('创建模块失败:', e);
        addMessage('system', '创建模块时发生网络错误');
    } finally {
        createModuleSubmitBtn.disabled = false;
        createModuleSubmitBtn.textContent = '创建模块';
    }
}
createModuleSubmitBtn.addEventListener('click', submitCreateModule);

// ===== 启动 =====
async function initApp() {
    await loadConfigLabels();
    await loadConfig();
    await loadSystemPrompt();
    await loadPages();
    initMarked();
    setupGridDrag();
    establishWebSocket();
}
initApp();