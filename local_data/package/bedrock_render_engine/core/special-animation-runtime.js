// ==== special-animation-runtime.js — 特殊动画运行时 ====
//
// 管理三类特殊动画的播放（叠加在 AnimGroupRuntime 之上）：
//   .blink     → 眨眼动画：随机间隔 3-5 秒触发，播放一次
//   .move      → 移动动画：模型移动时循环播放
//   .fast_move → 快速移动动画：模型快速移动时循环播放
//
// 叠加策略：
//   AnimGroupRuntime 先应用组动画（bone = rest + groupAnim）
//   SpecialAnimationRuntime 后应用特殊动画（对涉及的 bone：bone = rest + specialAnim）
//   未被特殊动画涉及的 bone 保持组动画结果不变
//
// 执行顺序（由 app.js 渲染循环控制）：
//   1. animGroupRuntime.tick(dt)   → 应用组动画
//   2. specialAnimRuntime.tick(dt) → 叠加特殊动画

import { interpolateBone } from './interpolator.js';
import { BedrockCoordinate } from './geometry-loader.js';
import { AnimationCategory, AnimationClassifier } from './animation-classifier.js';

/**
 * 特殊动画运行时
 */
export class SpecialAnimationRuntime {
    /**
     * @param {import('./molang-runtime.js').MolangRuntime} molang
     * @param {import('./animation-runtime.js').AnimationRuntime} animRuntime
     */
    constructor(molang, animRuntime) {
        this.molang = molang;
        this.animRuntime = animRuntime;

        /** @type {Map<string, import('./keyframe.js').Animation>} 全部动画表（由外部设置） */
        this.animations = new Map();

        // 分类缓存
        /** @type {string[]} 眨眼动画名列表 */
        this._blinkAnims = [];
        /** @type {string[]} 移动动画名列表 */
        this._moveAnims = [];
        /** @type {string[]} 快速移动动画名列表 */
        this._fastMoveAnims = [];

        // 眨眼状态
        /** @type {number} 距离下次眨眼的倒计时（秒） */
        this._blinkTimer = 0;
        /** @type {string|null} 当前正在播放的眨眼动画名 */
        this._blinkPlaying = null;
        /** @type {number} 眨眼动画播放时间 */
        this._blinkTime = 0;

        // 移动动画状态
        /** @type {string|null} 当前正在播放的移动动画名 */
        this._movePlaying = null;
        /** @type {number} 移动动画播放时间 */
        this._moveTime = 0;
        /** @type {number} 移动动画淡入淡出进度（0-1） */
        this._moveFade = 0;
        /** @type {boolean} 移动动画是否正在淡出 */
        this._moveFadingOut = false;

        // 移动状态（由 MovementController 设置）
        /** @type {boolean} 模型是否正在移动 */
        this._isMoving = false;
        /** @type {boolean} 模型是否正在快速移动 */
        this._isFastMoving = false;

        this._scheduleNextBlink();
    }

    /**
     * 设置可用动画表，自动分类
     * @param {Map<string, import('./keyframe.js').Animation>} animations
     */
    setAnimations(animations) {
        this.animations = animations;
        const categorized = AnimationClassifier.categorize(animations);
        this._blinkAnims = categorized.blink;
        this._moveAnims = categorized.move;
        this._fastMoveAnims = categorized.fastMove;
        // 重置播放状态
        this._blinkPlaying = null;
        this._movePlaying = null;
        this._moveFade = 0;
        this._moveFadingOut = false;
    }

    /**
     * 设置移动状态（由 MovementController 调用）
     * @param {boolean} isMoving 是否正在移动
     * @param {boolean} isFastMoving 是否正在快速移动
     */
    setMoveState(isMoving, isFastMoving) {
        const wasMoving = this._isMoving;
        this._isMoving = isMoving;
        this._isFastMoving = isFastMoving;

        // 移动状态变化时触发淡入/淡出
        if (!wasMoving && isMoving) {
            // 开始移动：启动移动动画
            this._startMoveAnimation();
        } else if (wasMoving && !isMoving) {
            // 停止移动：启动淡出
            this._moveFadingOut = true;
        }
    }

