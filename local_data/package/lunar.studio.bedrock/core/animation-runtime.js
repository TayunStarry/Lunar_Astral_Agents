// ==== animation-runtime.js — 动画运行时 ====

import { interpolateBone } from './interpolator.js';
import { BedrockCoordinate } from './geometry-loader.js';

/**
 * 动画运行时 — 驱动动画播放并应用骨骼变换
 * 参考 Blockbench js/animations/animation_player.js + BoneAnimator
 *
 * 职责：
 *   - 播放/停止/暂停动画
 *   - 每帧推进时间，处理循环模式
 *   - 对每个骨骼通道进行插值，应用 position/rotation/scale
 *   - 维护 MoLang 上下文（q.life_time 等）
 *   - 捕获/恢复静态姿势
 *
 * 坐标系约定：
 *   - rotation：.bbmodel 度数 → Three.js 弧度（直接转换，不取反轴）
 *   - position：像素单位，叠加到骨骼默认位置（rest pose）的偏移量
 *   - scale：无量纲，替换默认 [1,1,1]
 */
export class AnimationRuntime {
    /**
     * @param {import('./molang-runtime.js').MolangRuntime} molang
     * @param {import('./outliner.js').Outliner} outliner
     */
    constructor(molang, outliner) {
        this.molang = molang;
        this.outliner = outliner;

        /** @type {Map<string, import('./outliner.js').Bone>} boneName → Bone */
        this.boneMap = new Map();
        this._buildBoneMap();

        /** @type {import('./keyframe.js').Animation|null} 当前播放的动画 */
        this.currentAnimation = null;
        /** @type {number} 当前动画时间（秒） */
        this.currentTime = 0;
        /** @type {boolean} 是否正在播放 */
        this.playing = false;
        /** @type {number} 播放速度倍率 */
        this.speed = 1.0;

        /** @type {boolean|'hold_on_last_frame'|null} 循环模式覆盖（null=使用动画自带设置） */
        this.loopOverride = null;

        /** @type {Map<string, {position: Array, rotation: Array, scale: Array}>} 静态姿势快照 */
        this._restPoses = new Map();
        /** @type {boolean} 是否已捕获静态姿势 */
        this._restCaptured = false;

        /** @type {Array<(anim: string, time: number) => void>} 动画事件回调 */
        this.onAnimationEvent = [];

        /** @type {number} 实体累计存活时间（秒），用于 q.life_time */
        this.entityLifeTime = 0;
    }

    /**
     * 构建 boneName → Bone 查找表
     * @private
     */
    _buildBoneMap() {
        this.boneMap.clear();
        if (!this.outliner) return;
        this.outliner.traverseBones(bone => {
            this.boneMap.set(bone.name, bone);
        });
    }

    /**
     * 重建查找表（模型重新加载后调用）
     */
    rebuild() {
        this._buildBoneMap();
        this._restCaptured = false;
    }

    /**
     * 播放动画
     * @param {import('./keyframe.js').Animation} animation
     * @param {object} options { restart: boolean, speed: number }
     */
    play(animation, options = {}) {
        if (!animation) return;
        this.currentAnimation = animation;
        if (options.restart !== false || !this.playing) {
            this.currentTime = 0;
        }
        if (options.speed !== undefined) {
            this.speed = options.speed;
        }
        this.playing = true;
        this._captureRestPoses();
        this._emit('play', animation.name);
    }

    /**
     * 停止动画并恢复静态姿势
     */
    stop() {
        if (!this.playing) return;
        this.playing = false;
        this._restoreRestPoses();
        if (this.currentAnimation) {
            this._emit('stop', this.currentAnimation.name);
        }
    }

    /**
     * 暂停（保持当前姿势，不恢复静态）
     */
    pause() {
        this.playing = false;
        this._emit('pause', this.currentAnimation ? this.currentAnimation.name : '');
    }

    /**
     * 从当前时间恢复播放
     */
    resume() {
        if (this.currentAnimation) {
            this.playing = true;
            this._emit('resume', this.currentAnimation.name);
        }
    }

    /**
     * 跳转到指定时间
     * @param {number} time 秒
     */
    seek(time) {
        this.currentTime = Math.max(0, time);
    }

    /**
     * 每帧更新（由 renderer.onUpdate 调用）
     * @param {number} deltaTime 帧间隔（秒）
     */
    tick(deltaTime) {
        if (!this.playing || !this.currentAnimation) return;

        // 累计实体存活时间
        this.entityLifeTime += deltaTime;

        // 更新 MoLang 上下文
        this.molang.updateContext({
            life_time: this.entityLifeTime,
            time: this.currentTime,
            state_time: this.currentTime,
            anim_time: this.currentTime,
            life_time_in_animation: this.currentTime
        });

        // 推进动画时间
        this.currentTime += deltaTime * this.speed;

        // 处理循环与结束（loopOverride 优先于动画自带设置）
        const anim = this.currentAnimation;
        const len = anim.animationLength || 0;
        const effectiveLoop = this.loopOverride !== null ? this.loopOverride : anim.loop;
        if (len > 0 && this.currentTime >= len) {
            if (effectiveLoop === true) {
                this.currentTime = this.currentTime % len;
            } else if (effectiveLoop === 'hold_on_last_frame') {
                this.currentTime = len;
                this.playing = false;
                this._emit('finish', anim.name);
            } else {
                // loop === false：播放一次后停止并恢复静态
                this.currentTime = len;
                this.playing = false;
                this._applyBones(len);
                this._restoreRestPoses();
                this._emit('finish', anim.name);
                return;
            }
        }

        // 应用骨骼变换
        this._applyBones(this.currentTime);
    }

