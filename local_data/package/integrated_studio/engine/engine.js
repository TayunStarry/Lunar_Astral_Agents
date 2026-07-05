// ==== engine.js — 复合引擎工作室 · 引擎层入口 ====
//
// 职责：
//   - BroadcastChannel('integrated-studio-bus') 双向通信
//   - 初始化顺序：Renderer → AnimationRuntime → AnimGroupRuntime →
//     SpecialAnimationRuntime → MovementController → BodyRotationInterpreter →
//     PhysicsManager → CharacterPhysics
//   - onUpdate 渲染循环：动画 tick → 角色物理同步 → 物理步进 →
//     移动/旋转解释器 tick → 10Hz 遥测与 molang 广播
//   - 按键：Z 物理调试 / M molang 监控
//   - 点击检测：raycaster + BONE_PART_MAP → body_click 广播
//   - 全局物理：所有图元始终参与物理模拟

import * as THREE from '../vendor/three.module.js';
import * as CANNON from '../vendor/cannon-es.module.js';
import { Renderer } from '../core/renderer.js';
import { GeometryLoader } from '../core/geometry-loader.js';
import { TextureManager } from '../core/texture-manager.js';
import { MolangRuntime } from '../core/molang-runtime.js';
import { AnimationCodec } from '../core/animation-codec.js';
import { AnimationRuntime } from '../core/animation-runtime.js';
import { AnimGroup, AnimGroupRuntime } from '../core/anim-group-runtime.js';
import { SpecialAnimationRuntime } from '../core/special-animation-runtime.js';
import { MovementController } from '../core/movement-controller.js';
import { BodyRotationInterpreter } from '../core/body-rotation-interpreter.js';
import { PhysicsManager } from '../core/physics-manager.js';
import { Primitives } from '../core/primitives.js';
import { CameraController } from '../core/camera-controller.js';
import { CharacterPhysics } from '../core/character-physics.js';

// ==== 常量 ====
const CHANNEL_NAME = 'integrated-studio-bus';
const SOURCE_ENGINE = 'engine';

/** 遥测与 molang 广播频率（Hz） */
const TELEMETRY_HZ = 10;
const MOLANG_HZ = 10;

// ==== URL 模式检测（Q5） ====
const urlParams = new URLSearchParams(window.location.search);
const initialMode = urlParams.get('mode') === 'editor' ? 'editor' : 'embedded';
let currentMode = initialMode;

// 应用 body 类
if (currentMode === 'editor') {
    document.body.classList.add('mode-editor');
}

// ==== 广播频道 ====
const channel = new BroadcastChannel(CHANNEL_NAME);

// ==== 核心模块 ====
let renderer = null;
const textureManager = new TextureManager();
const molang = new MolangRuntime();
let animationRuntime = null;
let animGroupRuntime = null;
let specialAnimRuntime = null;
let movementController = null;
let bodyRotationInterpreter = null;
let physicsManager = null;
let primitives = null;
let cameraController = null;
let characterPhysics = null;

// ==== 状态 ====
let currentModel = null;
/** @type {Map<string, import('../core/keyframe.js').Animation>} */
const currentAnimations = new Map();

/** molang 查询表达式（单条，Q12） */
let molangQuery = null;

/** 物理调试可视化开关 */
let physicsDebugVisible = false;

/** 快速追踪开关（点击画布移动角色） */
let quickTrackEnabled = false;

/** 快速追踪地面标记 */
let quickTrackMarker = null;

/** 指南针相关状态 */
let compassPrevPos = null;
let compassAngle = 0;

/** 遥测与 molang 广播计时器 */
let telemetryTimer = 0;
let molangTimer = 0;

/** 图片资产库：uuid → { uuid, base64, name } */
const imageAssetStore = new Map();

/** 物理禁用模式：开启后新创建图元不自动启用物理化 */
let physicsDisabled = false;

/** FPS 采样（最近 30 秒，用于调试覆盖层显示平均 FPS） */
const _fpsSamples = [];
let _fpsSampleTimer = 0;

/** 当前选中的图元/组合体 ID（用于编辑模式） */
let selectedElementId = null;

/** 编辑模式：translate / rotate / scale */
let editMode = 'translate';

// ==== 指令队列（位移优先，10 秒超时） ====
const commandQueue = [];
let isExecutingCommand = false;
let movementTimeout = null;

// ==== 动作定义 ====
const ACTION_DEFINITIONS = {
    '荡秋千': { group: '荡秋千', mouseTracking: true },
    '翻花绳': { group: '翻花绳', mouseTracking: true },
};

// ==== 点击检测：骨骼→部位映射 ====
const BONE_PART_MAP = {
    'head': '头部',
    'hairBack': '马尾辫',
    'hair': '头发',
    'chest': '胸部',
    'rightArm': '右大臂',
    'leftArm': '左大臂',
    'rightForeArm': '右小臂',
    'leftForeArm': '左小臂',
    'rightHand': '右手',
    'LeftHand': 'leftHand',
    'rightLeg': '右大腿',
    'LeftLeg': '左大腿',
    'rightLowerLeg':'右小腿',
    'leftLowerLeg':'左小腿',
    'leftFoot':'左脚',
    'rightFoot':'右脚',
};

const raycaster = new THREE.Raycaster();
const mouseNDC = new THREE.Vector2();

// ==== 初始化 ====
async function init() {
    const canvas = document.getElementById('render-canvas');

    // 1. 创建渲染器
    renderer = new Renderer(canvas);
    renderer.gridHelper.visible = (currentMode === 'editor'); // 编辑器模式显示网格
    renderer.resize();
    renderer.start();

    // 2. 动画运行时
    animationRuntime = new AnimationRuntime(molang, null);

    // 3. 动画组运行时
    animGroupRuntime = new AnimGroupRuntime(molang, animationRuntime);
    animGroupRuntime.animations = currentAnimations;

    // 4. 特殊动画运行时（扩展了 jump/sneak 钩子 — Q10）
    specialAnimRuntime = new SpecialAnimationRuntime(molang, animationRuntime);

    // 5. 移动控制器（纯面板驱动，无 WASD）
    movementController = new MovementController({
        renderer,
        molang,
        onMoveStateChange: (isMoving, isFastMoving) => {
            specialAnimRuntime?.setMoveState(isMoving, isFastMoving);
        }
    });

    // 6. 身体旋转解释器（target_x 已 clamp — Q9）
    bodyRotationInterpreter = new BodyRotationInterpreter(molang, renderer, null);

    // 7. 注册移动方向同步
    movementController.onSetTarget(() => {
        bodyRotationInterpreter?.syncToTarget();
    });

    // 8. 闲置回调（已移除自动回正视角，改由面板"追踪聚焦"按钮触发）

    // 9. 图元管理器（engine_studio 移植）
    primitives = new Primitives(renderer);

    // 10. 相机控制器（越肩视角，WASD 模式启用）
    cameraController = new CameraController(renderer);

    // 11. 物理管理器（engine_studio 移植，构造签名适配为 renderer）
    physicsManager = new PhysicsManager(renderer);
    physicsManager.isActive = true; // 物理始终开启
    physicsManager._ensureGround(); // 确保地面存在，防止角色物理体无限下落

    // 12. 角色物理桥接（新建模块 — Q2/Q8/Q9/Q10）
    characterPhysics = new CharacterPhysics({
        renderer,
        physicsManager,
        molang,
        gravity: physicsManager.gravity,
    });
    characterPhysics.setSpecialAnimRuntime(specialAnimRuntime);

    // 13. 注入依赖到移动控制器
    movementController.setCharacterPhysics(characterPhysics);
    movementController.setSpecialAnimRuntime(specialAnimRuntime);

    // 14. 跳跃失败3次 → 坐下
    characterPhysics.setOnJumpFailed(() => movementController.sitDown());

    // 14. 挂载渲染循环
    renderer.onUpdate = onUpdate;

    // 14. 自动加载资源
    await autoLoadResources();

    // 14.5 自动加载纹理库到 imageAssetStore
    await autoLoadImageAssets();

    // 15. 角色物理初始化（模型加载后计算静态 AABB — Q8）
    if (currentModel) {
        characterPhysics.attachToModel(renderer.modelRoot);
        // 设置相机跟随目标（WASD 按下时启用越肩视角）
        cameraController?.setFollowTarget?.(renderer.modelRoot);
    }

    // 16. 默认开启鼠标追踪
    movementController.setMouseTracking(true);

    // 17. 绑定事件
    canvas.addEventListener('click', (event) => {
        canvas.focus(); // 确保 iframe 获取焦点以接收按键
        onCanvasClick(event);
    });
    canvas.addEventListener('mousemove', (event) => {
        if (!quickTrackEnabled || !renderer) return;
        const rect = canvas.getBoundingClientRect();
        const ndcX = ((event.clientX - rect.left) / rect.width) * 2 - 1;
        const ndcY = -((event.clientY - rect.top) / rect.height) * 2 + 1;
        updateQuickTrackMarker(ndcX, ndcY);
    });
    window.addEventListener('resize', () => renderer?.resize());
    bindKeyboard();
    bindBlur();

    // 18. 监听 BroadcastChannel
    channel.onmessage = (event) => {
        const msg = event.data;
        if (!msg || msg.source === SOURCE_ENGINE) return; // 忽略自己发的
        handleChannelMessage(msg);
    };

    // 19. 通知基座层引擎已就绪
    broadcast('engine_ready', { mode: currentMode });

    // 20. 隐藏加载遮罩
    hideLoadingMask();

    console.log(`[Engine] 引擎初始化完成（mode=${currentMode}）`);
}

