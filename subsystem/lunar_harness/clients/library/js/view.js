// view.js 预览 / 变更 / 详情三块视图的渲染。

import { fileURL } from './api.js';
import { humanSize, formatTime, kindOf, statusClass, isRunnable } from './model.js';
import { languageOf, highlightCode, highlightLines } from './syntax.js';

const MAX_TEXT_CHARS = 400000;
const OP_LABELS = {
    read: '读取', search: '检索', info: '查看属性', write: '写入', create: '新建',
    replace: '置换', append: '追加', delete: '删除', copy: '复制', move: '移动', rename: '重命名', run: '运行',
};

// renderPreview 渲染文件预览（文本 / 图片 / 视频 / 音频 / 其他）。
// 数据库与压缩包由 app.js 分发到专用视图，这里只保留保底提示。
export async function renderPreview(container, entry, fileState) {
    container.replaceChildren();
    const kind = kindOf(entry.name);
    if (kind === 'text') {
        await renderText(container, entry, fileState);
        return;
    }
    if (kind === 'image' || kind === 'audio') {
        renderMedia(container, entry, kind);
        return;
    }
    if (kind === 'video') {
        // 空文件没有可解码的数据，直接给出明确提示，避免播放器留下黑屏。
        if (!entry.size) {
            container.append(emptyState('fa-triangle-exclamation', '视频文件为空', '文件大小为 0 字节，无法播放'));
            return;
        }
        // 扩展名与响应 Content-Type 双重判断：避免把伪装成视频的文本/错误页当视频播放。
        const contentType = await probeContentType(fileURL(entry.path));
        if (contentType && !isVideoContentType(contentType)) {
            container.append(emptyState('fa-film', '该文件不是可播放的视频', `响应类型为 ${contentType}`));
            return;
        }
        renderMedia(container, entry, 'video');
        return;
    }
    if (kind === 'binary') {
        // 扩展名未知时，Content-Type 明确指出视频也允许用原生播放器打开。
        const contentType = await probeContentType(fileURL(entry.path));
        if (contentType && contentType.startsWith('video/')) {
            renderMedia(container, entry, 'video');
            return;
        }
    }
    container.append(emptyState('fa-file-shield', '该类型暂不支持在线预览', '可在右侧详情查看元数据'));
}

// probeContentType 通过 Range 读 1 字节探测响应类型；后端不支持 HEAD，只走 GET。
async function probeContentType(src) {
    try {
        const res = await fetch(src, { headers: { Range: 'bytes=0-0' } });
        const contentType = (res.headers.get('Content-Type') || '').toLowerCase();
        if (res.body && typeof res.body.cancel === 'function') {
            void res.body.cancel().catch(() => {});
        }
        return contentType;
    } catch {
        return '';
    }
}

function isVideoContentType(contentType) {
    return contentType.startsWith('video/')
        || contentType === 'application/octet-stream'
        || contentType.startsWith('binary/');
}

// renderText 文本预览：带行号，最近一次新增区间用绿色标记。
async function renderText(container, entry, fileState) {
    let content = '';
    try {
        const res = await fetch(fileURL(entry.path));
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        content = await res.text();
    } catch (err) {
        container.append(emptyState('fa-triangle-exclamation', '读取失败', String(err.message || err)));
        return;
    }
    if (content.length > MAX_TEXT_CHARS) {
        content = content.slice(0, MAX_TEXT_CHARS);
    }
    const normalized = content.replace(/\r\n/g, '\n');
    const lines = normalized.split('\n');
    const range = addedRange(fileState);
    // 预览是整份文件：整段高亮一次后按行拆分；未知语言 / 超大文件 / 拆分失败时返回 null，回退纯文本
    const language = languageOf(entry.path || '');
    const highlighted = highlightLines(normalized, language);
    const pre = document.createElement('pre');
    pre.className = 'text-view';
    lines.forEach((line, index) => {
        const no = index + 1;
        const row = document.createElement('div');
        row.className = 'text-line';
        if (range && no >= range.from && no <= range.to) {
            row.classList.add('added');
        }
        const gutter = document.createElement('span');
        gutter.className = 'ln';
        gutter.textContent = String(no);
        const text = document.createElement('span');
        text.className = 'tx';
        const html = highlighted && line !== '' ? highlighted[index] : '';
        if (html) text.innerHTML = html;
        else text.textContent = line === '' ? ' ' : line;
        row.append(gutter, text);
        pre.append(row);
    });
    container.append(pre);
    if (content.length >= MAX_TEXT_CHARS) {
        container.append(noteLine('内容过长，仅展示前 40 万字符'));
    }
}

