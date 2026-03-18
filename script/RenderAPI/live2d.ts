import { get } from 'http';
import * as EntryAPI from '../EntryAPI/code';

/** 定义 PIXI 应用程序实例，用于管理 Live2D 模型的渲染和显示 */
let pixiJSExample: any | null = null;

/** 定义 Live2D 模型实例，用于管理 Live2D 模型的行为和状态 */
let Live2DExample: any | null = null;

/** 当前加载的 Live2D 模型 */
let currentLive2DModel: any | null = null;

/** 当前情绪状态 */
let currentEmotionState: string | null = null;

/**
 * 基础情绪状态类，定义各种情绪状态的英文标识
 *
 * 提供静态属性返回对应的英文状态字符串
 */
class BaseEmotionalState {
    /**
     * 待机状态的英文标识
     * @returns {string} 待机状态标识 "idle"
     */
    static get IDLE(): string { return "idle"; }
    /**
     * 思考状态的英文标识
     * @returns {string} 思考状态标识 "thinking"
     */
    static get THINKING(): string { return "thinking"; }
    /**
     * 说话状态的英文标识
     * @returns {string} 说话状态标识 "speaking"
     */
    static get SPEAKING(): string { return "speaking"; }
    /**
     * 错误状态的英文标识
     * @returns {string} 错误状态标识 "error"
     */
    static get ERROR(): string { return "error"; }
    /**
     * 等待状态的英文标识
     * @returns {string} 等待状态标识 "await"
     */
    static get AWAIT(): string { return "await"; }
    /**
     * 开心状态的英文标识
     * @returns {string} 开心状态标识 "happy"
     */
    static get HAPPY(): string { return "happy"; }
    /**
     * 生气状态的英文标识
     * @returns {string} 生气状态标识 "angry"
     */
    static get ANGRY(): string { return "angry"; }
    /**
     * 害羞状态的英文标识
     * @returns {string} 害羞状态标识 "shy"
     */
    static get SHY(): string { return "shy"; }
    /**
     * 疑问状态的英文标识
     * @returns {string} 疑问状态标识 "question"
     */
    static get QUESTION(): string { return "question"; }
    /**
     * 无语状态的英文标识
     * @returns {string} 无语状态标识 "speechless"
     */
    static get SPEECHLESS(): string { return "speechless"; }
    /**
     * 悲伤状态的英文标识
     * @returns {string} 悲伤状态标识 "sad"
     */
    static get SAD(): string { return "sad"; }
    /**
     * 抵触状态的英文标识
     * @returns {string} 抵触状态标识 "resist"
     */
    static get RESIST(): string { return "resist"; }
    /**
     * 忍耐状态的英文标识
     * @returns {string} 忍耐状态标识 "patience"
     */
    static get PATIENCE(): string { return "patience"; }
    /**
     * 疲惫状态的英文标识
     * @returns {string} 疲惫状态标识 "tired"
     */
    static get TIRED(): string { return "tired"; }
    /**
     * 轻蔑状态的英文标识
     * @returns {string} 轻蔑状态标识 "contempt"
     */
    static get CONTEMPT(): string { return "contempt"; }
    /**
     * 尴尬状态的英文标识
     * @returns {string} 尴尬状态标识 "embarrassed"
     */
    static get EMBARRASSED(): string { return "embarrassed"; }
    /**
     * 困倦状态的英文标识
     * @returns {string} 困倦状态标识 "sleepy"
     */
    static get SLEEPY(): string { return "sleepy"; }
    /**
     * 分心状态的英文标识
     * @returns {string} 分心状态标识 "distracted"
     */
    static get DISTRACTED(): string { return "distracted"; }
};

/**
 * 情绪状态枚举类，继承自 BaseEmotionalState
 *
 * 用于表示角色的各种情绪状态，提供对应状态的中文描述
 *
 * 部分属性为情绪标签
 */
