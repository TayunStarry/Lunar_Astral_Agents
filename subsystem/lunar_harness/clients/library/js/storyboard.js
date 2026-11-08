// storyboard.js 演绎链提交与任务进度跟踪。
// 与创作链相互独立：接口、状态机、轮询均不复用创作链的跟踪状态。

// STORYBOARD_API 演绎链接口契约（见 server/library_storyboard.go）。
export const STORYBOARD_API = {
    submit: '/library/api/storyboard',
    status: '/library/api/storyboard/status',
    confirm: '/library/api/storyboard/confirm',
    pause: '/library/api/storyboard/pause',
    resume: '/library/api/storyboard/resume',
    cancel: '/library/api/storyboard/cancel',
};

// 资源句柄文法与后端 engine/resource/variable.go 的 HandlePattern 保持一致。
const HANDLE_PATTERN = /^@[DIVAGTU]\.\d{3,}(?:\|(?:§\d+|L\d+-\d+))?$/;
const POLL_INTERVAL_MS = 4000;
const PUSH_FRESH_MS = 10000;
// MAX_STORY_CHARS 与后端 maxStoryRunes 保持一致：超限直接拒绝，不截断。
const MAX_STORY_CHARS = 4000;
const TERMINAL_STATUS = new Set(['completed', 'failed', 'canceled']);

let observer = null;
let activeTask = null;
let pollTimer = null;
let pollBusy = false;
let pushConnected = false;
let lastPushAt = 0;

export class StoryboardError extends Error {
    constructor(message, status = 0) {
        super(message);
        this.name = 'StoryboardError';
        this.status = status;
        this.notFound = status === 404;
    }
}

// setStoryboardObserver 接收任务更新；UI 渲染留在 app.js。
export function setStoryboardObserver(fn) {
    observer = typeof fn === 'function' ? fn : null;
}

// prepareStoryboard 校验并规整输入：正文与句柄至少有一个。
export function prepareStoryboard(input = {}) {
    const text = String(input.text || '').trim();
    const handle = String(input.handle || '').trim();
    const title = String(input.title || '').trim();
    const style = String(input.style || '').trim();
    const ratio = String(input.ratio || '').trim();
    if (!text && !handle) {
        throw new StoryboardError('请填写故事正文，或带入一个故事资源句柄');
    }
    if (!text && handle && !HANDLE_PATTERN.test(handle)) {
        throw new StoryboardError('资源句柄格式应为 @D.020 这类形式');
    }
    // 通过句柄导入的正文由后端读取后再校验；这里先拦住明显超限的输入。
    if (text && [...text].length > MAX_STORY_CHARS) {
        throw new StoryboardError(`故事正文 ${[...text].length} 字，超过 ${MAX_STORY_CHARS} 字上限；请精简后再提交`);
    }
    const body = {};
    if (text) body.text = text;
    if (handle) body.handle = handle;
    if (title) body.title = title;
    if (style) body.style = style;
    if (ratio) body.ratio = ratio;
    return body;
}

// submitStoryboard 提交任务并返回已受理的任务视图。
export async function submitStoryboard(input) {
    const body = prepareStoryboard(input);
    stopStoryboardWatching();
    const data = await requestJSON(STORYBOARD_API.submit, {
        method: 'POST',
        body: JSON.stringify(body),
    });
    if (data.ok === false || !data.task) {
        throw new StoryboardError(data.error || '演绎系统未返回任务信息');
    }
    const task = normalizeTask(data.task);
    if (!task.taskId) {
        throw new StoryboardError('演绎系统未返回任务标识');
    }
    activeTask = task;
    return task;
}

// watchStoryboard 为已受理任务启动轮询看门狗。
//
// 入参可能是 submitStoryboard 返回的规范化任务（camelCase 字段），也可能是
// 原始推送（snake_case）。若对规范化任务再跑一遍 normalizeTask，taskId 会因
// 取不到 task_id 而变成空串，后续推送会被「只认当前任务」的守卫全部丢弃，
// 轮询也会用空 task_id 请求而静默失败——表现就是提交后再无任何更新。
export function watchStoryboard(task) {
    activeTask = task && task.taskId ? task : normalizeTask(task);
    lastPushAt = 0;
    schedulePoll(POLL_INTERVAL_MS);
}

// confirmStoryboard 提交审查后的完整分镜稿，后端确认后开始逐镜绘制。
export async function confirmStoryboard(taskId, shots, title = '') {
    const body = {
        task_id: taskId,
        shots: (Array.isArray(shots) ? shots : []).map((shot) => ({
            index: Number(shot.index) || 0,
            act: String(shot.act || '').trim(),
            scene: String(shot.scene || '').trim(),
            narration: String(shot.narration || '').trim(),
            prompt: String(shot.prompt || '').trim(),
            ratio: String(shot.ratio || '').trim(),
        })),
    };
    if (title) body.title = String(title).trim();
    return controlStoryboard(STORYBOARD_API.confirm, body);
}

// pauseStoryboard 暂停当前任务；已绘镜头保留，继续时从下一镜接着画。
export async function pauseStoryboard(taskId) {
    return controlStoryboard(STORYBOARD_API.pause, { task_id: taskId });
}

// resumeStoryboard 继续已暂停的任务。
export async function resumeStoryboard(taskId) {
    return controlStoryboard(STORYBOARD_API.resume, { task_id: taskId });
}

// cancelStoryboard 终止任务并保留已产出镜头。
export async function cancelStoryboard(taskId, reason = '') {
    const body = { task_id: taskId };
    if (reason) body.reason = reason;
    return controlStoryboard(STORYBOARD_API.cancel, body);
}

