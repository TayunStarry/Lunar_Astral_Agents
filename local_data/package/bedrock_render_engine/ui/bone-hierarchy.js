// ==== bone-hierarchy.js — 骨骼层级树面板 ====

/**
 * BoneHierarchyPanel — 骨骼层级树面板
 *
 * 功能：
 *   1. 树形展示骨骼层级（支持折叠/展开）
 *   2. 显示骨骼命名（从 .bbmodel outliner 正确读取）
 *   3. 高亮选中的骨骼（在 3D 场景中突出显示）
 *   4. 显示 cube 子节点数量
 *   5. 骨骼显隐控制（编辑动画组时，每行显示眼睛图标）
 *      - 三态循环：继承默认(暗淡) → 显式隐藏 → 显式可见 → 继承默认
 */
export class BoneHierarchyPanel {
    /**
     * @param {object} opts
     * @param {string} opts.containerId 面板容器元素 ID
     * @param {function} onSelectBone 选中骨骼时的回调 (bone) => void
     */
    constructor(opts, onSelectBone) {
        this.container = document.getElementById(opts.containerId);
        this.onSelectBone = onSelectBone || (() => {});
        this.outliner = null;       // Outliner 数据
        this.selectedUuid = null;   // 当前选中的骨骼 uuid
        this.collapsedSet = new Set(); // 折叠的骨骼 uuid 集合

        /** @type {import('../core/anim-group-runtime.js').AnimGroup|null} 当前编辑的动画组 */
        this.editingGroup = null;
        /** @type {function|null} 骨骼显隐切换回调 (boneName) => void */
        this.onToggleBoneVisibility = null;

        this._build();
    }

    /**
     * 构建面板基础结构
     */
    _build() {
        if (!this.container) return;
        this.container.innerHTML = `
            <div class="bone-hierarchy-panel">
                <div class="bone-hierarchy-header">
                    <span class="bone-hierarchy-title">
                        <i class="fas fa-sitemap"></i> 骨骼层级
                    </span>
                    <div class="bone-hierarchy-actions">
                        <button class="btn-bone-action" id="btn-expand-all" title="全部展开">
                            <i class="fas fa-expand-arrows-alt"></i>
                        </button>
                        <button class="btn-bone-action" id="btn-collapse-all" title="全部折叠">
                            <i class="fas fa-compress-arrows-alt"></i>
                        </button>
                    </div>
                </div>
                <div class="bone-visibility-context" id="bone-vis-context" style="display:none;">
                    <i class="fas fa-eye"></i>
                    <span>显隐编辑: <strong id="bone-vis-group-name">-</strong></span>
                    <small>点击眼睛切换（继承→隐藏→显示）</small>
                </div>
                <div class="bone-hierarchy-tree" id="bone-tree-root">
                    <div class="bone-empty-state">
                        <i class="fas fa-cube"></i>
                        <span>加载模型后显示骨骼层级</span>
                    </div>
                </div>
                <div class="bone-hierarchy-info" id="bone-info-panel" style="display:none;">
                    <div class="bone-info-row">
                        <span class="bone-info-label">名称</span>
                        <span class="bone-info-value" id="bone-info-name">-</span>
                    </div>
                    <div class="bone-info-row">
                        <span class="bone-info-label">Pivot</span>
                        <span class="bone-info-value" id="bone-info-origin">-</span>
                    </div>
                    <div class="bone-info-row">
                        <span class="bone-info-label">旋转</span>
                        <span class="bone-info-value" id="bone-info-rotation">-</span>
                    </div>
                    <div class="bone-info-row">
                        <span class="bone-info-label">Cube</span>
                        <span class="bone-info-value" id="bone-info-cubes">-</span>
                    </div>
                    <div class="bone-info-row">
                        <span class="bone-info-label">子骨骼</span>
                        <span class="bone-info-value" id="bone-info-children">-</span>
                    </div>
                </div>
            </div>
        `;

        // 绑定按钮事件
        const btnExpand = this.container.querySelector('#btn-expand-all');
        const btnCollapse = this.container.querySelector('#btn-collapse-all');
        if (btnExpand) btnExpand.addEventListener('click', () => this._expandAll());
        if (btnCollapse) btnCollapse.addEventListener('click', () => this._collapseAll());
    }

