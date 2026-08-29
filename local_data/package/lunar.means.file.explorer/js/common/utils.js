/**
 * 通用工具函数模块
 * 提供文件管理器所需的格式化、编码、校验、HTML 转义与 Toast 提示等辅助函数
 */

/**
 * 格式化文件大小
 * @param {number} bytes - 文件大小（字节）
 * @returns {string} - 格式化后的文件大小字符串
 */
function formatFileSize(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

/**
 * 格式化日期字符串
 * @param {string} dateString - 日期字符串（ISO 8601 格式）
 * @returns {string} - 格式化后的日期字符串
 */
function formatDate(dateString) {
    const date = new Date(dateString);
    return date.toLocaleString('zh-CN', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit'
    });
}

/**
 * 对文件名进行编码
 * @param {string} filename - 文件名
 * @returns {string} - 编码后的文件名
 */
function encodeFileName(filename) {
    const encodedParams = encodeURIComponent(filename);
    const decodedParams = encodedParams.replace(/%([0-9A-F]{2})/g, (_, p1) => String.fromCharCode(parseInt(p1, 16)));
    return btoa(decodedParams);
}

/**
 * 检查文件名是否有效
 * @param {string} filename - 文件名
 * @returns {boolean} - 是否有效
 */
function isValidFileName(filename) {
    const invalidChars = /[<>:/"\\|?*]/;
    return !invalidChars.test(filename) && filename.trim() !== '';
}

/**
 * HTML 转义（用于安全渲染后端返回的元数据文本）
 * @param {string} str - 原始文本
 * @returns {string} - 转义后的文本
 */
function mediaEscapeHTML(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

/**
 * 显示 Toast 通知
 * @param {string} message - 消息内容
 * @param {string} type - 消息类型 ('success', 'error', 'info')
 */
function showToast(message, type = 'info') {
    const toast = document.getElementById('toast');
    toast.textContent = message;
    toast.className = `toast ${type} show`;
    setTimeout(() => {
        toast.classList.remove('show');
    }, 3000);
}

/**
 * 从视频 URL 获取视频缩略图 URL
 * @param {string} videoUrl - 视频文件访问地址
 * @returns {Promise<string>} 缩略图的 data URL
 */
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
