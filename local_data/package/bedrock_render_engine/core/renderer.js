// ==== renderer.js — Three.js 渲染器 ====

import * as THREE from '../vendor/three.module.js';
import { BedrockCoordinate } from './geometry-loader.js';
import { Bone, CubeElement } from './outliner.js';
import { MaterialSystem } from './material-system.js';

/**
 * Renderer — Three.js 场景管理
 * 参考 Blockbench js/preview/preview.js + canvas.js，重写为独立 ES Module
 * 不依赖 DOM 结构（除 canvas），不依赖 Vue
 */
export class Renderer {
    constructor(canvas) {
        this.canvas = canvas;

        // Three.js 核心对象
        this.scene = new THREE.Scene();
        this.scene.background = null;  // 透明背景，让 CSS 渐变穿透

        this.camera = new THREE.PerspectiveCamera(45, 1, 0.1, 5000);
        this.camera.position.set(20, 25, 35);

        this.renderer = new THREE.WebGLRenderer({
            canvas: canvas,
            antialias: true,
            alpha: true,
            powerPreference: 'high-performance'
        });
        this.renderer.setPixelRatio(window.devicePixelRatio);
        this.renderer.outputColorSpace = THREE.SRGBColorSpace;

        // 光照
        const ambient = new THREE.AmbientLight(0xffffff, 0.6);
        this.scene.add(ambient);

        const dirLight = new THREE.DirectionalLight(0xffffff, 1.0);
        dirLight.position.set(10, 20, 15);
        this.scene.add(dirLight);

        const dirLight2 = new THREE.DirectionalLight(0xaabbff, 0.4);
        dirLight2.position.set(-10, 10, -15);
        this.scene.add(dirLight2);

        // 地面网格（参考）
        this.gridHelper = new THREE.GridHelper(40, 40, 0x444466, 0x222244);
        this.gridHelper.position.y = 0;
        this.scene.add(this.gridHelper);

        // 模型根容器
        this.modelRoot = new THREE.Group();
        this.scene.add(this.modelRoot);

        // z-fighting 微调：每个 cube 的额外膨胀偏移（用户可调）
        this.inflateBias = 0.01;

        // 简易轨道控制器
        this.controls = new SimpleOrbitControls(canvas, this.camera);

        // 点击跳过登场动画
        canvas.addEventListener('pointerdown', () => {
            if (this._introActive) this.skipIntroAnimation();
        });

        // FPS 计数
        this.fps = 0;
        this._frameCount = 0;
        this._lastFpsTime = performance.now();

        // 渲染循环
        this._animate = this._animate.bind(this);
        this._running = false;
        this._lastFrameTime = 0;

        // 材质系统
        this.materialSystem = new MaterialSystem();
        this.renderMethod = 'alphatest';

        // 登场动画状态
        /** @type {boolean} 登场动画是否正在播放 */
        this._introActive = false;
        /** @type {number} 登场动画开始时间戳（毫秒） */
        this._introStartTime = 0;
        /** @type {number} 登场动画持续时间（毫秒） */
        this._introDuration = 2500;
        /** @type {THREE.Vector3} 起始相机位置 */
        this._introStartPos = new THREE.Vector3();
        /** @type {THREE.Vector3} 结束相机位置 */
        this._introEndPos = new THREE.Vector3();
        /** @type {THREE.Vector3} 聚焦目标点 */
        this._introTarget = new THREE.Vector3();
        /** @type {boolean} 登场动画前的控制器状态 */
        this._introSavedControlsEnabled = true;

        // 当前纹理
        this.currentTexture = null;

        /** @type {((deltaTime: number) => void)|null} 每帧更新回调（动画系统挂载点） */
        this.onUpdate = null;
    }

    /**
     * 启动渲染循环
     */
    start() {
        if (this._running) return;
        this._running = true;
        this._animate();
    }

    /**
     * 停止渲染循环
     */
    stop() {
        this._running = false;
    }

    /**
     * 调整画布大小
     */
    resize() {
        const w = this.canvas.clientWidth || window.innerWidth;
        const h = this.canvas.clientHeight || window.innerHeight;
        this.camera.aspect = w / h;
        this.camera.updateProjectionMatrix();
        this.renderer.setSize(w, h, false);
    }

