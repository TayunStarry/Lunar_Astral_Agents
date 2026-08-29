// ============================================================
//  星月智能 · 消息终端 — 文件分类 / 上传 / 读取
// ============================================================

// ---------- 文件上传辅助 ----------
function getFileCategory(file) {
    if (file.type.startsWith('image/')) return 'image';
    if (file.type.startsWith('video/')) return 'video';
    if (file.type.startsWith('audio/')) return 'audio';
    const ext = file.name.slice(file.name.lastIndexOf('.') + 1).toLowerCase();
    const textExts = ['txt', 'md', 'json', 'xml', 'yaml', 'yml', 'toml', 'csv', 'html', 'htm', 'css', 'js', 'ts', 'jsx', 'tsx', 'go', 'rs', 'java', 'c', 'cpp', 'cxx', 'h', 'hpp', 'cs', 'py', 'rb', 'sh', 'ps1', 'bat', 'log'];
    if (textExts.includes(ext)) return 'text';
    return 'other';
}

async function calculateFileHash(file) {
    return new Promise((resolve) => {
        const reader = new FileReader();
        reader.onload = async (e) => {
            try {
                const arrayBuffer = e.target.result;
                const hashBuffer = await crypto.subtle.digest('SHA-256', arrayBuffer);
                const hashArray = Array.from(new Uint8Array(hashBuffer));
                const fullHash = hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
                resolve(fullHash.substring(0, 16));
            } catch {
                resolve(encodeFilePath(file.name).slice(-16));
            }
        };
        reader.onerror = () => resolve(encodeFilePath(file.name).slice(-16));
        reader.readAsArrayBuffer(file);
    });
}

// 保存文件到服务器，返回可访问的 fileUrl
async function saveFile(file) {
    const category = getFileCategory(file);
    const prefix = (category === 'image' || category === 'video' || category === 'audio') ? 'images/' : 'documents/';
    const fileHash = await calculateFileHash(file);
    const ext = file.name.slice(file.name.lastIndexOf('.')).toLowerCase() || '.bin';
    const newFileName = `${fileHash}${ext}`;
    const res = await fetch('/file/write', {
        method: 'POST',
        headers: {
            'X-File-Name': encodeFilePath(prefix + newFileName),
            'X-Overwrite': 'true'
        },
        body: file
    });
    if (!res.ok) throw new Error('文件上传失败');
    const result = await res.json();
    return `${window.location.origin}/file/read/${result.filename}`;
}

function fileToRawBase64(file) {
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => {
            const result = reader.result;
            if (typeof result !== 'string' || !result.startsWith('data:')) {
                reject(new Error('读取文件失败'));
                return;
            }
            resolve(result.slice(result.indexOf(',') + 1));
        };
        reader.onerror = () => reject(reader.error || new Error('FileReader error'));
        reader.readAsDataURL(file);
    });
}

function getAudioFormat(file) {
    const ext = file.name.slice(file.name.lastIndexOf('.') + 1).toLowerCase();
    if (ext === 'wav') return 'wav';
    if (ext === 'mp3') return 'mp3';
    return null;
}

function readFileAsDataUrl(file) {
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(reader.result);
        reader.onerror = reject;
        reader.readAsDataURL(file);
    });
}

function readFileAsText(file) {
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(reader.result);
        reader.onerror = reject;
        reader.readAsText(file);
    });
}
