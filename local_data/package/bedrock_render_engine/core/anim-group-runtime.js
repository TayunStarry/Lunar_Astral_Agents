// ==== anim-group-runtime.js — 动画组运行时 ====
//
// 职责：
//   - 管理动画组（默认组 + 自定义组）
//   - 默认组永久生效；自定义组互斥激活，平滑切换
//   - 骨骼显隐控制（自定义组覆盖默认组）
//   - 无限时间轴播放（repeat / return / hold）
//   - 导入/导出配置 JSON
//
// 参考 Blockbench animation player + Bedrock animation controller 概念重写

import { interpolateBone } from './interpolator.js';
import { BedrockCoordinate } from './geometry-loader.js';

/**
 * 动画组 — 一组按顺序播放的动画集合
 */
export class AnimGroup {
    /**
     * @param {string} name 组名
     */
    constructor(name) {
        this.name = name;
        /** @type {boolean} 是否为默认组（永久生效） */
        this.isDefault = false;
        /** @type {string[]} 动画全名列表（按播放顺序） */
        this.animations = [];
        /** @type {Object<string, boolean>} boneName → visible，未列出的骨骼保持默认 */
        this.boneVisibility = {};
        /** @type {'repeat'|'return'|'hold'} 循环模式 */
        this.loopMode = 'repeat';
        /** @type {number} 组切换过渡时长（秒） */
        this.transitionDuration = 0.3;
        /** @type {number} 骨骼显隐延迟（秒）：组激活后等待此时长再应用 boneVisibility */
        this.visibilityDelay = 0;
    }

    /**
     * 序列化为纯 JSON
     */
    toJSON() {
        return {
            name: this.name,
            isDefault: this.isDefault,
            animations: [...this.animations],
            boneVisibility: { ...this.boneVisibility },
            loopMode: this.loopMode,
            transitionDuration: this.transitionDuration,
            visibilityDelay: this.visibilityDelay
        };
    }

    /**
     * 从 JSON 反序列化
     * @param {object} data
     * @returns {AnimGroup}
     */
    static fromJSON(data) {
        const g = new AnimGroup(data.name || '未命名组');
        g.isDefault = !!data.isDefault;
        g.animations = Array.isArray(data.animations) ? [...data.animations] : [];
        g.boneVisibility = data.boneVisibility && typeof data.boneVisibility === 'object' ? { ...data.boneVisibility } : {};
        g.loopMode = ['repeat', 'return', 'hold'].includes(data.loopMode) ? data.loopMode : 'repeat';
        g.transitionDuration = typeof data.transitionDuration === 'number' ? data.transitionDuration : 0.3;
        g.visibilityDelay = typeof data.visibilityDelay === 'number' ? data.visibilityDelay : 0;
        return g;
    }
}

/**
 * 动画组运行时 — 驱动动画组播放与切换
 *
 * 播放规则：
 *   - 默认组始终播放（其动画序列循环）
 *   - 自定义组互斥：一次仅一个自定义组生效
 *   - 激活自定义组时，从当前姿势平滑过渡到目标组
 *   - 动画间切换也使用平滑过渡（复用 transitionDuration）
 *   - 骨骼显隐：自定义组的 boneVisibility 覆盖默认组（true=显式可见，false=显式隐藏）
 *
 * 无限时间轴：
 *   - 组内动画按顺序播放
 *   - 单个动画播放完毕后，根据 loopMode：
 *     repeat → 重新播放整个序列
 *     return → 回到初始姿势并停止（仅默认组继续）
 *     hold → 保持最后一帧
 *   - 函数动画部分（MoLang q.life_time）始终继续推演
 */
