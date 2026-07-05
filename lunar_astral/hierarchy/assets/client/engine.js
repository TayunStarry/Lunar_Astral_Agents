// ==== engine.js — 3D渲染引擎入口（生产环境） ====
//
// 职责：
//   - 初始化 Three.js 渲染器 + 动画系统 + 移动控制
//   - 通过 BroadcastChannel 接收主客户端的指令
//   - 实现指令队列（位移优先，动作排队，10秒位移超时）
//   - Raycasting 点击检测（头部/头发/胸部/手臂）
//   - 自动加载模型资源
//   - 默认开启鼠标追踪（锁定摄像头）

import * as THREE from './three.module.js';
import { Renderer } from './core/renderer.js';
import { GeometryLoader } from './core/geometry-loader.js';
import { TextureManager } from './core/texture-manager.js';
import { MolangRuntime } from './core/molang-runtime.js';
import { AnimationCodec } from './core/animation-codec.js';
import { AnimationRuntime } from './core/animation-runtime.js';
import { AnimGroup, AnimGroupRuntime } from './core/anim-group-runtime.js';
import { SpecialAnimationRuntime } from './core/special-animation-runtime.js';
import { MovementController } from './core/movement-controller.js';
import { BodyRotationInterpreter } from './core/body-rotation-interpreter.js';

// ==== 广播频道 ====
const channel = new BroadcastChannel('lunar-astral-renderer');

// ==== 核心模块 ====
let renderer = null;
const textureManager = new TextureManager();
const molang = new MolangRuntime();
let animationRuntime = null;
let animGroupRuntime = null;
let specialAnimRuntime = null;
let movementController = null;
let bodyRotationInterpreter = null;

// ==== 状态 ====
let currentModel = null;
/** @type {Map<string, import('./core/keyframe.js').Animation>} 动画全名 → Animation */
const currentAnimations = new Map();

// ==== 指令队列 ====
const commandQueue = [];
let isExecutingCommand = false;
let movementTimeout = null;

// ==== 动作定义（可扩展） ====
// 每个动作定义其动画组名和是否需要鼠标追踪
const ACTION_DEFINITIONS = {
    '荡秋千': { group: '荡秋千', mouseTracking: true },
    '翻花绳': { group: '翻花绳', mouseTracking: true },
    // 后续可扩展：
    // '射击': { group: '射击', mouseTracking: false },
    // '快速射击': { group: '快速射击', mouseTracking: false },
    // '长枪蓄力': { group: '长枪蓄力', mouseTracking: false },
    // '睡觉': { group: '睡觉', mouseTracking: false },
};

// ==== 点击检测 — 骨骼→部位映射 ====
const BONE_PART_MAP = {
    'headCheek': '头部',
    'RightLongHair': '头发',
    'LeftLongHair': '头发',
    'whole': '胸部',
    'RightArm': '右臂',
    'LeftArm': '左臂',
    'RightForeArm': '右臂',
    'LeftForeArm': '左臂',
    'RightHand': '右手',
    'LeftHand': '左手',
    'RightLeg': '右腿',
    'LeftLeg': '左腿',
};

// ==== Raycaster ====
const raycaster = new THREE.Raycaster();
const mouseNDC = new THREE.Vector2();

// ==== 初始化 ====
async function init() {
    const canvas = document.getElementById('render-canvas');

    // 1. 创建渲染器（移除登场动画，移除网格）
    renderer = new Renderer(canvas);
    renderer.gridHelper.visible = false; // 隐藏网格
    renderer.resize();
    renderer.start();

    // 2. 创建动画运行时
    animationRuntime = new AnimationRuntime(molang, null);

    // 3. 创建动画组运行时
    animGroupRuntime = new AnimGroupRuntime(molang, animationRuntime);
    animGroupRuntime.animations = currentAnimations;

    // 4. 创建特殊动画运行时
    specialAnimRuntime = new SpecialAnimationRuntime(molang, animationRuntime);

    // 5. 创建移动控制器
    movementController = new MovementController({
        renderer,
        molang,
        onMoveStateChange: (isMoving, isFastMoving) => {
            specialAnimRuntime?.setMoveState(isMoving, isFastMoving);
        }
    });

    // 6. 创建身体旋转解释器
    bodyRotationInterpreter = new BodyRotationInterpreter(molang, renderer, null);

    // 7. 注册移动方向同步
    movementController.onSetTarget(() => {
        bodyRotationInterpreter?.syncToTarget();
    });

    // 8. 注册闲置回调
    movementController.onIdle(() => {
        renderer?.moveCameraToFront(1.5);
    });

    // 9. 挂载渲染循环
    renderer.onUpdate = (dt) => {
        animGroupRuntime.tick(dt);
        specialAnimRuntime?.tick(dt);
        movementController?.tick(dt);
        bodyRotationInterpreter?.tick(dt);
    };

    // 10. 自动加载资源
    await autoLoadResources();

    // 11. 默认开启鼠标追踪（锁定摄像头）
    movementController.setMouseTracking(true);

    // 12. 绑定点击检测
    canvas.addEventListener('click', onCanvasClick);

    // 13. 窗口大小变化
    window.addEventListener('resize', () => renderer?.resize());

    // 14. 监听 BroadcastChannel 指令
    channel.onmessage = (event) => {
        const msg = event.data;
        if (msg && msg.type) {
            handleCommand(msg);
        }
    };

    console.log('[Engine] 3D渲染引擎初始化完成');
}

