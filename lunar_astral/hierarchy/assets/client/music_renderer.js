/**
 * 音乐渲染器模块
 *
 * 负责接收 ABC 记谱法乐谱，使用 Tone.js 合成音频播放，
 * 提供音频可视化、进度控制、速度调节、下载等功能。
 *
 * 依赖：
 *   - Tone.js（全局 window.Tone）：音频引擎与效果处理
 */

// ==== 音乐渲染器状态 ====
const state = {
    /** 当前 ABC 记谱法乐谱 */
    abcNotation: '',
    /** 当前作品标题 */
    title: '',
    /** 是否正在播放 */
    isPlaying: false,
    /** 是否已暂停（区别于完全停止） */
    isPaused: false,
    /** 总时长（秒） */
    totalDuration: 0,
    /** 当前 BPM */
    bpm: 120,
    /** 调式 */
    key: 'C',
    /** 拍号 */
    meter: '4/4',
    /** 解析后的音符序列 */
    notes: [],
    /** Tone.js 合成器实例 */
    toneSynth: null,
    /** Tone.js 混响实例 */
    toneReverb: null,
    /** 动画帧 ID */
    animationId: null,
    /** 音乐历史记录 */
    history: [],
};

// ==== DOM 元素缓存 ====
let elements = {};

/**
 * 初始化音乐渲染器
 *
 * 创建音乐播放模态框 DOM 结构并挂载到页面。
 * 应在 DOMContentLoaded 之后调用。
 */
export function initMusicRenderer() {
    if (document.getElementById('music-modal-overlay')) return;

    const overlay = document.createElement('div');
    overlay.id = 'music-modal-overlay';
    overlay.className = 'music-modal-overlay';
    overlay.innerHTML = `
        <div class="music-modal">
            <div class="music-modal-header">
                <div class="music-modal-title-area" id="music-title-area">
                    <span class="music-modal-title" id="music-title">音乐播放器</span>
                </div>
                <div class="music-modal-actions">
                    <button class="music-btn" id="music-btn-history" title="历史记录">
                        <i class="fas fa-history"></i>
                    </button>
                    <button class="music-btn" id="music-btn-download" title="下载乐谱">
                        <i class="fas fa-download"></i>
                    </button>
                    <button class="music-btn music-btn-close" id="music-btn-close" title="关闭">
                        <i class="fas fa-times"></i>
                    </button>
                </div>
            </div>
            <div class="music-modal-body">
                <div class="music-visualizer-container" id="music-visualizer-container">
                    <canvas id="music-visualizer-canvas"></canvas>
                    <div class="music-title-scroll" id="music-title-scroll" style="display:none;">
                        <span class="music-title-scroll-text" id="music-title-scroll-text"></span>
                    </div>
                </div>
            </div>
            <div class="music-modal-settings" id="music-settings">
                <div class="music-setting">
                    <label class="music-setting-label"><i class="fas fa-tachometer-alt"></i> 速度</label>
                    <select class="music-setting-select" id="music-setting-speed">
                        <option value="0.5">0.5x</option>
                        <option value="0.75">0.75x</option>
                        <option value="1" selected>1x</option>
                        <option value="1.25">1.25x</option>
                        <option value="1.5">1.5x</option>
                        <option value="2">2x</option>
                    </select>
                </div>
                <div class="music-setting">
                    <label class="music-setting-label"><i class="fas fa-music"></i> 调式</label>
                    <span class="music-setting-value" id="music-setting-key">C</span>
                </div>
                <div class="music-setting">
                    <label class="music-setting-label"><i class="fas fa-clock"></i> 拍号</label>
                    <span class="music-setting-value" id="music-setting-meter">4/4</span>
                </div>
            </div>
            <div class="music-modal-footer">
                <span class="music-time" id="music-time-current">00:00</span>
                <div class="music-progress" id="music-progress-bar">
                    <div class="music-progress-fill" id="music-progress-fill"></div>
                </div>
                <span class="music-time" id="music-time-total">00:00</span>
                <button class="music-btn music-btn-play-lg" id="music-btn-play" title="播放">
                    <i class="fas fa-play"></i>
                </button>
            </div>
        </div>
        <div class="music-history-panel" id="music-history-panel" style="display:none;">
            <div class="music-history-header">
                <span>创作历史</span>
                <button class="music-btn music-btn-close" id="music-btn-history-close">
                    <i class="fas fa-times"></i>
                </button>
            </div>
            <div class="music-history-list" id="music-history-list"></div>
        </div>
    `;

    document.body.appendChild(overlay);

    elements = {
        overlay: document.getElementById('music-modal-overlay'),
        title: document.getElementById('music-title'),
        titleArea: document.getElementById('music-title-area'),
        titleScroll: document.getElementById('music-title-scroll'),
        titleScrollText: document.getElementById('music-title-scroll-text'),
        visualizerCanvas: document.getElementById('music-visualizer-canvas'),
        btnPlay: document.getElementById('music-btn-play'),
        btnHistory: document.getElementById('music-btn-history'),
        btnDownload: document.getElementById('music-btn-download'),
        btnClose: document.getElementById('music-btn-close'),
        btnHistoryClose: document.getElementById('music-btn-history-close'),
        progressBar: document.getElementById('music-progress-bar'),
        progressFill: document.getElementById('music-progress-fill'),
        timeCurrent: document.getElementById('music-time-current'),
        timeTotal: document.getElementById('music-time-total'),
        speedSelect: document.getElementById('music-setting-speed'),
        keyDisplay: document.getElementById('music-setting-key'),
        meterDisplay: document.getElementById('music-setting-meter'),
        historyPanel: document.getElementById('music-history-panel'),
        historyList: document.getElementById('music-history-list'),
    };

    initCanvas();
    bindEvents();
}

