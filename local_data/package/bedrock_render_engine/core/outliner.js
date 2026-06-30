// ==== outliner.js — 骨骼层级数据结构 ====

/**
 * 骨骼（Bone）— 对应 .bbmodel outliner 中的 group 节点
 * 去 Vue 化的纯数据结构，参考 Blockbench js/outliner/types/group.js
 */
export class Bone {
    constructor(data) {
        this.uuid = data.uuid || crypto.randomUUID();
        this.name = data.name || '未命名骨骼';
        // pivot 点（Bedrock 的 origin 字段）
        this.origin = data.origin ? [...data.origin] : [0, 0, 0];
        // 默认旋转（度，XYZ 顺序）
        this.rotation = data.rotation ? [...data.rotation] : [0, 0,  0];
        this.export = data.export !== false;
        // 父骨骼引用（运行时填充）
        this.parent = null;
        // 子节点：Bone 或 CubeElement
        this.children = [];
        // Three.js Object3D 引用（运行时由 renderer 填充）
        this.sceneObject = null;
        // 该骨骼直接挂载的 cube 元素（运行时填充）
        this.cubes = [];
    }

    /**
     * 添加子骨骼
     * @param {Bone} bone
     */
    addChildBone(bone) {
        bone.parent = this;
        this.children.push(bone);
    }

    /**
     * 添加 cube 元素到该骨骼
     * @param {CubeElement} cube
     */
    addCube(cube) {
        cube.parent = this;
        this.cubes.push(cube);
    }

    /**
     * 递归遍历所有骨骼（包括自身）
     * @param {(bone: Bone) => void} callback
     */
    traverseBones(callback) {
        callback(this);
        for (const child of this.children) {
            if (child instanceof Bone) {
                child.traverseBones(callback);
            }
        }
    }

    /**
     * 递归遍历所有 cube
     * @param {(cube: CubeElement) => void} callback
     */
    traverseCubes(callback) {
        for (const cube of this.cubes) callback(cube);
        for (const child of this.children) {
            if (child instanceof Bone) {
                child.traverseCubes(callback);
            }
        }
    }
}

/**
 * Cube 元素 — 对应 .bbmodel elements 数组中的 cube 项
 * 参考 Blockbench js/outliner/types/cube.js，去 Vue
 */
export class CubeElement {
    constructor(data) {
        this.uuid = data.uuid || crypto.randomUUID();
        this.name = data.name || 'cube';
        // 立方体边界（Blockbench 内部坐标系，像素单位）
        this.from = data.from ? [...data.from] : [0, 0, 0];
        this.to = data.to ? [...data.to] : [0, 0, 0];
        // 局部 pivot（cube 自身的旋转中心），默认 [0,0,0]
        this.origin = data.origin ? [...data.origin] : [0, 0, 0];
        this.rotation = data.rotation ? [...data.rotation] : [0, 0, 0];
        // inflate 膨胀值：正值使 cube 向外膨胀，负值收缩（用于避免 z-fighting）
        this.inflate = data.inflate || 0;
        // per-face UV（box_uv=false 时使用）
        this.box_uv = data.box_uv || false;
        this.faces = data.faces || {};
        this.export = data.export !== false;
        // 父骨骼引用（运行时填充）
        this.parent = null;
        // Three.js 几何体引用（运行时由 renderer 填充）
        this.mesh = null;
    }

    /**
     * 计算立方体尺寸
     */
    get size() {
        return [
            Math.abs(this.to[0] - this.from[0]),
            Math.abs(this.to[1] - this.from[1]),
            Math.abs(this.to[2] - this.from[2])
        ];
    }

    /**
     * 计算 Bedrock 像素空间中心
     */
    get center() {
        return [
            (this.from[0] + this.to[0]) / 2,
            (this.from[1] + this.to[1]) / 2,
            (this.from[2] + this.to[2]) / 2
        ];
    }
}

/**
 * Outliner — 整棵骨骼树的根容器
 */
