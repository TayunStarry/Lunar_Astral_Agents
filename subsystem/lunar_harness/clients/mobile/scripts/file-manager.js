import {
    createFilePreview,
    getFileCategory,
    revokeAllFilePreviews,
    getVideoThumbnail,
    fileToRawBase64,
    getAudioFormat,
} from './file.js';
import { saveFile } from './fetch.js';
import { imageEnvelope, videoEnvelope, audioEnvelope, fileEnvelope } from '/shared/core/media.js';

/**
 * 待发送附件管理器
 *
 * 负责移动端文件的选择、悬浮气泡预览渲染、删除与上传，
 * 以及把附件转换为六类内容信封（与电脑端 send.js 规则一致）。
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
     * 音频文件不上传服务器（后续以 base64 直接嵌入消息）；
     * 图片、视频、文本与其他文件统一经 /file/write 保存，按内容哈希落盘，
     * 随后以对应信封下发，不再预入库换句柄。
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
        // 读取地址以服务端实际落点为准：rel_path 含 multimedia/ 等存储根，
        // filename 只是裸文件名（历史版本曾用它拼地址，导致 /file/read/ 404）。
        const rel = String(saveResult.rel_path || saveResult.filename || '');
        return {
            fileUrl: `${window.location.origin}/file/read/${rel}`,
            category,
            fileName: preview.name,
            fileSize: preview.file.size,
        };
    }

    /**
     * 构建消息内容项
     *
     * 规则与电脑端一致：
     * - 图片 → image_static / image_animated；视频 → video（句柄由服务端生成）
     * - 音频 → audio（data URI 内联下发，服务端转写，不落盘）
     * - 文本 / 其他文件 → file 信封（name / bytes 保留原始文件名与大小）
     *
     * @param {Array<object>} fileResults - processFileUpload 的结果列表
     * @returns {Promise<Array<object>>} 内容块数组
     */
    async buildFileContentBlocks(fileResults) {
        const contentBlocks = [];

        for (const res of fileResults) {
            const preview = this.previews.find(p => p.name === res.fileName);
            const sourceFile = preview?.file || { name: res.fileName, type: '', size: res.fileSize };

            if (res.category === 'audio') {
                if (!preview) continue;
                try {
                    const base64Data = await fileToRawBase64(preview.file);
                    const audioFormat = getAudioFormat(preview.file);
                    contentBlocks.push(audioEnvelope(preview.file, base64Data, audioFormat));
                } catch (err) {
                    this.showError?.(`无法读取音频 ${res.fileName}`);
                }
                continue;
            }

            if (res.category === 'image') {
                contentBlocks.push(imageEnvelope(sourceFile, res.fileUrl));
            }
            else if (res.category === 'video') {
                contentBlocks.push(videoEnvelope(sourceFile, res.fileUrl));
            }
            else {
                contentBlocks.push(fileEnvelope(sourceFile, res.fileUrl));
            }
        }

        return contentBlocks;
    }
}
