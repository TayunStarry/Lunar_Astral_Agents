// ==== engine.js — GIF / WebM 动态图引擎 · 引擎入口 ====
//
// 与月岩（lunar.rendering_engine）/ Live2D（lunar.live2d_engine）同构：
//   - 通信：同一条 /ws 连接，频道名由 ?channel= 指定；收发形状完全一致
//   - 指令：panel_ready 回 engine_ready + animation_list；action 播放同名动画；
//           query_molang 回空值；movement/rotation 等 3D 语义指令安全忽略
//   - 资源集：默认解析 WebM（/file/read/models/webm/，VP9 alpha，
//           目录联接 local_data/models/webm → assets/webm）；
//           Safari/iOS 等不支持 WebM alpha 的环境，或 WebM 连续播放失败时，
//           回退 GIF（/file/read/models/gif/，目录联接 local_data/models/gif → assets/preview）
//   - 动画名 = 文件名去扩展名；?video= / ?assets= 可覆盖两套基址
//   - 播放：WebM 由 <video> 的 ended 事件驱动，GIF 按帧延迟求和；
//           播完随机停 1–3 秒再随机下一张，不连续重复
//   - 匹配：context 文本交给 LunarActionMatcher（/agent/embeddings），
//           命中就播对应动画，未命中保持随机序列继续
//   - 生命周期：页面卸载时停排程、清定时器、释放视频解码器与 Blob URL

import { canPlayAlphaWebm, loadSource, resolveBases } from './core/source-loader.js';
import { GifViewController } from './core/view-control.js';

// ==== URL 参数 ====
const urlParams = new URLSearchParams(window.location.search);
const CHANNEL_NAME = urlParams.get('channel') || 'gif-engine';
const SOURCE_ENGINE = 'engine';
/** 两套资源基址：WebM 首选，GIF 回退（保留旧资源，不物理删除） */
const BASES = resolveBases(urlParams);

// ==== 统一通信与动作匹配（与另两套引擎同款） ====
const channel = new WsBridge(CHANNEL_NAME);
const actionMatcher = window.LunarActionMatcher
    ? new window.LunarActionMatcher({ endpoint: '/agent/embeddings' })
    : null;

// ==== 状态 ====
/** 动画清单：[{ name, file, ext, size }] */
let library = [];
/** 动画名列表（匹配与播放都用它） */
let names = [];
/** 当前解析的资源集：'webm' | 'gif' */
let mode = '';
let cache = null;
/** GifScheduler 或 VideoScheduler（两者 start/stop/play 接口一致） */
let scheduler = null;
/** 显示层视图变换（拖拽 / 缩放 / 复位），与 Live2D 引擎同一套交互 */
let viewControl = null;

// ============================================================
//  启动
// ============================================================
async function init() {
    try {
        const preferred = canPlayAlphaWebm() ? 'video' : 'image';
        try {
            await applySource(preferred);
        } catch (e) {
            if (preferred === 'image') throw e;
            console.warn('[GIF] WebM 不可用，改用 GIF 资源:', e);
            await applySource('image');
        }
        viewControl = new GifViewController({
            stage: document.getElementById('gif-stage'),
            layer: document.getElementById('gif-layer'),
            image: document.getElementById('gif-frame'),
            video: document.getElementById('gif-video'),
        });
        viewControl.bind();
        viewControl.reset();
        setMask(null);
        broadcast('engine_ready', { mode: 'embedded' });
        broadcastAnimationList();
        console.log(`[GIF] 已载入 ${names.length} 个动画（${mode}）`);
    } catch (err) {
        console.error('[GIF] 初始化失败:', err);
        setMask(`动画库加载失败：${(err && err.message) || err}`);
        broadcast('engine_error', { message: String((err && err.message) || err) });
    }
}

/** 装配指定资源集并同步到引擎状态 */
async function applySource(type) {
    const source = await loadSource({
        type,
        bases: BASES,
        video: document.getElementById('gif-video'),
        image: document.getElementById('gif-frame'),
        capacity: pickCapacity(),
        onFatal: err => fallbackToImage(err),
        onImageError: err => setMask(`动画加载失败：${err.message || err}`),
    });
    mode = source.mode;
    library = source.library;
    names = source.names;
    cache = source.cache;
    scheduler = source.scheduler;
}

/** WebM 连续失败时整体回退 GIF 资源集（只回退一次） */
async function fallbackToImage(err) {
    if (mode === 'gif') return;
    console.warn('[GIF] WebM 播放失败，回退 GIF 资源:', err);
    if (scheduler) scheduler.stop();
    scheduler = null;
    try {
        await applySource('image');
        broadcastAnimationList();
        setMask(null);
    } catch (e) {
        setMask(`动画库加载失败：${(e && e.message) || e}`);
    }
}

/** 缓存容量：低内存 / 少核心设备降档，压缩字节缓存桌面 6 张、移动 3 张 */
function pickCapacity() {
    const memory = Number(navigator.deviceMemory) || 0;
    const cores = Number(navigator.hardwareConcurrency) || 0;
    return (memory > 0 && memory <= 4) || (cores > 0 && cores <= 4) ? 3 : 6;
}

