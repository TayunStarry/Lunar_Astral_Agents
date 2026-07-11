// ==== app.js — lunar.studio.bedrock 主入口 ====
//
// 架构（重构后）：
//   - 资源导入：统一"导入资源"按钮 + 拖拽，FileLoader 智能分类
//   - 自动加载：init 阶段 fetch model/manifest.json，按依赖顺序加载模型+动画
//   - 动画系统：AnimGroupRuntime（默认组永久 + 自定义组互斥 + 平滑过渡）
//   - 管理面板：AnimGroupManager（组列表/编辑器/气泡选择/骨骼显隐/导入导出）
//   - 骨骼层级：默认全折叠

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
import { FileLoader } from './ui/file-loader.js';
import { AnimGroupManager } from './ui/anim-group-manager.js';
import { BoneHierarchyPanel } from './ui/bone-hierarchy.js';
import { MovementPanel } from './ui/movement-panel.js';
import { MolangPanel } from './ui/molang-panel.js';
import * as THREE from './vendor/three.module.js';

/**
 * App — 主应用类
 * 遵循项目 §4.2 复杂应用 ES Module 类架构
 */
class App {
    constructor() {
        // ==== 核心模块 ====
        this.renderer = null;
        this.textureManager = new TextureManager();
        this.molang = new MolangRuntime();
        /** @type {AnimationRuntime|null} */
        this.animationRuntime = null;
        /** @type {AnimGroupRuntime|null} */
        this.animGroupRuntime = null;
        /** @type {SpecialAnimationRuntime|null} */
        this.specialAnimRuntime = null;
        /** @type {MovementController|null} */
        this.movementController = null;
        /** @type {BodyRotationInterpreter|null} */
        this.bodyRotationInterpreter = null;

        // ==== UI 模块 ====
        /** @type {FileLoader|null} */
        this.fileLoader = null;
        /** @type {AnimGroupManager|null} */
        this.animGroupManager = null;
        /** @type {BoneHierarchyPanel|null} */
        this.boneHierarchyPanel = null;
        /** @type {MovementPanel|null} */
        this.movementPanel = null;
        /** @type {MolangPanel|null} */
        this.molangPanel = null;

        // ==== 状态 ====
        this.currentModel = null;
        this._boneHighlightHelper = null;
        /** @type {Map<string, import('./core/keyframe.js').Animation>} 动画全名 → Animation */
        this.currentAnimations = new Map();

        // ==== DOM 引用 ====
        this.elements = {
            canvas: document.getElementById('render-canvas'),
            btnImport: document.getElementById('btn-import'),
            resourceFileInput: document.getElementById('resource-file-input'),
            btnTogglePanels: document.getElementById('btn-toggle-panels'),
            btnTheme: document.getElementById('btn-theme'),
            btnHelp: document.getElementById('btn-help'),
            btnMovement: document.getElementById('btn-movement'),
            btnMolang: document.getElementById('btn-molang'),
            hierarchyTree: document.getElementById('hierarchy-tree'),
            animGroupContainer: document.getElementById('anim-group-container'),
            statusIcon: document.getElementById('status-icon'),
            statusText: document.getElementById('status-text'),
            toast: document.getElementById('toast'),
            modalOverlay: document.getElementById('modal-overlay'),
            modalContent: document.getElementById('modal-content')
        };

        // 面板状态（默认隐藏，仅显示模型）
        this._panelsVisible = false;
        this._darkMode = false;

        // 状态轮播
        this._statusCarouselIndex = 0;
        this._statusCarouselTimer = null;
    }

