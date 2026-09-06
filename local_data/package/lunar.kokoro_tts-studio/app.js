// ============================================================
// 『 星月智能 』Kokoro 语音合成工作台
// 左右双栏 · 点击音色卡片即合成 · 右侧标签页
// ============================================================

// ==== DOM 引用聚合 ====
const elements = {
    apiBaseInput: document.getElementById('api-base-input'),
    connectBtn: document.getElementById('connect-btn'),
    serviceStatus: document.getElementById('service-status'),
    presetChips: document.getElementById('preset-chips'),
    textInput: document.getElementById('text-input'),
    charCount: document.getElementById('char-count'),
    speedInput: document.getElementById('speed-input'),
    speedValue: document.getElementById('speed-value'),
    langSelect: document.getElementById('lang-select'),
    downloadBtn: document.getElementById('download-btn'),
    audioPlayer: document.getElementById('audio-player'),
    playPauseBtn: document.getElementById('play-pause-btn'),
    iconPlay: document.querySelector('.icon-play'),
    iconPause: document.querySelector('.icon-pause'),
    progressBar: document.getElementById('progress-bar'),
    timeDisplay: document.getElementById('time-display'),
    waveform: document.getElementById('waveform'),
    currentVoiceLabel: document.getElementById('current-voice-label'),
    statusMessage: document.getElementById('status-message'),
    // 均衡器
    eqLowSlider: document.getElementById('eq-low-slider'),
    eqLowMode: document.getElementById('eq-low-mode'),
    eqLowMulti: document.getElementById('eq-low-multi'),
    eqLowValue: document.getElementById('eq-low-value'),
    eqMidSlider: document.getElementById('eq-mid-slider'),
    eqMidMode: document.getElementById('eq-mid-mode'),
    eqMidMulti: document.getElementById('eq-mid-multi'),
    eqMidValue: document.getElementById('eq-mid-value'),
    eqHighSlider: document.getElementById('eq-high-slider'),
    eqHighMode: document.getElementById('eq-high-mode'),
    eqHighMulti: document.getElementById('eq-high-multi'),
    eqHighValue: document.getElementById('eq-high-value'),
    eqResetBtn: document.getElementById('eq-reset-btn'),
    eqVisLow: document.getElementById('eq-vis-low'),
    eqVisMid: document.getElementById('eq-vis-mid'),
    eqVisHigh: document.getElementById('eq-vis-high'),
    // 标签页
    tabBar: document.getElementById('tab-bar'),
    voiceGrids: {
        zf: document.getElementById('voice-grid-zf'),
        zm: document.getElementById('voice-grid-zm'),
        af: document.getElementById('voice-grid-af'),
        bf: document.getElementById('voice-grid-bf'),
    },
    tabCounts: {
        zf: document.getElementById('count-zf'),
        zm: document.getElementById('count-zm'),
        af: document.getElementById('count-af'),
        bf: document.getElementById('count-bf'),
    },
    // 读音词典
    dictSearchInput: document.getElementById('dict-search-input'),
    dictCount: document.getElementById('dict-count'),
    dictWordInput: document.getElementById('dict-word-input'),
    dictPinyinInput: document.getElementById('dict-pinyin-input'),
    dictGuessBtn: document.getElementById('dict-guess-btn'),
    dictAddBtn: document.getElementById('dict-add-btn'),
    dictList: document.getElementById('dict-list'),
};

// ==== 状态 ====
const state = {
    apiBase: localStorage.getItem('kokoro_api_base') || 'http://127.0.0.1:36789',
    voices: [],
    selectedVoice: '',
    currentAudioBase64: null,
    synthesizing: false,
    waveTimer: null,
    playing: false,
    editingWord: '',          // 读音词典当前正在编辑的词语（空 = 新增模式）
    dictEntries: null,        // 读音词典全量条目（供搜索过滤）
    eqCtx: null,              // 均衡器 AudioContext
    eqLowFilter: null,
    eqMidFilter: null,
    eqHighFilter: null,
    eqSourceNode: null,
};

// ==== 常量 ====
const API_KEY = 'kokoro_api_base';
const SAMPLE_VOICE_TEXT = '你好，欢迎使用星月智能语音助手。';