// ==== 渲染循环 ====
function onUpdate(dt) {
    // FPS 采样（每 0.5 秒取一次，保留 30 秒 = 60 个样本）
    _fpsSampleTimer += dt;
    if (_fpsSampleTimer >= 0.5) {
        _fpsSampleTimer = 0;
        _fpsSamples.push(renderer?.fps ?? 0);
        if (_fpsSamples.length > 60) _fpsSamples.shift();
    }

    // 1. 动画系统 tick
    animGroupRuntime?.tick(dt);
    specialAnimRuntime?.tick(dt);

    // 2. 角色物理：模型姿态 → 物理体（同步外部修改，如 WASD 力）
    characterPhysics?.prePhysicsStep(dt);

    // 3. 物理步进
    physicsManager?.update(dt);

    // 4. 角色物理：物理体 → 模型根位置（fixedRotation 保证不被撞翻 — Q9）
    characterPhysics?.syncToModel();

    // 5. 移动控制器与旋转解释器（动画层旋转，target_x 已 clamp）
    movementController?.tick(dt);
    bodyRotationInterpreter?.tick(dt);

    // 6. 相机控制器（跟随角色）
    cameraController?.tick(dt);

    // 7. 遥测广播（10Hz）
    telemetryTimer += dt;
    if (telemetryTimer >= 1 / TELEMETRY_HZ) {
        telemetryTimer = 0;
        broadcastTelemetry();
    }

    // 9. molang 值广播（10Hz，仅当有查询表达式时）
    molangTimer += dt;
    if (molangTimer >= 1 / MOLANG_HZ) {
        molangTimer = 0;
        if (molangQuery) {
            broadcastMolangValue();
        }
    }

    // 10. 调试覆盖层更新
    if (physicsDebugVisible) {
        updateDebugOverlay();
    }

    // 11. 指南针更新（编辑器模式下角色移动时显示）
    updateCompass();
}

// ==== 自动加载资源 ====
async function autoLoadResources() {
    const baseURL = '../model/';
    try {
        const manifestResp = await fetch(baseURL + 'manifest.json');
        if (!manifestResp.ok) {
            console.warn('[Engine] 未找到 manifest.json');
            return;
        }
        const manifest = await manifestResp.json();

        if (manifest.model) {
            const modelResp = await fetch(baseURL + manifest.model);
            if (modelResp.ok) {
                const modelJson = await modelResp.json();
                await applyModel(GeometryLoader.parse(modelJson));
            }
        }

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

        createDefaultAnimGroup();
        specialAnimRuntime?.setAnimations(currentAnimations);
        await loadAnimGroupConfig(baseURL);

        // 广播动画组列表给动画面板
        broadcastAnimationList();

        console.log(`[Engine] 已加载 ${currentAnimations.size} 个动画`);
    } catch (err) {
        console.error('[Engine] 自动加载失败:', err);
    }
}

// ==== 自动加载纹理库 ====
async function autoLoadImageAssets() {
    try {
        const resp = await fetch('/file/read/package/integrated_studio/property/images_config.json');
        if (!resp.ok) {
            console.warn('[Engine] 未找到 images_config.json');
            return;
        }
        const data = await resp.json();
        if (data.images) {
            for (const [uuid, info] of Object.entries(data.images)) {
                imageAssetStore.set(uuid, { uuid, base64: info.base64, name: info.name || uuid });
            }
            console.log(`[Engine] 已加载 ${imageAssetStore.size} 个纹理资产`);
            broadcast('image_assets_list', { images: Array.from(imageAssetStore.values()) });
        }
    } catch (err) {
        console.warn('[Engine] 纹理库加载失败:', err);
    }
}

// ==== 广播动画组列表（供动画面板渲染） ====
function broadcastAnimationList() {
    if (!animGroupRuntime) return;
    const groups = Array.from(animGroupRuntime.groups.values()).map(g => ({
        name: g.name,
        isDefault: !!g.isDefault,
        animationCount: Array.isArray(g.animations) ? g.animations.length : 0,
        animations: Array.isArray(g.animations) ? g.animations.slice() : [],
        boneVisibility: g.boneVisibility ? { ...g.boneVisibility } : {},
        loopMode: g.loopMode || 'repeat',
        transitionDuration: g.transitionDuration ?? 0.3,
        visibilityDelay: g.visibilityDelay ?? 0,
        isActive: animGroupRuntime.activeCustomGroup === g,
    }));
    const allAnims = Array.from(currentAnimations.keys());
    // 提取骨骼名列表（供面板渲染骨骼显隐控制）
    const bones = animationRuntime?.boneMap ? Array.from(animationRuntime.boneMap.keys()) : [];
    broadcast('animation_list', { groups, allAnimations: allAnims, bones });
}

// ==== 广播组合体列表（供元素面板渲染） ====
function broadcastCompoundsList() {
    if (!primitives) return;
    broadcast('compounds_list', { compounds: primitives.getCompounds() });
}

// ==== 保存资产到服务器 ====
async function handleAssetSave(id, name) {
    if (!primitives) return;
    const asset = primitives.exportAsset(id);
    if (!asset) {
        broadcast('asset_op_result', { ok: false, message: '资产导出失败' });
        return;
    }
    if (name) {
        asset.name = name;
    }
    try {
        const json = JSON.stringify(asset, null, 2);
        const safeName = (name || `asset_${id}`).replace(/[^\w\-]/g, '_');
        const relativePath = `package/integrated_studio/model/assets/${safeName}.json`;
        const encodedPath = btoa(unescape(encodeURIComponent(relativePath)));
        const resp = await fetch('/file/write', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'X-File-Name': encodedPath,
                'X-Overwrite': 'true',
            },
            body: json,
        });
        if (resp.ok) {
            broadcast('asset_op_result', { ok: true, message: `资产已保存：${safeName}.json` });
        } else {
            broadcast('asset_op_result', { ok: false, message: `保存失败：HTTP ${resp.status}` });
        }
    } catch (err) {
        broadcast('asset_op_result', { ok: false, message: '保存异常：' + err.message });
    }
}

