// ==== anim-group-manager.js — 动画组管理面板 ====
//
// 功能：
//   - 动画组列表（默认组 + 自定义组）
//   - 创建/删除/编辑动画组
//   - 气泡选择界面：从可用动画列表添加/移除动画到组
//   - 组命名、过渡参数、循环模式配置
//   - 骨骼显隐控制（每组独立设定）
//   - 点击自定义组激活（互斥），再点击停用
//   - 导入/导出配置 JSON

import { AnimGroup, AnimGroupRuntime } from '../core/anim-group-runtime.js';
import { AnimationClassifier } from '../core/animation-classifier.js';

/**
 * 动画组管理面板
 *
 * 依赖注入：
 *   deps = {
 *     runtime: AnimGroupRuntime,
 *     availableAnimations: Map<string, Animation>,  // 可用动画表
 *     availableBones: Array<Bone>,                   // 可用骨骼列表
 *     onToast: (msg, type) => void,
 *     onStatusUpdate: () => void,                    // 状态刷新回调
 *     onEditGroup: (group|null) => void              // 编辑组开关回调（通知骨骼层级面板进入/退出显隐编辑模式）
 *   }
 */
export class AnimGroupManager {
    constructor(deps) {
        this.runtime = deps.runtime;
        this.availableAnimations = deps.availableAnimations || new Map();
        this.availableBones = deps.availableBones || [];
        this.onToast = deps.onToast || (() => {});
        this.onStatusUpdate = deps.onStatusUpdate || (() => {});
        this.onEditGroup = deps.onEditGroup || (() => {});

        /** @type {string|null} 当前编辑的组名 */
        this.editingGroup = null;
        /** @type {string|null} 当前选中的组名（仅用于列表高亮，不激活） */
        this.selectedGroup = null;

        this._build();
        this._bindEvents();
    }

    /**
     * 更新可用动画表（过滤掉特殊动画，仅显示普通动画供选择）
     */
    setAvailableAnimations(animMap) {
        // 过滤掉特殊动画（.blink/.move/.fast_move），仅保留普通动画
        this.availableAnimations = AnimationClassifier.filterDisplayAnimations(animMap);
    }

    /**
     * 更新可用骨骼列表
     */
    setAvailableBones(bones) {
        this.availableBones = bones;
    }

    /**
     * 构建面板 DOM
     * @private
     */
    _build() {
        const container = document.getElementById('anim-group-container');
        if (!container) return;
        container.innerHTML = `
            <div class="agm-toolbar">
                <button class="btn-glass btn-glass-primary agm-btn-add" title="新建动画组">
                    <i class="fas fa-plus"></i> 新建组
                </button>
                <button class="btn-glass agm-btn-import" title="导入配置">
                    <i class="fas fa-file-import"></i>
                </button>
                <button class="btn-glass agm-btn-export" title="导出配置到本地文件">
                    <i class="fas fa-file-export"></i>
                </button>
                <button class="btn-glass agm-btn-save" title="保存配置（覆写 model/anim_group_config.json）" style="margin-left:auto">
                    <i class="fas fa-save"></i> 保存
                </button>
            </div>
            <div id="agm-group-list" class="agm-group-list"></div>
        `;
    }

    /**
     * 绑定事件
     * @private
     */
    _bindEvents() {
        const container = document.getElementById('anim-group-container');
        if (!container) return;

        container.querySelector('.agm-btn-add').addEventListener('click', () => this._createNewGroup());
        container.querySelector('.agm-btn-import').addEventListener('click', () => this._importConfig());
        container.querySelector('.agm-btn-export').addEventListener('click', () => this._exportConfig());
        container.querySelector('.agm-btn-save').addEventListener('click', () => this._saveConfig());
    }

    /**
     * 刷新整个面板
     */
    refresh() {
        this._renderGroupList();
        if (this.editingGroup) {
            this._renderEditor(this.editingGroup);
        }
    }

    /**
     * 轻量级进度更新（仅更新进度条和当前动画名，不重建 DOM）
     * 供高频状态栏刷新调用
     */
    updateProgress() {
        const list = document.getElementById('agm-group-list');
        if (!list) return;
        // 优先更新激活的自定义组，否则更新默认组
        let activeItem = list.querySelector('.agm-group-item.active');
        if (!activeItem) activeItem = list.querySelector('.agm-group-item.default');
        if (!activeItem) return;
        const progressFill = activeItem.querySelector('.agm-progress-fill');
        const currentAnim = activeItem.querySelector('.agm-current-anim');
        if (progressFill && this.runtime.playing) {
            progressFill.style.width = (this.runtime.progress * 100).toFixed(1) + '%';
        }
        if (currentAnim) {
            currentAnim.textContent = this.runtime.currentAnimationShortName || '';
        }
    }