    /**
     * 初始化
     */
    async init() {
        // 1. 创建渲染器
        this.renderer = new Renderer(this.elements.canvas);
        this.renderer.resize();
        this.renderer.start();

        // 2. 创建动画运行时（outliner 在模型加载后设置）
        this.animationRuntime = new AnimationRuntime(this.molang, null);

        // 3. 创建动画组运行时
        this.animGroupRuntime = new AnimGroupRuntime(this.molang, this.animationRuntime);
        this.animGroupRuntime.animations = this.currentAnimations;
        this.animGroupRuntime.onEvent.push((event, group) => {
            if (event === 'activate') {
                this.showToast(`▶ 激活: ${group.name}`, 'success');
            } else if (event === 'deactivate') {
                this.showToast(`■ 停用: ${group.name}`, 'success');
            }
        });

        // 4. 创建特殊动画运行时（叠加在 AnimGroupRuntime 之上：眨眼/移动/快速移动）
        this.specialAnimRuntime = new SpecialAnimationRuntime(this.molang, this.animationRuntime);

        // 5. 创建移动控制器（同步 q.target_x_rotation / q.target_y_rotation 等到 MoLang）
        this.movementController = new MovementController({
            renderer: this.renderer,
            molang: this.molang,
            onMoveStateChange: (isMoving, isFastMoving) => {
                this.specialAnimRuntime?.setMoveState(isMoving, isFastMoving);
            }
        });

        // 6. 创建身体旋转解释器（直接操作 modelRoot.rotation.y + headCheek 骨骼补偿）
        //    outliner 在模型加载后通过 setOutliner 设置，再调用 findBones()
        this.bodyRotationInterpreter = new BodyRotationInterpreter(this.molang, this.renderer, null);

        // 7. 注册移动方向同步：setTarget 时立即同步 body_y_rotation，避免"屁股朝目标"
        this.movementController.onSetTarget(() => {
            this.bodyRotationInterpreter?.syncToTarget();
        });

        // 8. 注册闲置回调：5 秒无操作 → 摄像头平滑移动到角色正面
        this.movementController.onIdle(() => {
            this.renderer?.moveCameraToFront(1.5);
        });

        // 9. 挂载到渲染循环（顺序：组动画 → 特殊动画叠加 → 移动控制 → 身体旋转解释 → MoLang 面板刷新）
        this.renderer.onUpdate = (dt) => {
            this.animGroupRuntime.tick(dt);
            this.specialAnimRuntime?.tick(dt);
            this.movementController?.tick(dt);
            this.bodyRotationInterpreter?.tick(dt);
            this.molangPanel?.refresh();
        };

        // 10. 初始化 UI 模块
        this.fileLoader = new FileLoader({
            onLoadBbmodel: (f) => this.loadBbmodelFromFile(f),
            onLoadAnimations: (fs) => this.loadAnimationsFromFiles(fs),
            onLoadControllers: (fs) => this._handleLegacyControllers(fs),
            onLoadAnimGroupConfig: (fs) => this._loadAnimGroupConfigFromFiles(fs),
            onToast: (msg, type) => this.showToast(msg, type)
        });

        this.animGroupManager = new AnimGroupManager({
            runtime: this.animGroupRuntime,
            availableAnimations: this.currentAnimations,
            availableBones: [],
            onToast: (msg, type) => this.showToast(msg, type),
            onStatusUpdate: () => this._updateAnimGroupStatus(),
            onEditGroup: (group) => this._onEditGroup(group)
        });

        this.boneHierarchyPanel = new BoneHierarchyPanel(
            { containerId: 'hierarchy-tree' },
            (bone) => this.highlightBone(bone)
        );

        // 10. 创建移动控制面板
        this.movementPanel = new MovementPanel({
            controller: this.movementController,
            onToast: (msg, type) => this.showToast(msg, type)
        });

        // 11. 创建 MoLang 调试面板
        this.molangPanel = new MolangPanel({ molang: this.molang });

        // 12. 绑定事件
        this.bindEvents();
        this.startStatusUpdate();

        // 13. 自动加载 model 文件夹资源
        await this._autoLoadResources();

        console.log('[lunar.studio.bedrock] 初始化完成（动画组架构）');
    }

    // ==== 自动加载 ====

