// ==== animation-panel.js — 动画时间轴控制面板 ====

/**
 * AnimationPanel — 动画播放增强控制
 *
 * 在动画进度条下方提供：
 *   - 时间轴 scrubber（拖拽 seek）
 *   - 速度切换（0.25x / 0.5x / 1x / 2x）
 *   - 循环模式覆盖（自动 / 一次 / 循环 / 保持最后一帧）
 *   - 暂停/恢复按钮
 *
 * 该模块只负责 UI 与事件绑定，实际控制委托给 AnimationRuntime
 */
export class AnimationPanel {
    /**
     * @param {{
     *   runtime: import('../core/animation-runtime.js').AnimationRuntime,
     *   onToast?: (msg: string, type?: string) => void
     * }} deps
     */
    constructor(deps) {
        this.runtime = deps.runtime;
        this.onToast = deps.onToast;
        this._container = null;
        this._scrubber = null;
        this._speedBtns = [];
        this._loopSelect = null;
        this._pauseBtn = null;
        this._userScrubbing = false;
        this._build();
    }

    _build() {
        // 找到 #anim-progress-bar 容器，在其下方插入控制条
        const progress = document.getElementById('anim-progress-bar');
        if (!progress) {
            console.warn('[AnimationPanel] 未找到 #anim-progress-bar');
            return;
        }

        const container = document.createElement('div');
        container.className = 'anim-controls';
        container.style.cssText = 'display:none;margin-top:8px;gap:6px;flex-wrap:wrap';
        container.innerHTML = `
            <input type="range" id="anim-scrubber" min="0" max="1000" value="0" step="1"
                class="anim-scrubber" style="flex:1 1 100%;height:18px">
            <div style="display:flex;gap:4px;align-items:center;flex:1 1 auto">
                <span style="font-size:11px;color:var(--text-secondary);margin-right:2px">速度</span>
                <button class="btn-glass anim-speed-btn" data-speed="0.25" style="height:24px;padding:0 6px;font-size:11px">0.25x</button>
                <button class="btn-glass anim-speed-btn" data-speed="0.5" style="height:24px;padding:0 6px;font-size:11px">0.5x</button>
                <button class="btn-glass anim-speed-btn active" data-speed="1" style="height:24px;padding:0 6px;font-size:11px">1x</button>
                <button class="btn-glass anim-speed-btn" data-speed="2" style="height:24px;padding:0 6px;font-size:11px">2x</button>
            </div>
            <div style="display:flex;gap:4px;align-items:center">
                <span style="font-size:11px;color:var(--text-secondary);margin-right:2px">循环</span>
                <select id="anim-loop-select" class="glass-select" style="height:24px;font-size:11px;padding:0 4px">
                    <option value="auto">自动</option>
                    <option value="once">一次</option>
                    <option value="loop">循环</option>
                    <option value="hold">保持末帧</option>
                </select>
            </div>
            <button id="anim-pause-btn" class="btn-glass" style="height:24px;padding:0 8px;font-size:11px" title="暂停/恢复">
                <i class="fas fa-pause"></i>
            </button>
        `;
        progress.parentElement.insertBefore(container, progress.nextSibling);
        this._container = container;

        this._scrubber = container.querySelector('#anim-scrubber');
        this._speedBtns = Array.from(container.querySelectorAll('.anim-speed-btn'));
        this._loopSelect = container.querySelector('#anim-loop-select');
        this._pauseBtn = container.querySelector('#anim-pause-btn');

        this._bind();
    }

    _bind() {
        // Scrubber 拖拽 seek
        this._scrubber.addEventListener('input', (e) => {
            const v = parseFloat(e.target.value);
            const ratio = v / 1000;
            const anim = this.runtime.currentAnimation;
            if (!anim || !anim.animationLength) return;
            this._userScrubbing = true;
            this.runtime.seek(ratio * anim.animationLength);
            // 立即应用一帧（暂停时也能看到效果）
            if (!this.runtime.playing) {
                // 触发一帧应用
                this.runtime._applyBones(this.runtime.currentTime);
            }
        });
        this._scrubber.addEventListener('change', () => {
            // 拖拽结束
            setTimeout(() => { this._userScrubbing = false; }, 50);
        });

        // 速度按钮
        for (const btn of this._speedBtns) {
            btn.addEventListener('click', () => {
                const speed = parseFloat(btn.dataset.speed);
                this.runtime.speed = speed;
                this._speedBtns.forEach(b => b.classList.remove('active'));
                btn.classList.add('active');
                this.onToast?.(`速度：${speed}x`, 'success');
            });
        }

        // 循环模式覆盖
        this._loopSelect.addEventListener('change', (e) => {
            const v = e.target.value;
            const mapping = {
                'auto': null,
                'once': false,
                'loop': true,
                'hold': 'hold_on_last_frame'
            };
            this.runtime.loopOverride = mapping[v] ?? null;
            this.onToast?.(`循环模式：${e.target.options[e.target.selectedIndex].text}`, 'success');
        });

        // 暂停/恢复
        this._pauseBtn.addEventListener('click', () => {
            if (!this.runtime.currentAnimation) return;
            if (this.runtime.playing) {
                this.runtime.pause();
                this._pauseBtn.innerHTML = '<i class="fas fa-play"></i>';
            } else {
                this.runtime.resume();
                this._pauseBtn.innerHTML = '<i class="fas fa-pause"></i>';
            }
        });
    }

    /**
     * 显示/隐藏控制面板
     * @param {boolean} visible
     */
    setVisible(visible) {
        if (this._container) {
            this._container.style.display = visible ? 'flex' : 'none';
        }
    }

    /**
     * 每帧更新 scrubber 位置（由 App 的状态更新循环调用）
     */
    update() {
        if (!this._scrubber) return;
        if (this._userScrubbing) return;
        const anim = this.runtime.currentAnimation;
        if (!anim || !anim.animationLength) {
            this._scrubber.value = 0;
            return;
        }
        const ratio = Math.min(1, this.runtime.currentTime / anim.animationLength);
        this._scrubber.value = Math.round(ratio * 1000);

        // 同步暂停按钮状态
        if (this.runtime.playing) {
            this._pauseBtn.innerHTML = '<i class="fas fa-pause"></i>';
        } else {
            this._pauseBtn.innerHTML = '<i class="fas fa-play"></i>';
        }
    }

    /**
     * 动画切换时重置 UI
     */
    reset() {
        if (this._scrubber) this._scrubber.value = 0;
        // 速度恢复 1x
        this.runtime.speed = 1.0;
        this.runtime.loopOverride = null;
        this._speedBtns.forEach(b => b.classList.toggle('active', b.dataset.speed === '1'));
        if (this._loopSelect) this._loopSelect.value = 'auto';
    }
}