    /**
     * 渲染循环
     * @private
     */
    _animate() {
        if (!this._running) return;

        const now = performance.now();
        this._frameCount++;
        if (now - this._lastFpsTime >= 1000) {
            this.fps = Math.round(this._frameCount * 1000 / (now - this._lastFpsTime));
            this._frameCount = 0;
            this._lastFpsTime = now;
        }

        // 计算帧间隔（秒），首帧取 16ms
        let deltaSec = 0.016;
        if (this._lastFrameTime > 0) {
            deltaSec = Math.min(0.1, (now - this._lastFrameTime) / 1000);
        }
        this._lastFrameTime = now;

        // 动画系统更新钩子
        if (this.onUpdate) {
            try {
                this.onUpdate(deltaSec);
            } catch (e) {
                console.warn('[Renderer] onUpdate 回调异常:', e);
            }
        }

        // 登场动画：相机从后方远处拉近并旋转到正面
        if (this._introActive) {
            const elapsed = now - this._introStartTime;
            let t = Math.min(1, elapsed / this._introDuration);
            // smoothstep 缓动（先慢后快再慢）
            const eased = t * t * (3 - 2 * t);

            // 球面插值：用弧线轨迹而非直线，避免穿过模型
            const startOffset = this._introStartPos.clone().sub(this._introTarget);
            const endOffset = this._introEndPos.clone().sub(this._introTarget);
            const startSph = new THREE.Spherical().setFromVector3(startOffset);
            const endSph = new THREE.Spherical().setFromVector3(endOffset);

            const radius = THREE.MathUtils.lerp(startSph.radius, endSph.radius, eased);
            const phi = THREE.MathUtils.lerp(startSph.phi, endSph.phi, eased);
            const theta = THREE.MathUtils.lerp(startSph.theta, endSph.theta, eased);

            const sph = new THREE.Spherical(radius, phi, theta);
            const offset = new THREE.Vector3().setFromSpherical(sph);
            this.camera.position.copy(this._introTarget).add(offset);
            this.controls.target.copy(this._introTarget);
            this.camera.lookAt(this._introTarget);

            if (t >= 1) {
                this._introActive = false;
                // 同步控制器内部球面状态，避免 update() 闪回旧位置
                const endOffset = this._introEndPos.clone().sub(this._introTarget);
                this.controls._spherical.setFromVector3(endOffset);
                this.controls.enabled = this._introSavedControlsEnabled;
                this.controls.update();
            }
        } else {
            this.controls.update();
        }

        this.renderer.render(this.scene, this.camera);
        requestAnimationFrame(this._animate);
    }

    /**
     * 从 Outliner 构建模型
     * @param {import('./outliner.js').Outliner} outliner
     * @param {import('./texture-manager.js').TextureManager} textureManager
     */
    buildModel(outliner, textureManager) {
        // 清空旧模型
        this.clearModel();

        // 获取默认纹理
        this.currentTexture = textureManager.getDefault();
        this.materialSystem.setDefaultTexture(this.currentTexture);
        const texWidth = textureManager.metadata.get('0')?.uv_width || 16;
        const texHeight = textureManager.metadata.get('0')?.uv_height || 16;

        // 递归构建骨骼层级（根骨骼的 parentBone 为 null）
        for (const rootBone of outliner.roots) {
            const boneObject = this._buildBoneObject(rootBone, null, texWidth, texHeight);
            if (boneObject) {
                this.modelRoot.add(boneObject);
            }
        }

        // 播放登场动画（相机从后方远处旋转到正面）
        this.playIntroAnimation();
    }

    /**
     * 播放登场动画
     * 相机从模型后方高处开始，拉近并旋转到正面，聚焦角色
     * @param {number} duration 持续时间（秒），默认 2.5
     */
    playIntroAnimation(duration = 2.5) {
        const box = new THREE.Box3();
        box.makeEmpty();
        this.modelRoot.traverse((obj) => {
            if (obj.isMesh) box.expandByObject(obj);
        });
        if (box.isEmpty()) {
            this._fitCameraToModel();
            return;
        }

        const center = box.getCenter(new THREE.Vector3());
        const size = box.getSize(new THREE.Vector3());
        const maxDim = Math.max(size.x, size.y, size.z, 1);

        // 结束位置：正面（-Z 方向），拉近，略偏上俯视
        const endDist = maxDim * 1.0;
        const endPos = new THREE.Vector3(
            center.x - maxDim * 0.2,
            center.y + maxDim * 0.3,
            center.z - endDist  // -Z = 正面
        );

        // 起始位置：后方远处（+Z 方向，看到后脑勺），更高俯视
        const startDist = maxDim * 3.5;
        const startPos = new THREE.Vector3(
            center.x + maxDim * 0.3,
            center.y + maxDim * 1.2,
            center.z + startDist  // +Z = 后方
        );

        this._introActive = true;
        this._introStartTime = performance.now();
        this._introDuration = duration * 1000;
        this._introStartPos.copy(startPos);
        this._introEndPos.copy(endPos);
        this._introTarget.copy(center);
        this._introSavedControlsEnabled = this.controls.enabled;
        this.controls.enabled = false;

        // 立即设置起始位置
        this.camera.position.copy(startPos);
        this.camera.lookAt(center);
        this.controls.target.copy(center);
    }

