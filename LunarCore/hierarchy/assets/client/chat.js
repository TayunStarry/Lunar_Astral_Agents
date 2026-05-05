import { randomBorderColor, escapeHtml, clearContainer } from './util.js';
import { TTS } from './tts.js';

function processThinkTags(content) {
    return content
        .replace(/<think>/gi, '<details class="think-block"><summary class="toggle_think_button">思考过程</summary>')
        .replace(/<\/think>/gi, '</details>');
}

async function renderMarkdown(content) {
    if (window.marked) {
        let html = await window.marked.parse(content);
        html = processThinkTags(html);
        return html;
    }
    return escapeHtml(content);
}

function highlightCode(container) {
    container.querySelectorAll('pre code').forEach((block) => {
        if (window.hljs) {
            window.hljs.highlightElement(block);
        }
    });
}

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
        }
        catch (error) {
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

function renderECharts(container) {
    // 将 code.language-echarts 转换为 echarts-placeholder
    const echartsCodeBlocks = container.querySelectorAll('pre code.language-echarts');
    echartsCodeBlocks.forEach(block => {
        try {
            const text = block.textContent || '';
            // 验证是否为有效 JSON（ECharts 配置）
            JSON.parse(text);
            const placeholder = document.createElement('div');
            placeholder.className = 'echarts-placeholder';
            placeholder.setAttribute('data-chart', text);
            const pre = block.parentElement;
            if (pre) {
                pre.parentElement?.replaceChild(placeholder, pre);
            }
        } catch (e) {
            console.error('Invalid ECharts config:', e);
        }
    });

    // 原有的占位符初始化逻辑
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
            // 保存图表实例以便后续清理
            placeholder._echartsInstance = chart;
            // 延迟调用 resize 确保 DOM 布局完成
            setTimeout(() => {
                chart.resize();
            }, 100);
            // 窗口大小变化时重新调整
            window.addEventListener('resize', () => chart.resize());
        } catch (error) {
            console.error('ECharts rendering error:', error);
        }
    });
}

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
/** 从视频 URL 获取视频缩略图 URL */
async function getVideoThumbnailFromUrl(videoUrl) {
    return new Promise((resolve, reject) => {
        const video = document.createElement('video');
        video.preload = 'metadata';
        video.muted = true;
        video.crossOrigin = 'anonymous';
        video.onloadeddata = () => {
            video.currentTime = 1;
        };
        video.onseeked = () => {
            const canvas = document.createElement('canvas');
            canvas.width = video.videoWidth || 640;
            canvas.height = video.videoHeight || 360;
            const ctx = canvas.getContext('2d');
            if (ctx) {
                ctx.drawImage(video, 0, 0, canvas.width, canvas.height);
                resolve(canvas.toDataURL('image/jpeg'));
            } else {
                reject(new Error('Failed to get video context'));
            }
            URL.revokeObjectURL(video.src);
        };
        video.onerror = () => {
            URL.revokeObjectURL(video.src);
            reject(new Error('Failed to load video'));
        };
        video.src = videoUrl;
    });
}

function isVideoUrl(url) {
    const videoExtensions = ['.mp4', '.webm', '.ogg', '.mov', '.avi', '.flv'];
    const lowerUrl = url.toLowerCase();
    return videoExtensions.some(ext => lowerUrl.endsWith(ext));
}

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

    // 添加扬声器按钮（仅助理消息）
    if (message.role === 'assistant' && message.audioBase64) {
        const speakerBtn = document.createElement('button');
        speakerBtn.className = 'speaker-btn';
        speakerBtn.innerHTML = '<i class="fas fa-volume-up"></i>';
        speakerBtn.onclick = () => {
            const arrayBuffer = TTS.base64ToArrayBuffer(message.audioBase64);
            TTS.playAudioBuffer(arrayBuffer);
        };
        header.appendChild(speakerBtn);
    }

    const contentDiv = document.createElement('div');
    contentDiv.className = 'message-content';

    messageElement.appendChild(header);
    messageElement.appendChild(contentDiv);
    return messageElement;
}

