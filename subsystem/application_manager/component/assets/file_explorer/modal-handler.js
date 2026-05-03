/**
 * 模态框处理模块
 * 负责文件管理器的所有模态框相关功能
 */

import { formatFileSize, formatDate, isTextFile, showToast } from './utils.js';
import { uploadFile } from './file-operations.js';

/**
 * 当前编辑的文件
 * @type {Object|null}
 */
let currentEditFile = null;

/**
 * 显示文本模态框
 * @param {Object} file - 要预览的文件对象
 * @param {string} currentPath - 当前路径
 * @param {Function} onSave - 保存完成回调
 */
export async function showTextModal(file, currentPath, onSave) {
    try {
        const response = await fetch(`/read/${file.path}`);
        if (!response.ok) throw new Error('读取文件失败');
        const content = await response.text();

        const modal = document.getElementById('text-modal');
        const modalTitle = document.getElementById('text-modal-title');
        const modalFileInfo = document.getElementById('text-modal-file-info');
        const modalContent = document.getElementById('text-modal-content');
        const modalEditor = document.getElementById('text-modal-editor');
        const modalSave = document.getElementById('text-modal-save');
        const modalEdit = document.getElementById('text-modal-edit');
        const modalCopy = document.getElementById('text-modal-copy');
        const modalDownload = document.getElementById('text-modal-download');

        modalTitle.textContent = file.name;
        const fileSize = formatFileSize(file.size);
        const lastModified = formatDate(file.lastModified);
        modalFileInfo.textContent = `${fileSize} · ${lastModified}`;
        modalEditor.value = content;

        // 显示预览，隐藏编辑器和保存按钮
        modalContent.style.display = 'block';
        modalEditor.style.display = 'none';
        modalSave.style.display = 'none';

        // 尝试对代码文件应用语法高亮
        try {
            if (isTextFile(file.name)) {
                const ext = file.name.toLowerCase().slice(file.name.lastIndexOf('.') + 1);
                const highlighted = hljs.highlight(content, { language: ext }).value;
                modalContent.innerHTML = highlighted;
                modalContent.className = 'hljs';
            } else {
                modalContent.textContent = content;
                modalContent.className = '';
            }
        } catch (error) {
            modalContent.textContent = content;
            modalContent.className = '';
        }

        // 显示模态框
        modal.classList.add('show');

        // 保存当前文件引用
        currentEditFile = file;

        // 编辑按钮点击事件
        modalEdit.onclick = () => {
            modalContent.style.display = 'none';
            modalEditor.style.display = 'block';
            modalSave.style.display = 'inline-block';
        };

        // 复制按钮点击事件
        modalCopy.onclick = () => {
            navigator.clipboard.writeText(content)
                .then(() => showToast('已复制到剪贴板', 'success'))
                .catch(() => showToast('复制失败', 'error'));
        };

        // 下载按钮点击事件
        modalDownload.onclick = () => {
            const link = document.createElement('a');
            link.href = `/download/${file.path}`;
            link.download = file.name;
            document.body.appendChild(link);
            link.click();
            document.body.removeChild(link);
        };

        // 保存按钮点击事件
        modalSave.onclick = async () => {
            try {
                const newContent = modalEditor.value;
                await saveTextFile(file, newContent, currentPath);

                // 保存成功后，更新预览内容
                modalContent.textContent = newContent;
                modalContent.style.display = 'block';
                modalEditor.style.display = 'none';
                modalSave.style.display = 'none';

                // 重新应用语法高亮
                try {
                    if (isTextFile(file.name)) {
                        const ext = file.name.toLowerCase().slice(file.name.lastIndexOf('.') + 1);
                        const highlighted = hljs.highlight(newContent, { language: ext }).value;
                        modalContent.innerHTML = highlighted;
                        modalContent.className = 'hljs';
                    }
                } catch (error) {
                    modalContent.textContent = newContent;
                    modalContent.className = '';
                }

                showToast('保存成功', 'success');
                onSave();
            } catch (error) {
                showToast('保存失败', 'error');
                console.error('保存失败:', error);
            }
        };
    } catch (error) {
        showToast('预览文件失败', 'error');
        console.error('预览文件失败:', error);
    }
}

/**
 * 关闭文本模态框
 * @param {Event} event - 点击事件对象
 */
export function closeTextModal(event) {
    const modal = document.getElementById('text-modal');
    if (
        event.target === modal ||
        event.target.closest('.modal-close-btn') ||
        event.target.classList.contains('close')
    ) {
        modal.classList.remove('show');
        currentEditFile = null;
    }
}

