// ==== controller-vm.js — 动画控制器状态机虚拟机 ====

import { Controller } from './controller-codec.js';

/**
 * ControllerVM — 驱动动画控制器状态机
 * 参考 Blockbench js/animations/animation_controllers.js:updatePreview (行 1119)
 *
 * 职责：
 *   - 维护当前控制器、当前状态、状态停留时间
 *   - 每帧求值转移条件，触发状态切换
 *   - 执行 on_entry / on_exit MoLang 语句
 *   - 通过 animationRuntime 播放状态引用的动画
 *   - 处理嵌套控制器引用（target 以 "controller." 开头）
 *
 * 简化项（Phase 4）：
 *   - blend_transition：暂不实现平滑混合，直接切换状态（动画自身的插值提供基础平滑）
 *   - 多动画叠加：状态引用多个动画时，仅播放第一个（Phase 7 实现完整混合）
 */
export class ControllerVM {
    /**
     * @param {import('./molang-runtime.js').MolangRuntime} molang
     * @param {import('./animation-runtime.js').AnimationRuntime} animationRuntime
     * @param {Map<string, import('./controller-codec.js').Controller>} controllers
     */
    constructor(molang, animationRuntime, controllers) {
        this.molang = molang;
        this.animationRuntime = animationRuntime;
        this.controllers = controllers;

        /** @type {Controller|null} 当前控制器 */
        this.currentController = null;
        /** @type {string|null} 当前状态名 */
        this.currentStateName = null;
        /** @type {number} 当前状态停留时间（秒） */
        this.stateTime = 0;
        /** @type {boolean} 是否正在运行 */
        this.running = false;

        /** @type {Array<(event: string, controller: string, state: string) => void>} 事件回调 */
        this.onEvent = [];

        /** @type {number} 上次 tick 时间戳 */
        this._lastTickTime = 0;
    }

    /**
     * 启动控制器
     * @param {Controller|string} controller 控制器对象或名称
     */
    play(controller) {
        let ctrl = null;
        if (typeof controller === 'string') {
            ctrl = this.controllers.get(controller);
            if (!ctrl) {
                // 尝试短名匹配
                for (const [fullName, c] of this.controllers) {
                    if (Controller.matchesShortName(fullName, controller)) {
                        ctrl = c;
                        break;
                    }
                }
            }
        } else {
            ctrl = controller;
        }

        if (!ctrl) {
            console.warn('[ControllerVM] 控制器未找到');
            return;
        }

        this.currentController = ctrl;
        this.currentStateName = ctrl.initialState;
        this.stateTime = 0;
        this.running = true;

        // 进入初始状态
        this._enterState(ctrl.initialState);
        this._emit('start', ctrl.name, ctrl.initialState);
    }

    /**
     * 停止控制器
     */
    stop() {
        if (!this.running) return;
        this.running = false;
        if (this.currentController && this.currentStateName) {
            const state = this.currentController.states.get(this.currentStateName);
            if (state) this._execStatements(state.onExit);
        }
        this.animationRuntime.stop();
        this._emit('stop', this.currentController ? this.currentController.name : '', '');
        this.currentController = null;
        this.currentStateName = null;
        this.stateTime = 0;
    }

    /**
     * 强制转移到指定状态（手动跳转，用于调试）
     * @param {string} target 目标状态名或 controller.xxx 引用
     */
    forceTransition(target) {
        if (!this.running || !this.currentController) return;
        this._transition(target);
    }

    /**
     * 每帧更新（由外部驱动，通常通过 renderer.onUpdate）
     * @param {number} deltaTime 帧间隔（秒）
     */
    tick(deltaTime) {
        if (!this.running || !this.currentController || !this.currentStateName) return;

        this.stateTime += deltaTime;

        // 更新 MoLang 上下文
        this.molang.updateContext({
            state_time: this.stateTime,
            life_time: this.animationRuntime.entityLifeTime
        });

        // 求值转移条件
        const state = this.currentController.states.get(this.currentStateName);
        if (!state) {
            console.warn(`[ControllerVM] 状态 ${this.currentStateName} 不存在`);
            this.stop();
            return;
        }

        for (const trans of state.transitions) {
            try {
                const condition = this.molang.eval(trans.condition);
                if (condition) {
                    this._transition(trans.target);
                    return;
                }
            } catch (e) {
                // 条件求值失败，跳过
            }
        }
    }