export async function renderMessage(message, container) {
    const messageElement = createMessageElement(message);
    const contentDiv = messageElement.querySelector('.message-content');

    if (message.imageUrls && message.imageUrls.length > 0) {
        const mediaContainer = document.createElement('div');
        mediaContainer.className = 'images-container';
        for (const mediaUrl of message.imageUrls) {
            const mediaContainerItem = document.createElement('div');
            mediaContainerItem.className = 'labeled-image-container';

            if (isVideoUrl(mediaUrl)) {
                try {
                    const thumbnail = await getVideoThumbnailFromUrl(mediaUrl);
                    const img = document.createElement('img');
                    img.src = thumbnail;
                    img.className = 'image-just-drawn video-thumbnail';
                    img.alt = '视频封面';
                    img.style.cursor = 'pointer';
                    img.onclick = () => window.previewImage?.(mediaUrl, '视频');

                    const playIcon = document.createElement('div');
                    playIcon.className = 'play-icon-overlay';
                    playIcon.innerHTML = '<i class="fas fa-play"></i>';
                    mediaContainerItem.appendChild(img);
                    mediaContainerItem.appendChild(playIcon);
                } catch (err) {
                    console.warn('Failed to get video thumbnail:', err);
                    const img = document.createElement('img');
                    img.src = `/read/resources/placeholder/blank-0${Math.floor(Math.random() * 3)}.png`;
                    img.className = 'image-just-drawn';
                    img.alt = '视频';
                    img.onclick = () => window.previewImage?.(mediaUrl, '视频');
                    mediaContainerItem.appendChild(img);
                }
            } else {
                const img = document.createElement('img');
                img.src = mediaUrl;
                img.className = 'image-just-drawn';
                img.alt = typeof message.content === 'string' ? message.content : '图片';
                img.onerror = () => {
                    img.src = `/read/resources/placeholder/blank-0${Math.floor(Math.random() * 3)}.png`;
                };
                img.onclick = () => window.previewImage?.(mediaUrl, typeof message.content === 'string' ? message.content : '图片');
                mediaContainerItem.appendChild(img);
            }
            mediaContainer.appendChild(mediaContainerItem);
        }
        contentDiv.appendChild(mediaContainer);
    } else if (message.imageUrl) {
        const imgContainer = document.createElement('div');
        imgContainer.className = 'labeled-image-container';

        if (isVideoUrl(message.imageUrl)) {
            try {
                const thumbnail = await getVideoThumbnailFromUrl(message.imageUrl);
                const img = document.createElement('img');
                img.src = thumbnail;
                img.className = 'image-just-drawn video-thumbnail';
                img.alt = '视频封面';
                img.style.cursor = 'pointer';
                img.onclick = () => window.previewImage?.(message.imageUrl, '视频');

                const playIcon = document.createElement('div');
                playIcon.className = 'play-icon-overlay';
                playIcon.innerHTML = '<i class="fas fa-play"></i>';
                imgContainer.appendChild(img);
                imgContainer.appendChild(playIcon);
            } catch (err) {
                console.warn('Failed to get video thumbnail:', err);
                const img = document.createElement('img');
                img.src = `/read/resources/placeholder/blank-0${Math.floor(Math.random() * 3)}.png`;
                img.className = 'image-just-drawn';
                img.alt = '视频';
                img.onclick = () => window.previewImage?.(message.imageUrl, '视频');
                imgContainer.appendChild(img);
            }
        } else {
            const img = document.createElement('img');
            img.src = message.imageUrl;
            img.className = 'image-just-drawn';
            img.alt = typeof message.content === 'string' ? message.content : '图片';
            img.onerror = () => {
                img.src = `/read/resources/placeholder/blank-0${Math.floor(Math.random() * 3)}.png`;
            };
            img.onclick = () => window.previewImage?.(message.imageUrl, typeof message.content === 'string' ? message.content : '图片');
            imgContainer.appendChild(img);
        }
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

export async function renderAllMessages(container, messages, clearFirst = true) {
    if (clearFirst) {
        clearContainer(container);
    }
    for (const message of messages) {
        await renderMessage(message, container);
    }
}