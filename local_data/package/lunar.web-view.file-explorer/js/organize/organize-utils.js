/**
 * 智能整理模块（常量与预处理）
 * 当当前层级文件数 ≥ ORGANIZE_THRESHOLD 时提供「智能整理」按钮，
 * 通过多模态 AI 识别文件并生成 移动/重命名 方案后自动执行。
 * 本文件负责常量、类别判断、文件预处理、AI 调用与进度/日志渲染。
 */

/** 智能整理触发阈值（当前层级文件数达到该值显示按钮） */
const ORGANIZE_THRESHOLD = 50;
/** 整理 AI 模型名（与全项目约定一致，硬编码） */
const ORGANIZE_MODEL = 'system-multimodal';
/** 文本内容取样长度（开头 / 结尾各 2000 字） */
const TEXT_SAMPLE_LEN = 2000;

/** 整理文件类别映射 */
const ORGANIZE_CATEGORIES = {
    text: ['.txt', '.md', '.log', '.csv', '.json', '.xml', '.yaml', '.yml', '.toml', '.ini', '.cfg', '.conf', '.html', '.css', '.js', '.ts', '.jsx', '.tsx', '.vue', '.go', '.py', '.java', '.c', '.cpp', '.h', '.hpp', '.rs', '.rb', '.sh', '.bat', '.ps1', '.sql', '.pem'],
    image: ['.png', '.jpg', '.jpeg', '.webp', '.gif', '.svg', '.bmp', '.ico', '.tiff', '.tif', '.avif']
};

/** 智能整理是否进行中 */
let isOrganizing = false;

/**
 * 获取文件的整理类别
 * @param {string} name - 文件名
 * @returns {string} text / image / other
 */
function getOrganizeCategory(name) {
    const ext = name.slice(name.lastIndexOf('.')).toLowerCase();
    for (const [cat, exts] of Object.entries(ORGANIZE_CATEGORIES)) {
        if (exts.includes(ext)) return cat;
    }
    return 'other';
}

/**
 * 读取文本文件内容样本（开头 2000 字 + 结尾 2000 字，不足 4000 字取全部）
 * @param {string} relativePath - 相对路径
 * @returns {Promise<string>}
 */
async function readTextSample(relativePath) {
    const response = await fetch(`/file/read/${relativePath}`);
    if (!response.ok) throw new Error('读取文本失败');
    const text = await response.text();
    if (text.length <= TEXT_SAMPLE_LEN * 2) return text;
    return text.substring(0, TEXT_SAMPLE_LEN) +
        '\n\n......[中间内容省略]......\n\n' +
        text.substring(text.length - TEXT_SAMPLE_LEN);
}

/**
 * 通过 /resize 处理图片，返回去除 data URI 前缀的 base64
 * @param {string} relativePath - 图片相对路径
 * @returns {Promise<string>}
 */
async function resizeImageData(relativePath) {
    // 先从服务器获取图片 Blob（/resize 需要服务端来源的数据）
    const fetchResponse = await fetch(`/file/read/${relativePath}`);
    if (!fetchResponse.ok) throw new Error('获取图片失败');
    const blob = await fetchResponse.blob();

    const formData = new FormData();
    formData.append('image', blob);
    const resizeResponse = await fetch('/resize', { method: 'POST', body: formData });
    if (!resizeResponse.ok) throw new Error('图片缩放失败');

    // /resize 返回帧数组，每帧含带 data URI 前缀的 base64，取第一帧
    const resizeData = await resizeResponse.json();
    const frames = Array.isArray(resizeData) ? resizeData : (resizeData.frames || []);
    const frame = frames[0] || {};
    let base64 = frame.base64 || resizeData.base64 || '';
    base64 = base64.replace(/^data:image\/\w+;base64,/, '');
    return base64;
}

/**
 * 预处理单个文件，构造提交给 AI 的文件描述
 * @param {Object} file - 文件对象
 * @returns {Promise<Object>}
 */
async function preprocessOrganizeFile(file) {
    const category = getOrganizeCategory(file.name);
    const meta = {
        name: file.name,
        size: formatFileSize(file.size),
        ext: file.name.slice(file.name.lastIndexOf('.')).toLowerCase(),
        category: category
    };

    switch (category) {
        case 'image': {
            try {
                const base64 = await resizeImageData(file.path);
                return { ...meta, type: 'image', base64 };
            } catch (err) {
                return { ...meta, type: 'meta', note: `图片预处理失败: ${err.message}` };
            }
        }
        case 'text': {
            try {
                const content = await readTextSample(file.path);
                return { ...meta, type: 'text', content };
            } catch (err) {
                return { ...meta, type: 'meta', note: `文本读取失败: ${err.message}` };
            }
        }
        default:
            return { ...meta, type: 'meta' };
    }
}

/**
 * 调用多模态 AI 接口
 * @param {Array<Object>} messages - 消息数组
 * @returns {Promise<string>} AI 返回的文本内容
 */
async function callOrganizeAI(messages) {
    const response = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            model: ORGANIZE_MODEL,
            messages: messages,
            stream: false
        })
    });
    if (!response.ok) throw new Error(`AI 调用失败: ${response.statusText}`);
    const data = await response.json();
    // OpenAI 兼容响应
    if (data.choices && data.choices[0]) {
        return data.choices[0].message.content;
    }
    // 代理包装响应
    if (data.success && data.data && data.data.choices) {
        return data.data.choices[0].message.content;
    }
    throw new Error('AI 响应格式异常');
}

/**
 * 执行整理操作（/file/organize）
 * @param {string} basePath - 工作目录基础路径（相对 LocalDir）
 * @param {Array<Object>} operations - 操作列表
 * @returns {Promise<Object>}
 */
async function executeOrganizeOperations(basePath, operations) {
    const response = await fetch('/file/organize', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ base_path: basePath, operations: operations })
    });
    if (!response.ok) throw new Error('执行整理操作失败');
    return await response.json();
}

/**
 * 更新整理进度条与状态文字
 * @param {number} percent - 进度百分比
 * @param {string} status - 状态文字
 */
function setOrganizeProgress(percent, status) {
    document.getElementById('organize-progress-fill').style.width = `${percent}%`;
    document.getElementById('organize-status').textContent = status;
}

/**
 * 追加整理日志
 * @param {string} message - 日志内容
 * @param {string} type - 日志类型: info / success / error / warning
 */
function addOrganizeLog(message, type = 'info') {
    const log = document.getElementById('organize-log');
    const item = document.createElement('div');
    item.className = `organize-log-item ${type}`;
    const icons = {
        success: 'fa-check-circle',
        error: 'fa-exclamation-circle',
        info: 'fa-circle-info',
        warning: 'fa-exclamation-triangle'
    };
    item.innerHTML = `<i class="fas ${icons[type] || icons.info}"></i><span>${message}</span>`;
    log.appendChild(item);
    log.scrollTop = log.scrollHeight;
}