// ==== 应用模型 ====
async function applyModel(result) {
    currentModel = result;
    await textureManager.loadFromBBModel(result.textures);
    renderer.buildModel(result.outliner, textureManager);
    animationRuntime.outliner = result.outliner;
    animationRuntime.rebuild();

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

// ==== 按键监听 ====
function bindKeyboard() {
    window.addEventListener('keydown', (e) => {
        if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') return;

        const code = e.code;

        // 选中元素编辑控制：WASD/Space/Shift 优先用于编辑
        if (selectedElementId && !movementController?._hasTarget) {
            const dirMap = {
                'KeyW': 'north', 'KeyS': 'south',
                'KeyD': 'east', 'KeyA': 'west',
                'Space': 'up', 'ShiftLeft': 'down', 'ShiftRight': 'down',
            };
            const dir = dirMap[code];
            if (dir) {
                handleChannelMessage({ type: 'element_edit_step', payload: { direction: dir, step: 1 } });
                return; // 不再传递给角色移动控制
            }
        }
        // 模式切换快捷键
        if (code === 'Digit1') { editMode = 'translate'; broadcast('edit_mode_changed', { mode: 'translate' }); return; }
        if (code === 'Digit2') { editMode = 'rotate'; broadcast('edit_mode_changed', { mode: 'rotate' }); return; }
        if (code === 'Digit3') { editMode = 'scale'; broadcast('edit_mode_changed', { mode: 'scale' }); return; }

        switch (code) {
            case 'KeyZ':
                togglePhysicsDebug();
                break;
            case 'KeyM':
                toggleMolangMonitor();
                break;
        }
    });
}

// ==== 失焦处理 ====
function bindBlur() {
    window.addEventListener('blur', () => {
        // 无需清空按键状态（已移除 WASD）
    });
}

// ==== 物理调试可视化 ====
function togglePhysicsDebug() {
    physicsDebugVisible = !physicsDebugVisible;
    document.body.classList.toggle('show-debug', physicsDebugVisible);
    physicsManager?.setDebugVisible?.(physicsDebugVisible);
    // 确保角色碰撞箱在开启时也被创建/显示
    if (physicsDebugVisible) {
        characterPhysics?.setDebugVisible?.(true);
    } else {
        characterPhysics?.setDebugVisible?.(false);
    }
}

// ==== MoLang 监控模态框（按键 M） ====
let molangMonitorVisible = false;
let molangMonitorTimer = null;

function toggleMolangMonitor() {
    molangMonitorVisible = !molangMonitorVisible;
    document.body.classList.toggle('show-molang', molangMonitorVisible);
    if (molangMonitorVisible) {
        // 每 100ms 更新模态框内容
        updateMolangMonitor();
        molangMonitorTimer = setInterval(updateMolangMonitor, 100);
    } else {
        if (molangMonitorTimer) {
            clearInterval(molangMonitorTimer);
            molangMonitorTimer = null;
        }
    }
}

function updateMolangMonitor() {
    const body = document.getElementById('mm-body');
    if (!body || !molang) return;

    const changes = molang.getRecentChanges?.(30) ?? [];
    if (changes.length === 0) {
        body.innerHTML = '<div class="mm-empty">暂无变量变更</div>';
        return;
    }

    body.innerHTML = `
        <table>
            <thead><tr><th>变量名</th><th>当前值</th><th>变更于</th></tr></thead>
            <tbody>
                ${changes.map(c => `
                    <tr>
                        <td class="mm-key">${c.key}</td>
                        <td>${typeof c.value === 'number' ? c.value.toFixed(3) : c.value}</td>
                        <td class="mm-age">${c.age < 1 ? '刚刚' : c.age.toFixed(1) + '秒前'}</td>
                    </tr>
                `).join('')}
            </tbody>
        </table>
    `;
}

function updateDebugOverlay() {
    const overlay = document.getElementById('debug-overlay');
    if (!overlay) return;

    const bodyCount = physicsManager?.bodies.size ?? 0;

    // 模型面数
    let faceCount = 0;
    if (renderer?.modelRoot) {
        renderer.modelRoot.traverse(child => {
            if (child.isMesh && child.geometry) {
                const idx = child.geometry.index;
                if (idx) faceCount += idx.count / 3;
                else {
                    const pos = child.geometry.getAttribute('position');
                    if (pos) faceCount += pos.count / 3;
                }
            }
        });
    }
    const faceStr = faceCount >= 1000 ? (faceCount / 1000).toFixed(1) + 'K' : Math.round(faceCount);

    // 30秒平均FPS
    const avgFps = _fpsSamples.length > 0
        ? (_fpsSamples.reduce((a, b) => a + b, 0) / _fpsSamples.length).toFixed(1)
        : '--';

    // 物理引擎逻辑帧率
    const physStepRate = physicsManager?._stepRate ?? 60;

    overlay.textContent =
        `面数: ${faceStr}  物理体: ${bodyCount}\n` +
        `平均FPS: ${avgFps}  物理帧率: ${physStepRate}Hz`;
}

// ==== BroadcastChannel 消息处理 ====
async function handleChannelMessage(msg) {
    const { type, payload, source } = msg;

    switch (type) {
        case 'panel_ready':
            // 面板就绪握手：晚订阅的面板可重新请求状态
            if (source === 'animation-panel') {
                broadcastAnimationList();
            }
            if (source === 'elements-panel') {
                broadcastCompoundsList();
                // 广播所有现有图元信息（用于解散组合体后子图元回归列表）
                for (const mesh of primitives?.getAll?.() || []) {
                    if (!primitives?.isCompound?.(mesh.userData.id)) {
                        broadcast('primitive_created', {
                            id: mesh.userData.id,
                            type: mesh.userData.type,
                            color: mesh.material?.color?.getHex?.() ?? 0x9d6bff,
                            isCompound: false,
                        });
                    }
                }
            }
            break;

        case 'query_molang':
            // 设置 molang 查询表达式（Q12）
            molangQuery = payload?.expression || null;
            if (!molangQuery) {
                broadcast('molang_value', { value: null });
            }
            break;

        case 'mode_changed':
            handleModeChange(payload?.mode);
            break;

        case 'action':
            enqueueCommand({ type: 'action', action: payload.action });
            break;

        case 'movement':
            enqueueCommand({
                type: 'movement',
                position: payload.position,
                resumeTracking: payload.resumeTracking !== false
            });
            break;

        case 'rotation':
            if (payload) {
                const cur = movementController?.currentRotation ?? { yaw: 0, pitch: 0 };
                movementController?.setRotation(
                    payload.yaw ?? cur.yaw,
                    payload.pitch ?? cur.pitch
                );
            }
            break;

        case 'mouse_tracking':
            movementController?.setMouseTracking(payload.enabled);
            // 开启鼠标追踪时自动执行一次聚焦
            if (payload.enabled && renderer?.modelRoot) {
                cameraController?.stopOverShoulder?.();
                renderer.moveCameraToFront(1.5);
            }
            break;

        case 'focus_character':
            if (renderer?.modelRoot) {
                // 退出越肩模式，让轨道控制器接管
                cameraController?.stopOverShoulder?.();
                // 使用渲染器的 moveCameraToFront 实现相机移动到角色前方
                renderer.moveCameraToFront(1.5);
            }
            break;

        case 'quick_track':
            quickTrackEnabled = !!payload?.enabled;
            if (!quickTrackEnabled) {
                hideQuickTrackMarker();
            }
            break;

        case 'highlight_bone': {
            // 在3D模型上高亮闪烁指定骨骼
            const boneName = payload?.boneName;
            if (!boneName || !animationRuntime?.boneMap) break;
            const bone = animationRuntime.boneMap.get(boneName);
            if (!bone?.sceneObject) break;
            const meshes = [];
            bone.sceneObject.traverse(child => {
                if (child.isMesh && child.material) meshes.push(child);
            });
            if (meshes.length === 0) break;
            // 保存原始材质，用克隆材质高亮（避免影响共享材质的其他骨骼）
            const originals = meshes.map(m => {
                const origMat = m.material;
                const highlightMat = origMat.clone();
                if (highlightMat.emissive !== undefined) {
                    highlightMat.emissive.setHex(0x9d6bff);
                    highlightMat.emissiveIntensity = 0.8;
                } else {
                    highlightMat.color.lerp(new THREE.Color(0x9d6bff), 0.5);
                    highlightMat.transparent = true;
                    highlightMat.opacity = 0.85;
                }
                m.material = highlightMat;
                return { mesh: m, originalMat: origMat, highlightMat };
            });
            // 600ms 后恢复原始材质
            setTimeout(() => {
                for (const orig of originals) {
                    if (orig.mesh.material === orig.highlightMat) {
                        orig.mesh.material = orig.originalMat;
                    }
                    orig.highlightMat.dispose();
                }
            }, 600);
            break;
        }

        case 'animation_control':
            handleAnimationControl(payload);
            break;

        case 'anim_group_create':
            handleAnimGroupCreate(payload);
            break;

        case 'anim_group_delete':
            handleAnimGroupDelete(payload);
            break;

        case 'anim_group_update':
            handleAnimGroupUpdate(payload);
            break;

        case 'anim_group_save':
            handleAnimGroupSave();
            break;

        case 'anim_group_export':
            handleAnimGroupExport();
            break;

        case 'anim_group_import':
            handleAnimGroupImport(payload);
            break;

        case 'body_rotation':
            handleBodyRotation(payload);
            break;

        case 'physics_param':
            handlePhysicsParam(payload);
            break;

        case 'primitive_add': {
            const spec = payload.spec || {};
            // 位置由引擎基于角色附近 R50/H40 随机生成
            const charPos = renderer?.modelRoot?.position || { x: 0, y: 0, z: 0 };
            const spawnPos = {
                x: charPos.x + (Math.random() - 0.5) * 100,
                y: 40,
                z: charPos.z + (Math.random() - 0.5) * 100,
            };
            spec.position = spawnPos;
            const createdMesh = primitives?.addFromSpec(spec);
            if (createdMesh) {
                // 应用透明度
                if (spec.opacity !== undefined && spec.opacity < 1) {
                    createdMesh.traverse(child => {
                        if (child.isMesh && child.material) {
                            child.material.transparent = true;
                            child.material.opacity = spec.opacity;
                            child.material.needsUpdate = true;
                        }
                    });
                }
                broadcast('primitive_created', {
                    id: createdMesh.userData.id,
                    type: createdMesh.userData.type,
                    color: spec.color,
                    isCompound: false,
                });
                // 全局物理：新增图元，仅在物理未禁用时启用
                if (!physicsDisabled) {
                    physicsManager?.addPrimitive?.(createdMesh);
                }
                // 广播生成位置信息给元素面板
                broadcast('primitive_spawn_info', {
                    id: createdMesh.userData.id,
                    position: {
                        x: createdMesh.position.x,
                        y: createdMesh.position.y,
                        z: createdMesh.position.z,
                    },
                });
            }
            break;
        }

        case 'primitive_remove': {
            const id = payload.id;
            // 先获取mesh引用再删除（removeById后getById返回null）
            const meshToRemove = primitives?.getById?.(id);
            // 若是组合体，先解散子图元（同时广播每个子图元的 removed）
            if (primitives?.isCompound?.(id)) {
                const childIds = primitives.dissolveCompound(id);
                // 解散后，子图元变回独立图元，广播给面板
                broadcast('compound_dissolved', { compoundId: id, childIds });
                // 子图元已存在，无需广播 primitive_created
            } else {
                primitives?.removeById(id);
                broadcast('primitive_removed', { id });
            }
            // 同步移除物理体
            if (meshToRemove) physicsManager?.removePrimitive?.(meshToRemove);
            broadcastCompoundsList();
            break;
        }

        case 'compound_create': {
            const group = primitives?.createCompound?.(payload.meshIds || [], {
                name: payload.name,
                anchored: payload.anchored,
            });
            if (group) {
                broadcast('compound_created', {
                    id: group.userData.id,
                    name: group.userData.name,
                    memberIds: group.userData.compoundMemberIds,
                    anchored: !!group.userData.physics?.anchored,
                });
                broadcastCompoundsList();
            } else {
                broadcast('compound_op_result', { ok: false, message: '组合体创建失败' });
            }
            break;
        }

        case 'compound_dissolve': {
            const id = payload.id;
            if (!primitives?.isCompound?.(id)) {
                broadcast('compound_op_result', { ok: false, message: '指定 ID 不是组合体' });
                break;
            }
            // 先移除组合体物理体
            const groupMesh = primitives.getById(id);
            if (groupMesh) physicsManager?.removePrimitive?.(groupMesh);
            const childIds = primitives.dissolveCompound(id);
            broadcast('compound_dissolved', { compoundId: id, childIds });
            broadcastCompoundsList();
            break;
        }

        case 'texture_apply': {
            let dataUrl = payload.dataUrl;
            // 如果传了 textureUUID，从图片资产库查找 base64
            if (!dataUrl && payload.textureUUID) {
                const imgAsset = imageAssetStore.get(payload.textureUUID);
                if (imgAsset) dataUrl = imgAsset.base64;
            }
            if (!dataUrl) {
                broadcast('texture_op_result', { ok: false, id: payload.id, action: 'apply' });
                break;
            }
            const repeat = { x: payload.repeatU || 1, y: payload.repeatV || 1 };
            const ok = primitives?.applyTexture?.(payload.id, dataUrl, { repeat });
            if (ok && payload.textureUUID) {
                const mesh = primitives?.getById?.(payload.id);
                if (mesh) mesh.userData.textureUUID = payload.textureUUID;
            }
            broadcast('texture_op_result', { ok: !!ok, id: payload.id, action: 'apply' });
            break;
        }

        case 'texture_clear': {
            const ok = primitives?.clearTexture?.(payload.id);
            broadcast('texture_op_result', { ok: !!ok, id: payload.id, action: 'clear' });
            break;
        }

        case 'texture_list_request': {
            const textures = [];
            for (const [uuid, asset] of imageAssetStore) {
                textures.push({ uuid: asset.uuid, name: asset.name, thumbnail: asset.base64 });
            }
            broadcast('texture_list', { textures });
            break;
        }

        case 'image_asset_save': {
            const { uuid, base64, name } = payload || {};
            if (!uuid || !base64) break;
            imageAssetStore.set(uuid, { uuid, base64, name: name || uuid });
            broadcast('image_assets_list', { images: Array.from(imageAssetStore.values()) });
            break;
        }

        case 'image_asset_delete': {
            const { uuid } = payload || {};
            if (!uuid) break;
            imageAssetStore.delete(uuid);
            broadcast('image_assets_list', { images: Array.from(imageAssetStore.values()) });
            break;
        }

        case 'image_assets_list_request': {
            broadcast('image_assets_list', { images: Array.from(imageAssetStore.values()) });
            break;
        }

        case 'assets_list_request': {
            // 扫描 model/assets/ 目录中的 JSON 文件列表
            try {
                const listPath = 'package/integrated_studio/model/assets/';
                const resp = await fetch(`/file/read/${listPath}`);
                if (resp.ok) {
                    const dirListing = await resp.json();
                    // dirListing 可能是文件名数组或对象
                    const files = Array.isArray(dirListing) ? dirListing : Object.keys(dirListing || {});
                    const assets = files
                        .filter(f => f.endsWith('.json'))
                        .map(f => {
                            const name = f.replace(/\.json$/, '');
                            return { id: name, name, type: 'unknown', primitiveCount: 0 };
                        });
                    broadcast('assets_list', { assets });
                } else {
                    broadcast('assets_list', { assets: [] });
                }
            } catch (err) {
                broadcast('assets_list', { assets: [] });
            }
            break;
        }

        case 'asset_save_one': {
            await handleAssetSave(payload.id, payload.name);
            break;
        }

        case 'asset_import': {
            try {
                const json = typeof payload.asset === 'string' ? JSON.parse(payload.asset) : payload.asset;
                const obj = primitives?.importAsset?.(json, imageAssetStore);
                if (obj) {
                    if (obj.isGroup) {
                        broadcast('compound_created', {
                            id: obj.userData.id,
                            name: obj.userData.name,
                            memberIds: obj.userData.compoundMemberIds,
                            anchored: !!obj.userData.physics?.anchored,
                        });
                        broadcastCompoundsList();
                    } else {
                        broadcast('primitive_created', {
                            id: obj.userData.id,
                            type: obj.userData.type,
                            isCompound: false,
                        });
                    }
                    broadcast('asset_op_result', { ok: true, message: '资产已导入' });
                } else {
                    broadcast('asset_op_result', { ok: false, message: '资产格式无效' });
                }
            } catch (err) {
                broadcast('asset_op_result', { ok: false, message: '导入异常：' + err.message });
            }
            break;
        }

        case 'config_save': {
            const configType = payload?.type; // 'physics', 'scene', 'images', 'all'
            const saveData = {};

            if (configType === 'physics' || configType === 'all') {
                saveData.physics = {
                    restitution: physicsManager?.restitution ?? 0.3,
                    friction: physicsManager?.friction ?? 0.3,
                    linearDamping: physicsManager?.linearDamping ?? 0.1,
                    angularDamping: physicsManager?.angularDamping ?? 0.1,
                    fallSpeedMultiplier: physicsManager?.fallSpeedMultiplier ?? 3.0,
                    gravity: physicsManager?.gravity ?? -9.82,
                };
            }

            if (configType === 'scene' || configType === 'all') {
                const prims = [];
                for (const mesh of primitives?.getAll?.() || []) {
                    const exportData = primitives.exportAsset(mesh.userData.id);
                    if (exportData) prims.push(exportData);
                }
                for (const c of primitives?.getCompounds?.() || []) {
                    const exportData = primitives.exportAsset(c.id);
                    if (exportData) prims.push(exportData);
                }
                saveData.scene = {
                    primitives: prims,
                    characterPosition: renderer?.modelRoot?.position
                        ? { x: renderer.modelRoot.position.x, y: renderer.modelRoot.position.y, z: renderer.modelRoot.position.z }
                        : { x: 0, y: 0, z: 0 },
                };
            }

            if (configType === 'images' || configType === 'all') {
                const images = {};
                for (const [uuid, asset] of imageAssetStore) {
                    images[uuid] = { base64: asset.base64, name: asset.name };
                }
                saveData.images = images;
            }

            // 写入 property/ 文件夹
            const fileName = configType === 'all' ? 'all_config' : `${configType}_config`;
            const relativePath = `package/integrated_studio/property/${fileName}.json`;
            try {
                const encodedPath = btoa(unescape(encodeURIComponent(relativePath)));
                const resp = await fetch('/file/write', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'X-File-Name': encodedPath,
                        'X-Overwrite': 'true',
                    },
                    body: JSON.stringify(saveData, null, 2),
                });
                if (resp.ok) {
                    broadcast('config_save_result', { type: configType, ok: true, message: `已保存至 property/${fileName}.json` });
                } else {
                    broadcast('config_save_result', { type: configType, ok: false, message: `保存失败：HTTP ${resp.status}` });
                }
            } catch (err) {
                broadcast('config_save_result', { type: configType, ok: false, message: '保存异常：' + err.message });
            }
            break;
        }

        case 'config_load': {
            const configType = payload?.type; // 'physics', 'scene', 'images', 'all'
            const fileName = configType === 'all' ? 'all_config' : `${configType}_config`;
            const relativePath = `package/integrated_studio/property/${fileName}.json`;
            try {
                const resp = await fetch(`/file/read/${relativePath}`);
                if (!resp.ok) {
                    broadcast('config_load_result', { type: configType, ok: false, message: `文件不存在或读取失败：HTTP ${resp.status}` });
                    break;
                }
                const data = await resp.json();

                if ((configType === 'physics' || configType === 'all') && data.physics) {
                    const p = data.physics;
                    if (physicsManager) {
                        if (p.restitution !== undefined) physicsManager.restitution = p.restitution;
                        if (p.friction !== undefined) physicsManager.friction = p.friction;
                        if (p.linearDamping !== undefined) physicsManager.linearDamping = p.linearDamping;
                        if (p.angularDamping !== undefined) physicsManager.angularDamping = p.angularDamping;
                        if (p.fallSpeedMultiplier !== undefined) physicsManager.fallSpeedMultiplier = p.fallSpeedMultiplier;
                        if (p.gravity !== undefined) physicsManager.gravity = p.gravity;
                    }
                    broadcast('physics_param', {
                        restitution: physicsManager?.restitution,
                        friction: physicsManager?.friction,
                        linearDamping: physicsManager?.linearDamping,
                        angularDamping: physicsManager?.angularDamping,
                        fallSpeedMultiplier: physicsManager?.fallSpeedMultiplier,
                        gravity: physicsManager?.gravity,
                    });
                }

                if ((configType === 'scene' || configType === 'all') && data.scene) {
                    physicsManager?.reset();
                    primitives?.clear();
                    const sceneData = data.scene;
                    if (sceneData.primitives) {
                        for (const asset of sceneData.primitives) {
                            const obj = primitives?.importAsset?.(asset, imageAssetStore);
                            if (obj && !physicsDisabled) physicsManager?.addPrimitive?.(obj);
                            // 广播每个图元/组合体的创建事件，供元素面板刷新列表
                            if (obj) {
                                if (obj.isGroup || obj.userData?.type === 'group') {
                                    broadcast('compound_created', {
                                        id: obj.userData.id,
                                        name: obj.userData.name,
                                        memberIds: obj.userData.compoundMemberIds,
                                        anchored: !!obj.userData.physics?.anchored,
                                    });
                                } else {
                                    broadcast('primitive_created', {
                                        id: obj.userData.id,
                                        type: obj.userData.type,
                                        color: obj.material?.color?.getHex?.() ?? 0x9d6bff,
                                        isCompound: false,
                                    });
                                }
                            }
                        }
                    }
                    if (sceneData.characterPosition && renderer?.modelRoot) {
                        renderer.modelRoot.position.set(
                            sceneData.characterPosition.x,
                            sceneData.characterPosition.y,
                            sceneData.characterPosition.z
                        );
                    }
                    broadcastCompoundsList();
                }

                if ((configType === 'images' || configType === 'all') && data.images) {
                    imageAssetStore.clear();
                    for (const [uuid, info] of Object.entries(data.images)) {
                        imageAssetStore.set(uuid, { uuid, base64: info.base64, name: info.name || uuid });
                    }
                    broadcast('image_assets_list', { images: Array.from(imageAssetStore.values()) });
                }

                broadcast('config_load_result', { type: configType, ok: true, message: `已从 property/${fileName}.json 加载` });
            } catch (err) {
                broadcast('config_load_result', { type: configType, ok: false, message: '加载异常：' + err.message });
            }
            break;
        }

        case 'scene_load': {
            // 同 config_load scene 分支
            const data = payload;
            if (!data?.scene) break;
            physicsManager?.reset();
            primitives?.clear();
            for (const asset of data.scene.primitives || []) {
                const obj = primitives?.importAsset?.(asset, imageAssetStore);
                if (obj && !physicsDisabled) physicsManager?.addPrimitive?.(obj);
            }
            if (data.scene.characterPosition && renderer?.modelRoot) {
                renderer.modelRoot.position.set(
                    data.scene.characterPosition.x,
                    data.scene.characterPosition.y,
                    data.scene.characterPosition.z
                );
            }
            broadcastCompoundsList();
            break;
        }

        case 'physics_disabled': {
            physicsDisabled = !!payload?.enabled;
            break;
        }

        case 'element_select': {
            const id = payload?.id;
            if (selectedElementId) {
                // 取消之前的选中
                broadcast('element_deselected', { id: selectedElementId });
            }
            selectedElementId = id;
            if (id) {
                // 选中时：卸载物理体 + 网格对齐
                const mesh = primitives?.getById?.(id);
                if (mesh) {
                    physicsManager?.removePrimitive?.(mesh);
                    // 网格对齐：位置规整到整数
                    mesh.position.x = Math.round(mesh.position.x);
                    mesh.position.y = Math.round(mesh.position.y);
                    mesh.position.z = Math.round(mesh.position.z);
                    // 显示选中高亮
                    mesh.traverse(child => {
                        if (child.isMesh && child.material) {
                            child.userData._origEmissive = child.material.emissive?.clone?.();
                            child.userData._origEmissiveIntensity = child.material.emissiveIntensity ?? 0;
                            if (child.material.emissive !== undefined) {
                                child.material.emissive.setHex(0x6c9bcf);
                                child.material.emissiveIntensity = 0.4;
                            }
                        }
                    });
                }
                broadcast('element_selected', { id, position: mesh ? { x: mesh.position.x, y: mesh.position.y, z: mesh.position.z } : null });
            }
            break;
        }

        case 'element_deselect': {
            if (selectedElementId) {
                const mesh = primitives?.getById?.(selectedElementId);
                if (mesh) {
                    // 移除选中高亮
                    mesh.traverse(child => {
                        if (child.isMesh && child.material) {
                            if (child.userData._origEmissive) {
                                child.material.emissive?.copy(child.userData._origEmissive);
                                child.material.emissiveIntensity = child.userData._origEmissiveIntensity ?? 0;
                            }
                            delete child.userData._origEmissive;
                            delete child.userData._origEmissiveIntensity;
                        }
                    });
                }
                broadcast('element_deselected', { id: selectedElementId });
                selectedElementId = null;
            }
            break;
        }

        case 'element_edit_mode': {
            editMode = payload?.mode || 'translate'; // translate, rotate, scale
            break;
        }

        case 'element_edit_step': {
            // WASD/Space/Shift 控制选中元素
            if (!selectedElementId) break;
            const mesh = primitives?.getById?.(selectedElementId);
            if (!mesh) break;
            const step = payload?.step || 1;
            const dir = payload?.direction; // 'up','down','north','south','east','west'

            switch (editMode) {
                case 'translate':
                    if (dir === 'north') mesh.position.z -= step;
                    else if (dir === 'south') mesh.position.z += step;
                    else if (dir === 'east') mesh.position.x += step;
                    else if (dir === 'west') mesh.position.x -= step;
                    else if (dir === 'up') mesh.position.y += step;
                    else if (dir === 'down') mesh.position.y -= step;
                    break;
                case 'rotate':
                    const angle = (Math.PI / 12) * step; // 15度步进
                    if (dir === 'north' || dir === 'south') mesh.rotation.x += (dir === 'north' ? -angle : angle);
                    else if (dir === 'east' || dir === 'west') mesh.rotation.y += (dir === 'east' ? angle : -angle);
                    else if (dir === 'up' || dir === 'down') mesh.rotation.z += (dir === 'up' ? angle : -angle);
                    break;
                case 'scale':
                    const s = 0.1 * step;
                    if (dir === 'north' || dir === 'south') mesh.scale.y += (dir === 'north' ? s : -s);
                    else if (dir === 'east' || dir === 'west') mesh.scale.x += (dir === 'east' ? s : -s);
                    else if (dir === 'up' || dir === 'down') mesh.scale.z += (dir === 'up' ? s : -s);
                    break;
            }
            // 位移模式网格对齐
            if (editMode === 'translate') {
                mesh.position.x = Math.round(mesh.position.x);
                mesh.position.y = Math.round(mesh.position.y);
                mesh.position.z = Math.round(mesh.position.z);
            }
            broadcast('element_transform', {
                id: selectedElementId,
                position: { x: mesh.position.x, y: mesh.position.y, z: mesh.position.z },
                rotation: { x: mesh.rotation.x, y: mesh.rotation.y, z: mesh.rotation.z },
                scale: { x: mesh.scale.x, y: mesh.scale.y, z: mesh.scale.z },
                mode: editMode,
            });
            break;
        }

        case 'element_physics_enable': {
            const id = payload?.id;
            const mesh = primitives?.getById?.(id);
            if (mesh) physicsManager?.addPrimitive?.(mesh);
            break;
        }

        case 'element_physics_disable': {
            const id = payload?.id;
            const mesh = primitives?.getById?.(id);
            if (mesh) physicsManager?.removePrimitive?.(mesh);
            break;
        }

        case 'unload_all_physics': {
            // 卸载所有非角色图元/组合体的物理效果并对齐网格
            for (const mesh of primitives?.getAll?.() || []) {
                physicsManager?.removePrimitive?.(mesh);
                mesh.position.x = Math.round(mesh.position.x);
                mesh.position.y = Math.round(mesh.position.y);
                mesh.position.z = Math.round(mesh.position.z);
            }
            broadcast('all_physics_unloaded', {});
            break;
        }

        case 'theme_changed':
            document.body.classList.toggle('dark-mode', payload?.dark === true);
            break;

        case 'config_import_json': {
            const configType = payload?.type;
            const data = payload?.data;
            if (!data) break;
            // 复用 config_load 的加载逻辑，但从 payload 读取而非文件
            if ((configType === 'physics') && data.physics) {
                const p = data.physics;
                if (physicsManager) {
                    if (p.restitution !== undefined) physicsManager.restitution = p.restitution;
                    if (p.friction !== undefined) physicsManager.friction = p.friction;
                    if (p.linearDamping !== undefined) physicsManager.linearDamping = p.linearDamping;
                    if (p.angularDamping !== undefined) physicsManager.angularDamping = p.angularDamping;
                    if (p.fallSpeedMultiplier !== undefined) physicsManager.fallSpeedMultiplier = p.fallSpeedMultiplier;
                    if (p.gravity !== undefined) physicsManager.gravity = p.gravity;
                }
                broadcast('physics_param', {
                    restitution: physicsManager?.restitution,
                    friction: physicsManager?.friction,
                    linearDamping: physicsManager?.linearDamping,
                    angularDamping: physicsManager?.angularDamping,
                    fallSpeedMultiplier: physicsManager?.fallSpeedMultiplier,
                    gravity: physicsManager?.gravity,
                });
            }

            if ((configType === 'scene') && data.scene) {
                physicsManager?.reset();
                primitives?.clear();
                const sceneData = data.scene;
                if (sceneData.primitives) {
                    for (const asset of sceneData.primitives) {
                        const obj = primitives?.importAsset?.(asset, imageAssetStore);
                        if (obj && !physicsDisabled) physicsManager?.addPrimitive?.(obj);
                    }
                }
                if (sceneData.characterPosition && renderer?.modelRoot) {
                    renderer.modelRoot.position.set(
                        sceneData.characterPosition.x,
                        sceneData.characterPosition.y,
                        sceneData.characterPosition.z
                    );
                }
                broadcastCompoundsList();
            }

            broadcast('config_load_result', { type: configType, ok: true, message: 'JSON 配置已导入' });
            break;
        }

        case 'asset_save_library': {
            // 保存当前选中的图元/组合体到资产库
            const name = payload?.name;
            if (!selectedElementId) {
                broadcast('asset_op_result', { ok: false, message: '请先在元素页选中一个图元或组合体' });
                break;
            }
            await handleAssetSave(selectedElementId, name);
            break;
        }

        case 'asset_import_json': {
            const assetData = payload?.asset;
            if (!assetData) break;
            try {
                const json = typeof assetData === 'string' ? JSON.parse(assetData) : assetData;
                const obj = primitives?.importAsset?.(json, imageAssetStore);
                if (obj) {
                    if (!physicsDisabled) physicsManager?.addPrimitive?.(obj);
                    if (obj.isGroup || obj.userData?.type === 'group') {
                        broadcast('compound_created', {
                            id: obj.userData.id,
                            name: obj.userData.name,
                            memberIds: obj.userData.compoundMemberIds,
                            anchored: !!obj.userData.physics?.anchored,
                        });
                    }
                    broadcast('primitive_created', { id: obj.userData.id, type: obj.userData.type, color: obj.userData.color });
                    broadcast('asset_op_result', { ok: true, message: '资产已导入场景' });
                }
            } catch (err) {
                broadcast('asset_op_result', { ok: false, message: '导入失败：' + err.message });
            }
            break;
        }

        case 'asset_delete': {
            const assetId = payload?.assetId;
            if (!assetId) break;
            // 从场景中删除（如果存在）
            const mesh = primitives?.getById?.(assetId);
            if (mesh) {
                physicsManager?.removePrimitive?.(mesh);
                primitives?.removeById?.(assetId);
                broadcast('primitive_removed', { id: assetId });
            }
            broadcast('asset_op_result', { ok: true, message: '资产已删除' });
            break;
        }

        case 'asset_import_scene': {
            // 从 model/assets/ 读取资产文件并导入到场景
            const assetId = payload?.assetId;
            if (!assetId) break;
            try {
                const relativePath = `package/integrated_studio/model/assets/${assetId}.json`;
                const resp = await fetch(`/file/read/${relativePath}`);
                if (!resp.ok) {
                    broadcast('asset_op_result', { ok: false, message: `资产文件不存在：${assetId}.json` });
                    break;
                }
                const asset = await resp.json();
                const obj = primitives?.importAsset?.(asset, imageAssetStore);
                if (obj) {
                    if (!physicsDisabled) physicsManager?.addPrimitive?.(obj);
                    if (obj.isGroup || obj.userData?.type === 'group') {
                        broadcast('compound_created', {
                            id: obj.userData.id,
                            name: obj.userData.name,
                            memberIds: obj.userData.compoundMemberIds,
                            anchored: !!obj.userData.physics?.anchored,
                        });
                    } else {
                        broadcast('primitive_created', { id: obj.userData.id, type: obj.userData.type, isCompound: false });
                    }
                    broadcastCompoundsList();
                    broadcast('asset_op_result', { ok: true, message: `资产「${assetId}」已导入场景` });
                } else {
                    broadcast('asset_op_result', { ok: false, message: '资产格式无效' });
                }
            } catch (err) {
                broadcast('asset_op_result', { ok: false, message: '导入异常：' + err.message });
            }
            break;
        }

        default:
            console.debug('[Engine] 未识别的消息:', type);
    }
}