    /**
     * 每帧更新（在 AnimGroupRuntime.tick 之后调用）
     * @param {number} deltaTime 帧间隔（秒）
     */
    tick(deltaTime) {
        // 1. 处理眨眼动画
        this._tickBlink(deltaTime);

        // 2. 处理移动动画
        this._tickMove(deltaTime);
    }

    // ==== 眨眼动画 ====

    /**
     * 安排下一次眨眼
     * @private
     */
    _scheduleNextBlink() {
        // 随机 3-5 秒
        this._blinkTimer = 3 + Math.random() * 2;
    }

    /**
     * 眨眼动画帧更新
     * @param {number} dt
     * @private
     */
    _tickBlink(dt) {
        if (this._blinkAnims.length === 0) return;

        if (this._blinkPlaying) {
            // 正在播放眨眼动画
            this._blinkTime += dt;
            const anim = this.animations.get(this._blinkPlaying);
            const animLen = anim?.animationLength || 0.2;

            if (this._blinkTime >= animLen) {
                // 眨眼结束
                this._blinkPlaying = null;
                this._scheduleNextBlink();
            } else {
                // 应用眨眼动画到骨骼
                this._applySpecialAnimation(this._blinkPlaying, this._blinkTime, 1.0);
            }
        } else {
            // 倒计时
            this._blinkTimer -= dt;
            if (this._blinkTimer <= 0) {
                // 触发眨眼：随机选择一个眨眼动画
                this._blinkPlaying = this._blinkAnims[Math.floor(Math.random() * this._blinkAnims.length)];
                this._blinkTime = 0;
            }
        }
    }

    // ==== 移动动画 ====

    /**
     * 启动移动动画
     * @private
     */
    _startMoveAnimation() {
        // 优先选择 fast_move，其次 move
        if (this._isFastMoving && this._fastMoveAnims.length > 0) {
            this._movePlaying = this._fastMoveAnims[Math.floor(Math.random() * this._fastMoveAnims.length)];
        } else if (this._moveAnims.length > 0) {
            this._movePlaying = this._moveAnims[Math.floor(Math.random() * this._moveAnims.length)];
        }
        this._moveTime = 0;
        this._moveFade = 0;
        this._moveFadingOut = false;
    }

    /**
     * 移动动画帧更新
     * @param {number} dt
     * @private
     */
    _tickMove(dt) {
        if (!this._movePlaying) return;

        // 切换 fast_move ↔ move（速度变化时）
        const desiredAnim = this._isFastMoving
            ? (this._fastMoveAnims[0] || this._moveAnims[0])
            : (this._moveAnims[0] || this._fastMoveAnims[0]);
        if (desiredAnim && desiredAnim !== this._movePlaying && !this._moveFadingOut) {
            // 切换动画类型
            this._movePlaying = desiredAnim;
            this._moveTime = 0;
            this._moveFade = 0;
        }

        const anim = this.animations.get(this._movePlaying);
        if (!anim) {
            this._movePlaying = null;
            return;
        }

        // 淡入/淡出
        const fadeSpeed = 5; // 0.2 秒完成淡入/淡出
        if (this._moveFadingOut) {
            this._moveFade -= dt * fadeSpeed;
            if (this._moveFade <= 0) {
                this._moveFade = 0;
                this._movePlaying = null;
                this._moveFadingOut = false;
                return;
            }
        } else {
            this._moveFade = Math.min(1, this._moveFade + dt * fadeSpeed);
        }

        // 推进时间（循环）
        this._moveTime += dt;
        const animLen = anim.animationLength || 1;
        if (animLen > 0 && this._moveTime >= animLen) {
            this._moveTime = this._moveTime % animLen;
        }

        // 应用移动动画到骨骼（带淡入淡出权重）
        this._applySpecialAnimation(this._movePlaying, this._moveTime, this._moveFade);
    }