// ==== Canvas 初始化 ====
function initCanvas() {
    const canvas = elements.visualizerCanvas;
    const container = canvas.parentElement;
    resizeCanvas();
    window.addEventListener('resize', resizeCanvas);
    // 初始绘制空状态
    drawVisualizerIdle();
}

function resizeCanvas() {
    const canvas = elements.visualizerCanvas;
    const container = canvas.parentElement;
    if (!container) return;
    const rect = container.getBoundingClientRect();
    const dpr = window.devicePixelRatio || 1;
    canvas.width = rect.width * dpr;
    canvas.height = rect.height * dpr;
    canvas.style.width = rect.width + 'px';
    canvas.style.height = rect.height + 'px';
    const ctx = canvas.getContext('2d');
    // 重置变换矩阵后再缩放，避免累积
    ctx.setTransform(1, 0, 0, 1, 0, 0);
    ctx.scale(dpr, dpr);
}

// ==== 事件绑定 ====
function bindEvents() {
    elements.btnClose.addEventListener('click', hideMusicModal);
    elements.overlay.addEventListener('click', (e) => {
        if (e.target === elements.overlay) hideMusicModal();
    });

    elements.btnPlay.addEventListener('click', togglePlayback);
    elements.btnHistory.addEventListener('click', toggleHistoryPanel);
    elements.btnHistoryClose.addEventListener('click', () => {
        elements.historyPanel.style.display = 'none';
    });
    elements.btnDownload.addEventListener('click', downloadScore);

    // 速度选择：如果在播放中则从当前位置重新调度
    elements.speedSelect.addEventListener('change', (e) => {
        if (state.isPlaying) {
            const currentRealTime = Tone.Transport.seconds * (state._speed || 1);
            seekTo(currentRealTime);
        }
    });

    // 进度条点击跳转
    elements.progressBar.addEventListener('click', (e) => {
        if (state.totalDuration <= 0) return;
        const rect = elements.progressBar.getBoundingClientRect();
        const ratio = (e.clientX - rect.left) / rect.width;
        const seekTime = Math.max(0, Math.min(state.totalDuration, ratio * state.totalDuration));
        seekTo(seekTime);
    });

    // 键盘快捷键
    document.addEventListener('keydown', (e) => {
        if (!elements.overlay.classList.contains('visible')) return;
        if (e.code === 'Space') {
            e.preventDefault();
            togglePlayback();
        } else if (e.code === 'Escape') {
            hideMusicModal();
        }
    });
}

