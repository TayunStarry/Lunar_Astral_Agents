import { SceneManager } from './scene-manager.js';
import { CameraController } from './camera-controller.js';
import { PhysicsManager } from './physics-manager.js';
import { UIManager } from './ui-manager.js';

// ============ 应用入口 ============
class App {
    constructor() {
        const canvas = document.getElementById('render-canvas');
        this.sceneManager = new SceneManager(canvas);
        this.cameraController = new CameraController(this.sceneManager.camera, canvas);
        this.physicsManager = new PhysicsManager(this.sceneManager);
        this.uiManager = new UIManager(this.sceneManager, this.cameraController, this.physicsManager);

        this._lastTime = performance.now();
        this._fpsAccum = 0; this._fpsCount = 0; this._fpsTimer = 0;

        this.animate = this.animate.bind(this);
        this.animate();

        // 检查恢复旧工程
        this._initRecovery();

        console.log('%c『 星月智能 』轻量渲染引擎 已就绪 ✓', 'color:#6c9bcf;font-size:14px;font-weight:bold');
        this.uiManager.showToast('轻量渲染引擎已就绪', 'success');
    }

    async _initRecovery() {
        const choice = await this.uiManager._showRecoveryModal();
        if (!choice) return;

        if (choice === 1) {
            // 继续之前的工程
            try {
                const resp = await fetch('/file/read/package/engine_studio/status.json');
                if (resp.ok) {
                    const data = await resp.json();
                    await this.uiManager._loadSceneData(data);
                    this.uiManager.showToast('已恢复之前的工程', 'success');
                }
            } catch (e) {
                this.uiManager.showToast('恢复失败，使用新工程', 'error');
            }
        } else if (choice === 2) {
            // 新建工程并保存旧工程状态
            try {
                const resp = await fetch('/file/read/package/engine_studio/status.json');
                if (resp.ok) {
                    const blob = await resp.blob();
                    const url = URL.createObjectURL(blob);
                    const a = document.createElement('a');
                    a.href = url;
                    a.download = `scene_backup_${Date.now()}.json`;
                    a.click();
                    URL.revokeObjectURL(url);
                    this.uiManager.showToast('旧工程已保存为文件', 'success');
                }
            } catch (e) { /* 忽略 */ }
        }
        // choice === 3: 直接放弃
    }

    animate() {
        requestAnimationFrame(this.animate);
        const now = performance.now();
        const dt = Math.min((now - this._lastTime) / 1000, 0.1);
        this._lastTime = now;

        this._fpsAccum += 1 / dt; this._fpsCount++; this._fpsTimer += dt;
        if (this._fpsTimer >= 0.5) {
            this.uiManager.updateStatus(Math.round(this._fpsAccum / this._fpsCount));
            this._fpsAccum = 0; this._fpsCount = 0; this._fpsTimer = 0;
        }

        // 关键帧动画播放（带线性插值）
        const sm = this.sceneManager;
        if (sm.isPlaying && sm.keyframes.length > 1) {
            const currentFrame = sm.keyframes[sm._playFrameIdx];
            sm._interpTimer += dt;
            if (sm._interpTimer >= currentFrame.delay) {
                sm._interpTimer -= currentFrame.delay;
                sm._playFrameIdx = (sm._playFrameIdx + 1) % sm.keyframes.length;
                this.uiManager.refresh();
            }
            // 线性插值到下一帧
            const nextIdx = (sm._playFrameIdx + 1) % sm.keyframes.length;
            const t = sm._interpTimer / currentFrame.delay;
            sm.interpolateFrames(sm._playFrameIdx, nextIdx, t);
        }

        this.cameraController.update();
        this.physicsManager.update(dt);
        this.sceneManager.render();
    }
}

document.addEventListener('DOMContentLoaded', () => { new App(); });