export class AnimGroupRuntime {
    /**
     * @param {import('./molang-runtime.js').MolangRuntime} molang
     * @param {import('./animation-runtime.js').AnimationRuntime} animRuntime
     */
    constructor(molang, animRuntime) {
        this.molang = molang;
        this.animRuntime = animRuntime;
        /** @type {Map<string, import('./keyframe.js').Animation>} 可用动画表（由外部设置） */
        this.animations = new Map();

        /** @type {Map<string, AnimGroup>} 组名 → AnimGroup */
        this.groups = new Map();
        /** @type {AnimGroup|null} 默认组 */
        this.defaultGroup = null;
        /** @type {AnimGroup|null} 当前激活的自定义组 */
        this.activeCustomGroup = null;

        // 播放状态
        /** @type {number} 当前组内动画索引 */
        this.currentIndex = 0;
        /** @type {number} 当前动画播放时间 */
        this.currentTime = 0;
        /** @type {boolean} 是否正在播放 */
        this.playing = false;
        /** @type {boolean} 是否停留在最后一帧（hold 模式：停止关键帧推进但持续推演 MoLang 函数动画） */
        this.holding = false;

        // 骨骼显隐延迟状态
        /** @type {boolean} 是否正在等待显隐延迟 */
        this._visDelayActive = false;
        /** @type {number} 显隐延迟计时器（秒） */
        this._visDelayTimer = 0;
        /** @type {number} 当前显隐延迟时长（秒） */
        this._visDelayDuration = 0;

        // 过渡状态
        /** @type {boolean} 是否正在过渡 */
        this.transitioning = false;
        /** @type {number} 过渡进度（0-1） */
        this.transitionAlpha = 0;
        /** @type {number} 过渡时长（秒） */
        this.transitionDuration = 0.3;
        /** @type {Map<string, {position:Array, rotation:Array, scale:Array, visible:boolean}>} 过渡起始姿势 */
        this._transitionFromPoses = null;
        /** @type {AnimGroup|null} 过渡目标组 */
        this._transitionToGroup = null;

        /** @type {Array<(event:string, group:AnimGroup|null) => void>} 事件回调 */
        this.onEvent = [];
    }

    /**
     * 获取当前生效的组（自定义组优先，否则默认组）
     */
    get currentGroup() {
        return this.activeCustomGroup || this.defaultGroup;
    }

    /**
     * 添加动画组
     * @param {AnimGroup} group
     */
    addGroup(group) {
        this.groups.set(group.name, group);
        if (group.isDefault) {
            this.defaultGroup = group;
        }
    }

    /**
     * 移除动画组
     * @param {string} name
     */
    removeGroup(name) {
        const g = this.groups.get(name);
        if (!g) return;
        if (g.isDefault) return; // 默认组不可删除
        if (this.activeCustomGroup === g) {
            this.deactivateCustomGroup();
        }
        this.groups.delete(name);
    }

    /**
     * 设置默认组
     * @param {string} name
     */
    setDefaultGroup(name) {
        const g = this.groups.get(name);
        if (!g) return;
        if (this.defaultGroup) this.defaultGroup.isDefault = false;
        g.isDefault = true;
        this.defaultGroup = g;
    }

    /**
     * 激活自定义组（互斥，平滑过渡）
     * @param {string} name 组名
     */
    activateGroup(name) {
        const g = this.groups.get(name);
        if (!g || g.isDefault) return;

        if (this.activeCustomGroup === g) return; // 已激活

        // 捕获当前姿势作为过渡起点
        this._captureTransitionStart();

        this.activeCustomGroup = g;
        this.currentIndex = 0;
        this.currentTime = 0;
        this.transitioning = true;
        this.transitionAlpha = 0;
        this.transitionDuration = g.transitionDuration || 0.3;
        this._transitionToGroup = g;
        this.playing = true;
        this.holding = false;
        this._startVisibilityDelay(g.visibilityDelay);

        this._emit('activate', g);
    }

    /**
     * 停用当前自定义组，回到默认组
     */
    deactivateCustomGroup() {
        if (!this.activeCustomGroup) return;

        this._captureTransitionStart();

        const prev = this.activeCustomGroup;
        this.activeCustomGroup = null;
        this.currentIndex = 0;
        this.currentTime = 0;
        this.transitioning = true;
        this.transitionAlpha = 0;
        this.transitionDuration = (this.defaultGroup?.transitionDuration) || 0.3;
        this._transitionToGroup = this.defaultGroup;
        this.playing = true;
        this.holding = false;
        // 回到默认组：默认组的显隐始终生效，无延迟
        this._visDelayActive = false;

        this._emit('deactivate', prev);
    }

