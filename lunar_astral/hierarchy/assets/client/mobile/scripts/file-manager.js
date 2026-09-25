import {
    createFilePreview,
    getFileCategory,
    revokeAllFilePreviews,
    getVideoThumbnail,
    formatFileSize,
    fileToRawBase64,
    getAudioFormat,
} from './file.js';
import { saveFile } from './fetch.js';

/**
 * 待发送附件管理器
 *
 * 负责移动端文件的选择、悬浮气泡预览渲染、删除与上传，
 * 以及把附件转换为 OpenAI 兼容的消息内容块（与电脑端 send.js 规则一致）。
 */
export class FilePreviewManager {
    /** @type {Array<{file: File, url: string, type: string, name: string}>} 待发送附件预览 */
    previews = [];
    previewArea = null;
    showError = null;
    /** 附件数量变化回调（用于切换发送 / 文件按钮） */
    onChange = null;

    /**
     * @param {HTMLElement} previewArea - 悬浮气泡预览区 DOM 元素
     * @param {(msg: string) => void} showError - 错误提示回调
     * @param {() => void} onChange - 附件变化回调
     */
    constructor(previewArea, showError, onChange) {
        this.previewArea = previewArea;
        this.showError = showError;
        this.onChange = onChange;
    }

    /** 处理文件选择：生成预览气泡并追加到预览区 */
    handleFileSelect(files) {
        for (const file of files) {
            const preview = createFilePreview(file);
            this.previews.push(preview);
            this.renderFilePreview(preview);
        }
        if (this.onChange) this.onChange();
    }

    /** 渲染单个附件气泡（图片 / 视频显示缩略图，其余显示类型图标） */
    renderFilePreview(preview) {
        const item = document.createElement('div');
        item.className = 'file-preview-item';
        item.dataset.name = preview.name;

        if (preview.type === 'image') {
            const img = document.createElement('img');
            img.src = preview.url;
            img.alt = preview.name;
            item.appendChild(img);
        }
        else if (preview.type === 'video') {
            const video = document.createElement('video');
            video.src = preview.url;
            video.muted = true;
            item.appendChild(video);
            getVideoThumbnail(preview.file).then(thumbnail => {
                const img = document.createElement('img');
                img.src = thumbnail;
                img.alt = preview.name;
                item.insertBefore(img, video);
                video.style.display = 'none';
            }).catch(() => {
                // 缩略图失败时保留 video 元素本身
            });
        }
        else {
            const icon = document.createElement('i');
            icon.className = preview.type === 'audio'
                ? 'fas fa-music'
                : (preview.type === 'text' ? 'fas fa-file-alt' : 'fas fa-file');
            icon.classList.add('file-preview-icon');
            item.appendChild(icon);
        }

        const label = document.createElement('div');
        label.className = 'file-label';
        label.textContent = preview.name;
        item.appendChild(label);

        const removeBtn = document.createElement('button');
        removeBtn.className = 'remove-btn';
        removeBtn.type = 'button';
        removeBtn.title = '移除';
        removeBtn.innerHTML = '<i class="fas fa-times"></i>';
        removeBtn.addEventListener('click', () => this.removeFilePreview(preview, item));
        item.appendChild(removeBtn);

        this.previewArea?.appendChild(item);
    }

    /** 移除单个附件气泡并释放 blob URL */
    removeFilePreview(preview, item) {
        const index = this.previews.indexOf(preview);
        if (index > -1) {
            this.previews.splice(index, 1);
        }
        if (preview.url.startsWith('blob:')) {
            URL.revokeObjectURL(preview.url);
        }
        item.remove();
        if (this.onChange) this.onChange();
    }

    /** 清空全部附件气泡 */
    clearFilePreviews() {
        revokeAllFilePreviews(this.previews);
        this.previews = [];
        if (this.previewArea) {
            this.previewArea.innerHTML = '';
        }
        if (this.onChange) this.onChange();
    }

    /**
     * 上传附件并返回处理结果
     *
     * 音频文件不上传服务器（后续以 base64 直接嵌入消息），
     * 其余文件统一经 /file/write 保存并返回可访问 URL。
     */
    async processFileUpload(preview) {
        const category = getFileCategory(preview.file);

        if (category === 'audio') {
            return {
                fileUrl: null,
                category,
                fileName: preview.name,
                fileSize: preview.file.size,
                previewUrl: URL.createObjectURL(preview.file),
            };
        }

        const saveResult = await saveFile(preview.file, true);
        return {
            fileUrl: `${window.location.origin}/file/read/${saveResult.filename}`,
            category,
            fileName: preview.name,
            fileSize: preview.file.size,
        };
    }

    /**
     * 构建 OpenAI 兼容的消息内容块
     *
     * 规则与电脑端一致：
     * - 图片 → image_url；视频 → 文本标记 + video_url
     * - 音频 → input_audio（仅 wav/mp3 支持，其余退回 audio_url）
     * - 文本 → 围栏代码块；其他 → 文件名/大小/访问链接
     *
     * @param {Array<object>} fileResults - processFileUpload 的结果列表
     * @returns {Promise<Array<object>>} 内容块数组
     */
    async buildFileContentBlocks(fileResults) {
        const contentBlocks = [];

        for (const res of fileResults) {
            const preview = this.previews.find(p => p.name === res.fileName);

            if (res.category === 'audio') {
                if (!preview) continue;
                try {
                    const base64Data = await fileToRawBase64(preview.file);
                    const audioFormat = getAudioFormat(preview.file);
                    if (audioFormat) {
                        contentBlocks.push({ type: 'text', text: '[语音] ' });
                        contentBlocks.push({
                            type: 'input_audio',
                            input_audio: { data: base64Data, format: audioFormat }
                        });
                    } else {
                        // llama.cpp 仅接受 wav/mp3，其他格式退回可访问链接
                        const url = preview.url;
                        contentBlocks.push({ type: 'text', text: `[音频] 名称: ${res.fileName} 访问链接: ${url}` });
                    }
                } catch (err) {
                    this.showError?.(`无法读取音频 ${res.fileName}`);
                }
                continue;
            }

            if (res.category === 'image') {
                contentBlocks.push({ type: 'image_url', image_url: { url: res.fileUrl } });
            }
            else if (res.category === 'video') {
                contentBlocks.push({ type: 'text', text: '[视频] ' });
                contentBlocks.push({ type: 'video_url', video_url: { url: res.fileUrl } });
            }
            else if (res.category === 'text') {
                if (!preview) continue;
                try {
                    const rawText = await preview.file.text();
                    const MAX_TEXT_LEN = 50000;
                    const textContent = rawText.length > MAX_TEXT_LEN
                        ? rawText.slice(0, MAX_TEXT_LEN) + '\n\n[文件内容过长，已截断]'
                        : rawText;
                    contentBlocks.push({
                        type: 'text',
                        text: `【文件 ${res.fileName}】\n内容：\n\`\`\`\n${textContent}\n\`\`\`\n访问链接：${res.fileUrl}`
                    });
                } catch (err) {
                    this.showError?.(`无法读取文件 ${res.fileName}`);
                }
            }
            else {
                contentBlocks.push({
                    type: 'text',
                    text: `【文件 ${res.fileName}】\n大小：${formatFileSize(res.fileSize)}\n访问链接：${res.fileUrl}`
                });
            }
        }

        return contentBlocks;
    }
}