export class EmotionalState extends BaseEmotionalState {
    /**
     * 待机状态的中文描述
     * @returns {string} 待机状态描述 "待机中"
     */
    static get [BaseEmotionalState.IDLE](): string { return "待机中" };
    /**
     * 思考状态的中文描述
     * @returns {string} 思考状态描述 "思考中..."
     */
    static get [BaseEmotionalState.THINKING](): string { return "思考中..." };
    /**
     * 说话状态的中文描述
     * @returns {string} 说话状态描述 "说话中"
     */
    static get [BaseEmotionalState.SPEAKING](): string { return "说话中" };
    /**
     * 错误状态的中文描述
     * @returns {string} 错误状态描述 "出错了"
     */
    static get [BaseEmotionalState.ERROR](): string { return "出错了" };
    /**
     * 等待状态的中文描述
     * @returns {string} 等待状态描述 "等待"
     */
    static get [BaseEmotionalState.AWAIT](): string { return "等待" };
    /**
     * 开心状态的中文描述
     * @returns {string} 开心状态描述 "开心"
     */
    static get [BaseEmotionalState.HAPPY](): string { return "开心" };
    /**
     * 生气状态的中文描述
     * @returns {string} 生气状态描述 "生气"
     */
    static get [BaseEmotionalState.ANGRY](): string { return "生气" };
    /**
     * 害羞状态的中文描述
     * @returns {string} 害羞状态描述 "害羞"
     */
    static get [BaseEmotionalState.SHY](): string { return "害羞" };
    /**
     * 疑问状态的中文描述
     * @returns {string} 疑问状态描述 "疑问"
     */
    static get [BaseEmotionalState.QUESTION](): string { return "疑问" };
    /**
     * 无语状态的中文描述
     * @returns {string} 无语状态描述 "无语"
     */
    static get [BaseEmotionalState.SPEECHLESS](): string { return "无语" };
    /**
     * 悲伤状态的中文描述
     * @returns {string} 悲伤状态描述 "悲伤"
     */
    static get [BaseEmotionalState.SAD](): string { return "悲伤" };
    /**
     * 抵触状态的中文描述
     * @returns {string} 抵触状态描述 "抵触"
     */
    static get [BaseEmotionalState.RESIST](): string { return "抵触" };
    /**
     * 忍耐状态的中文描述
     * @returns {string} 忍耐状态描述 "忍耐"
     */
    static get [BaseEmotionalState.PATIENCE](): string { return "忍耐" };
    /**
     * 疲惫状态的中文描述
     * @returns {string} 疲惫状态描述 "疲惫"
     */
    static get [BaseEmotionalState.TIRED](): string { return "疲惫" };
    /**
     * 轻蔑状态的中文描述
     * @returns {string} 轻蔑状态描述 "轻蔑"
     */
    static get [BaseEmotionalState.CONTEMPT](): string { return "轻蔑" };
    /**
     * 尴尬状态的中文描述
     * @returns {string} 尴尬状态描述 "尴尬"
     */
    static get [BaseEmotionalState.EMBARRASSED](): string { return "尴尬" };
    /**
     * 困倦状态的中文描述
     * @returns {string} 困倦状态描述 "困倦"
     */
    static get [BaseEmotionalState.SLEEPY](): string { return "困倦" };
    /**
     * 分心状态的中文描述
     * @returns {string} 分心状态描述 "分心"
     */
    static get [BaseEmotionalState.DISTRACTED](): string { return "分心" };
    /** 开发者 */
    public static developer: string = 'TayunStarry';
    /**
     * 获取所有大写get函数名的数组
     * @returns {Array<string>} 包含所有大写get函数名的数组
     */
    static getAllUppercaseGetters(): Array<string> {
        return Object.getOwnPropertyNames(BaseEmotionalState).filter(prop => /^[A-Z_]+$/.test(prop));
    }
};

/**
 * 等待 PIXI 对象加载完成的函数
 *
 * 该函数会返回一个 Promise，当 window.PIXI 对象可用时，Promise 会被 resolve
 *
 * 每 100 毫秒检查一次 PIXI 对象是否已加载
 *
 * @returns {Promise} 当 PIXI 对象加载完成时 resolve 的 Promise
 */
function waitForPIXI(): Promise<any> {
    return new Promise(
        resolve => {
            /**
             * 定义检查函数，若 PIXI 对象已加载则 resolve，否则 100 毫秒后再次检查
             */
            const check = () => { if ((window as any).PIXI) resolve(void 0); else setTimeout(check, 100); };
            // 立即执行第一次检查
            check();
        }
    );
};

