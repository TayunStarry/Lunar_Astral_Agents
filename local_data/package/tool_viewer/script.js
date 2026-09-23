// ==== DOM 引用 ====
const markdownBody = document.getElementById('markdownBody');
const docTitle = document.getElementById('docTitle');
const btnBack = document.getElementById('btnBack');

// ==== 初始化 ====
init();

function init() {
    bindEvents();
    loadDocument();
}

function bindEvents() {
    btnBack.addEventListener('click', () => {
        window.location.href = '/';
    });
}

// ==== 依赖就绪 ====
const MARKED_SRC = '/file/read/package/marked.min.js';

/** 动态注入单个脚本，返回加载完成的 Promise */
function injectScript(src) {
    return new Promise((resolve, reject) => {
        const script = document.createElement('script');
        script.src = src;
        script.onload = resolve;
        script.onerror = () => reject(new Error('无法加载 ' + src));
        document.head.appendChild(script);
    });
}

/** 轮询等待某个全局对象就绪，超时返回 false */
function waitForGlobal(name, timeoutMs) {
    if (typeof window[name] !== 'undefined') return Promise.resolve(true);
    return new Promise(resolve => {
        const deadline = Date.now() + timeoutMs;
        const tick = () => {
            if (typeof window[name] !== 'undefined') return resolve(true);
            if (Date.now() >= deadline) return resolve(false);
            setTimeout(tick, 25);
        };
        tick();
    });
}

/**
 * 确保 marked 就绪后再渲染。
 */
async function ensureMarked() {
    if (await waitForGlobal('marked', 2000)) return true;
    try {
        await injectScript(MARKED_SRC);
    } catch (e) {
        console.warn(e.message);
    }
    return typeof window.marked !== 'undefined';
}

/** 代码高亮属于渐进增强：hljs 排在 echarts 之后，必然晚于 marked，不应阻塞渲染 */
function highlightCodeBlocks() {
    if (typeof hljs === 'undefined') return false;
    markdownBody.querySelectorAll('pre code').forEach(block => {
        hljs.highlightElement(block);
    });
    return true;
}

/** KaTeX 公式渲染属于渐进增强：auto-render 就绪后对全文执行一次 */
function renderMathInDoc() {
    if (typeof renderMathInElement !== 'function') return false;
    try {
        renderMathInElement(markdownBody, {
            delimiters: [
                { left: '$$', right: '$$', display: true },
                { left: '$', right: '$', display: false },
                { left: '\\[', right: '\\]', display: true },
                { left: '\\(', right: '\\)', display: false }
            ],
            throwOnError: false
        });
        return true;
    } catch (e) {
        console.warn('公式渲染失败:', e);
        return false;
    }
}

/** 将 language-echarts 代码块替换为图表容器并初始化图表 */
function renderEChartsInDoc() {
    if (typeof echarts === 'undefined') return false;
    markdownBody.querySelectorAll('pre code.language-echarts').forEach(block => {
        const text = block.textContent || '';
        let config;
        try {
            config = JSON.parse(text);
        } catch (e) {
            console.warn('ECharts 配置解析失败:', e);
            return;
        }
        const placeholder = document.createElement('div');
        placeholder.className = 'echarts-container';
        const pre = block.closest('pre');
        if (pre) pre.replaceWith(placeholder);
        else block.replaceWith(placeholder);
        const chart = echarts.init(placeholder);
        chart.setOption(config);
        // 延迟 resize 确保 DOM 布局完成后尺寸正确
        setTimeout(() => chart.resize(), 100);
    });
    return true;
}

/** 窗口尺寸变化时重置所有已初始化的图表（仅注册一次） */
let echartsResizeBound = false;
function bindEChartsResize() {
    if (echartsResizeBound) return;
    echartsResizeBound = true;
    window.addEventListener('resize', () => {
        if (typeof echarts === 'undefined') return;
        markdownBody.querySelectorAll('.echarts-container').forEach(el => {
            const instance = echarts.getInstanceByDom(el);
            if (instance) instance.resize();
        });
    });
}

// ==== 加载文档 ====
async function loadDocument() {
    try {
        // 从 URL 查询参数获取目标 md 路径
        const params = new URLSearchParams(window.location.search);
        let targetUrl = params.get('url') || params.get('md');
        // URLSearchParams 已完成百分号解码，不可再次 decodeURIComponent：
        // 标题含 '%' 时它会抛 URIError，把整个加载流程打断在 try 之外。
        const title = params.get('title') || '工具文档';

        docTitle.textContent = title;

        if (!targetUrl) {
            showError('未指定文档路径');
            return;
        }

        // 协议相对地址（//host/x.md）会绕过下面的同源前缀拼装，必须拒绝
        if (targetUrl.startsWith('//')) {
            showError('非法的文档路径');
            return;
        }

        // 如果 url 参数已经是完整路径，直接使用；否则拼接 /file/read/package/ 前缀
        if (!targetUrl.startsWith('/')) {
            targetUrl = '/file/read/package/' + targetUrl;
        }

        const response = await fetch(targetUrl);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }

        const markdownText = await response.text();

        if (await ensureMarked()) {
            window.marked.setOptions({
                breaks: true,
                gfm: true
            });
            markdownBody.innerHTML = window.marked.parse(markdownText);
        } else {
            // 真降级：给出可见提示，避免把整篇原文静默伪装成一个代码块
            markdownBody.innerHTML =
                '<div class="error">' +
                '<i class="fas fa-exclamation-triangle"></i>' +
                '<p>Markdown 渲染库加载失败，以下为纯文本原文</p>' +
                '</div>' +
                `<pre style="white-space: pre-wrap; font-family: inherit;">${escapeHtml(markdownText)}</pre>`;
        }

        // 高亮 / 公式 / 图表均为渐进增强：晚到就补做一次，而不是永久放弃
        if (!highlightCodeBlocks()) {
            waitForGlobal('hljs', 20000).then(highlightCodeBlocks);
        }
        if (!renderMathInDoc()) {
            waitForGlobal('renderMathInElement', 20000).then(renderMathInDoc);
        }
        if (!renderEChartsInDoc()) {
            waitForGlobal('echarts', 20000).then(() => {
                if (renderEChartsInDoc()) bindEChartsResize();
            });
        }
        bindEChartsResize();

    } catch (error) {
        console.error('加载文档失败:', error);
        showError(`加载文档失败: ${error.message}`);
    }
}

// ==== 错误展示 ====
function showError(message) {
    markdownBody.innerHTML = `
        <div class="error">
            <i class="fas fa-exclamation-triangle"></i>
            <p>${escapeHtml(message)}</p>
        </div>
    `;
}

// ==== HTML 转义 ====
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}