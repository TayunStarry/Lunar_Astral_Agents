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
    // 共享 AudioContext：decodeAudioData 不需要用户手势
    let sharedContext = null;

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
        const title = document.createElement('div');
        title.className = 'lunar-music-title';
        title.innerHTML = '<i class="fas fa-music"></i>';
        const titleText = document.createElement('span');
        titleText.textContent = opts.title || '月华演奏曲';
        title.appendChild(titleText);
        head.appendChild(title);
        card.appendChild(head);

        if (opts.summary) {
            const summary = document.createElement('div');
            summary.className = 'lunar-music-summary';
            summary.textContent = opts.summary;
            card.appendChild(summary);
        }

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
            for (let index = 0; index < PEAK_COUNT; index++) {
                const ratio = Math.max(0.04, peaks[index]);
                const height = Math.max(2, ratio * (cssHeight - 4));
                const x = index * step + (step - barWidth) / 2;
                const isPlayed = (index + 0.5) / PEAK_COUNT <= playedUntil;
                const gradient = ctx.createLinearGradient(0, middle - height / 2, 0, middle + height / 2);
                if (isPlayed) {
                    gradient.addColorStop(0, 'rgba(157, 107, 255, 0.95)');
                    gradient.addColorStop(1, 'rgba(108, 155, 207, 0.95)');
                } else {
                    gradient.addColorStop(0, 'rgba(157, 107, 255, 0.28)');
                    gradient.addColorStop(1, 'rgba(108, 155, 207, 0.24)');
                }
                ctx.fillStyle = gradient;
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
            ctx.fillStyle = 'rgba(255, 255, 255, 0.85)';
            ctx.fillRect(Math.max(0, Math.min(cssWidth - 1, cursorX)), 0, 1, cssHeight);
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

    global.LunarMusicPlayer = { createCard };
})(window);
