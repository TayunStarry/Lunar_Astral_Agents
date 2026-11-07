/**
 * 星月智能 · 桌面端 / 移动端共享 Markdown 渲染
 *
 * Markdown 解析、think 块转换、代码高亮、ECharts / Mermaid / KaTeX 增强。
 * 各端按需加载对应库：库不存在时对应增强自动跳过（移动端只做基础 Markdown）。
 */
import { escapeHtml } from './utils.js';

let mermaidInitialized = false;

/** 等待 marked 就绪（桌面端历史脚本加载顺序兜底） */
export async function ensureMarked(retries = 50, interval = 100) {
	if (window.marked) return true;
	for (let i = 0; i < retries; i++) {
		await new Promise(r => setTimeout(r, interval));
		if (window.marked) return true;
	}
	return false;
}

/** 初始化 Mermaid（主题跟随各端传入） */
export function initMermaid({ dark = false, force = false } = {}) {
	if (!window.mermaid) return false;
	if (mermaidInitialized && !force) return true;
	window.mermaid.initialize({
		startOnLoad: false,
		theme: dark ? 'dark' : 'default',
		securityLevel: 'loose',
		fontFamily: 'inherit'
	});
	mermaidInitialized = true;
	return true;
}

export function isMermaidInitialized() {
	return mermaidInitialized;
}

/** 将 <think>…</think> 转为可折叠的思考过程块 */
export function processThinkTags(html) {
	return html.replace(/<think>([\s\S]*?)<\/think>/gi, (match, content) => {
		return `<div class="think-block"><div class="think-summary"><i class="fas fa-chevron-right toggle-icon"></i> 思考过程</div><div class="think-content">${content}</div></div>`;
	});
}

/**
 * Markdown → HTML
 * @param {string} rawContent
 * @param {{processThink?:boolean}} [options]
 */
export async function renderMarkdown(rawContent, { processThink = true } = {}) {
	let html = processThink ? processThinkTags(rawContent) : rawContent;
	if (window.marked) {
		html = await window.marked.parse(html);
	} else {
		html = '<p>' + escapeHtml(rawContent).replace(/\n/g, '<br>') + '</p>';
	}
	return html;
}

/** 同步 Markdown 渲染（移动端历史列表等同步构建 DOM 的场景） */
export function renderMarkdownSync(rawContent, { processThink = true } = {}) {
	const source = processThink ? processThinkTags(rawContent) : rawContent;
	if (!window.marked) {
		return '<p>' + escapeHtml(rawContent).replace(/\n/g, '<br>') + '</p>';
	}
	const html = window.marked.parse(source);
	if (typeof html !== 'string') throw new Error('marked.parse 未返回同步字符串');
	return html;
}

export function highlightCode(container) {
	if (!window.hljs) return;
	container.querySelectorAll('pre code').forEach(block => {
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

export function renderECharts(container) {
	if (!window.echarts) return;
	container.querySelectorAll('pre code.language-echarts').forEach(block => {
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
			if (pre && pre.tagName === 'PRE') pre.replaceWith(chartDiv);
			else block.replaceWith(chartDiv);
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

export async function renderMermaid(container) {
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
			if (pre && pre.tagName === 'PRE') pre.replaceWith(wrapper);
			else block.replaceWith(wrapper);
		} catch (e) {
			console.error('Mermaid 渲染失败', e);
			const errDiv = document.createElement('div');
			errDiv.className = 'mermaid-error';
			errDiv.textContent = `Mermaid 渲染失败：${e.message || String(e)}`;
			const pre = block.parentElement;
			if (pre && pre.tagName === 'PRE') pre.replaceWith(errDiv);
			else block.replaceWith(errDiv);
		}
	}
}

export function renderMath(container) {
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

/** 对已插入 DOM 的内容统一做增强（表格样式 / 高亮 / 图表 / Mermaid / 数学） */
export async function enhanceContent(container, { tables = true } = {}) {
	if (!container) return;
	if (tables) container.querySelectorAll('table').forEach(t => t.classList.add('markdown-table'));
	highlightCode(container);
	renderECharts(container);
	await renderMermaid(container);
	renderMath(container);
}