    /**
     * 进入动画组显隐编辑模式
     * @param {import('../core/anim-group-runtime.js').AnimGroup} group
     * @param {function} onToggleBoneVisibility (boneName) => void
     */
    setEditingGroup(group, onToggleBoneVisibility) {
        this.editingGroup = group;
        this.onToggleBoneVisibility = onToggleBoneVisibility;
        const ctx = this.container?.querySelector('#bone-vis-context');
        if (ctx) {
            ctx.style.display = group ? '' : 'none';
            const nameEl = this.container.querySelector('#bone-vis-group-name');
            if (nameEl && group) nameEl.textContent = group.name;
        }
        this._render();
    }

    /**
     * 退出显隐编辑模式
     */
    clearEditingGroup() {
        this.editingGroup = null;
        this.onToggleBoneVisibility = null;
        const ctx = this.container?.querySelector('#bone-vis-context');
        if (ctx) ctx.style.display = 'none';
        this._render();
    }

    /**
     * 加载 Outliner 数据，渲染骨骼树
     * 默认全折叠（仅根骨骼可见，子骨骼需点击展开）
     * @param {import('../core/outliner.js').Outliner} outliner
     */
    loadOutliner(outliner) {
        this.outliner = outliner;
        this.selectedUuid = null;
        this.collapsedSet.clear();
        // 默认全折叠：所有有子骨骼的骨骼都加入 collapsedSet
        this._collapseAll();
        this._render();
    }

    /**
     * 渲染骨骼树
     */
    _render() {
        const treeRoot = this.container.querySelector('#bone-tree-root');
        if (!treeRoot) return;

        if (!this.outliner || this.outliner.roots.length === 0) {
            treeRoot.innerHTML = `
                <div class="bone-empty-state">
                    <i class="fas fa-cube"></i>
                    <span>加载模型后显示骨骼层级</span>
                </div>
            `;
            return;
        }

        treeRoot.innerHTML = '';
        for (const rootBone of this.outliner.roots) {
            const node = this._renderBoneNode(rootBone, 0);
            if (node) treeRoot.appendChild(node);
        }
    }

    /**
     * 递归渲染骨骼节点
     * @param {import('../core/outliner.js').Bone} bone
     * @param {number} depth 层级深度
     * @returns {HTMLElement}
     */
    _renderBoneNode(bone, depth) {
        const hasChildren = bone.children.some(c => c.constructor.name === 'Bone');
        const hasCubes = bone.cubes.length > 0;
        const isCollapsed = this.collapsedSet.has(bone.uuid);
        const isSelected = this.selectedUuid === bone.uuid;

        // 显隐状态计算（三态：继承 / 显式可见 / 显式隐藏）
        const hasOverride = this.editingGroup && (bone.name in this.editingGroup.boneVisibility);
        const isExplicitlyHidden = hasOverride && this.editingGroup.boneVisibility[bone.name] === false;
        const isExplicitlyVisible = hasOverride && this.editingGroup.boneVisibility[bone.name] === true;

        const node = document.createElement('div');
        node.className = 'bone-node';
        node.dataset.uuid = bone.uuid;

        const row = document.createElement('div');
        row.className = 'bone-row' + (isSelected ? ' selected' : '');
        if (isExplicitlyHidden) row.classList.add('bone-hidden');
        row.style.paddingLeft = `${depth * 18 + 8}px`;
        row.dataset.uuid = bone.uuid;

        // 折叠/展开箭头
        const arrow = document.createElement('span');
        arrow.className = 'bone-arrow' + (hasChildren ? '' : ' invisible');
        if (hasChildren) {
            arrow.innerHTML = isCollapsed
                ? '<i class="fas fa-caret-right"></i>'
                : '<i class="fas fa-caret-down"></i>';
            arrow.addEventListener('click', (e) => {
                e.stopPropagation();
                this._toggleCollapse(bone.uuid);
            });
        } else {
            arrow.innerHTML = '<i class="fas fa-circle" style="font-size:4px;opacity:0.3;"></i>';
        }
        row.appendChild(arrow);

        // 骨骼图标
        const icon = document.createElement('span');
        icon.className = 'bone-icon';
        icon.innerHTML = '<i class="fas fa-bone"></i>';
        row.appendChild(icon);

        // 骨骼名称
        const name = document.createElement('span');
        name.className = 'bone-name';
        name.textContent = bone.name;
        row.appendChild(name);

        // cube 数量徽章
        if (hasCubes) {
            const badge = document.createElement('span');
            badge.className = 'bone-cube-badge';
            badge.textContent = bone.cubes.length;
            badge.title = `${bone.cubes.length} 个立方体`;
            row.appendChild(badge);
        }

        // 显隐切换眼睛（仅编辑模式显示，三态：继承→隐藏→显示→继承）
        if (this.editingGroup) {
            const eye = document.createElement('button');
            eye.className = 'bone-visibility-toggle';
            if (isExplicitlyVisible) {
                eye.classList.add('explicit-visible');
                eye.innerHTML = '<i class="fas fa-eye"></i>';
                eye.title = '显式可见（点击移除覆盖）';
            } else if (isExplicitlyHidden) {
                eye.classList.add('explicit-hidden');
                eye.innerHTML = '<i class="fas fa-eye-slash"></i>';
                eye.title = '显式隐藏（点击设为可见）';
            } else {
                eye.classList.add('inheriting');
                eye.innerHTML = '<i class="fas fa-eye"></i>';
                eye.title = '继承默认（点击隐藏）';
            }
            eye.addEventListener('click', (e) => {
                e.stopPropagation();
                this.onToggleBoneVisibility?.(bone.name);
            });
            row.appendChild(eye);
        }

        // 点击选中
        row.addEventListener('click', () => {
            this._selectBone(bone.uuid);
        });

        node.appendChild(row);

        // 子骨骼（折叠时不渲染）
        if (hasChildren && !isCollapsed) {
            const childContainer = document.createElement('div');
            childContainer.className = 'bone-children';
            for (const child of bone.children) {
                if (child.constructor.name === 'Bone') {
                    const childNode = this._renderBoneNode(child, depth + 1);
                    if (childNode) childContainer.appendChild(childNode);
                }
            }
            node.appendChild(childContainer);
        }

        return node;
    }

