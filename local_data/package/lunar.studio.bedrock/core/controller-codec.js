// ==== controller-codec.js — 动画控制器 JSON 编解码 ====

/**
 * Bedrock 动画控制器解析器
 * 参考 Blockbench js/animations/animation_controllers.js
 *
 * 输入格式（format_version 1.10.0）：
 * {
 *   "format_version": "1.10.0",
 *   "animation_controllers": {
 *     "controller.animation.name": {
 *       "initial_state": "default",
 *       "states": {
 *         "default": {
 *           "animations": ["anim1", "anim2"],
 *           "transitions": [
 *             {"targetState": "MoLang condition"},
 *             {"controller.other": "another condition"}
 *           ],
 *           "blend_transition": 0.35,
 *           "blend_via_shortest_path": true,
 *           "on_entry": ["v.x = 1;"],
 *           "on_exit": ["v.x = 0;"]
 *         }
 *       }
 *     }
 *   }
 * }
 */

/**
 * 控制器状态
 */
export class ControllerState {
    /**
     * @param {string} name
     */
    constructor(name) {
        this.name = name;
        /** @type {Array<string|{name: string, blend_weight: string}>} 动画引用 */
        this.animations = [];
        /** @type {Array<{target: string, condition: string}>} 转移规则 */
        this.transitions = [];
        /** @type {number} 混合过渡时长（秒） */
        this.blendTransition = 0;
        /** @type {boolean} 旋转走最短路径 */
        this.blendViaShortestPath = false;
        /** @type {Array<string>} on_entry MoLang 语句 */
        this.onEntry = [];
        /** @type {Array<string>} on_exit MoLang 语句 */
        this.onExit = [];
    }

    /**
     * 是否引用了其他控制器（target 以 "controller." 开头）
     */
    get hasControllerTransitions() {
        return this.transitions.some(t => t.target.startsWith('controller.'));
    }
}

/**
 * 动画控制器
 */
export class Controller {
    /**
     * @param {string} name
     */
    constructor(name) {
        this.name = name;
        /** @type {string} 初始状态名 */
        this.initialState = 'default';
        /** @type {Map<string, ControllerState>} 状态表 */
        this.states = new Map();
    }

    /**
     * 获取短名（去掉 "controller.animation." 前缀）
     * 例如 "controller.animation.player_to_individuation.attack_with_items" → "attack_with_items"
     */
    get shortName() {
        const parts = this.name.split('.');
        // 去掉 "controller" 和 "animation" 前缀
        if (parts.length > 2 && parts[0] === 'controller' && parts[1] === 'animation') {
            return parts.slice(2).join('.');
        }
        return this.name;
    }

    /**
     * 根据短名查找状态
     * 短名如 "attack_with_items" 匹配 "controller.animation.xxx.attack_with_items"
     */
    static matchesShortName(fullName, shortName) {
        const parts = fullName.split('.');
        if (parts.length > 2 && parts[0] === 'controller' && parts[1] === 'animation') {
            return parts.slice(2).join('.') === shortName;
        }
        return fullName === shortName;
    }
}

/**
 * 控制器编解码
 */
export class ControllerCodec {
    /**
     * 解析控制器 JSON
     * @param {object|string} json
     * @returns {Map<string, Controller>} controllerName → Controller
     */
    static parse(json) {
        if (typeof json === 'string') {
            json = JSON.parse(json);
        }
        const result = new Map();
        if (!json || !json.animation_controllers) {
            return result;
        }
        for (const [name, ctrlData] of Object.entries(json.animation_controllers)) {
            try {
                const ctrl = ControllerCodec.parseController(name, ctrlData);
                result.set(name, ctrl);
            } catch (e) {
                console.warn(`[ControllerCodec] 解析控制器失败: ${name}`, e);
            }
        }
        return result;
    }

    /**
     * 解析单个控制器
     * @param {string} name
     * @param {object} data
     * @returns {Controller}
     */
    static parseController(name, data) {
        const ctrl = new Controller(name);
        ctrl.initialState = data.initial_state || 'default';

        if (data.states) {
            for (const [stateName, stateData] of Object.entries(data.states)) {
                const state = ControllerCodec.parseState(stateName, stateData);
                ctrl.states.set(stateName, state);
            }
        }

        return ctrl;
    }

    /**
     * 解析单个状态
     * @param {string} name
     * @param {object} data
     * @returns {ControllerState}
     */
    static parseState(name, data) {
        const state = new ControllerState(name);

        // animations：数组，元素为字符串或 {name, blend_weight} 对象
        if (Array.isArray(data.animations)) {
            for (const anim of data.animations) {
                if (typeof anim === 'string') {
                    state.animations.push(anim);
                } else if (typeof anim === 'object' && anim) {
                    // 对象形式：{ animationName: blend_weight_expression }
                    for (const [animName, blendWeight] of Object.entries(anim)) {
                        state.animations.push({ name: animName, blend_weight: blendWeight });
                    }
                }
            }
        }

        // transitions：数组，每个元素是 {target: condition} 的单键对象
        if (Array.isArray(data.transitions)) {
            for (const trans of data.transitions) {
                if (typeof trans === 'object' && trans) {
                    for (const [target, condition] of Object.entries(trans)) {
                        state.transitions.push({ target, condition: String(condition) });
                    }
                }
            }
        }

        // blend_transition：混合时长（秒）
        if (typeof data.blend_transition === 'number') {
            state.blendTransition = data.blend_transition;
        }

        // blend_via_shortest_path：旋转走最短路径
        if (data.blend_via_shortest_path === true) {
            state.blendViaShortestPath = true;
        }

        // on_entry / on_exit：MoLang 语句数组
        if (Array.isArray(data.on_entry)) {
            state.onEntry = data.on_entry.map(s => String(s));
        } else if (typeof data.on_entry === 'string') {
            state.onEntry = [String(data.on_entry)];
        }

        if (Array.isArray(data.on_exit)) {
            state.onExit = data.on_exit.map(s => String(s));
        } else if (typeof data.on_exit === 'string') {
            state.onExit = [String(data.on_exit)];
        }

        return state;
    }
}
