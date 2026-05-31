import { WebSocketClient } from './socket.js';
import { createFilePreview, getFileCategory, revokeAllFilePreviews, getVideoThumbnail, formatFileSize } from './file.js';
import { saveFile, sendMessages } from './fetch.js';
import { Live2D, EmotionalStateEnum } from './live2d.js';
import { renderMessage, renderAllMessages } from './chat.js';
import { TTS } from './tts.js';

class LunarCoreApp {
    wsClient = null;
    historyMessages = [];
    filePreviews = [];
    isModalOpen = false;
    isLoading = false;
    messageInput = null;
    sendButton = null;
    filePreviewArea = null;
    modalOverlay = null;
    modalBody = null;
    errorToast = null;

    constructor() {
        this.initElements();
        this.initEventListeners();
        this.initWebSocket();
        this.initLive2D();
    }

    initElements() {
        this.messageInput = document.getElementById('messageInput');
        this.sendButton = document.getElementById('sendButton');
        this.filePreviewArea = document.getElementById('filePreviewArea');
        this.modalOverlay = document.getElementById('modalOverlay');
        this.modalBody = document.getElementById('modalBody');
        this.errorToast = document.getElementById('errorToast');

        document.getElementById('chatHistoryButton')?.addEventListener('click', () => this.toggleModal());
        document.getElementById('modalClose')?.addEventListener('click', () => this.toggleModal());
        this.modalOverlay?.addEventListener('click', (e) => {
            if (e.target === this.modalOverlay) this.toggleModal();
        });
    }

