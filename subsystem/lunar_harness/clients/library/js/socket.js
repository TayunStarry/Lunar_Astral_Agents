// socket.js 资源库唯一主通讯通道。
//
// 一条 /ws?channel=library 连接同时承载：
//   - 上行命令：{"type":"command","id":…,"action":…,"data":…}
//   - 定向下行回执：{"type":"library_reply","data":{"id":…,"ok":…,"result":…}}
//   - 频道推送：storyboard_task / creation_task / library_sync
//
// 所有推送处理器都在本模块内单独 try/catch：单条异常数据不会打断后续消息，
// 也不会把异常抛进 WebSocket 事件回调之外。
//
// 轮询已彻底移除：请求失败由回执的 ok/error 或超时明确告知，不再静默。

const RECONNECT_BASE_MS = 1000;
const RECONNECT_MAX_MS = 15000;
const OPEN_TIMEOUT_MS = 10000;
const REPLY_TIMEOUT_MS = 20000;
const HEARTBEAT_INTERVAL_MS = 20000;
const HEARTBEAT_TIMEOUT_MS = 8000;
// 推送分片处理：每个宏任务最多占用主线程这么久，处理不完立即让出，
// 保证高频推送不会把一次事件循环拖成「长任务」（渲染掉帧、页面无响应）。
const PUSH_SLICE_BUDGET_MS = 6;

let socket = null;
let opened = false;
let retryDelay = RECONNECT_BASE_MS;
let reconnectTimer = null;
let heartbeatTimer = null;
let nextCommandId = 0;

// pending 命令回执表：id -> { resolve, reject, timer, action }
const pending = new Map();
// pushHandlers 推送类型 -> 处理器集合
const pushHandlers = new Map();
// statusHandlers 连接状态监听
const statusHandlers = new Set();
// openWaiters 等待连接建立的命令
let openWaiters = [];
// pushQueue 待分发的频道推送：按到达顺序排队，分批消费
const pushQueue = [];
let pushDrainTimer = null;

// isSocketOpen 当前连接是否可发命令。
export function isSocketOpen() {
    return opened;
}

// onSocketStatus 订阅连接状态变化；fn(state, message)，state 为 open / closed。
export function onSocketStatus(fn) {
    if (typeof fn !== 'function') return () => {};
    statusHandlers.add(fn);
    return () => statusHandlers.delete(fn);
}

// onPush 订阅某类频道推送；返回取消订阅函数。
export function onPush(type, fn) {
    if (typeof fn !== 'function') return () => {};
    let handlers = pushHandlers.get(type);
    if (!handlers) {
        handlers = new Set();
        pushHandlers.set(type, handlers);
    }
    handlers.add(fn);
    return () => handlers.delete(fn);
}

// connectSocket 启动连接；重复调用不会叠加第二条连接。
export function connectSocket() {
    if (socket && (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING)) {
        return;
    }
    clearReconnect();
    const scheme = location.protocol === 'https:' ? 'wss' : 'ws';
    let current;
    try {
        current = new WebSocket(`${scheme}://${location.host}/ws?channel=library`);
    } catch (err) {
        emitStatus('closed', `无法建立实时通道：${err.message || err}`);
        scheduleReconnect();
        return;
    }
    socket = current;

    current.addEventListener('open', () => {
        if (socket !== current) return;
        opened = true;
        retryDelay = RECONNECT_BASE_MS;
        startHeartbeat();
        flushOpenWaiters();
        emitStatus('open', '');
    });

    current.addEventListener('message', onMessage);

    current.addEventListener('close', () => {
        if (socket !== current) return;
        const wasOpen = opened;
        opened = false;
        stopHeartbeat();
        // 未送达的命令立刻失败，让界面能提示而不是永远等下去。
        failAllPending('实时连接已断开，命令未送达；正在重连，请稍后重试');
        emitStatus('closed', wasOpen ? '实时连接已断开，正在重连…' : '实时通道连接失败，正在重试…');
        scheduleReconnect();
    });

    // error 之后浏览器必定再派发 close，这里只兜底提示。
    current.addEventListener('error', () => {
        if (socket === current && opened) {
            emitStatus('closed', '实时通道出现错误，正在重连…');
        }
    });
}

// socketRequest 发送命令并等待对应回执；超时 / 断线 / 后端错误都会 reject。
export async function socketRequest(action, data = {}, options = {}) {
    const timeout = Number(options.timeout) > 0 ? Number(options.timeout) : REPLY_TIMEOUT_MS;
    if (!opened) {
        await waitOpen(Math.min(timeout, OPEN_TIMEOUT_MS));
    }
    if (!opened || !socket) {
        throw new Error(`实时通道尚未连接，无法执行 ${action}`);
    }
    nextCommandId += 1;
    const id = `${Date.now().toString(36)}-${nextCommandId.toString(36)}`;

    return new Promise((resolve, reject) => {
        const timer = setTimeout(() => {
            pending.delete(id);
            reject(new Error(`命令超时（${action}），请检查实时连接后重试`));
        }, timeout);
        pending.set(id, { resolve, reject, timer, action });
        try {
            socket.send(JSON.stringify({ type: 'command', id, action, data }));
        } catch (err) {
            clearTimeout(timer);
            pending.delete(id);
            reject(new Error(`命令发送失败：${err.message || err}`));
        }
    });
}

