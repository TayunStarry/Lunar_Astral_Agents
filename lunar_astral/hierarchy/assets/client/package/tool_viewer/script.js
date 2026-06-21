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

// ==== 加载文档 ====
async function loadDocument() {
    // 从 URL 查询参数获取目标 md 路径
    const params = new URLSearchParams(window.location.search);
    let targetUrl = params.get('url') || params.get('md');
    const title = params.get('title') || '工具文档';

    docTitle.textContent = decodeURIComponent(title);

    if (!targetUrl) {
        showError('未指定文档路径');
        return;
    }

    // 如果 url 参数已经是完整路径，直接使用；否则拼接 /file/read/package/ 前缀
    if (!targetUrl.startsWith('/')) {
        targetUrl = '/file/read/package/' + targetUrl;
    }

    try {
        const response = await fetch(targetUrl);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }

        const markdownText = await response.text();

        // 使用 marked.js 渲染（由 standard_dependency 已注入全局 marked）
        if (typeof marked !== 'undefined') {
            marked.setOptions({
                breaks: true,
                gfm: true
            });
            markdownBody.innerHTML = marked.parse(markdownText);
        } else {
            // 降级：纯文本展示
            markdownBody.innerHTML = `<pre style="white-space: pre-wrap; font-family: inherit;">${escapeHtml(markdownText)}</pre>`;
        }

        // 如果 highlight.js 可用，对代码块进行高亮
        if (typeof hljs !== 'undefined') {
            markdownBody.querySelectorAll('pre code').forEach(block => {
                hljs.highlightElement(block);
            });
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