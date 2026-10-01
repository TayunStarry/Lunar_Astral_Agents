/**
 * 图像工坊 - 页面逻辑（Zero-LTP：纯人类操作，无智能体）
 * 职责：
 * - 生成参数：正面/负面提示词、参考图像（图生图）、噪声强度、宽高、迭代步数、
 *   生成数量、提示词权重、随机种子、图像超分
 * - 生成流程：POST /generate 提交任务 → GET /generate/wait?task_id= 等待完成
 *   （兼容裸 JSON 与 SSE data: 帧两种响应）→ read_path 即 /file/read/multimedia/generated/…
 * - 生成画廊：浏览 /file/list/multimedia/generated（新图高亮），点击经
 *   multimedia_preview 的全局 previewImage(path, fileName) 预览
 * - 参考图像：经 /file/write 上传到 multimedia/reference，init_img 传相对路径
 */

// ===== DOM 引用 =====
const themeToggle = document.getElementById('themeToggle');
const themeIcon = document.getElementById('themeIcon');
const themeLabel = document.getElementById('themeLabel');
const promptInput = document.getElementById('promptInput');
const negativeInput = document.getElementById('negativeInput');
const pickRefBtn = document.getElementById('pickRefBtn');
const clearRefBtn = document.getElementById('clearRefBtn');
const refPreviewWrap = document.getElementById('refPreviewWrap');
const refPreviewImg = document.getElementById('refPreviewImg');
const refHint = document.getElementById('refHint');
const strengthInput = document.getElementById('strengthInput');
const stepsInput = document.getElementById('stepsInput');
const widthInput = document.getElementById('widthInput');
const heightInput = document.getElementById('heightInput');
const batchInput = document.getElementById('batchInput');
const cfgInput = document.getElementById('cfgInput');
const seedInput = document.getElementById('seedInput');
const superResInput = document.getElementById('superResInput');
const generateBtn = document.getElementById('generateBtn');
const genStatus = document.getElementById('genStatus');
const galleryStats = document.getElementById('galleryStats');
const refreshGalleryBtn = document.getElementById('refreshGalleryBtn');
const galleryGrid = document.getElementById('galleryGrid');
const refFileInput = document.getElementById('refFileInput');

// ===== 常量 =====

/** 生成结果目录（相对 local_data，经 /file/list 与 /file/read 访问） */
const GENERATED_DIR = 'multimedia/generated';
/** 参考图像上传目录（相对 local_data，init_img 传相对路径） */
const REFERENCE_DIR = 'multimedia/reference';
/** 画廊单次渲染上限（按名称倒序取最新） */
const GALLERY_MAX = 120;
/** 图片扩展名过滤 */
const IMAGE_EXT_RE = /\.(png|webp|jpe?g)$/i;

// ===== 深色/浅色模式切换 =====
const THEME_KEY = 'isTheme';

function applyTheme(dark) {
    document.body.classList.toggle('dark-mode', dark);
    if (themeIcon) themeIcon.className = 'fas ' + (dark ? 'fa-sun' : 'fa-moon');
    if (themeLabel) themeLabel.textContent = dark ? '浅色' : '深色';
}
if (themeToggle) {
    themeToggle.addEventListener('click', () => {
        const dark = !document.body.classList.contains('dark-mode');
        applyTheme(dark);
        try { localStorage.setItem(THEME_KEY, dark ? 'dark' : 'light'); } catch (e) { }
    });
}
applyTheme(localStorage.getItem(THEME_KEY) === 'dark');

// ===== Toast =====

function showToast(text, kind) {
    const toast = document.getElementById('toast');
    if (!toast) return;
    toast.textContent = text;
    toast.className = 'toast visible' + (kind ? ' ' + kind : '');
    clearTimeout(showToast._timer);
    showToast._timer = setTimeout(() => { toast.className = 'toast'; }, 2600);
}

// ===== 多媒体预览浮层（multimedia_preview 组件） =====

/**
 * 打开多媒体预览浮层（全局 previewImage，由 multimedia_preview/script.js 提供）
 * 组件不可用或调用异常时回退为新窗口打开，保证图片始终可查看
 * @param {string} url - 图片地址（/file/read/... 形式）
 * @param {string} name - 展示的文件名
 */
function openPreview(url, name) {
    const fallback = () => { try { window.open(url, '_blank'); } catch (e) { /* 忽略 */ } };
    try {
        if (typeof previewImage === 'function') {
            Promise.resolve(previewImage(url, name)).catch((e) => {
                console.warn('预览浮层打开失败，回退新窗口:', e);
                fallback();
            });
        } else {
            console.warn('预览组件未加载（previewImage 未定义），回退新窗口');
            fallback();
        }
    } catch (e) {
        console.warn('预览浮层调用异常，回退新窗口:', e);
        fallback();
    }
}