// 音色分组（前缀 -> 标签页 key / 语言徽标）
const VOICE_GROUPS = [
    { key: 'zf', title: '中文女声', badge: 'zh', match: v => v.name.startsWith('zf') },
    { key: 'zm', title: '中文男声', badge: 'zh', match: v => v.name.startsWith('zm') },
    { key: 'af', title: '英文女声', badge: 'en', match: v => v.name.startsWith('af') },
    { key: 'bf', title: '英文男声', badge: 'en', match: v => v.name.startsWith('bf') },
];

// ============================================================
// 初始化
// ============================================================
function init() {
    elements.apiBaseInput.value = state.apiBase;
    bindEvents();
    elements.textInput.focus();
    updateCharCount();
    updateEQVisualizer();
    loadVoices();
}

function bindEvents() {
    // 连接服务
    elements.connectBtn.addEventListener('click', loadVoices);
    elements.apiBaseInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') loadVoices();
    });

    // 预设文本
    elements.presetChips.addEventListener('click', (e) => {
        const chip = e.target.closest('.chip');
        if (!chip) return;
        document.querySelectorAll('.chip').forEach(c => c.classList.remove('active'));
        chip.classList.add('active');
        elements.textInput.value = chip.dataset.text;
        updateCharCount();
    });

    // 调音器
    elements.textInput.addEventListener('input', updateCharCount);
    elements.speedInput.addEventListener('input', () => {
        elements.speedValue.textContent = elements.speedInput.value;
    });

    // 播放控制
    elements.playPauseBtn.addEventListener('click', togglePlayPause);
    elements.progressBar.addEventListener('input', handleSeek);
    elements.downloadBtn.addEventListener('click', downloadAudio);

    elements.audioPlayer.addEventListener('timeupdate', handleTimeUpdate);
    elements.audioPlayer.addEventListener('ended', handleAudioEnded);
    elements.audioPlayer.addEventListener('loadedmetadata', handleMetadataLoaded);
    elements.audioPlayer.addEventListener('play', () => setPlayingUI(true));
    elements.audioPlayer.addEventListener('pause', () => setPlayingUI(false));

    // 标签页切换
    elements.tabBar.addEventListener('click', (e) => {
        const tab = e.target.closest('.tab');
        if (!tab) return;
        switchTab(tab.dataset.tab);
    });

    // 读音词典
    elements.dictGuessBtn.addEventListener('click', guessDictReading);
    elements.dictWordInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') guessDictReading();
    });
    elements.dictAddBtn.addEventListener('click', addDictEntry);
    elements.dictPinyinInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') addDictEntry();
    });
    // 手动改动词语时退出编辑模式
    elements.dictWordInput.addEventListener('input', () => {
        if (state.editingWord && elements.dictWordInput.value.trim() !== state.editingWord) {
            state.editingWord = '';
            setAddBtnLabel(false);
        }
    });
    // 词典搜索：按词语实时过滤卡片
    elements.dictSearchInput.addEventListener('input', () => {
        renderDictList(state.dictEntries || {}, getDictFilter());
    });

    // 均衡器
    elements.eqLowSlider.addEventListener('input', () => { updateEQ(); updateEQVisualizer(); });
    elements.eqLowMode.addEventListener('change', () => { updateEQ(); updateEQVisualizer(); });
    elements.eqLowMulti.addEventListener('change', () => { updateEQ(); updateEQVisualizer(); });
    elements.eqMidSlider.addEventListener('input', () => { updateEQ(); updateEQVisualizer(); });
    elements.eqMidMode.addEventListener('change', () => { updateEQ(); updateEQVisualizer(); });
    elements.eqMidMulti.addEventListener('change', () => { updateEQ(); updateEQVisualizer(); });
    elements.eqHighSlider.addEventListener('input', () => { updateEQ(); updateEQVisualizer(); });
    elements.eqHighMode.addEventListener('change', () => { updateEQ(); updateEQVisualizer(); });
    elements.eqHighMulti.addEventListener('change', () => { updateEQ(); updateEQVisualizer(); });
    elements.eqResetBtn.addEventListener('click', () => { resetEQ(); updateEQVisualizer(); });
}

