// ==== third-person-control.js — 第三人称越肩操控模式 ====
//
// 月岩引擎的第三人称越肩键盘操作模式（按 C 进入 / 退出）：
//   - 模型面朝与鼠标环绕的视向完全同步（root.rotation.y = 视向 yaw）
//   - W/S：模型 + 摄像头沿模型面朝方向前后移动
//   - A/D：模型左右平移（相对视向）
//   - Space：跳跃（接地 + 冷却，由 CharacterPhysics 独立跳落）
//   - 鼠标拖拽：环绕视角，模型与摄像头同步转向
//   - 停止输入时模型不自动回正旋转；摄像头每帧始终摆在模型后方聚焦角色头部
//
// 角色竖直（跳/落）与"对附近物理体的推挤力"全部由 CharacterPhysics 负责，
// 本控制器只负责水平位置、视向、相机与推挤力的触发（推挤由 cp 按位移速度统一施加）。

import * as THREE from '../vendor/three.module.js';

const KEY_W = 'KeyW';
const KEY_S = 'KeyS';
const KEY_A = 'KeyA';
const KEY_D = 'KeyD';
const KEY_SPRINT_L = 'ShiftLeft';
const KEY_SPRINT_R = 'ShiftRight';
const KEY_JUMP = 'Space';

export class ThirdPersonController {
    /**
     * @param {object} deps 依赖注入
     * @param {import('./renderer.js').Renderer} deps.renderer
     * @param {import('./character-physics.js').CharacterPhysics} deps.characterPhysics
     * @param {import('./molang-runtime.js').MolangRuntime} deps.molang
     * @param {import('./special-animation-runtime.js').SpecialAnimationRuntime} deps.specialAnimRuntime
     * @param {(moving: boolean, sprint: boolean) => void} deps.onMoveStateChange 走路动画驱动
     */
    constructor(deps) {
        this.renderer = deps.renderer;
        this.characterPhysics = deps.characterPhysics;
        this.molang = deps.molang;
        this.specialAnimRuntime = deps.specialAnimRuntime || null;
        this.onMoveStateChange = deps.onMoveStateChange || (() => {});

        /** 当前是否处于越肩操控模式 */
        this.active = false;
        /** 当前按下的键（code 集合） */
        this.keys = new Set();

        // ---- 视向参数（球面相机） ----
        this.yaw = 0;
        this.pitch = 1.1;
        this.distance = 36;
        this.focusHeight = 30;

        // ---- 移动 / 跳跃参数 ----
        this.moveSpeed = 26;
        this.sprintSpeed = 46;
        this.jumpVelocity = 55;

        // 第三人称操控下角色质量（进入时设置）
        this.characterMass = 100;
        this.defaultMass = 100;

        // ---- 鼠标拖拽状态 ----
        this._mouseDown = false;
        this._lastMX = 0;
        this._lastMY = 0;

        this._bindMouse();
    }

    _bindMouse() {
        const canvas = this.renderer.canvas;
        if (!canvas) return;
        this._onPointerDown = (e) => {
            if (!this.active) return;
            this._mouseDown = true;
            this._lastMX = e.clientX;
            this._lastMY = e.clientY;
            canvas.setPointerCapture?.(e.pointerId);
        };
        this._onPointerMove = (e) => {
            if (!this.active || !this._mouseDown) return;
            const dx = e.clientX - this._lastMX;
            const dy = e.clientY - this._lastMY;
            this._lastMX = e.clientX;
            this._lastMY = e.clientY;
            this.yaw -= dx * 0.005;
            this.pitch = Math.max(0.35, Math.min(1.55, this.pitch - dy * 0.005));
        };
        this._onPointerUp = () => { this._mouseDown = false; };
        this._onWheel = (e) => {
            e.preventDefault();
            if (!this.active) return;
            this.distance = Math.max(6, Math.min(90, this.distance * (e.deltaY > 0 ? 1.08 : 0.92)));
        };
        canvas.addEventListener('pointerdown', this._onPointerDown);
        window.addEventListener('pointermove', this._onPointerMove);
        window.addEventListener('pointerup', this._onPointerUp);
        window.addEventListener('pointercancel', this._onPointerUp);
        canvas.addEventListener('wheel', this._onWheel, { passive: false });
    }