// ===== 生成完成提示音 =====

/** 生成完成提示音（相对 local_data 的 audios 目录，经 /file/read 访问） */
const PROMPT_TONE_URL = '/file/read/audios/prompt-tone.mp3';

/** 播放生成完成提示音（自动播放被浏览器策略拦截时静默忽略） */
function playPromptTone() {
    try {
        const audio = new Audio(PROMPT_TONE_URL);
        audio.volume = 0.6;
        const playing = audio.play();
        if (playing && typeof playing.catch === 'function') {
            playing.catch((e) => console.warn('提示音播放被拦截:', e && e.message));
        }
    } catch (e) {
        console.warn('提示音播放失败:', e);
    }
}

// ===== 文件接口（参考图像上传） =====

/** 文件名编码（与琉璃文件接口约定一致：encodeURIComponent 后按字节 base64） */
function encodeFileName(filename) {
    const encodedParams = encodeURIComponent(filename);
    const decodedParams = encodedParams.replace(/%([0-9A-F]{2})/g, (_, p1) => String.fromCharCode(parseInt(p1, 16)));
    return btoa(decodedParams);
}

/** 写入二进制文件到 local_data（POST /file/write，覆盖写） */
function writeFileToServer(path, blob) {
    return new Promise((resolve, reject) => {
        const xhr = new XMLHttpRequest();
        xhr.addEventListener('load', () => {
            if (xhr.status === 200) resolve();
            else reject(new Error(xhr.responseText || '写入失败（HTTP ' + xhr.status + '）'));
        });
        xhr.addEventListener('error', () => reject(new Error('写入失败')));
        xhr.addEventListener('timeout', () => reject(new Error('写入超时')));
        xhr.open('POST', '/file/write');
        xhr.setRequestHeader('X-File-Name', encodeFileName(path));
        xhr.setRequestHeader('X-Overwrite', 'true');
        xhr.send(blob);
    });
}

/** 列出目录文件（GET /file/list/<path>，与 file-explorer 用法一致） */
async function listServerFiles(path) {
    const resp = await fetch('/file/list/' + path, { cache: 'no-store' });
    if (!resp.ok) throw new Error('读取目录失败：' + path + '（HTTP ' + resp.status + '）');
    const files = await resp.json();
    return Array.isArray(files) ? files : [];
}

// ===== 生成画廊 =====

/**
 * 加载生成画廊（multimedia/generated，按名称倒序 = 最新在前）
 * @param {boolean} highlightNew - 高亮本次新生成的图片
 */
async function loadGallery(highlightNew) {
    try {
        const files = await listServerFiles(GENERATED_DIR);
        const list = files
            .filter(f => !f.isDir && IMAGE_EXT_RE.test(String(f.name)))
            .map(f => String(f.name))
            .sort()
            .reverse()
            .slice(0, GALLERY_MAX);

        // 已展示的文件名集合（用于识别新增高亮）
        const known = new Set(Array.from(galleryGrid.querySelectorAll('.gallery-item')).map(el => el.dataset.name));
        galleryGrid.innerHTML = '';
        if (list.length === 0) {
            galleryGrid.innerHTML = '<div class="list-empty" id="galleryEmpty">暂无生成结果。</div>';
            galleryStats.textContent = '';
            return;
        }
        let newCount = 0;
        for (const name of list) {
            const isNew = !!highlightNew && !known.has(name);
            if (isNew) newCount++;
            const item = document.createElement('div');
            item.className = 'gallery-item' + (isNew ? ' is-new' : '');
            item.dataset.name = name;
            const url = '/file/read/' + GENERATED_DIR + '/' + encodeURIComponent(name);
            item.innerHTML = '<div class="gallery-thumb"><img loading="lazy" alt=""></div>'
                + '<div class="gallery-meta"><span class="gallery-name"></span><span class="gallery-size"></span></div>';
            item.querySelector('img').src = url;
            item.querySelector('.gallery-name').textContent = name;
            // 点击经全局多媒体预览浮层查看大图
            item.addEventListener('click', () => openPreview(url, name));
            galleryGrid.appendChild(item);
        }
        galleryStats.textContent = '共 ' + list.length + ' 张' + (newCount > 0 ? ' · 新增 ' + newCount + ' 张' : '');
    } catch (e) {
        galleryGrid.innerHTML = '<div class="list-empty">画廊加载失败：' + escapeText(e.message) + '</div>';
        galleryStats.textContent = '';
    }
}