// ============================================================
// 标签页
// ============================================================
function switchTab(key) {
    document.querySelectorAll('.tab').forEach(t => {
        t.classList.toggle('active', t.dataset.tab === key);
    });
    document.querySelectorAll('.tab-panel').forEach(p => {
        p.classList.toggle('active', p.id === 'panel-' + key);
    });
    if (key === 'dict') loadDictEntries();
}

// ============================================================
// 服务连接与音色加载
// ============================================================
async function loadVoices() {
    const base = elements.apiBaseInput.value.trim().replace(/\/+$/, '');
    if (!base) {
        showStatus('请填写 Kokoro 服务地址', 'error');
        return;
    }
    state.apiBase = base;
    localStorage.setItem(API_KEY, base);

    setStatusUI('connecting');
    showStatus('正在连接 Kokoro 服务…', 'info');
    try {
        const res = await fetch(`${state.apiBase}/voices`);
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data = await res.json();
        if (!data.success) throw new Error(data.error || '获取音色失败');
        state.voices = data.voices || [];
        renderVoices();
        setStatusUI('online', state.voices.length);
        showStatus(`连接成功，共 ${state.voices.length} 种音色`, 'success');
    } catch (err) {
        setStatusUI('offline');
        showStatus('无法连接 Kokoro 服务: ' + err.message, 'error');
    }
}

function setStatusUI(mode, count) {
    const el = elements.serviceStatus;
    el.classList.remove('online', 'offline');
    if (mode === 'online') {
        el.classList.add('online');
        el.textContent = `已连接 · ${count} 音色`;
    } else if (mode === 'offline') {
        el.classList.add('offline');
        el.textContent = '连接失败';
    } else {
        el.textContent = '连接中…';
    }
}

// ============================================================
// 音色渲染（按标签页分组）
// ============================================================
function renderVoices() {
    for (const g of VOICE_GROUPS) {
        const list = state.voices.filter(g.match);
        const grid = elements.voiceGrids[g.key];
        grid.innerHTML = '';
        list.forEach(v => grid.appendChild(createVoiceCard(v.name, g.badge)));
        const tab = elements.tabCounts[g.key];
        tab.textContent = list.length;
    }
}

function createVoiceCard(name, badge) {
    const card = document.createElement('div');
    card.className = 'voice-card';
    card.dataset.voice = name;
    card.innerHTML = `
        <div class="voice-name">${name}</div>
        <div class="voice-meta">
            <span class="group-badge badge-${badge}">${badge === 'zh' ? '中文' : '英文'}</span>
            <button class="audition-btn" title="点击卡片试听">
                <i class="fas fa-play"></i>
            </button>
        </div>`;
    card.addEventListener('click', () => {
        selectVoice(name);
        auditionVoice(name);
    });
    return card;
}

function highlightVoiceCard(name) {
    document.querySelectorAll('.voice-card').forEach(c => {
        c.classList.toggle('selected', c.dataset.voice === name);
    });
}

// ============================================================
// 音色选择与试听
// ============================================================
function selectVoice(name) {
    state.selectedVoice = name;
    highlightVoiceCard(name);
}

function auditionVoice(name) {
    const text = elements.textInput.value.trim() || SAMPLE_VOICE_TEXT;
    synthesizeSpeech(name, text);
}