    /**
     * 渲染动画组列表
     * @private
     */
    _renderGroupList() {
        const list = document.getElementById('agm-group-list');
        if (!list) return;
        list.innerHTML = '';

        const groups = Array.from(this.runtime.groups.values());
        if (groups.length === 0) {
            list.innerHTML = `
                <div class="empty-state">
                    <i class="fas fa-layer-group"></i>
                    <p>无动画组</p>
                    <small>点击"新建组"创建</small>
                </div>`;
            return;
        }

        for (const group of groups) {
            const item = document.createElement('div');
            item.className = 'agm-group-item';
            if (this.runtime.activeCustomGroup === group) item.classList.add('active');
            if (this.runtime.defaultGroup === group) item.classList.add('default');
            if (this.selectedGroup === group.name) item.classList.add('selected');

            const animCount = group.animations.length;
            const isPlaying = this.runtime.activeCustomGroup === group || (this.runtime.defaultGroup === group && !this.runtime.activeCustomGroup);

            item.innerHTML = `
                <div class="agm-group-header">
                    <div class="agm-group-name">
                        ${group.isDefault ? '<i class="fas fa-home" style="color:var(--success)" title="默认组"></i>' : '<i class="fas fa-layer-group"></i>'}
                        <span>${this._escape(group.name)}</span>
                    </div>
                    <div class="agm-group-actions">
                        <button class="btn-bone-action agm-btn-play" title="${group.isDefault ? '默认组（永久）' : '激活/停用'}">
                            <i class="fas ${isPlaying && !group.isDefault ? 'fa-stop' : 'fa-play'}"></i>
                        </button>
                        <button class="btn-bone-action agm-btn-edit" title="编辑">
                            <i class="fas fa-edit"></i>
                        </button>
                        ${!group.isDefault ? '<button class="btn-bone-action agm-btn-del" title="删除"><i class="fas fa-trash"></i></button>' : ''}
                    </div>
                </div>
                <div class="agm-group-info">
                    <span class="agm-anim-count">${animCount} 个动画</span>
                    <span class="agm-loop-mode">${group.loopMode}</span>
                    <span class="agm-transition">${group.transitionDuration}s</span>
                </div>
                ${isPlaying && this.runtime.playing ? `
                <div class="agm-group-progress">
                    <div class="agm-progress-bar">
                        <div class="agm-progress-fill" style="width:${(this.runtime.progress * 100).toFixed(1)}%"></div>
                    </div>
                    <span class="agm-current-anim">${this._escape(this.runtime.currentAnimationShortName)}</span>
                </div>` : ''}
            `;

            // 播放/停用按钮
            item.querySelector('.agm-btn-play').addEventListener('click', (e) => {
                e.stopPropagation();
                if (group.isDefault) {
                    this.onToast('默认组始终生效', 'warning');
                    return;
                }
                if (this.runtime.activeCustomGroup === group) {
                    this.runtime.deactivateCustomGroup();
                    this.onToast(`停用: ${group.name}`, 'success');
                } else {
                    this.runtime.activateGroup(group.name);
                    this.onToast(`激活: ${group.name}`, 'success');
                }
                this.refresh();
                this.onStatusUpdate();
            });

            // 编辑按钮
            item.querySelector('.agm-btn-edit').addEventListener('click', (e) => {
                e.stopPropagation();
                this._toggleEditor(group.name);
            });

            // 删除按钮
            const delBtn = item.querySelector('.agm-btn-del');
            if (delBtn) {
                delBtn.addEventListener('click', (e) => {
                    e.stopPropagation();
                    if (confirm(`删除动画组 "${group.name}"？`)) {
                        this.runtime.removeGroup(group.name);
                        if (this.editingGroup === group.name) {
                            this.editingGroup = null;
                            const editor = document.getElementById('agm-editor');
                            if (editor) editor.style.display = 'none';
                            this._switchLeftPanelTab('hierarchy');
                            this._setEditorTabVisible(false); // 隐藏"动画组编辑"tab
                            this.onEditGroup(null); // 通知骨骼层级面板退出显隐编辑
                        }
                        this.refresh();
                        this.onToast(`已删除: ${group.name}`, 'success');
                    }
                });
            }

            list.appendChild(item);
        }
    }

