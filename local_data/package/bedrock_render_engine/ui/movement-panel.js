// ==== movement-panel.js — 移动控制面板 ====
//
// 功能模块：
//   - 位置控制：X/Y/Z 坐标输入 + 移动按钮
//   - 朝向控制：偏航/俯仰滑块 + 精确输入
//   - 鼠标追踪：启用/禁用开关
//   - 自动锁定：锁定目标为鼠标位置
//
// 面板浮动在视口左侧（骨骼层级 tab 下方），默认隐藏

import { MovementController } from '../core/movement-controller.js';

/**
 * 移动控制面板
 */
export class MovementPanel {
    /**
     * @param {{
     *   controller: MovementController,
     *   onToast?: (msg: string, type?: string) => void
     * }} deps
     */
    constructor(deps) {
        this.controller = deps.controller;
        this.onToast = deps.onToast || (() => {});
        this._container = null;
        this._build();
        this._bindEvents();
        this._syncFromController();
    }

    /**
     * 构建面板 DOM
     * @private
     */
    _build() {
        const container = document.createElement('div');
        container.className = 'glass-panel movement-panel';
        container.style.cssText = `
            position:fixed; top:56px; left:16px; z-index:50;
            width: 280px; padding: 12px; display: none;
            flex-direction: column; gap: 10px; font-size: 12px;
            max-height: calc(100vh - 120px); overflow-y: auto;
        `;
        container.innerHTML = `
            <div style="display:flex;justify-content:space-between;align-items:center;border-bottom:1px solid var(--border-color);padding-bottom:6px">
                <span style="font-weight:600;color:var(--brand);font-size:13px">
                    <i class="fas fa-arrows-alt"></i> 移动控制
                </span>
                <button class="btn-bone-action" id="mp-close" title="关闭">
                    <i class="fas fa-times"></i>
                </button>
            </div>

            <!-- 位置控制 -->
            <div>
                <div style="color:var(--text-secondary);margin-bottom:4px;font-weight:500">
                    <i class="fas fa-map-marker-alt"></i> 目标位置
                </div>
                <div style="display:flex;gap:4px;align-items:center">
                    <div style="flex:1;display:flex;flex-direction:column;gap:2px">
                        <label style="font-size:10px;color:var(--text-secondary)">X</label>
                        <input type="number" id="mp-pos-x" value="0" step="0.5" style="width:100%;height:26px;padding:0 6px;font-size:12px;border:1px solid var(--glass-border);border-radius:4px;background:var(--input-bg);color:var(--text-primary)">
                    </div>
                    <div style="flex:1;display:flex;flex-direction:column;gap:2px">
                        <label style="font-size:10px;color:var(--text-secondary)">Y</label>
                        <input type="number" id="mp-pos-y" value="0" step="0.5" style="width:100%;height:26px;padding:0 6px;font-size:12px;border:1px solid var(--glass-border);border-radius:4px;background:var(--input-bg);color:var(--text-primary)">
                    </div>
                    <div style="flex:1;display:flex;flex-direction:column;gap:2px">
                        <label style="font-size:10px;color:var(--text-secondary)">Z</label>
                        <input type="number" id="mp-pos-z" value="0" step="0.5" style="width:100%;height:26px;padding:0 6px;font-size:12px;border:1px solid var(--glass-border);border-radius:4px;background:var(--input-bg);color:var(--text-primary)">
                    </div>
                </div>
                <div style="display:flex;gap:4px;margin-top:4px">
                    <button class="btn-glass" id="mp-move" style="flex:1;height:28px;font-size:11px">
                        <i class="fas fa-play"></i> 移动到目标
                    </button>
                    <button class="btn-glass" id="mp-teleport" style="flex:1;height:28px;font-size:11px">
                        <i class="fas fa-bolt"></i> 瞬移
                    </button>
                </div>
            </div>

            <!-- 朝向控制 -->
            <div>
                <div style="color:var(--text-secondary);margin-bottom:4px;font-weight:500">
                    <i class="fas fa-compass"></i> 朝向控制
                </div>
                <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:2px">
                    <span style="font-size:10px;color:var(--text-secondary)">偏航 (target_y_rotation)</span>
                    <span id="mp-yaw-val" style="font-size:11px;font-family:monospace;color:var(--text-primary)">0°</span>
                </div>
                <input type="range" id="mp-yaw" min="-180" max="180" step="1" value="0" style="width:100%;height:16px">
                <div style="display:flex;justify-content:space-between;align-items:center;margin-top:4px;margin-bottom:2px">
                    <span style="font-size:10px;color:var(--text-secondary)">俯仰 (target_x_rotation)</span>
                    <span id="mp-pitch-val" style="font-size:11px;font-family:monospace;color:var(--text-primary)">0°</span>
                </div>
                <input type="range" id="mp-pitch" min="-89" max="89" step="1" value="0" style="width:100%;height:16px">
            </div>

            <!-- 鼠标追踪 -->
            <div style="border-top:1px solid var(--border-color);padding-top:8px;display:flex;flex-direction:column;gap:6px">
                <label style="display:flex;align-items:center;gap:6px;cursor:pointer">
                    <input type="checkbox" id="mp-mouse-track" style="cursor:pointer">
                    <span style="font-size:12px">
                        <i class="fas fa-mouse"></i> 鼠标追踪（朝向鼠标）
                    </span>
                </label>
                <label style="display:flex;align-items:center;gap:6px;cursor:pointer">
                    <input type="checkbox" id="mp-mouse-lock" style="cursor:pointer">
                    <span style="font-size:12px">
                        <i class="fas fa-crosshairs"></i> 自动锁定（移动到鼠标）
                    </span>
                </label>
            </div>

            <!-- 当前状态 -->
            <div style="border-top:1px solid var(--border-color);padding-top:8px;font-size:11px;color:var(--text-secondary)">
                <div style="display:flex;justify-content:space-between">
                    <span>当前位置:</span>
                    <span id="mp-cur-pos" style="font-family:monospace">0, 0, 0</span>
                </div>
                <div style="display:flex;justify-content:space-between">
                    <span>移动状态:</span>
                    <span id="mp-move-state" style="font-family:monospace">静止</span>
                </div>
            </div>
        `;
        document.body.appendChild(container);
        this._container = container;
    }

