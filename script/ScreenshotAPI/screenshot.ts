import * as EntryAPI from '../EntryAPI/code';
/**
 * 屏幕捕获核心功能
 *
 * 提供持续捕捉、区域截图、最大截图等功能
 */
export class ScreenshotCore {
    /** 持续捕捉配置 */
    private continuousConfig: EntryAPI.ContinuousConfig = { enabled: false, interval: 1000, intervalId: null };
    /** 截图配置 */
    private screenshotConfig: EntryAPI.RegionConfig = { x: 0, y: 0, width: 1920, height: 1080 };
    /** 缩放比例 */
    private scale: number = 1.0;
    /**
     * 更新缩放比例
     */
    public set updateScale(scale: number) {
        this.scale = scale;
    };
    /**
     * 获取当前缩放比例
     */
    public get updateScale(): number {
        return this.scale;
    };
    /**
     * 获取截图
     *
     * 根据配置调用相应的 API 端点获取截图
     */
    public async getScreenshot(): Promise<ImageData | null> {
        try {
            /** 截图配置 */
            const region = this.screenshotConfig;
            /** 缩放比例 */
            const scale = this.scale;
            /** 截图服务的数据响应 */
            let fetchResponse: Response | undefined = undefined;
            // 如果启用了最大截图功能，使用 POST 请求
            if (this.maximumScreenshot) fetchResponse = await fetch('/capture',
                {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json', 'Cache-Control': 'no-store, no-cache, must-revalidate, max-age=0' },
                    body: JSON.stringify(
                        {
                            display_index: -1,
                            scale: scale.toString(),
                            format: 'png'
                        }
                    )
                }
            );
            else {
                /** 区域截图端点的 URL */
                const url = `/capture/region?region=${region.x},${region.y},${region.width},${region.height}&scale=${scale}`;
                fetchResponse = await fetch(url);
            }
            // 检查响应是否成功
            if (!fetchResponse || !fetchResponse.ok) throw new Error(`HTTP ${fetchResponse.status}: ${fetchResponse.statusText}`);
            /** 截图服务返回的二进制数据 */
            const blob = await fetchResponse.blob();
            /** 新建空白图像对象 */
            const img = new Image();
            // 加载图像数据
            await new Promise(
                (resolve, reject) => {
                    img.onload = resolve;
                    img.onerror = reject;
                    img.src = URL.createObjectURL(blob);
                }
            );
            /** 临时 canvas 元素 */
            const tempCanvas = document.createElement('canvas');
            // 设置 canvas 尺寸为图像尺寸
            tempCanvas.width = img.width;
            tempCanvas.height = img.height;
            /** 临时 canvas 上下文 */
            const tempCtx = tempCanvas.getContext('2d')!;
            // 将图像绘制到 canvas
            tempCtx.drawImage(img, 0, 0);
            // 返回绘制后的图像数据(base64编码)
            return tempCtx.getImageData(0, 0, tempCanvas.width, tempCanvas.height);
        }
        catch (err: any) {
            console.error('截图获取失败:', err);
            EntryAPI.showSystemMessage('截图获取失败: ' + err.message, 'error');
            return null;
        }
    };
    /**
     * 是否为全屏截图
     */
    public get maximumScreenshot(): boolean {
        /** 获取截图配置 */
        const region = this.screenshotConfig;
        // 检查是否为全屏截图
        return region.width.toString() == EntryAPI.regionWidthSlider.max && region.height.toString() == EntryAPI.regionHeightSlider.max;
    };
    /**
     * 捕捉画面（单次）
     *
     * 获取截图并显示到画布
     */
    public async captureScreen(): Promise<void> {
        try {
            // 禁用"捕捉画面"按钮，显示加载状态
            EntryAPI.captureSceneButton.disabled = true;
            EntryAPI.captureSceneButton.innerHTML = '<i class="fas fa-spinner fa-spin"></i> 捕捉中...';

            // 获取截图
            const imageData = await this.getScreenshot();
            if (!imageData) return;

            // 设置画布尺寸
            EntryAPI.canvas.width = imageData.width;
            EntryAPI.canvas.height = imageData.height;
            EntryAPI.drawCanvas.width = imageData.width;
            EntryAPI.drawCanvas.height = imageData.height;
            EntryAPI.previewCanvas.width = imageData.width;
            EntryAPI.previewCanvas.height = imageData.height;

            // 绘制截图到主画布
            EntryAPI.canvasCtx.putImageData(imageData, 0, 0);
            EntryAPI.canvasWrapper.style.display = 'block';

            EntryAPI.captureSceneButton.disabled = false;
            EntryAPI.captureSceneButton.innerHTML = '<i class="fas fa-camera"></i> 捕捉画面';

            // 清空历史记录
            EntryAPI.historyManager.clear();

        } catch (err: any) {
            console.error('捕捉失败:', err);
            EntryAPI.showSystemMessage('捕捉失败: ' + err.message, 'error');

            // 重置按钮状态
            EntryAPI.captureSceneButton.disabled = false;
            EntryAPI.captureSceneButton.innerHTML = '<i class="fas fa-camera"></i> 捕捉画面';
        }
    };
    /**
     * 切换持续捕捉
     */
    public toggleContinuousCapture(): void {
        if (this.continuousConfig.enabled) {
            // 停止持续捕捉
            this.stopContinuousCapture();
            EntryAPI.continuousCaptureButton.innerHTML = '<i class="fas fa-play"></i> 持续捕捉';
            EntryAPI.continuousCaptureButton.classList.remove('active');
        } else {
            // 开始持续捕捉
            this.startContinuousCapture();
            EntryAPI.continuousCaptureButton.innerHTML = '<i class="fas fa-pause"></i> 停止持续';
            EntryAPI.continuousCaptureButton.classList.add('active');
        }
    };
    /**
     * 开始持续捕捉
     */
    private startContinuousCapture(): void {
        // 开启持续捕捉标志
        this.continuousConfig.enabled = true;
        // 先立即捕捉一次
        this.captureScreen();
        // 设置定时器
        this.continuousConfig.intervalId = window.setInterval(() => {
            this.captureScreen();
        }, this.continuousConfig.interval);
    };
    /**
     * 停止持续捕捉
     */
    public stopContinuousCapture(): void {
        // 关闭持续捕捉标志
        this.continuousConfig.enabled = false;
        // 清除定时器
        if (this.continuousConfig.intervalId) {
            clearInterval(this.continuousConfig.intervalId);
            this.continuousConfig.intervalId = null;
        }
    };
    /**
     * 设置捕捉间隔
     */
    public setCaptureInterval(interval: number): void {
        this.continuousConfig.interval = interval;

        // 如果正在持续捕捉，重启定时器
        if (this.continuousConfig.enabled) {
            this.stopContinuousCapture();
            this.startContinuousCapture();
        }
    };
    /**
     * 更新截图配置
     */
    public set accessScreenshotConfig(config: Partial<EntryAPI.RegionConfig>) {
        this.screenshotConfig = { ...this.screenshotConfig, ...config };
    };
    /**
     * 获取截图配置
     */
    public get accessScreenshotConfig(): EntryAPI.RegionConfig {
        return { ...this.screenshotConfig };
    };
    /**
     * 计算画布坐标缩放比例
     *
     * 根据绘图画布的实际显示尺寸与逻辑尺寸，返回 X、Y 方向的缩放比例，
     * 用于将鼠标在 DOM 上的坐标转换为画布上的真实像素坐标。
     *
     * @returns {CanvasScale} 包含 scaleX 与 scaleY 的缩放比例对象
     */
    public calculateScale(): EntryAPI.CanvasScale {
        /** 获取绘图画布的 DOM 矩形信息 */
        const rect = EntryAPI.drawCanvas.getBoundingClientRect();
        /** 计算 X 方向的缩放比例 */
        const scaleX = EntryAPI.drawCanvas.width / rect.width;
        /** 计算 Y 方向的缩放比例 */
        const scaleY = EntryAPI.drawCanvas.height / rect.height;
        // 返回包含 X、Y 方向缩放比例的对象
        return { scaleX, scaleY };
    };
    /**
     * 下载合并后的截图
     *
     * 将主画布与绘图画布叠加后生成 PNG 文件并上传至服务器
     */
    public async downloadImage(): Promise<void> {
        /** 创建临时画布用于合并主画布与绘图画布 */
        const tempCanvas = document.createElement('canvas');
        // 设置临时画布大小与主画布一致
        [tempCanvas.width, tempCanvas.height] = [EntryAPI.canvas.width, EntryAPI.canvas.height];
        /** 获取临时画布2D上下文 */
        const tempCtx = tempCanvas.getContext('2d')!;
        // 先绘制主截图
        tempCtx.drawImage(EntryAPI.canvas, 0, 0);
        // 再绘制用户标注
        tempCtx.drawImage(EntryAPI.drawCanvas, 0, 35);
        /** 将合并结果转为 Blob */
        const blob = await new Promise<Blob | null>(resolve => tempCanvas.toBlob(resolve, 'image/png'));
        // 检查是否成功生成 Blob
        if (!blob) return EntryAPI.showSystemMessage('图片生成失败', 'error');
        /** 生成带时间戳的文件名 */
        const fileName = 'screenshot-' + new Date().toISOString().replace(/:/g, '-') + '.png';
        /** 创建文件对象 */
        const file = new File([blob], fileName, { type: 'image/png' });
        // 上传文件至服务器
        await EntryAPI.saveImageToServer(file);
        // 提示用户图片已保存
        EntryAPI.showSystemMessage('图片已保存', 'success');
    };
    /**
     * 获取主画布与绘图画布合并后的 DataURL（base64 格式）
     *
     * 用于将用户标注与原始截图一起导出为 PNG 图片数据
     *
     * @returns {string} 合并后的 PNG 图片 DataURL
     */
    public getMergedImageData(): string {
        /** 创建临时画布，用于叠加主截图与绘图画布 */
        const tempCanvas = document.createElement('canvas');
        // 设置临时画布尺寸与主画布一致
        [tempCanvas.width, tempCanvas.height] = [EntryAPI.canvas.width, EntryAPI.canvas.height];
        /** 获取临时画布的 2D 绘图上下文 */
        const tempCtx = tempCanvas.getContext('2d')!;
        // 先绘制原始截图
        tempCtx.drawImage(EntryAPI.canvas, 0, 0);
        // 再绘制用户标注
        tempCtx.drawImage(EntryAPI.drawCanvas, 0, 35);
        // 返回合并后的 PNG DataURL
        return tempCanvas.toDataURL('image/png');
    };
    /**
     * 重置所有截图工具状态
     *
     * 停止媒体流、隐藏相关容器、禁用/启用按钮、清空画布并清空历史记录
     *
     * @param historyManager 历史记录管理器实例，用于清空历史记录
     */
    public resetTool(historyManager: EntryAPI.HistoryManager): void {
        // 停止持续捕捉
        this.stopContinuousCapture();
        EntryAPI.canvasWrapper.style.display = 'none';

        // 禁用下载、撤销、持续捕捉按钮，启用捕捉画面按钮
        EntryAPI.downloadSceneButton.disabled = true;
        EntryAPI.undoDrawButton.disabled = true;
        EntryAPI.continuousCaptureButton.disabled = true;
        EntryAPI.captureSceneButton.disabled = false;
        EntryAPI.captureSceneButton.innerHTML = '<i class="fas fa-camera"></i> 捕捉画面';
        EntryAPI.continuousCaptureButton.innerHTML = '<i class="fas fa-play"></i> 持续捕捉';
        EntryAPI.continuousCaptureButton.classList.remove('active');

        // 清空主画布、绘图画布与预览画布
        EntryAPI.canvasCtx.clearRect(0, 0, EntryAPI.canvas.width, EntryAPI.canvas.height);
        EntryAPI.drawCtx.clearRect(0, 0, EntryAPI.drawCanvas.width, EntryAPI.drawCanvas.height);
        EntryAPI.previewCtx.clearRect(0, 0, EntryAPI.previewCanvas.width, EntryAPI.previewCanvas.height);

        // 清空历史记录并重置撤销按钮状态
        historyManager.clear();
    };
}