    /**
     * 创建新动画组
     * @private
     */
    _createNewGroup() {
        const name = prompt('输入动画组名称：', `自定义组${this.runtime.groups.size}`);
        if (!name) return;
        if (this.runtime.groups.has(name)) {
            this.onToast(`组名已存在: ${name}`, 'error');
            return;
        }
        const g = new AnimGroup(name);
        g.loopMode = 'repeat';
        g.transitionDuration = 0.3;
        g.visibilityDelay = 0;
        this.runtime.addGroup(g);
        this.refresh();
        this.onToast(`已创建: ${name}`, 'success');
    }

    /**
     * 切换编辑器显示
     * 打开时：显示"动画组编辑"tab 并切换过去，确保左侧面板可见
     * 关闭时：切回"骨骼层级"tab 并隐藏"动画组编辑"tab
     * @private
     */
    _toggleEditor(groupName) {
        if (this.editingGroup === groupName) {
            // 关闭编辑器
            this.editingGroup = null;
            const editor = document.getElementById('agm-editor');
            if (editor) editor.style.display = 'none';
            this._switchLeftPanelTab('hierarchy'); // 切回骨骼层级
            this._setEditorTabVisible(false);     // 隐藏"动画组编辑"tab
            this.onEditGroup(null); // 通知骨骼层级面板退出显隐编辑
        } else {
            this.editingGroup = groupName;
            this._renderEditor(groupName);
            const editor = document.getElementById('agm-editor');
            if (editor) editor.style.display = '';
            this._setEditorTabVisible(true);      // 显示"动画组编辑"tab
            this._switchLeftPanelTab('agm-editor'); // 切到动画组编辑
            // 通知骨骼层级面板进入显隐编辑模式
            const group = this.runtime.groups.get(groupName);
            this.onEditGroup(group);
        }
        this._renderGroupList();
    }

    /**
     * 显示/隐藏"动画组编辑"tab
     * @param {boolean} visible
     * @private
     */
    _setEditorTabVisible(visible) {
        const tab = document.querySelector('#left-panel .panel-tab[data-tab="agm-editor"]');
        if (tab) tab.style.display = visible ? '' : 'none';
    }

    /**
     * 切换左侧面板的 tab，并确保左侧面板可见
     * @param {string} tabName 'hierarchy' 或 'agm-editor'
     * @private
     */
    _switchLeftPanelTab(tabName) {
        const leftPanel = document.getElementById('left-panel');
        if (!leftPanel) return;
        // 确保左侧面板可见（如果被隐藏则显示）
        if (leftPanel.style.display === 'none') {
            leftPanel.style.display = '';
            // 通知 App 同步面板状态
            const app = window.__bedrockRenderEngine;
            if (app) {
                app._panelsVisible = true;
                const btn = document.getElementById('btn-toggle-panels');
                if (btn) btn.style.opacity = '1';
            }
        }
        // 切换 tab（仅对可见的 tab 生效）
        leftPanel.querySelectorAll('.panel-tab').forEach(t => t.classList.remove('active'));
        leftPanel.querySelectorAll('.panel-body').forEach(b => b.style.display = 'none');
        const tab = leftPanel.querySelector(`.panel-tab[data-tab="${tabName}"]`);
        const body = leftPanel.querySelector(`#tab-${tabName}`);
        if (tab) tab.classList.add('active');
        if (body) body.style.display = '';
    }