    /**
     * 开始骨骼显隐延迟倒计时
     * @param {number} delay 延迟时长（秒），0 表示立即生效
     * @private
     */
    _startVisibilityDelay(delay) {
        this._visDelayTimer = 0;
        this._visDelayDuration = Math.max(0, delay || 0);
        this._visDelayActive = this._visDelayDuration > 0;
    }

    /**
     * 捕获过渡起始姿势
     * @private
     */
    _captureTransitionStart() {
        if (!this.animRuntime || !this.animRuntime.outliner) return;
        this._transitionFromPoses = new Map();
        this.animRuntime.outliner.traverseBones(bone => {
            if (bone.sceneObject) {
                const o = bone.sceneObject;
                this._transitionFromPoses.set(bone.name, {
                    position: [o.position.x, o.position.y, o.position.z],
                    rotation: [o.rotation.x, o.rotation.y, o.rotation.z],
                    scale: [o.scale.x, o.scale.y, o.scale.z],
                    visible: o.visible
                });
            }
        });
    }

    /**
     * 开始播放（默认组）
     */
    play() {
        if (!this.defaultGroup) return;
        // 确保 rest poses 已捕获
        if (this.animRuntime && !this.animRuntime._restCaptured) {
            this.animRuntime._captureRestPoses();
        }
        this.currentIndex = 0;
        this.currentTime = 0;
        this.playing = true;
        this.holding = false;
        // 默认组开始播放：显隐始终生效，无延迟
        this._visDelayActive = false;
        this._emit('play', this.currentGroup);
    }

    /**
     * 停止播放
     */
    stop() {
        this.playing = false;
        this.holding = false;
        this._visDelayActive = false;
        this.activeCustomGroup = null;
        this.transitioning = false;
        if (this.animRuntime) this.animRuntime.stop();
        this._emit('stop', null);
    }

    /**
     * 每帧更新（由 renderer.onUpdate 调用）
     * @param {number} deltaTime 帧间隔（秒）
     */
    tick(deltaTime) {
        if (!this.playing) return;

        // 累计 MoLang 生命时间
        this.animRuntime.entityLifeTime += deltaTime;

        const group = this.currentGroup;
        if (!group || group.animations.length === 0) {
            // 无动画可播，仅更新 MoLang 上下文
            this._updateMolangContext();
            return;
        }

        this._updateMolangContext();

        // 处理过渡
        if (this.transitioning) {
            this.transitionAlpha += deltaTime / Math.max(0.001, this.transitionDuration);
            if (this.transitionAlpha >= 1) {
                this.transitionAlpha = 1;
                this.transitioning = false;
                this._transitionFromPoses = null;
                this._transitionToGroup = null;
            }
        }

        // 推进动画时间（hold 状态下不推进，固定在最后一帧）
        if (!this.transitioning && !this.holding) {
            this.currentTime += deltaTime;
        }

        // 获取当前动画
        const animName = group.animations[this.currentIndex];
        const anim = this.animations.get(animName);
        if (!anim) {
            // 动画不存在，跳到下一个
            this._advanceToNext(group);
            return;
        }

        const animLen = anim.animationLength || 0;

        // hold 状态下固定 currentTime 在 animLen
        if (this.holding && animLen > 0) {
            this.currentTime = animLen;
        }

        // 检查动画是否结束（hold 状态下不再触发 _advanceToNext）
        if (!this.holding && animLen > 0 && this.currentTime >= animLen) {
            this._advanceToNext(group);
            return;
        }

        // 应用动画到骨骼（hold 状态下继续应用，持续推演 MoLang 函数动画）
        if (this.transitioning && this._transitionFromPoses) {
            this._applyWithTransition(anim, this.currentTime, this.transitionAlpha, this._transitionFromPoses);
        } else {
            this._applyAnimation(anim, this.currentTime);
        }

        // 应用骨骼显隐
        // 延迟期间仅应用默认组的 boneVisibility；延迟结束后合并自定义组的覆盖
        if (this._visDelayActive) {
            this._visDelayTimer += deltaTime;
            if (this._visDelayTimer >= this._visDelayDuration) {
                this._visDelayActive = false;
            }
            // 延迟期间：仅默认组生效
            this._applyBoneVisibility(group, true);
        } else {
            // 正常：默认组 + 自定义组覆盖
            this._applyBoneVisibility(group, false);
        }
    }