// ==== 模式切换处理（Q5.3） ====
function handleModeChange(mode) {
    if (mode !== 'editor' && mode !== 'embedded') return;
    if (mode === currentMode) return;

    currentMode = mode;
    document.body.classList.toggle('mode-editor', mode === 'editor');
    if (renderer?.gridHelper) {
        renderer.gridHelper.visible = (mode === 'editor');
    }
    console.log(`[Engine] 模式切换至 ${mode}`);
}

// ==== 动画控制消息处理 ====
function handleAnimationControl(payload) {
    if (!animGroupRuntime || !payload) return;

    switch (payload.action) {
        case 'play_group':
            animGroupRuntime.activateGroup(payload.groupName);
            break;
        case 'pause':
            // AnimGroupRuntime 无 pause，使用 AnimationRuntime.pause()
            animationRuntime?.pause?.();
            break;
        case 'resume':
            animationRuntime?.resume?.();
            break;
        case 'set_speed':
            // speed 是 AnimationRuntime 的直接属性
            if (animationRuntime) animationRuntime.speed = payload.speed ?? 1;
            break;
    }
}

// ==== 动画组管理消息处理 ====

/** 创建动画组 */
function handleAnimGroupCreate(payload) {
    if (!animGroupRuntime || !payload) return;
    const name = (payload.name || '').trim();
    if (!name) {
        broadcast('anim_group_op_result', { ok: false, message: '组名不能为空' });
        return;
    }
    if (animGroupRuntime.groups.has(name)) {
        broadcast('anim_group_op_result', { ok: false, message: `组「${name}」已存在` });
        return;
    }
    const group = new AnimGroup(name);
    group.isDefault = false;
    group.loopMode = ['repeat', 'return', 'hold'].includes(payload.loopMode) ? payload.loopMode : 'repeat';
    group.transitionDuration = typeof payload.transitionDuration === 'number' ? payload.transitionDuration : 0.3;
    group.visibilityDelay = typeof payload.visibilityDelay === 'number' ? payload.visibilityDelay : 0;
    group.animations = Array.isArray(payload.animations) ? [...payload.animations] : [];
    animGroupRuntime.addGroup(group);
    broadcastAnimationList();
    broadcast('anim_group_op_result', { ok: true, message: `组「${name}」已创建` });
}