// ============================================================
// 合成（点击音色卡片触发）
// ============================================================
async function synthesizeSpeech(voiceOverride, textOverride) {
    const text = (textOverride || elements.textInput.value).trim();
    if (!text) {
        showStatus('请输入要合成的文本', 'error');
        elements.textInput.focus();
        return;
    }
    const voice = voiceOverride || state.selectedVoice;
    if (!voice) {
        showStatus('请先选择音色', 'error');
        return;
    }
    if (state.synthesizing) return;

    const speed = parseFloat(elements.speedInput.value);
    const lang = elements.langSelect.value;

    state.synthesizing = true;
    setVoiceBusy(voice, true);
    showStatus(`正在合成（${voice}）…`, 'info');
    startWaveform();

    try {
        const res = await fetch(`${state.apiBase}/tts`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ text, voice, speed, lang }),
        });
        const result = await res.json();
        if (result.success && result.audio) {
            loadAudio(result.audio);
            // 精简布局可能移除当前音色标签，做空值保护
            if (elements.currentVoiceLabel) {
                elements.currentVoiceLabel.textContent = voice;
            }
            elements.audioPlayer.play().catch(() => {});
            showStatus(`合成成功 · ${voice}`, 'success');
        } else {
            showStatus('合成失败: ' + (result.error || '服务端错误'), 'error');
        }
    } catch (err) {
        showStatus('请求失败: ' + err.message, 'error');
    } finally {
        state.synthesizing = false;
        setVoiceBusy(voice, false);
        stopWaveform();
    }
}

function setVoiceBusy(name, busy) {
    document.querySelectorAll('.voice-card').forEach(c => {
        if (c.dataset.voice === name) c.classList.toggle('busy', busy);
    });
}

function loadAudio(base64Audio) {
    state.currentAudioBase64 = base64Audio;
    elements.audioPlayer.src = 'data:audio/wav;base64,' + base64Audio;
    elements.audioPlayer.load();
    resetPlayerUI();
}

// ============================================================
// 播放控制
// ============================================================
function togglePlayPause() {
    if (!state.currentAudioBase64) {
        showStatus('还没有可播放的音频，点击音色卡片合成一段吧', 'error');
        return;
    }
    if (elements.audioPlayer.paused) {
        elements.audioPlayer.play().catch(err => showStatus('播放失败: ' + err.message, 'error'));
    } else {
        elements.audioPlayer.pause();
    }
}

function setPlayingUI(playing) {
    state.playing = playing;
    elements.iconPlay.classList.toggle('hidden', playing);
    elements.iconPause.classList.toggle('hidden', !playing);
    if (playing) startWaveform(); else stopWaveform();
}

function handleSeek() {
    if (!elements.audioPlayer.duration || !isFinite(elements.audioPlayer.duration)) return;
    const t = (elements.progressBar.value / 100) * elements.audioPlayer.duration;
    elements.audioPlayer.currentTime = t;
}

function handleTimeUpdate() {
    const d = elements.audioPlayer.duration;
    if (d && isFinite(d)) {
        elements.progressBar.value = (elements.audioPlayer.currentTime / d) * 100;
        elements.timeDisplay.textContent =
            formatTime(elements.audioPlayer.currentTime) + ' / ' + formatTime(d);
    }
}

function handleMetadataLoaded() {
    const d = elements.audioPlayer.duration;
    elements.timeDisplay.textContent = '0:00 / ' + (d && isFinite(d) ? formatTime(d) : '0:00');
    elements.progressBar.value = 0;
}

function handleAudioEnded() {
    resetPlayerUI();
    stopWaveform();
}

function resetPlayerUI() {
    elements.iconPlay.classList.remove('hidden');
    elements.iconPause.classList.add('hidden');
    elements.progressBar.value = 0;
    elements.timeDisplay.textContent = '0:00 / 0:00';
}

function formatTime(sec) {
    if (isNaN(sec) || !isFinite(sec)) return '0:00';
    const m = Math.floor(sec / 60);
    const s = Math.floor(sec % 60);
    return m + ':' + String(s).padStart(2, '0');
}