    /**
     * 自动加载 model 文件夹资源
     * 流程：fetch manifest.json → fetch index.bbmodel → fetch animation/*.json → 创建默认动画组
     * @private
     */
    async _autoLoadResources() {
        const baseURL = '/file/read/package/lunar.studio.bedrock/model/';
        try {
            this.setStatusMode('自动加载中...');

            // 1. 获取清单
            const manifestResp = await fetch(baseURL + 'manifest.json');
            if (!manifestResp.ok) {
                console.warn('[App] 未找到 model/manifest.json，跳过自动加载');
                this.setStatusMode('就绪');
                return;
            }
            const manifest = await manifestResp.json();

            // 2. 加载模型
            if (manifest.model) {
                const modelResp = await fetch(baseURL + manifest.model);
                if (modelResp.ok) {
                    const modelJson = await modelResp.json();
                    await this._loadBbmodelFromJson(modelJson, manifest.model);
                } else {
                    console.warn(`[App] 模型加载失败: ${manifest.model}`);
                }
            }

            // 3. 加载动画
            if (Array.isArray(manifest.animations)) {
                const animFiles = [];
                for (const animPath of manifest.animations) {
                    try {
                        const resp = await fetch(baseURL + animPath);
                        if (resp.ok) {
                            const json = await resp.json();
                            const anims = AnimationCodec.parse(json);
                            for (const [name, anim] of anims) {
                                this.currentAnimations.set(name, anim);
                            }
                            animFiles.push(animPath);
                        }
                    } catch (e) {
                        console.warn(`[App] 动画加载失败: ${animPath}`, e);
                    }
                }
                if (animFiles.length > 0) {
                    this.showToast(`自动加载 ${this.currentAnimations.size} 个动画`, 'success');
                }
            }

            // 4. 创建默认动画组并启动
            this._createDefaultAnimGroup();

            // 5. 更新管理面板
            this.animGroupManager?.setAvailableAnimations(this.currentAnimations);
            this.animGroupManager?.refresh();

            // 6. 通知特殊动画运行时加载分类后的动画（眨眼/移动/快速移动）
            this.specialAnimRuntime?.setAnimations(this.currentAnimations);

            // 7. 尝试加载已保存的动画组配置（覆盖默认组）
            await this._tryLoadSavedConfig();

            this.setStatusMode('就绪');
        } catch (err) {
            console.error('[App] 自动加载失败:', err);
            this.showToast(`自动加载失败: ${err.message}`, 'error');
            this.setStatusMode('错误');
        }
    }

    /**
     * 创建默认动画组（包含所有已加载的动画）
     * 如果已有默认组，替换其动画列表
     * @private
     */
    _createDefaultAnimGroup() {
        if (this.currentAnimations.size === 0) return;

        // 如果已有默认组，更新其动画列表
        if (this.animGroupRuntime.defaultGroup) {
            this.animGroupRuntime.defaultGroup.animations = Array.from(this.currentAnimations.keys());
        } else {
            // 创建新的默认组
            const defaultGroup = new AnimGroup('默认组');
            defaultGroup.isDefault = true;
            defaultGroup.loopMode = 'repeat';
            defaultGroup.transitionDuration = 0.3;
            defaultGroup.animations = Array.from(this.currentAnimations.keys());
            this.animGroupRuntime.addGroup(defaultGroup);
        }

        // 启动播放
        this.animGroupRuntime.play();
    }

    /**
     * 尝试加载已保存的动画组配置
     * @private
     */
    async _tryLoadSavedConfig() {
        const baseURL = '/file/read/package/lunar.studio.bedrock/model/';
        try {
            const resp = await fetch(baseURL + 'anim_group_config.json');
            if (!resp.ok) return;
            const json = await resp.json();
            if (json && json.animation_groups) {
                this.animGroupRuntime.importConfig(json, this.currentAnimations);
                this.animGroupManager?.refresh();
                this.showToast('已加载保存的动画组配置', 'success');
            }
        } catch (e) {
            // 配置文件不存在是正常情况，忽略
        }
    }

    // ==== 资源导入 ====

    /**
     * 从 File 对象加载 .bbmodel
     */
    async loadBbmodelFromFile(file) {
        try {
            this.setStatusMode('加载中...');
            this.showToast(`正在加载 ${file.name}...`, 'success');
            const result = await GeometryLoader.loadFromFile(file);
            await this._applyModel(result, file.name);
            this.setStatusMode('就绪');
        } catch (err) {
            console.error('[App] 加载 .bbmodel 失败:', err);
            this.showToast(`加载失败：${err.message}`, 'error');
            this.setStatusMode('错误');
        }
    }

    /**
     * 从 JSON 对象加载 .bbmodel（用于自动加载）
     * @param {object} json
     * @param {string} fileName
     * @private
     */
    async _loadBbmodelFromJson(json, fileName) {
        try {
            this.setStatusMode('加载模型...');
            const result = GeometryLoader.parse(json);
            await this._applyModel(result, fileName);
            this.setStatusMode('就绪');
        } catch (err) {
            console.error('[App] 加载 .bbmodel 失败:', err);
            this.showToast(`模型加载失败：${err.message}`, 'error');
        }
    }