    /**
     * 渲染编辑器
     * @private
     */
    _renderEditor(groupName) {
        const group = this.runtime.groups.get(groupName);
        if (!group) return;
        const editor = document.getElementById('agm-editor');
        if (!editor) return;

        // 渲染动画选择气泡
        const animBubbles = Array.from(this.availableAnimations.keys()).map(fullName => {
            const shortName = AnimGroupRuntime.shortName(fullName);
            const selected = group.animations.includes(fullName);
            return `<button class="agm-bubble ${selected ? 'selected' : ''}" data-anim="${this._escape(fullName)}">
                ${this._escape(shortName)}
            </button>`;
        }).join('');

        // 骨骼显隐覆盖数量
        const boneOverrideCount = Object.keys(group.boneVisibility).length;

        editor.innerHTML = `
            <div class="agm-editor-header">
                <i class="fas fa-edit"></i>
                <span>编辑: ${this._escape(group.name)}</span>
                <button class="btn-bone-action agm-btn-close-editor" title="关闭">
                    <i class="fas fa-times"></i>
                </button>
            </div>
            <div class="agm-editor-section">
                <label class="agm-label">循环模式</label>
                <select class="glass-select agm-loop-select" style="width:100%;height:30px">
                    <option value="repeat" ${group.loopMode === 'repeat' ? 'selected' : ''}>repeat（重复播放）</option>
                    <option value="return" ${group.loopMode === 'return' ? 'selected' : ''}>return（返回初始）</option>
                    <option value="hold" ${group.loopMode === 'hold' ? 'selected' : ''}>hold（保持最后帧）</option>
                </select>
            </div>
            <div class="agm-editor-section">
                <label class="agm-label">过渡时长: <span class="agm-trans-val">${group.transitionDuration.toFixed(2)}s</span></label>
                <input type="range" class="agm-trans-slider" min="0" max="2" step="0.05" value="${group.transitionDuration}" style="width:100%">
            </div>
            <div class="agm-editor-section">
                <label class="agm-label">显隐延迟: <span class="agm-vis-delay-val">${group.visibilityDelay.toFixed(2)}s</span></label>
                <input type="range" class="agm-vis-delay-slider" min="0" max="3" step="0.05" value="${group.visibilityDelay}" style="width:100%">
                <small>组激活后等待此时长再应用骨骼显隐设置</small>
            </div>
            ${!group.isDefault ? `
            <div class="agm-editor-section">
                <label class="agm-label">
                    <input type="checkbox" class="agm-set-default" ${group.isDefault ? 'checked' : ''}> 设为默认组
                </label>
            </div>` : ''}
            <div class="agm-editor-section">
                <label class="agm-label">动画序列（点击添加/移除）</label>
                <div class="agm-bubble-list">${animBubbles || '<small>无可用动画</small>'}</div>
                ${group.animations.length > 0 ? `
                <div class="agm-sequence">
                    <small>播放顺序：</small>
                    <ol class="agm-seq-list">
                        ${group.animations.map((name, i) => `
                            <li>
                                <span>${this._escape(AnimGroupRuntime.shortName(name))}</span>
                                <button class="btn-bone-action agm-seq-up" data-idx="${i}" ${i === 0 ? 'disabled' : ''} title="上移"><i class="fas fa-chevron-up"></i></button>
                                <button class="btn-bone-action agm-seq-down" data-idx="${i}" ${i === group.animations.length - 1 ? 'disabled' : ''} title="下移"><i class="fas fa-chevron-down"></i></button>
                                <button class="btn-bone-action agm-seq-del" data-idx="${i}" title="移除"><i class="fas fa-times"></i></button>
                            </li>
                        `).join('')}
                    </ol>
                </div>` : ''}
            </div>
            <div class="agm-editor-section">
                <div class="agm-bone-hint">
                    <i class="fas fa-info-circle"></i>
                    <span>骨骼显隐控制请切换到<strong>「骨骼层级」</strong>标签页</span>
                    <small>当前覆盖: ${boneOverrideCount} 个骨骼</small>
                </div>
            </div>
        `;

        this._bindEditorEvents(group);
    }