function downloadAudio() {
    if (!state.currentAudioBase64) {
        showStatus('没有可下载的音频', 'error');
        return;
    }
    try {
        const byteChars = atob(state.currentAudioBase64);
        const bytes = new Uint8Array(byteChars.length);
        for (let i = 0; i < byteChars.length; i++) bytes[i] = byteChars.charCodeAt(i);
        const blob = new Blob([bytes], { type: 'audio/wav' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = 'kokoro_' + state.selectedVoice + '_' + Date.now() + '.wav';
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
        showStatus('音频已保存', 'success');
    } catch (err) {
        showStatus('下载失败: ' + err.message, 'error');
    }
}

// ============================================================
// 波形动画
// ============================================================
function startWaveform() {
    if (state.waveTimer) return;
    const wave = elements.waveform;
    wave.innerHTML = '';
    const bars = [];
    for (let i = 0; i < 36; i++) {
        const bar = document.createElement('div');
        bar.className = 'waveform-bar';
        bar.style.height = '6px';
        bars.push(bar);
        wave.appendChild(bar);
    }
    state.waveTimer = setInterval(() => {
        bars.forEach((bar, i) => {
            const h = Math.max(4, Math.min(52, 8 + Math.sin(Date.now() * 0.006 + i * 0.4) * 12 + Math.random() * 16));
            bar.style.height = h + 'px';
        });
    }, 110);
}

function stopWaveform() {
    if (state.waveTimer) {
        clearInterval(state.waveTimer);
        state.waveTimer = null;
    }
    if (state.playing || state.synthesizing) return;
    elements.waveform.innerHTML = '<span class="waveform-idle">点击音色卡片开始合成</span>';
}

// ============================================================
// 工具函数
// ============================================================
function updateCharCount() {
    // 页面精简布局可能移除计数元素，做空值保护
    if (elements.charCount) {
        elements.charCount.textContent = elements.textInput.value.length;
    }
}

function showStatus(message, type) {
    const el = elements.statusMessage;
    el.textContent = message;
    el.className = 'status-toast ' + type + ' show';
    if (type !== 'info') {
        setTimeout(() => el.classList.remove('show'), 2600);
    }
}

// ============================================================
// 读音词典
// ============================================================
async function loadDictEntries() {
    elements.dictList.innerHTML = '<div class="dict-empty">加载中…</div>';
    try {
        const res = await fetch(`${state.apiBase}/dict`);
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data = await res.json();
        state.dictEntries = data.entries || {};
        renderDictList(state.dictEntries, getDictFilter());
    } catch (err) {
        state.dictEntries = null;
        elements.dictList.innerHTML = '<div class="dict-empty">读取词典失败: ' + escapeHtml(err.message) + '</div>';
    }
}

// 当前搜索过滤词（去除首尾空格，忽略大小写）
function getDictFilter() {
    return elements.dictSearchInput.value.trim().toLowerCase();
}

// 渲染词典卡片列表；query 非空时按词语包含关系过滤
function renderDictList(entries, query) {
    let keys = Object.keys(entries).sort();
    if (query) {
        keys = keys.filter(w => w.toLowerCase().includes(query));
    }
    elements.dictCount.textContent = keys.length + ' / ' + Object.keys(entries).length;
    if (Object.keys(entries).length === 0) {
        elements.dictList.innerHTML = '<div class="dict-empty">暂无自定义读音，添加一个试试（如：高兴 → gao1 xing4）</div>';
        return;
    }
    if (keys.length === 0) {
        elements.dictList.innerHTML = '<div class="dict-empty">没有匹配「' + escapeHtml(query) + '」的条目</div>';
        return;
    }
    elements.dictList.innerHTML = '';
    keys.forEach(word => {
        const card = document.createElement('div');
        card.className = 'dict-card';
        card.innerHTML = `
            <div class="dict-card-word">${escapeHtml(word)}</div>
            <div class="dict-card-pinyin">${escapeHtml(entries[word])}</div>
            <div class="dict-card-actions">
                <button class="dict-edit-btn" title="修改读音">
                    <i class="fas fa-pen"></i><span>修改</span>
                </button>
                <button class="dict-del-btn" title="删除">
                    <i class="fas fa-trash-can"></i><span>删除</span>
                </button>
            </div>`;
        card.querySelector('.dict-edit-btn').addEventListener('click', () => editDictEntry(word, entries[word]));
        card.querySelector('.dict-del-btn').addEventListener('click', () => deleteDictEntry(word));
        elements.dictList.appendChild(card);
    });
}

// 将词条载入输入框进入编辑模式，保存即为更新
function editDictEntry(word, pinyin) {
    state.editingWord = word;
    elements.dictWordInput.value = word;
    elements.dictPinyinInput.value = pinyin;
    setAddBtnLabel(true);
    elements.dictWordInput.scrollIntoView({ behavior: 'smooth', block: 'center' });
    elements.dictPinyinInput.focus();
}

function setAddBtnLabel(editing) {
    const icon = elements.dictAddBtn.querySelector('i');
    const span = elements.dictAddBtn.querySelector('span');
    if (editing) {
        icon.className = 'fas fa-pen';
        span.textContent = '更新';
    } else {
        icon.className = 'fas fa-plus';
        span.textContent = '添加';
    }
}

async function addDictEntry() {
    const word = elements.dictWordInput.value.trim();
    const pinyin = elements.dictPinyinInput.value.trim();
    if (!word || !pinyin) {
        showStatus('请填写词语与拼音', 'error');
        return;
    }
    try {
        const res = await fetch(`${state.apiBase}/dict`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ word, pinyin }),
        });
        const data = await res.json();
        if (!data.success) {
            showStatus('添加失败: ' + (data.error || '未知错误'), 'error');
            return;
        }
        showStatus(`已保存「${word}」→ ${data.pinyin}`, 'success');
        state.editingWord = '';
        setAddBtnLabel(false);
        elements.dictPinyinInput.value = '';
        await loadDictEntries();
    } catch (err) {
        showStatus('请求失败: ' + err.message, 'error');
    }
}