    /**
     * 应用模型到渲染器和各模块
     * @param {object} result GeometryLoader.parse 返回值
     * @param {string} fileName
     * @private
     */
    async _applyModel(result, fileName) {
        this.currentModel = result;

        // 加载纹理
        await this.textureManager.loadFromBBModel(result.textures);

        // 构建模型
        this.renderer.buildModel(result.outliner, this.textureManager);

        // 同步动画运行时的 outliner 引用
        this.animationRuntime.outliner = result.outliner;
        this.animationRuntime.rebuild();

        // 更新骨骼层级面板（默认全折叠）
        this.boneHierarchyPanel?.loadOutliner(result.outliner);

        // 更新管理面板的可用骨骼列表
        const bones = [];
        result.outliner.traverseBones(bone => bones.push(bone));
        this.animGroupManager?.setAvailableBones(bones);

        // 更新身体旋转解释器的骨骼引用（用于直接操作 headCheek 等骨骼）
        if (this.bodyRotationInterpreter) {
            this.bodyRotationInterpreter.outliner = result.outliner;
            this.bodyRotationInterpreter.findBones();
        }

        const boneCount = result.outliner.boneCount;
        const cubeCount = result.outliner.cubeCount;
        this.showToast(`加载成功：${boneCount} 骨骼 / ${cubeCount} 立方体`, 'success');
    }

    /**
     * 从 File 对象数组加载动画 JSON
     */
    async loadAnimationsFromFiles(files) {
        try {
            const loaded = [];
            for (const file of files) {
                const text = await file.text();
                const json = JSON.parse(text);
                const anims = AnimationCodec.parse(json);
                for (const [name, anim] of anims) {
                    this.currentAnimations.set(name, anim);
                    loaded.push(name);
                }
            }
            this.animGroupManager?.setAvailableAnimations(this.currentAnimations);
            this.animGroupManager?.refresh();
            // 通知特殊动画运行时重新分类
            this.specialAnimRuntime?.setAnimations(this.currentAnimations);
            this.showToast(`加载 ${loaded.length} 个动画（共 ${this.currentAnimations.size} 个）`, 'success');

            // 如果尚无默认组，自动创建
            if (!this.animGroupRuntime.defaultGroup && this.currentAnimations.size > 0) {
                this._createDefaultAnimGroup();
            }
        } catch (err) {
            console.error('[App] 加载动画失败:', err);
            this.showToast(`动画加载失败：${err.message}`, 'error');
        }
    }

    /**
     * 处理旧版控制器文件（已弃用）
     * @private
     */
    async _handleLegacyControllers(files) {
        this.showToast(`旧版控制器已弃用，请使用动画组管理面板配置`, 'warning');
    }

    /**
     * 从 File 对象数组加载动画组配置 JSON
     * @private
     */
    async _loadAnimGroupConfigFromFiles(files) {
        try {
            for (const file of files) {
                const text = await file.text();
                const json = JSON.parse(text);
                if (json && json.animation_groups) {
                    this.animGroupRuntime.importConfig(json, this.currentAnimations);
                    this.animGroupManager?.refresh();
                    this.showToast(`动画组配置已导入`, 'success');
                } else {
                    this.showToast(`无效的动画组配置文件: ${file.name}`, 'error');
                }
            }
        } catch (err) {
            console.error('[App] 加载动画组配置失败:', err);
            this.showToast(`配置加载失败：${err.message}`, 'error');
        }
    }

    // ==== 骨骼高亮 ====

    /**
     * 高亮选中的骨骼（在 3D 场景中绘制包围盒）
     * @param {import('./core/outliner.js').Bone} bone
     */
    highlightBone(bone) {
        if (!this.renderer || !bone || !bone.sceneObject) return;

        // 清除旧的高亮
        if (this._boneHighlightHelper) {
            this.renderer.scene.remove(this._boneHighlightHelper);
            this._boneHighlightHelper.geometry?.dispose();
            this._boneHighlightHelper.material?.dispose();
            this._boneHighlightHelper = null;
        }

        // 计算骨骼子树的包围盒
        const box = new THREE.Box3();
        box.makeEmpty();
        bone.sceneObject.traverse((obj) => {
            if (obj.isMesh) box.expandByObject(obj);
        });
        if (box.isEmpty()) return;

        // 用线框 BoxHelper 显示高亮
        const helper = new THREE.Box3Helper(box, new THREE.Color(0x9d6bff));
        helper.material.linewidth = 2;
        helper.material.transparent = true;
        helper.material.opacity = 0.9;
        this.renderer.scene.add(helper);
        this._boneHighlightHelper = helper;
    }

