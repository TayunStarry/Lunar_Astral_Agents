import { WebSocketClient } from './websocket';
import { sendMessages, sendVideoUrls } from './api';
import {
    createFilePreview,
    isMediaFile,
    revokeAllFilePreviews,
    getVideoThumbnail,
    type FilePreview,
} from './fileUpload';
import {
    initLive2D,
    setStateWithTimeout,
    setEmotionState,
    getLive2DModel,
    isLive2DReady,
    EmotionalState,
} from './live2d';
import { renderMessage, clearContainer, renderAllMessages } from './message';
import type { HistoryMessage, OpenAIMessage, WebSocketMessage, ContentBlock } from './types';

class LunarCoreApp {
    private wsClient: WebSocketClient | null = null;
    private historyMessages: HistoryMessage[] = [];
    private filePreviews: FilePreview[] = [];
    private isModalOpen: boolean = false;
    private isLoading: boolean = false;

    private messageInput!: HTMLTextAreaElement;
    private sendButton!: HTMLButtonElement;
    private filePreviewArea!: HTMLElement;
    private chatHistoryPanel!: HTMLElement;
    private modalOverlay!: HTMLElement;
    private modalBody!: HTMLElement;
    private errorToast!: HTMLElement;

    constructor() {
        this.initElements();
        this.initEventListeners();
        this.initWebSocket();
        this.initLive2D();
    }

    private initElements(): void {
        this.messageInput = document.getElementById('messageInput') as HTMLTextAreaElement;
        this.sendButton = document.getElementById('sendButton') as HTMLButtonElement;
        this.filePreviewArea = document.getElementById('filePreviewArea') as HTMLElement;
        this.chatHistoryPanel = document.getElementById('chatHistoryPanel') as HTMLElement;
        this.modalOverlay = document.getElementById('modalOverlay') as HTMLElement;
        this.modalBody = document.getElementById('modalBody') as HTMLElement;
        this.errorToast = document.getElementById('errorToast') as HTMLElement;

        document.getElementById('chatHistoryButton')?.addEventListener('click', () => this.toggleModal());
        document.getElementById('modalClose')?.addEventListener('click', () => this.toggleModal());
        this.modalOverlay?.addEventListener('click', (e) => {
            if (e.target === this.modalOverlay) this.toggleModal();
        });
    }