// renderMedia 媒体预览：图片 / 视频 / 音频各用原生控件。
function renderMedia(container, entry, kind) {
    const wrap = document.createElement('div');
    wrap.className = 'media-wrap';
    const src = fileURL(entry.path);
    if (kind === 'image') {
        const img = document.createElement('img');
        img.src = src;
        img.alt = entry.name;
        img.loading = 'lazy';
        img.addEventListener('error', () => mediaError(wrap, '当前内嵌环境无法解码该图片格式'));
        wrap.append(img);
    } else if (kind === 'video') {
        const video = document.createElement('video');
        video.src = src;
        video.controls = true;
        video.preload = 'metadata';
        video.playsInline = true;
        video.addEventListener('error', () => void reportVideoError(wrap, video, src));
        wrap.append(video);
    } else {
        const audio = document.createElement('audio');
        audio.src = src;
        audio.controls = true;
        audio.preload = 'metadata';
        audio.addEventListener('error', () => mediaError(wrap, '当前内嵌环境无法解码该音频格式'));
        wrap.append(audio);
    }
    container.append(wrap);
    container.append(noteLine(kind === 'image' ? '图片按原始比例展示，动图会自动播放' : '拖动进度条可定位播放位置'));
}

// renderChanges 最近 10 次 AI 操作：新增绿色、删除红色删除线。
export function renderChanges(container, fileState) {
    container.replaceChildren();
    if (!fileState || !fileState.records || fileState.records.length === 0) {
        container.append(emptyState('fa-clock-rotate-left', '暂无 AI 操作记录', '文件被读取、写入或运行后会在这里留下痕迹'));
        return;
    }
    const list = document.createElement('div');
    list.className = 'change-list';
    const records = [...fileState.records].reverse();
    const language = languageOf(fileState.path || '');
    for (const record of records) {
        const item = document.createElement('div');
        item.className = 'change-item';
        const head = document.createElement('div');
        head.className = 'change-head';
        const op = document.createElement('span');
        op.className = 'change-op';
        op.textContent = OP_LABELS[record.op] || record.op || '操作';
        const time = document.createElement('span');
        time.textContent = formatTime(record.at);
        head.append(op, time);
        const body = document.createElement('div');
        body.className = 'change-body';
        if (record.added) body.append(changeLine('added', record.added, language));
        if (record.removed) body.append(changeLine('removed', record.removed, language));
        if (!record.added && !record.removed) {
            const empty = document.createElement('div');
            empty.className = 'change-empty';
            empty.textContent = '本次操作没有正文变更';
            body.append(empty);
        }
        item.append(head, body);
        list.append(item);
    }
    container.append(list);
    container.append(noteLine('仅保留最近 10 次操作的视觉标记'));
}

// renderDetails 右侧详情：管理入口 + 固定字段 + AI 状态 + 沙箱入口。
export function renderDetails(container, entry, meta, fileState, onRun, actions) {
    container.replaceChildren();
    if (!entry) {
        container.append(emptyState('fa-hand-pointer', '选择文件查看详情', ''));
        return;
    }
    const head = document.createElement('div');
    head.className = 'detail-head';
    const titleRow = document.createElement('div');
    titleRow.className = 'detail-title-row';
    const title = document.createElement('div');
    title.className = `detail-title ${statusClass(fileState && fileState.status)}`;
    title.textContent = entry.name;
    titleRow.append(title, statusPill(fileState));
    const path = document.createElement('div');
    path.className = 'detail-path';
    path.textContent = entry.path;
    head.append(titleRow, path);
    container.append(head);

    // 重命名 / 移动 / 删除从主显示区迁到这里，主显示区只保留浏览与刷新。
    if (!entry.dir && actions) {
        const manage = section('管理');
        const bar = document.createElement('div');
        bar.className = 'detail-actions';
        bar.append(
            actionButton('fa-pen', '重命名', actions.onRename),
            actionButton('fa-folder-open', '移动', actions.onMove),
            actionButton('fa-trash', '删除', actions.onDelete, true),
        );
        manage.append(bar);
        container.append(manage);
    }

    const info = section('文件信息');
    info.append(row('类型', entry.dir ? '文件夹' : (extLabel(entry.name) || '未知')));
    info.append(row('大小', humanSize(entry.size)));
    info.append(row('修改时间', formatTime(entry.mtime)));
    container.append(info);

    if (!entry.dir) {
        const metaSection = section('资源元数据');
        metaSection.append(row('入库时间', meta && meta.created_at ? formatTime(meta.created_at) : '—'));
        metaSection.append(row('句柄 ID', (meta && meta.handle) || (fileState && fileState.handle) || '—'));
        metaSection.append(row('保留积分', meta ? String(meta.points ?? 0) : '—'));
        metaSection.append(descriptionRow(meta));
        container.append(metaSection);

        const gcSection = section('回收状态');
        gcSection.append(gcStatusRow(meta));
        container.append(gcSection);

        const ai = section('AI 操作状态');
        ai.append(statusRow(fileState));
        if (isRunnable(entry.name)) {
            const runBtn = document.createElement('button');
            runBtn.className = 'btn-glass primary';
            runBtn.innerHTML = '<i class="fas fa-play"></i> 在沙箱中运行';
            runBtn.addEventListener('click', () => onRun && onRun(entry));
            ai.append(runBtn);
        }
        container.append(ai);
    }
}