    /**
     * 动画组编辑模式切换回调
     * 当用户在 AnimGroupManager 中打开/关闭编辑器时，通知骨骼层级面板
     * 进入/退出显隐编辑模式
     * @param {import('./core/anim-group-runtime.js').AnimGroup|null} group
     * @private
     */
    _onEditGroup(group) {
        if (!this.boneHierarchyPanel) return;
        if (group) {
            this.boneHierarchyPanel.setEditingGroup(group, (boneName) => {
                this._toggleBoneVisibilityForGroup(group, boneName);
            });
            this.showToast(`骨骼层级已进入「${group.name}」显隐编辑模式`, 'success');
        } else {
            this.boneHierarchyPanel.clearEditingGroup();
        }
    }

    /**
     * 切换骨骼在动画组中的显隐覆盖
     * 三态循环：继承默认(无覆盖) → 显式隐藏 → 显式可见 → 继承默认
     * @param {import('./core/anim-group-runtime.js').AnimGroup} group
     * @param {string} boneName
     * @private
     */
    _toggleBoneVisibilityForGroup(group, boneName) {
        const hasOverride = boneName in group.boneVisibility;
        if (!hasOverride) {
            // 继承 → 显式隐藏
            group.boneVisibility[boneName] = false;
        } else if (group.boneVisibility[boneName] === false) {
            // 显式隐藏 → 显式可见
            group.boneVisibility[boneName] = true;
        } else {
            // 显式可见 → 移除覆盖（继承默认）
            delete group.boneVisibility[boneName];
        }
        // 刷新骨骼层级面板（更新眼睛图标状态）
        this.boneHierarchyPanel?._render();
        // 刷新编辑器（更新覆盖数量提示）
        if (this.animGroupManager?.editingGroup === group.name) {
            this.animGroupManager._renderEditor(group.name);
        }
        // 实时应用到运行时（如果该组正在播放）
        if (this.animGroupRuntime) {
            // 编辑时取消显隐延迟，立即生效
            this.animGroupRuntime._visDelayActive = false;
            this.animGroupRuntime._applyBoneVisibility(
                this.animGroupRuntime.activeCustomGroup || this.animGroupRuntime.defaultGroup,
                false
            );
        }
    }

    /**
     * 设置 z-fighting 微调偏移，并重建当前模型
     * @param {number} bias 0~0.1
     */
    setInflateBias(bias) {
        if (!this.renderer) return;
        this.renderer.setInflateBias(bias);
        if (this.currentModel) {
            this.renderer.buildModel(this.currentModel.outliner, this.textureManager);
            this._boneHighlightHelper = null;
            this.boneHierarchyPanel?.clearSelection();
            this.animationRuntime.outliner = this.currentModel.outliner;
            this.animationRuntime.rebuild();
        }
    }

    // ==== 事件绑定 ====

