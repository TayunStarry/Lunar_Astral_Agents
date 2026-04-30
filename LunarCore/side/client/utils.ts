const BORDER_COLORS = [
	'var(--status-218838)',
	'var(--status-3a5a8a)',
	'var(--status-4a6fa5)',
	'var(--status-6c9bcf)',
	'var(--status-8a2be2)',
	'var(--status-9d6bff)',
	'var(--status-dc3545)',
	'var(--status-fbbf24)',
	'var(--status-ffc107)',
	'var(--status-20c997)',
	'var(--status-ff6b9c)',
];

/**
 * 获取随机边框颜色
 *
 * @returns {string} - CSS颜色值
 */
export function randomBorderColor(): string {
	return BORDER_COLORS[Math.floor(Math.random() * BORDER_COLORS.length)];
}

/**
 * HTML字符转义
 *
 * @param {string} text - 原始文本
 *
 * @returns {string} - 转义后的HTML安全字符串
 */
export function escapeHtml(text: string): string {
	const div = document.createElement('div');
	div.textContent = text;
	return div.innerHTML;
}

/**
 * 处理思考标签，将<think>转为可折叠的HTML
 *
 * @param {string} content - 原始内容
 *
 * @returns {string} - 处理后的HTML字符串
 */
export function processThinkTags(content: string): string {
	return content
		.replace(/<think>/gi, '<details class="think-block"><summary class="toggle_think_button">思考过程</summary>')
		.replace(/<\/think>/gi, '</details>');
}

/**
 * 渲染Markdown内容
 *
 * @param {string} content - Markdown文本
 *
 * @returns {Promise<string>} - 渲染后的HTML字符串
 */
export async function renderMarkdown(content: string): Promise<string> {
	if (window.marked) {
		let html = await window.marked.parse(content);
		html = processThinkTags(html);
		return html;
	}
	return escapeHtml(content);
}

/**
 * 代码高亮处理
 *
 * @param {HTMLElement} container - 包含代码块的容器
 */
export function highlightCode(container: HTMLElement): void {
	container.querySelectorAll('pre code').forEach((block) => {
		if (window.hljs) {
			window.hljs.highlightElement(block as HTMLElement);
		}
	});
}

/**
 * 渲染Mermaid图表
 *
 * @param {HTMLElement} container - 包含Mermaid代码块的容器
 *
 * @returns {Promise<void>}
 */
export async function renderMermaid(container: HTMLElement): Promise<void> {
	const mermaidBlocks = container.querySelectorAll('code.language-mermaid');

	for (const block of Array.from(mermaidBlocks)) {
		const textContent = block.textContent || '';
		if (textContent.length <= 20) continue;

		try {
			const graphDefinition = textContent;
			await window.mermaid.parse(graphDefinition);

			const id = `mermaid-${Date.now()}-${Math.floor(Math.random() * 10000)}`;
			const { svg } = await window.mermaid.render(id, graphDefinition);

			const mermaidContainer = document.createElement('div');
			mermaidContainer.className = 'mermaid-container';

			const parser = new DOMParser();
			const doc = parser.parseFromString(svg, 'image/svg+xml');
			const svgElement = doc.documentElement;

			const chartType = svgElement.getAttribute('aria-roledescription');
			if (chartType === 'flowchart' || chartType === 'classDiagram') {
				const viewBox = svgElement.getAttribute('viewBox');
				if (viewBox) {
					const values = viewBox.split(/\s+/).map(parseFloat);
					if (values.length === 4 && values.every(v => !isNaN(v))) {
						if (chartType === 'flowchart') {
							values[0] *= 0.45;
							values[1] *= 0.45;
							values[2] *= 1.05;
							values[3] *= 1.05;
						} else {
							values[0] *= 0;
							values[1] *= 0.35;
							values[2] *= 1.05;
							values[3] *= 1.25;
						}
						svgElement.setAttribute('viewBox', values.join(' '));
					}
				}
			}

			const modifiedSVG = new XMLSerializer().serializeToString(svgElement);
			mermaidContainer.innerHTML = `<div style="width: 100%; border: 10px dashed #eee; padding: 0px">${modifiedSVG}</div>`;

			const parent = block.parentElement;
			if (parent) {
				parent.insertBefore(mermaidContainer, block);
				parent.removeChild(block);
			}
		} catch (error) {
			console.error('Mermaid rendering error:', error);
			const errorContainer = document.createElement('div');
			errorContainer.className = 'mermaid-error';
			errorContainer.textContent = error instanceof Error ? error.message : 'Mermaid rendering failed';
			const parent = block.parentElement;
			if (parent) {
				parent.insertBefore(errorContainer, block);
			}
		}
	}
}