// ============================================================
//  动作匹配 / 指令
// ============================================================

/**
 * 后端推送的对话消息：按 50% 概率用嵌入模型在动画名里挑最匹配的一张。
 * 与另两套引擎一致：后端只推消息，匹配与播放完全由引擎侧完成。
 */
function reactToPushedMessage(data) {
    if (!actionMatcher || !scheduler || !data || data.type !== 'text') return;
    const text = String(data.content || '').trim();
    if (!text || names.length === 0) return;
    actionMatcher.consider(text, names, name => scheduler.play(name));
}

/** 指令名 → 动画名（容忍带扩展名与大小写差异），命中即播放 */
function playByName(raw) {
    if (!scheduler) return false;
    const key = String(raw || '').trim();
    if (!key) return false;
    const bare = key.replace(/\.(webm|gif|apng|webp|png)$/i, '');
    const name = names.find(item => item === key)
        || names.find(item => item === bare)
        || names.find(item => item.toLowerCase() === bare.toLowerCase());
    if (!name) return false;
    scheduler.play(name);
    return true;
}

/** 引擎指令分发（仅处理有意义的类型，其余安全忽略） */
function handleChannelMessage(msg) {
    const { type, payload } = msg || {};
    switch (type) {
        case 'panel_ready':
            // 面板就绪握手：晚订阅的面板/智能体可重新请求状态与动画列表
            broadcast('engine_ready', { mode: 'embedded' });
            broadcastAnimationList();
            break;

        case 'action': {
            const name = payload?.action || payload?.emotion || payload?.state;
            if (name) playByName(name);
            break;
        }

        case 'query_molang':
            broadcast('molang_value', { expression: null, value: null });
            break;

        case 'mode_changed':
            // 无编辑器模式，仅记录不处理
            break;

        default:
            // movement/rotation/quick_track/physics_*/primitive_* 等 3D 指令：无对应表现
            break;
    }
}

// ============================================================
//  统一总线：入站 / 出站
// ============================================================
channel.onmessage = (event) => {
    const raw = event.data;
    if (!raw || raw.source === SOURCE_ENGINE) return; // 忽略自己发的
    // 智能体引擎命令包装：{type:"engine", data:{type,payload,source}} → 解包
    let msg = raw;
    if (raw.type === 'engine' && raw.data && raw.data.type) {
        msg = { type: raw.data.type, source: raw.data.source, payload: raw.data.payload };
    } else if (raw.type === 'context') {
        reactToPushedMessage(raw.data); // 展示类消息：引擎自行匹配动作，不执行指令
        return;
    }
    handleChannelMessage(msg);
};

/** 上报动画列表（形状与另两套引擎的 animation_list 一致） */
function broadcastAnimationList() {
    broadcast('animation_list', {
        groups: [],
        allAnimations: names.slice(),
        bones: [],
        actionDefinitions: names.map(name => ({ name, mouseTracking: false })),
    });
}

/** 总线出站（形状与另两套引擎完全一致） */
function broadcast(type, payload) {
    channel.postMessage({
        type,
        source: SOURCE_ENGINE,
        payload,
        timestamp: Date.now(),
    });
}

// ============================================================
//  UI 辅助
// ============================================================

/** 加载遮罩：text 为 null 时淡出收起，否则显示并替换提示文案 */
function setMask(text) {
    const mask = document.getElementById('loading-mask');
    if (!mask) return;
    if (text === null) {
        mask.classList.add('hidden');
        setTimeout(() => { mask.style.display = 'none'; }, 400);
        return;
    }
    mask.classList.remove('hidden');
    mask.style.display = 'flex';
    const label = mask.querySelector('.mask-text');
    if (label) label.textContent = text;
}

/** 页面卸载：停排程、清缓存、断开总线，不残留定时器与解码资源 */
function teardown() {
    if (scheduler) scheduler.stop();
    if (cache) cache.dispose();
    if (viewControl) {
        viewControl.destroy();
        viewControl = null;
    }
    const image = document.getElementById('gif-frame');
    if (image) image.removeAttribute('src');
    channel.close();
}

// ==== 对外暴露（调试 / 宿主可选调用） ====
window.GifEngine = {
    init,
    play: playByName,
    next: () => {
        if (!scheduler || names.length === 0) return;
        scheduler.play(names[Math.floor(Math.random() * names.length)]);
    },
    getLibrary: () => library.slice(),
    isReady: () => !!scheduler,
    /** 当前解析的资源集：'webm' | 'gif' */
    getMode: () => mode,
    getNames: () => names.slice(),
    // 视图：与 Live2D 引擎同名的复位 / 缩放 / 状态读取入口
    resetView: () => viewControl?.reset(),
    zoomBy: (factor) => {
        const stage = document.getElementById('gif-stage');
        if (!stage || !viewControl) return;
        viewControl.zoomAt(factor, stage.clientWidth / 2, stage.clientHeight / 2);
    },
    getView: () => (viewControl ? viewControl.getView() : { zoom: 1, panX: 0, panY: 0 }),
    stop: teardown,
};

// ==== 启动 ====
window.addEventListener('pagehide', teardown);
init();