export class Outliner {
    constructor() {
        /** @type {Bone[]} */
        this.roots = [];
        /** @type {Map<string, Bone|CubeElement>} uuid → 节点 */
        this.index = new Map();
    }

    /**
     * 从 .bbmodel 的 outliner + elements + groups 构建 Outliner
     *
     * .bbmodel format_version 5.0 结构：
     *   - outliner：树结构，group 节点为 {uuid, isOpen, children}，cube 引用为字符串 uuid
     *   - groups：扁平数组，包含所有骨骼的元数据（name, origin, rotation, ...）
     *   - elements：扁平数组，包含所有 cube 的定义
     *
     * @param {Array} outlinerData .bbmodel.outliner 数组（树结构）
     * @param {Array} elementsData .bbmodel.elements 数组（cube 定义）
     * @param {Array} [groupsData] .bbmodel.groups 数组（骨骼元数据）
     * @returns {Outliner}
     */
    static fromBBModel(outlinerData, elementsData, groupsData = []) {
        const outliner = new Outliner();

        // 建立 uuid → CubeElement 索引
        const cubeByUuid = new Map();
        for (const el of elementsData) {
            const cube = new CubeElement(el);
            cubeByUuid.set(cube.uuid, cube);
            outliner.index.set(cube.uuid, cube);
        }

        // 建立 uuid → 骨骼元数据索引（name/origin/rotation 等存储在 groups 中）
        const groupByUuid = new Map();
        for (const g of groupsData) {
            if (g && g.uuid) groupByUuid.set(g.uuid, g);
        }

        // 递归构建骨骼树
        for (const node of outlinerData) {
            const bone = Outliner._buildBoneRecursive(node, cubeByUuid, groupByUuid, outliner.index);
            if (bone) outliner.roots.push(bone);
        }

        return outliner;
    }

    /**
     * 递归构建骨骼节点
     * @param {object|string} node outliner 中的节点（对象=骨骼，字符串=cube uuid）
     * @param {Map} cubeByUuid
     * @param {Map} groupByUuid 骨骼元数据查找表（来自 .bbmodel.groups）
     * @param {Map} index
     * @returns {Bone|CubeElement|null}
     */
    static _buildBoneRecursive(node, cubeByUuid, groupByUuid, index) {
        // 字符串 → cube 引用
        if (typeof node === 'string') {
            return cubeByUuid.get(node) || null;
        }

        // 对象 → 骨骼
        if (typeof node === 'object' && node.uuid) {
            // 合并 outliner 节点（含 uuid/isOpen/children）与 groups 元数据（含 name/origin/rotation）
            const groupMeta = groupByUuid.get(node.uuid) || {};
            const boneData = { ...groupMeta, ...node }; // node 的 children 覆盖 groupMeta 的 children（两者一致）
            const bone = new Bone(boneData);
            index.set(bone.uuid, bone);

            if (Array.isArray(node.children)) {
                for (const childNode of node.children) {
                    const child = Outliner._buildBoneRecursive(childNode, cubeByUuid, groupByUuid, index);
                    if (child instanceof Bone) {
                        bone.addChildBone(child);
                    } else if (child instanceof CubeElement) {
                        bone.addCube(child);
                    }
                }
            }
            return bone;
        }

        return null;
    }

    /**
     * 遍历所有骨骼
     * @param {(bone: Bone) => void} callback
     */
    traverseBones(callback) {
        for (const root of this.roots) {
            root.traverseBones(callback);
        }
    }

    /**
     * 遍历所有 cube
     * @param {(cube: CubeElement) => void} callback
     */
    traverseCubes(callback) {
        for (const root of this.roots) {
            root.traverseCubes(callback);
        }
    }

    /**
     * 统计骨骼总数
     */
    get boneCount() {
        let count = 0;
        this.traverseBones(() => count++);
        return count;
    }

    /**
     * 统计 cube 总数
     */
    get cubeCount() {
        let count = 0;
        this.traverseCubes(() => count++);
        return count;
    }
}