/** 删除动画组（默认组不可删除） */
function handleAnimGroupDelete(payload) {
    if (!animGroupRuntime || !payload) return;
    const name = payload.name;
    const g = animGroupRuntime.groups.get(name);
    if (!g) {
        broadcast('anim_group_op_result', { ok: false, message: `组「${name}」不存在` });
        return;
    }
    if (g.isDefault) {
        broadcast('anim_group_op_result', { ok: false, message: '默认组不可删除' });
        return;
    }
    animGroupRuntime.removeGroup(name);
    broadcastAnimationList();
    broadcast('anim_group_op_result', { ok: true, message: `组「${name}」已删除` });
}

/** 更新动画组配置 */
function handleAnimGroupUpdate(payload) {
    if (!animGroupRuntime || !payload) return;
    const name = payload.name;
    const g = animGroupRuntime.groups.get(name);
    if (!g) {
        broadcast('anim_group_op_result', { ok: false, message: `组「${name}」不存在` });
        return;
    }
    if (g.isDefault) {
        // 默认组仅允许更新动画序列与显隐
        if (Array.isArray(payload.animations)) g.animations = [...payload.animations];
        if (payload.boneVisibility) g.boneVisibility = { ...payload.boneVisibility };
    } else {
        if (Array.isArray(payload.animations)) g.animations = [...payload.animations];
        if (payload.boneVisibility) g.boneVisibility = { ...payload.boneVisibility };
        if (['repeat', 'return', 'hold'].includes(payload.loopMode)) g.loopMode = payload.loopMode;
        if (typeof payload.transitionDuration === 'number') g.transitionDuration = payload.transitionDuration;
        if (typeof payload.visibilityDelay === 'number') g.visibilityDelay = payload.visibilityDelay;
    }
    // 若是当前激活组，重新启动播放以应用新配置
    if (animGroupRuntime.activeCustomGroup === g) {
        animGroupRuntime.activateGroup(name);
    }
    broadcastAnimationList();
    broadcast('anim_group_op_result', { ok: true, message: `组「${name}」已更新` });
}

