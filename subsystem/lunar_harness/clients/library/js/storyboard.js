// storyboard.js 演绎链的任务模型：增量接收、按序合并。
//
// 通讯约定（全部走 /ws?channel=library）：
//   - 命令：storyboard.submit / status / list / confirm / pause / resume / cancel
//   - 推送：storyboard_task（v=2 增量协议，带 seq + epoch）
//
// 为什么是增量：规划阶段模型逐行流式返回，后端会把「标量变化 + 变化镜头 + 过程文本尾部」
// 合成一条推送。前端不再每次重建整份任务对象，而是按下标合并镜头、按 seq 丢弃旧快照，
// 因此单条消息体量与内存占用都不随镜头数增长。
//
// 过程文本（推理实况）不在本页渲染：它只产出给控制台查看，这里不再缓存，
// 否则文本会随任务时长无界增长——这正是旧实现里内存持续上涨的来源之一。

import { socketRequest, onPush, onSocketStatus } from './socket.js';

// 资源句柄文法与后端 engine/resource/variable.go 的 HandlePattern 保持一致。
const HANDLE_PATTERN = /^@[DIVAGTU]\.\d{3,}(?:\|(?:§\d+|L\d+-\d+))?$/;
// MAX_STORY_CHARS 与后端 maxStoryRunes 保持一致：超限直接拒绝，不截断。
const MAX_STORY_CHARS = 4000;
// MAX_SHOTS 与后端 maxReviewShots 保持一致：本地镜头表硬上限。
const MAX_SHOTS = 200;
const TERMINAL_STATUS = new Set(['completed', 'failed', 'canceled']);

let observer = null;
let pushBound = false;

// model 前端唯一任务模型：镜头以 index 为键，逐条合并。
const model = {
    taskId: '',
    status: '',
    stage: '',
    progress: 0,
    summary: '',
    title: '',
    event: '',
    shotIndex: 0,
    docPath: '',
    docHandle: '',
    error: '',
    handles: [],
    seq: 0,
    epoch: '',
    shots: new Map(),
    shotCount: 0,
};

export class StoryboardError extends Error {
    constructor(message, status = 0) {
        super(message);
        this.name = 'StoryboardError';
        this.status = status;
    }
}

// setStoryboardObserver 接收任务更新；UI 渲染留在 app.js。
export function setStoryboardObserver(fn) {
    observer = typeof fn === 'function' ? fn : null;
}

// bindStoryboardPush 注册推送与重连恢复（幂等，重复调用不会重复订阅）。
export function bindStoryboardPush() {
    if (pushBound) return;
    pushBound = true;
    onPush('storyboard_task', (raw) => {
        ingestStoryboardPush(raw);
    });
    // 每次（重）连上实时通道都主动对齐一次：补上断线期间漏掉的增量。
    onSocketStatus((state) => {
        if (state === 'open') void restoreStoryboard();
    });
}

// ingestStoryboardPush 处理一条增量推送：seq 更旧的直接丢弃。
export function ingestStoryboardPush(raw) {
    if (!ingest(raw, 'push')) return;
    notify({ kind: 'task', task: view() });
}

// applyStoryboardSnapshot 用一份完整快照对齐模型。
//
// 提交回执、状态查询、重连恢复都走这里：这些场景拿到的都是全量视图，
// 直接覆盖本地镜头表，不必逐条比对差异。
export function applyStoryboardSnapshot(raw) {
    if (!ingest(raw, 'full')) return null;
    return view();
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
    const result = await socketRequest('storyboard.submit', body);
    return acceptTask(unwrapTask(result, '演绎系统未返回任务信息'));
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
            original: String(shot.original || '').trim(),
            prompt: String(shot.prompt || '').trim(),
            ratio: String(shot.ratio || '').trim(),
        })),
    };
    if (title) body.title = String(title).trim();
    return controlStoryboard('storyboard.confirm', body);
}

