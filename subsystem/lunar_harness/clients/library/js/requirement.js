// requirement.js 需求提交与创作任务进度跟踪。
// 接口地址、请求体、响应字段集中在这里定义；推送优先，轮询兜底。

// REQUIREMENT_API 创作链接口契约（见 server/library_creation.go）。
export const REQUIREMENT_API = {
    submit: '/library/api/create',
    status: '/library/api/create/status',
};

// 资源句柄文法与后端 engine/resource/variable.go 的 HandlePattern 保持一致：
// 类型码 D/I/V/A/G/T/U + 三位起序号，可选段落/行锚点。
const HANDLE_PATTERN = /^@[DIVAGTU]\.\d{3,}(?:\|(?:§\d+|L\d+-\d+))?$/;
// 轮询兜底间隔；推送新鲜期内跳过轮询。
const POLL_INTERVAL_MS = 4000;
const PUSH_FRESH_MS = 10000;
const TERMINAL_STATUS = new Set(['completed', 'failed']);

let observer = null;
let activeTask = null;
let pollTimer = null;
let pollBusy = false;
let pushConnected = false;
let lastPushAt = 0;

export class RequirementError extends Error {
    constructor(message, status = 0) {
        super(message);
        this.name = 'RequirementError';
        this.status = status;
        this.notFound = status === 404;
    }
}

// setRequirementObserver 接收任务更新或终态错误；UI 渲染留在 app.js。
export function setRequirementObserver(fn) {
    observer = typeof fn === 'function' ? fn : null;
}

// prepareRequirement 校验并规整输入：有句柄走 assigned，留空走 random。
export function prepareRequirement(handle, description) {
    const mode = String(handle || '').trim() ? 'assigned' : 'random';
    const cleanHandle = String(handle || '').trim();
    const cleanDescription = String(description || '').trim();
    if (mode === 'assigned' && !HANDLE_PATTERN.test(cleanHandle)) {
        throw new RequirementError('资源句柄格式应为 @D.020 这类形式');
    }
    if (mode === 'assigned' && !cleanDescription) {
        throw new RequirementError('指定资源句柄时请填写需求描述');
    }
    return { mode, handle: cleanHandle, description: cleanDescription };
}

// submitRequirement 提交任务并返回已受理的任务视图；成功后开始跟踪进度。
export async function submitRequirement(input) {
    const body = prepareRequirement(input.handle, input.description);
    stopWatching();
    const data = await requestJSON(REQUIREMENT_API.submit, {
        method: 'POST',
        body: JSON.stringify(buildRequestBody(body)),
    });
    if (data.ok === false || !data.task) {
        throw new RequirementError(data.error || '创作系统未返回任务信息');
    }
    const task = normalizeTask(data.task);
    if (!task.taskId) {
        throw new RequirementError('创作系统未返回任务标识');
    }
    activeTask = task;
    return task;
}

// watchRequirement 为已受理任务启动轮询看门狗（单个计时器，重复调用不会叠加）。
// 同 watchStoryboard：入参可能已经是规范化任务，重复 normalize 会把 taskId 清空，
// 导致推送被守卫丢弃、轮询用空 task_id 静默失败。
export function watchRequirement(task) {
    activeTask = task && task.taskId ? task : normalizeTask(task);
    lastPushAt = 0;
    schedulePoll(POLL_INTERVAL_MS);
}

// handleCreationEvent 处理 WebSocket 的 creation_task 推送；只认当前跟踪的任务。
export function handleCreationEvent(raw) {
    const task = normalizeTask(raw);
    if (!task.taskId) return;
    if (!activeTask || task.taskId !== activeTask.taskId) return;
    lastPushAt = Date.now();
    applyTask(task);
}

// setPushConnected 由 WebSocket 生命周期驱动；断线时立即用状态接口兜底一次。
export function setPushConnected(connected) {
    pushConnected = Boolean(connected);
    if (!pushConnected && activeTask && !TERMINAL_STATUS.has(activeTask.status)) {
        void pollNow();
    }
}

// stopWatching 停止当前任务的轮询与跟踪（页面卸载、重新提交时调用）。
export function stopWatching() {
    stopPolling();
    activeTask = null;
    lastPushAt = 0;
}

// ==== 内部：请求与字段对齐 ====