    /**
     * 绑定事件
     * @private
     */
    _bindEvents() {
        const c = this._container;

        // 关闭按钮
        c.querySelector('#mp-close').addEventListener('click', () => {
            this.setVisible(false);
        });

        // 移动到目标
        c.querySelector('#mp-move').addEventListener('click', () => {
            const x = parseFloat(c.querySelector('#mp-pos-x').value) || 0;
            const y = parseFloat(c.querySelector('#mp-pos-y').value) || 0;
            const z = parseFloat(c.querySelector('#mp-pos-z').value) || 0;
            this.controller.setTarget(x, y, z);
            this.onToast(`移动到 (${x}, ${y}, ${z})`, 'success');
        });

        // 瞬移
        c.querySelector('#mp-teleport').addEventListener('click', () => {
            const x = parseFloat(c.querySelector('#mp-pos-x').value) || 0;
            const y = parseFloat(c.querySelector('#mp-pos-y').value) || 0;
            const z = parseFloat(c.querySelector('#mp-pos-z').value) || 0;
            this.controller.setPosition(x, y, z);
            this.onToast(`瞬移到 (${x}, ${y}, ${z})`, 'success');
        });

        // 偏航滑块
        const yawSlider = c.querySelector('#mp-yaw');
        const yawVal = c.querySelector('#mp-yaw-val');
        yawSlider.addEventListener('input', (e) => {
            const v = parseFloat(e.target.value);
            yawVal.textContent = v.toFixed(0) + '°';
            this.controller.setRotation(v, parseFloat(c.querySelector('#mp-pitch').value));
        });

        // 俯仰滑块
        const pitchSlider = c.querySelector('#mp-pitch');
        const pitchVal = c.querySelector('#mp-pitch-val');
        pitchSlider.addEventListener('input', (e) => {
            const v = parseFloat(e.target.value);
            pitchVal.textContent = v.toFixed(0) + '°';
            this.controller.setRotation(parseFloat(yawSlider.value), v);
        });

        // 鼠标追踪开关
        c.querySelector('#mp-mouse-track').addEventListener('change', (e) => {
            this.controller.setMouseTracking(e.target.checked);
            this.onToast(`鼠标追踪：${e.target.checked ? '启用' : '禁用'}`, 'success');
        });

        // 自动锁定开关
        c.querySelector('#mp-mouse-lock').addEventListener('change', (e) => {
            this.controller.setMouseLock(e.target.checked);
            // 联动鼠标追踪开关
            if (e.target.checked) {
                c.querySelector('#mp-mouse-track').checked = true;
            }
            this.onToast(`自动锁定：${e.target.checked ? '启用' : '禁用'}`, 'success');
        });

        // 注册控制器回调（更新状态显示）
        this.controller.onPositionChange((pos) => {
            const el = c.querySelector('#mp-cur-pos');
            if (el) el.textContent = `${pos.x.toFixed(1)}, ${pos.y.toFixed(1)}, ${pos.z.toFixed(1)}`;
        });

        // 定期更新移动状态显示
        setInterval(() => {
            const stateEl = c.querySelector('#mp-move-state');
            if (!stateEl) return;
            if (this.controller.isFastMoving) {
                stateEl.textContent = '快速移动';
                stateEl.style.color = 'var(--warning)';
            } else if (this.controller.isMoving) {
                stateEl.textContent = '移动中';
                stateEl.style.color = 'var(--success)';
            } else {
                stateEl.textContent = '静止';
                stateEl.style.color = 'var(--text-secondary)';
            }
        }, 200);
    }

    /**
     * 从控制器同步面板状态
     * @private
     */
    _syncFromController() {
        const c = this._container;
        const pos = this.controller.currentPosition;
        const rot = this.controller.currentRotation;
        c.querySelector('#mp-pos-x').value = pos.x.toFixed(1);
        c.querySelector('#mp-pos-y').value = pos.y.toFixed(1);
        c.querySelector('#mp-pos-z').value = pos.z.toFixed(1);
        c.querySelector('#mp-yaw').value = rot.yaw;
        c.querySelector('#mp-yaw-val').textContent = rot.yaw.toFixed(0) + '°';
        c.querySelector('#mp-pitch').value = rot.pitch;
        c.querySelector('#mp-pitch-val').textContent = rot.pitch.toFixed(0) + '°';
    }

    /**
     * 显示/隐藏
     * @param {boolean} visible
     */
    setVisible(visible) {
        if (this._container) {
            this._container.style.display = visible ? 'flex' : 'none';
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