// ==== 自动加载资源 ====
async function autoLoadResources() {
    const baseURL = './model/';
    try {
        // 1. 获取清单
        const manifestResp = await fetch(baseURL + 'manifest.json');
        if (!manifestResp.ok) {
            console.warn('[Engine] 未找到 manifest.json');
            return;
        }
        const manifest = await manifestResp.json();

        // 2. 加载模型
        if (manifest.model) {
            const modelResp = await fetch(baseURL + manifest.model);
            if (modelResp.ok) {
                const modelJson = await modelResp.json();
                await applyModel(GeometryLoader.parse(modelJson));
            }
        }

        // 3. 加载动画
        if (Array.isArray(manifest.animations)) {
            for (const animPath of manifest.animations) {
                try {
                    const resp = await fetch(baseURL + animPath);
                    if (resp.ok) {
                        const json = await resp.json();
                        const anims = AnimationCodec.parse(json);
                        for (const [name, anim] of anims) {
                            currentAnimations.set(name, anim);
                        }
                    }
                } catch (e) {
                    console.warn(`[Engine] 动画加载失败: ${animPath}`, e);
                }
            }
        }

        // 4. 创建默认动画组并启动
        createDefaultAnimGroup();

        // 5. 通知特殊动画运行时加载分类
        specialAnimRuntime?.setAnimations(currentAnimations);

        // 6. 加载动画组配置
        await loadAnimGroupConfig(baseURL);

        console.log(`[Engine] 已加载 ${currentAnimations.size} 个动画`);
    } catch (err) {
        console.error('[Engine] 自动加载失败:', err);
    }
}

// ==== 应用模型 ====
async function applyModel(result) {
    currentModel = result;
    await textureManager.loadFromBBModel(result.textures);
    renderer.buildModel(result.outliner, textureManager);
    animationRuntime.outliner = result.outliner;
    animationRuntime.rebuild();

    // 更新身体旋转解释器骨骼引用
    if (bodyRotationInterpreter) {
        bodyRotationInterpreter.outliner = result.outliner;
        bodyRotationInterpreter.findBones();
    }
}

// ==== 创建默认动画组 ====
function createDefaultAnimGroup() {
    if (currentAnimations.size === 0) return;

    if (animGroupRuntime.defaultGroup) {
        animGroupRuntime.defaultGroup.animations = Array.from(currentAnimations.keys());
    } else {
        const defaultGroup = new AnimGroup('默认组');
        defaultGroup.isDefault = true;
        defaultGroup.loopMode = 'repeat';
        defaultGroup.transitionDuration = 0.3;
        defaultGroup.animations = Array.from(currentAnimations.keys());
        animGroupRuntime.addGroup(defaultGroup);
    }
    animGroupRuntime.play();
}

// ==== 加载动画组配置 ====
async function loadAnimGroupConfig(baseURL) {
    try {
        const resp = await fetch(baseURL + 'anim_group_config.json');
        if (!resp.ok) return;
        const json = await resp.json();
        if (json && json.animation_groups) {
            animGroupRuntime.importConfig(json, currentAnimations);
        }
    } catch (e) {
        // 配置文件不存在是正常的
    }
}

// ==== BroadcastChannel 指令处理 ====
function handleCommand(msg) {
    switch (msg.type) {
        case 'action':
            enqueueCommand({ type: 'action', action: msg.action });
            break;
        case 'movement':
            enqueueCommand({
                type: 'movement',
                position: msg.position,
                resumeTracking: msg.resumeTracking !== false
            });
            break;
        case 'mouse_tracking':
            if (movementController) {
                movementController.setMouseTracking(msg.enabled);
            }
            break;
    }
}

// ==== 指令队列 ====
function enqueueCommand(cmd) {
    // 如果队列中存在位移指令，忽视其他指令
    if (cmd.type === 'movement') {
        // 位移指令：清空队列中所有非位移指令，添加位移指令
        commandQueue.length = 0;
        commandQueue.push(cmd);
    } else {
        // 非位移指令：如果队列中有位移指令，忽略
        const hasMovement = commandQueue.some(c => c.type === 'movement');
        if (hasMovement) return;
        commandQueue.push(cmd);
    }
    processQueue();
}