// ==== 渲染乐谱入口 ====
/**
 * 加载并准备播放 ABC 记谱法乐谱
 *
 * @param {string} abcNotation - ABC 记谱法乐谱文本
 */
export function renderMusicScore(abcNotation) {
    if (!abcNotation || typeof abcNotation !== 'string') {
        console.warn('[音乐渲染器] 无效的乐谱数据');
        return;
    }

    if (!window.Tone) {
        console.error('[音乐渲染器] Tone.js 库未加载');
        showError('音频引擎未加载，请刷新页面后重试');
        return;
    }

    // 停止当前播放
    stopPlayback();

    // 保存乐谱
    state.abcNotation = abcNotation;

    // 提取元信息
    const titleMatch = abcNotation.match(/^T:\s*(.+)/m);
    state.title = titleMatch ? titleMatch[1].trim() : '未命名作品';

    const tempoMatch = abcNotation.match(/^Q:\s*(.+)/m);
    if (tempoMatch) {
        const tempoStr = tempoMatch[1].trim();
        const bpmMatch = tempoStr.match(/(\d+)/);
        if (bpmMatch) state.bpm = parseInt(bpmMatch[1]);
    }

    const keyMatch = abcNotation.match(/^K:\s*(.+)/m);
    state.key = keyMatch ? keyMatch[1].trim() : 'C';

    const meterMatch = abcNotation.match(/^M:\s*(.+)/m);
    state.meter = meterMatch ? meterMatch[1].trim() : '4/4';

    // 解析音符
    state.notes = parseABCNotes(abcNotation);
    if (state.notes.length === 0) {
        console.warn('[音乐渲染器] 无法解析 ABC 乐谱中的音符');
        showError('乐谱中未找到有效音符');
        return;
    }

    // 计算总时长
    state.totalDuration = state.notes[state.notes.length - 1].time + state.notes[state.notes.length - 1].duration;

    // 更新 UI
    updateUI();
    resetPlaybackState();

    // 显示模态框
    showMusicModal();

    // 添加到历史记录
    addToHistory(state.title, abcNotation);

    console.log(`[音乐渲染器] 乐谱就绪: "${state.title}", ${state.notes.length} 个音符, ${state.totalDuration.toFixed(1)}s`);
}

// ==== UI 更新 ====
function updateUI() {
    elements.title.textContent = state.title;
    elements.titleScrollText.textContent = state.title;
    elements.keyDisplay.textContent = state.key;
    elements.meterDisplay.textContent = state.meter;
    elements.timeTotal.textContent = formatTime(state.totalDuration);
    elements.timeCurrent.textContent = '00:00';
    elements.progressFill.style.width = '0%';
    elements.speedSelect.value = '1';
}

function resetPlaybackState() {
    state.isPlaying = false;
    state.isPaused = false;
    updatePlayButton();
    stopAnimation();
    showScrollingTitle();
}

// ==== 播放控制 ====
function togglePlayback() {
    if (state.isPlaying) {
        pausePlayback();
    } else {
        startPlayback();
    }
}

function startPlayback() {
    if (!state.abcNotation || state.notes.length === 0) return;

    // 确保音频上下文已启动
    Tone.start().then(() => {
        if (state.isPaused) {
            resumePlayback();
        } else {
            beginNewPlayback();
        }
    });
}