    private initEventListeners(): void {
        this.sendButton?.addEventListener('click', () => this.handleSend());

        this.messageInput?.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                this.handleSend();
            }
        });

        this.messageInput?.addEventListener('input', () => {
            this.autoResizeTextarea();
        });

        this.setupDragAndDrop();
    }

    private setupDragAndDrop(): void {
        const inputContainer = document.getElementById('inputContainer');

        ['dragenter', 'dragover', 'dragleave', 'drop'].forEach(eventName => {
            inputContainer?.addEventListener(eventName, (e) => {
                e.preventDefault();
                e.stopPropagation();
            });
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

    private async handleFileSelect(files: File[]): Promise<void> {
        for (const file of files) {
            const preview = createFilePreview(file);
            this.filePreviews.push(preview);
            this.renderFilePreview(preview);
        }
    }

    private renderFilePreview(preview: FilePreview): void {
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

            try {
                const thumbnail = await getVideoThumbnail(preview.file);
                const img = document.createElement('img');
                img.src = thumbnail;
                item.insertBefore(img, video);
                video.style.display = 'none';
            } catch (e) {
                console.warn('Failed to get video thumbnail:', e);
            }
        } else {
            const icon = document.createElement('i');
            icon.className = 'fas fa-file-alt';
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

    private removeFilePreview(preview: FilePreview, item: HTMLElement): void {
        const index = this.filePreviews.indexOf(preview);
        if (index > -1) {
            this.filePreviews.splice(index, 1);
        }
        if (preview.url.startsWith('blob:')) {
            URL.revokeObjectURL(preview.url);
        }
        item.remove();
    }

    private autoResizeTextarea(): void {
        if (this.messageInput) {
            this.messageInput.style.height = 'auto';
            this.messageInput.style.height = Math.min(this.messageInput.scrollHeight, 120) + 'px';
        }
    }

    private initWebSocket(): void {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}`;

        this.wsClient = new WebSocketClient(wsUrl);

        this.wsClient.onConnect(() => {
            console.log('WebSocket connected');
            this.setLoadingState(false);
        });

        this.wsClient.onMessage((message: WebSocketMessage) => {
            this.handleWebSocketMessage(message);
        });

        this.wsClient.onError((error) => {
            console.error('WebSocket error:', error);
            this.showError('连接错误，请刷新页面');
        });

        this.wsClient.connect();
    }

    private async handleWebSocketMessage(message: WebSocketMessage): Promise<void> {
        switch (message.type) {
            case 'context':
                if (message.data.type === 'response' || message.data.type === 'active') {
                    await this.handleAssistantMessage(message.data.content || '');
                }
                break;

            case 'image':
                if (message.data.images) {
                    for (const imageBase64 of message.data.images) {
                        await this.handleAssistantMessage('', imageBase64);
                    }
                }
                break;

            case 'error':
                this.showError(message.data.content || '发生错误');
                this.setLoadingState(false);
                setStateWithTimeout(EmotionalState.IDLE);
                break;
        }
    }

    private async handleAssistantMessage(content: string, imageBase64?: string): Promise<void> {
        const assistantMessage: HistoryMessage = {
            role: 'assistant',
            content: content,
            imageUrl: imageBase64 ? `data:image/jpeg;base64,${imageBase64}` : undefined,
            timestamp: Date.now(),
        };

        this.historyMessages.push(assistantMessage);
        await renderMessage(assistantMessage, this.chatHistoryPanel);

        if (this.isModalOpen) {
            await renderMessage(assistantMessage, this.modalBody);
        }

        this.setLoadingState(false);
        setStateWithTimeout(EmotionalState.IDLE);
    }

    private async handleSend(): Promise<void> {
        const text = this.messageInput?.value.trim();
        const hasFiles = this.filePreviews.length > 0;

        if (!text && !hasFiles) {
            return;
        }

        this.setLoadingState(true);
        setEmotionState(EmotionalState.AWAIT);

        try {
            const openAIMessages: OpenAIMessage[] = [];
            let userContent: string | ContentBlock[] = [];

            if (hasFiles) {
                const contentBlocks: ContentBlock[] = [];

                for (const preview of this.filePreviews) {
                    if (isMediaFile(preview.file)) {
                        if (preview.type === 'image') {
                            const response = await fetch(preview.url);
                            const blob = await response.blob();
                            const base64 = await this.blobToBase64(blob);
                            contentBlocks.push({
                                type: 'image_url',
                                image_url: { url: `data:${preview.file.type};base64,${base64}` },
                            });
                        } else if (preview.type === 'video') {
                            const response = await fetch(preview.url);
                            const blob = await response.blob();
                            const base64 = await this.blobToBase64(blob);
                            contentBlocks.push({
                                type: 'image_url',
                                image_url: { url: `data:${preview.file.type};base64,${base64}` },
                            });
                        }
                    }
                }

                if (text) {
                    contentBlocks.push({ type: 'text', text });
                }

                userContent = contentBlocks;
            } else {
                userContent = text;
            }

            openAIMessages.push({
                role: 'user',
                content: userContent,
            });

            const userMessage: HistoryMessage = {
                role: 'user',
                content: text || (hasFiles ? '[文件]' : ''),
                timestamp: Date.now(),
            };

            if (hasFiles && this.filePreviews.some(p => p.type === 'image')) {
                const firstImage = this.filePreviews.find(p => p.type === 'image');
                if (firstImage) {
                    userMessage.imageUrl = firstImage.url;
                }
            }

            this.historyMessages.push(userMessage);
            await renderMessage(userMessage, this.chatHistoryPanel);

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
            setStateWithTimeout(EmotionalState.IDLE);
        }
    }

    private async blobToBase64(blob: Blob): Promise<string> {
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

    private clearFilePreviews(): void {
        revokeAllFilePreviews(this.filePreviews);
        this.filePreviews = [];
        if (this.filePreviewArea) {
            this.filePreviewArea.innerHTML = '';
        }
    }

    private setLoadingState(loading: boolean): void {
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

    private toggleModal(): void {
        this.isModalOpen = !this.isModalOpen;

        if (this.isModalOpen) {
            this.modalOverlay?.classList.add('visible');
            clearContainer(this.modalBody);
            renderAllMessages(this.modalBody, this.historyMessages, false);
        } else {
            this.modalOverlay?.classList.remove('visible');
        }
    }

    private showError(message: string): void {
        if (this.errorToast) {
            this.errorToast.textContent = message;
            this.errorToast.classList.add('visible');
            setTimeout(() => {
                this.errorToast.classList.remove('visible');
            }, 3000);
        }
    }

    private async initLive2D(): Promise<void> {
        try {
            await initLive2D();
        } catch (error) {
            console.error('Failed to initialize Live2D:', error);
            this.showError('Live2D 初始化失败');
        }
    }
}

document.addEventListener('DOMContentLoaded', () => {
    new LunarCoreApp();
});