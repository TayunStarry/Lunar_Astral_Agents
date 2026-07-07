// ==== molang-panel.js — MoLang 调试面板 ====
//
// 实时显示所有 MoLang 上下文变量（query.* / variables.* / temp.* / context.*）
// 非零值高亮，便于调试动画系统、移动系统等

const DEFAULT_QUERY_VALUES = {
    life_time: 0, time: 0, state_time: 0, vertical_speed: 0,
    ground_speed: 0, target_x_rotation: 0, target_y_rotation: 0,
    body_y_rotation: 0, is_sneaking: 0, is_moving: 0,
    is_in_water: 0, is_sprinting: 0, is_on_ground: 1,
    all_animations_finished: 0, any_animation_finished: 0,
    is_first_person: 0, is_gliding: 0, is_swimming: 0,
    is_jumping: 0, is_falling: 0, is_sleeping: 0,
    is_riding: 0, item_is_charged: 0, health: 20,
    max_health: 20, y_head_rotation: 0, head_yaw: 0,
    body_yaw: 0, walk_distance: 0, move_speed: 0,
    lateral_speed: 0
};

/**
 * MoLang 调试面板 — 浮动玻璃面板，实时显示所有 MoLang 变量
 */
export class MolangPanel {
    /**
     * @param {{ molang: import('../core/molang-runtime.js').MolangRuntime }} deps
     */
    constructor(deps) {
        this.molang = deps.molang;
        this._container = null;
        this._visible = false;
        this._rows = new Map(); // varName → DOM element
        this._build();
    }

    /**
     * 构建面板 DOM
     * @private
     */
    _build() {
        const container = document.createElement('div');
        container.id = 'molang-debug-panel';
        container.className = 'glass-panel molang-debug-panel';
        container.style.cssText = `
            position: fixed;
            top: 80px;
            right: 16px;
            width: 280px;
            max-height: calc(100vh - 120px);
            overflow-y: auto;
            padding: 12px;
            z-index: 1000;
            display: none;
            font-size: 12px;
            line-height: 1.5;
        `;

        container.innerHTML = `
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px;
                        padding-bottom:6px;border-bottom:1px solid rgba(128,128,128,0.3)">
                <span style="font-weight:bold;font-size:13px">MoLang 调试</span>
                <span style="font-size:10px;opacity:0.6" id="molang-debug-count">0 vars</span>
            </div>
            <div id="molang-debug-query-section">
                <div style="font-size:11px;opacity:0.7;margin-bottom:4px">▸ query</div>
                <div id="molang-debug-query-list"></div>
            </div>
            <div id="molang-debug-var-section" style="display:none;margin-top:8px">
                <div style="font-size:11px;opacity:0.7;margin-bottom:4px">▸ variables</div>
                <div id="molang-debug-var-list"></div>
            </div>
            <div id="molang-debug-temp-section" style="display:none;margin-top:8px">
                <div style="font-size:11px;opacity:0.7;margin-bottom:4px">▸ temp</div>
                <div id="molang-debug-temp-list"></div>
            </div>
        `;

        document.body.appendChild(container);
        this._container = container;
        this._queryList = container.querySelector('#molang-debug-query-list');
        this._varList = container.querySelector('#molang-debug-var-list');
        this._varSection = container.querySelector('#molang-debug-var-section');
        this._tempList = container.querySelector('#molang-debug-temp-list');
        this._tempSection = container.querySelector('#molang-debug-temp-section');
        this._countLabel = container.querySelector('#molang-debug-count');

        // 预建 query 变量行
        for (const name of Object.keys(DEFAULT_QUERY_VALUES)) {
            this._createRow(name, this._queryList);
        }
    }