/**
 * 保存文本文件
 * @param {Object} file - 要保存的文件对象
 * @param {string} content - 新的文件内容
 * @param {string} currentPath - 当前路径
 * @returns {Promise} - 保存完成后的 Promise 对象
 */
async function saveTextFile(file, content, currentPath) {
    try {
        const blob = new Blob([content], { type: 'text/plain' });
        const uploadFileObj = new File([blob], file.name, { type: 'text/plain' });
        await uploadFile(uploadFileObj, currentPath, () => {}, true);
    } catch (error) {
        throw new Error('保存文件失败');
    }
}

/**
 * 显示二维码模态框
 */
export async function showQRCode() {
    try {
        const currentUrl = window.location.origin;
        const qrcodeContainer = document.getElementById('qrcode-container');
        const qrcodeUrl = document.getElementById('qrcode-url');
        const qrcodeModal = document.getElementById('qrcode-modal');

        // 清空二维码容器
        qrcodeContainer.innerHTML = '';

        // 生成二维码
        new QRCode(qrcodeContainer, {
            text: currentUrl,
            width: 256,
            height: 256,
            colorDark: '#000000',
            colorLight: '#ffffff',
            correctLevel: QRCode.CorrectLevel.H
        });

        // 显示URL
        qrcodeUrl.textContent = currentUrl;

        // 显示模态框
        qrcodeModal.classList.add('show');
    } catch (error) {
        showToast('生成二维码失败', 'error');
        console.error('生成二维码失败:', error);
    }
}

/**
 * 关闭二维码模态框
 * @param {Event} event - 点击事件对象
 */
export function closeQRCodeModal(event) {
    const modal = document.getElementById('qrcode-modal');
    if (
        event.target === modal ||
        event.target.closest('.modal-close-btn') ||
        event.target.classList.contains('close')
    ) {
        modal.classList.remove('show');
    }
}

/**
 * 检查是否在媒体预览中
 * @returns {boolean} - 是否在媒体预览中
 */
function isInMediaPreview() {
    return !!document.querySelector('.image-preview-container');
}

/**
 * 处理键盘事件
 * @param {Event} event - 键盘事件对象
 * @param {Array} currentMediaList - 当前媒体列表
 * @param {number} currentMediaIndex - 当前媒体索引
 * @param {Function} onMediaChange - 媒体变化回调
 * @returns {number} - 更新后的媒体索引
 */
export function handleKeyboardEvent(event, currentMediaList, currentMediaIndex, onMediaChange) {
    // 处理 ESC 键关闭模态框
    if (event.key === 'Escape') {
        const textModal = document.getElementById('text-modal');
        if (textModal.classList.contains('show')) {
            textModal.classList.remove('show');
            currentEditFile = null;
        }

        const qrcodeModal = document.getElementById('qrcode-modal');
        if (qrcodeModal.classList.contains('show')) {
            qrcodeModal.classList.remove('show');
        }

        return currentMediaIndex;
    }

    // 处理媒体预览的键盘导航
    if (isInMediaPreview() && currentMediaList.length > 0) {
        const imagePreviewContainer = document.querySelector('.image-preview-container');
        const imageInfo = document.querySelector('.image-info');
        const imagePreview = document.querySelector('.image-preview');
        const videoPreview = document.querySelector('.video-preview');
        const imageDragContainer = document.querySelector('.image-drag-container');

        let newIndex = currentMediaIndex;
        let handled = false;

        switch (event.key) {
            case 'ArrowLeft':
                newIndex = (currentMediaIndex - 1 + currentMediaList.length) % currentMediaList.length;
                handled = true;
                break;
            case 'ArrowRight':
                newIndex = (currentMediaIndex + 1) % currentMediaList.length;
                handled = true;
                break;
        }

        if (handled && newIndex !== currentMediaIndex) {
            const file = currentMediaList[newIndex];
            const quicklyLoadMediaPreview = () => {
                if (file.name.toLowerCase().match(/\.(jpg|jpeg|png|gif|bmp|svg|webp)$/i)) {
                    imageDragContainer.style.display = 'block';
                    imagePreview.style.display = 'block';
                    videoPreview.style.display = 'none';
                    imagePreview.alt = file.name;
                    imagePreview.src = `/read/${file.path}`;
                } else {
                    imageDragContainer.style.display = 'none';
                    imagePreview.style.display = 'none';
                    videoPreview.style.display = 'block';
                    videoPreview.alt = file.name;
                    videoPreview.src = `/read/${file.path}`;
                }
                imageInfo.style.display = 'none';
            };

            quicklyLoadMediaPreview();
            onMediaChange(newIndex);
            return newIndex;
        }
    }

    return currentMediaIndex;
}