function escapeText(text) {
    const div = document.createElement('div');
    div.textContent = String(text);
    return div.innerHTML;
}

refreshGalleryBtn.addEventListener('click', () => { loadGallery(false); showToast('画廊已刷新', 'success'); });

// ===== 参考图像（图生图） =====

/** 当前参考图像的相对路径（相对 local_data）；空表示文生图 */
let refImagePath = '';

/** 更新参考图像 UI 状态 */
function updateRefUI(hasRef) {
    refPreviewWrap.classList.toggle('hidden', !hasRef);
    clearRefBtn.classList.toggle('hidden', !hasRef);
    refHint.textContent = hasRef
        ? '已选择参考图像（图生图模式），噪声强度生效。'
        : '未选择参考图像（文生图模式）。选择后自动切换为图生图并启用噪声强度。';
}

/** 清除参考图像 */
function clearRefImage() {
    refImagePath = '';
    refPreviewImg.src = '';
    updateRefUI(false);
}

clearRefBtn.addEventListener('click', clearRefImage);

/** 选择参考图像：本地预览 + 上传到 multimedia/reference */
pickRefBtn.addEventListener('click', () => {
    refFileInput.value = '';
    refFileInput.onchange = async () => {
        const file = refFileInput.files && refFileInput.files[0];
        if (!file) return;
        pickRefBtn.disabled = true;
        try {
            // 本地预览
            const dataUrl = await new Promise((resolve, reject) => {
                const reader = new FileReader();
                reader.onload = () => resolve(String(reader.result || ''));
                reader.onerror = () => reject(new Error('读取图片失败：' + file.name));
                reader.readAsDataURL(file);
            });
            // 上传到参考目录（后端按相对 local_data 路径读取 init_img）
            const timestamp = new Date().toISOString().replace(/[-:T]/g, '').slice(0, 14);
            const safeName = file.name.replace(/[^\w.\-\u4e00-\u9fa5]/g, '_');
            const fileName = timestamp + '_' + safeName;
            const blob = new Blob([file], { type: file.type || 'image/png' });
            await writeFileToServer(REFERENCE_DIR + '/' + fileName, blob);
            refImagePath = REFERENCE_DIR + '/' + fileName;
            refPreviewImg.src = dataUrl;
            updateRefUI(true);
            showToast('参考图像已上传：' + fileName, 'success');
        } catch (e) {
            showToast(e.message, 'error');
        } finally {
            pickRefBtn.disabled = false;
        }
    };
    refFileInput.click();
});

// ===== 参数收集与校验 =====

/** 取 32 的倍数（限制 256~2816）。依据：Qwen-Image 2.1 为 16× VAE 压缩 + 2×2 patch 的 DiT，
 *  官方要求宽高可被 32 整除；官方 2K 比例（如 3:2 的 2528×1696）是 32 的倍数而不是 64 的倍数。 */
function snap32(value, fallback) {
    let v = parseInt(value, 10);
    if (!isFinite(v)) v = fallback;
    v = Math.max(256, Math.min(2816, v));
    return Math.round(v / 32) * 32;
}

/** 收集并校验生成参数（非法时抛出中文错误） */
function collectParams() {
    const prompt = promptInput.value.trim();
    if (!prompt) throw new Error('正面提示词不能为空');
    const negative = negativeInput.value.trim();
    const steps = Math.max(1, Math.min(100, parseInt(stepsInput.value, 10) || 40));
    const batch = Math.max(1, Math.min(8, parseInt(batchInput.value, 10) || 1));
    const cfg = Math.max(1, Math.min(30, parseFloat(cfgInput.value) || 7));
    let strength = parseFloat(strengthInput.value);
    if (!isFinite(strength)) strength = 0.75;
    strength = Math.max(0.05, Math.min(1, strength));
    const seedRaw = String(seedInput.value).trim();
    const seed = seedRaw === '' ? 0 : (parseInt(seedRaw, 10) || 0);
    return {
        prompt: prompt,
        negative_prompt: negative,
        batch_size: batch,
        width: snap32(widthInput.value, 1024),
        height: snap32(heightInput.value, 1024),
        steps: steps,
        seed: seed,
        cfg_scale: cfg,
        strength: strength,
        init_img: refImagePath,
        allow_super_resolution: !!superResInput.checked
    };
}

// ===== 生成流程 =====

