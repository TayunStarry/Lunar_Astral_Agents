/**
 * 工具函数模块
 * 提供文件管理器所需的各种辅助函数
 */

/**
 * 格式化文件大小
 * @param {number} bytes - 文件大小（字节）
 * @returns {string} - 格式化后的文件大小字符串
 */
export function formatFileSize(bytes) {
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
export function formatDate(dateString) {
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
export function encodeFileName(filename) {
    const encodedParams = encodeURIComponent(filename);
    const decodedParams = encodedParams.replace(/%([0-9A-F]{2})/g, (_, p1) => String.fromCharCode(parseInt(p1, 16)));
    return btoa(decodedParams);
}

/**
 * 检查文件名是否有效
 * @param {string} filename - 文件名
 * @returns {boolean} - 是否有效
 */
export function isValidFileName(filename) {
    const invalidChars = /[<>:/"\\|?*]/;
    return !invalidChars.test(filename) && filename.trim() !== '';
}

/**
 * 检查文件是否为图片文件
 * @param {string} filename - 文件名
 * @returns {boolean} - 是否为图片文件
 */
export function isImageFile(filename) {
    const imageExtensions = ['.jpg', '.jpeg', '.png', '.gif', '.bmp', '.svg', '.webp'];
    const extension = filename.toLowerCase().slice(filename.lastIndexOf('.'));
    return imageExtensions.includes(extension);
}

/**
 * 检查文件是否为视频文件
 * @param {string} filename - 文件名
 * @returns {boolean} - 是否为视频文件
 */
export function isVideoFile(filename) {
    const videoExtensions = ['.mp4', '.avi', '.mov', '.wmv', '.flv', '.webm', '.mkv'];
    const ext = filename.toLowerCase().slice(filename.lastIndexOf('.'));
    return videoExtensions.includes(ext);
}

/**
 * 检查文件是否为音频文件
 * @param {string} filename - 文件名
 * @returns {boolean} - 是否为音频文件
 */
export function isAudioFile(filename) {
    const audioExtensions = ['.mp3', '.wav', '.ogg', '.flac', '.aac', '.m4a', '.wma'];
    const ext = filename.toLowerCase().slice(filename.lastIndexOf('.'));
    return audioExtensions.includes(ext);
}

/**
 * 检查文件是否为文本文件
 * @param {string} filename - 文件名
 * @returns {boolean} - 是否为文本文件
 */
export function isTextFile(filename) {
    const plainText = ['.txt', '.md', '.log'];
    const web = ['.html', '.css', '.js', '.ts', '.jsx', '.tsx', '.vue'];
    const backend = ['.py', '.java', '.php', '.rb', '.go', '.rs', '.kt', '.scala', '.cs', '.swift'];
    const system = ['.c', '.cpp', '.cxx', '.h', '.hpp'];
    const data = ['.json', '.xml', '.csv', '.sql', '.yml', '.yaml'];
    const script = ['.sh', '.bat', '.ps1'];
    const config = ['.pem'];
    const textExtensions = [...plainText, ...web, ...backend, ...system, ...data, ...script, ...config];
    const ext = filename.toLowerCase().slice(filename.lastIndexOf('.'));
    return textExtensions.includes(ext);
}

/**
 * 获取文件图标
 * @param {string} filename - 文件名
 * @returns {string} - 文件图标 HTML 字符串
 */
export function getFileIcon(filename) {
    const ext = filename.toLowerCase().slice(filename.lastIndexOf('.'));
    const iconMap = {
        '.txt': '<i class="fas fa-file-alt"></i>',
        '.md': '<i class="fab fa-markdown"></i>',
        '.json': '<i class="fas fa-file-code"></i>',
        '.html': '<i class="fab fa-html5"></i>',
        '.css': '<i class="fab fa-css3-alt"></i>',
        '.js': '<i class="fab fa-js"></i>',
        '.ts': '<i class="fab fa-js"></i>',
        '.pdf': '<i class="fas fa-file-pdf"></i>',
        '.doc': '<i class="fas fa-file-word"></i>',
        '.docx': '<i class="fas fa-file-word"></i>',
        '.xls': '<i class="fas fa-file-excel"></i>',
        '.xlsx': '<i class="fas fa-file-excel"></i>',
        '.ppt': '<i class="fas fa-file-powerpoint"></i>',
        '.pptx': '<i class="fas fa-file-powerpoint"></i>',
        '.zip': '<i class="fas fa-file-archive"></i>',
        '.rar': '<i class="fas fa-file-archive"></i>',
        '.7z': '<i class="fas fa-file-archive"></i>',
        '.mp3': '<i class="fas fa-file-audio"></i>',
        '.mp4': '<i class="fas fa-file-video"></i>',
    };
    return iconMap[ext] || '<i class="fas fa-file"></i>';
}

/**
 * 获取文件类型的 MIME 字符串
 * @param {string} extension - 文件扩展名
 * @returns {string} - 文件类型的 MIME 字符串
 */
export function getFileType(extension) {
    const mimeTypes = {
        '.txt': 'text/plain',
        '.md': 'text/markdown',
        '.json': 'application/json',
        '.html': 'text/html',
        '.css': 'text/css',
        '.js': 'application/javascript',
        '.jpg': 'image/jpeg',
        '.jpeg': 'image/jpeg',
        '.png': 'image/png',
        '.gif': 'image/gif',
        '.svg': 'image/svg+xml',
        '.webp': 'image/webp',
        '.mp4': 'video/mp4',
        '.avi': 'video/x-msvideo',
        '.mov': 'video/quicktime',
        '.wmv': 'video/x-ms-wmv',
        '.flv': 'video/x-flv',
        '.webm': 'video/webm',
        '.pdf': 'application/pdf',
        '.zip': 'application/zip'
    };
    return mimeTypes[extension.toLowerCase()] || 'application/octet-stream';
}

/**
 * 显示 Toast 通知
 * @param {string} message - 消息内容
 * @param {string} type - 消息类型 ('success', 'error', 'info')
 */
export function showToast(message, type = 'info') {
    const toast = document.getElementById('toast');
    toast.textContent = message;
    toast.className = `toast ${type} show`;
    setTimeout(() => {
        toast.classList.remove('show');
    }, 3000);
}
