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
 * standard_dependency 对依赖脚本是「发射后不管」的异步注入，直接读全局 marked 会与它竞态：
 * 文本文档抓取往往先于 39KB 的 marked.min.js 下载+编译完成，此时 typeof marked === 'undefined'，
 * 整篇文档就会被塞进一个 <pre>，看起来就是一整块代码。
 * 这里只等 marked 本身（它是依赖列表第一项），不等 echarts/katex 等无关的大库。
 */
async function ensureMarked() {
    if (await waitForGlobal('marked', 2000)) return true;

    // 兜底：不依赖 standard_dependency，自行加载
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

        // 使用 marked.js 渲染（由 standard_dependency 注入，必要时在此等待/兜底加载）
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

        // 高亮晚到就补做一次，而不是永久放弃
        if (!highlightCodeBlocks()) {
            waitForGlobal('hljs', 20000).then(highlightCodeBlocks);
        }

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