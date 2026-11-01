// model.js 前端数据模型：类型判定、目录树组装、格式化。

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
const RUNNABLE_EXTS = new Set(['.js', '.mjs', '.html', '.htm']);

// extOf 小写扩展名。
export function extOf(name) {
    const idx = name.lastIndexOf('.');
    return idx >= 0 ? name.slice(idx).toLowerCase() : '';
}

// kindOf 预览类型：text / image / video / audio / binary。
export function kindOf(name) {
    const ext = extOf(name);
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

// buildTree 扁平条目 → 按根组织的树。
export function buildTree(entries, roots) {
    const rootNodes = roots.map((root) => ({
        name: root, path: root, dir: true, root: true, expanded: true, children: [],
    }));
    const byPath = new Map(rootNodes.map((node) => [node.path, node]));
    for (const entry of entries) {
        const node = {
            name: entry.name,
            path: entry.path,
            dir: entry.dir,
            root: false,
            size: entry.size,
            mtime: entry.mtime,
            children: [],
        };
        byPath.set(node.path, node);
        const slash = entry.path.lastIndexOf('/');
        const parentPath = slash >= 0 ? entry.path.slice(0, slash) : entry.root;
        const parent = byPath.get(parentPath) || byPath.get(entry.root);
        if (parent) parent.children.push(node);
    }
    const sortNodes = (nodes) => {
        nodes.sort((a, b) => (a.dir === b.dir ? a.name.localeCompare(b.name, 'zh-Hans-CN') : a.dir ? -1 : 1));
        nodes.forEach((node) => sortNodes(node.children));
    };
    sortNodes(rootNodes);
    return rootNodes;
}

// childrenOf 目录节点下的直接子项。
export function childrenOf(entries, dirPath) {
    return entries.filter((entry) => {
        if (entry.dir && entry.path === dirPath) return false;
        const slash = entry.path.lastIndexOf('/');
        const parent = slash >= 0 ? entry.path.slice(0, slash) : entry.root;
        return parent === dirPath;
    });
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