// buildRequestBody 只发后端声明的字段（DisallowUnknownFields）。
function buildRequestBody({ mode, handle, description }) {
    const body = { mode };
    if (description) body.description = description;
    if (handle) body.handle = handle;
    return body;
}

async function requestJSON(url, options = {}) {
    const headers = { Accept: 'application/json' };
    if (options.body) headers['Content-Type'] = 'application/json';
    let res;
    try {
        res = await fetch(url, { ...options, headers });
    } catch (err) {
        throw new RequirementError(`网络请求失败：${err.message || err}`);
    }
    const data = await res.json().catch(() => null);
    if (!res.ok) {
        throw new RequirementError((data && data.error) || `请求失败（HTTP ${res.status}）`, res.status);
    }
    if (!data) {
        throw new RequirementError('创作系统返回了无法解析的响应');
    }
    return data;
}

// normalizeTask 对齐 server.LibraryCreationTask 的实际 JSON 字段。
function normalizeTask(raw) {
    const source = raw && typeof raw === 'object' ? raw : {};
    return {
        taskId: firstText(source.task_id),
        mode: firstText(source.mode),
        status: firstText(source.status) || 'queued',
        stage: firstText(source.stage),
        progress: clampProgress(source.progress),
        summary: firstText(source.summary),
        handles: Array.isArray(source.handles)
            ? source.handles.filter((item) => typeof item === 'string' && item.trim()).map((item) => item.trim())
            : [],
        error: firstText(source.error),
        createdAt: firstText(source.created_at),
        updatedAt: firstText(source.updated_at),
    };
}

function firstText(...values) {
    for (const value of values) {
        if (typeof value === 'string' && value.trim()) return value.trim();
    }
    return '';
}

function clampProgress(value) {
    const number = Number(value);
    if (!Number.isFinite(number) || number <= 0) return 0;
    return Math.min(100, Math.round(number));
}

// ==== 内部：轮询与通知 ====

function schedulePoll(delay) {
    if (pollTimer) return;
    pollTimer = setTimeout(() => {
        pollTimer = null;
        void tickPoll();
    }, delay);
}

function stopPolling() {
    if (pollTimer) clearTimeout(pollTimer);
    pollTimer = null;
}

async function tickPoll() {
    const task = activeTask;
    if (!task || TERMINAL_STATUS.has(task.status)) return;
    // 推送仍新鲜时跳过本次轮询，避免与 WebSocket 重复查询。
    if (pushConnected && Date.now() - lastPushAt < PUSH_FRESH_MS) {
        schedulePoll(POLL_INTERVAL_MS);
        return;
    }
    await pollNow();
}

async function pollNow() {
    if (pollBusy) return;
    const task = activeTask;
    if (!task || TERMINAL_STATUS.has(task.status)) return;
    pollBusy = true;
    try {
        const data = await requestJSON(`${REQUIREMENT_API.status}?task_id=${encodeURIComponent(task.taskId)}`);
        if (data.ok === false || !data.task) {
            throw new RequirementError(data.error || '任务状态查询失败');
        }
        const fresh = normalizeTask(data.task);
        if (!activeTask || fresh.taskId !== activeTask.taskId) return;
        applyTask(fresh);
    } catch (err) {
        // 任务不存在/已过期属于终态；网络抖动则静默下轮重试。
        if (activeTask && activeTask.taskId === task.taskId && err.notFound) {
            const failed = { ...task, status: 'failed', error: err.message };
            activeTask = failed;
            stopPolling();
            notify({ kind: 'task', task: failed });
        }
    } finally {
        pollBusy = false;
        // 单点驱动下一轮：终态或已切换任务时不再续约。
        if (activeTask && !TERMINAL_STATUS.has(activeTask.status)) {
            schedulePoll(POLL_INTERVAL_MS);
        }
    }
}

function applyTask(task) {
    activeTask = task;
    notify({ kind: 'task', task });
    // 终态不再轮询，但保留 activeTask 供界面继续展示结果。
    if (TERMINAL_STATUS.has(task.status)) stopPolling();
}

function notify(payload) {
    if (observer) observer(payload);
}

// 页面卸载时停止计时器，避免离开后仍在轮询。
if (typeof window !== 'undefined') {
    window.addEventListener('pagehide', stopWatching);
}
