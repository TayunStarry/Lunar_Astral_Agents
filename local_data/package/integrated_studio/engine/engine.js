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

        switch (e.code) {
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
    const grounded = characterPhysics?.isGrounded ? '接地' : '空中';
    const sneaking = characterPhysics?.isSneaking ? ' 潜行' : '';
    const vel = characterPhysics?._body?.velocity;
    const speed = vel ? Math.sqrt(vel.x ** 2 + vel.z ** 2).toFixed(1) : '0';
    const vy = vel ? vel.y.toFixed(1) : '0';

    overlay.textContent =
        `物理体: ${bodyCount}  ${grounded}${sneaking}\n` +
        `水平速度: ${speed}  垂直速度: ${vy}`;
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
                // 全局物理：新增图元立即加入物理世界
                physicsManager?.addPrimitive?.(createdMesh);
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
            const ok = primitives?.applyTexture?.(payload.id, payload.dataUrl, {
                repeat: payload.repeat,
            });
            broadcast('texture_op_result', { ok: !!ok, id: payload.id, action: 'apply' });
            break;
        }

        case 'texture_clear': {
            const ok = primitives?.clearTexture?.(payload.id);
            broadcast('texture_op_result', { ok: !!ok, id: payload.id, action: 'clear' });
            break;
        }

        case 'asset_export_one': {
            const asset = primitives?.exportAsset?.(payload.id);
            broadcast('asset_export_result', { id: payload.id, asset });
            break;
        }

        case 'asset_save_one': {
            await handleAssetSave(payload.id, payload.name);
            break;
        }

        case 'asset_import': {
            try {
                const json = typeof payload.asset === 'string' ? JSON.parse(payload.asset) : payload.asset;
                const obj = primitives?.importAsset?.(json);
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

        case 'theme_changed':
            document.body.classList.toggle('dark-mode', payload?.dark === true);
            break;

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

    broadcast('telemetry', {
        fps: renderer?.fps ?? 0,
        mode: currentMode,
        character: charPos ? { x: charPos.x, y: charPos.y, z: charPos.z } : null,
        camera: camPos ? { x: camPos.x, y: camPos.y, z: camPos.z } : null,
        physicsBodyCount: physicsManager?.bodies.size ?? 0,
        isGrounded: characterPhysics?.isGrounded ?? false,
        isSneaking: characterPhysics?.isSneaking ?? false,
        isMoving: movementController?.isMoving ?? false,
        isFastMoving: movementController?.isFastMoving ?? false,
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