    /**
     * 跳过登场动画（立即到达终点）
     */
    skipIntroAnimation() {
        if (!this._introActive) return;
        this._introActive = false;
        this.camera.position.copy(this._introEndPos);
        this.controls.target.copy(this._introTarget);
        // 同步控制器内部球面状态
        const endOffset = this._introEndPos.clone().sub(this._introTarget);
        this.controls._spherical.setFromVector3(endOffset);
        this.controls.enabled = this._introSavedControlsEnabled;
        this.controls.update();
    }

    /**
     * 递归构建骨骼的 Three.js Object3D
     *
     * 关键点：Three.js 的层级变换是相对父级的。
     * .bbmodel 中 bone.origin 是绝对世界坐标，子骨骼需转为相对父骨骼 origin 的偏移。
     *
     * @param {Bone} bone
     * @param {Bone|null} parentBone 父骨骼（根骨骼为 null）
     * @param {number} texWidth
     * @param {number} texHeight
     * @returns {THREE.Group|null}
     * @private
     */
    _buildBoneObject(bone, parentBone, texWidth, texHeight) {
        const boneObject = new THREE.Group();
        boneObject.name = bone.name;

        // 位置：根骨骼用绝对 origin；子骨骼用相对父骨骼 origin 的偏移
        if (parentBone) {
            boneObject.position.set(
                bone.origin[0] - parentBone.origin[0],
                bone.origin[1] - parentBone.origin[1],
                bone.origin[2] - parentBone.origin[2]
            );
        } else {
            boneObject.position.set(bone.origin[0], bone.origin[1], bone.origin[2]);
        }

        // 旋转：.bbmodel 已是 Blockbench 内部坐标系，直接转弧度
        // Euler 顺序 ZYX（Blockbench Format.euler_order 默认值，= extrinsic XYZ）
        const rot = BedrockCoordinate.rotationToThree(bone.rotation);
        boneObject.rotation.order = 'ZYX';
        boneObject.rotation.set(rot[0], rot[1], rot[2]);
        bone.sceneObject = boneObject;

        // 构建该骨骼直接挂载的 cube 网格
        for (const cube of bone.cubes) {
            const mesh = this._buildCubeMesh(cube, bone, texWidth, texHeight);
            if (mesh) {
                boneObject.add(mesh);
                cube.mesh = mesh;
            }
        }

        // 递归构建子骨骼（传入当前骨骼作为 parentBone）
        for (const child of bone.children) {
            if (child instanceof Bone) {
                const childObject = this._buildBoneObject(child, bone, texWidth, texHeight);
                if (childObject) {
                    boneObject.add(childObject);
                }
            }
        }

        return boneObject;
    }