/** 保存动画组配置到服务器（覆写 model/anim_group_config.json） */
async function handleAnimGroupSave() {
    if (!animGroupRuntime) return;
    try {
        const config = animGroupRuntime.exportConfig();
        const json = JSON.stringify(config, null, 2);
        const relativePath = 'package/integrated_studio/model/anim_group_config.json';
        const encodedPath = btoa(unescape(encodeURIComponent(relativePath)));
        const resp = await fetch('/file/write', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'X-File-Name': encodedPath,
                'X-Overwrite': 'true',
            },
            body: json,
        });
        if (resp.ok) {
            broadcast('anim_group_op_result', { ok: true, message: '动画组配置已保存到服务器' });
        } else {
            broadcast('anim_group_op_result', { ok: false, message: `保存失败：HTTP ${resp.status}` });
        }
    } catch (err) {
        broadcast('anim_group_op_result', { ok: false, message: '保存异常：' + err.message });
    }
}

/** 导出动画组配置（返回 JSON 字符串，由面板触发下载） */
function handleAnimGroupExport() {
    if (!animGroupRuntime) return;
    const config = animGroupRuntime.exportConfig();
    broadcast('anim_group_export_result', { config });
}

/** 从 JSON 字符串导入动画组配置 */
function handleAnimGroupImport(payload) {
    if (!animGroupRuntime || !payload) return;
    try {
        const json = typeof payload.config === 'string' ? JSON.parse(payload.config) : payload.config;
        if (!json || !json.animation_groups) {
            broadcast('anim_group_op_result', { ok: false, message: '配置格式无效' });
            return;
        }
        animGroupRuntime.importConfig(json, currentAnimations);
        broadcastAnimationList();
        broadcast('anim_group_op_result', { ok: true, message: '动画组配置已导入' });
    } catch (err) {
        broadcast('anim_group_op_result', { ok: false, message: '导入异常：' + err.message });
    }
}