function beginNewPlayback() {
    // 清理之前的合成器
    disposeSynth();

    // 创建合成器
    state.toneSynth = new Tone.PolySynth(Tone.Synth, {
        oscillator: { type: 'triangle' },
        envelope: {
            attack: 0.02,
            decay: 0.1,
            sustain: 0.3,
            release: 0.5,
        },
    }).toDestination();

    // 添加混响效果
    state.toneReverb = new Tone.Reverb({
        decay: 1.5,
        wet: 0.3,
    }).toDestination();
    state.toneSynth.connect(state.toneReverb);

    // Transport.bpm 保持 60（1拍=1秒），速度通过调整调度时间实现
    Tone.Transport.bpm.value = 60;

    // 获取速度倍率：speed < 1 则时间拉长（慢放），speed > 1 则时间缩短（快放）
    const speed = parseFloat(elements.speedSelect.value);
    const timeScale = 1 / speed;

    // 使用 Transport.schedule 调度所有音符（时间单位：秒）
    state.notes.forEach(({ note, duration, time }) => {
        const scheduledTime = time * timeScale;
        const noteDuration = duration * timeScale * 0.9;
        Tone.Transport.schedule((t) => {
            state.toneSynth.triggerAttackRelease(note, noteDuration, t);
        }, scheduledTime);
    });

    // 调度结束事件
    const adjustedTotal = state.totalDuration * timeScale;
    Tone.Transport.schedule(() => {
        stopPlayback();
    }, adjustedTotal + 0.1);

    // 记录当前速度倍率用于进度换算
    state._speed = speed;
    state._timeScale = timeScale;

    // 开始播放
    Tone.Transport.start();
    state.isPlaying = true;
    state.isPaused = false;
    updatePlayButton();
    startAnimation();
    hideScrollingTitle();
}

function resumePlayback() {
    Tone.Transport.start();
    state.isPlaying = true;
    state.isPaused = false;
    updatePlayButton();
    startAnimation();
    hideScrollingTitle();
}

function pausePlayback() {
    Tone.Transport.pause();
    state.isPlaying = false;
    state.isPaused = true;
    updatePlayButton();
    stopAnimation();
    showScrollingTitle();
}

function stopPlayback() {
    Tone.Transport.stop();
    Tone.Transport.cancel(); // 清除所有调度事件
    state.isPlaying = false;
    state.isPaused = false;
    updatePlayButton();
    stopAnimation();
    disposeSynth();
    showScrollingTitle();
    // 重置进度
    elements.progressFill.style.width = '0%';
    elements.timeCurrent.textContent = '00:00';
}

function seekTo(seconds) {
    if (state.totalDuration <= 0) return;

    const wasPlaying = state.isPlaying;
    // 记录暂停位置
    const resumePosition = seconds;

    // 停止当前播放
    Tone.Transport.stop();
    Tone.Transport.cancel();
    disposeSynth();

    // 重新创建合成器
    state.toneSynth = new Tone.PolySynth(Tone.Synth, {
        oscillator: { type: 'triangle' },
        envelope: {
            attack: 0.02,
            decay: 0.1,
            sustain: 0.3,
            release: 0.5,
        },
    }).toDestination();

    state.toneReverb = new Tone.Reverb({
        decay: 1.5,
        wet: 0.3,
    }).toDestination();
    state.toneSynth.connect(state.toneReverb);

    Tone.Transport.bpm.value = 60;

    const speed = parseFloat(elements.speedSelect.value);
    const timeScale = 1 / speed;

    // 只调度 seek 位置之后的音符
    state.notes.forEach(({ note, duration, time }) => {
        const noteEnd = time + duration;
        if (noteEnd <= seconds) return; // 音符已结束，跳过

        // 计算在 Transport 时间轴上的调度时间
        let scheduledTime = (time - seconds) * timeScale;
        if (scheduledTime < 0) scheduledTime = 0;

        // 如果音符在 seek 时间点已经开始，缩短其剩余时长
        let remainingDuration = duration;
        if (time < seconds) {
            remainingDuration = noteEnd - seconds;
        }
        const noteDuration = remainingDuration * timeScale * 0.9;

        Tone.Transport.schedule((t) => {
            state.toneSynth.triggerAttackRelease(note, noteDuration, t);
        }, scheduledTime);
    });

    // 调度结束事件
    const remainingReal = state.totalDuration - seconds;
    const adjustedRemaining = remainingReal * timeScale;
    Tone.Transport.schedule(() => {
        stopPlayback();
    }, adjustedRemaining + 0.1);

    state._speed = speed;
    state._timeScale = timeScale;

    if (wasPlaying) {
        Tone.Transport.start();
        state.isPlaying = true;
        state.isPaused = false;
        updatePlayButton();
        startAnimation();
        hideScrollingTitle();
    } else {
        // 暂停状态：更新进度显示但不播放
        state.isPlaying = false;
        state.isPaused = true;
        updatePlayButton();
        showScrollingTitle();
        const progress = (seconds / state.totalDuration) * 100;
        elements.progressFill.style.width = progress + '%';
        elements.timeCurrent.textContent = formatTime(seconds);
    }
}