    /**
     * 绑定事件
     */
    bindEvents() {
        // 统一资源导入按钮
        this.elements.btnImport.addEventListener('click', () => {
            this.elements.resourceFileInput.click();
        });
        this.elements.resourceFileInput.addEventListener('change', async (e) => {
            const files = Array.from(e.target.files);
            if (files.length === 0) return;
            await this.fileLoader?._classifyAndLoad(files);
            e.target.value = '';
        });

        // 面板切换（左/右侧面板）
        this.elements.btnTogglePanels.addEventListener('click', () => {
            this._panelsVisible = !this._panelsVisible;
            document.querySelectorAll('.side-panel').forEach(p => {
                p.style.display = this._panelsVisible ? '' : 'none';
            });
            // 切换按钮图标状态
            this.elements.btnTogglePanels.style.opacity = this._panelsVisible ? '1' : '0.7';
        });
        // 初始隐藏状态：按钮半透明
        this.elements.btnTogglePanels.style.opacity = '0.7';

        // 主题切换
        this.elements.btnTheme.addEventListener('click', () => {
            this._darkMode = !this._darkMode;
            document.body.classList.toggle('dark-mode', this._darkMode);
            this.elements.btnTheme.innerHTML = this._darkMode
                ? '<i class="fas fa-sun"></i>'
                : '<i class="fas fa-moon"></i>';
        });

        // 移动控制面板切换
        if (this.elements.btnMovement) {
            this.elements.btnMovement.addEventListener('click', () => {
                this.movementPanel?.toggle();
            });
        }

        // MoLang 调试面板切换
        if (this.elements.btnMolang) {
            this.elements.btnMolang.addEventListener('click', () => {
                this.molangPanel?.toggle();
            });
        }

        // 帮助
        this.elements.btnHelp.addEventListener('click', () => {
            this.showModal(`
                <h3 style="margin-top:0"><i class="fas fa-question-circle"></i> 操作说明</h3>
                <div style="margin-top:16px;font-size:14px;line-height:1.8">
                    <p><b>导入资源</b>：点击右下角"导入"按钮或拖拽文件到页面，自动识别模型/动画/动画组配置</p>
                    <p><b>自动加载</b>：页面打开时自动加载 model 文件夹中的 index.bbmodel 及关联动画</p>
                    <p><b>切换面板</b>：点击右下角"切换面板"按钮，显示/隐藏骨骼层级与动画组管理面板</p>
                    <p><b>移动控制</b>：点击右下角"移动控制"按钮（<i class="fas fa-arrows-alt"></i>）打开移动面板</p>
                    <ul style="margin-left:20px">
                        <li>位置控制：输入 X/Y/Z 坐标，点击"移动到目标"平滑移动，或"瞬移"直接到位</li>
                        <li>朝向控制：拖动偏航/俯仰滑块精确调整（对应 q.target_y_rotation / q.target_x_rotation）</li>
                        <li>鼠标追踪：启用后模型朝向鼠标投影点；自动锁定会持续移动到鼠标位置</li>
                        <li>移动速度：滑块调节 1~30 单位/秒</li>
                    </ul>
                    <p><b>动画组管理</b>：右侧"动画组"面板管理动画组</p>
                    <ul style="margin-left:20px">
                        <li>默认组（<i class="fas fa-home" style="color:var(--success)"></i>）：永久生效，循环播放</li>
                        <li>自定义组：点击 ▶ 激活（互斥），再点击停用，平滑过渡</li>
                        <li>编辑组：点击 <i class="fas fa-edit"></i> 设置循环模式、过渡时长、动画序列、骨骼显隐</li>
                        <li>气泡选择：点击动画气泡添加/移除到组</li>
                    </ul>
                    <p><b>特殊动画</b>：名称含 <code>.blink</code> / <code>.move</code> / <code>.fast_move</code> 的动画自动归类，不在列表显示</p>
                    <ul style="margin-left:20px">
                        <li><code>.blink</code>：随机 3-5 秒间隔触发一次眨眼</li>
                        <li><code>.move</code>：模型移动时循环播放</li>
                        <li><code>.fast_move</code>：模型长距离/快速移动时循环播放</li>
                    </ul>
                    <p><b>身体旋转追踪</b>：设置朝向后，头部立即转向目标（<code>q.target_y_rotation</code>），身体缓慢转动到面朝方向（<code>q.body_y_rotation</code>）；当头身角度差 > 35° 时强制加速身体旋转</p>
                    <p><b>摄像头自动归位</b>：5 秒无操作后，摄像头平滑移动到角色正面</p>
                    <p><b>MoLang 调试</b>：点击右下角 <i class="fas fa-code"></i> 按钮打开调试面板，实时显示所有 MoLang 变量值</p>
                    <p><b>动画名称</b>：所有动画仅显示最后一段（如 standby_animation-0）</p>
                    <p><b>骨骼层级</b>：左侧面板默认全折叠，点击箭头展开，点击名称高亮</p>
                    <p><b>配置导入导出</b>：动画组面板工具栏 <i class="fas fa-file-import"></i>/<i class="fas fa-file-export"></i> 按钮导入导出 JSON 配置</p>
                    <p><b>视角控制</b>：左键拖拽旋转 / 右键拖拽平移 / 滚轮缩放</p>
                    <p><b>状态气泡</b>：左上角气泡每 3 秒轮播 FPS / 骨骼数 / 立方体数 / 三角面 / 动画状态 / 相机位置</p>
                </div>
                <div style="text-align:right;margin-top:16px">
                    <button class="btn-glass btn-glass-primary" onclick="document.getElementById('modal-overlay').classList.remove('visible')">关闭</button>
                </div>
            `);
        });

        // 标签切换
        document.querySelectorAll('.panel-tab').forEach(tab => {
            tab.addEventListener('click', () => {
                const tabName = tab.dataset.tab;
                const panelBody = tab.parentElement.parentElement;
                panelBody.querySelectorAll('.panel-tab').forEach(t => t.classList.remove('active'));
                tab.classList.add('active');
                panelBody.querySelectorAll('.panel-body').forEach(b => b.style.display = 'none');
                const target = panelBody.querySelector(`#tab-${tabName}`);
                if (target) target.style.display = '';
            });
        });

        // 窗口大小变化
        window.addEventListener('resize', () => {
            this.renderer?.resize();
        });

        // 模态框点击遮罩关闭
        this.elements.modalOverlay.addEventListener('click', (e) => {
            if (e.target === this.elements.modalOverlay) {
                this.elements.modalOverlay.classList.remove('visible');
            }
        });
    }