// ==== 身体旋转消息处理（target_x clamp — Q9） ====
function handleBodyRotation(payload) {
    if (!bodyRotationInterpreter || !payload) return;

    if (payload.target_x !== undefined) {
        const clamped = THREE.MathUtils.clamp(payload.target_x, TARGET_X_CLAMP.min, TARGET_X_CLAMP.max);
        bodyRotationInterpreter.setTargetX?.(clamped);
    }
    if (payload.target_y !== undefined) {
        bodyRotationInterpreter.setTargetY?.(payload.target_y);
    }
}

// ==== 物理参数消息处理 ====
function handlePhysicsParam(payload) {
    if (!physicsManager || !payload) return;

    if (payload.gravity !== undefined) {
        physicsManager.gravity = payload.gravity;
        characterPhysics?.setGravity(payload.gravity);
    }
    if (payload.targetSpeed !== undefined) {
        physicsManager.controlConfig.targetSpeed = payload.targetSpeed;
    }
    if (payload.forceGain !== undefined) {
        physicsManager.controlConfig.forceGain = payload.forceGain;
    }
    if (payload.jumpImpulse !== undefined) {
        physicsManager.controlConfig.jumpImpulse = payload.jumpImpulse;
    }
    if (payload.debugVisible !== undefined) {
        // 切换物理碰撞体线框可视化（physics 面板的调试开关）
        physicsManager.setDebugVisible?.(payload.debugVisible);
    }
    if (payload.fixedRotation !== undefined) {
        characterPhysics?.setFixedRotation?.(payload.fixedRotation);
    }
    if (payload.restitution !== undefined) {
        physicsManager.restitution = payload.restitution;
        // 更新所有已有 DYNAMIC 刚体的弹性
        for (const [id, body] of physicsManager.bodies) {
            if (body.type === CANNON.Body.DYNAMIC) body.restitution = payload.restitution;
        }
    }
    if (payload.friction !== undefined) {
        physicsManager.friction = payload.friction;
        // 更新所有已有 DYNAMIC 刚体的摩擦
        for (const [id, body] of physicsManager.bodies) {
            if (body.type === CANNON.Body.DYNAMIC) body.friction = payload.friction;
        }
    }
    if (payload.linearDamping !== undefined) {
        physicsManager.linearDamping = payload.linearDamping;
        // 更新所有已有 DYNAMIC 刚体的线性阻尼
        for (const [id, body] of physicsManager.bodies) {
            if (body.type === CANNON.Body.DYNAMIC) body.linearDamping = payload.linearDamping;
        }
    }
    if (payload.angularDamping !== undefined) {
        physicsManager.angularDamping = payload.angularDamping;
        // 更新所有已有 DYNAMIC 刚体的角阻尼
        for (const [id, body] of physicsManager.bodies) {
            if (body.type === CANNON.Body.DYNAMIC) body.angularDamping = payload.angularDamping;
        }
    }
    if (payload.fallSpeedMultiplier !== undefined) {
        physicsManager.fallSpeedMultiplier = payload.fallSpeedMultiplier;
    }
}