function disposeSynth() {
    if (state.toneSynth) {
        state.toneSynth.dispose();
        state.toneSynth = null;
    }
    if (state.toneReverb) {
        state.toneReverb.dispose();
        state.toneReverb = null;
    }
}

// ==== 进度动画 ====
function startAnimation() {
    stopAnimation();
    function tick() {
        if (!state.isPlaying) {
            state.animationId = null;
            return;
        }
        // Transport.seconds 是经过 timeScale 缩放的时间，换算回真实时间
        const realTime = Tone.Transport.seconds * (state._speed || 1);
        if (state.totalDuration > 0) {
            const progress = Math.min(100, (realTime / state.totalDuration) * 100);
            elements.progressFill.style.width = progress + '%';
            elements.timeCurrent.textContent = formatTime(realTime);
        }
        drawVisualizerPlaying(realTime);
        state.animationId = requestAnimationFrame(tick);
    }
    state.animationId = requestAnimationFrame(tick);
}

function stopAnimation() {
    if (state.animationId) {
        cancelAnimationFrame(state.animationId);
        state.animationId = null;
    }
}

// ==== 可视化 ====
function drawVisualizerIdle() {
    const canvas = elements.visualizerCanvas;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    const dpr = window.devicePixelRatio || 1;
    const w = canvas.width / dpr;
    const h = canvas.height / dpr;
    ctx.clearRect(0, 0, w, h);
    // 绘制一条平线
    ctx.strokeStyle = 'rgba(157, 107, 255, 0.2)';
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.moveTo(0, h / 2);
    ctx.lineTo(w, h / 2);
    ctx.stroke();
}