/**
 * 等待 Pixi-Live2D 插件加载完成的异步函数
 *
 * 该函数会返回一个 Promise，当 window.PIXI.live2d 对象可用时，Promise 会被 resolve
 * 每 100 毫秒检查一次 Pixi-Live2D 插件是否已加载
 *
 * @returns {Promise} 当 Pixi-Live2D 插件加载完成时 resolve 的 Promise
 */
async function loadLive2DPlugin(): Promise<any> {
    return new Promise(
        resolve => {
            /**
             * 定义检查函数，若 Pixi-Live2D 插件已加载则 resolve，否则 100 毫秒后再次检查
             */
            const check = () => { if ((window as any).PIXI?.live2d) resolve(void 0); else setTimeout(check, 100); };
            // 立即执行第一次检查
            check();
        }
    );
};

/**
 * 初始化 PIXI 应用程序
 *
 * 此函数用于初始化 PIXI 应用程序实例，会先检查是否已有应用存在，若存在则销毁，
 *
 * 然后根据 live2dContainer 的尺寸创建新的 PIXI 应用，并设置相应的参数。
 *
 * 最后在页面上显示当前正在加载的模型信息。
 */
function initApplication() {
    // 如果应用已存在，销毁它以避免冲突
    if (pixiJSExample) pixiJSExample.destroy(true);
    /**
     * 临时显示容器获取尺寸
     */
    const wasHidden = EntryAPI.live2dContainer?.parentElement?.style.display === 'none';
    // 临时显示容器获取尺寸
    if (wasHidden) {
        EntryAPI.live2dContainer.parentElement.style.display = 'block';
        EntryAPI.live2dContainer.parentElement.style.visibility = 'hidden';
    }
    /**
     * 配置 PIXI 应用程序的参数
     */
    const parameters = {
        // 设置画布背景透明
        transparent: true,
        // 设置画布宽度为容器的宽度
        width: EntryAPI.live2dContainer?.clientWidth || 0,
        // 设置画布高度为容器的高度
        height: EntryAPI.live2dContainer?.clientHeight || 0,
        // 指定使用的 canvas 元素
        view: document.getElementById('live2dCanvas'),
        // 开启抗锯齿以提高渲染质量
        antialias: true
    };
    // 创建新的 PIXI 应用程序实例
    pixiJSExample = new (window as any).PIXI.Application(parameters);
    /**
     * 获取用于显示模型加载信息的元素
     */
    const modelInfo = document.querySelector('.live2d-model-intel');
    // 在页面上显示当前正在加载的模型名称
    if (modelInfo) modelInfo.textContent = `加载模型: ${currentLive2DModel?.name || '未知'}...`;
    // 恢复临时显示容器的显示状态
    if (wasHidden) {
        EntryAPI.live2dContainer.parentElement.style.display = 'none';
        EntryAPI.live2dContainer.parentElement.style.visibility = 'visible';
    }
};

/**
 * 加载 Live2D 模型的异步函数
 *
 * 该函数负责加载指定的 Live2D 模型，处理模型加载前的清理工作，
 *
 * 设置模型的属性，将模型添加到舞台，并处理交互和呼吸动画。
 *
 * 若加载失败，会捕获错误并显示错误信息。
 */
