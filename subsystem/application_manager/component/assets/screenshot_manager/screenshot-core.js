/* ============================================================
   screenshot-core.js - 截图核心功能
   ============================================================ */

import { showToast } from './ui.js';

export class ScreenshotCore {
    constructor(domElements) {
        this.dom = domElements;
        this.continuousConfig = { enabled: false, interval: 1000, intervalId: null };
        this.screenshotConfig = { x: 0, y: 0, width: 3840, height: 2160 };
        this.scale = 1.0;
        this.historyManager = null;
    }

    set updateScale(v) { this.scale = v; }
    get updateScale() { return this.scale; }

    get maximumScreenshot() {
        const { width, height } = this.screenshotConfig;
        return width === 3840 && height === 2160;
    }

    async getScreenshot() {
        try {
            let response;
            if (this.maximumScreenshot) {
                response = await fetch('/capture', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ display_index: -1, scale: String(this.scale), format: 'png' })
                });
            } else {
                const { x, y, width, height } = this.screenshotConfig;
                const url = `/capture/region?region=${x},${y},${width},${height}&scale=${this.scale}&format=png`;
                response = await fetch(url);
            }

            if (!response.ok) throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            const blob = await response.blob();
            const img = await new Promise((resolve, reject) => {
                const img = new Image();
                img.onload = () => resolve(img);
                img.onerror = reject;
                img.src = URL.createObjectURL(blob);
            });

            const tempCanvas = document.createElement('canvas');
            tempCanvas.width = img.width;
            tempCanvas.height = img.height;
            const tempCtx = tempCanvas.getContext('2d');
            tempCtx.drawImage(img, 0, 0);
            return tempCtx.getImageData(0, 0, img.width, img.height);
        } catch (err) {
            console.error('截图失败:', err);
            showToast('截图失败: ' + err.message, 'error');
            return null;
        }
    }

    async captureScreen() {
        this.dom.captureSceneButton.disabled = true;
        this.dom.captureSceneButton.innerHTML = '<i class="fas fa-spinner fa-spin"></i> 截取中...';
        const imageData = await this.getScreenshot();
        if (!imageData) {
            this.dom.captureSceneButton.disabled = false;
            this.dom.captureSceneButton.innerHTML = '<i class="fas fa-camera"></i> 捕捉画面';
            return;
        }

        this.dom.canvas.width = imageData.width;
        this.dom.canvas.height = imageData.height;
        this.dom.drawCanvas.width = imageData.width;
        this.dom.drawCanvas.height = imageData.height;
        this.dom.previewCanvas.width = imageData.width;
        this.dom.previewCanvas.height = imageData.height;

        this.dom.canvasCtx.putImageData(imageData, 0, 0);
        this.dom.drawCtx.clearRect(0, 0, this.dom.drawCanvas.width, this.dom.drawCanvas.height);
        this.dom.previewCtx.clearRect(0, 0, this.dom.previewCanvas.width, this.dom.previewCanvas.height);

        this.dom.canvasWrapper.classList.add('has-image');
        if (this.historyManager) this.historyManager.clear();
        this.dom.downloadSceneButton.disabled = false;
        this.dom.continuousCaptureButton.disabled = false;
        this.dom.captureSceneButton.disabled = false;
        this.dom.captureSceneButton.innerHTML = '<i class="fas fa-camera"></i> 捕捉画面';
        showToast('画面截取成功', 'success');
    }

    toggleContinuousCapture() {
        if (this.continuousConfig.enabled) {
            this.stopContinuousCapture();
            this.dom.continuousCaptureButton.innerHTML = '<i class="fas fa-play"></i> 持续捕捉';
        } else {
            this.startContinuousCapture();
            this.dom.continuousCaptureButton.innerHTML = '<i class="fas fa-pause"></i> 停止';
        }
    }

    async startContinuousCapture() {
        this.continuousConfig.enabled = true;
        await this.captureScreen();
        this.continuousConfig.intervalId = setInterval(() => this.captureScreen(), this.continuousConfig.interval);
    }

    stopContinuousCapture() {
        this.continuousConfig.enabled = false;
        if (this.continuousConfig.intervalId) {
            clearInterval(this.continuousConfig.intervalId);
            this.continuousConfig.intervalId = null;
        }
    }

    setCaptureInterval(ms) {
        this.continuousConfig.interval = ms;
        if (this.continuousConfig.enabled) {
            this.stopContinuousCapture();
            this.startContinuousCapture();
        }
    }

    set accessScreenshotConfig(cfg) { Object.assign(this.screenshotConfig, cfg); }
    get accessScreenshotConfig() { return { ...this.screenshotConfig }; }

    calculateScale() {
        const rect = this.dom.drawCanvas.getBoundingClientRect();
        return {
            scaleX: this.dom.drawCanvas.width / (rect.width || 1),
            scaleY: this.dom.drawCanvas.height / (rect.height || 1)
        };
    }

    async downloadImage() {
        if (!this.dom.canvas.width) return showToast('请先截取画面', 'warning');
        const tempCanvas = document.createElement('canvas');
        tempCanvas.width = this.dom.canvas.width;
        tempCanvas.height = this.dom.canvas.height;
        const ctx = tempCanvas.getContext('2d');
        ctx.drawImage(this.dom.canvas, 0, 0);
        ctx.drawImage(this.dom.drawCanvas, 0, 0);
        const blob = await new Promise(r => tempCanvas.toBlob(r, 'image/png'));
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `screenshot-${Date.now()}.png`;
        a.click();
        URL.revokeObjectURL(url);
        showToast('下载完成', 'success');
    }

    getMergedImageData() {
        if (!this.dom.canvas.width) return '';
        const tempCanvas = document.createElement('canvas');
        tempCanvas.width = this.dom.canvas.width;
        tempCanvas.height = this.dom.canvas.height;
        const ctx = tempCanvas.getContext('2d');
        ctx.drawImage(this.dom.canvas, 0, 0);
        ctx.drawImage(this.dom.drawCanvas, 0, 0);
        return tempCanvas.toDataURL('image/png');
    }

    reset() {
        this.stopContinuousCapture();
        this.dom.canvasWrapper.classList.remove('has-image');
        this.dom.canvas.width = this.dom.canvas.height = 0;
        this.dom.drawCanvas.width = this.dom.drawCanvas.height = 0;
        this.dom.previewCanvas.width = this.dom.previewCanvas.height = 0;
        this.dom.downloadSceneButton.disabled = true;
        this.dom.continuousCaptureButton.disabled = true;
        this.dom.captureSceneButton.disabled = false;
        this.dom.captureSceneButton.innerHTML = '<i class="fas fa-camera"></i> 捕捉画面';
        this.dom.continuousCaptureButton.innerHTML = '<i class="fas fa-play"></i> 持续捕捉';
        if (this.historyManager) this.historyManager.clear();
    }

    setHistoryManager(hm) {
        this.historyManager = hm;
    }
}
