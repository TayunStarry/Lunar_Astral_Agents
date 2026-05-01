import { randomBorderColor, escapeHtml, clearContainer } from './utils.js';
import { EmotionalStateEnum } from './live2dManager.js';

/**
 * 处理思考标签，将<think>转为可折叠的HTML
 *
 * @param {string} content - 原始内容
 *
 * @returns {string} - 处理后的HTML字符串
 */
function processThinkTags(content) {
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
async function renderMarkdown(content) {
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
function highlightCode(container) {
    container.querySelectorAll('pre code').forEach((block) => {
        if (window.hljs) {
            window.hljs.highlightElement(block);
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
async function renderMermaid(container) {
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
function renderECharts(container) {
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
async function renderMath(container) {
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
 * 创建消息DOM元素
 *
 * @param {Message} message - 消息对象
 *
 * @returns {HTMLElement} - 消息元素
 */
function createMessageElement(message) {
    const messageElement = document.createElement('div');
    messageElement.classList.add('message');
    if (message.role === 'user') {
        messageElement.classList.add('user-message');
    } else {
        messageElement.classList.add('assistant-message');
    }
    messageElement.style.borderColor = randomBorderColor();

    const header = document.createElement('div');
    header.className = 'message-header';
    header.textContent = message.role === 'user' ? '你' : '月华';

    const contentDiv = document.createElement('div');
    contentDiv.className = 'message-content';

    messageElement.appendChild(header);
    messageElement.appendChild(contentDiv);
    return messageElement;
}

/**
 * 渲染单条消息到容器
 *
 * @param {Message} message - 消息对象
 * @param {HTMLElement} container - 目标容器元素
 *
 * @returns {Promise<HTMLElement>} - 渲染后的消息元素
 */
export async function renderMessage(message, container) {
    const messageElement = createMessageElement(message);
    const contentDiv = messageElement.querySelector('.message-content');

    if (message.imageUrls && message.imageUrls.length > 0) {
        const imagesContainer = document.createElement('div');
        imagesContainer.className = 'images-container';
        for (const imageUrl of message.imageUrls) {
            const imgContainer = document.createElement('div');
            imgContainer.className = 'labeled-image-container';
            const img = document.createElement('img');
            img.src = imageUrl;
            img.className = 'image-just-drawn';
            img.alt = typeof message.content === 'string' ? message.content : '图片';
            img.onerror = () => {
                img.src = `/read/resources/placeholder/blank-0${Math.floor(Math.random() * 3)}.png`;
            };
            img.onclick = () => window.previewImage?.(imageUrl, typeof message.content === 'string' ? message.content : '图片');
            imgContainer.appendChild(img);
            imagesContainer.appendChild(imgContainer);
        }
        contentDiv.appendChild(imagesContainer);
    } else if (message.imageUrl) {
        const imgContainer = document.createElement('div');
        imgContainer.className = 'labeled-image-container';
        const img = document.createElement('img');
        img.src = message.imageUrl;
        img.className = 'image-just-drawn';
        img.alt = typeof message.content === 'string' ? message.content : '图片';
        img.onerror = () => {
            img.src = `/read/resources/placeholder/blank-0${Math.floor(Math.random() * 3)}.png`;
        };
        img.onclick = () => window.previewImage?.(message.imageUrl, typeof message.content === 'string' ? message.content : '图片');
        imgContainer.appendChild(img);
        contentDiv.appendChild(imgContainer);
    }

    if (typeof message.content === 'string' && message.content) {
        const markdownContent = document.createElement('div');
        markdownContent.className = 'markdown-content';
        markdownContent.innerHTML = await renderMarkdown(message.content);
        contentDiv.appendChild(markdownContent);
        highlightCode(markdownContent);
        await renderMermaid(markdownContent);
        renderECharts(markdownContent);
        await renderMath(markdownContent);
    }

    container.appendChild(messageElement);
    container.scrollTo({ top: container.scrollHeight, behavior: 'smooth' });
    return messageElement;
}

/**
 * 渲染所有消息到容器
 *
 * @param {HTMLElement} container - 目标容器元素
 * @param {Message[]} messages - 消息数组
 * @param {boolean} [clearFirst=true] - 是否先清空容器
 *
 * @returns {Promise<void>}
 */
export async function renderAllMessages(container, messages, clearFirst = true) {
    if (clearFirst) {
        clearContainer(container);
    }
    for (const message of messages) {
        await renderMessage(message, container);
    }
}