    /**
     * 推进到下一个动画
     * 过渡策略：
     *   - 序列内切换不同动画（A→B）：启动平滑过渡，避免姿势突变
     *   - repeat 循环回放（最后→第一个）：不启动过渡，避免循环停顿
     *   - return/hold：按各自语义处理
     * @param {AnimGroup} group
     * @private
     */
    _advanceToNext(group) {
        const wasLast = this.currentIndex >= group.animations.length - 1;
        const transDuration = group.transitionDuration || 0.3;

        if (wasLast) {
            // 序列播放完毕，根据 loopMode 决定
            switch (group.loopMode) {
                case 'repeat':
                    // 循环回放：直接重置，不启动过渡，避免循环停顿
                    this.holding = false;
                    this.currentIndex = 0;
                    this.currentTime = 0;
                    break;
                case 'return':
                    // 回到初始状态，停止（仅自定义组；默认组继续 repeat）
                    if (group.isDefault) {
                        this.holding = false;
                        this.currentIndex = 0;
                        this.currentTime = 0;
                    } else {
                        this.playing = false;
                        this.holding = false;
                        this.animRuntime._restoreRestPoses();
                        this.deactivateCustomGroup();
                    }
                    break;
                case 'hold':
                    // 停在最后一帧，持续推演 MoLang 函数动画（如荡秋千的函数曲线）
                    {
                        const animName = group.animations[this.currentIndex];
                        const anim = this.animations.get(animName);
                        this.currentTime = anim?.animationLength || 0;
                    }
                    this.holding = true;
                    // 不设置 playing = false，保持 tick 循环以持续推演 MoLang
                    break;
            }
        } else {
            // 推进到下一个不同动画，启动平滑过渡
            this.holding = false;
            this._captureTransitionStart();
            this.currentIndex++;
            this.currentTime = 0;
            this.transitioning = true;
            this.transitionAlpha = 0;
            this.transitionDuration = transDuration;
        }
    }

    /**
     * 更新 MoLang 上下文
     * @private
     */
    _updateMolangContext() {
        this.molang.updateContext({
            life_time: this.animRuntime.entityLifeTime,
            time: this.currentTime,
            state_time: this.currentTime,
            anim_time: this.currentTime,
            life_time_in_animation: this.currentTime
        });
    }

    /**
     * 应用动画到骨骼（叠加到 rest pose）
     * @param {import('./keyframe.js').Animation} anim
     * @param {number} time
     * @private
     */
    _applyAnimation(anim, time) {
        const outliner = this.animRuntime.outliner;
        if (!outliner) return;
        const boneMap = this.animRuntime.boneMap;
        const restPoses = this.animRuntime._restPoses;

        for (const [boneName, boneAnim] of anim.bones) {
            const bone = boneMap.get(boneName);
            if (!bone || !bone.sceneObject) continue;

            const result = interpolateBone(boneAnim, time, this.molang);
            const rest = restPoses.get(bone.name);

            if (result.rotation) {
                const restRot = rest ? rest.rotation : [0, 0, 0];
                const animRot = BedrockCoordinate.rotationToThree(result.rotation);
                bone.sceneObject.rotation.order = 'ZYX';
                bone.sceneObject.rotation.set(
                    restRot[0] + animRot[0],
                    restRot[1] + animRot[1],
                    restRot[2] + animRot[2]
                );
            }

            if (result.position) {
                const restPos = rest ? rest.position : [0, 0, 0];
                bone.sceneObject.position.set(
                    restPos[0] + result.position[0],
                    restPos[1] + result.position[1],
                    restPos[2] + result.position[2]
                );
            }

            if (result.scale) {
                const restScale = rest ? rest.scale : [1, 1, 1];
                const sx = result.scale[0] !== undefined ? result.scale[0] : 1;
                const sy = result.scale[1] !== undefined ? result.scale[1] : 1;
                const sz = result.scale[2] !== undefined ? result.scale[2] : 1;
                if (sx === 0 || sy === 0 || sz === 0) {
                    bone.sceneObject.visible = false;
                } else {
                    bone.sceneObject.visible = true;
                    bone.sceneObject.scale.set(restScale[0] * sx, restScale[1] * sy, restScale[2] * sz);
                }
            }
        }
    }