// pauseStoryboard 暂停当前任务；已绘镜头保留，继续时从下一镜接着画。
export async function pauseStoryboard(taskId) {
    return controlStoryboard('storyboard.pause', { task_id: taskId });
}

// resumeStoryboard 继续已暂停的任务。
export async function resumeStoryboard(taskId) {
    return controlStoryboard('storyboard.resume', { task_id: taskId });
}

// cancelStoryboard 终止任务并保留已产出镜头。
export async function cancelStoryboard(taskId, reason = '') {
    const body = { task_id: taskId };
    if (reason) body.reason = reason;
    return controlStoryboard('storyboard.cancel', body);
}

// ==== 内部：模型合并 ====

// controlStoryboard 发送控制命令；回执是完整快照，直接覆盖本地模型。
async function controlStoryboard(action, body) {
    const result = await socketRequest(action, body);
    return acceptTask(unwrapTask(result, '演绎任务控制失败'));
}

// acceptTask 用回执快照对齐模型并返回对外视图。
function acceptTask(raw) {
    applyStoryboardSnapshot(raw);
    return view();
}

// restoreStoryboard 重连后对齐状态：优先续接本页正在跟踪的任务，其次接住最近未结束的任务。
async function restoreStoryboard() {
    if (model.taskId) {
        // 本页已有跟踪目标：只把它的状态对齐到最新，不抢占用户正在看的任务。
        if (TERMINAL_STATUS.has(model.status)) return;
        try {
            const result = await socketRequest('storyboard.status', { task_id: model.taskId });
            acceptTask(result && result.task);
        } catch {
            // 任务已过期时保留当前画面，等待下一次推送或用户操作。
        }
        return;
    }
    try {
        const result = await socketRequest('storyboard.list', { limit: 5 });
        const tasks = result && Array.isArray(result.tasks) ? result.tasks : [];
        const running = tasks.find((item) => item && item.task_id &&
            !TERMINAL_STATUS.has(String(item.status || '')));
        if (running) acceptTask(running);
    } catch {
        // 恢复失败不打扰用户：连接状态提示已经由 socket.js 负责。
    }
}

function unwrapTask(result, fallback) {
    if (!result || result.ok === false || !result.task) {
        throw new StoryboardError((result && result.error) || fallback);
    }
    if (!firstText(result.task.task_id)) throw new StoryboardError('演绎系统未返回任务标识');
    return result.task;
}

// ingest 唯一的模型写入口：mode 为 push 时按增量合并，为 full 时整体覆盖。
function ingest(raw, mode) {
    const source = raw && typeof raw === 'object' ? raw : null;
    if (!source) return false;
    const taskId = firstText(source.task_id);
    if (!taskId) return false;

    if (mode === 'push') {
        // 未知任务不抢当前焦点：避免历史任务的迟到推送覆盖正在看的任务。
        if (model.taskId !== taskId) return false;
    } else if (model.taskId !== taskId) {
        resetModel();
        model.taskId = taskId;
    }

    const epoch = firstText(source.epoch);
    if (epoch && model.epoch && epoch !== model.epoch) {
        // 进程重启：本地序号归零，镜头表作废等待全量。
        model.seq = 0;
        model.shots.clear();
        model.shotCount = 0;
    }
    const seq = Number(source.seq) || 0;
    if (seq > 0 && seq <= model.seq) return false;
    if (seq > 0) model.seq = seq;
    if (epoch) model.epoch = epoch;

    applyScalars(source);
    applyShots(source);
    return true;
}