async function deleteDictEntry(word) {
    try {
        const res = await fetch(`${state.apiBase}/dict?word=${encodeURIComponent(word)}`, { method: 'DELETE' });
        const data = await res.json();
        if (!data.success) {
            showStatus('删除失败: ' + (data.error || '未知错误'), 'error');
            return;
        }
        showStatus(`已删除「${word}」`, 'success');
        await loadDictEntries();
    } catch (err) {
        showStatus('请求失败: ' + err.message, 'error');
    }
}

async function guessDictReading() {
    const word = elements.dictWordInput.value.trim();
    if (!word) {
        showStatus('请先输入要查询的词语', 'error');
        return;
    }
    try {
        const res = await fetch(`${state.apiBase}/dict/guess?word=${encodeURIComponent(word)}`);
        const data = await res.json();
        if (!data.success) {
            showStatus('查询失败: ' + (data.error || '未知错误'), 'error');
            return;
        }
        elements.dictPinyinInput.value = data.pinyin;
        // 查询成功后联动搜索框过滤，让对应词条在列表中可见
        elements.dictSearchInput.value = word;
        renderDictList(state.dictEntries || {}, getDictFilter());
        showStatus(
            data.in_dict
                ? `「${word}」当前使用词典读音: ${data.pinyin}`
                : `「${word}」当前默认读音: ${data.pinyin}（可修改后保存）`,
            'success'
        );
    } catch (err) {
        showStatus('请求失败: ' + err.message, 'error');
    }
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// ============================================================
// 音频均衡器（Web Audio 三频段：200Hz 低频 / 1kHz 中频 / 8kHz 高频）
// ============================================================
function initEQ() {
    if (state.eqCtx) return;

    const Ctx = window.AudioContext || window.webkitAudioContext;
    state.eqCtx = new Ctx();

    state.eqLowFilter = state.eqCtx.createBiquadFilter();
    state.eqLowFilter.type = 'lowshelf';
    state.eqLowFilter.frequency.value = 200;
    state.eqLowFilter.gain.value = 0;

    state.eqMidFilter = state.eqCtx.createBiquadFilter();
    state.eqMidFilter.type = 'peaking';
    state.eqMidFilter.frequency.value = 1000;
    state.eqMidFilter.Q.value = 1;
    state.eqMidFilter.gain.value = 0;

    state.eqHighFilter = state.eqCtx.createBiquadFilter();
    state.eqHighFilter.type = 'highshelf';
    state.eqHighFilter.frequency.value = 8000;
    state.eqHighFilter.gain.value = 0;

    state.eqLowFilter.connect(state.eqMidFilter);
    state.eqMidFilter.connect(state.eqHighFilter);
    state.eqHighFilter.connect(state.eqCtx.destination);

    if (state.eqCtx.state === 'suspended') {
        state.eqCtx.resume();
    }
}

// 将 <audio> 接入均衡器链路（MediaElementSource -> 低/中/高 滤波器 -> 输出）
function connectEQToAudioElement() {
    if (!state.eqCtx) initEQ();
    if (!state.eqSourceNode && elements.audioPlayer) {
        state.eqSourceNode = state.eqCtx.createMediaElementSource(elements.audioPlayer);
        state.eqSourceNode.connect(state.eqLowFilter);
    }
}

function getEQValue(slider, mode, multi) {
    const sliderVal = parseFloat(slider.value);
    const multiplier = parseInt(multi.value, 10);
    const value = sliderVal * multiplier;
    return mode.value === 'cut' ? -value : value;
}

function updateEQ() {
    const lowGain = getEQValue(elements.eqLowSlider, elements.eqLowMode, elements.eqLowMulti);
    const midGain = getEQValue(elements.eqMidSlider, elements.eqMidMode, elements.eqMidMulti);
    const highGain = getEQValue(elements.eqHighSlider, elements.eqHighMode, elements.eqHighMulti);

    elements.eqLowValue.textContent = (lowGain > 0 ? '+' : '') + lowGain.toFixed(1) + 'dB';
    elements.eqMidValue.textContent = (midGain > 0 ? '+' : '') + midGain.toFixed(1) + 'dB';
    elements.eqHighValue.textContent = (highGain > 0 ? '+' : '') + highGain.toFixed(1) + 'dB';

    if (state.eqCtx) {
        if (state.eqLowFilter) state.eqLowFilter.gain.value = lowGain;
        if (state.eqMidFilter) state.eqMidFilter.gain.value = midGain;
        if (state.eqHighFilter) state.eqHighFilter.gain.value = highGain;
    }
}

function updateEQVisualizer() {
    if (!elements.eqVisLow || !elements.eqVisMid || !elements.eqVisHigh) return;

    const lowGain = getEQValue(elements.eqLowSlider, elements.eqLowMode, elements.eqLowMulti);
    const midGain = getEQValue(elements.eqMidSlider, elements.eqMidMode, elements.eqMidMulti);
    const highGain = getEQValue(elements.eqHighSlider, elements.eqHighMode, elements.eqHighMulti);

    const baselineY = 237;
    const minH = 15;
    const maxH = 200;
    const lowH = Math.max(minH, Math.min(maxH, 20 + (lowGain / 36) * 180));
    const midH = Math.max(minH, Math.min(maxH, 20 + (midGain / 36) * 180));
    const highH = Math.max(minH, Math.min(maxH, 20 + (highGain / 36) * 180));

    elements.eqVisLow.setAttribute('y', baselineY - lowH);
    elements.eqVisLow.setAttribute('height', lowH);
    elements.eqVisMid.setAttribute('y', baselineY - midH);
    elements.eqVisMid.setAttribute('height', midH);
    elements.eqVisHigh.setAttribute('y', baselineY - highH);
    elements.eqVisHigh.setAttribute('height', highH);
}

function resetEQ() {
    elements.eqLowSlider.value = 0;
    elements.eqLowMode.value = 'boost';
    elements.eqLowMulti.value = '3';
    elements.eqMidSlider.value = 0;
    elements.eqMidMode.value = 'boost';
    elements.eqMidMulti.value = '3';
    elements.eqHighSlider.value = 0;
    elements.eqHighMode.value = 'boost';
    elements.eqHighMulti.value = '3';

    elements.eqLowValue.textContent = '0dB';
    elements.eqMidValue.textContent = '0dB';
    elements.eqHighValue.textContent = '0dB';

    if (state.eqLowFilter) state.eqLowFilter.gain.value = 0;
    if (state.eqMidFilter) state.eqMidFilter.gain.value = 0;
    if (state.eqHighFilter) state.eqHighFilter.gain.value = 0;
}

// 播放前确保均衡器已接入音频链路
(function hookEQ() {
    if (!elements.audioPlayer) return;
    const originalPlay = elements.audioPlayer.play.bind(elements.audioPlayer);
    elements.audioPlayer.play = function () {
        connectEQToAudioElement();
        return originalPlay();
    };
})();

// ==== 启动 ====
document.addEventListener('DOMContentLoaded', init);