/** 更新生成状态条（kind: info/running/success/error） */
function setGenStatus(kind, text) {
    if (!genStatus) return;
    genStatus.className = 'gen-status' + (kind ? ' ' + kind : '');
    const icons = { info: 'fa-circle-info', running: 'fa-spinner', success: 'fa-circle-check', error: 'fa-circle-xmark' };
    genStatus.innerHTML = '<i class="fas ' + (icons[kind] || icons.info) + '"></i><span></span>';
    genStatus.querySelector('span').textContent = text;
}

/** 生成结果最长等待（毫秒，后端 SSE 上限 5 分钟，前端放宽） */
const WAIT_MAX_MS = 360000;
/** 等待轮询间隔（毫秒） */
const WAIT_STEP_MS = 2000;

/**
 * 等待生成任务完成（GET /generate/wait?task_id=）
 * 兼容两种响应：任务已结束的裸 JSON、等待中的 SSE「data: {...}」帧
 * @param {string} taskId - 任务 ID
 * @returns {Promise<Object>} { task_id, status, result, error, read_path }
 */
async function waitTask(taskId) {
    const started = Date.now();
    while (Date.now() - started < WAIT_MAX_MS) {
        const resp = await fetch('/generate/wait?task_id=' + encodeURIComponent(taskId), { cache: 'no-store' });
        if (!resp.ok) {
            let msg = '等待结果失败（HTTP ' + resp.status + '）';
            try { msg = (await resp.text()) || msg; } catch (e) { }
            throw new Error(msg);
        }
        const text = (await resp.text()).trim();
        let payload = null;
        if (text.startsWith('{')) {
            // 裸 JSON（任务已结束直接返回）
            payload = JSON.parse(text);
        } else {
            // SSE 帧：取最后一个 data: 行
            const matches = text.match(/data:\s*(\{[\s\S]*?\})/g);
            if (!matches || matches.length === 0) throw new Error('等待结果响应格式异常');
            payload = JSON.parse(matches[matches.length - 1].replace(/^data:\s*/, ''));
        }
        if (payload && (payload.status === 'completed' || payload.status === 'failed')) {
            return payload;
        }
        // 仍在生成：间隔后重试
        await new Promise(resolve => setTimeout(resolve, WAIT_STEP_MS));
    }
    throw new Error('等待生成结果超时（超过 6 分钟）');
}

/** 生成图像主流程 */
async function handleGenerate() {
    let params;
    try {
        params = collectParams();
    } catch (e) {
        showToast(e.message, 'error');
        return;
    }
    generateBtn.disabled = true;
    try {
        setGenStatus('running', '任务提交中…');
        // 提交任务（琉璃将 /generate 代理至月华 36789）
        const resp = await fetch('/generate', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(params)
        });
        if (!resp.ok) {
            let msg = '任务提交失败（HTTP ' + resp.status + '）';
            try { msg = (await resp.text()) || msg; } catch (e) { }
            throw new Error(msg);
        }
        const queued = await resp.json();
        if (!queued.task_id) throw new Error(queued.message || '任务提交响应异常');
        setGenStatus('running', '生成中…（队列位置 ' + (queued.queue_pos !== undefined ? queued.queue_pos : '-')
            + '，' + params.width + '×' + params.height + '，' + params.steps + ' 步'
            + (params.batch_size > 1 ? '，共 ' + params.batch_size + ' 张' : '')
            + (params.allow_super_resolution ? '，含超分' : '') + '）');
        // 等待完成
        const result = await waitTask(queued.task_id);
        if (result.status !== 'completed') {
            throw new Error(result.error || ('任务状态异常：' + result.status));
        }
        const newName = String(result.read_path || result.result || '').split('/').pop();
        setGenStatus('success', '生成完成：' + (newName || result.read_path || ''));
        // 播放生成完成提示音
        playPromptTone();
        showToast('图像生成完成' + (newName ? '：' + newName : ''), 'success');
        // 刷新画廊并高亮新图
        await loadGallery(true);
        // 自动用多媒体预览浮层展示刚生成的图片
        const newUrl = String(result.read_path || '') ||
            ('/file/read/' + GENERATED_DIR + '/' + encodeURIComponent(newName));
        if (newName) openPreview(newUrl, newName);
    } catch (e) {
        setGenStatus('error', '生成失败：' + e.message);
        showToast(e.message, 'error');
    } finally {
        generateBtn.disabled = false;
    }
}

generateBtn.addEventListener('click', handleGenerate);

// ===== 初始化 =====
loadGallery(false);