/**
 * 渲染ECharts图表
 *
 * @param {HTMLElement} container - 包含ECharts占位符的容器
 */
export function renderECharts(container: HTMLElement): void {
	container.querySelectorAll('.echarts-placeholder').forEach(async (placeholder) => {
		try {
			const chartData = placeholder.getAttribute('data-chart');
			if (!chartData) return;

			const config = JSON.parse(chartData);
			const chartContainer = document.createElement('div');
			chartContainer.style.width = '100%';
			chartContainer.style.height = '400px';

			placeholder.appendChild(chartContainer);

			const chart = window.echarts.init(chartContainer);
			chart.setOption(config);
		} catch (error) {
			console.error('ECharts rendering error:', error);
		}
	});
}

/**
 * 渲染数学公式（KaTeX）
 *
 * @param {HTMLElement} container - 包含数学公式的容器
 *
 * @returns {Promise<void>}
 */
export async function renderMath(container: HTMLElement): Promise<void> {
	if (window.katex && window.renderMathInElement) {
		try {
			window.renderMathInElement(container, {
				delimiters: [
					{ left: '$$', right: '$$', display: true },
					{ left: '$', right: '$', display: false },
					{ left: '\\[', right: '\\]', display: true },
					{ left: '\\(', right: '\\)', display: false },
				],
				throwOnError: false,
			});
		} catch (error) {
			console.error('KaTeX rendering error:', error);
		}
	}
}

/**
 * 移除代码注释
 *
 * @param {string} text - 源代码文本
 *
 * @returns {string} - 移除注释后的文本
 */
export function removeCodeComments(text: string): string {
	return text
		.replace(/\/\/.*$/gm, '')
		.replace(/\/\*[\s\S]*?\*\//g, '')
		.replace(/'/g, '"');
}

/**
 * 编码文件名（用于HTTP传输）
 *
 * @param {string} filename - 原始文件名
 *
 * @returns {string} - Base64编码后的文件名
 */
export function encodeFileName(filename: string): string {
	return btoa(unescape(encodeURIComponent(filename)));
}

/**
 * Blob转换为Base64字符串
 *
 * @param {Blob} blob - Blob对象
 *
 * @returns {Promise<string>} - Base64编码字符串（不含data URI前缀）
 */
export async function blobToBase64(blob: Blob): Promise<string> {
	return new Promise((resolve, reject) => {
		const reader = new FileReader();
		reader.onload = () => {
			const result = reader.result as string;
			resolve(result.split(',')[1] || '');
		};
		reader.onerror = reject;
		reader.readAsDataURL(blob);
	});
}

/**
 * 清空容器内容
 *
 * @param {HTMLElement} container - 目标容器
 */
export function clearContainer(container: HTMLElement): void {
	container.innerHTML = '';
}

/**
 * 预览图片（创建全屏遮罩查看器）
 *
 * @param {string} url - 图片URL
 * @param {string} alt - 图片描述文字
 */
export function previewImage(url: string, alt: string): void {
	const overlay = document.createElement('div');
	overlay.style.cssText = `
    position: fixed;
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
    background: rgba(0, 0, 0, 0.9);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 9999;
    cursor: pointer;
  `;

	const img = document.createElement('img');
	img.src = url;
	img.style.cssText = 'max-width: 90%; max-height: 90%; object-fit: contain;';

	overlay.appendChild(img);
	overlay.onclick = () => overlay.remove();
	document.body.appendChild(overlay);
}