// handleStoryboardEvent 处理 WebSocket 的 storyboard_task 推送；只认当前跟踪的任务。
export function handleStoryboardEvent(raw) {
    const task = normalizeTask(raw);
    if (!task.taskId) return;
    if (!activeTask || task.taskId !== activeTask.taskId) return;
    lastPushAt = Date.now();
    applyTask(task);
}

// setStoryboardPushConnected 由 WebSocket 生命周期驱动；断线时立即兜底查询一次。
export function setStoryboardPushConnected(connected) {
    pushConnected = Boolean(connected);
    if (!pushConnected && activeTask && !TERMINAL_STATUS.has(activeTask.status)) {
        void pollNow();
    }
}

// stopStoryboardWatching 停止当前任务的轮询与跟踪。
export function stopStoryboardWatching() {
    stopPolling();
    activeTask = null;
    lastPushAt = 0;
}

// ==== 内部：请求与字段对齐 ====

async function requestJSON(url, options = {}) {
    const headers = { Accept: 'application/json' };
    if (options.body) headers['Content-Type'] = 'application/json';
    let res;
    try {
        res = await fetch(url, { ...options, headers });
    } catch (err) {
        throw new StoryboardError(`网络请求失败：${err.message || err}`);
    }
    const data = await res.json().catch(() => null);
    if (!res.ok) {
        throw new StoryboardError((data && data.error) || `请求失败（HTTP ${res.status}）`, res.status);
    }
    if (!data) {
        throw new StoryboardError('演绎系统返回了无法解析的响应');
    }
    return data;
}

// normalizeTask 对齐 server.LibraryStoryboardTask 的实际 JSON 字段。
function normalizeTask(raw) {
    const source = raw && typeof raw === 'object' ? raw : {};
    return {
        taskId: firstText(source.task_id),
        status: firstText(source.status) || 'queued',
        stage: firstText(source.stage),
        progress: clampProgress(source.progress),
        summary: firstText(source.summary),
        title: firstText(source.title),
        // event / shot_index 为增量事件标记，前端据此逐条追加渲染进度。
        event: firstText(source.event),
        shotIndex: Number(source.shot_index) || 0,
        // thought 为累计过程文本，delta 为本次新增；都是可读推演，不是原始 JSON。
        thought: typeof source.thought === 'string' ? source.thought : '',
        delta: typeof source.delta === 'string' ? source.delta : '',
        // thought_reset 要求前端清空已显示的过程文本（规划重跑时）。
        thoughtReset: source.thought_reset === true,
        docPath: firstText(source.doc_path),
        docHandle: firstText(source.doc_handle),
        shots: Array.isArray(source.shots) ? source.shots.map(normalizeShot) : [],
        handles: Array.isArray(source.handles)
            ? source.handles.filter((item) => typeof item === 'string' && item.trim()).map((item) => item.trim())
            : [],
        error: firstText(source.error),
        createdAt: firstText(source.created_at),
        updatedAt: firstText(source.updated_at),
    };
}

function normalizeShot(raw) {
    const source = raw && typeof raw === 'object' ? raw : {};
    return {
        index: Number(source.index) || 0,
        act: firstText(source.act),
        scene: firstText(source.scene),
        narration: firstText(source.narration),
        prompt: firstText(source.prompt),
        ratio: firstText(source.ratio),
        status: firstText(source.status),
        handle: firstText(source.handle),
        file: firstText(source.file),
        error: firstText(source.error),
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
        const data = await requestJSON(`${STORYBOARD_API.status}?task_id=${encodeURIComponent(task.taskId)}`);
        if (data.ok === false || !data.task) {
            throw new StoryboardError(data.error || '任务状态查询失败');
        }
        const fresh = normalizeTask(data.task);
        if (!activeTask || fresh.taskId !== activeTask.taskId) return;
        applyTask(fresh);
    } catch (err) {
        if (activeTask && activeTask.taskId === task.taskId && err.notFound) {
            const failed = { ...task, status: 'failed', error: err.message };
            activeTask = failed;
            stopPolling();
            notify({ kind: 'task', task: failed });
        }
    } finally {
        pollBusy = false;
        if (activeTask && !TERMINAL_STATUS.has(activeTask.status)) {
            schedulePoll(POLL_INTERVAL_MS);
        }
    }
}

function applyTask(task) {
    // 增量事件可能不携带全量镜头/累计文本：保留已渲染内容，避免进度被清空。
    if (activeTask && activeTask.taskId === task.taskId) {
        if (task.shots.length === 0 && activeTask.shots.length > 0) {
            task = { ...task, shots: activeTask.shots };
        }
        if (!task.thought && activeTask.thought && !task.thoughtReset) {
            task = { ...task, thought: activeTask.thought };
        }
    }
    activeTask = task;
    notify({ kind: 'task', task });
    if (TERMINAL_STATUS.has(task.status)) stopPolling();
}

// controlStoryboard 发送任务控制请求并把返回的最新状态交给统一渲染。
async function controlStoryboard(url, body) {
    const data = await requestJSON(url, {
        method: 'POST',
        body: JSON.stringify(body),
    });
    if (data.ok === false || !data.task) {
        throw new StoryboardError(data.error || '演绎任务控制失败');
    }
    const task = normalizeTask(data.task);
    if (!activeTask || activeTask.taskId !== task.taskId) {
        activeTask = task;
        notify({ kind: 'task', task });
    } else {
        applyTask(task);
    }
    if (!TERMINAL_STATUS.has(task.status)) {
        lastPushAt = Date.now();
        schedulePoll(POLL_INTERVAL_MS);
    }
    return task;
}

function notify(payload) {
    if (observer) observer(payload);
}

if (typeof window !== 'undefined') {
    window.addEventListener('pagehide', stopStoryboardWatching);
}
