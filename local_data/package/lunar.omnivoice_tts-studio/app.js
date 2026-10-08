// ============================================================
// 『 星月智能 』OmniVoice 语音合成工作台
// 左右双栏 · 属性设计 / 声音克隆 · 同源 /omnivoice/* 端点
// ============================================================

// ===== DOM 引用聚合 =====
const el = {
    presetChips: document.getElementById('preset-chips'),
    textInput: document.getElementById('text-input'),
    charCount: document.getElementById('char-count'),
    langSelect: document.getElementById('lang-select'),
    seedInput: document.getElementById('seed-input'),
    postprocToggle: document.getElementById('postproc-toggle'),
    engineDot: document.getElementById('engine-dot'),
    engineText: document.getElementById('engine-text'),
    synthBtn: document.getElementById('synth-btn'),
    playPauseBtn: document.getElementById('play-pause-btn'),
    iconPlay: document.querySelector('.icon-play'),
    iconPause: document.querySelector('.icon-pause'),
    progressBar: document.getElementById('progress-bar'),
    timeDisplay: document.getElementById('time-display'),
    downloadBtn: document.getElementById('download-btn'),
    waveform: document.getElementById('waveform'),
    waveHint: document.getElementById('wave-hint'),
    audioPlayer: document.getElementById('audio-player'),
    tabBar: document.getElementById('tab-bar'),
    instructClearBtn: document.getElementById('instruct-clear-btn'),
    instructPreviewText: document.getElementById('instruct-preview-text'),
    uploadArea: document.getElementById('upload-area'),
    audioUpload: document.getElementById('audio-upload'),
    uploadContent: document.getElementById('upload-content'),
    fileInfo: document.getElementById('file-info'),
    fileName: document.getElementById('file-name'),
    fileRemoveBtn: document.getElementById('file-remove-btn'),
    refTextInput: document.getElementById('ref-text-input'),
    cloneSynthBtn: document.getElementById('clone-synth-btn'),
    aboutVersion: document.getElementById('about-version'),
    aboutLangCount: document.getElementById('about-lang-count'),
    langCloud: document.getElementById('lang-cloud'),
    statusMessage: document.getElementById('status-message'),
};

// ===== 状态 =====
const state = {
    synthesizing: false,
    currentAudioBase64: null,
    uploadedRefAudioPath: null,
    health: null,
};

// ===== 端点（琉璃内嵌引擎，同源调用） =====
function apiUrl(path) { return '/omnivoice/' + path; }

// ===== Toast 提示 =====
let toastTimer = null;
function showStatus(msg, type = 'info') {
    el.statusMessage.textContent = msg;
    el.statusMessage.className = 'status-toast visible ' + type;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { el.statusMessage.classList.remove('visible'); }, 3200);
}

// ===== 初始化 =====
document.addEventListener('DOMContentLoaded', () => {
    // 深色模式（localStorage 持久化，键名 ovTheme）
    const dark = localStorage.getItem('ovTheme') === 'dark';
    document.body.classList.toggle('dark-mode', dark);

    bindEvents();
    refreshHealth();
});

