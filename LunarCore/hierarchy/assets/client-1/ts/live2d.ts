import { fetchLive2DSetting } from './api';
import type { Live2DModelConfig, EmotionalState } from './types';

declare global {
    interface Window {
        PIXI: any;
        live2d: any;
    }
}

let pixiJSExample: any = null;
let Live2DExample: any = null;
let currentLive2DModel: Live2DModelConfig | null = null;
let currentEmotionState: EmotionalState = 'IDLE';

export const EmotionalState = {
    IDLE: 'IDLE',
    THINKING: 'THINKING',
    AWAIT: 'AWAIT',
    SPEAKING: 'SPEAKING',
    HAPPY: 'HAPPY',
    SAD: 'SAD',
    ANGRY: 'ANGRY',
} as const;

export function getLive2DContainer(): HTMLElement | null {
    return document.getElementById('live2dContainer');
}

export function getLive2DCanvas(): HTMLCanvasElement | null {
    return document.getElementById('live2dCanvas') as HTMLCanvasElement;
}

export function getModelIntelElement(): HTMLElement | null {
    return document.getElementById('modelIntel');
}

function waitForPIXI(): Promise<void> {
    return new Promise((resolve) => {
        const check = () => {
            if ((window as any).PIXI) {
                resolve();
            } else {
                setTimeout(check, 100);
            }
        };
        check();
    });
}

async function loadLive2DPlugin(): Promise<void> {
    return new Promise((resolve) => {
        const check = () => {
            if ((window as any).PIXI?.live2d) {
                resolve();
            } else {
                setTimeout(check, 100);
            }
        };
        check();
    });
}

function initApplication(): void {
    if (pixiJSExample) {
        pixiJSExample.destroy(true);
    }

    const container = getLive2DContainer();
    const wasHidden = container?.parentElement?.style.display === 'none';

    if (wasHidden && container?.parentElement) {
        container.parentElement.style.display = 'block';
        container.parentElement.style.visibility = 'hidden';
    }

    const canvas = getLive2DCanvas();
    const parameters = {
        transparent: true,
        width: container?.clientWidth || 0,
        height: container?.clientHeight || 0,
        view: canvas,
        antialias: true,
    };

    pixiJSExample = new (window as any).PIXI.Application(parameters);

    const modelInfo = getModelIntelElement();
    if (modelInfo) {
        modelInfo.textContent = `加载模型: ${currentLive2DModel?.name || '未知'}...`;
    }

    if (wasHidden && container?.parentElement) {
        container.parentElement.style.display = 'none';
        container.parentElement.style.visibility = 'visible';
    }
}

async function loadModel(): Promise<void> {
    const modelInfo = getModelIntelElement();

    try {
        if (Live2DExample) {
            pixiJSExample.stage.removeChild(Live2DExample);
            Live2DExample.destroy();
            Live2DExample = null;
        }

        if (modelInfo) {
            modelInfo.textContent = `加载模型: ${currentLive2DModel?.name || '未知'}...`;
        }

        if (!currentLive2DModel) {
            throw new Error('No Live2D model configured');
        }

        Live2DExample = await (window as any).PIXI.live2d.Live2DModel.from(
            currentLive2DModel.url,
            { autoInteract: currentLive2DModel.autoInteract }
        );

        Live2DExample.scale.set(currentLive2DModel.scale);
        Live2DExample.anchor.set(0.5, 0.5);

        const container = getLive2DContainer();
        Live2DExample.x = (container?.clientWidth || 0) * currentLive2DModel.x;
        Live2DExample.y = (container?.clientHeight || 0) * currentLive2DModel.y;

        pixiJSExample.stage.addChild(Live2DExample);

        if (modelInfo) {
            modelInfo.textContent = currentLive2DModel?.name || '未知';
        }
    } catch (error) {
        if (error instanceof Error) {
            showError(`Live2D 加载失败: ${error.message}`);
            if (modelInfo) {
                modelInfo.textContent = 'Live2D 加载失败';
            }
        }
        throw error;
    }
}

function showError(message: string): void {
    const container = getLive2DContainer();
    if (!container) return;

    const errorDiv = document.createElement('div');
    errorDiv.className = 'live2d-error-message';
    errorDiv.innerHTML = `
        <h2><i class="fas fa-exclamation-triangle"></i> 出错了</h2>
        <p>${message}</p>
        <p>请检查控制台获取详细信息</p>
        <button id="reload-btn" style="margin-top: 20px; padding: 10px 20px; cursor: pointer;">重新加载</button>
    `;

    container?.appendChild(errorDiv);

    document.getElementById('reload-btn')?.addEventListener('click', () => {
        errorDiv.remove();
        initLive2D();
    });
}

export async function initLive2D(): Promise<void> {
    const errorDiv = document.querySelector('.live2d-error-message');
    if (errorDiv) errorDiv.remove();

    try {
        await waitForPIXI();
        await loadLive2DPlugin();
        initApplication();

        currentLive2DModel = await fetchLive2DSetting();
        await loadModel();

        window.addEventListener('resize', reloadLive2DContainer);
        setEmotionState(EmotionalState.IDLE);
        reloadLive2DContainer();
    } catch (error) {
        if (error instanceof Error) {
            showError(`初始化失败: ${error.message}`);
        }
        throw error;
    }
}

export function reloadLive2DContainer(): void {
    const container = getLive2DContainer();
    if (!container) return;

    if (pixiJSExample) {
        pixiJSExample.renderer.resize(container.clientWidth, container.clientHeight);
    }

    if (Live2DExample && currentLive2DModel) {
        const scale = container.clientHeight < 500
            ? currentLive2DModel.scale * 0.65
            : currentLive2DModel.scale;

        Live2DExample.scale.x = scale;
        Live2DExample.scale.y = scale;
        Live2DExample.x = container.clientWidth * currentLive2DModel.x;
        Live2DExample.y = container.clientHeight * currentLive2DModel.y;
    }
}

export function setEmotionState(state: EmotionalState): void {
    currentEmotionState = state;

    if (Live2DExample && Live2DExample.motion) {
        const motionMap: Record<EmotionalState, string | null> = {
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
                Live2DExample.motion(motion);
            } catch (e) {
                console.warn(`Motion ${motion} not available:`, e);
            }
        }
    }
}

export function getCurrentEmotionState(): EmotionalState {
    return currentEmotionState;
}

export function setStateWithTimeout(state: string, duration: number = 9000): void {
    setEmotionState(state as EmotionalState);

    if (state !== EmotionalState.IDLE && state !== EmotionalState.THINKING) {
        setTimeout(() => {
            if (currentEmotionState === state) {
                setEmotionState(EmotionalState.IDLE);
            }
        }, duration);
    }
}

export function getLive2DModel(): any {
    return Live2DExample;
}

export function isLive2DReady(): boolean {
    return Live2DExample !== null;
}