// ==== 内部：连接与重连 ====

function scheduleReconnect() {
    if (reconnectTimer) return;
    const delay = retryDelay;
    retryDelay = Math.min(RECONNECT_MAX_MS, retryDelay * 2);
    reconnectTimer = setTimeout(() => {
        reconnectTimer = null;
        connectSocket();
    }, delay);
}

function clearReconnect() {
    if (reconnectTimer) clearTimeout(reconnectTimer);
    reconnectTimer = null;
}

function startHeartbeat() {
    stopHeartbeat();
    heartbeatTimer = setInterval(() => {
        // 半开连接不会触发 close：用一次 ping 命令探测，失败即主动断开重连。
        void socketRequest('ping', {}, { timeout: HEARTBEAT_TIMEOUT_MS }).catch(() => {
            if (socket) socket.close();
        });
    }, HEARTBEAT_INTERVAL_MS);
}

function stopHeartbeat() {
    if (heartbeatTimer) clearInterval(heartbeatTimer);
    heartbeatTimer = null;
}

function waitOpen(timeoutMs) {
    return new Promise((resolve, reject) => {
        const waiter = { resolve, reject, timer: null };
        waiter.timer = setTimeout(() => {
            openWaiters = openWaiters.filter((item) => item !== waiter);
            reject(new Error('实时通道连接超时，请稍后重试'));
        }, timeoutMs);
        openWaiters.push(waiter);
    });
}

function flushOpenWaiters() {
    const waiters = openWaiters;
    openWaiters = [];
    waiters.forEach((waiter) => {
        clearTimeout(waiter.timer);
        waiter.resolve();
    });
}

function failAllPending(message) {
    const entries = Array.from(pending.values());
    pending.clear();
    entries.forEach((entry) => {
        clearTimeout(entry.timer);
        entry.reject(new Error(message));
    });
}

// ==== 内部：消息分发 ====

function onMessage(event) {
    let payload = null;
    try {
        payload = JSON.parse(event.data);
    } catch {
        return;
    }
    if (!payload || !payload.type) return;

    if (payload.type === 'library_reply') {
        // 回执必须立刻结算：命令的 Promise 在等它，排队会让超时判定失真。
        settleReply(payload.data);
        return;
    }

    if (!pushHandlers.has(payload.type)) return;
    // 推送只入队，不在事件回调里直接渲染：同一帧内到达的多条推送合并处理，
    // 单条推送处理再慢也不会把后续消息的事件循环堵死。
    pushQueue.push(payload);
    schedulePushDrain();
}

function schedulePushDrain() {
    if (pushDrainTimer !== null) return;
    // 用宏任务而不是 requestAnimationFrame：窗口最小化时 rAF 会停摆，
    // 那样推送会一直堆在队列里无法消费。
    pushDrainTimer = setTimeout(drainPushes, 0);
}

function drainPushes() {
    pushDrainTimer = null;
    const deadline = nowMs() + PUSH_SLICE_BUDGET_MS;
    while (pushQueue.length > 0) {
        const payload = pushQueue.shift();
        dispatchPush(payload);
        if (nowMs() >= deadline) break;
    }
    if (pushQueue.length > 0) schedulePushDrain();
}

function dispatchPush(payload) {
    const handlers = pushHandlers.get(payload.type);
    if (!handlers) return;
    handlers.forEach((fn) => {
        try {
            fn(payload.data);
        } catch (err) {
            // 单条推送渲染失败只影响这一条，后续推送照常处理。
            console.error(`[library] 推送处理失败：${payload.type}`, err);
        }
    });
}

function nowMs() {
    return typeof performance !== 'undefined' && typeof performance.now === 'function'
        ? performance.now()
        : Date.now();
}

function settleReply(data) {
    if (!data || typeof data !== 'object') return;
    const entry = pending.get(data.id);
    if (!entry) return;
    pending.delete(data.id);
    clearTimeout(entry.timer);
    if (data.ok === false) {
        entry.reject(new Error(data.error || `命令失败：${entry.action}`));
        return;
    }
    entry.resolve(data.result);
}

function emitStatus(state, message) {
    statusHandlers.forEach((fn) => {
        try {
            fn(state, message);
        } catch (err) {
            console.error('[library] 连接状态回调失败', err);
        }
    });
}

// 页面重新可见时立即探活：后台标签页的连接可能已被系统回收。
if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', () => {
        if (document.visibilityState === 'visible' && !opened) connectSocket();
    });
}
