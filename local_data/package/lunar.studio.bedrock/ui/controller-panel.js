// ==== controller-panel.js — 控制器状态可视化面板 ====

/**
 * ControllerPanel — 控制器状态机可视化
 *
 * 在控制器列表下方追加一个状态视图区域：
 *   - 当前控制器名 + 当前状态 + 状态停留时间
 *   - 所有状态列表，高亮当前状态
 *   - 当前状态的可转移目标 + MoLang 条件
 *   - 点击状态可手动跳转（用于调试）
 *
 * 该模块只读取 ControllerVM 和 Controller 数据，不修改状态机
 * 手动跳转通过 forceTransition 委托给外部
 */
export class ControllerPanel {
    /**
     * @param {{
     *   vm: import('../core/controller-vm.js').ControllerVM,
     *   onForceTransition?: (target: string) => void,
     *   onToast?: (msg: string, type?: string) => void
     * }} deps
     */
    constructor(deps) {
        this.vm = deps.vm;
        this.onForceTransition = deps.onForceTransition;
        this.onToast = deps.onToast;
        this._container = null;
        this._build();

        // 订阅 VM 事件以触发刷新
        this.vm.onEvent.push(() => this.refresh());
    }

    _build() {
        // 找到 #tab-controllers 容器，在 #controller-list 上方插入状态视图
        const tabControllers = document.getElementById('tab-controllers');
        if (!tabControllers) {
            console.warn('[ControllerPanel] 未找到 #tab-controllers');
            return;
        }

        const container = document.createElement('div');
        container.className = 'controller-state-view';
        container.style.cssText = 'display:none;margin-bottom:10px;padding:8px;border-radius:8px;background:rgba(255,255,255,0.06)';
        container.innerHTML = `
            <div style="display:flex;justify-content:space-between;font-size:11px;color:var(--text-secondary);margin-bottom:6px">
                <span>当前控制器</span>
                <span id="ctrl-state-time">-</span>
            </div>
            <div id="ctrl-current-name" style="font-size:13px;font-weight:600;color:var(--brand);margin-bottom:4px">-</div>
            <div id="ctrl-current-state" style="font-size:12px;color:var(--accent);margin-bottom:8px">状态：-</div>
            <div id="ctrl-states-list" style="display:flex;flex-wrap:wrap;gap:4px;margin-bottom:8px"></div>
            <div id="ctrl-transitions" style="font-size:11px;line-height:1.5"></div>
        `;
        tabControllers.insertBefore(container, tabControllers.firstChild);
        this._container = container;
    }

    /**
     * 显示/隐藏
     * @param {boolean} visible
     */
    setVisible(visible) {
        if (this._container) {
            this._container.style.display = visible ? '' : 'none';
        }
    }

    /**
     * 刷新视图（每次 VM 事件触发，或定时刷新 state_time）
     */
    refresh() {
        if (!this._container) return;
        if (!this.vm.running || !this.vm.currentController) {
            this.setVisible(false);
            return;
        }
        this.setVisible(true);

        const ctrl = this.vm.currentController;
        const curStateName = this.vm.currentStateName;
        const curState = ctrl.states.get(curStateName);

        // 标题区
        this._container.querySelector('#ctrl-current-name').textContent = ctrl.name.split('.').pop();
        this._container.querySelector('#ctrl-current-state').textContent = `状态：${curStateName}`;
        this._container.querySelector('#ctrl-state-time').textContent = `t = ${this.vm.stateTime.toFixed(2)}s`;

        // 状态列表
        const statesList = this._container.querySelector('#ctrl-states-list');
        statesList.innerHTML = '';
        for (const [name, state] of ctrl.states) {
            const chip = document.createElement('div');
            const isCurrent = name === curStateName;
            chip.className = `state-chip ${isCurrent ? 'active' : ''}`;
            chip.style.cssText = `
                padding:2px 8px;border-radius:10px;font-size:11px;cursor:pointer;
                background:${isCurrent ? 'var(--brand)' : 'rgba(255,255,255,0.1)'};
                color:${isCurrent ? '#fff' : 'var(--text-secondary)'};
                transition:all 0.2s;
            `;
            chip.textContent = name;
            chip.title = `点击跳转到 "${name}"`;
            chip.addEventListener('click', () => {
                if (this.onForceTransition) {
                    this.onForceTransition(name);
                    this.onToast?.(`手动跳转：${name}`, 'success');
                }
            });
            statesList.appendChild(chip);
        }

        // 转移条件
        const transContainer = this._container.querySelector('#ctrl-transitions');
        if (!curState || curState.transitions.length === 0) {
            transContainer.innerHTML = `<div style="color:var(--text-secondary)">无转移条件</div>`;
        } else {
            const lines = curState.transitions.map(t => {
                const target = t.target;
                const cond = t.condition;
                return `<div style="margin-bottom:3px">
                    <span style="color:var(--text-secondary)">→</span>
                    <span style="color:var(--brand)">${target}</span>
                    <span style="color:var(--text-secondary)"> 当 </span>
                    <code style="font-family:'SF Mono',Consolas,monospace;font-size:10px;color:var(--text-primary);background:rgba(0,0,0,0.2);padding:1px 4px;border-radius:3px">${this._escape(cond)}</code>
                </div>`;
            });
            transContainer.innerHTML = `<div style="color:var(--text-secondary);margin-bottom:4px">转移条件：</div>${lines.join('')}`;
        }
    }

    /**
     * HTML 转义
     * @param {string} s
     * @private
     */
    _escape(s) {
        return String(s).replace(/[&<>"']/g, c => ({
            '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
        }[c]));
    }
}