// addedRange 最近一次新增内容的行区间（用于文本滚动高亮）。
function addedRange(fileState) {
    if (!fileState || !fileState.records) return null;
    for (let i = fileState.records.length - 1; i >= 0; i -= 1) {
        const record = fileState.records[i];
        if (record.added && record.start_line > 0) {
            const count = record.lines > 0 ? record.lines : 1;
            return { from: record.start_line, to: record.start_line + count - 1 };
        }
    }
    return null;
}

function statusPill(fileState) {
    const status = fileState ? fileState.status : 'normal';
    const el = document.createElement('span');
    el.className = 'detail-status';
    if (status === 'error') {
        el.classList.add('bad');
        el.textContent = '异常';
    } else if (status === 'ok') {
        el.classList.add('ok');
        el.textContent = '正常';
    } else if (status === 'active') {
        el.classList.add('warn');
        el.textContent = 'AI 操作';
    } else {
        el.textContent = '无操作';
    }
    return el;
}

function statusRow(fileState) {
    const wrap = row('最近状态', '');
    const value = wrap.querySelector('.v');
    const status = fileState ? fileState.status : 'normal';
    if (status === 'error') {
        value.className = 'v bad';
        value.textContent = '运行异常（红）';
    } else if (status === 'ok') {
        value.className = 'v good';
        value.textContent = '运行正常（绿）';
    } else if (status === 'active') {
        value.className = 'v warn';
        value.textContent = 'AI 最近操作（黄）';
    } else {
        value.className = 'v';
        value.textContent = '无操作记录';
    }
    return wrap;
}

function descriptionRow(meta) {
    const wrap = row('描述状态', '');
    const value = wrap.querySelector('.v');
    const has = Boolean(meta && meta.has_description);
    value.className = has ? 'v good' : 'v';
    value.textContent = has ? '已添加描述' : '未添加描述';
    return wrap;
}

// gcStatusRow 回收预期：把剩余结算次数折算成小时，直接给出销毁时点。
function gcStatusRow(meta) {
    const wrap = row('回收预期', '');
    const value = wrap.querySelector('.v');
    if (!meta) {
        value.textContent = '—';
        return wrap;
    }
    // 不在回收范围的资源不会被积分回收，也就没有销毁时点。
    if (!meta.gc_tracked) {
        value.textContent = '永不销毁';
        return wrap;
    }
    if (meta.state === 'lost') {
        value.className = 'v bad';
        value.textContent = '文件本体已回收，句柄保留';
        return wrap;
    }
    const cycles = Number(meta.gc_cycles_left || 0);
    const cycleHours = Number(meta.gc_cycle_hours || 12);
    value.className = cycles <= 3 ? 'v bad' : cycles <= 8 ? 'v warn' : 'v';
    value.textContent = `预计 ${cycles * cycleHours} 小时后销毁`;
    return wrap;
}

// actionButton 详情里的管理操作按钮，样式沿用主显示区的玻璃按钮。
function actionButton(icon, label, handler, danger = false) {
    const btn = document.createElement('button');
    btn.className = `btn-glass btn-sm${danger ? ' danger' : ''}`;
    btn.innerHTML = `<i class="fas ${icon}"></i><span>${label}</span>`;
    btn.addEventListener('click', () => handler && handler());
    return btn;
}