// ==== 指令队列（沿用 lunar_astral 模式） ====
function enqueueCommand(cmd) {
    if (cmd.type === 'movement') {
        commandQueue.length = 0;
        commandQueue.push(cmd);
    } else {
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

async function executeAction(actionName) {
    const def = ACTION_DEFINITIONS[actionName];
    if (!def) {
        console.warn(`[Engine] 未知动作: ${actionName}`);
        return;
    }
    if (movementController?.isMoving) return;

    movementController?.setMouseTracking(def.mouseTracking);
    animGroupRuntime?.activateGroup(def.group);
    broadcast('action_started', { action: actionName });
}

async function executeMovement(position, resumeTracking) {
    if (!movementController || !position) return;

    movementController.setMouseTracking(false);
    movementController.setTarget(position.x ?? 0, position.y ?? 0, position.z ?? 0);

    return new Promise((resolve) => {
        if (movementTimeout) clearTimeout(movementTimeout);

        const timeoutId = setTimeout(() => {
            const pos = movementController.currentPosition;
            movementController.setPosition(pos.x, pos.y, pos.z);
            onMovementComplete(resumeTracking);
            resolve();
        }, 10000);

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
    if (resumeTracking && movementController) {
        movementController.setMouseTracking(true);
    }
    broadcast('movement_complete', {});
}

// ==== 快速追踪地面标记 ====
function ensureQuickTrackMarker() {
    if (quickTrackMarker) return;
    // 外环标记
    const geo = new THREE.RingGeometry(1.0, 1.6, 32);
    const mat = new THREE.MeshBasicMaterial({
        color: 0x9d6bff,
        transparent: true,
        opacity: 0.6,
        side: THREE.DoubleSide,
        depthWrite: false,
    });
    quickTrackMarker = new THREE.Mesh(geo, mat);
    quickTrackMarker.rotation.x = -Math.PI / 2; // 平放在地面
    quickTrackMarker.position.y = 0.05; // 略高于地面
    quickTrackMarker.visible = false;
    renderer?.scene?.add(quickTrackMarker);
}

function updateQuickTrackMarker(ndcX, ndcY) {
    if (!quickTrackEnabled || !renderer) return;
    ensureQuickTrackMarker();
    raycaster.setFromCamera(new THREE.Vector2(ndcX, ndcY), renderer.camera);
    const groundPlane = new THREE.Plane(new THREE.Vector3(0, 1, 0), 0);
    const intersection = new THREE.Vector3();
    const hit = raycaster.ray.intersectPlane(groundPlane, intersection);
    if (hit) {
        quickTrackMarker.position.x = intersection.x;
        quickTrackMarker.position.z = intersection.z;
        quickTrackMarker.visible = true;
    } else {
        quickTrackMarker.visible = false;
    }
}

function hideQuickTrackMarker() {
    if (quickTrackMarker) quickTrackMarker.visible = false;
}

// ==== 点击检测 ====
function onCanvasClick(event) {
    if (!renderer || !currentModel) return;

    const canvas = renderer.canvas;
    const rect = canvas.getBoundingClientRect();
    mouseNDC.x = ((event.clientX - rect.left) / rect.width) * 2 - 1;
    mouseNDC.y = -((event.clientY - rect.top) / rect.height) * 2 + 1;

    // 快速追踪模式：射线检测地面并移动角色
    if (quickTrackEnabled) {
        raycaster.setFromCamera(mouseNDC, renderer.camera);
        // 射线与 y=0 平面求交
        const groundPlane = new THREE.Plane(new THREE.Vector3(0, 1, 0), 0);
        const intersection = new THREE.Vector3();
        const hit = raycaster.ray.intersectPlane(groundPlane, intersection);
        if (hit) {
            movementController?.setMouseTracking(false);
            movementController?.setTarget(intersection.x, 0, intersection.z);
            // 隐藏标记（角色移动中）
            hideQuickTrackMarker();
            return;
        }
    }

    raycaster.setFromCamera(mouseNDC, renderer.camera);
    const intersects = raycaster.intersectObjects(renderer.modelRoot.children, true);

    if (intersects.length === 0) return;

    const hit = intersects[0];
    const boneName = findBoneNameFromObject(hit.object);
    if (!boneName) return;

    const part = BONE_PART_MAP[boneName];
    if (!part) return;

    broadcast('body_click', { partName: part, boneName });
}

function findBoneNameFromObject(obj) {
    let current = obj;
    while (current) {
        if (current === renderer.modelRoot || current === renderer.scene) {
            return null;
        }
        if (current.name && animationRuntime?.boneMap?.has(current.name)) {
            return current.name;
        }
        current = current.parent;
    }
    return null;
}

// ==== 遥测广播（10Hz） ====
function broadcastTelemetry() {
    const charPos = characterPhysics?.getPosition();
    const camPos = renderer?.camera.position;

    // 统计场景图元/组合体数
    const allPrims = primitives?.getAll?.() || [];
    const compoundCount = primitives?.getCompounds?.()?.length ?? 0;
    const primCount = allPrims.length;

    broadcast('telemetry', {
        fps: renderer?.fps ?? 0,
        mode: currentMode,
        character: charPos ? { x: charPos.x, y: charPos.y, z: charPos.z } : null,
        camera: camPos ? { x: camPos.x, y: camPos.y, z: camPos.z } : null,
        isMoving: movementController?.isMoving ?? false,
        isFastMoving: movementController?.isFastMoving ?? false,
        primitiveCount: primCount,
        compoundCount: compoundCount,
    });
}

// ==== molang 值广播（10Hz，Q12） ====
function broadcastMolangValue() {
    if (!molangQuery) return;

    let value = null;
    try {
        value = molang.evaluate?.(molangQuery) ?? null;
    } catch (e) {
        value = null;
    }

    broadcast('molang_value', {
        expression: molangQuery,
        value: value,
    });
}

// ==== 广播工具 ====
function broadcast(type, payload) {
    channel.postMessage({
        type,
        source: SOURCE_ENGINE,
        payload,
        timestamp: Date.now(),
    });
}

// ==== 隐藏加载遮罩 ====
function hideLoadingMask() {
    const mask = document.getElementById('loading-mask');
    if (mask) {
        mask.classList.add('hidden');
        setTimeout(() => { mask.style.display = 'none'; }, 500);
    }
}

// ==== 指南针更新（编辑器模式下角色移动时显示方向） ====
function updateCompass() {
    const container = document.getElementById('compass-container');
    if (!container) return;

    // 仅在编辑器模式下且角色正在移动时显示
    const isEditor = currentMode === 'editor';
    const isMoving = movementController?.isMoving ?? false;

    if (!isEditor || !isMoving) {
        container.classList.remove('visible');
        compassPrevPos = null;
        return;
    }

    const curPos = renderer?.modelRoot?.position;
    if (!curPos) {
        container.classList.remove('visible');
        return;
    }

    // 检测位移方向
    if (compassPrevPos) {
        const dx = curPos.x - compassPrevPos.x;
        const dz = curPos.z - compassPrevPos.z;
        const hSpeed = Math.sqrt(dx * dx + dz * dz);
        if (hSpeed > 0.01) {
            // atan2(dx, dz) → -Z 为北(0°), +X 为东(90°)
            compassAngle = Math.atan2(dx, dz) * 180 / Math.PI;
        }
    }
    compassPrevPos = { x: curPos.x, y: curPos.y, z: curPos.z };

    // 更新箭头旋转
    const arrow = document.getElementById('compass-arrow');
    if (arrow) {
        arrow.setAttribute('transform', `rotate(${compassAngle}, 50, 50)`);
    }

    container.classList.add('visible');
}

// ==== 启动 ====
init().catch(err => {
    console.error('[Engine] 启动失败:', err);
    const mask = document.getElementById('loading-mask');
    if (mask) {
        mask.innerHTML = `<span style="color:#dc3545;">引擎加载失败：${err.message}</span>`;
    }
});