    /**
     * 带过渡的动画应用（从起始姿势插值到目标动画姿势）
     * @param {import('./keyframe.js').Animation} anim
     * @param {number} time
     * @param {number} alpha 过渡进度 0-1
     * @param {Map} fromPoses 起始姿势
     * @private
     */
    _applyWithTransition(anim, time, alpha, fromPoses) {
        const outliner = this.animRuntime.outliner;
        if (!outliner) return;
        const boneMap = this.animRuntime.boneMap;
        const restPoses = this.animRuntime._restPoses;
        const easeAlpha = alpha * alpha * (3 - 2 * alpha); // smoothstep

        // 先收集目标动画影响的骨骼
        const animatedBones = new Set();
        for (const [boneName] of anim.bones) {
            animatedBones.add(boneName);
        }

        // 对所有骨骼做过渡
        outliner.traverseBones(bone => {
            if (!bone.sceneObject) return;
            const from = fromPoses.get(bone.name);
            if (!from) return;

            const boneAnim = anim.bones.get(bone.name);
            const rest = restPoses.get(bone.name);

            if (boneAnim) {
                const result = interpolateBone(boneAnim, time, this.molang);
                // 旋转过渡
                if (result.rotation) {
                    const restRot = rest ? rest.rotation : [0, 0, 0];
                    const animRot = BedrockCoordinate.rotationToThree(result.rotation);
                    const targetRot = [restRot[0] + animRot[0], restRot[1] + animRot[1], restRot[2] + animRot[2]];
                    bone.sceneObject.rotation.order = 'ZYX';
                    bone.sceneObject.rotation.set(
                        from.rotation[0] + (targetRot[0] - from.rotation[0]) * easeAlpha,
                        from.rotation[1] + (targetRot[1] - from.rotation[1]) * easeAlpha,
                        from.rotation[2] + (targetRot[2] - from.rotation[2]) * easeAlpha
                    );
                }
                // 位置过渡
                if (result.position) {
                    const restPos = rest ? rest.position : [0, 0, 0];
                    const targetPos = [restPos[0] + result.position[0], restPos[1] + result.position[1], restPos[2] + result.position[2]];
                    bone.sceneObject.position.set(
                        from.position[0] + (targetPos[0] - from.position[0]) * easeAlpha,
                        from.position[1] + (targetPos[1] - from.position[1]) * easeAlpha,
                        from.position[2] + (targetPos[2] - from.position[2]) * easeAlpha
                    );
                }
                // 缩放过渡
                if (result.scale) {
                    const restScale = rest ? rest.scale : [1, 1, 1];
                    const sx = result.scale[0] !== undefined ? result.scale[0] : 1;
                    const sy = result.scale[1] !== undefined ? result.scale[1] : 1;
                    const sz = result.scale[2] !== undefined ? result.scale[2] : 1;
                    const targetSx = sx === 0 ? 0.0001 : restScale[0] * sx;
                    const targetSy = sy === 0 ? 0.0001 : restScale[1] * sy;
                    const targetSz = sz === 0 ? 0.0001 : restScale[2] * sz;
                    bone.sceneObject.visible = true;
                    bone.sceneObject.scale.set(
                        from.scale[0] + (targetSx - from.scale[0]) * easeAlpha,
                        from.scale[1] + (targetSy - from.scale[1]) * easeAlpha,
                        from.scale[2] + (targetSz - from.scale[2]) * easeAlpha
                    );
                }
            } else {
                // 该骨骼不在当前动画中，从起始姿势过渡到 rest pose
                const restRot = rest ? rest.rotation : [0, 0, 0];
                const restPos = rest ? rest.position : [0, 0, 0];
                const restScale = rest ? rest.scale : [1, 1, 1];
                bone.sceneObject.rotation.order = 'ZYX';
                bone.sceneObject.rotation.set(
                    from.rotation[0] + (restRot[0] - from.rotation[0]) * easeAlpha,
                    from.rotation[1] + (restRot[1] - from.rotation[1]) * easeAlpha,
                    from.rotation[2] + (restRot[2] - from.rotation[2]) * easeAlpha
                );
                bone.sceneObject.position.set(
                    from.position[0] + (restPos[0] - from.position[0]) * easeAlpha,
                    from.position[1] + (restPos[1] - from.position[1]) * easeAlpha,
                    from.position[2] + (restPos[2] - from.position[2]) * easeAlpha
                );
                bone.sceneObject.visible = true;
                bone.sceneObject.scale.set(
                    from.scale[0] + (restScale[0] - from.scale[0]) * easeAlpha,
                    from.scale[1] + (restScale[1] - from.scale[1]) * easeAlpha,
                    from.scale[2] + (restScale[2] - from.scale[2]) * easeAlpha
                );
            }
        });
    }