// applyScalars 只覆盖本次确实携带的字段：推送省略未变化的字段。
function applyScalars(source) {
    const status = firstText(source.status);
    if (status) model.status = status;
    const stage = firstText(source.stage);
    if (stage) model.stage = stage;
    const progress = clampProgress(source.progress);
    if (progress > model.progress) model.progress = progress;
    const summary = firstText(source.summary);
    if (summary) model.summary = summary;
    const title = firstText(source.title);
    if (title) model.title = title;
    const event = firstText(source.event);
    if (event) model.event = event;
    const shotIndex = Number(source.shot_index) || 0;
    if (shotIndex > 0) model.shotIndex = shotIndex;
    const docPath = firstText(source.doc_path);
    if (docPath) model.docPath = docPath;
    const docHandle = firstText(source.doc_handle);
    if (docHandle) model.docHandle = docHandle;
    const error = firstText(source.error);
    if (error) model.error = error;
    if (Array.isArray(source.handles)) {
        model.handles = source.handles
            .filter((item) => typeof item === 'string' && item.trim())
            .map((item) => item.trim());
    }
}

// applyShots 合并镜头：shots 为全量、shot_patch 为差异，两者都不出现时表示镜头表未变。
function applyShots(source) {
    if (Array.isArray(source.shots)) {
        model.shots.clear();
        source.shots.slice(0, MAX_SHOTS).forEach((item) => {
            const shot = normalizeShot(item);
            if (shot) model.shots.set(shot.index, shot);
        });
    } else if (Array.isArray(source.shot_patch)) {
        source.shot_patch.forEach((item) => {
            const shot = normalizeShot(item);
            if (!shot) return;
            if (!model.shots.has(shot.index) && model.shots.size >= MAX_SHOTS) return;
            model.shots.set(shot.index, shot);
        });
    }
    const count = Number(source.shot_count) || 0;
    if (count > 0) {
        model.shotCount = cutShots(count);
    }
}

// cutShots 裁剪本地多出的镜头（审查后删减镜头时后端只会下发新的总数）。
function cutShots(count) {
    model.shots.forEach((shot, index) => {
        if (index > count) model.shots.delete(index);
    });
    return Math.min(count, MAX_SHOTS);
}

function normalizeShot(raw) {
    const source = raw && typeof raw === 'object' ? raw : null;
    if (!source) return null;
    const index = Number(source.index) || 0;
    if (index <= 0) return null;
    return {
        index,
        act: firstText(source.act),
        scene: firstText(source.scene),
        narration: firstText(source.narration),
        original: firstText(source.original),
        prompt: firstText(source.prompt),
        ratio: firstText(source.ratio),
        status: firstText(source.status),
        handle: firstText(source.handle),
        file: firstText(source.file),
        voiceStatus: firstText(source.voice_status),
        voiceFile: firstText(source.voice_file),
        voiceDurationMs: Number(source.voice_duration_ms) || 0,
        voiceError: firstText(source.voice_error),
        error: firstText(source.error),
    };
}

// view 生成对外只读视图：镜头按序号升序，供 UI 直接消费。
function view() {
    return {
        taskId: model.taskId,
        status: model.status || 'queued',
        stage: model.stage,
        progress: model.progress,
        summary: model.summary,
        title: model.title,
        event: model.event,
        shotIndex: model.shotIndex,
        docPath: model.docPath,
        docHandle: model.docHandle,
        error: model.error,
        handles: model.handles.slice(),
        seq: model.seq,
        epoch: model.epoch,
        shots: orderedShots(),
    };
}

function orderedShots() {
    const list = Array.from(model.shots.values());
    list.sort((a, b) => a.index - b.index);
    return list;
}

function resetModel() {
    model.taskId = '';
    model.status = '';
    model.stage = '';
    model.progress = 0;
    model.summary = '';
    model.title = '';
    model.event = '';
    model.shotIndex = 0;
    model.docPath = '';
    model.docHandle = '';
    model.error = '';
    model.handles = [];
    model.seq = 0;
    model.epoch = '';
    model.shots.clear();
    model.shotCount = 0;
}

function firstText(value) {
    if (typeof value !== 'string') return '';
    const text = value.trim();
    return text;
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
        // 渲染失败只影响这一次更新，不能影响后续推送。
        console.error('[library] 演绎进度渲染失败', err);
    }
}
