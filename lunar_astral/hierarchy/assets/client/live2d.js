import { fetchLive2DSetting } from './fetch.js';

// PIXI 相关全局变量
let pixiJSExample = null;
let Live2DModelInstance = null;
let currentLive2DModel = null;
let currentEmotionState = 'IDLE';

// 情绪状态枚举
export const EmotionalStateEnum = {
    IDLE: 'IDLE',
    THINKING: 'THINKING',
    AWAIT: 'AWAIT',
    SPEAKING: 'SPEAKING',
    HAPPY: 'HAPPY',
    SAD: 'SAD',
    ANGRY: 'ANGRY',
};

/**
 * Live2D核心模块
 *
 * 提供Live2D模型加载、渲染、情绪状态管理等功能
 */
export const Live2D = {
    /**
     * 初始化Live2D模块
     *
     * @returns {Promise<void>}
     */
    async init() {
        const errorDiv = document.querySelector('.live2d-error-message');
        if (errorDiv) errorDiv.remove();

        try {
            await this.waitForPIXI();
            await this.loadLive2DPlugin();
            this.initApplication();
            currentLive2DModel = (await fetchLive2DSetting());
            await this.loadModel();
            window.addEventListener('resize', () => this.reloadContainer());
            this.setEmotionState(EmotionalStateEnum.IDLE);
            this.reloadContainer();
        }
        catch (error) {
            if (error instanceof Error) {
                this.showError(`初始化失败: ${error.message}`);
            }
            throw error;
        }
    },

    /**
     * 等待PIXI库加载完成
     *
     * @returns {Promise<void>}
     */
    waitForPIXI() {
        return new Promise((resolve) => {
            const check = () => {
                if (window.PIXI) {
                    resolve();
                } else {
                    setTimeout(check, 100);
                }
            };
            check();
        });
    },

    /**
     * 等待Live2D插件加载完成
     *
     * @returns {Promise<void>}
     */
    loadLive2DPlugin() {
        return new Promise((resolve) => {
            const check = () => {
                if (window.PIXI?.live2d) {
                    resolve();
                } else {
                    setTimeout(check, 100);
                }
            };
            check();
        });
    },

    /**
     * 初始化PIXI应用
     */
    initApplication() {
        if (pixiJSExample) {
            pixiJSExample.destroy(true);
        }
        const container = document.getElementById('live2dContainer');
        const wasHidden = container?.parentElement?.style.display === 'none';
        if (wasHidden && container?.parentElement) {
            container.parentElement.style.display = 'block';
            container.parentElement.style.visibility = 'hidden';
        }
        const canvas = document.getElementById('live2dCanvas');
        const parameters = {
            transparent: true,
            width: container?.clientWidth || 0,
            height: container?.clientHeight || 0,
            view: canvas,
            antialias: true,
        };
        pixiJSExample = new window.PIXI.Application(parameters);
        const modelInfo = document.getElementById('modelIntel');
        if (modelInfo) {
            modelInfo.textContent = `加载模型: ${currentLive2DModel?.name || '未知'}...`;
        }
        if (wasHidden && container?.parentElement) {
            container.parentElement.style.display = 'none';
            container.parentElement.style.visibility = 'visible';
        }
    },

    /**
     * 加载Live2D模型
     *
     * @returns {Promise<void>}
     */
    async loadModel() {
        const modelInfo = document.getElementById('modelIntel');
        try {
            if (Live2DModelInstance) {
                pixiJSExample?.stage.removeChild(Live2DModelInstance);
                Live2DModelInstance.destroy();
                Live2DModelInstance = null;
            }
            if (modelInfo) {
                modelInfo.textContent = `加载模型: ${currentLive2DModel?.name || '未知'}...`;
            }
            if (!currentLive2DModel) {
                throw new Error('No Live2D model configured');
            }
            const model = await window.PIXI.live2d.Live2DModel.from(currentLive2DModel.url, { autoInteract: currentLive2DModel.autoInteract });
            Live2DModelInstance = model;
            model.scale.set(currentLive2DModel.scale);
            model.anchor.set(0.5, 0.5);
            const container = document.getElementById('live2dContainer');
            model.x = (container?.clientWidth || 0) * currentLive2DModel.x;
            model.y = (container?.clientHeight || 0) * currentLive2DModel.y;
            pixiJSExample?.stage.addChild(Live2DModelInstance);
            if (modelInfo) {
                modelInfo.textContent = currentLive2DModel?.name || '未知';
            }
        }
        catch (error) {
            if (error instanceof Error) {
                this.showError(`Live2D 加载失败: ${error.message}`);
                if (modelInfo) {
                    modelInfo.textContent = 'Live2D 加载失败';
                }
            }
            throw error;
        }
    },

    /**
     * 显示错误信息
     *
     * @param {string} message - 错误信息
     */
    showError(message) {
        const container = document.getElementById('live2dContainer');
        if (!container) return;
        const errorDiv = document.createElement('div');
        errorDiv.className = 'live2d-error-message';
        errorDiv.innerHTML = `
			<h2><i class="fas fa-exclamation-triangle"></i> 出错了</h2>
			<p>${message}</p>
			<p>请检查控制台获取详细信息</p>
			<button id="reload-btn" style="margin-top: 20px; padding: 10px 20px; cursor: pointer;">重新加载</button>
		`;
        container.appendChild(errorDiv);
        document.getElementById('reload-btn')?.addEventListener('click', () => {
            errorDiv.remove();
            this.init();
        });
    },

    /**
     * 重新加载容器尺寸
     */
    reloadContainer() {
        const container = document.getElementById('live2dContainer');
        if (!container) return;
        if (pixiJSExample) {
            pixiJSExample.renderer.resize(container.clientWidth, container.clientHeight);
        }
        if (Live2DModelInstance && currentLive2DModel) {
            const scale = container.clientHeight < 500
                ? currentLive2DModel.scale * 0.65
                : currentLive2DModel.scale;
            Live2DModelInstance.scale.x = scale;
            Live2DModelInstance.scale.y = scale;
            Live2DModelInstance.x = container.clientWidth * currentLive2DModel.x;
            Live2DModelInstance.y = container.clientHeight * currentLive2DModel.y;
        }
    },

    /**
     * 设置情绪状态
     *
     * @param {string} state - 情绪状态
     */
    setEmotionState(state) {
        currentEmotionState = state;
        if (Live2DModelInstance && Live2DModelInstance.motion) {
            const motionMap = {
                IDLE: 'idle',
                THINKING: 'thinking',
                AWAIT: 'waiting',
                SPEAKING: 'speaking',
                HAPPY: 'happy',
                SAD: 'sad',
                ANGRY: 'angry',
            };
            const motion = motionMap[state];
            if (motion) {
                try {
                    Live2DModelInstance.motion(motion);
                } catch (e) {
                    console.warn(`Motion ${motion} not available:`, e);
                }
            }
        }
    },

    /**
     * 获取当前情绪状态
     *
     * @returns {string} - 当前情绪状态
     */
    getCurrentEmotionState() {
        return currentEmotionState;
    },

    /**
     * 设置情绪状态并自动恢复
     *
     * @param {string} state - 情绪状态
     * @param {number} [duration=9000] - 持续时间（毫秒）
     */
    setStateWithTimeout(state, duration = 9000) {
        this.setEmotionState(state);
        if (state !== EmotionalStateEnum.IDLE && state !== EmotionalStateEnum.THINKING) {
            setTimeout(() => {
                if (currentEmotionState === state) {
                    this.setEmotionState(EmotionalStateEnum.IDLE);
                }
            }, duration);
        }
    },

    /**
     * 获取Live2D模型实例
     *
     * @returns {Live2DModel | null} - 模型实例
     */
    getModel() {
        return Live2DModelInstance;
    },

    /**
     * 检查Live2D是否准备就绪
     *
     * @returns {boolean} - 是否准备就绪
     */
    isReady() {
        return Live2DModelInstance !== null;
    },
};