async function loadModel() {
    /**
     * 获取用于显示模型加载信息的元素
     */
    const modelInfo = document.querySelector('.live2d-model-intel');
    // 尝试加载Live2D模型
    try {
        // 如果已有Live2D模型存在，从舞台移除该模型并销毁，然后将模型引用置为 null
        if (Live2DExample) {
            pixiJSExample.stage.removeChild(Live2DExample);
            Live2DExample.destroy();
            Live2DExample = null;
        }
        // 在页面上显示当前正在加载的Live2D名称
        if (modelInfo) modelInfo.textContent = `加载模型: ${currentLive2DModel?.name || '未知'}...`;
        // 异步加载指定路径的 Live2D 模型
        Live2DExample = await (window as any).PIXI.live2d.Live2DModel.from(currentLive2DModel.url, { autoInteract: currentLive2DModel.autoInteract });
        // 设置模型的缩放比例
        Live2DExample.scale.set(currentLive2DModel.scale);
        // 设置模型的锚点为中心点
        Live2DExample.anchor.set(0.5, 0.5);
        // 根据容器宽度和配置参数设置模型的 x 坐标
        Live2DExample.x = EntryAPI.live2dContainer?.clientWidth || 0 * currentLive2DModel.x;
        // 根据容器高度和配置参数设置模型的 y 坐标
        Live2DExample.y = EntryAPI.live2dContainer?.clientHeight || 0 * currentLive2DModel.y;
        // 将加载好的模型添加到 PIXI 舞台
        pixiJSExample.stage.addChild(Live2DExample);
        // 在页面上显示当前已加载完成的模型名称
        if (modelInfo) modelInfo.textContent = currentLive2DModel?.name || '未知';
    }
    catch (error) {
        if (error instanceof Error) {
            // 捕获异常并显示错误信息
            EntryAPI.showSystemMessage(`${error.name} | ${error.message} | ${error.stack}`, "error");
            // 在页面上显示 Live2D 加载失败的信息
            if (modelInfo) modelInfo.textContent = "Live2D 加载失败";
            // 调用 showError 函数显示具体的错误信息
            showError(`Live2D 加载失败: ${error.message}`);
        }
    }
};

/**
 * 显示错误信息的函数
 *
 * 该函数会在 Live2D 模型容器中创建一个错误提示框，显示错误信息，并提供重新加载按钮。
 *
 * @param {string} message - 需要显示的错误信息
 */
function showError(message: string) {
    /**
     * 错误提示框的样式
     */
    const errorDiv = document.createElement('div');
    // 设置错误信息 div 的类名，便于样式控制
    errorDiv.className = 'live2d-error-message';
    // 设置错误信息 div 的 HTML 内容，包含错误图标、错误信息、提示语和重新加载按钮
    errorDiv.innerHTML = `
                <h2><i class="fas fa-exclamation-triangle"></i> 出错了</h2>
                <p>${message}</p>
                <p>请检查控制台获取详细信息</p>
                <button id="reload-btn" style="margin-top: 20px; padding: 10px 20px; cursor: pointer;">重新加载</button>
            `;
    // 将错误信息 div 添加到 Live2D 模型容器中
    EntryAPI.live2dContainer?.appendChild(errorDiv);
    // 添加重新加载按钮事件，点击按钮时调用 initLive2D 函数重新初始化 Live2D 模型
    document.getElementById('reload-btn')?.addEventListener('click', initLive2D);
};

/**
 * 初始化 Live2D 模型的异步函数
 *
 * 此函数负责完成 Live2D 模型的初始化工作，包括清除错误信息、等待 PIXI 加载、
 *
 * 初始化 PIXI 应用、加载 Live2D 模型，并设置窗口大小变化的响应事件。
 *
 * 若初始化过程中出现错误，会捕获错误并显示错误信息。
 */
export async function initLive2D() {
    try {
        /**
         * 移除之前显示的错误信息，保证初始化时界面干净
         */
        const errorDiv = document.querySelector('.live2d-error-message');
        // 如果存在错误信息元素，则将其删除
        if (errorDiv) errorDiv.remove();
        // 等待 PIXI 库加载完成，确保后续操作依赖的 PIXI 对象可用
        await waitForPIXI();
        // 检查 Pixi-Live2D 插件是否已加载，并在控制台输出提示信息
        await loadLive2DPlugin();
        // 初始化 PIXI 应用程序，为后续加载模型做准备
        initApplication();
        // 异步加载当前指定的 Live2D 模型
        await loadModel();
        // 为窗口添加大小变化的监听事件，确保窗口大小改变时 Live2D 模型能正确显示
        window.addEventListener('resize', reloadLive2DContainer);
        // 设置初始状态为 IDLE
        setEmotionState(EmotionalState.IDLE);
        // 重新加载 Live2D 模型容器，确保模型在新容器尺寸下正确显示
        reloadLive2DContainer();
    }
    catch (error) {
        if (error instanceof Error) {
            // 捕获异常并显示错误信息
            EntryAPI.showSystemMessage(`${error.name} | ${error.message} | ${error.stack}`, "error");
            // 调用显示错误信息的函数，向用户展示初始化失败的具体原因
            showError(`初始化失败: ${error.message}`);
        }
    }
};
/**
 * 重新加载并调整 Live2D 模型容器
 *
 * 此函数用于在容器尺寸变化时，调整 PIXI 渲染器大小并重新定位模型
 */