function drawVisualizerPlaying(currentTime) {
    const canvas = elements.visualizerCanvas;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    const dpr = window.devicePixelRatio || 1;
    const w = canvas.width / dpr;
    const h = canvas.height / dpr;

    // 如果 canvas 尺寸为 0（模态框可能还在过渡中），跳过绘制
    if (w <= 0 || h <= 0) return;

    ctx.clearRect(0, 0, w, h);

    if (state.notes.length === 0) return;

    // 绘制波形背景参考线
    ctx.strokeStyle = 'rgba(157, 107, 255, 0.08)';
    ctx.lineWidth = 1;
    ctx.setLineDash([4, 8]);
    ctx.beginPath();
    ctx.moveTo(0, h / 2);
    ctx.lineTo(w, h / 2);
    ctx.stroke();
    ctx.setLineDash([]);

    // 计算可见窗口（前后各 2 秒）
    const windowStart = Math.max(0, currentTime - 2);
    const windowEnd = Math.min(state.totalDuration, currentTime + 2);
    const windowDuration = windowEnd - windowStart;

    if (windowDuration <= 0) return;

    // 为每个音符绘制条形
    const barWidth = Math.max(2, w / 80);
    const barGap = 1;

    // 采样音符：按时间槽分组
    const timeSlots = new Map();
    const slotSize = windowDuration / 60;

    state.notes.forEach(note => {
        if (note.time + note.duration < windowStart || note.time > windowEnd) return;
        const slot = Math.floor((note.time - windowStart) / slotSize);
        if (!timeSlots.has(slot)) timeSlots.set(slot, []);
        timeSlots.get(slot).push(note);
    });

    // 绘制条形
    timeSlots.forEach((notes, slot) => {
        const x = (slot / 60) * w;
        // 计算该时间槽的平均音高
        const avgNote = notes.reduce((sum, n) => {
            const noteNum = noteToNumber(n.note);
            return sum + noteNum;
        }, 0) / notes.length;

        // 音高映射到高度 (C3=48, C6=84)
        const normalizedPitch = (noteToNumber(notes[0].note) - 48) / 36;
        const barHeight = h * 0.15 + normalizedPitch * h * 0.7;

        // 根据距离当前位置的远近调整透明度
        const distFromCenter = Math.abs((slot / 60) * windowDuration - (currentTime - windowStart));
        const alpha = Math.max(0.15, 1 - distFromCenter / (windowDuration / 2));

        // 当前播放位置附近的音符高亮
        const isNearCurrent = distFromCenter < 0.3;
        const hue = isNearCurrent ? 270 : 260;
        const saturation = isNearCurrent ? '80%' : '60%';
        const lightness = isNearCurrent ? '65%' : '50%';

        ctx.fillStyle = `hsla(${hue}, ${saturation}, ${lightness}, ${alpha})`;
        ctx.fillRect(x, h / 2 - barHeight / 2, barWidth, barHeight);
    });

    // 绘制当前位置指示线
    const indicatorX = ((currentTime - windowStart) / windowDuration) * w;
    ctx.strokeStyle = 'rgba(157, 107, 255, 0.8)';
    ctx.lineWidth = 2;
    ctx.shadowColor = 'rgba(157, 107, 255, 0.5)';
    ctx.shadowBlur = 8;
    ctx.beginPath();
    ctx.moveTo(indicatorX, 0);
    ctx.lineTo(indicatorX, h);
    ctx.stroke();
    ctx.shadowBlur = 0;
}