    /**
     * 构建 cube 的 BufferGeometry 网格
     *
     * 参考 Blockbench cube.js updateGeometry：
     * 1. 几何体顶点 = cube.from/to - cube.origin（几何体围绕 cube 的 pivot 构建）
     * 2. mesh.position = cube.origin - bone.origin（在骨骼局部空间的位置）
     * 3. cube 旋转直接应用到 mesh（因几何体已围绕原点，旋转即绕 pivot）
     *
     * @param {CubeElement} cube
     * @param {Bone} bone 父骨骼
     * @param {number} texWidth
     * @param {number} texHeight
     * @returns {THREE.Mesh|null}
     * @private
     */
    _buildCubeMesh(cube, bone, texWidth, texHeight) {
        // 几何体以 cube.origin 为中心构建（参考 Blockbench cube.js updateGeometry）
        const cox = cube.origin[0], coy = cube.origin[1], coz = cube.origin[2];

        // 应用 inflate 膨胀：参考 Blockbench cube.js adjustFromAndToForInflateAndStretch
        // inflate 正值向外膨胀，负值收缩；额外加 0.01 微膨胀防止共面 z-fighting
        const inflate = (cube.inflate || 0) + this.inflateBias;
        const x1 = cube.from[0] - cox - inflate, y1 = cube.from[1] - coy - inflate, z1 = cube.from[2] - coz - inflate;
        const x2 = cube.to[0] - cox + inflate, y2 = cube.to[1] - coy + inflate, z2 = cube.to[2] - coz + inflate;

        // 构建 6 面，跳过 texture=null 的面
        const positions = [];
        const normals = [];
        const uvs = [];
        const indices = [];
        let vertexOffset = 0;

        // 面定义：[normal, [4 corners], uv_face_key]
        const faces = [
            { normal: [0, 0, 1], corners: [[x1, y1, z2], [x2, y1, z2], [x2, y2, z2], [x1, y2, z2]], key: 'south' },   // +Z
            { normal: [0, 0, -1], corners: [[x2, y1, z1], [x1, y1, z1], [x1, y2, z1], [x2, y2, z1]], key: 'north' },  // -Z
            { normal: [1, 0, 0], corners: [[x2, y1, z1], [x2, y1, z2], [x2, y2, z2], [x2, y2, z1]], key: 'east' },    // +X
            { normal: [-1, 0, 0], corners: [[x1, y1, z2], [x1, y1, z1], [x1, y2, z1], [x1, y2, z2]], key: 'west' },   // -X
            { normal: [0, 1, 0], corners: [[x1, y2, z2], [x2, y2, z2], [x2, y2, z1], [x1, y2, z1]], key: 'up' },      // +Y
            { normal: [0, -1, 0], corners: [[x1, y1, z1], [x2, y1, z1], [x2, y1, z2], [x1, y1, z2]], key: 'down' }    // -Y
        ];

        for (const face of faces) {
            const faceData = cube.faces[face.key];
            if (!faceData || faceData.texture === null || faceData.texture === undefined) continue;
            if (!faceData.uv || faceData.uv.length < 4) continue;

            // UV 转换：Bedrock 像素空间 → Three.js 0-1 空间
            // Bedrock UV 原点在左上，Three.js 默认在左下
            // 由于 texture.flipY = false，纹理加载不翻转，UV 直接使用 Bedrock 的左上原点
            const [u1, v1, u2, v2] = faceData.uv;
            const ru1 = u1 / texWidth;
            const ru2 = u2 / texWidth;
            const rv1 = v1 / texHeight;
            const rv2 = v2 / texHeight;

            // 4 个顶点（顺序：左下、右下、右上、左上，对应 UV 的 (u1,v2)、(u2,v2)、(u2,v1)、(u1,v1)）
            // 但因为 flipY=false，v 增长方向向下，所以 (u1,v1) 是左上
            for (const corner of face.corners) {
                positions.push(corner[0], corner[1], corner[2]);
                normals.push(face.normal[0], face.normal[1], face.normal[2]);
            }
            // UV：左上 → 右上 → 右下 → 左下（对应 corners 的顺序）
            uvs.push(ru1, rv1, ru2, rv1, ru2, rv2, ru1, rv2);

            // 两个三角形（0-1-2, 0-2-3）
            indices.push(vertexOffset, vertexOffset + 1, vertexOffset + 2);
            indices.push(vertexOffset, vertexOffset + 2, vertexOffset + 3);
            vertexOffset += 4;
        }

        if (positions.length === 0) return null;

        const geometry = new THREE.BufferGeometry();
        geometry.setAttribute('position', new THREE.Float32BufferAttribute(positions, 3));
        geometry.setAttribute('normal', new THREE.Float32BufferAttribute(normals, 3));
        geometry.setAttribute('uv', new THREE.Float32BufferAttribute(uvs, 2));
        geometry.setIndex(indices);

        // 材质：根据 render method 创建
        const material = this._createMaterial();

        const mesh = new THREE.Mesh(geometry, material);
        mesh.name = cube.name;

        // mesh 在骨骼局部空间的位置 = cube.origin - bone.origin
        // （几何体已围绕 cube.origin 构建，故 mesh 需偏移到 cube.origin 在骨骼空间的位置）
        mesh.position.set(
            cube.origin[0] - bone.origin[0],
            cube.origin[1] - bone.origin[1],
            cube.origin[2] - bone.origin[2]
        );

        // cube 自身旋转：几何体已围绕原点（即 cube.pivot），直接旋转 mesh 即绕 pivot 旋转
        // Euler 顺序 ZYX（与骨骼一致，Blockbench 默认值）
        if (cube.rotation && (cube.rotation[0] !== 0 || cube.rotation[1] !== 0 || cube.rotation[2] !== 0)) {
            const rot = BedrockCoordinate.rotationToThree(cube.rotation);
            mesh.rotation.order = 'ZYX';
            mesh.rotation.set(rot[0], rot[1], rot[2]);
        }

        return mesh;
    }

