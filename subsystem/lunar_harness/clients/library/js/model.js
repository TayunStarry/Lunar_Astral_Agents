// model.js 前端数据模型：类型判定、目录子项筛选、格式化。

const TEXT_EXTS = new Set([
    '.txt', '.md', '.markdown', '.json', '.jsonl', '.ndjson', '.log', '.csv', '.tsv',
    '.xml', '.yaml', '.yml', '.toml', '.ini', '.cfg', '.conf', '.env', '.gitignore',
    '.js', '.mjs', '.cjs', '.ts', '.tsx', '.jsx', '.vue', '.html', '.htm', '.css',
    '.go', '.py', '.rs', '.java', '.kt', '.c', '.h', '.cpp', '.hpp', '.cs', '.rb',
    '.php', '.swift', '.sh', '.ps1', '.bat', '.cmd', '.sql', '.lua', '.r', '.svg',
]);

const IMAGE_EXTS = new Set(['.png', '.jpg', '.jpeg', '.webp', '.gif', '.bmp', '.ico', '.apng', '.tif', '.tiff', '.avif']);
const VIDEO_EXTS = new Set(['.mp4', '.webm', '.mkv', '.mov', '.avi', '.wmv', '.flv', '.m4v', '.mpg', '.mpeg']);
const AUDIO_EXTS = new Set(['.mp3', '.wav', '.flac', '.ogg', '.m4a', '.aac', '.opus', '.wma']);
const DATABASE_EXTS = new Set(['.db', '.sqlite', '.sqlite3']);
const ARCHIVE_SUFFIXES = ['.tar.gz', '.tgz', '.zip', '.tar', '.gz'];
const RUNNABLE_EXTS = new Set(['.js', '.mjs', '.html', '.htm']);

// extOf 小写扩展名。
export function extOf(name) {
    const idx = name.lastIndexOf('.');
    return idx >= 0 ? name.slice(idx).toLowerCase() : '';
}

// kindOf 预览类型：text / image / video / audio / database / archive / binary。
export function kindOf(name) {
    const lower = String(name || '').toLowerCase();
    // 复合扩展名优先：.tar.gz 的 extOf 只会得到 .gz。
    if (ARCHIVE_SUFFIXES.some((suffix) => lower.endsWith(suffix))) return 'archive';
    const ext = extOf(name);
    if (DATABASE_EXTS.has(ext)) return 'database';
    if (IMAGE_EXTS.has(ext)) return 'image';
    if (VIDEO_EXTS.has(ext)) return 'video';
    if (AUDIO_EXTS.has(ext)) return 'audio';
    if (TEXT_EXTS.has(ext)) return 'text';
    return 'binary';
}

// isRunnable 是否提供沙箱运行入口。
export function isRunnable(name) {
    return RUNNABLE_EXTS.has(extOf(name));
}

// statusClass 状态 → CSS 类名。
export function statusClass(status) {
    switch (status) {
        case 'active': return 'status-active';
        case 'ok': return 'status-ok';
        case 'error': return 'status-error';
        default: return '';
    }
}

// humanSize 人类可读体积。
export function humanSize(bytes) {
    if (!bytes || bytes < 1024) return `${bytes || 0} B`;
    const units = ['KB', 'MB', 'GB', 'TB'];
    let value = bytes / 1024;
    let index = 0;
    while (value >= 1024 && index < units.length - 1) {
        value /= 1024;
        index += 1;
    }
    return `${value.toFixed(value >= 100 ? 0 : 1)} ${units[index]}`;
}

// formatTime 秒级时间戳 → 本地时间。
export function formatTime(seconds) {
    if (!seconds) return '—';
    const date = new Date(seconds * 1000);
    const pad = (n) => String(n).padStart(2, '0');
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

// parentPathOf 条目的父目录路径（存储根本身不参与展示）。
export function parentPathOf(entry) {
    const slash = entry.path.lastIndexOf('/');
    return slash >= 0 ? entry.path.slice(0, slash) : '';
}

// childrenOf 目录下的直接子项；dirPath 为空串时返回所有存储根的直接子项。
// database 是扁平视图：直接返回其所有子目录中的库文件，不再保留中间层级。
export function childrenOf(entries, dirPath) {
    if (dirPath === 'database') {
        return entries.filter((entry) => !entry.dir && entry.path.startsWith('database/'));
    }
    return entries.filter((entry) => {
        if (entry.dir && entry.path === dirPath) return false;
        const parent = parentPathOf(entry);
        if (dirPath === '') {
            // 顶层目录（如 database）直接展示；4 个句柄存储根仍按原规则拍平。
            if (parent === '') return entry.dir;
            return parent !== 'database' && !parent.includes('/');
        }
        return parent === dirPath;
    });
}

// rootLabel 存储根的中文名，用于在去掉存储根层后仍能区分资源归属。
export function rootLabel(root) {
    switch (root) {
        case 'documents': return '文档';
        case 'audios': return '音频';
        case 'multimedia': return '多媒体';
        case 'unknown': return '未知';
        case 'database': return '数据库';
        default: return root || '';
    }
}

// parentOf 上级路径（根目录返回空串）。
export function parentOf(path) {
    const slash = path.lastIndexOf('/');
    return slash >= 0 ? path.slice(0, slash) : '';
}

// nameOf 路径中的文件名。
export function nameOf(path) {
    const slash = path.lastIndexOf('/');
    return slash >= 0 ? path.slice(slash + 1) : path;
}