    initEventListeners() {
        this.sendButton?.addEventListener('click', () => this.handleSend());
        this.messageInput?.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                this.handleSend();
            }
        });
        this.messageInput?.addEventListener('input', () => { this.autoResizeTextarea(); });
        this.setupDragAndDrop();
    }

    setupDragAndDrop() {
        const inputContainer = document.getElementById('mainContainerPanel');
        ['dragenter', 'dragover', 'dragleave', 'drop'].forEach(eventName => {
            inputContainer?.addEventListener(eventName, (e) => { e.preventDefault(); e.stopPropagation(); });
        });
        ['dragenter', 'dragover'].forEach(eventName => {
            inputContainer?.addEventListener(eventName, () => {
                inputContainer.classList.add('drag-over');
            });
        });
        ['dragleave', 'drop'].forEach(eventName => {
            inputContainer?.addEventListener(eventName, () => {
                inputContainer.classList.remove('drag-over');
            });
        });
        inputContainer?.addEventListener('drop', async (e) => {
            const files = e.dataTransfer?.files;
            if (files) {
                await this.handleFileSelect(Array.from(files));
            }
        });
    }

    async handleFileSelect(files) {
        for (const file of files) {
            const category = getFileCategory(file);
            if (category === 'other') {
                this.showError(`文件 ${file.name} 不在允许的类型白名单中，将只发送文件名和大小`);
            }
            const preview = createFilePreview(file);
            this.filePreviews.push(preview);
            this.renderFilePreview(preview);
        }
    }

    renderFilePreview(preview) {
        const item = document.createElement('div');
        item.className = 'file-preview-item';
        item.dataset.name = preview.name;

        if (preview.type === 'image') {
            const img = document.createElement('img');
            img.src = preview.url;
            item.appendChild(img);
        } else if (preview.type === 'video') {
            const video = document.createElement('video');
            video.src = preview.url;
            video.muted = true;
            item.appendChild(video);
            getVideoThumbnail(preview.file).then(thumbnail => {
                const img = document.createElement('img');
                img.src = thumbnail;
                item.insertBefore(img, video);
                video.style.display = 'none';
            }).catch(e => {
                console.warn('Failed to get video thumbnail:', e);
            });
        } else {
            const icon = document.createElement('i');
            icon.className = preview.type === 'text' ? 'fas fa-file-alt' : 'fas fa-file';
            icon.style.cssText = 'font-size: 24px; color: white; display: flex; align-items: center; justify-content: center; height: 100%;';
            item.appendChild(icon);
        }

        const label = document.createElement('div');
        label.className = 'file-label';
        label.textContent = preview.name;
        item.appendChild(label);

        const removeBtn = document.createElement('button');
        removeBtn.className = 'remove-btn';
        removeBtn.innerHTML = '<i class="fas fa-times"></i>';
        removeBtn.onclick = () => this.removeFilePreview(preview, item);
        item.appendChild(removeBtn);

        this.filePreviewArea?.appendChild(item);
    }

    removeFilePreview(preview, item) {
        const index = this.filePreviews.indexOf(preview);
        if (index > -1) {
            this.filePreviews.splice(index, 1);
        }
        if (preview.url.startsWith('blob:')) {
            URL.revokeObjectURL(preview.url);
        }
        item.remove();
    }

    autoResizeTextarea() {
        if (this.messageInput) {
            this.messageInput.style.height = 'auto';
            this.messageInput.style.height = Math.min(this.messageInput.scrollHeight, 120) + 'px';
            Live2D.reloadContainer();
        }
    }

    initWebSocket() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws`;
        this.wsClient = new WebSocketClient(wsUrl);
        this.wsClient.onConnect(() => {
            console.log('WebSocket connected');
            this.setLoadingState(false);
        });
        this.wsClient.onMessage((message) => {
            this.handleWebSocketMessage(message);
        });
        this.wsClient.onError((error) => {
            console.error('WebSocket error:', error);
            this.showError('连接错误，请刷新页面');
        });
        this.wsClient.connect();
    }

    async handleWebSocketMessage(message) {
        switch (message.type) {
            case 'context':
                if (message.data.type === 'response' || message.data.type === 'active') {
                    const content = message.data.content || '';
                    console.log('content', content);
                    const audioBase64 = await TTS.generateAndPlay(content);
                    await this.handleAssistantMessage(content, undefined, audioBase64);
                }
                break;
            case 'image':
                if (message.data.images) {
                    for (const imageBase64 of message.data.images) {
                        const byteString = atob(imageBase64);
                        const mimeType = 'image/jpeg';
                        const ab = new ArrayBuffer(byteString.length);
                        const ia = new Uint8Array(ab);
                        for (let i = 0; i < byteString.length; i++) {
                            ia[i] = byteString.charCodeAt(i);
                        }
                        const blob = new Blob([ab], { type: mimeType });
                        const file = new File([blob], `assistant_${Date.now()}.jpg`, { type: 'image/jpeg' });
                        const fileUrl = `${window.location.origin}/file/read/${file.name}`;
                        await this.handleAssistantMessage('', fileUrl, '');
                    }
                }
                break;
            case 'error':
                this.showError(message.data.content || '发生错误');
                this.setLoadingState(false);
                Live2D.setStateWithTimeout(EmotionalStateEnum.IDLE);
                break;
        }
    }

    async handleAssistantMessage(content, imageUrl, audioBase64) {
        const assistantMessage = {
            role: 'assistant',
            content: content,
            imageUrl: imageUrl || undefined,
            audioBase64: audioBase64 || undefined,
            timestamp: Date.now(),
        };
        this.historyMessages.push(assistantMessage);

        if (this.isModalOpen) {
            await renderMessage(assistantMessage, this.modalBody);
        }
        this.setLoadingState(false);
        Live2D.setStateWithTimeout(EmotionalStateEnum.IDLE);
    }

    async processFileUpload(preview) {
        const saveResult = await saveFile(preview.file, true);
        const fileUrl = `${window.location.origin}/file/read/${saveResult.filename}`;
        const category = getFileCategory(preview.file);

        return {
            fileUrl,
            category,
            fileName: preview.name,
            fileSize: preview.file.size
        };
    }

    async handleSend() {
        const text = this.messageInput?.value.trim();
        const hasFiles = this.filePreviews.length > 0;

        if (!text && !hasFiles) return;

        this.setLoadingState(true);
        Live2D.setEmotionState(EmotionalStateEnum.AWAIT);

        try {
            const openAIMessages = [];
            const contentBlocks = [];
            const uploadedFileUrls = [];
            const userContentParts = [];

            const filePromises = this.filePreviews.map(preview => this.processFileUpload(preview));
            const fileResults = await Promise.all(filePromises);

            for (const res of fileResults) {
                uploadedFileUrls.push(res.fileUrl);

                if (res.category === 'text') {
                    const file = this.filePreviews.find(p => p.name === res.fileName)?.file;
                    if (file) {
                        try {
                            const rawText = await file.text();
                            const MAX_TEXT_LEN = 50000;
                            let textContent = rawText;
                            if (rawText.length > MAX_TEXT_LEN) {
                                textContent = rawText.slice(0, MAX_TEXT_LEN) + '\n\n[文件内容过长，已截断]';
                                this.showError(`文件 ${res.fileName} 内容超过限制，仅截取前 ${MAX_TEXT_LEN} 字符`);
                            }
                            const textBlock = `【文件 ${res.fileName}】\n内容：\n\`\`\`\n${textContent}\n\`\`\`\n访问链接：${res.fileUrl}`;
                            contentBlocks.push({ type: 'text', text: textBlock });
                            userContentParts.push(`[文本文件: ${res.fileName}]`);
                        } catch (err) {
                            console.error(`读取文本文件失败: ${res.fileName}`, err);
                            this.showError(`无法读取文件 ${res.fileName}，未加入上下文`);
                            contentBlocks.push({ type: 'text', text: `【文件 ${res.fileName}】无法读取内容，访问链接：${res.fileUrl}` });
                            userContentParts.push(`[无法读取: ${res.fileName}]`);
                        }
                    }
                } else if (res.category === 'image') {
                    contentBlocks.push({ type: 'image_url', image_url: { url: res.fileUrl } });
                    userContentParts.push(`[图片: ${res.fileName}]`);
                } else if (res.category === 'video') {
                    contentBlocks.push({ type: 'image_url', image_url: { url: res.fileUrl } });
                    userContentParts.push(`[视频: ${res.fileName}]`);
                } else {
                    const fileSize = formatFileSize(res.fileSize);
                    contentBlocks.push({ type: 'text', text: `【文件 ${res.fileName}】\n大小：${fileSize}\n访问链接：${res.fileUrl}` });
                    userContentParts.push(`[文件: ${res.fileName} (${fileSize})]`);
                }
            }

            if (text) {
                contentBlocks.push({ type: 'text', text: text });
            }

            const userContent = contentBlocks.length > 0 ? contentBlocks : text;
            openAIMessages.push({ role: 'user', content: userContent });

            const userMessage = {
                role: 'user',
                content: text || (userContentParts.length > 0 ? userContentParts.join(' ') : ''),
                timestamp: Date.now(),
            };
            if (uploadedFileUrls.length > 0) {
                userMessage.imageUrls = uploadedFileUrls;
            }
            this.historyMessages.push(userMessage);

            if (this.isModalOpen) {
                await renderMessage(userMessage, this.modalBody);
            }

            await sendMessages(openAIMessages);

            this.messageInput.value = '';
            this.clearFilePreviews();
            this.autoResizeTextarea();
        } catch (error) {
            console.error('Send error:', error);
            this.showError(error instanceof Error ? error.message : '发送失败');
            this.setLoadingState(false);
            Live2D.setStateWithTimeout(EmotionalStateEnum.IDLE);
        }
    }

    clearFilePreviews() {
        revokeAllFilePreviews(this.filePreviews);
        this.filePreviews = [];
        if (this.filePreviewArea) {
            this.filePreviewArea.innerHTML = '';
        }
    }

    setLoadingState(loading) {
        this.isLoading = loading;
        if (this.sendButton) {
            this.sendButton.disabled = loading;
            if (loading) {
                this.sendButton.innerHTML = '<span class="loading-indicator"></span>';
            } else {
                this.sendButton.innerHTML = '<i class="fas fa-paper-plane"></i>';
            }
        }
    }

    toggleModal() {
        this.isModalOpen = !this.isModalOpen;
        if (this.isModalOpen) {
            this.modalOverlay?.classList.add('visible');
            if (this.modalBody) {
                this.modalBody.innerHTML = '';
            }
            renderAllMessages(this.modalBody, this.historyMessages, false);
        } else {
            this.modalOverlay?.classList.remove('visible');
        }
    }

    showError(message) {
        if (this.errorToast) {
            this.errorToast.textContent = message;
            this.errorToast.classList.add('visible');
            setTimeout(() => { this.errorToast?.classList.remove('visible'); }, 3000);
        }
    }

    async initLive2D() {
        try {
            await Live2D.init();
        } catch (error) {
            console.error('Failed to initialize Live2D:', error);
            this.showError('Live2D 初始化失败');
        }
    }
}

document.addEventListener('DOMContentLoaded', () => {
    window.app = new LunarCoreApp();
});