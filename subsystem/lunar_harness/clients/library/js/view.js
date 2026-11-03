// view.js 预览 / 变更 / 详情三块视图的渲染。

import { fileURL } from './api.js';
import { humanSize, formatTime, kindOf, statusClass, isRunnable } from './model.js';
import { languageOf, highlightCode, highlightLines } from './syntax.js';

const MAX_TEXT_CHARS = 400000;
const OP_LABELS = {
    read: '读取', search: '检索', info: '查看属性', write: '写入', create: '新建',
    replace: '置换', delete: '删除', copy: '复制', move: '移动', rename: '重命名', run: '运行',
};

// renderPreview 渲染文件预览（文本 / 图片 / 视频 / 音频 / 其他）。
export async function renderPreview(container, entry, fileState) {
    container.replaceChildren();
    const kind = kindOf(entry.name);
    if (kind === 'text') {
        await renderText(container, entry, fileState);
        return;
    }
    if (kind === 'image' || kind === 'video' || kind === 'audio') {
        renderMedia(container, entry, kind);
        return;
    }
    container.append(emptyState('fa-file-shield', '该类型暂不支持在线预览', '可在右侧详情查看元数据'));
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
        video.addEventListener('error', () => mediaError(wrap, '当前内嵌环境无法播放该视频编码'));
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

// renderDetails 右侧详情：固定字段 + AI 状态 + 沙箱入口。
export function renderDetails(container, entry, meta, fileState, onRun) {
    container.replaceChildren();
    if (!entry) {
        container.append(emptyState('fa-hand-pointer', '选择左侧文件查看详情', ''));
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
        gcSection.append(row('结算周期', `每 ${(meta && meta.gc_cycle_hours) || 12} 小时`));
        gcSection.append(gcStatusRow(meta));
        const note = document.createElement('div');
        note.className = 'detail-note';
        note.textContent = '只有 multimedia 根媒体与哈希命名的临时音频参与结算；文档、子目录与命名音频永不被积分回收。未被取用时每次 -1，被取用时 +1，积分降到 -20 触发回收；在资源库打开或 AI 读取都算一次取用。';
        gcSection.append(note);
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

// gcStatusRow 用「还剩几次结算」替代裸积分，让用户能直接读懂回收时点。
function gcStatusRow(meta) {
    const wrap = row('回收预计', '');
    const value = wrap.querySelector('.v');
    if (!meta) {
        value.textContent = '—';
        return wrap;
    }
    if (!meta.gc_tracked) {
        value.textContent = '不在回收范围';
        return wrap;
    }
    if (meta.state === 'lost') {
        value.className = 'v bad';
        value.textContent = '文件本体已回收，句柄保留';
        return wrap;
    }
    const cycles = Number(meta.gc_cycles_left || 0);
    value.className = cycles <= 3 ? 'v bad' : cycles <= 8 ? 'v warn' : 'v';
    value.textContent = meta.gc_protected
        ? `持续未用约 ${cycles} 次结算后回收本体（描述保留）`
        : `持续未用约 ${cycles} 次结算后回收`;
    return wrap;
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
