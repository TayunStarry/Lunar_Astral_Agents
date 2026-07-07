// ==== material-panel.js — 材质参数面板 ====

/**
 * MaterialPanel — 材质高级参数面板
 *
 * 在工具栏下方提供材质细调：
 *   - alphatest 阈值滑块（影响 opaque/alphatest/double_sided）
 *   - translucent 不透明度滑块（影响 translucent）
 *   - z-fighting 微调滑块（inflate bias，解决共面闪烁）
 *   - 当前材质描述显示
 *
 * 控制委托给 Renderer.setAlphaTestThreshold / setTranslucentOpacity / setInflateBias
 */
export class MaterialPanel {
    /**
     * @param {{
     *   renderer: import('../core/renderer.js').Renderer,
     *   onInflateBiasChange?: (bias: number) => void,
     *   onToast?: (msg: string, type?: string) => void
     * }} deps
     */
    constructor(deps) {
        this.renderer = deps.renderer;
        this.onInflateBiasChange = deps.onInflateBiasChange;
        this.onToast = deps.onToast;
        this._container = null;
        this._alphaSlider = null;
        this._alphaValue = null;
        this._opacitySlider = null;
        this._opacityValue = null;
        this._inflateSlider = null;
        this._inflateValue = null;
        this._descLabel = null;
        this._build();
    }

    _build() {
        // 插入到工具栏下方（固定在视口顶部下方）
        const toolbar = document.getElementById('toolbar');
        if (!toolbar) {
            console.warn('[MaterialPanel] 未找到 #toolbar');
            return;
        }

        const container = document.createElement('div');
        container.className = 'glass-panel material-panel';
        container.style.cssText = `
            position:fixed;top:64px;right:16px;z-index:50;
            padding:10px 12px;display:none;flex-direction:column;gap:8px;
            min-width:240px;font-size:11px;
        `;
        container.innerHTML = `
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:2px">
                <span style="font-weight:600;color:var(--brand)"><i class="fas fa-palette"></i> 材质参数</span>
                <button id="mat-panel-close" class="btn-glass" style="height:20px;width:20px;padding:0;font-size:10px">
                    <i class="fas fa-times"></i>
                </button>
            </div>
            <div>
                <div style="display:flex;justify-content:space-between;color:var(--text-secondary);margin-bottom:3px">
                    <span>alphatest 阈值</span>
                    <span id="mat-alpha-value">0.50</span>
                </div>
                <input type="range" id="mat-alpha-slider" min="0" max="1" step="0.05" value="0.5"
                    style="width:100%;height:16px">
            </div>
            <div>
                <div style="display:flex;justify-content:space-between;color:var(--text-secondary);margin-bottom:3px">
                    <span>translucent 不透明度</span>
                    <span id="mat-opacity-value">1.00</span>
                </div>
                <input type="range" id="mat-opacity-slider" min="0" max="1" step="0.05" value="1"
                    style="width:100%;height:16px">
            </div>
            <div>
                <div style="display:flex;justify-content:space-between;color:var(--text-secondary);margin-bottom:3px">
                    <span title="为每个 cube 添加微量膨胀，避免共面 z-fighting 闪烁">z-fighting 微调 <i class="fas fa-info-circle"></i></span>
                    <span id="mat-inflate-value">0.01</span>
                </div>
                <input type="range" id="mat-inflate-slider" min="0" max="0.1" step="0.005" value="0.01"
                    style="width:100%;height:16px">
            </div>
            <div style="padding-top:6px;border-top:1px solid rgba(255,255,255,0.1)">
                <div style="color:var(--text-secondary);margin-bottom:2px">当前配置</div>
                <div id="mat-desc" style="color:var(--text-primary);font-weight:600">-</div>
            </div>
        `;
        document.body.appendChild(container);
        this._container = container;

        this._alphaSlider = container.querySelector('#mat-alpha-slider');
        this._alphaValue = container.querySelector('#mat-alpha-value');
        this._opacitySlider = container.querySelector('#mat-opacity-slider');
        this._opacityValue = container.querySelector('#mat-opacity-value');
        this._inflateSlider = container.querySelector('#mat-inflate-slider');
        this._inflateValue = container.querySelector('#mat-inflate-value');
        this._descLabel = container.querySelector('#mat-desc');

        this._bind();
    }

    _bind() {
        this._alphaSlider.addEventListener('input', (e) => {
            const v = parseFloat(e.target.value);
            this._alphaValue.textContent = v.toFixed(2);
            this.renderer.setAlphaTestThreshold(v);
            this._updateDesc();
        });

        this._opacitySlider.addEventListener('input', (e) => {
            const v = parseFloat(e.target.value);
            this._opacityValue.textContent = v.toFixed(2);
            this.renderer.setTranslucentOpacity(v);
            this._updateDesc();
        });

        // z-fighting 微调：改变会触发模型重建，使用 change 事件避免拖拽中频繁重建
        this._inflateSlider.addEventListener('input', (e) => {
            const v = parseFloat(e.target.value);
            this._inflateValue.textContent = v.toFixed(3);
        });
        this._inflateSlider.addEventListener('change', (e) => {
            const v = parseFloat(e.target.value);
            if (this.onInflateBiasChange) {
                this.onInflateBiasChange(v);
                this.onToast?.(`z-fighting 微调：${v.toFixed(3)}（已重建模型）`, 'success');
            }
        });

        this._container.querySelector('#mat-panel-close').addEventListener('click', () => {
            this.setVisible(false);
        });
    }

    _updateDesc() {
        if (this._descLabel) {
            this._descLabel.textContent = this.renderer.getMaterialDescription();
        }
    }

    /**
     * 显示/隐藏
     * @param {boolean} visible
     */
    setVisible(visible) {
        if (this._container) {
            this._container.style.display = visible ? 'flex' : 'none';
            if (visible) this._updateDesc();
        }
    }

    /**
     * 切换可见性
     */
    toggle() {
        const willShow = this._container.style.display === 'none';
        this.setVisible(willShow);
        return willShow;
    }
}