function noteToNumber(note) {
    const notes = { C: 0, D: 2, E: 4, F: 5, G: 7, A: 9, B: 11 };
    const match = note.match(/^([A-Ga-g])([#b]?)(\d+)$/);
    if (!match) return 60;
    let base = notes[match[1].toUpperCase()] || 0;
    if (match[2] === '#') base++;
    if (match[2] === 'b') base--;
    return base + (parseInt(match[3]) + 1) * 12;
}

// ==== 滚动标题 ====
function showScrollingTitle() {
    if (elements.titleScroll) {
        elements.titleScroll.style.display = 'flex';
    }
}

function hideScrollingTitle() {
    if (elements.titleScroll) {
        elements.titleScroll.style.display = 'none';
    }
}

// ==== 下载 ====
function downloadScore() {
    if (!state.abcNotation) return;
    const blob = new Blob([state.abcNotation], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    const safeName = state.title.replace(/[\\/:*?"<>|]/g, '_');
    a.download = `${safeName}.abc`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
}

// ==== 音符解析 ====
function parseABCNotes(abcNotation) {
    const notes = [];

    const lengthMatch = abcNotation.match(/^L:\s*(\d+)\/(\d+)/m);
    let defaultDuration = 0.125;
    if (lengthMatch) {
        defaultDuration = parseInt(lengthMatch[1]) / parseInt(lengthMatch[2]);
    }

    const tempoMatch = abcNotation.match(/^Q:\s*(\d+)\/(\d+)=(\d+)/m);
    let bpm = 120;
    let beatDuration = 0.25;
    if (tempoMatch) {
        beatDuration = parseInt(tempoMatch[1]) / parseInt(tempoMatch[2]);
        bpm = parseInt(tempoMatch[3]);
    }

    const secondsPerBeat = 60 / bpm;
    const defaultSeconds = defaultDuration / beatDuration * secondsPerBeat;

    const keyIndex = abcNotation.search(/^K:.*$/m);
    if (keyIndex === -1) return notes;

    const musicBody = abcNotation.substring(keyIndex).replace(/^K:.*$/m, '').trim();

    const noteRegex = /([\^=_]*)([a-gA-G])([,']*)(\d*)(\/?\d*)/g;
    let match;
    let currentTime = 0;

    while ((match = noteRegex.exec(musicBody)) !== null) {
        const accidental = match[1];
        const noteName = match[2];
        const octave = match[3];
        const durationNum = match[4];
        const durationFrac = match[5];

        let duration = defaultSeconds;
        if (durationNum) {
            duration = defaultSeconds * parseInt(durationNum);
        }
        if (durationFrac && durationFrac.startsWith('/')) {
            const fracVal = parseInt(durationFrac.substring(1));
            if (fracVal > 0) duration = defaultSeconds / fracVal;
        }

        let fullNote = noteName;
        if (noteName === noteName.toLowerCase()) {
            fullNote += '4';
        } else {
            fullNote += '3';
        }
        if (octave.includes("'")) fullNote = fullNote.replace(/\d/, m => parseInt(m) + 1);
        if (octave.includes(',')) fullNote = fullNote.replace(/\d/, m => parseInt(m) - 1);

        if (accidental.includes('^')) fullNote += '#';
        if (accidental.includes('_')) fullNote += 'b';

        notes.push({
            note: fullNote,
            duration: duration,
            time: currentTime,
        });

        currentTime += duration;
    }

    return notes;
}

// ==== 播放按钮 ====
function updatePlayButton() {
    const icon = elements.btnPlay.querySelector('i');
    if (state.isPlaying) {
        icon.className = 'fas fa-pause';
        elements.btnPlay.title = '暂停';
    } else {
        icon.className = 'fas fa-play';
        elements.btnPlay.title = '播放';
    }
}

// ==== 模态框 ====
function showMusicModal() {
    elements.overlay.classList.add('visible');
    elements.historyPanel.style.display = 'none';
    // 模态框从隐藏变为可见后，重新调整 Canvas 尺寸
    // 使用 requestAnimationFrame 等待 CSS transition 完成后再调整
    requestAnimationFrame(() => {
        resizeCanvas();
        drawVisualizerIdle();
    });
}

function hideMusicModal() {
    stopPlayback();
    elements.overlay.classList.remove('visible');
}

// ==== 历史记录 ====
function toggleHistoryPanel() {
    const isVisible = elements.historyPanel.style.display !== 'none';
    elements.historyPanel.style.display = isVisible ? 'none' : 'block';
    if (!isVisible) renderHistoryList();
}

function addToHistory(title, abcNotation) {
    state.history.unshift({ title, abcNotation, time: new Date().toLocaleTimeString() });
    if (state.history.length > 20) state.history.pop();
}

function renderHistoryList() {
    if (state.history.length === 0) {
        elements.historyList.innerHTML = '<div class="music-history-empty">暂无创作历史</div>';
        return;
    }

    elements.historyList.innerHTML = state.history.map((item, index) => `
        <div class="music-history-item" data-index="${index}">
            <span class="music-history-title">${escapeHtmlSafe(item.title)}</span>
            <span class="music-history-time">${item.time}</span>
        </div>
    `).join('');

    elements.historyList.querySelectorAll('.music-history-item').forEach(item => {
        item.addEventListener('click', () => {
            const index = parseInt(item.dataset.index);
            const record = state.history[index];
            if (record) {
                elements.historyPanel.style.display = 'none';
                renderMusicScore(record.abcNotation);
            }
        });
    });
}

// ==== 工具函数 ====
function formatTime(seconds) {
    if (!isFinite(seconds) || seconds < 0) return '00:00';
    const m = Math.floor(seconds / 60);
    const s = Math.floor(seconds % 60);
    return String(m).padStart(2, '0') + ':' + String(s).padStart(2, '0');
}

function escapeHtmlSafe(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

function showError(message) {
    // 可以用 toast 显示，这里简单记录
    console.error('[音乐渲染器]', message);
}

// ==== 销毁 ====
export function destroyMusicRenderer() {
    stopPlayback();
    disposeSynth();
    if (elements.overlay) {
        elements.overlay.remove();
        elements = {};
    }
    state.history = [];
    state.abcNotation = '';
    state.notes = [];
}