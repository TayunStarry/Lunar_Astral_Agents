/**
 * 文件类型判断模块
 * 提供文件扩展名识别、图标与 MIME 类型映射等辅助函数
 */

/**
 * 检查文件是否为图片文件
 * @param {string} filename - 文件名
 * @returns {boolean} - 是否为图片文件
 */
function isImageFile(filename) {
    const imageExtensions = ['.jpg', '.jpeg', '.png', '.gif', '.bmp', '.svg', '.webp'];
    const extension = filename.toLowerCase().slice(filename.lastIndexOf('.'));
    return imageExtensions.includes(extension);
}

/**
 * 检查文件是否为视频文件
 * @param {string} filename - 文件名
 * @returns {boolean} - 是否为视频文件
 */
function isVideoFile(filename) {
    const videoExtensions = ['.mp4', '.avi', '.mov', '.wmv', '.flv', '.webm', '.mkv'];
    const ext = filename.toLowerCase().slice(filename.lastIndexOf('.'));
    return videoExtensions.includes(ext);
}

/**
 * 检查文件是否为音频文件
 * @param {string} filename - 文件名
 * @returns {boolean} - 是否为音频文件
 */
function isAudioFile(filename) {
    const audioExtensions = ['.mp3', '.wav', '.ogg', '.flac', '.aac', '.m4a', '.wma'];
    const ext = filename.toLowerCase().slice(filename.lastIndexOf('.'));
    return audioExtensions.includes(ext);
}

/**
 * 可重编码的图片格式扩展名列表（png / jpg / jpeg / webp）
 */
const CONVERTIBLE_IMAGE_EXTS = ['.png', '.jpg', '.jpeg', '.webp'];

/**
 * 检查文件是否为可重编码的图片文件
 * @param {string} filename - 文件名
 * @returns {boolean} - 是否可重编码
 */
function isConvertibleImage(filename) {
    const ext = filename.toLowerCase().slice(filename.lastIndexOf('.'));
    return CONVERTIBLE_IMAGE_EXTS.includes(ext);
}

/**
 * 检查文件是否为 GGUF 模型文件
 * @param {string} filename - 文件名
 * @returns {boolean} - 是否为 GGUF 文件
 */
function isGGUFFile(filename) {
    const ext = filename.toLowerCase().slice(filename.lastIndexOf('.'));
    return ext === '.gguf';
}

/**
 * 检查文件是否为 ZIP 压缩包
 * @param {string} filename - 文件名
 * @returns {boolean} - 是否为 ZIP 文件
 */
function isZipFile(filename) {
    const ext = filename.toLowerCase().slice(filename.lastIndexOf('.'));
    return ext === '.zip';
}

/**
 * 检查文件是否为文本文件
 * @param {string} filename - 文件名
 * @returns {boolean} - 是否为文本文件
 */
function isTextFile(filename) {
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
function getFileIcon(filename) {
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
function getFileType(extension) {
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