    /**
     * 应用所有骨骼变换
     *
     * 参考 Blockbench timeline_animators.js：
     *   - displayRotation: bone.rotation.x += degToRad(arr[0])  → 叠加到 rest pose
     *   - displayPosition: bone.position.x += arr[0]            → 叠加到 rest pose
     *   - displayScale:    bone.scale.x *= arr[0] || 0.00001    → 乘法 + 零保护
     *
     * 关键：动画变换是叠加/乘法到静态姿势（rest pose）上，而非替换。
     * 这对有非零默认旋转的骨骼（如尾巴 180° 基础旋转）至关重要。
     *
     * @param {number} time
     * @private
     */
    _applyBones(time) {
        const anim = this.currentAnimation;
        if (!anim) return;

        for (const [boneName, boneAnim] of anim.bones) {
            const bone = this.boneMap.get(boneName);
            if (!bone || !bone.sceneObject) continue;

            const result = interpolateBone(boneAnim, time, this.molang);
            const rest = this._restPoses.get(bone.name);

            // 旋转：叠加到 rest pose（度 → 弧度）
            // Blockbench: bone.rotation.x += Math.degToRad(arr[0])
            // rest pose 旋转已存储为弧度，动画旋转转弧度后相加
            // Euler 顺序 ZYX（与 renderer.js 中骨骼/cube 一致，Blockbench 默认值）
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

            // 位置：叠加到 rest pose（像素单位）
            // Blockbench: bone.position.x += arr[0]
            if (result.position) {
                const restPos = rest ? rest.position : [0, 0, 0];
                bone.sceneObject.position.set(
                    restPos[0] + result.position[0],
                    restPos[1] + result.position[1],
                    restPos[2] + result.position[2]
                );
            }

            // 缩放：乘法叠加到 rest pose
            // Blockbench: bone.scale.x *= arr[0] || 0.00001
            // Bedrock 语义：scale=0 表示隐藏该骨骼（如眨眼时眼皮 Y 缩为 0）
            // 任一分量为 0 时直接 visible=false，避免退化矩阵且语义正确
            if (result.scale) {
                const restScale = rest ? rest.scale : [1, 1, 1];
                const sx = result.scale[0] !== undefined ? result.scale[0] : 1;
                const sy = result.scale[1] !== undefined ? result.scale[1] : 1;
                const sz = result.scale[2] !== undefined ? result.scale[2] : 1;
                if (sx === 0 || sy === 0 || sz === 0) {
                    bone.sceneObject.visible = false;
                } else {
                    bone.sceneObject.visible = true;
                    bone.sceneObject.scale.set(
                        restScale[0] * sx,
                        restScale[1] * sy,
                        restScale[2] * sz
                    );
                }
            }
        }
    }

    /**
     * 捕获所有骨骼的静态姿势
     * @private
     */
    _captureRestPoses() {
        if (this._restCaptured) return;
        this._restPoses.clear();
        this.outliner.traverseBones(bone => {
            if (bone.sceneObject) {
                this._restPoses.set(bone.name, {
                    position: [bone.sceneObject.position.x, bone.sceneObject.position.y, bone.sceneObject.position.z],
                    rotation: [bone.sceneObject.rotation.x, bone.sceneObject.rotation.y, bone.sceneObject.rotation.z],
                    scale: [bone.sceneObject.scale.x, bone.sceneObject.scale.y, bone.sceneObject.scale.z],
                    visible: bone.sceneObject.visible
                });
            }
        });
        this._restCaptured = true;
    }

    /**
     * 恢复所有骨骼到静态姿势
     * @private
     */
    _restoreRestPoses() {
        for (const [name, pose] of this._restPoses) {
            const bone = this.boneMap.get(name);
            if (bone && bone.sceneObject) {
                bone.sceneObject.position.set(pose.position[0], pose.position[1], pose.position[2]);
                bone.sceneObject.rotation.set(pose.rotation[0], pose.rotation[1], pose.rotation[2]);
                bone.sceneObject.scale.set(pose.scale[0], pose.scale[1], pose.scale[2]);
                bone.sceneObject.visible = pose.visible;
            }
        }
    }

    /**
     * 触发事件
     * @private
     */
    _emit(type, name) {
        for (const cb of this.onAnimationEvent) {
            try {
                cb(type, name);
            } catch (e) {
                console.warn('[AnimationRuntime] 事件回调异常:', e);
            }
        }
    }

    /**
     * 获取当前播放进度（0-1）
     */
    get progress() {
        if (!this.currentAnimation || !this.currentAnimation.animationLength) return 0;
        return Math.min(1, this.currentTime / this.currentAnimation.animationLength);
    }

    /**
     * 获取当前动画名
     */
    get currentAnimationName() {
        return this.currentAnimation ? this.currentAnimation.name : '';
    }
}
