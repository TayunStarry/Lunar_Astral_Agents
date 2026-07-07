// ==== viewport.js — 视口控制面板 ====

/**
 * ViewportPanel — 视口预设与显示选项
 *
 * 提供相机预设按钮（前/后/左/右/上/下/等距）、网格切换、线框模式
 * 浮动在视口左下角
 */
export class ViewportPanel {
    /**
     * @param {{
     *   renderer: import('../core/renderer.js').Renderer,
     *   onToast?: (msg: string, type?: string) => void
     * }} deps
     */
    constructor(deps) {
        this.renderer = deps.renderer;
        this.onToast = deps.onToast;
        this._container = null;
        this._gridBtn = null;
        this._wireBtn = null;
        this._gridOn = true;
        this._wireOn = false;
        this._build();
    }

    _build() {
        const container = document.createElement('div');
        container.className = 'glass-panel viewport-panel';
        container.style.cssText = `
            position:fixed;bottom:48px;left:16px;z-index:50;
            padding:6px;display:flex;gap:4px;align-items:center;
        `;
        container.innerHTML = `
            <button class="btn-glass vp-btn" data-preset="front" title="前视图" style="height:28px;width:28px;padding:0;font-size:11px">F</button>
            <button class="btn-glass vp-btn" data-preset="back" title="后视图" style="height:28px;width:28px;padding:0;font-size:11px">B</button>
            <button class="btn-glass vp-btn" data-preset="left" title="左视图" style="height:28px;width:28px;padding:0;font-size:11px">L</button>
            <button class="btn-glass vp-btn" data-preset="right" title="右视图" style="height:28px;width:28px;padding:0;font-size:11px">R</button>
            <button class="btn-glass vp-btn" data-preset="top" title="顶视图" style="height:28px;width:28px;padding:0;font-size:11px">T</button>
            <button class="btn-glass vp-btn" data-preset="iso" title="等距视图" style="height:28px;width:28px;padding:0;font-size:11px"><i class="fas fa-cube"></i></button>
            <div style="width:1px;height:18px;background:rgba(255,255,255,0.2);margin:0 2px"></div>
            <button class="btn-glass" id="vp-grid" title="切换网格" style="height:28px;width:28px;padding:0;font-size:11px">
                <i class="fas fa-th"></i>
            </button>
            <button class="btn-glass" id="vp-wire" title="切换线框" style="height:28px;width:28px;padding:0;font-size:11px">
                <i class="fas fa-vector-square"></i>
            </button>
            <button class="btn-glass" id="vp-fit" title="适配相机" style="height:28px;width:28px;padding:0;font-size:11px">
                <i class="fas fa-expand"></i>
            </button>
        `;
        document.body.appendChild(container);
        this._container = container;

        this._gridBtn = container.querySelector('#vp-grid');
        this._wireBtn = container.querySelector('#vp-wire');

        this._bind();
    }

    _bind() {
        // 相机预设
        this._container.querySelectorAll('.vp-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                const preset = btn.dataset.preset;
                this.renderer.setCameraPreset(preset);
                this.onToast?.(`视角：${preset}`, 'success');
            });
        });

        // 网格切换
        this._gridBtn.addEventListener('click', () => {
            this._gridOn = !this._gridOn;
            this.renderer.setGridVisible(this._gridOn);
            this._gridBtn.style.opacity = this._gridOn ? '1' : '0.5';
            this.onToast?.(`网格：${this._gridOn ? '显示' : '隐藏'}`, 'success');
        });

        // 线框切换
        this._wireBtn.addEventListener('click', () => {
            this._wireOn = !this._wireOn;
            this.renderer.setWireframe(this._wireOn);
            this._wireBtn.style.color = this._wireOn ? 'var(--accent)' : '';
            this.onToast?.(`线框：${this._wireOn ? '开' : '关'}`, 'success');
        });

        // 适配相机
        this._container.querySelector('#vp-fit').addEventListener('click', () => {
            // 调用等距预设作为"适配"
            this.renderer.setCameraPreset('iso');
            this.onToast?.('视角已重置', 'success');
        });
    }
}