    /**
     * 绑定编辑器事件
     * @private
     */
    _bindEditorEvents(group) {
        const editor = document.getElementById('agm-editor');

        // 关闭编辑器
        editor.querySelector('.agm-btn-close-editor').addEventListener('click', () => {
            this.editingGroup = null;
            editor.style.display = 'none';
            this._switchLeftPanelTab('hierarchy'); // 切回骨骼层级
            this._setEditorTabVisible(false);      // 隐藏"动画组编辑"tab
            this.onEditGroup(null); // 通知骨骼层级面板退出显隐编辑
            this._renderGroupList();
        });

        // 循环模式
        editor.querySelector('.agm-loop-select').addEventListener('change', (e) => {
            group.loopMode = e.target.value;
            this.refresh();
        });

        // 过渡时长
        const transSlider = editor.querySelector('.agm-trans-slider');
        transSlider.addEventListener('input', (e) => {
            group.transitionDuration = parseFloat(e.target.value);
            editor.querySelector('.agm-trans-val').textContent = group.transitionDuration.toFixed(2) + 's';
        });

        // 显隐延迟
        const visDelaySlider = editor.querySelector('.agm-vis-delay-slider');
        visDelaySlider.addEventListener('input', (e) => {
            group.visibilityDelay = parseFloat(e.target.value);
            editor.querySelector('.agm-vis-delay-val').textContent = group.visibilityDelay.toFixed(2) + 's';
        });

        // 设为默认组
        const setDefault = editor.querySelector('.agm-set-default');
        if (setDefault) {
            setDefault.addEventListener('change', (e) => {
                if (e.target.checked) {
                    this.runtime.setDefaultGroup(group.name);
                    this.refresh();
                }
            });
        }

        // 动画气泡选择
        editor.querySelectorAll('.agm-bubble').forEach(bubble => {
            bubble.addEventListener('click', () => {
                const fullName = bubble.dataset.anim;
                const idx = group.animations.indexOf(fullName);
                if (idx >= 0) {
                    group.animations.splice(idx, 1);
                } else {
                    group.animations.push(fullName);
                }
                this._renderEditor(group.name);
            });
        });

        // 序列调整
        editor.querySelectorAll('.agm-seq-up').forEach(btn => {
            btn.addEventListener('click', () => {
                const i = parseInt(btn.dataset.idx);
                if (i > 0) {
                    [group.animations[i], group.animations[i - 1]] = [group.animations[i - 1], group.animations[i]];
                    this._renderEditor(group.name);
                }
            });
        });
        editor.querySelectorAll('.agm-seq-down').forEach(btn => {
            btn.addEventListener('click', () => {
                const i = parseInt(btn.dataset.idx);
                if (i < group.animations.length - 1) {
                    [group.animations[i], group.animations[i + 1]] = [group.animations[i + 1], group.animations[i]];
                    this._renderEditor(group.name);
                }
            });
        });
        editor.querySelectorAll('.agm-seq-del').forEach(btn => {
            btn.addEventListener('click', () => {
                const i = parseInt(btn.dataset.idx);
                group.animations.splice(i, 1);
                this._renderEditor(group.name);
            });
        });
    }

    /**
     * 导出配置到本地文件下载
     * @private
     */
    _exportConfig() {
        const config = this.runtime.exportConfig();
        const json = JSON.stringify(config, null, 2);
        const blob = new Blob([json], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = 'anim_group_config.json';
        a.click();
        URL.revokeObjectURL(url);
        this.onToast('配置已导出', 'success');
    }

    /**
     * 保存配置到服务器（覆写 model/anim_group_config.json）
     * @private
     */
    async _saveConfig() {
        try {
            const config = this.runtime.exportConfig();
            const json = JSON.stringify(config, null, 2);
            const relativePath = 'package/bedrock_render_engine/model/anim_group_config.json';
            const encodedPath = btoa(unescape(encodeURIComponent(relativePath)));
            const resp = await fetch('/file/write', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-File-Name': encodedPath,
                    'X-Overwrite': 'true'
                },
                body: json
            });
            if (!resp.ok) {
                throw new Error(`服务器返回 ${resp.status}: ${resp.statusText}`);
            }
            const groupCount = config.animation_groups?.groups?.length || 0;
            this.onToast(`配置已保存（${groupCount} 个动画组）`, 'success');
        } catch (err) {
            console.error('[AnimGroupManager] 保存配置失败:', err);
            this.onToast(`保存失败: ${err.message}`, 'error');
        }
    }

    /**
     * 导入配置
     * @private
     */
    _importConfig() {
        const input = document.createElement('input');
        input.type = 'file';
        input.accept = '.json';
        input.addEventListener('change', async (e) => {
            const file = e.target.files[0];
            if (!file) return;
            try {
                const text = await file.text();
                const json = JSON.parse(text);
                this.runtime.importConfig(json, this.availableAnimations);
                this.refresh();
                this.onToast('配置已导入', 'success');
            } catch (err) {
                this.onToast('导入失败: ' + err.message, 'error');
            }
        });
        input.click();
    }

    /**
     * HTML 转义
     * @private
     */
    _escape(s) {
        if (!s) return '';
        return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
    }
}