export function reloadLive2DContainer() {
    // 检查 live2dContainer 元素是否存在
    if (!EntryAPI.live2dContainer) {
        EntryAPI.showSystemMessage("未找到 live2dContainer 元素", "error");
        return;
    }
    // 如果 PIXI 应用已初始化，调整渲染器的大小以适应容器
    if (pixiJSExample) pixiJSExample.renderer.resize(EntryAPI.live2dContainer.clientWidth, EntryAPI.live2dContainer.clientHeight);
    // 重新定位模型，根据新的容器尺寸和模型配置更新模型位置
    if (Live2DExample && currentLive2DModel) {
        // 根据容器高度调整模型缩放
        const scale = EntryAPI.live2dContainer.clientHeight < 500 ? currentLive2DModel.scale * 0.65 : currentLive2DModel.scale;
        Live2DExample.scale.x = scale;
        Live2DExample.scale.y = scale;
        // 重新定位模型
        Live2DExample.x = EntryAPI.live2dContainer.clientWidth * currentLive2DModel.x;
        Live2DExample.y = EntryAPI.live2dContainer.clientHeight * currentLive2DModel.y;
    }
};

/**
 * 根据表情名称获取对应的动作组名。
 *
 * 该函数会先检查模型是否加载，再验证表情对应的动作组是否存在，
 *
 * 最后从有效动作组中随机选择一个返回。若过程中出现问题会显示错误信息并返回 undefined。
 *
 * @param {string} emotion - 表情名称
 *
 * @returns {string | undefined} - 对应的单个动作组名，若未找到或出现问题则返回 undefined
 */
function emotionToMotion(emotion: string): string | undefined {
    // 检查模型是否已加载，若未加载则显示错误信息并返回 undefined
    if (!Live2DExample) {
        EntryAPI.showSystemMessage('live2D模型未加载，无法获取动作组', "error");
        return undefined;
    }
    /**
     * 获取所有动作组名
     *
     * @type {string[]} - 所有动作组名
     */
    const motionGroups: string[] = Object.keys(Live2DExample.internalModel.motionManager.motionGroups) || [];
    /**
     * 获取当前表情对应的动作组名
     *
     * @type {string[] | undefined} - 当前表情对应的动作组数组
     */
    const currentMotion: string[] | undefined = currentLive2DModel.mapping[emotion];

    // 检查模型是否加载了动作组，若未加载则显示错误信息并返回 undefined
    if (motionGroups.length === 0) {
        EntryAPI.showSystemMessage("当前live2D模型未加载动作组", "error");
        return undefined;
    }
    // 检查当前表情是否定义了动作组，若未定义则显示错误信息并返回 undefined
    if (!currentMotion || currentMotion.length === 0) {
        EntryAPI.showSystemMessage(`表情"${emotion}"未定义动作组`, "error");
        return undefined;
    }
    /**
     * 过滤出当前表情对应的且模型中存在的有效动作组
     */
    const validMotions = currentMotion.filter(motion => motionGroups.includes(motion));
    // 若没有有效动作组，从所有动作组中随机选择一个返回
    if (validMotions.length === 0) return motionGroups[Math.floor(Math.random() * motionGroups.length)];
    // 从有效动作中随机选择一个返回
    return validMotions[Math.floor(Math.random() * validMotions.length)];
};

/**
 * 设置 Live2D 模型的情绪状态
 * 该函数会更新当前情绪状态，更新状态指示器，并根据情绪状态播放对应的动作
 *
 * @param {string} state - 要设置的情绪状态
 */
export function setEmotionState(state: string) {
    // 更新当前情绪状态为传入的状态
    currentEmotionState = state;
    // 调用 updateStatusIndicator 函数更新状态指示器的显示
    updateStatusIndicator(state);
    /**
     * 根据传入的情绪状态获取对应的动作组名
     */
    const motion = emotionToMotion(state);
    /**
     * 获取指定动作组中的动作数量，若无法获取则默认为 0
     */
    const index = Live2DExample?.internalModel?.coreModel?.motionManager?.getMotionGroup(motion).length || 0;
    // 若未获取到有效的动作组名，则直接返回
    if (!motion) return;
    // 在指定动作组中随机选择一个动作并播放
    Live2DExample.motion(motion, Math.floor(Math.random() * index));
};