    /**
     * 根据 render method 创建材质（委托给 MaterialSystem）
     * @returns {THREE.Material}
     * @private
     */
    _createMaterial() {
        return this.materialSystem.createMaterial(this.currentTexture);
    }

    /**
     * 切换 render method，重建所有材质
     * @param {string} method opaque|alphatest|translucent|double_sided
     */
    setRenderMethod(method) {
        this.renderMethod = method;
        this.materialSystem.setRenderMethod(method);
        this.materialSystem.updateAllMaterials(this.modelRoot);
    }

    /**
     * 切换光照模式
     * @param {boolean} lit true=MeshLambertMaterial, false=MeshBasicMaterial
     */
    setLit(lit) {
        this.materialSystem.setLit(lit);
        this.materialSystem.updateAllMaterials(this.modelRoot);
    }

    /**
     * 设置 alphatest 阈值
     * @param {number} threshold 0-1
     */
    setAlphaTestThreshold(threshold) {
        this.materialSystem.setAlphaTestThreshold(threshold);
        this.materialSystem.updateAllMaterials(this.modelRoot);
    }

    /**
     * 设置 translucent 全局不透明度
     * @param {number} opacity 0-1
     */
    setTranslucentOpacity(opacity) {
        this.materialSystem.setTranslucentOpacity(opacity);
        this.materialSystem.updateAllMaterials(this.modelRoot);
    }

    /**
     * 获取材质系统描述
     */
    getMaterialDescription() {
        return this.materialSystem.description;
    }

    /**
     * 自动适配相机到模型
     * @private
     */
    _fitCameraToModel() {
        const box = new THREE.Box3();
        box.makeEmpty();
        this.modelRoot.traverse((obj) => {
            if (obj.isMesh) {
                box.expandByObject(obj);
            }
        });

        if (box.isEmpty()) return;

        const center = box.getCenter(new THREE.Vector3());
        const size = box.getSize(new THREE.Vector3());
        const maxDim = Math.max(size.x, size.y, size.z, 1);
        const distance = maxDim * 2.5;

        this.camera.position.set(center.x + distance * 0.6, center.y + distance * 0.8, center.z + distance);
        this.controls.target.copy(center);
        this.controls.update();
    }

    /**
     * 清空当前模型
     */
    clearModel() {
        this.modelRoot.traverse((obj) => {
            if (obj.isMesh) {
                obj.geometry?.dispose();
                obj.material?.dispose();
            }
        });
        while (this.modelRoot.children.length > 0) {
            this.modelRoot.remove(this.modelRoot.children[0]);
        }
    }

    /**
     * 获取模型三角面数
     */
    getTriangleCount() {
        let count = 0;
        this.modelRoot.traverse((obj) => {
            if (obj.isMesh && obj.geometry?.index) {
                count += obj.geometry.index.count / 3;
            }
        });
        return Math.round(count);
    }

    /**
     * 获取相机位置字符串
     */
    getCameraPositionString() {
        const p = this.camera.position;
        return `${p.x.toFixed(1)}, ${p.y.toFixed(1)}, ${p.z.toFixed(1)}`;
    }