function section(titleText) {
    const wrap = document.createElement('div');
    wrap.className = 'detail-section';
    const head = document.createElement('div');
    head.className = 'detail-section-title';
    head.textContent = titleText;
    wrap.append(head);
    return wrap;
}

function row(key, value) {
    const wrap = document.createElement('div');
    wrap.className = 'detail-row';
    const k = document.createElement('span');
    k.className = 'k';
    k.textContent = key;
    const v = document.createElement('span');
    v.className = 'v';
    v.textContent = value;
    wrap.append(k, v);
    return wrap;
}

// changeLine 一条新增 / 删除片段：保留红绿底色与 +/- 标记，代码部分叠加语法高亮。
function changeLine(kind, text, language) {
    const line = document.createElement('div');
    line.className = `change-line ${kind}`;
    const mark = document.createElement('span');
    mark.className = 'cl-mark';
    mark.textContent = kind === 'added' ? '+' : '−';
    const code = document.createElement('span');
    code.className = 'cl-text';
    const html = highlightCode(text, language);
    if (html) code.innerHTML = html;
    else code.textContent = text;
    line.append(mark, code);
    return line;
}

// 视频编码四字符码 → 可读名称（MP4 的 stsd 盒内会出现这些标识）。
const VIDEO_CODEC_LABELS = {
    hvc1: 'HEVC (H.265)',
    hev1: 'HEVC (H.265)',
    av01: 'AV1',
    vp09: 'VP9',
    vp08: 'VP8',
    avc1: 'H.264 (AVC)',
};

// reportVideoError 播放失败时给出可读原因：媒体错误码 + 实际编码探测，
// 避免静默黑屏；编码是否被内核支持由浏览器决定，这里只解释失败原因。
async function reportVideoError(wrap, video, src) {
    const error = video.error;
    const code = error && error.code ? `媒体错误 ${error.code}` : '';
    const message = error && error.message ? error.message : '';
    const codec = await sniffVideoCodec(src);
    const reason = codec
        ? `检测到 ${codec} 编码，当前浏览器内核不支持该编码`
        : '可能是 HEVC/H.265、AV1 编码或 MKV 封装，当前浏览器内核无法解码';
    const detail = [code, message].filter(Boolean).join('，');
    mediaError(wrap, detail ? `无法播放该视频（${detail}）：${reason}` : `无法播放该视频：${reason}`);
}

// sniffVideoCodec 读取文件首尾片段并查找 MP4 编码标识。
// moov 盒可能位于文件头或文件尾，因此两段都探测；仅在播放失败时调用。
async function sniffVideoCodec(src) {
    const decoder = new TextDecoder('latin1');
    const readRange = async (range) => {
        try {
            const res = await fetch(src, { headers: { Range: range } });
            if (!res.ok) return '';
            return decoder.decode(await res.arrayBuffer());
        } catch {
            return '';
        }
    };
    const text = await readRange('bytes=0-262143') + await readRange('bytes=-262144');
    for (const [fourcc, label] of Object.entries(VIDEO_CODEC_LABELS)) {
        if (text.includes(fourcc)) return label;
    }
    if (text.includes('matroska')) return 'Matroska (MKV)';
    return '';
}

// mediaError 解码失败时给出可读说明，而不是留一片空白。
function mediaError(wrap, text) {
    wrap.replaceChildren();
    const note = document.createElement('div');
    note.className = 'empty-state';
    note.innerHTML = '<i class="fas fa-triangle-exclamation"></i>';
    const p = document.createElement('p');
    p.textContent = text;
    note.append(p);
    wrap.append(note);
}

function noteLine(text) {
    const note = document.createElement('div');
    note.className = 'media-note';
    note.textContent = text;
    return note;
}

function emptyState(icon, title, sub) {
    const wrap = document.createElement('div');
    wrap.className = 'empty-state';
    const i = document.createElement('i');
    i.className = `fas ${icon}`;
    const titleEl = document.createElement('p');
    titleEl.textContent = title;
    wrap.append(i, titleEl);
    if (sub) {
        const subEl = document.createElement('span');
        subEl.textContent = sub;
        wrap.append(subEl);
    }
    return wrap;
}

function extLabel(name) {
    const dot = name.lastIndexOf('.');
    if (dot < 0) return '';
    return name.slice(dot + 1).toUpperCase();
}