function bindEvents() {
    // 预设文本
    el.presetChips.addEventListener('click', (e) => {
        const chip = e.target.closest('.chip');
        if (!chip) return;
        el.presetChips.querySelectorAll('.chip').forEach(c => c.classList.remove('active'));
        chip.classList.add('active');
        el.textInput.value = chip.dataset.text;
        updateCharCount();
    });

    el.textInput.addEventListener('input', updateCharCount);

    // 主题切换快捷键：Ctrl+D
    document.addEventListener('keydown', (e) => {
        if (e.ctrlKey && e.key.toLowerCase() === 'd') {
            e.preventDefault();
            const dark = document.body.classList.toggle('dark-mode');
            localStorage.setItem('ovTheme', dark ? 'dark' : 'light');
        }
    });

    // 属性设计 chips（单选组：同组内切换）
    document.querySelectorAll('.attr-chips').forEach(group => {
        group.addEventListener('click', (e) => {
            const chip = e.target.closest('.chip');
            if (!chip) return;
            const wasActive = chip.classList.contains('active');
            group.querySelectorAll('.chip').forEach(c => c.classList.remove('active'));
            if (!wasActive) chip.classList.add('active');
            updateInstructPreview();
        });
    });
    el.instructClearBtn.addEventListener('click', () => {
        document.querySelectorAll('.attr-chips .chip.active').forEach(c => c.classList.remove('active'));
        updateInstructPreview();
    });

    // 标签页切换
    el.tabBar.addEventListener('click', (e) => {
        const tab = e.target.closest('.tab');
        if (!tab) return;
        el.tabBar.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
        tab.classList.add('active');
        document.querySelectorAll('.tab-panel').forEach(p => p.classList.remove('active'));
        document.getElementById('panel-' + tab.dataset.tab).classList.add('active');
    });

    // 合成按钮（两处入口共用）
    el.synthBtn.addEventListener('click', () => synthesize(false));
    el.cloneSynthBtn.addEventListener('click', () => synthesize(true));

    // 播放器
    el.playPauseBtn.addEventListener('click', togglePlay);
    el.audioPlayer.addEventListener('ended', resetPlayIcon);
    el.audioPlayer.addEventListener('timeupdate', updateProgress);
    el.progressBar.addEventListener('input', () => {
        if (el.audioPlayer.duration) {
            el.audioPlayer.currentTime = (el.progressBar.value / 100) * el.audioPlayer.duration;
        }
    });
    el.downloadBtn.addEventListener('click', downloadWav);

    // 参考音频上传
    el.uploadArea.addEventListener('click', (e) => {
        if (!e.target.closest('#file-remove-btn')) el.audioUpload.click();
    });
    el.audioUpload.addEventListener('change', () => {
        if (el.audioUpload.files.length) uploadRefAudio(el.audioUpload.files[0]);
    });
    ['dragover', 'dragleave', 'drop'].forEach(evt => {
        el.uploadArea.addEventListener(evt, (e) => {
            e.preventDefault();
            el.uploadArea.classList.toggle('dragover', evt === 'dragover');
            if (evt === 'drop' && e.dataTransfer.files.length) {
                el.audioUpload.files = e.dataTransfer.files;
                uploadRefAudio(e.dataTransfer.files[0]);
            }
        });
    });
    el.fileRemoveBtn.addEventListener('click', removeUploadedFile);
}

// ===== 字数 / instruct 预览 =====
function updateCharCount() {
    el.charCount.textContent = String(el.textInput.value.length);
}

function updateInstructPreview() {
    const parts = [];
    document.querySelectorAll('.attr-group').forEach(group => {
        const active = group.querySelector('.chip.active');
        if (active) parts.push(active.dataset.value);
    });
    // 引擎按逗号切分 instruct 条目（英文逗号或中文全角逗号），必须以 ", " 连接
    el.instructPreviewText.textContent = parts.length ? parts.join(', ') : '（空 = 自动音色）';
}

// ===== 引擎健康检测 =====
async function refreshHealth() {
    try {
        const resp = await fetch(apiUrl('health'));
        const data = await resp.json();
        state.health = data;
        if (data.loaded) {
            el.engineDot.className = 'fas fa-circle ready';
            el.engineText.textContent = `引擎就绪${data.version ? ' · ' + data.version : ''}`;
        } else {
            el.engineDot.className = 'fas fa-circle';
            el.engineText.textContent = '引擎未加载（首次合成时将自动初始化，约需数秒~数十秒）';
        }
        el.aboutVersion.textContent = data.version || '—';
        el.aboutLangCount.textContent = data.languages || '—';
        await loadLanguages();
        el.synthBtn.disabled = false;
        el.cloneSynthBtn.disabled = !state.uploadedRefAudioPath;
    } catch (err) {
        el.engineDot.className = 'fas fa-circle error';
        el.engineText.textContent = '引擎不可达：' + err.message;
        showStatus('无法连接 OmniVoice 引擎，请确认琉璃服务已启动', 'error');
    }
}