/**
 * 获取当前 Live2D 模型的情绪状态
 *
 * @returns {string} - 当前情绪状态
 */
export function getEmotionState(): string {
    return currentEmotionState || EmotionalState.IDLE;
}
/**
 * 更新状态指示器
 * @param {string} state - 状态名称，用于更新指示器显示的状态信息
 */
function updateStatusIndicator(state: string) {
    // 若找到状态指示器元素
    if (EntryAPI.emotionStatusPanel) {
        // 设置状态指示器的内部 HTML 结构，包含情绪指示器和状态文本
        EntryAPI.emotionStatusPanel.innerHTML = [
            '<div class="emotion-indicator"></div>',
            `<span>${EmotionalState[state] || ''}</span>`
        ].join('');
        // 重置状态指示器的类名，确保初始状态正确
        EntryAPI.emotionStatusPanel.className = "emotion-status-panel";
        // 根据传入的状态名添加对应的状态类，用于样式控制
        EntryAPI.emotionStatusPanel.classList.add(`status-${state}`);
    }
    // 若未找到状态指示器元素
    else EntryAPI.showSystemMessage("未找到 .emotion-status-panel 元素", "error");
};

/**
 * 设置 Live2D 模型的状态，并在指定时间后恢复为空闲状态
 *
 * 该函数会先检查 EmotionalState 对象是否已定义，若未定义则输出警告信息并终止执行。
 *
 * 然后设置模型为指定状态，若指定状态不是空闲状态和思考状态，则在指定时间后将模型状态恢复为空闲状态。
 *
 * @param {string} state - 要设置的模型状态名称
 *
 * @param {number} [duration=9000] - 状态持续时间，单位为毫秒，默认为 9000 毫秒
 */
export function setStateWithTimeout(state: string, duration: number = 9000) {
    // 设置 Live2D 模型的状态
    setEmotionState(state);
    // 如果当前设置的状态不是空闲状态和思考状态，则在指定时间后尝试恢复为空闲状态
    if (state !== EmotionalState.IDLE && state !== EmotionalState.THINKING) {
        setTimeout(
            () => {
                // 仅当当前状态仍为初始设置的状态时，才将模型状态恢复为空闲状态
                if (currentEmotionState === state) setEmotionState(EmotionalState.IDLE);
            },
            duration
        );
    }
};

/**
 * 异步获取 Live2D 模型的配置文件
 *
 * 该函数会尝试从指定路径获取 Live2D 模型的配置文件，
 * 若获取成功则将配置解析为 JSON 格式并赋值给全局变量 currentLive2DModel，
 * 若获取失败则打印错误信息并将 currentLive2DModel 设置为空对象作为默认配置。
 *
 * @returns {Promise<void>} 此函数不返回实际值，仅更新全局变量 currentLive2DModel
 */
export async function fetchLive2DSetting(): Promise<void> {
    try {
        /**
         * 发起网络请求，获取 Live2D 模型的配置文件
         * 配置文件路径为 '../models/setting.json'
         */
        const response = await fetch('/read/resources/live2d/setting.json');
        // 检查响应状态，若请求失败则抛出包含状态码的错误信息
        if (!response.ok) throw new Error(`HTTP 错误！状态码: ${response.status}`);
        /**
         * 从响应中获取原始文本内容
         * 此文本可能包含注释和单引号，后续需要处理
         */
        const rawText = await response.text();
        /**
         * 剔除注释内容和单引号，确保 JSON 格式的正确性
         */
        const jsonText = EntryAPI.removeCodeComments(rawText);
        // 将处理后的文本解析为 JSON 格式，并赋值给全局变量 currentLive2DModel
        currentLive2DModel = JSON.parse(jsonText);
    }
    catch (error) {
        if (error instanceof Error) {
            // 捕获异常并显示错误信息
            EntryAPI.showSystemMessage(`${error.name} | ${error.message} | ${error.stack}`, "error");
        }
        else EntryAPI.showSystemMessage(`${error}`, "error");
        // 加载失败时，将 currentLive2DModel 设置为空对象作为默认配置，保证程序健壮性
        currentLive2DModel = {};
    }
};