    /**
     * 设置相机预设视角
     * @param {'front'|'back'|'left'|'right'|'top'|'bottom'|'iso'} preset
     */
    setCameraPreset(preset) {
        // 取消登场动画
        if (this._introActive) this.skipIntroAnimation();
        // 计算模型中心与大小
        const box = new THREE.Box3();
        box.makeEmpty();
        this.modelRoot.traverse(obj => {
            if (obj.isMesh) box.expandByObject(obj);
        });
        const center = box.isEmpty() ? new THREE.Vector3(0, 0, 0) : box.getCenter(new THREE.Vector3());
        const size = box.isEmpty() ? new THREE.Vector3(1, 1, 1) : box.getSize(new THREE.Vector3());
        const maxDim = Math.max(size.x, size.y, size.z, 1);
        const dist = maxDim * 2.5;

        const offsets = {
            front: [0, 0, dist],
            back: [0, 0, -dist],
            left: [-dist, 0, 0],
            right: [dist, 0, 0],
            top: [0, dist, 0.001],
            bottom: [0, -dist, 0.001],
            iso: [dist * 0.6, dist * 0.8, dist]
        };
        const off = offsets[preset] || offsets.iso;
        this.camera.position.set(center.x + off[0], center.y + off[1], center.z + off[2]);
        this.controls.target.copy(center);
        this.controls.update();
    }

    /**
     * 设置网格可见性
     * @param {boolean} visible
     */
    setGridVisible(visible) {
        this.gridHelper.visible = visible;
    }

    /**
     * 设置线框模式
     * @param {boolean} on
     */
    setWireframe(on) {
        this.modelRoot.traverse(obj => {
            if (obj.isMesh && obj.material) {
                obj.material.wireframe = on;
            }
        });
    }

    /**
     * 设置 z-fighting 微调偏移（inflate bias）
     * 需要重新构建模型才能生效
     * @param {number} bias 0~0.1 之间的微小值
     */
    setInflateBias(bias) {
        this.inflateBias = Math.max(0, Math.min(0.1, bias));
    }

    /**
     * 设置地面（网格 Y 位置）可见性 alias
     */
    get gridVisible() {
        return this.gridHelper.visible;
    }
}

/**
 * SimpleOrbitControls — 简易轨道控制器
 * 不依赖 Three.js examples，自实现鼠标拖拽旋转 + 滚轮缩放 + 右键平移
 */
class SimpleOrbitControls {
    constructor(canvas, camera) {
        this.canvas = canvas;
        this.camera = camera;
        this.target = new THREE.Vector3(0, 0, 0);

        // 球面坐标
        this._spherical = new THREE.Spherical();
        this._spherical.setFromVector3(camera.position.clone().sub(this.target));

        // 拖拽状态
        this._isRotating = false;
        this._isPanning = false;
        this._lastX = 0;
        this._lastY = 0;

        this._bindEvents();
    }

    _bindEvents() {
        this.canvas.addEventListener('mousedown', this._onMouseDown.bind(this));
        window.addEventListener('mousemove', this._onMouseMove.bind(this));
        window.addEventListener('mouseup', this._onMouseUp.bind(this));
        this.canvas.addEventListener('wheel', this._onWheel.bind(this), { passive: false });
        this.canvas.addEventListener('contextmenu', (e) => e.preventDefault());
    }

    _onMouseDown(e) {
        this._lastX = e.clientX;
        this._lastY = e.clientY;
        if (e.button === 0) this._isRotating = true;
        else if (e.button === 2) this._isPanning = true;
    }

    _onMouseMove(e) {
        const dx = e.clientX - this._lastX;
        const dy = e.clientY - this._lastY;
        this._lastX = e.clientX;
        this._lastY = e.clientY;

        if (this._isRotating) {
            this._spherical.theta -= dx * 0.005;
            this._spherical.phi -= dy * 0.005;
            this._spherical.phi = Math.max(0.1, Math.min(Math.PI - 0.1, this._spherical.phi));
        } else if (this._isPanning) {
            const offset = new THREE.Vector3().copy(this.camera.position).sub(this.target);
            const panSpeed = offset.length() * 0.0015;
            const right = new THREE.Vector3().crossVectors(this.camera.up, offset).normalize();
            const up = new THREE.Vector3().crossVectors(offset, right).normalize();
            this.target.addScaledVector(right, -dx * panSpeed);
            this.target.addScaledVector(up, dy * panSpeed);
        }
    }

    _onMouseUp() {
        this._isRotating = false;
        this._isPanning = false;
    }

    _onWheel(e) {
        e.preventDefault();
        const scale = e.deltaY > 0 ? 1.1 : 0.9;
        this._spherical.radius = Math.max(1, Math.min(500, this._spherical.radius * scale));
    }

    update() {
        // 应用球面坐标到相机
        const offset = new THREE.Vector3().setFromSpherical(this._spherical);
        this.camera.position.copy(this.target).add(offset);
        this.camera.lookAt(this.target);
    }
}