    // ==== 动画应用 ====

    /**
     * 将特殊动画应用到骨骼（叠加在 rest pose 之上，覆盖组动画对相同骨骼的设置）
     * @param {string} animName 动画全名
     * @param {number} time 播放时间
     * @param {number} weight 混合权重（0-1，1=完全覆盖）
     * @private
     */
    _applySpecialAnimation(animName, time, weight) {
        if (!this.animRuntime || !this.animRuntime.outliner) return;
        const anim = this.animations.get(animName);
        if (!anim) return;

        const boneMap = this.animRuntime.boneMap;
        const restPoses = this.animRuntime._restPoses;
        if (!boneMap || !restPoses) return;

        for (const [boneName, boneAnim] of anim.bones) {
            const bone = boneMap.get(boneName);
            if (!bone || !bone.sceneObject) continue;

            const result = interpolateBone(boneAnim, time, this.molang);
            const rest = restPoses.get(bone.name);
            const restRot = rest ? rest.rotation : [0, 0, 0];
            const restPos = rest ? rest.position : [0, 0, 0];
            const restScale = rest ? rest.scale : [1, 1, 1];

            bone.sceneObject.rotation.order = 'ZYX';

            // 旋转：rest + animRot，带权重混合
            if (result.rotation) {
                const animRot = BedrockCoordinate.rotationToThree(result.rotation);
                const targetRot = [
                    restRot[0] + animRot[0],
                    restRot[1] + animRot[1],
                    restRot[2] + animRot[2]
                ];
                if (weight >= 1) {
                    bone.sceneObject.rotation.set(targetRot[0], targetRot[1], targetRot[2]);
                } else {
                    bone.sceneObject.rotation.set(
                        bone.sceneObject.rotation.x + (targetRot[0] - bone.sceneObject.rotation.x) * weight,
                        bone.sceneObject.rotation.y + (targetRot[1] - bone.sceneObject.rotation.y) * weight,
                        bone.sceneObject.rotation.z + (targetRot[2] - bone.sceneObject.rotation.z) * weight
                    );
                }
            }

            // 位置：rest + animPos，带权重混合
            if (result.position) {
                const targetPos = [
                    restPos[0] + result.position[0],
                    restPos[1] + result.position[1],
                    restPos[2] + result.position[2]
                ];
                if (weight >= 1) {
                    bone.sceneObject.position.set(targetPos[0], targetPos[1], targetPos[2]);
                } else {
                    bone.sceneObject.position.set(
                        bone.sceneObject.position.x + (targetPos[0] - bone.sceneObject.position.x) * weight,
                        bone.sceneObject.position.y + (targetPos[1] - bone.sceneObject.position.y) * weight,
                        bone.sceneObject.position.z + (targetPos[2] - bone.sceneObject.position.z) * weight
                    );
                }
            }

            // 缩放：rest * animScale，带权重混合
            if (result.scale) {
                const sx = result.scale[0] !== undefined ? result.scale[0] : 1;
                const sy = result.scale[1] !== undefined ? result.scale[1] : 1;
                const sz = result.scale[2] !== undefined ? result.scale[2] : 1;
                if (sx === 0 || sy === 0 || sz === 0) {
                    if (weight >= 0.5) bone.sceneObject.visible = false;
                } else {
                    bone.sceneObject.visible = true;
                    const targetSx = restScale[0] * sx;
                    const targetSy = restScale[1] * sy;
                    const targetSz = restScale[2] * sz;
                    if (weight >= 1) {
                        bone.sceneObject.scale.set(targetSx, targetSy, targetSz);
                    } else {
                        bone.sceneObject.scale.set(
                            bone.sceneObject.scale.x + (targetSx - bone.sceneObject.scale.x) * weight,
                            bone.sceneObject.scale.y + (targetSy - bone.sceneObject.scale.y) * weight,
                            bone.sceneObject.scale.z + (targetSz - bone.sceneObject.scale.z) * weight
                        );
                    }
                }
            }
        }
    }
}
