// requirement.js 创作链的单通道任务跟踪。
//
// 与演绎链同构：命令与推送都走 /ws?channel=library，不再保留轮询兜底。
// 命令失败由回执明确返回错误，界面负责提示，不会静默卡住。

import { socketRequest, onPush, onSocketStatus } from './socket.js';

// 资源句柄文法与后端 engine/resource/variable.go 的 HandlePattern 保持一致：
// 类型码 D/I/V/A/G/T/U + 三位起序号，可选段落/行锚点。
const HANDLE_PATTERN = /^@[DIVAGTU]\.\d{3,}(?:\|(?:§\d+|L\d+-\d+))?$/;
const TERMINAL_STATUS = new Set(['completed', 'failed']);

let observer = null;
let activeTask = null;
let pushBound = false;

export class RequirementError extends Error {
    constructor(message, status = 0) {
        super(message);
        this.name = 'RequirementError';
        this.status = status;
    }
}

// setRequirementObserver 接收任务更新或终态错误；UI 渲染留在 app.js。
export function setRequirementObserver(fn) {
    observer = typeof fn === 'function' ? fn : null;
}

// bindRequirementPush 注册推送与重连恢复（幂等）。
export function bindRequirementPush() {
    if (pushBound) return;
    pushBound = true;
    onPush('creation_task', (raw) => {
        applySnapshot(raw, false);
    });
    onSocketStatus((state) => {
        if (state === 'open') void restoreRequirement();
    });
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

// submitRequirement 提交任务并返回已受理的任务视图。
export async function submitRequirement(input) {
    const body = prepareRequirement(input.handle, input.description);
    const result = await socketRequest('creation.submit', buildRequestBody(body));
    const task = unwrapTask(result, '创作系统未返回任务信息');
    activeTask = task;
    return task;
}

// ==== 内部：请求与字段对齐 ====

// buildRequestBody 只发后端声明的字段（DisallowUnknownFields）。
function buildRequestBody({ mode, handle, description }) {
    const body = { mode };
    if (description) body.description = description;
    if (handle) body.handle = handle;
    return body;
}

// restoreRequirement 重连后接住最近未结束的创作任务。
async function restoreRequirement() {
    // 本页已有跟踪目标时不抢占：只在页面刚打开、尚无任务时恢复最近未结束的任务。
    if (activeTask) return;
    try {
        const result = await socketRequest('creation.list', { limit: 5 });
        const tasks = result && Array.isArray(result.tasks) ? result.tasks : [];
        const running = tasks.find((item) => item && item.task_id &&
            !TERMINAL_STATUS.has(String(item.status || '')));
        if (running) applySnapshot(running, true);
    } catch {
        // 恢复失败不打扰用户：连接状态提示由 socket.js 统一负责。
    }
}

function unwrapTask(result, fallback) {
    if (!result || result.ok === false || !result.task) {
        throw new RequirementError((result && result.error) || fallback);
    }
    const task = normalizeTask(result.task);
    if (!task.taskId) throw new RequirementError('创作系统未返回任务标识');
    return task;
}

// applySnapshot 唯一的快照入口：未知任务不抢当前焦点。
function applySnapshot(raw, force) {
    const task = normalizeTask(raw);
    if (!task.taskId) return;
    if (!activeTask || activeTask.taskId !== task.taskId) {
        if (!force) return;
        activeTask = task;
        notify({ kind: 'task', task });
        return;
    }
    activeTask = {
        ...task,
        handles: task.handles.length ? task.handles : activeTask.handles,
    };
    notify({ kind: 'task', task: activeTask });
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

function notify(payload) {
    if (!observer) return;
    try {
        observer(payload);
    } catch (err) {
        console.error('[library] 创作进度渲染失败', err);
    }
}