    /**
     * 应用骨骼显隐（自定义组覆盖默认组）
     * 合并策略：默认组的 boneVisibility 始终生效；自定义组的覆盖在延迟结束后生效
     * @param {AnimGroup} group
     * @param {boolean} delayActive 是否处于显隐延迟期（延迟期仅应用默认组）
     * @private
     */
    _applyBoneVisibility(group, delayActive = false) {
        if (!this.animRuntime.outliner) return;
        // 默认组的显隐始终生效
        const merged = {};
        if (this.defaultGroup) {
            Object.assign(merged, this.defaultGroup.boneVisibility);
        }
        // 自定义组的覆盖仅在延迟结束后生效
        if (this.activeCustomGroup && !delayActive) {
            Object.assign(merged, this.activeCustomGroup.boneVisibility);
        }
        for (const [boneName, visible] of Object.entries(merged)) {
            const bone = this.animRuntime.boneMap.get(boneName);
            if (bone && bone.sceneObject) {
                bone.sceneObject.visible = visible;
            }
        }
    }

    /**
     * 触发事件
     * @private
     */
    _emit(event, group) {
        for (const cb of this.onEvent) {
            try { cb(event, group); } catch (e) { console.warn('[AnimGroupRuntime] 事件回调异常:', e); }
        }
    }

    /**
     * 获取当前播放进度（0-1）
     */
    get progress() {
        const group = this.currentGroup;
        if (!group || group.animations.length === 0) return 0;
        const animName = group.animations[this.currentIndex];
        const anim = this.animations.get(animName);
        if (!anim || !anim.animationLength) return 0;
        return Math.min(1, this.currentTime / anim.animationLength);
    }

    /**
     * 获取当前动画名（短名）
     */
    get currentAnimationShortName() {
        const group = this.currentGroup;
        if (!group || group.animations.length === 0) return '';
        return AnimGroupRuntime.shortName(group.animations[this.currentIndex]);
    }

    /**
     * 截取动画短名（最后一段）
     * "animation.player_to_individuation.standby_animation-0" → "standby_animation-0"
     */
    static shortName(fullName) {
        if (!fullName) return '';
        const parts = fullName.split('.');
        return parts[parts.length - 1];
    }

    /**
     * 导出全部配置为 JSON
     * @returns {object}
     */
    exportConfig() {
        return {
            format_version: '1.0.0',
            animation_groups: {
                default_group: this.defaultGroup?.name || null,
                groups: Array.from(this.groups.values()).map(g => g.toJSON())
            }
        };
    }

    /**
     * 从 JSON 导入配置
     * @param {object} json
     * @param {Map<string, import('./keyframe.js').Animation>} availableAnimations 可用动画表（用于校验）
     */
    importConfig(json, availableAnimations) {
        if (!json || !json.animation_groups) return;
        this.groups.clear();
        this.defaultGroup = null;
        this.activeCustomGroup = null;

        const cfg = json.animation_groups;
        const groups = Array.isArray(cfg.groups) ? cfg.groups : [];
        for (const gData of groups) {
            const g = AnimGroup.fromJSON(gData);
            // 校验动画是否存在
            if (availableAnimations) {
                g.animations = g.animations.filter(name => availableAnimations.has(name));
            }
            this.addGroup(g);
        }

        // 设置默认组
        const defaultName = cfg.default_group;
        if (defaultName && this.groups.has(defaultName)) {
            this.setDefaultGroup(defaultName);
        } else {
            // 找第一个 isDefault 的组
            for (const g of this.groups.values()) {
                if (g.isDefault) { this.defaultGroup = g; break; }
            }
        }
    }
}