function processQueue() {
    if (isExecutingCommand || commandQueue.length === 0) return;
    const cmd = commandQueue.shift();
    executeCommand(cmd);
}

async function executeCommand(cmd) {
    isExecutingCommand = true;

    try {
        switch (cmd.type) {
            case 'action':
                await executeAction(cmd.action);
                break;
            case 'movement':
                await executeMovement(cmd.position, cmd.resumeTracking);
                break;
        }
    } catch (e) {
        console.warn('[Engine] 指令执行异常:', e);
    } finally {
        isExecutingCommand = false;
        processQueue();
    }
}

// ==== 执行动作指令 ====
async function executeAction(actionName) {
    const def = ACTION_DEFINITIONS[actionName];
    if (!def) {
        console.warn(`[Engine] 未知动作: ${actionName}`);
        return;
    }

    // 移动时不执行动作（位移优先）
    if (movementController?.isMoving) return;

    // 设置鼠标追踪状态
    if (movementController) {
        movementController.setMouseTracking(def.mouseTracking);
    }

    // 激活动画组
    if (animGroupRuntime) {
        animGroupRuntime.activateGroup(def.group);
    }

    // 通知主客户端动作已执行
    channel.postMessage({ type: 'action_started', action: actionName });
}

// ==== 执行位移指令 ====
async function executeMovement(position, resumeTracking) {
    if (!movementController) return;

    // 位移期间关闭鼠标追踪
    movementController.setMouseTracking(false);

    // 设置目标位置
    movementController.setTarget(position.x, position.y, position.z);

    // 设置10秒超时
    return new Promise((resolve) => {
        if (movementTimeout) clearTimeout(movementTimeout);

        const timeoutId = setTimeout(() => {
            // 超时：停在当前位置
            const pos = movementController.currentPosition;
            movementController.setPosition(pos.x, pos.y, pos.z);
            onMovementComplete(resumeTracking);
            resolve();
        }, 10000);

        // 监听到达目标（轮询检测）
        const checkArrival = setInterval(() => {
            if (!movementController.isMoving) {
                clearTimeout(timeoutId);
                clearInterval(checkArrival);
                onMovementComplete(resumeTracking);
                resolve();
            }
        }, 100);
    });
}

function onMovementComplete(resumeTracking) {
    // 位移完成后可选恢复鼠标追踪
    if (resumeTracking && movementController) {
        movementController.setMouseTracking(true);
    }
    // 通知主客户端位移完成
    channel.postMessage({ type: 'movement_complete' });
}

// ==== 点击检测 ====
function onCanvasClick(event) {
    if (!renderer || !currentModel) return;

    const canvas = renderer.canvas;
    const rect = canvas.getBoundingClientRect();
    mouseNDC.x = ((event.clientX - rect.left) / rect.width) * 2 - 1;
    mouseNDC.y = -((event.clientY - rect.top) / rect.height) * 2 + 1;

    raycaster.setFromCamera(mouseNDC, renderer.camera);

    // 检测与模型所有 mesh 的交点（递归搜索 modelRoot 下所有子对象）
    const intersects = raycaster.intersectObjects(renderer.modelRoot.children, true);

    console.log(`[点击检测] NDC(${mouseNDC.x.toFixed(2)}, ${mouseNDC.y.toFixed(2)}), 命中数=${intersects.length}`);

    if (intersects.length === 0) return; // 点击空白区域忽略

    // 找到被点击的骨骼
    const hit = intersects[0];
    console.log(`[点击检测] 命中对象: ${hit.object.name || hit.object.type}, 父级: ${hit.object.parent?.name || 'null'}`);
    const boneName = findBoneNameFromObject(hit.object);
    console.log(`[点击检测] 骨骼名: ${boneName || '未找到'}`);
    if (!boneName) return;

    // 映射到身体部位
    const part = BONE_PART_MAP[boneName];
    if (!part) return;

    // 通知主客户端（用于生成触摸提示词发送给AI）
    console.log(`[点击检测] 部位: ${part}, 发送 body_click`);
    channel.postMessage({ type: 'body_click', part, boneName });
}

/**
 * 从 Three.js Object3D 向上查找骨骼名称
 * mesh → bone Group → bone.name
 */
function findBoneNameFromObject(obj) {
    let current = obj;
    while (current) {
        // 跳过 modelRoot 和 scene
        if (current === renderer.modelRoot || current === renderer.scene) {
            return null;
        }
        // 检查这个对象是否是骨骼（在 outliner 中注册过的 Group）
        if (current.name && animationRuntime?.boneMap?.has(current.name)) {
            return current.name;
        }
        current = current.parent;
    }
    return null;
}

// ==== 启动 ====
init();
