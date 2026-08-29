// ============================================================
//  星月智能 · 消息终端 — Markdown / 图表 / 数学渲染
// ============================================================

// ---------- Markdown / 图表 / 数学渲染 ----------
async function ensureMarked() {
    if (window.marked) return true;
    for (let i = 0; i < 50; i++) {
        await new Promise(r => setTimeout(r, 100));
        if (window.marked) return true;
    }
    return false;
}

function initMermaid() {
    if (mermaidInitialized || !window.mermaid) return;
    window.mermaid.initialize({
        startOnLoad: false,
        theme: isDarkMode ? 'dark' : 'default',
        securityLevel: 'loose',
        fontFamily: 'inherit'
    });
    mermaidInitialized = true;
}

function processThinkTags(html) {
    return html.replace(/<think>([\s\S]*?)<\/think>/gi, (match, content) => {
        return `<div class="think-block"><div class="think-summary"><i class="fas fa-chevron-right toggle-icon"></i> 思考过程</div><div class="think-content">${content}</div></div>`;
    });
}

async function renderMarkdown(rawContent) {
    let html = processThinkTags(rawContent);
    if (window.marked) {
        html = await window.marked.parse(html);
    } else {
        html = '<p>' + escapeHtml(rawContent).replace(/\n/g, '<br>') + '</p>';
    }
    return html;
}

function highlightCode(container) {
    if (!window.hljs) return;
    container.querySelectorAll('pre code').forEach((block) => {
        if (block.parentElement && block.parentElement.classList.contains('hljs')) return;
        const langClass = Array.from(block.classList).find(c => c.startsWith('language-'));
        if (langClass && (langClass === 'language-echarts' || langClass === 'language-mermaid')) return;
        try {
            window.hljs.highlightElement(block);
        } catch (e) {
            console.warn('代码高亮失败', e);
        }
    });
}

function renderECharts(container) {
    if (!window.echarts) return;
    container.querySelectorAll('pre code.language-echarts').forEach((block) => {
        try {
            const clean = block.textContent.trim();
            const config = JSON.parse(clean);
            const chartDiv = document.createElement('div');
            chartDiv.className = 'echarts-container';
            const inner = document.createElement('div');
            inner.style.width = '100%';
            inner.style.height = '100%';
            chartDiv.appendChild(inner);
            const pre = block.parentElement;
            if (pre && pre.tagName === 'PRE') {
                pre.replaceWith(chartDiv);
            } else {
                block.replaceWith(chartDiv);
            }
            const chart = window.echarts.init(inner);
            chart.setOption(config);
            chartDiv._echartsInstance = chart;
            setTimeout(() => chart.resize(), 100);
            window.addEventListener('resize', () => chart.resize());
        } catch (e) {
            console.warn('ECharts 渲染失败', e);
        }
    });
}

async function renderMermaid(container) {
    if (!window.mermaid || !mermaidInitialized) return;
    const blocks = Array.from(container.querySelectorAll('pre code.language-mermaid'));
    for (const block of blocks) {
        const code = (block.textContent || '').trim();
        if (!code) continue;
        try {
            await window.mermaid.parse(code);
            const id = `mermaid-${Date.now()}-${Math.floor(Math.random() * 10000)}`;
            const { svg } = await window.mermaid.render(id, code);
            const wrapper = document.createElement('div');
            wrapper.className = 'mermaid-container';
            wrapper.innerHTML = svg;
            const pre = block.parentElement;
            if (pre && pre.tagName === 'PRE') {
                pre.replaceWith(wrapper);
            } else {
                block.replaceWith(wrapper);
            }
        } catch (e) {
            console.error('Mermaid 渲染失败', e);
            const errDiv = document.createElement('div');
            errDiv.className = 'mermaid-error';
            errDiv.textContent = `Mermaid 渲染失败：${e.message || String(e)}`;
            const pre = block.parentElement;
            if (pre && pre.tagName === 'PRE') {
                pre.replaceWith(errDiv);
            } else {
                block.replaceWith(errDiv);
            }
        }
    }
}

function renderMath(container) {
    if (typeof window.renderMathInElement === 'function') {
        try {
            window.renderMathInElement(container, {
                delimiters: [
                    { left: '$$', right: '$$', display: true },
                    { left: '$', right: '$', display: false },
                    { left: '\\(', right: '\\)', display: false },
                    { left: '\\[', right: '\\]', display: true }
                ],
                throwOnError: false
            });
        } catch (e) {
            console.warn('KaTeX 渲染失败', e);
        }
    }
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
    contentDiv.querySelectorAll('table').forEach(t => t.classList.add('markdown-table'));
    highlightCode(contentDiv);
    renderECharts(contentDiv);
    await renderMermaid(contentDiv);
    renderMath(contentDiv);
}
