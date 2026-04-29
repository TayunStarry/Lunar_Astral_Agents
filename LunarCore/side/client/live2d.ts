import { EmotionalState, Live2DSetting } from './types';
import { fetchLive2DSetting } from './file-handler';

interface Live2DModel {
  scale: {
    set: (scale: number) => void;
    x: number;
    y: number;
  };
  anchor: {
    set: (x: number, y: number) => void;
  };
  x: number;
  y: number;
  motion?: (motionName: string) => void;
  destroy: () => void;
}

interface PIXIApplication {
  stage: {
    removeChild: (child: unknown) => void;
    addChild: (child: unknown) => void;
  };
  renderer: {
    resize: (width: number, height: number) => void;
  };
  destroy: (removeView?: boolean) => void;
}

let pixiJSExample: PIXIApplication | null = null;
let Live2DModelInstance: Live2DModel | null = null;
let currentLive2DModel: Live2DSetting | null = null;
let currentEmotionState: EmotionalState = 'IDLE';

export const EmotionalStateEnum: Record<string, EmotionalState> = {
  IDLE: 'IDLE',
  THINKING: 'THINKING',
  AWAIT: 'AWAIT',
  SPEAKING: 'SPEAKING',
  HAPPY: 'HAPPY',
  SAD: 'SAD',
  ANGRY: 'ANGRY',
};

export const Live2D = {
  async init(): Promise<void> {
    const errorDiv = document.querySelector('.live2d-error-message');
    if (errorDiv) errorDiv.remove();

    try {
      await this.waitForPIXI();
      await this.loadLive2DPlugin();
      this.initApplication();

      currentLive2DModel = (await fetchLive2DSetting()) as Live2DSetting;
      await this.loadModel();

      window.addEventListener('resize', () => this.reloadContainer());
      this.setEmotionState(EmotionalStateEnum.IDLE);
      this.reloadContainer();
    } catch (error) {
      if (error instanceof Error) {
        this.showError(`初始化失败: ${error.message}`);
      }
      throw error;
    }
  },

  waitForPIXI(): Promise<void> {
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

  loadLive2DPlugin(): Promise<void> {
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

  initApplication(): void {
    if (pixiJSExample) {
      pixiJSExample.destroy(true);
    }

    const container = document.getElementById('live2dContainer');
    const wasHidden = container?.parentElement?.style.display === 'none';

    if (wasHidden && container?.parentElement) {
      container.parentElement.style.display = 'block';
      container.parentElement.style.visibility = 'hidden';
    }

    const canvas = document.getElementById('live2dCanvas') as HTMLCanvasElement;
    const parameters = {
      transparent: true,
      width: container?.clientWidth || 0,
      height: container?.clientHeight || 0,
      view: canvas,
      antialias: true,
    };

    pixiJSExample = new window.PIXI.Application(parameters) as PIXIApplication;

    const modelInfo = document.getElementById('modelIntel');
    if (modelInfo) {
      modelInfo.textContent = `加载模型: ${currentLive2DModel?.name || '未知'}...`;
    }

    if (wasHidden && container?.parentElement) {
      container.parentElement.style.display = 'none';
      container.parentElement.style.visibility = 'visible';
    }
  },

  async loadModel(): Promise<void> {
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

      const model = await window.PIXI.live2d.Live2DModel.from(
        currentLive2DModel.url,
        { autoInteract: currentLive2DModel.autoInteract }
      ) as Live2DModel;
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
    } catch (error) {
      if (error instanceof Error) {
        this.showError(`Live2D 加载失败: ${error.message}`);
        if (modelInfo) {
          modelInfo.textContent = 'Live2D 加载失败';
        }
      }
      throw error;
    }
  },

  showError(message: string): void {
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

  reloadContainer(): void {
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

  setEmotionState(state: EmotionalState): void {
    currentEmotionState = state;

    if (Live2DModelInstance && Live2DModelInstance.motion) {
      const motionMap: Record<EmotionalState, string> = {
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

  getCurrentEmotionState(): EmotionalState {
    return currentEmotionState;
  },

  setStateWithTimeout(state: EmotionalState, duration = 9000): void {
    this.setEmotionState(state);

    if (state !== EmotionalStateEnum.IDLE && state !== EmotionalStateEnum.THINKING) {
      setTimeout(() => {
        if (currentEmotionState === state) {
          this.setEmotionState(EmotionalStateEnum.IDLE);
        }
      }, duration);
    }
  },

  getModel(): Live2DModel | null {
    return Live2DModelInstance;
  },

  isReady(): boolean {
    return Live2DModelInstance !== null;
  },
};