    /**
     * 执行状态转移
     * @param {string} target 目标状态名或控制器引用
     * @private
     */
    _transition(target) {
        if (!this.currentController) return;

        // 嵌套控制器引用：target 以 "controller." 开头
        if (target.startsWith('controller.')) {
            // 执行当前状态 on_exit
            const oldState = this.currentController.states.get(this.currentStateName);
            if (oldState) this._execStatements(oldState.onExit);

            this._emit('transition', this.currentController.name, target);

            // 查找目标控制器（按短名）
            const shortName = target.replace(/^controller\./, '');
            let targetCtrl = null;
            for (const [fullName, c] of this.controllers) {
                if (Controller.matchesShortName(fullName, shortName)) {
                    targetCtrl = c;
                    break;
                }
            }
            if (targetCtrl) {
                this.currentController = targetCtrl;
                this.currentStateName = targetCtrl.initialState;
                this.stateTime = 0;
                this._enterState(targetCtrl.initialState);
            } else {
                console.warn(`[ControllerVM] 嵌套控制器未找到: ${target}`);
            }
            return;
        }

        // 普通状态转移
        const oldStateName = this.currentStateName;
        const oldState = this.currentController.states.get(oldStateName);
        if (oldState) this._execStatements(oldState.onExit);

        this._emit('transition', this.currentController.name, `${oldStateName} → ${target}`);

        this.currentStateName = target;
        this.stateTime = 0;
        this._enterState(target);
    }

    /**
     * 进入状态
     * @param {string} stateName
     * @private
     */
    _enterState(stateName) {
        const state = this.currentController.states.get(stateName);
        if (!state) {
            console.warn(`[ControllerVM] 状态 ${stateName} 不存在`);
            this.stop();
            return;
        }

        // 执行 on_entry
        this._execStatements(state.onEntry);

        // 播放状态引用的动画
        if (state.animations.length > 0) {
            const firstAnim = state.animations[0];
            const animName = typeof firstAnim === 'string' ? firstAnim : firstAnim.name;
            const anim = this.animationRuntime.currentAnimations?.get(animName) ||
                         this._findAnimationByName(animName);
            if (anim) {
                this.animationRuntime.play(anim, { restart: true });
            } else {
                console.warn(`[ControllerVM] 动画未找到: ${animName}`);
                this.animationRuntime.stop();
            }
        } else {
            // 无动画引用，停止当前动画
            this.animationRuntime.stop();
        }

        this._emit('enter_state', this.currentController.name, stateName);
    }

    /**
     * 在动画映射中查找（兼容短名）
     * @param {string} name
     * @returns {import('./keyframe.js').Animation|null}
     * @private
     */
    _findAnimationByName(name) {
        // 直接匹配
        if (this.animationRuntime.currentAnimations?.has(name)) {
            return this.animationRuntime.currentAnimations.get(name);
        }
        // 这里 animationRuntime 没有 currentAnimations，需要从 app 层注入
        // 通过 _animationLookup 回退
        if (this._animationLookup) {
            return this._animationLookup(name);
        }
        return null;
    }

    /**
     * 设置动画查找函数（由 app.js 注入）
     * @param {(name: string) => import('./keyframe.js').Animation|null} fn
     */
    setAnimationLookup(fn) {
        this._animationLookup = fn;
    }

    /**
     * 执行 MoLang 语句数组
     * @param {Array<string>} statements
     * @private
     */
    _execStatements(statements) {
        if (!statements || statements.length === 0) return;
        for (const stmt of statements) {
            try {
                this.molang.eval(stmt);
            } catch (e) {
                console.warn(`[ControllerVM] MoLang 语句执行失败: ${stmt}`, e);
            }
        }
    }

    /**
     * 触发事件
     * @private
     */
    _emit(event, controller, state) {
        for (const cb of this.onEvent) {
            try {
                cb(event, controller, state);
            } catch (e) {
                console.warn('[ControllerVM] 事件回调异常:', e);
            }
        }
    }

    /**
     * 获取当前状态信息
     */
    get status() {
        return {
            controller: this.currentController ? this.currentController.name : '',
            state: this.currentStateName || '',
            stateTime: this.stateTime,
            running: this.running
        };
    }
}
