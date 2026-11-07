// ============================================================
//  星月智能 · 消息终端 — Markdown / 图表 / 数学渲染（共享层适配）
//  解析与增强逻辑在 /shared/core/markdown.js，
//  这里仅保留桌面端「文件引用占位 → 按钮」的渲染流程。
// ============================================================

// ---------- Markdown / 图表 / 数学渲染 ----------
async function ensureMarked() {
    return window.LunarShared.markdown.ensureMarked();
}

function initMermaid() {
    const ready = window.LunarShared.markdown.initMermaid({
        dark: isDarkMode,
        // 主题切换时 mermaidInitialized 已被置 false，需要强制重新初始化
        force: !mermaidInitialized
    });
    if (ready) mermaidInitialized = true;
}

function processThinkTags(html) {
    return window.LunarShared.markdown.processThinkTags(html);
}

async function renderMarkdown(rawContent) {
    return window.LunarShared.markdown.renderMarkdown(rawContent, { processThink: true });
}

function highlightCode(container) {
    window.LunarShared.markdown.highlightCode(container);
}

function renderECharts(container) {
    window.LunarShared.markdown.renderECharts(container);
}

async function renderMermaid(container) {
    await window.LunarShared.markdown.renderMermaid(container);
}

function renderMath(container) {
    window.LunarShared.markdown.renderMath(container);
}

async function fillMarkdownContent(el, content) {
    const contentDiv = el.querySelector('.markdown-content');
    if (!contentDiv || !content) return;
    contentDiv.innerHTML = '<i class="fas fa-spinner fa-pulse"></i> 加载中...';
    // 文件引用格式统一为 `(#name):`。它是普通括号文本，markdown 解析天然保留，
    // 故无需占位绕过（`[x]:` 才会触发参考式链接定义）。渲染前占位、渲染后还原为按钮。
    const refs = [];
    const tokenContent = content.replace(/\((#[A-Za-z0-9_.\-]+)\)(?::)?/g, (m, name) => {
        refs.push('#' + name.replace(/^#/, ''));
        return '\uE000' + (refs.length - 1) + '\uE001';
    });
    let html = await renderMarkdown(tokenContent);
    html = html.replace(/\uE000(\d+)\uE001/g, (match, idx) => {
        const ref = refs[+idx];
        const name = ref.slice(1);
        return `<span class="file-ref-inline" data-ref="${ref}" title="点击载入文件引用 ${ref}"><i class="fas fa-file-alt"></i><span class="file-ref-inline-name">${name}</span></span>`;
    });
    contentDiv.innerHTML = html;
    contentDiv.querySelectorAll('.file-ref-inline').forEach(btn => {
        btn.addEventListener('click', () => addFileReference(btn.dataset.ref));
    });
    await window.LunarShared.markdown.enhanceContent(contentDiv);
}