    // ==== UI 更新 ====

    /**
     * 更新动画组状态（由 AnimGroupManager 调用）
     * @private
     */
    _updateAnimGroupStatus() {
        // 触发状态栏刷新
        if (this.animGroupManager) {
            this.animGroupManager.refresh();
        }
    }

    /**
     * 启动状态气泡轮播
     * 左上角气泡每 3 秒轮播一项：FPS → 骨骼 → 立方体 → 三角面 → 动画状态 → 相机
     */
    startStatusUpdate() {
        // 立即显示第一项
        this._updateStatusBubble();

        // 每 3 秒切换一项
        this._statusCarouselTimer = setInterval(() => {
            this._statusCarouselIndex = (this._statusCarouselIndex + 1) % 6;
            this._updateStatusBubble();
        }, 3000);

        // 高频更新动画组进度（不影响气泡文字，仅刷新管理面板进度条）
        setInterval(() => {
            if (this.animGroupRuntime?.playing) {
                this.animGroupManager?.updateProgress();
            }
        }, 100);
    }

    /**
     * 更新状态气泡内容（根据当前轮播索引）
     * @private
     */
    _updateStatusBubble() {
        if (!this.renderer || !this.elements.statusText) return;

        const icons = [
            'fas fa-tachometer-alt',  // FPS
            'fas fa-bone',             // 骨骼
            'fas fa-cube',             // 立方体
            'fas fa-vector-square',    // 三角面
            'fas fa-film',             // 动画
            'fas fa-video'             // 相机
        ];
        let text = '';
        switch (this._statusCarouselIndex) {
            case 0: // FPS
                text = `${this.renderer.fps} FPS`;
                break;
            case 1: // 骨骼数
                text = this.currentModel ? `${this.currentModel.outliner.boneCount} 骨骼` : '无模型';
                break;
            case 2: // 立方体数
                text = this.currentModel ? `${this.currentModel.outliner.cubeCount} 立方体` : '无模型';
                break;
            case 3: // 三角面数
                text = `${this.renderer.getTriangleCount()} 面`;
                break;
            case 4: // 动画状态
                if (this.animGroupRuntime?.playing && this.animGroupRuntime.currentGroup) {
                    const g = this.animGroupRuntime.currentGroup;
                    const animShort = this.animGroupRuntime.currentAnimationShortName;
                    text = animShort ? `${g.name} ▶ ${animShort}` : g.name;
                } else {
                    text = this._statusModeText || '就绪';
                }
                break;
            case 5: // 相机位置
                text = this.renderer.getCameraPositionString();
                break;
        }
        this.elements.statusIcon.className = icons[this._statusCarouselIndex];
        this.elements.statusText.textContent = text;
    }

    // ==== 工具方法 ====

    /**
     * 设置状态模式（暂存到 _statusModeText，供气泡轮播读取）
     */
    setStatusMode(mode) {
        this._statusModeText = mode;
        // 如果当前轮播正在显示"动画状态"项，立即刷新气泡
        if (this._statusCarouselIndex === 4) {
            this._updateStatusBubble();
        }
    }

    /**
     * 显示 Toast
     */
    showToast(message, type = 'success') {
        const toast = this.elements.toast;
        toast.textContent = message;
        toast.className = `toast ${type} visible`;
        clearTimeout(this._toastTimeout);
        this._toastTimeout = setTimeout(() => {
            toast.classList.remove('visible');
        }, 3000);
    }

    /**
     * 显示模态框
     */
    showModal(htmlContent) {
        this.elements.modalContent.innerHTML = htmlContent;
        this.elements.modalOverlay.classList.add('visible');
    }
}

// ==== 启动 ====
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => {
        const app = new App();
        app.init();
        window.__bedrockRenderEngine = app;  // 暴露给调试
    });
} else {
    const app = new App();
    app.init();
    window.__bedrockRenderEngine = app;
}
