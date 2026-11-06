// ============================================================
//  星月智能 · 共享音乐播放器
//  桌面端与移动端共用同一套「播放器 + 波形 + 进度高亮」。
//  数据来自后端推送：{ title, summary, duration, audio }。
// ============================================================

(function (global) {
    'use strict';

    // 波形柱数量：窄屏也能保持清晰的疏密
    const PEAK_COUNT = 96;
    // 已解析的波形峰值缓存（同一音频地址只解码一次，历史重渲染直接复用）
    const peakCache = new Map();
    // 用户显式收起/关闭过的音频地址（刷新或重开页面后不再自动展开）
    const DISMISSED_KEY = 'lunar.music.dismissed';
    // 共享 AudioContext：decodeAudioData 不需要用户手势
    let sharedContext = null;

    function readDismissed() {
        try {
            const list = JSON.parse(global.localStorage.getItem(DISMISSED_KEY) || '[]');
            return Array.isArray(list) ? list : [];
        } catch (error) {
            return [];
        }
    }

    function writeDismissed(list) {
        try {
            global.localStorage.setItem(DISMISSED_KEY, JSON.stringify(list));
        } catch (error) {
            // 本地存储不可用时静默：不影响播放器本身
        }
    }

    function isDismissed(url) {
        const key = String(url || '');
        if (!key) return false;
        return readDismissed().indexOf(key) !== -1;
    }

    function dismiss(url) {
        const key = String(url || '');
        if (!key) return;
        const list = readDismissed().filter(item => item !== key);
        list.push(key);
        writeDismissed(list);
    }

    function undismiss(url) {
        const key = String(url || '');
        if (!key) return;
        writeDismissed(readDismissed().filter(item => item !== key));
    }

    function getAudioContext() {
        if (!sharedContext) {
            const Ctor = global.AudioContext || global.webkitAudioContext;
            if (!Ctor) throw new Error('当前浏览器不支持 Web Audio');
            sharedContext = new Ctor();
        }
        return sharedContext;
    }

    function formatTime(seconds) {
        if (!Number.isFinite(seconds) || seconds <= 0) return '--:--';
        const total = Math.round(seconds);
        const minutes = Math.floor(total / 60);
        const rest = total % 60;
        return minutes + ':' + String(rest).padStart(2, '0');
    }

    /** 从 WAV 数据提取归一化峰值；失败时抛错由调用方展示失败状态 */
    async function computePeaks(url) {
        if (peakCache.has(url)) return peakCache.get(url);
        const response = await fetch(url, { credentials: 'same-origin' });
        if (!response.ok) throw new Error('HTTP ' + response.status);
        const payload = await response.arrayBuffer();
        const buffer = await getAudioContext().decodeAudioData(payload);
        const channel = buffer.getChannelData(0);
        const block = Math.max(1, Math.floor(channel.length / PEAK_COUNT));
        const peaks = new Float32Array(PEAK_COUNT);
        let maxPeak = 0;
        for (let index = 0; index < PEAK_COUNT; index++) {
            const start = index * block;
            const end = Math.min(start + block, channel.length);
            let peak = 0;
            for (let cursor = start; cursor < end; cursor++) {
                const value = Math.abs(channel[cursor]);
                if (value > peak) peak = value;
            }
            peaks[index] = peak;
            if (peak > maxPeak) maxPeak = peak;
        }
        if (maxPeak > 0) {
            for (let index = 0; index < PEAK_COUNT; index++) peaks[index] /= maxPeak;
        }
        peakCache.set(url, peaks);
        return peaks;
    }

    /** 创建音乐卡片：播放/暂停、波形、已播高亮、当前时间/总时长 */
    function createCard(options) {
        const opts = options || {};
        const audioUrl = String(opts.audio || '');
        const card = document.createElement('div');
        card.className = 'lunar-music-card';
        card.dataset.state = audioUrl ? 'loading' : 'empty';

        const head = document.createElement('div');
        head.className = 'lunar-music-head';

        // 封面：渐变底圆角方形，与标题区垂直居中
        const cover = document.createElement('span');
        cover.className = 'lunar-music-cover';
        cover.innerHTML = '<i class="fas fa-music"></i>';
        head.appendChild(cover);

        const main = document.createElement('div');
        main.className = 'lunar-music-main';
        const title = document.createElement('div');
        title.className = 'lunar-music-title';
        const titleText = document.createElement('span');
        titleText.textContent = opts.title || '月华演奏曲';
        title.appendChild(titleText);
        main.appendChild(title);
        if (opts.summary) {
            const summary = document.createElement('div');
            summary.className = 'lunar-music-summary';
            summary.textContent = opts.summary;
            main.appendChild(summary);
        }
        head.appendChild(main);

        // 收起按钮：优先交给调用方处理（如折叠为迷你条），未提供时直接移除卡片
        const collapse = document.createElement('button');
        collapse.type = 'button';
        collapse.className = 'lunar-music-collapse';
        collapse.title = '收起';
        collapse.setAttribute('aria-label', '收起音乐卡片');
        collapse.innerHTML = '<i class="fas fa-chevron-down"></i>';
        head.appendChild(collapse);
        card.appendChild(head);

        const player = document.createElement('div');
        player.className = 'lunar-music-player';

        const toggle = document.createElement('button');
        toggle.type = 'button';
        toggle.className = 'lunar-music-toggle';
        toggle.title = audioUrl ? '播放' : '暂无音频';
        toggle.innerHTML = '<i class="fas fa-play"></i>';
        player.appendChild(toggle);

        const wave = document.createElement('div');
        wave.className = 'lunar-music-wave';
        wave.setAttribute('role', 'slider');
        wave.setAttribute('aria-label', '播放进度');
        wave.setAttribute('aria-valuemin', '0');
        wave.setAttribute('aria-valuemax', '100');
        wave.setAttribute('aria-valuenow', '0');
        wave.tabIndex = 0;
        const canvas = document.createElement('canvas');
        canvas.className = 'lunar-music-canvas';
        wave.appendChild(canvas);
        const stateText = document.createElement('span');
        stateText.className = 'lunar-music-state';
        wave.appendChild(stateText);
        player.appendChild(wave);
        card.appendChild(player);

        const times = document.createElement('div');
        times.className = 'lunar-music-times';
        const currentText = document.createElement('span');
        currentText.className = 'lunar-music-current';
        currentText.textContent = '0:00';
        const totalText = document.createElement('span');
        totalText.className = 'lunar-music-total';
        totalText.textContent = formatTime(Number(opts.duration) || 0);
        times.appendChild(currentText);
        times.appendChild(totalText);
        card.appendChild(times);

        const notice = document.createElement('div');
        notice.className = 'lunar-music-notice';
        notice.hidden = true;
        notice.textContent = '波形加载失败，播放器仍可使用';
        card.appendChild(notice);

        // ---- 播放与波形状态 ----
        let peaks = null;
        let fallbackDuration = Number(opts.duration) || 0;
        let scrubbing = false;

        const audio = audioUrl ? new Audio() : null;
        if (audio) {
            audio.preload = 'metadata';
            audio.src = audioUrl;
        }

        function currentDuration() {
            if (audio && Number.isFinite(audio.duration) && audio.duration > 0) return audio.duration;
            return fallbackDuration;
        }

        function draw(progress) {
            const cssWidth = wave.clientWidth;
            const cssHeight = wave.clientHeight;
            if (cssWidth <= 0 || cssHeight <= 0) return;
            const dpr = global.devicePixelRatio || 1;
            if (canvas.width !== Math.round(cssWidth * dpr) || canvas.height !== Math.round(cssHeight * dpr)) {
                canvas.width = Math.round(cssWidth * dpr);
                canvas.height = Math.round(cssHeight * dpr);
            }
            const ctx = canvas.getContext('2d');
            ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
            ctx.clearRect(0, 0, cssWidth, cssHeight);
            if (!peaks) return;

            const step = cssWidth / PEAK_COUNT;
            const barWidth = Math.max(1.5, step * 0.62);
            const middle = cssHeight / 2;
            const playedUntil = Math.max(0, Math.min(1, progress || 0));
            // 单色波形的取色：已播用主题色，未播用次级文字色，均通过透明度压存在感
            const computed = global.getComputedStyle(card);
            const accentColor = (computed.getPropertyValue('--accent-color') || '').trim() || '#9d6bff';
            const mutedColor = (computed.getPropertyValue('--text-secondary') || '').trim() || 'rgba(238, 241, 255, 0.68)';
            for (let index = 0; index < PEAK_COUNT; index++) {
                const ratio = Math.max(0.04, peaks[index]);
                const height = Math.max(2, ratio * (cssHeight - 4));
                const x = index * step + (step - barWidth) / 2;
                const isPlayed = (index + 0.5) / PEAK_COUNT <= playedUntil;
                ctx.globalAlpha = isPlayed ? 0.85 : 0.28;
                ctx.fillStyle = isPlayed ? accentColor : mutedColor;
                if (typeof ctx.roundRect === 'function') {
                    ctx.beginPath();
                    ctx.roundRect(x, middle - height / 2, barWidth, height, barWidth / 2);
                    ctx.fill();
                } else {
                    ctx.fillRect(x, middle - height / 2, barWidth, height);
                }
            }
            // 已播进度游标
            const cursorX = playedUntil * cssWidth;
            ctx.globalAlpha = 0.55;
            ctx.fillStyle = accentColor;
            ctx.fillRect(Math.max(0, Math.min(cssWidth - 1, cursorX)), 0, 1, cssHeight);
            ctx.globalAlpha = 1;
        }

        function renderProgress() {
            const duration = currentDuration();
            const position = audio ? audio.currentTime : 0;
            const progress = duration > 0 ? position / duration : 0;
            draw(progress);
            currentText.textContent = formatTime(position);
            if (duration > 0) totalText.textContent = formatTime(duration);
            wave.setAttribute('aria-valuenow', String(Math.round(progress * 100)));
        }

        function setState(state, message) {
            card.dataset.state = state;
            stateText.textContent = message || '';
            if (state === 'empty' || state === 'audio-error') {
                toggle.disabled = true;
                toggle.innerHTML = '<i class="fas fa-play"></i>';
            } else {
                toggle.disabled = false;
            }
        }

        function updateToggleIcon() {
            if (!audio) return;
            toggle.innerHTML = audio.paused
                ? '<i class="fas fa-play"></i>'
                : '<i class="fas fa-pause"></i>';
            toggle.title = audio.paused ? '播放' : '暂停';
        }

        toggle.addEventListener('click', () => {
            if (!audio || card.dataset.state === 'audio-error' || card.dataset.state === 'empty') return;
            if (audio.paused) {
                getAudioContext().resume().catch(() => {});
                audio.play().catch(() => setState('audio-error', '音频播放失败'));
            } else {
                audio.pause();
            }
        });

        // 折叠/展开状态：折叠后由调用方决定迷你条呈现方式
        function setCollapsed(collapsed) {
            const isCollapsed = Boolean(collapsed);
            card.classList.toggle('is-collapsed', isCollapsed);
            card.dataset.collapsed = isCollapsed ? '1' : '0';
            collapse.title = isCollapsed ? '展开' : '收起';
            collapse.setAttribute('aria-label', isCollapsed ? '展开音乐卡片' : '收起音乐卡片');
            collapse.innerHTML = isCollapsed
                ? '<i class="fas fa-chevron-up"></i>'
                : '<i class="fas fa-chevron-down"></i>';
        }
        card.setCollapsed = setCollapsed;

        function expandCard() {
            setCollapsed(false);
            if (typeof opts.onExpand === 'function') opts.onExpand(card);
        }

        function collapseCard() {
            // 收起时默认暂停，避免卡片不可见但声音仍在播放；调用方可显式关闭该行为
            if (opts.pauseOnClose !== false && audio && !audio.paused) audio.pause();
            if (typeof opts.onClose === 'function') {
                opts.onClose(card);
            } else {
                if (resizeObserver) resizeObserver.disconnect();
                card.remove();
            }
        }

        function toggleCard() {
            if (card.classList.contains('is-collapsed')) {
                expandCard();
            } else {
                collapseCard();
            }
        }

        collapse.addEventListener('click', (event) => {
            event.stopPropagation();
            toggleCard();
        });

        // 桌面端可选：点击卡片本体即可展开/收起；移动端不启用，避免误触
        if (opts.toggleOnCardClick) {
            card.classList.add('is-clickable');
            card.addEventListener('click', (event) => {
                if (event.target.closest('button, .lunar-music-wave, a, input, textarea, select')) return;
                toggleCard();
            });
        }

        if (audio) {
            audio.addEventListener('loadedmetadata', () => {
                if (Number.isFinite(audio.duration) && audio.duration > 0) fallbackDuration = audio.duration;
                totalText.textContent = formatTime(audio.duration);
                renderProgress();
            });
            audio.addEventListener('timeupdate', renderProgress);
            audio.addEventListener('play', updateToggleIcon);
            audio.addEventListener('pause', updateToggleIcon);
            audio.addEventListener('ended', () => {
                updateToggleIcon();
                renderProgress();
            });
            audio.addEventListener('error', () => setState('audio-error', '音频加载失败'));
        }

        function seekFromEvent(event) {
            if (!audio || card.dataset.state === 'audio-error' || card.dataset.state === 'empty') return;
            const rect = wave.getBoundingClientRect();
            if (rect.width <= 0) return;
            const ratio = Math.max(0, Math.min(1, (event.clientX - rect.left) / rect.width));
            const duration = currentDuration();
            if (duration > 0) audio.currentTime = ratio * duration;
            renderProgress();
        }

        wave.addEventListener('pointerdown', (event) => {
            scrubbing = true;
            wave.setPointerCapture?.(event.pointerId);
            seekFromEvent(event);
        });
        wave.addEventListener('pointermove', (event) => {
            if (scrubbing) seekFromEvent(event);
        });
        wave.addEventListener('pointerup', () => { scrubbing = false; });
        wave.addEventListener('pointercancel', () => { scrubbing = false; });
        wave.addEventListener('keydown', (event) => {
            if (!audio) return;
            const duration = currentDuration();
            if (duration <= 0) return;
            if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
                event.preventDefault();
                const delta = (event.key === 'ArrowLeft' ? -1 : 1) * Math.max(1, duration / 20);
                audio.currentTime = Math.max(0, Math.min(duration, audio.currentTime + delta));
                renderProgress();
            }
        });

        // ---- 波形加载 ----
        if (audioUrl) {
            setState('loading', '音频加载中…');
            computePeaks(audioUrl)
                .then((data) => {
                    peaks = data;
                    setState('ready', '');
                    renderProgress();
                })
                .catch(() => {
                    // 波形不可用时不阻断播放：用占位波形继续展示已播进度
                    peaks = new Float32Array(PEAK_COUNT);
                    for (let index = 0; index < PEAK_COUNT; index++) {
                        peaks[index] = 0.22 + 0.18 * Math.abs(Math.sin(index * 0.6));
                    }
                    notice.hidden = false;
                    setState('ready', '');
                    renderProgress();
                });
        } else {
            setState('empty', '暂无演奏音频');
        }

        const resizeObserver = typeof ResizeObserver === 'function'
            ? new ResizeObserver(() => renderProgress())
            : null;
        if (resizeObserver) resizeObserver.observe(wave);

        renderProgress();
        return card;
    }

    global.LunarMusicPlayer = { createCard, isDismissed, dismiss, undismiss };
})(window);