    /**
     * 切换折叠/展开
     * @param {string} uuid
     */
    _toggleCollapse(uuid) {
        if (this.collapsedSet.has(uuid)) {
            this.collapsedSet.delete(uuid);
        } else {
            this.collapsedSet.add(uuid);
        }
        this._render();
    }

    /**
     * 全部展开
     */
    _expandAll() {
        this.collapsedSet.clear();
        this._render();
    }

    /**
     * 全部折叠
     */
    _collapseAll() {
        if (!this.outliner) return;
        this.outliner.traverseBones(bone => {
            const hasChildren = bone.children.some(c => c.constructor.name === 'Bone');
            if (hasChildren) this.collapsedSet.add(bone.uuid);
        });
        this._render();
    }

    /**
     * 选中骨骼
     * @param {string} uuid
     */
    _selectBone(uuid) {
        this.selectedUuid = uuid;
        this._render();

        // 更新信息面板
        if (this.outliner) {
            const bone = this.outliner.index.get(uuid);
            if (bone) {
                this._showBoneInfo(bone);
                this.onSelectBone(bone);
            }
        }
    }

    /**
     * 显示骨骼详细信息
     * @param {import('../core/outliner.js').Bone} bone
     */
    _showBoneInfo(bone) {
        const panel = this.container.querySelector('#bone-info-panel');
        if (!panel) return;
        panel.style.display = 'block';

        this.container.querySelector('#bone-info-name').textContent = bone.name;
        this.container.querySelector('#bone-info-origin').textContent =
            `[${bone.origin[0].toFixed(1)}, ${bone.origin[1].toFixed(1)}, ${bone.origin[2].toFixed(1)}]`;
        this.container.querySelector('#bone-info-rotation').textContent =
            `[${bone.rotation[0].toFixed(1)}°, ${bone.rotation[1].toFixed(1)}°, ${bone.rotation[2].toFixed(1)}°]`;
        this.container.querySelector('#bone-info-cubes').textContent = bone.cubes.length;

        const childBones = bone.children.filter(c => c.constructor.name === 'Bone').length;
        this.container.querySelector('#bone-info-children').textContent = childBones;
    }

    /**
     * 清除选中状态
     */
    clearSelection() {
        this.selectedUuid = null;
        this._render();
        const panel = this.container.querySelector('#bone-info-panel');
        if (panel) panel.style.display = 'none';
    }

    /**
     * 获取当前选中的骨骼
     * @returns {import('../core/outliner.js').Bone|null}
     */
    getSelectedBone() {
        if (!this.outliner || !this.selectedUuid) return null;
        return this.outliner.index.get(this.selectedUuid) || null;
    }
}