// ===== 语言列表 =====
async function loadLanguages() {
    try {
        const resp = await fetch(apiUrl('langs'));
        const data = await resp.json();
        if (!data.success || !Array.isArray(data.languages)) return;

        // 语言下拉：auto + 全量（名称显示，id 传参）
        el.langSelect.innerHTML = '<option value="">自动识别</option>';
        for (const lang of data.languages) {
            const opt = document.createElement('option');
            opt.value = lang.id;
            opt.textContent = `${lang.name} (${lang.id})`;
            el.langSelect.appendChild(opt);
        }
        // 默认选中中文（存在时）
        const zh = data.languages.find(l => l.id === 'zh' || /chinese/i.test(l.name));
        if (zh) el.langSelect.value = zh.id;

        // 语言云
        el.langCloud.innerHTML = '';
        for (const lang of data.languages) {
            const tag = document.createElement('span');
            tag.className = 'lang-tag';
            tag.innerHTML = `<i class="fas fa-globe"></i>`;
            tag.appendChild(document.createTextNode(`${lang.name} (${lang.id})`));
            el.langCloud.appendChild(tag);
        }
    } catch (err) {
        // 语言列表加载失败不阻塞主流程
        console.warn('语言列表加载失败:', err);
    }
}

// ===== 参考音频上传 =====
async function uploadRefAudio(file) {
    const ext = '.' + (file.name.split('.').pop() || '').toLowerCase();
    if (ext !== '.wav') {
        showStatus('仅支持 WAV 格式参考音频', 'error');
        el.audioUpload.value = '';
        return;
    }
    try {
        showStatus('正在上传参考音频…', 'info');
        const arrayBuffer = await file.arrayBuffer();
        const hashBuffer = await crypto.subtle.digest('SHA-256', arrayBuffer);
        const hashPrefix = Array.from(new Uint8Array(hashBuffer)).slice(0, 4)
            .map(b => b.toString(16).padStart(2, '0')).join('');
        const saveName = 'audios/' + hashPrefix + ext;
        const encoded = btoa(unescape(encodeURIComponent(saveName)));

        const resp = await fetch('/file/write', {
            method: 'POST',
            headers: { 'X-File-Name': encoded, 'X-Overwrite': 'true' },
            body: arrayBuffer,
        });
        if (!resp.ok) {
            showStatus('上传失败: ' + (await resp.text()), 'error');
            return;
        }
        const result = await resp.json();
        state.uploadedRefAudioPath = result.path;
        el.fileName.textContent = file.name;
        el.fileInfo.classList.remove('hidden');
        el.uploadContent.classList.add('hidden');
        el.cloneSynthBtn.disabled = false;
        showStatus('参考音频已就绪', 'success');
    } catch (err) {
        showStatus('上传失败: ' + err.message, 'error');
    }
}

function removeUploadedFile(e) {
    e.stopPropagation();
    state.uploadedRefAudioPath = null;
    el.audioUpload.value = '';
    el.fileInfo.classList.add('hidden');
    el.uploadContent.classList.remove('hidden');
    el.cloneSynthBtn.disabled = true;
}