    /**
     * 创建一行显示
     * @param {string} name
     * @param {HTMLElement} parent
     * @private
     */
    _createRow(name, parent) {
        const row = document.createElement('div');
        row.style.cssText = `
            display: flex; justify-content: space-between; align-items: center;
            padding: 1px 4px; border-radius: 3px;
            transition: background 0.2s;
        `;
        row.innerHTML = `
            <span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;flex:1">${name}</span>
            <span style="font-family:monospace;margin-left:8px;min-width:60px;text-align:right">0</span>
        `;
        parent.appendChild(row);
        this._rows.set(name, row);
    }

    /**
     * 每帧刷新（由渲染循环调用）
     */
    refresh() {
        if (!this._visible) return;

        const ctx = this.molang?.getAllContext();
        if (!ctx) return;

        let activeCount = 0;

        // 更新 query 变量
        for (const [name, row] of this._rows) {
            const value = ctx.query[name];
            const displayVal = this._formatValue(value);
            const valueEl = row.children[1];
            if (valueEl.textContent !== displayVal) {
                valueEl.textContent = displayVal;
            }

            // 高亮非默认值
            const isDefault = DEFAULT_QUERY_VALUES[name] !== undefined
                && Math.abs((value || 0) - DEFAULT_QUERY_VALUES[name]) < 0.001;
            if (!isDefault) {
                row.style.background = 'rgba(157,107,255,0.15)';
                row.style.fontWeight = 'bold';
                activeCount++;
            } else {
                row.style.background = '';
                row.style.fontWeight = '';
            }
        }

        // 更新 variables 区域
        this._updateDynamicSection(ctx.variables, this._varList, this._varSection);

        // 更新 temp 区域
        this._updateDynamicSection(ctx.temp, this._tempList, this._tempSection);

        this._countLabel.textContent = `${activeCount} active`;
    }

    /**
     * 更新动态变量区域
     * @param {object} vars
     * @param {HTMLElement} listEl
     * @param {HTMLElement} sectionEl
     * @private
     */
    _updateDynamicSection(vars, listEl, sectionEl) {
        const keys = Object.keys(vars || {});
        if (keys.length === 0) {
            sectionEl.style.display = 'none';
            listEl.innerHTML = '';
            return;
        }
        sectionEl.style.display = '';

        // 增量更新
        const existing = {};
        for (const child of listEl.children) {
            const name = child.children[0].textContent;
            existing[name] = child;
        }

        for (const name of keys) {
            const value = vars[name];
            const displayVal = this._formatValue(value);
            if (existing[name]) {
                const valueEl = existing[name].children[1];
                if (valueEl.textContent !== displayVal) {
                    valueEl.textContent = displayVal;
                }
                delete existing[name];
            } else {
                const row = document.createElement('div');
                row.style.cssText = `
                    display: flex; justify-content: space-between; align-items: center;
                    padding: 1px 4px; border-radius: 3px;
                    background: rgba(157,107,255,0.15); font-weight: bold;
                `;
                row.innerHTML = `
                    <span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;flex:1">${name}</span>
                    <span style="font-family:monospace;margin-left:8px;min-width:60px;text-align:right">${displayVal}</span>
                `;
                listEl.appendChild(row);
            }
        }

        // 移除已不存在的行
        for (const name of Object.keys(existing)) {
            existing[name].remove();
        }
    }

    /**
     * 格式化数值显示
     * @param {*} value
     * @returns {string}
     * @private
     */
    _formatValue(value) {
        if (value === undefined || value === null) return '-';
        if (typeof value === 'number') {
            if (Number.isInteger(value)) return String(value);
            return value.toFixed(2);
        }
        return String(value);
    }

    /**
     * 切换面板显隐
     */
    toggle() {
        this.setVisible(!this._visible);
    }

    /**
     * 设置面板显隐
     * @param {boolean} visible
     */
    setVisible(visible) {
        this._visible = visible;
        if (this._container) {
            this._container.style.display = visible ? '' : 'none';
        }
    }

    /** @returns {boolean} */
    get visible() { return this._visible; }
}