    toggle() {
        if (this.active) this.exit();
        else this.enter();
        return this.active;
    }

    enter() {
        if (this.active) return;
        const root = this.renderer.modelRoot;
        if (!root || !this.characterPhysics) return;
        this.active = true;
        this.keys.clear();
        this.renderer.suppressControls = true;
        this.characterPhysics.setMass?.(this.characterMass);
        this.onMoveStateChange(false, false);
        this._syncCamera(root.position);
    }

    exit() {
        if (!this.active) return;
        this.active = false;
        this.keys.clear();
        this._mouseDown = false;
        this.renderer.suppressControls = false;
        this.characterPhysics.setMass?.(this.defaultMass);
        this.specialAnimRuntime?.setMoveState?.(false, false);
        this.specialAnimRuntime?.setJumping?.(false);
        this.onMoveStateChange(false, false);
    }

    setKey(code, isDown) {
        if (!this.active) return;
        if (isDown) this.keys.add(code);
        else this.keys.delete(code);
    }

    tick(dt) {
        if (!this.active) return;
        const root = this.renderer.modelRoot;
        const cp = this.characterPhysics;
        if (!root || !cp) return;

        // 1. 模型面朝与视向同步（停止输入时不自动回正）
        root.rotation.y = this.yaw;

        // 2. 按键合成移动向量（相对视向 yaw）
        const sy = Math.sin(this.yaw);
        const cy = Math.cos(this.yaw);
        const fwd = (this.keys.has(KEY_W) ? 1 : 0) - (this.keys.has(KEY_S) ? 1 : 0);
        const strafe = (this.keys.has(KEY_D) ? 1 : 0) - (this.keys.has(KEY_A) ? 1 : 0);
        let mvx = (-sy) * fwd + cy * strafe;
        let mvz = (-cy) * fwd + (-sy) * strafe;
        const sprint = this.keys.has(KEY_SPRINT_L) || this.keys.has(KEY_SPRINT_R);
        const moving = fwd !== 0 || strafe !== 0;
        if (moving) {
            const len = Math.hypot(mvx, mvz) || 1;
            const speed = sprint ? this.sprintSpeed : this.moveSpeed;
            root.position.x += (mvx / len) * speed * dt;
            root.position.z += (mvz / len) * speed * dt;
        }
        this.onMoveStateChange(moving, moving && sprint);

        // 3. 跳跃：交给 CharacterPhysics 独立跳落（含接地/冷却判定）
        if (this.keys.has(KEY_JUMP) && cp.isGrounded) {
            cp.startJump?.(this.jumpVelocity);
        }

        // 4. 竖直位置由 CharacterPhysics.prePhysicsStep 写入；此处相机跟随
        this._syncCamera(root.position);

        // 5. MoLang 上下文
        this.molang?.updateContext?.({
            is_moving: moving ? 1 : 0,
            is_sprinting: moving && sprint ? 1 : 0,
        });
    }

    /**
     * 将摄像头放到模型后方，聚焦头部（优先头部骨骼，否则兜底基准+focusHeight）
     */
    _syncCamera(pos) {
        const cam = this.renderer.camera;
        if (!cam) return;
        let fx = pos.x, fz = pos.z, fy = pos.y + this.focusHeight;
        const head = this.renderer.getHeadWorldPosition?.();
        if (head && head.y > pos.y) {
            fx = head.x;
            fy = head.y;
            fz = head.z;
        }
        const sinP = Math.sin(this.pitch);
        const cosP = Math.cos(this.pitch);
        cam.position.set(
            fx + this.distance * sinP * Math.sin(this.yaw),
            fy + this.distance * cosP,
            fz + this.distance * sinP * Math.cos(this.yaw)
        );
        cam.lookAt(fx, fy, fz);
    }
}