// ===== 合成 =====
async function synthesize(cloneMode) {
    if (state.synthesizing) {
        showStatus('正在合成中，请稍候…', 'info');
        return;
    }
    const text = el.textInput.value.trim();
    if (!text) {
        showStatus('请输入要合成的文本', 'error');
        el.textInput.focus();
        return;
    }

    const body = { text, lang: el.langSelect.value || undefined };

    if (cloneMode) {
        if (!state.uploadedRefAudioPath) {
            showStatus('请先上传参考音频', 'error');
            return;
        }
        body.ref_audio = state.uploadedRefAudioPath;
        body.ref_text = el.refTextInput.value.trim();
        if (!body.ref_text) {
            showStatus('声音克隆需要填写参考音频的转写文本', 'error');
            el.refTextInput.focus();
            return;
        }
    } else {
        const instruct = el.instructPreviewText.textContent.trim();
        if (instruct && !instruct.startsWith('（')) body.instruct = instruct;
    }

    const seed = parseInt(el.seedInput.value, 10);
    if (seed > 0) body.seed = seed;
    body.disable_postproc = !el.postprocToggle.checked;

    state.synthesizing = true;
    el.synthBtn.disabled = true;
    el.cloneSynthBtn.disabled = true;
    el.synthBtn.querySelector('span').textContent = '合成中…';
    el.waveHint.textContent = '正在合成（首次运行需预热，请耐心等待）…';
    el.waveHint.classList.remove('hidden');
    startWaveAnimation();

    try {
        const resp = await fetch(apiUrl('tts'), {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        const data = await resp.json();
        if (!data.success) throw new Error(data.error || '合成失败');

        stopWaveAnimation();
        playAudio(data.audio);
        showStatus(`合成完成（${data.voice || '自动音色'}）`, 'success');
        refreshHealth();
    } catch (err) {
        stopWaveAnimation();
        el.waveHint.textContent = '合成结果将显示在这里';
        showStatus('合成失败: ' + err.message, 'error');
    } finally {
        state.synthesizing = false;
        el.synthBtn.disabled = false;
        el.synthBtn.querySelector('span').textContent = '开始合成';
        el.cloneSynthBtn.disabled = !state.uploadedRefAudioPath;
    }
}

// ===== 播放器 =====
function playAudio(base64) {
    state.currentAudioBase64 = base64;
    const blob = base64ToBlob(base64, 'audio/wav');
    el.audioPlayer.src = URL.createObjectURL(blob);
    el.waveHint.classList.add('hidden');
    buildWaveBars();
    el.audioPlayer.play();
    setPlaying(true);
    el.playPauseBtn.disabled = false;
    el.progressBar.disabled = false;
    el.downloadBtn.disabled = false;
}

function base64ToBlob(base64, mime) {
    const bin = atob(base64);
    const bytes = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
    return new Blob([bytes], { type: mime });
}

function togglePlay() {
    if (!el.audioPlayer.src) return;
    if (el.audioPlayer.paused) { el.audioPlayer.play(); setPlaying(true); }
    else { el.audioPlayer.pause(); setPlaying(false); }
}

function setPlaying(playing) {
    el.iconPlay.classList.toggle('hidden', playing);
    el.iconPause.classList.toggle('hidden', !playing);
}

function resetPlayIcon() {
    setPlaying(false);
    el.progressBar.value = 0;
    updateProgress();
}

function fmtTime(sec) {
    if (!isFinite(sec)) return '0:00';
    const m = Math.floor(sec / 60), s = Math.floor(sec % 60);
    return `${m}:${String(s).padStart(2, '0')}`;
}

function updateProgress() {
    if (!el.audioPlayer.duration) return;
    const pct = (el.audioPlayer.currentTime / el.audioPlayer.duration) * 100;
    el.progressBar.value = pct;
    el.timeDisplay.textContent = `${fmtTime(el.audioPlayer.currentTime)} / ${fmtTime(el.audioPlayer.duration)}`;
}

function downloadWav() {
    if (!state.currentAudioBase64) return;
    const blob = base64ToBlob(state.currentAudioBase64, 'audio/wav');
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `omnivoice_${Date.now()}.wav`;
    a.click();
    URL.revokeObjectURL(url);
}

// ===== 波形动画（装饰性） =====
let waveTimer = null;
let waveBars = [];

function buildWaveBars() {
    el.waveform.querySelectorAll('.wave-bar').forEach(b => b.remove());
    waveBars = [];
    const count = Math.min(60, Math.floor(el.waveform.clientWidth / 10));
    for (let i = 0; i < count; i++) {
        const bar = document.createElement('div');
        bar.className = 'wave-bar';
        bar.style.height = (6 + Math.random() * 18) + 'px';
        el.waveform.appendChild(bar);
        waveBars.push(bar);
    }
}

function startWaveAnimation() {
    if (!waveBars.length) buildWaveBars();
    waveTimer = setInterval(() => {
        waveBars.forEach(bar => {
            bar.style.height = (6 + Math.random() * (el.waveform.clientHeight - 20)) + 'px';
        });
    }, 130);
}

function stopWaveAnimation() {
    clearInterval(waveTimer);
    waveTimer = null;
    waveBars.forEach((bar, i) => {
        const h = 8 + Math.abs(Math.sin(i * 0.55)) * (el.waveform.clientHeight - 20);
        bar.style.height = Math.max(8, h) + 'px';
    });
}
