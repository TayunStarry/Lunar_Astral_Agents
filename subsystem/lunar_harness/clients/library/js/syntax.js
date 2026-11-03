// syntax.js 变更视图与预览视图的语法高亮：复用 /reliance/highlight 的 highlight.js，未知语言安全回退。

const MAX_CACHE = 24;
const MAX_LINE_CACHE = 8;
// 预览是整份文件：超过该长度不再高亮，避免一次高亮长时间阻塞界面（视觉上退回纯文本）。
const MAX_HIGHLIGHT_CHARS = 120000;
// 只匹配 highlight.js 实际产出的 <span ...> / </span>，标签内不会出现换行。
const TAG_RE = /<(\/?)([A-Za-z][A-Za-z0-9-]*)(?:\s[^>]*)?>/g;

// 扩展名 → highlight.js 语言名（仅覆盖资源库常见代码文件）
const LANGUAGES = {
    '.go': 'go',
    '.ts': 'typescript', '.tsx': 'typescript', '.mts': 'typescript', '.cts': 'typescript',
    '.js': 'javascript', '.mjs': 'javascript', '.cjs': 'javascript', '.jsx': 'javascript',
    '.html': 'xml', '.htm': 'xml', '.xml': 'xml', '.svg': 'xml', '.vue': 'xml',
    '.css': 'css', '.scss': 'css', '.less': 'css',
    '.json': 'json', '.jsonl': 'json', '.ndjson': 'json',
    '.md': 'markdown', '.markdown': 'markdown',
    '.cpp': 'cpp', '.cc': 'cpp', '.cxx': 'cpp', '.hpp': 'cpp', '.hh': 'cpp', '.hxx': 'cpp',
    '.c': 'cpp', '.h': 'cpp',
};

const cache = new Map();     // 变更视图片段：语言 + 文本 → 高亮 HTML
const lineCache = new Map(); // 预览视图整文件：语言 + 文本 → 按行拆分后的 HTML 数组

// languageOf 按文件路径推断 highlight.js 语言名；未知类型返回空串。
export function languageOf(path) {
    const dot = path.lastIndexOf('.');
    return dot < 0 ? '' : (LANGUAGES[path.slice(dot).toLowerCase()] || '');
}

// highlightRaw 调用一次 highlight.js 返回整段 HTML；缺库 / 未知语言 / 异常均返回 null。
function highlightRaw(text, language) {
    const hljs = globalThis.hljs;
    if (!hljs || typeof hljs.highlight !== 'function') return null;
    try {
        if (!hljs.getLanguage(language)) return null;
        return hljs.highlight(text, { language, ignoreIllegals: true }).value;
    } catch {
        return null;
    }
}

// remember 写入定长缓存并返回写入值。
function remember(map, key, value, limit) {
    if (map.size >= limit) map.delete(map.keys().next().value);
    map.set(key, value);
    return value;
}

// highlightCode 变更视图的单段代码：整段只高亮一次并缓存，失败返回 null 交由调用方回退纯文本。
export function highlightCode(text, language) {
    if (!text || !language) return null;
    const key = `${language}\u0000${text}`;
    const cached = cache.get(key);
    if (cached) return cached;
    const html = highlightRaw(text, language);
    if (html === null) return null;
    return remember(cache, key, html, MAX_CACHE);
}

// highlightLines 预览视图：整份文件只高亮一次，再按行拆成若干段自闭合 HTML。
// 跨行的 span 会在行尾闭合、下一行行首重新打开；无法安全拆分时该行回退为转义纯文本。
// 未知语言 / 超大文件 / 拆分失败返回 null，调用方走原有纯文本渲染。
export function highlightLines(text, language) {
    if (!text || !language) return null;
    if (text.length > MAX_HIGHLIGHT_CHARS) return null;
    const key = `${language}\u0000${text}`;
    const cached = lineCache.get(key);
    if (cached) return cached;
    const html = highlightRaw(text, language);
    if (html === null) return null;
    const rows = splitHighlighted(html, text);
    if (!rows) return null;
    return remember(lineCache, key, rows, MAX_LINE_CACHE);
}

// splitHighlighted 把整段高亮 HTML 按换行拆成每行自闭合片段。
// 维护跨行未闭合标签栈：行首重开、行尾补闭合；遇到多余闭合标签则该行整体转义回退。
function splitHighlighted(html, plain) {
    const htmlLines = html.split('\n');
    const plainLines = plain.split('\n');
    if (htmlLines.length !== plainLines.length) return null;
    const stack = [];
    const rows = new Array(htmlLines.length);
    for (let i = 0; i < htmlLines.length; i += 1) {
        const line = htmlLines[i];
        const prefix = stack.map((tag) => tag.raw).join('');
        let body = '';
        let last = 0;
        let unsafe = false;
        TAG_RE.lastIndex = 0;
        let match = TAG_RE.exec(line);
        while (match) {
            body += line.slice(last, match.index);
            last = TAG_RE.lastIndex;
            if (match[1] === '/') {
                // 闭合标签找不到对应开标签：本行结构不可信，整行回退纯文本
                if (stack.length === 0) {
                    unsafe = true;
                    break;
                }
                stack.pop();
            } else {
                stack.push({ raw: match[0], name: match[2].toLowerCase() });
            }
            body += match[0];
            match = TAG_RE.exec(line);
        }
        if (unsafe) {
            stack.length = 0;
            rows[i] = escapeHtml(plainLines[i]);
            continue;
        }
        body += line.slice(last);
        let suffix = '';
        for (let s = stack.length - 1; s >= 0; s -= 1) {
            suffix += `</${stack[s].name}>`;
        }
        rows[i] = prefix + body + suffix;
    }
    return rows;
}

// escapeHtml 纯文本转义，用于单行兜底，保证不会注入破损标签。
function escapeHtml(text) {
    return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}
