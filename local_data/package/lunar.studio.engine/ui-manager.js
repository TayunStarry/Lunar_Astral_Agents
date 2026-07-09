import * as THREE from './three.module.js';
import { PRIMITIVES } from './primitives.js';
import { AssetManager } from './asset-manager.js';
import { TextureGenerator } from './texture-generator.js';

// ============ UI 管理器 ============
class UIManager {
    constructor(sm, cc, pm) {
        this.sm = sm;
        this.cc = cc;
        this.pm = pm;
        // 注入相机控制器引用，供物理操控模式获取视角方向与启动越肩视角
        this.pm.setCameraController(this.cc);
        this._toastTimer = null;
        this._panelsVisible = true;
        this._dirty = false;  // 自动保存脏标记

        // 每10秒检查一次，有变更则自动保存
        this._saveInterval = setInterval(() => {
            if (this._dirty) {
                console.log('[自动保存] 定时器触发, 检测到变更, 开始保存...');
                this._dirty = false;
                this._doAutoSave();
            }
        }, 10000);

        // 面板切换
        this.btnTogglePanels = document.getElementById('btn-toggle-panels');
        this.leftPanel = document.getElementById('left-panel');
        this.leftArea = document.getElementById('left-area');
        this.rightPanel = document.getElementById('right-panel');
        this.cameraPanel = document.getElementById('camera-panel');

        // 标签页
        this._tabElements = document.querySelectorAll('.panel-tab');
        this._tabBodies = {
            hierarchy: document.getElementById('tab-hierarchy'),
            settings: document.getElementById('tab-settings'),
            keyframes: document.getElementById('tab-keyframes'),
            inspector: document.getElementById('tab-inspector'),
            textures: document.getElementById('tab-textures'),
        };
        // 每个面板的标签组
        this._panelTabs = {
            'left-panel': ['hierarchy', 'settings', 'keyframes'],
            'right-panel': ['inspector', 'textures'],
        };

        // DOM 引用
        this.toolbar = document.getElementById('toolbar');
        this.hierarchyTree = document.getElementById('hierarchy-tree');
        this.inspectorBody = document.getElementById('inspector-body');
        this.importInput = document.getElementById('import-file-input');
        this.groupImportInput = document.getElementById('import-group-input');
        this.textureInput = document.getElementById('texture-file-input');
        this.texturePoolGrid = document.getElementById('texture-pool-grid');
        this.toast = document.getElementById('toast');
        this.statusFps = document.getElementById('status-fps');
        this.statusObjects = document.getElementById('status-objects');
        this.statusFaces = document.getElementById('status-faces');
        this.statusMode = document.getElementById('status-mode');
        this.btnTheme = document.getElementById('btn-theme');

        // 关键帧 DOM 引用
        this.btnKfAdd = document.getElementById('btn-kf-add');
        this.btnKfDelete = document.getElementById('btn-kf-delete');
        this.btnKfPlay = document.getElementById('btn-kf-play');
        this.btnKfStop = document.getElementById('btn-kf-stop');
        this.keyframeList = document.getElementById('keyframe-list');

        this.bindEvents();
        this.refresh();
    }

    bindEvents() {
        // 面板切换
        this.btnTogglePanels.addEventListener('click', () => this._togglePanels());

        // 标签页
        this._tabElements.forEach(tab => {
            tab.addEventListener('click', () => this._switchTab(tab.dataset.tab));
        });

        // 工具栏
        this.toolbar.addEventListener('click', e => {
            const btn = e.target.closest('[data-action]');
            if (!btn) return;
            this._handleAction(btn.dataset.action);
        });

        // 图元面板
        const primitivePanel = document.getElementById('primitive-panel');
        if (primitivePanel) {
            primitivePanel.addEventListener('click', e => {
                const btn = e.target.closest('[data-action]');
                if (!btn) return;
                this._handleAction(btn.dataset.action);
            });
        }

        // 导入方案
        this.importInput.addEventListener('change', e => {
            if (e.target.files.length > 0) { this._handleImport(e.target.files[0]); this.importInput.value = ''; }
        });

        // 导入组合体资产
        this.groupImportInput.addEventListener('change', async e => {
            if (e.target.files.length > 0) {
                try {
                    const data = await AssetManager.importGroupAsset(e.target.files[0]);
                    await this._loadGroupAsset(data);
                    this.showToast(`组合体 "${data.name}" 已导入`, 'success');
                } catch (err) { this.showToast('导入失败: ' + err.message, 'error'); }
                this.groupImportInput.value = '';
            }
        });

        // 导入纹理按钮
        const btnImportTex = document.getElementById('btn-import-texture');
        if (btnImportTex) btnImportTex.addEventListener('click', () => this.textureInput.click());

        // 导入纹理
        this.textureInput.addEventListener('change', e => {
            if (e.target.files.length > 0) { this._handleTextureImport(e.target.files); this.textureInput.value = ''; }
        });

        // 纹理池网格点击委托（添加按钮 + 删除按钮）
        this.texturePoolGrid.addEventListener('click', e => {
            if (e.target.closest('#texture-pool-add') || e.target.closest('.texture-pool-add')) {
                e.stopPropagation();
                this.textureInput.click();
                return;
            }
            const delBtn = e.target.closest('.tex-delete');
            if (delBtn) {
                const name = delBtn.dataset.name;
                this.sm.removeTexture(name);
                this.renderTexturePool();
                this.renderInspector();
                this.showToast(`纹理 "${name}" 已删除`, 'success');
            }
        });

        // 主题
        this.btnTheme.addEventListener('click', () => {
            document.body.classList.toggle('dark-mode');
            const icon = this.btnTheme.querySelector('i');
            icon.className = document.body.classList.contains('dark-mode') ? 'fas fa-sun' : 'fas fa-moon';
        });

        // 操作说明
        const btnHelp = document.getElementById('btn-help');
        if (btnHelp) btnHelp.addEventListener('click', () => this._showHelpModal());

        // 关键帧操作
        this.btnKfAdd.addEventListener('click', () => this._kfAdd());
        this.btnKfDelete.addEventListener('click', () => this._kfDelete());
        this.btnKfPlay.addEventListener('click', () => this._kfTogglePlay());
        this.btnKfStop.addEventListener('click', () => this._kfStop());

        // 画布点击选择（物理操控模式下禁止选中）
        const canvas = this.sm.canvas;
        canvas.addEventListener('click', e => {
            if (e.button !== 0) return;
            if (this.pm.controlState.active) return; // 操控模式下禁止鼠标选中
            this._pickObject(e.clientX, e.clientY, e);
        });

        // 场景设置绑定
        this._bindSettings();

        // 纹理生成器
        this._bindTextureGenerator();

        // 摄像机坐标
        document.getElementById('btn-cam-goto').addEventListener('click', () => this._gotoCamera());

        // 键盘快捷键
        document.addEventListener('keydown', e => {
            const sel = this.sm.selected;

            // Q/E 优先处理（即使在输入框聚焦时也生效）
            // Q: 打开物理属性配置模态框
            if (e.key === 'q' || e.key === 'Q') {
                if (!sel) { this.showToast('请先选中图元', 'info'); return; }
                this._showPhysicsModal(sel);
                return;
            }
            // E: 对整个场景施加/取消物理效果
            if (e.key === 'e' || e.key === 'E') {
                const wasControl = this.pm.controlState.active;
                const added = this.pm.toggleAll();
                this.showToast(added ? '物理效果已施加到全部图元' : '物理效果已取消', 'success');
                // 取消物理时 reset() 会自动 stopControl，需同步状态显示
                if (wasControl && !this.pm.controlState.active) this._syncControlStatus();
                return;
            }
            // Z: 切换碰撞体线框可视化
            if (e.key === 'z' || e.key === 'Z') {
                const visible = this.pm.toggleDebug();
                this.showToast(visible ? '碰撞体可视化已开启（青色线框）' : '碰撞体可视化已关闭', 'info');
                return;
            }
            // X: 切换物理操控模式（越肩视角 + WASD 施力 + 空格跳跃）
            if (e.key === 'x' || e.key === 'X') {
                if (this.pm.controlState.active) {
                    this.pm.stopControl();
                    this._syncControlStatus();
                    this.showToast('已退出物理操控模式，视角已恢复', 'info');
                } else {
                    if (!sel) { this.showToast('请先选中图元', 'info'); return; }
                    if (this.pm.bodies.size === 0) { this.showToast('请先按 E 启动物理模拟', 'info'); return; }
                    if (!this.pm.canControl(sel)) {
                        this.showToast('该图元不可操控（需为动态刚体，请检查是否锚定）', 'error'); return;
                    }
                    if (this.pm.startControl(sel)) {
                        // 取消所有选中状态（操控模式禁止选中）
                        this.sm.deselect();
                        this.refresh();
                        // 自动隐藏面板，确保操作界面简洁
                        if (this._panelsVisible) {
                            this._panelsVisible = false;
                            this.leftArea.classList.add('hidden');
                            this.rightPanel.classList.add('hidden');
                            this.cameraPanel.style.display = 'none';
                        }
                        this._syncControlStatus();
                        this.showToast('物理操控模式已开启：WASD 移动 · 空格跳跃 · 鼠标旋转视角 · X 退出', 'success');
                    }
                }
                return;
            }

            // 其他快捷键在输入框聚焦时不生效
            if (e.target.tagName === 'INPUT' || e.target.tagName === 'SELECT' || e.target.tagName === 'TEXTAREA') return;

            // 物理操控模式：WASD/Space 路由到物理管理器（基于视角方向施力）
            if (this.pm.controlState.active) {
                const code = e.code;
                if (code === 'KeyW' || code === 'KeyA' || code === 'KeyS' || code === 'KeyD' || code === 'Space') {
                    e.preventDefault();
                    this.pm.setControlKey(code, true);
                    return;
                }
            }

            // 删除
            if (e.key === 'Delete' || e.key === 'Backspace') {
                const wasControl = this.pm.controlState.active;
                if (sel) this.pm.removeBody(sel);
                this.sm.deleteSelected(); this.refresh(); this.showToast('已删除', 'success');
                if (wasControl && !this.pm.controlState.active) this._syncControlStatus();
                return;
            }
            // Ctrl+G 组合 / Ctrl+Shift+G 取消组合
            if ((e.ctrlKey || e.metaKey) && e.key === 'g' && !e.shiftKey) {
                e.preventDefault();
                const group = this.sm.groupSelected();
                if (group) { this.refresh(); this.showToast('已组合', 'success'); }
                else this.showToast('请至少选择2个对象（Ctrl+点击多选）', 'info');
                return;
            }
            if ((e.ctrlKey || e.metaKey) && e.key === 'G') {
                e.preventDefault();
                const children = this.sm.ungroupSelected();
                if (children) { this.refresh(); this.showToast('已取消组合', 'success'); }
                else this.showToast('当前选中对象不是组合体', 'info');
                return;
            }

            // 1/2/3/4: 切换操作模式（选中/未选中均可用）
            if (e.key === '1') {
                this._operationMode = 'translate';
                this._updateStatusMode('位移');
                this.showToast('操作模式: 位移', 'info');
                return;
            }
            if (e.key === '2') {
                this._operationMode = 'rotate';
                this._updateStatusMode('旋转');
                this.showToast('操作模式: 旋转', 'info');
                return;
            }
            if (e.key === '3') {
                this._operationMode = 'scale';
                this._updateStatusMode('缩放');
                this.showToast('操作模式: 缩放', 'info');
                return;
            }
            if (e.key === '4') {
                this._operationMode = 'sunDir';
                this._updateStatusMode('光照');
                this.showToast('操作模式: 光照方向调整', 'info');
                return;
            }

            // ==== 无选中时操作 ====
            if (!sel) {
                this.cc._cameraSpaceEnabled = true;
                return;
            }
            this.cc._cameraSpaceEnabled = false;

            // ==== 选中对象快捷键 ====
            if (!sel) return;

            // R: 按住环绕（在 keydown 中启动，keyup 中停止）
            if (e.key === 'r' || e.key === 'R') {
                e.preventDefault();
                if (!sel) return;
                const box = new THREE.Box3().setFromObject(sel);
                const center = new THREE.Vector3(); box.getCenter(center);
                this.cc.startOrbit(center);
                this.showToast(`环绕: ${sel.userData.name}`, 'success');
                return;
            }

            // 方向键/空格/Shift: 根据操作模式执行不同操作
            const step = 0.5;
            const mode = this._operationMode || 'translate';
            const angleStep = Math.PI / 18; // 10度

            if (mode === 'translate') {
                if (e.key === 'ArrowLeft') { sel.position.x -= step; this.refresh(); return; }
                if (e.key === 'ArrowRight') { sel.position.x += step; this.refresh(); return; }
                if (e.key === 'ArrowUp') { sel.position.z -= step; this.refresh(); return; }
                if (e.key === 'ArrowDown') { sel.position.z += step; this.refresh(); return; }
                if (e.key === ' ') { e.preventDefault(); sel.position.y += step; this.refresh(); return; }
                if (e.key === 'Shift') { sel.position.y -= step; this.refresh(); return; }
            } else if (mode === 'rotate') {
                if (e.key === 'ArrowLeft') { sel.rotation.y += angleStep; this.refresh(); return; }
                if (e.key === 'ArrowRight') { sel.rotation.y -= angleStep; this.refresh(); return; }
                if (e.key === 'ArrowUp') { sel.rotation.x += angleStep; this.refresh(); return; }
                if (e.key === 'ArrowDown') { sel.rotation.x -= angleStep; this.refresh(); return; }
                if (e.key === ' ') { e.preventDefault(); sel.rotation.z += angleStep; this.refresh(); return; }
                if (e.key === 'Shift') { sel.rotation.z -= angleStep; this.refresh(); return; }
            } else if (mode === 'scale') {
                const scaleStep = 0.1;
                if (e.key === 'ArrowLeft') { sel.scale.x = Math.max(0.1, sel.scale.x - scaleStep); this.refresh(); return; }
                if (e.key === 'ArrowRight') { sel.scale.x += scaleStep; this.refresh(); return; }
                if (e.key === 'ArrowUp') { sel.scale.z = Math.max(0.1, sel.scale.z - scaleStep); this.refresh(); return; }
                if (e.key === 'ArrowDown') { sel.scale.z += scaleStep; this.refresh(); return; }
                if (e.key === ' ') { e.preventDefault(); sel.scale.y += scaleStep; this.refresh(); return; }
                if (e.key === 'Shift') { sel.scale.y = Math.max(0.1, sel.scale.y - scaleStep); this.refresh(); return; }
            } else if (mode === 'sunDir') {
                const sun = this.sm.sunLight;
                if (e.key === 'ArrowLeft') { sun.position.x -= step; this.refresh(); return; }
                if (e.key === 'ArrowRight') { sun.position.x += step; this.refresh(); return; }
                if (e.key === 'ArrowUp') { sun.position.z -= step; this.refresh(); return; }
                if (e.key === 'ArrowDown') { sun.position.z += step; this.refresh(); return; }
                if (e.key === ' ') { e.preventDefault(); sun.position.y += step; this.refresh(); return; }
                if (e.key === 'Shift') { sun.position.y = Math.max(0.5, sun.position.y - step); this.refresh(); return; }
            }

            // ==== 操作模式切换 ====
        });

        // R 键松开时停止环绕；物理操控模式下释放 WASD/Space
        document.addEventListener('keyup', e => {
            if (e.key === 'r' || e.key === 'R') {
                this.cc.stopOrbit();
            }
            if (this.pm.controlState.active) {
                const code = e.code;
                if (code === 'KeyW' || code === 'KeyA' || code === 'KeyS' || code === 'KeyD' || code === 'Space') {
                    this.pm.setControlKey(code, false);
                }
            }
        });
    }

    /**
     * 同步物理操控模式的状态显示
     * 进入时显示"物理操控"并添加视觉反馈；退出时恢复原模式并移除反馈
     */
    _syncControlStatus() {
        if (this.pm.controlState.active) {
            this._updateStatusMode('物理');
        } else {
            const modeMap = { translate: '位移', rotate: '旋转', scale: '缩放', sunDir: '光照' };
            this._updateStatusMode(modeMap[this._operationMode] || '位移');
        }
    }

    _bindSettings() {
        const bind = (id, callback) => {
            const el = document.getElementById(id);
            if (!el) return;
            el.addEventListener('input', () => callback(el));
        };
        bind('sky-top', el => this.sm.setSkyboxColors(el.value, document.getElementById('sky-mid').value, document.getElementById('sky-bottom').value));
        bind('sky-mid', el => this.sm.setSkyboxColors(document.getElementById('sky-top').value, el.value, document.getElementById('sky-bottom').value));
        bind('sky-bottom', el => this.sm.setSkyboxColors(document.getElementById('sky-top').value, document.getElementById('sky-mid').value, el.value));
        bind('light-ambient', el => { this.sm.setAmbientIntensity(parseFloat(el.value)); el.nextElementSibling.textContent = parseFloat(el.value).toFixed(2); });
        bind('light-hemi', el => { this.sm.setHemiIntensity(parseFloat(el.value)); el.nextElementSibling.textContent = parseFloat(el.value).toFixed(2); });
        bind('light-sun', el => { this.sm.setSunIntensity(parseFloat(el.value)); el.nextElementSibling.textContent = parseFloat(el.value).toFixed(2); });
        bind('light-dir-x', () => this._updateSunDir());
        bind('light-dir-y', () => this._updateSunDir());
        bind('light-dir-z', () => this._updateSunDir());

        // 切换按钮（替代 checkbox）
        const bindToggle = (id, callback) => {
            const el = document.getElementById(id);
            if (!el) return;
            el.addEventListener('click', () => {
                el.classList.toggle('active');
                el.textContent = el.classList.contains('active') ? '开启' : '关闭';
                callback(el.classList.contains('active'));
                this._scheduleAutoSave();
            });
        };
        bindToggle('light-shadows', active => this.sm.setShadowsEnabled(active));
        bindToggle('ground-grid', active => this.sm.setGridVisible(active));
        bindToggle('ground-plane', active => this.sm.setGroundVisible(active));

        bind('ground-size', el => this.sm.setGroundSize(parseInt(el.value) || 20));
        bind('ground-color', el => this.sm.setGridColor(el.value));

        // 物理模拟参数
        const bindPhys = (id, setter) => {
            const el = document.getElementById(id);
            if (!el) return;
            el.addEventListener('input', () => {
                setter(parseFloat(el.value));
                this._scheduleAutoSave();
            });
        };
        bindPhys('phys-gravity', v => { if (this.pm) this.pm.gravity = v; });
        bindPhys('phys-ground-y', v => { if (this.pm) this.pm.groundY = v; });
        bindPhys('phys-mass-single', v => { if (this.pm) this.pm.massSingle = v; });
        bindPhys('phys-linear-damping', v => { if (this.pm) this.pm.linearDamping = v; });
        bindPhys('phys-angular-damping', v => { if (this.pm) this.pm.angularDamping = v; });
    }

    _updateSunDir() {
        const x = parseFloat(document.getElementById('light-dir-x').value) || 10;
        const y = parseFloat(document.getElementById('light-dir-y').value) || 15;
        const z = parseFloat(document.getElementById('light-dir-z').value) || 8;
        this.sm.setSunDirection(x, y, z);
    }

    _bindTextureGenerator() {
        const genColor = document.getElementById('gen-color');
        const genPattern = document.getElementById('gen-pattern');
        const btnPreview = document.getElementById('btn-gen-preview');
        const btnAdd = document.getElementById('btn-gen-add');
        const previewCanvas = document.getElementById('gen-preview-canvas');
        if (!genColor || !genPattern || !btnPreview || !btnAdd || !previewCanvas) return;

        let lastResult = null;

        const doPreview = () => {
            lastResult = TextureGenerator.generate(genColor.value, genPattern.value);
            previewCanvas.width = lastResult.canvas.width;
            previewCanvas.height = lastResult.canvas.height;
            previewCanvas.getContext('2d').drawImage(lastResult.canvas, 0, 0);
            previewCanvas.style.width = '100%';
            previewCanvas.style.height = 'auto';
        };

        btnPreview.addEventListener('click', doPreview);
        // 切换图案时自动预览
        genPattern.addEventListener('change', doPreview);
        genColor.addEventListener('input', doPreview);

        // 初始预览
        doPreview();

        btnAdd.addEventListener('click', () => {
            if (!lastResult) { doPreview(); }
            const name = `纹理_${genColor.value.replace('#', '')}_${genPattern.value}`;
            this.sm.addTexture(name, lastResult.base64);
            this.showToast(`纹理 "${name}" 已添加到纹理池`, 'success');
        });
    }

    _gotoCamera() {
        const px = parseFloat(document.getElementById('cam-pos-x').value) || 0;
        const py = parseFloat(document.getElementById('cam-pos-y').value) || 0;
        const pz = parseFloat(document.getElementById('cam-pos-z').value) || 0;
        const tx = parseFloat(document.getElementById('cam-tgt-x').value) || 0;
        const ty = parseFloat(document.getElementById('cam-tgt-y').value) || 0;
        const tz = parseFloat(document.getElementById('cam-tgt-z').value) || 0;
        this.cc.setView(new THREE.Vector3(px, py, pz), new THREE.Vector3(tx, ty, tz));
        this.showToast('摄像机已定位', 'success');
    }

    _togglePanels() {
        this._panelsVisible = !this._panelsVisible;
        this.leftArea.classList.toggle('hidden', !this._panelsVisible);
        this.rightPanel.classList.toggle('hidden', !this._panelsVisible);
        this.cameraPanel.style.display = this._panelsVisible ? 'flex' : 'none';
        this.btnTogglePanels.querySelector('i').className = this._panelsVisible ? 'fas fa-columns' : 'fas fa-columns';
        this.showToast(this._panelsVisible ? '面板已显示' : '面板已隐藏', 'success');
    }

    _switchTab(tabName) {
        // 确定目标标签属于哪个面板，只影响该面板内的标签
        let targetPanel = null;
        for (const [panelId, tabs] of Object.entries(this._panelTabs)) {
            if (tabs.includes(tabName)) { targetPanel = panelId; break; }
        }

        this._tabElements.forEach(t => {
            const parentPanel = t.closest('.side-panel');
            if (!parentPanel || !parentPanel.id) return;
            if (targetPanel && parentPanel.id !== targetPanel) return; // 跳过其他面板的标签
            t.classList.toggle('active', t.dataset.tab === tabName);
        });
        Object.entries(this._tabBodies).forEach(([k, v]) => {
            if (!v) return;
            // 只切换目标面板内的 body
            if (targetPanel && this._panelTabs[targetPanel] && !this._panelTabs[targetPanel].includes(k)) return;
            v.style.display = k === tabName ? '' : 'none';
        });
    }

    showPrimitiveDialog(type) {
        const def = PRIMITIVES[type];
        if (!def) return;
        const overlay = document.getElementById('modal-overlay');
        const content = document.getElementById('modal-content');

        let html = `<h3 style="margin-bottom:12px"><i class="fas ${def.icon}"></i> 添加${def.name}</h3>`;
        if (def.params) {
            for (const p of def.params) {
                html += `<div class="inspector-row"><label>${p.label}</label>
                    <input type="range" class="prim-param" data-key="${p.key}" min="${p.min}" max="${p.max}" step="${p.step}" value="${p.default}" style="flex:1">
                    <span class="prim-param-val" data-key="${p.key}" style="width:42px;text-align:right;font-size:12px;font-variant-numeric:tabular-nums">${p.default}</span></div>`;
            }
        }
        html += `<div style="display:flex;gap:8px;margin-top:16px">
            <button class="btn-glass btn-glass-primary" id="prim-dialog-add" style="flex:1;height:36px;font-size:14px"><i class="fas fa-plus"></i> 添加</button>
            <button class="btn-glass" id="prim-dialog-cancel" style="flex:1;height:36px;font-size:14px">取消</button></div>`;

        content.innerHTML = html;
        overlay.classList.add('visible');

        // 实时更新参数值
        content.querySelectorAll('.prim-param').forEach(slider => {
            const valEl = content.querySelector(`.prim-param-val[data-key="${slider.dataset.key}"]`);
            slider.addEventListener('input', () => {
                const v = parseFloat(slider.value);
                valEl.textContent = Number.isInteger(v) && slider.step >= 1 ? v : v.toFixed(2);
            });
        });

        const close = () => { overlay.classList.remove('visible'); };
        content.querySelector('#prim-dialog-cancel').addEventListener('click', close);
        overlay.addEventListener('click', (e) => { if (e.target === overlay) close(); });

        content.querySelector('#prim-dialog-add').addEventListener('click', () => {
            const params = {};
            content.querySelectorAll('.prim-param').forEach(slider => {
                const v = parseFloat(slider.value);
                params[slider.dataset.key] = Number.isInteger(v) && slider.step >= 1 ? v : v;
            });
            this.sm.addPrimitive(type, params);
            this.refresh();
            this.showToast(`${def.name} 已添加`, 'success');
            close();
        });
    }

    _handleAction(action) {
        // 图元添加 → 弹出参数配置对话框
        if (action.startsWith('add-')) {
            const type = action.replace('add-', '');
            this.showPrimitiveDialog(type);
            return;
        }
        switch (action) {
            case 'delete-selected': { const sel = this.sm.selected; if (sel) this.pm.removeBody(sel); this.sm.deleteSelected(); this.refresh(); this.showToast('已删除', 'success'); break; }
            case 'duplicate-selected': this.sm.duplicateSelected(); this.refresh(); this.showToast('已复制', 'success'); break;
            case 'group-selected': {
                const group = this.sm.groupSelected();
                if (group) { this.refresh(); this.showToast('已组合', 'success'); }
                else this.showToast('请至少选择2个对象（Ctrl+点击多选）', 'info');
                break;
            }
            case 'ungroup-selected': {
                const children = this.sm.ungroupSelected();
                if (children) { this.refresh(); this.showToast('已取消组合', 'success'); }
                else this.showToast('当前选中对象不是组合体', 'info');
                break;
            }
            case 'export-scene': AssetManager.exportScene(this.sm, this.pm); this.showToast('方案已导出', 'success'); break;
            case 'import-scene': this.importInput.click(); break;
            case 'import-group-asset': this.groupImportInput.click(); break;
            case 'view-top': if (this.sm.selected) { this.cc.focusOnAxis(this.sm.selected, 'top'); } else { this.cc.setView(new THREE.Vector3(0, 10, 0.01), new THREE.Vector3(0, 0, 0)); } break;
            case 'view-front': if (this.sm.selected) { this.cc.focusOnAxis(this.sm.selected, 'front'); } else { this.cc.setView(new THREE.Vector3(0, 0, 10), new THREE.Vector3(0, 0, 0)); } break;
            case 'view-right': if (this.sm.selected) { this.cc.focusOnAxis(this.sm.selected, 'right'); } else { this.cc.setView(new THREE.Vector3(10, 0, 0), new THREE.Vector3(0, 0, 0)); } break;
        }
        this.refresh();
    }

    async _handleImport(file) {
        try {
            const data = await AssetManager.importScene(file);
            await this._loadSceneData(data);
            this.showToast(`方案已导入：${data.objects.length} 个对象`, 'success');
        } catch (err) { this.showToast('导入失败: ' + err.message, 'error'); }
    }

    async _handleTextureImport(files) {
        const fileList = Array.from(files).filter(f => f.type.match(/image\/(png|jpeg|webp)/));
        if (fileList.length === 0) return;
        const results = await Promise.all(fileList.map(file => {
            return new Promise((resolve, reject) => {
                const reader = new FileReader();
                reader.onload = e => resolve({ name: file.name.replace(/\.[^.]+$/, ''), base64: e.target.result });
                reader.onerror = reject;
                reader.readAsDataURL(file);
            });
        }));
        for (const { name, base64 } of results) {
            this.sm.addTexture(name, base64);
        }
        this.renderTexturePool();
        this.showToast(`已导入 ${results.length} 个纹理`, 'success');
    }

    // ============ 冲突对话框 ============
    _showConflictModal(title, message, option1, option2) {
        return new Promise((resolve) => {
            const overlay = document.createElement('div');
            overlay.style.cssText = 'position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,0.6);z-index:10000;display:flex;align-items:center;justify-content:center;';
            overlay.innerHTML = `
                <div class="glass-panel" style="padding:24px;max-width:420px;text-align:center;">
                    <h3 style="margin:0 0 12px;">${title}</h3>
                    <p style="margin:0 0 20px;color:var(--text-dim);">${message}</p>
                    <div style="display:flex;gap:12px;justify-content:center;">
                        <button class="btn-glass" id="conflict-opt1" style="flex:1;padding:10px 0;">${option1}</button>
                        <button class="btn-glass btn-glass-primary" id="conflict-opt2" style="flex:1;padding:10px 0;">${option2}</button>
                    </div>
                </div>
            `;
            document.body.appendChild(overlay);
            overlay.querySelector('#conflict-opt1').addEventListener('click', () => { overlay.remove(); resolve(1); });
            overlay.querySelector('#conflict-opt2').addEventListener('click', () => { overlay.remove(); resolve(2); });
        });
    }

    // ============ 方案导入（合并策略） ============
    async _loadSceneData(data) {
        // 纹理导入：冲突时弹窗询问
        if (data.textures) {
            for (const t of data.textures) {
                if (this.sm.texturePool.has(t.name)) {
                    const choice = await this._showConflictModal(
                        '纹理冲突',
                        `纹理 "${t.name}" 已存在，如何处理？`,
                        '保留',
                        '覆盖'
                    );
                    if (choice === 2) {
                        this.sm.removeTexture(t.name);
                        this.sm.addTexture(t.name, t.base64);
                    }
                } else {
                    this.sm.addTexture(t.name, t.base64);
                }
            }
        }
        if (data.skybox) {
            const currentTop = '#' + this.sm.skySphere.material.uniforms.topColor.value.getHexString();
            const currentMid = '#' + this.sm.skySphere.material.uniforms.midColor.value.getHexString();
            const currentBottom = '#' + this.sm.skySphere.material.uniforms.bottomColor.value.getHexString();
            const isDefaultSky = (currentTop === '#ffffff' && currentMid === '#ffffff' && currentBottom === '#ffffff');
            const isSameSky = (currentTop === data.skybox.top && currentMid === data.skybox.mid && currentBottom === data.skybox.bottom);
            let applySky = false;
            if (!isDefaultSky && !isSameSky) {
                const choice = await this._showConflictModal(
                    '天空盒冲突',
                    `当前天空盒配置与导入方案不同。`,
                    '保留',
                    '覆盖'
                );
                if (choice === 2) applySky = true;
            } else {
                applySky = true;
            }
            if (applySky) {
                this.sm.setSkyboxColors(data.skybox.top, data.skybox.mid, data.skybox.bottom);
                // 同步更新天空盒输入框
                const skyTopEl = document.getElementById('sky-top');
                const skyMidEl = document.getElementById('sky-mid');
                const skyBottomEl = document.getElementById('sky-bottom');
                if (skyTopEl) skyTopEl.value = data.skybox.top || '#ffffff';
                if (skyMidEl) skyMidEl.value = data.skybox.mid || '#ffffff';
                if (skyBottomEl) skyBottomEl.value = data.skybox.bottom || '#ffffff';
            }
        }
        if (data.lighting) {
            const currentAmbient = this.sm.ambientLight.intensity;
            const currentHemi = this.sm.hemiLight.intensity;
            const currentSun = this.sm.sunLight.intensity;
            const isDefaultLight = (currentAmbient === 0.6 && currentHemi === 0.5 && currentSun === 1.2);
            const isSameLight = (currentAmbient === data.lighting.ambient && currentHemi === data.lighting.hemi && currentSun === data.lighting.sun);
            if (!isDefaultLight && !isSameLight) {
                const choice = await this._showConflictModal(
                    '光照冲突',
                    `当前光照配置与导入方案不同。`,
                    '保留',
                    '覆盖'
                );
                if (choice === 2) {
                    this.sm.setAmbientIntensity(data.lighting.ambient ?? 0.6);
                    this.sm.setHemiIntensity(data.lighting.hemi ?? 0.5);
                    this.sm.setSunIntensity(data.lighting.sun ?? 1.2);
                    if (data.lighting.sunDir) this.sm.setSunDirection(data.lighting.sunDir.x, data.lighting.sunDir.y, data.lighting.sunDir.z);
                    this.sm.setShadowsEnabled(data.lighting.shadows ?? true);
                    // 更新阴影按钮
                    const shadowsBtn = document.getElementById('light-shadows');
                    if (shadowsBtn) {
                        const active = data.lighting.shadows ?? true;
                        shadowsBtn.classList.toggle('active', active);
                        shadowsBtn.textContent = active ? '开启' : '关闭';
                    }
                }
            } else {
                this.sm.setAmbientIntensity(data.lighting.ambient ?? 0.6);
                this.sm.setHemiIntensity(data.lighting.hemi ?? 0.5);
                this.sm.setSunIntensity(data.lighting.sun ?? 1.2);
                if (data.lighting.sunDir) this.sm.setSunDirection(data.lighting.sunDir.x, data.lighting.sunDir.y, data.lighting.sunDir.z);
                this.sm.setShadowsEnabled(data.lighting.shadows ?? true);
                // 更新阴影按钮
                const shadowsBtn = document.getElementById('light-shadows');
                if (shadowsBtn) {
                    const active = data.lighting.shadows ?? true;
                    shadowsBtn.classList.toggle('active', active);
                    shadowsBtn.textContent = active ? '开启' : '关闭';
                }
            }
        }

        // 地面设置
        if (data.ground) {
            this.sm.setGridVisible(data.ground.gridVisible ?? true);
            this.sm.setGroundVisible(data.ground.groundVisible ?? true);
            if (data.ground.size != null) this.sm.setGroundSize(data.ground.size);
            if (data.ground.color) this.sm.setGridColor(data.ground.color);
            // 更新 UI
            const gridBtn = document.getElementById('ground-grid');
            const planeBtn = document.getElementById('ground-plane');
            const sizeEl = document.getElementById('ground-size');
            const colorEl = document.getElementById('ground-color');
            if (gridBtn) { gridBtn.classList.toggle('active', data.ground.gridVisible ?? true); gridBtn.textContent = (data.ground.gridVisible ?? true) ? '开启' : '关闭'; }
            if (planeBtn) { planeBtn.classList.toggle('active', data.ground.groundVisible ?? true); planeBtn.textContent = (data.ground.groundVisible ?? true) ? '开启' : '关闭'; }
            if (sizeEl) sizeEl.value = data.ground.size ?? 20;
            if (colorEl) colorEl.value = data.ground.color ?? '#444466';
        }

        // 物理模拟参数
        if (data.physics && this.pm) {
            this.pm.gravity = data.physics.gravity ?? -9.82;
            this.pm.groundY = data.physics.groundY ?? -0.5;
            this.pm.massSingle = data.physics.massSingle ?? 1;
            this.pm.linearDamping = data.physics.linearDamping ?? 0.1;
            this.pm.angularDamping = data.physics.angularDamping ?? 0.1;
            // 更新 UI
            const physGravity = document.getElementById('phys-gravity');
            const physGroundY = document.getElementById('phys-ground-y');
            const physMassSingle = document.getElementById('phys-mass-single');
            const physLinearDamping = document.getElementById('phys-linear-damping');
            const physAngularDamping = document.getElementById('phys-angular-damping');
            if (physGravity) physGravity.value = data.physics.gravity ?? -9.82;
            if (physGroundY) physGroundY.value = data.physics.groundY ?? -0.5;
            if (physMassSingle) physMassSingle.value = data.physics.massSingle ?? 1;
            if (physLinearDamping) physLinearDamping.value = data.physics.linearDamping ?? 0.1;
            if (physAngularDamping) physAngularDamping.value = data.physics.angularDamping ?? 0.1;
        }

        for (const objData of data.objects) {
            if (objData.type === 'group') {
                const group = new THREE.Group();
                group.userData = {
                    id: objData.id != null ? objData.id : this.sm.nextId++,
                    name: objData.name || '组合体',
                    type: 'group',
                    children: [],
                    physics: objData.physics ? JSON.parse(JSON.stringify(objData.physics)) : { enabled: false, anchored: false, autoRotate: { enabled: false, axis: 'y', speed: 1 }, initialKinetic: { enabled: false, velocity: { x: 0, y: 0, z: 0 } }, attraction: { enabled: false, targetId: '', strength: 1 }, repulsion: { enabled: false, targetId: '', strength: 1 } },
                };
                if (objData.id != null) {
                    this.sm.nextId = Math.max(this.sm.nextId, objData.id + 1);
                }
                group.position.set(objData.position.x, objData.position.y, objData.position.z);
                group.rotation.set(objData.rotation.x, objData.rotation.y, objData.rotation.z);
                group.scale.set(objData.scale.x, objData.scale.y, objData.scale.z);

                for (const childData of (objData.children || [])) {
                    const child = this._createMeshFromData(childData);
                    group.add(child);
                    group.userData.children.push(child.userData.id);
                }

                this.sm.scene.add(group);
                this.sm.objects.push(group);
                this.sm.groups.push(group);
            } else {
                const mesh = this._createMeshFromData(objData);
                this.sm.scene.add(mesh);
                this.sm.objects.push(mesh);
            }
        }
        // 导入关键帧
        if (data.keyframes && Array.isArray(data.keyframes)) {
            this.sm.keyframes = data.keyframes;
        }
        this.renderTexturePool();
        this.refresh();
    }

    async _loadGroupAsset(data) {
        const group = new THREE.Group();
        group.userData = {
            id: data.id != null ? data.id : this.sm.nextId++,
            name: data.name || '导入组合体',
            type: 'group',
            children: [],
            physics: data.physics ? JSON.parse(JSON.stringify(data.physics)) : { enabled: false, anchored: false, autoRotate: { enabled: false, axis: 'y', speed: 1 }, initialKinetic: { enabled: false, velocity: { x: 0, y: 0, z: 0 } }, attraction: { enabled: false, targetId: '', strength: 1 }, repulsion: { enabled: false, targetId: '', strength: 1 } },
        };
        if (data.id != null) {
            this.sm.nextId = Math.max(this.sm.nextId, data.id + 1);
        }
        group.position.set(data.position.x, data.position.y, data.position.z);
        group.rotation.set(data.rotation.x, data.rotation.y, data.rotation.z);
        group.scale.set(data.scale.x, data.scale.y, data.scale.z);

        for (const childData of (data.children || [])) {
            const child = this._createMeshFromData(childData);
            group.add(child);
            group.userData.children.push(child.userData.id);
        }

        this.sm.scene.add(group);
        this.sm.objects.push(group);
        this.sm.groups.push(group);
        this.sm.select(group);
        this.refresh();
    }

    _createMeshFromData(objData) {
        const geo = objData.geometry ? AssetManager.deserializeGeometry(objData.geometry) : new THREE.BoxGeometry(1, 1, 1);

        // 多面材质
        let mat, faceNames, faceVisible;
        if (objData.faceMaterials && objData.faceNames) {
            const mats = objData.faceMaterials.map(fm => this.sm._createMaterial({
                color: fm.color, roughness: fm.roughness, metalness: fm.metalness,
                opacity: fm.opacity, transparent: fm.transparent,
                side: fm.side, flatShading: fm.flatShading, wireframe: fm.wireframe,
                textureName: fm.textureName,
            }));
            mat = mats;
            faceNames = objData.faceNames;
            faceVisible = objData.faceVisible;
        } else {
            mat = this.sm._createMaterial(objData.material);
        }

        const mesh = new THREE.Mesh(geo, mat);
        mesh.castShadow = true; mesh.receiveShadow = true;
        mesh.position.set(objData.position.x, objData.position.y, objData.position.z);
        mesh.rotation.set(objData.rotation.x, objData.rotation.y, objData.rotation.z);
        mesh.scale.set(objData.scale.x, objData.scale.y, objData.scale.z);
        mesh.userData = {
            id: objData.id != null ? objData.id : this.sm.nextId++,
            name: objData.name || '导入对象',
            type: objData.type || 'imported',
            primitiveType: objData.primitiveType || null,
            primitiveParams: objData.geometry ? { ...objData.geometry } : null,
            faceNames: faceNames || null,
            faceVisible: faceVisible || null,
            textureName: objData.material?.textureName || null,
            physics: objData.physics ? JSON.parse(JSON.stringify(objData.physics)) : { enabled: false, anchored: false, autoRotate: { enabled: false, axis: 'y', speed: 1 }, initialKinetic: { enabled: false, velocity: { x: 0, y: 0, z: 0 } }, attraction: { enabled: false, targetId: '', strength: 1 }, repulsion: { enabled: false, targetId: '', strength: 1 } },
        };
        if (objData.id != null) {
            this.sm.nextId = Math.max(this.sm.nextId, objData.id + 1);
        }
        return mesh;
    }

    // ============ 对象拾取 ============
    _pickObject(clientX, clientY, event) {
        const rect = this.sm.canvas.getBoundingClientRect();
        const mouse = new THREE.Vector2(
            ((clientX - rect.left) / rect.width) * 2 - 1,
            -((clientY - rect.top) / rect.height) * 2 + 1
        );
        this.sm.raycaster.setFromCamera(mouse, this.sm.camera);
        // 过滤：仅保留有有效材质和几何体的可拾取对象
        // 同时检查材质数组与几何体 group materialIndex 是否匹配，避免 Three.js raycast 崩溃
        const pickable = this.sm.objects.filter(obj => {
            if (obj.isGroup) return true;
            if (!obj.isMesh || !obj.material || !obj.geometry) return false;
            if (Array.isArray(obj.material) && obj.geometry.groups) {
                for (const group of obj.geometry.groups) {
                    if (group.materialIndex >= obj.material.length) return false;
                }
            }
            return true;
        });
        const intersects = this.sm.raycaster.intersectObjects(pickable, true);
        if (intersects.length > 0) {
            let obj = intersects[0].object;
            // 如果点击的是组合体子对象，选择父级组合体
            if (obj.parent && obj.parent !== this.sm.scene && this.sm.objects.includes(obj.parent)) {
                obj = obj.parent;
            }
            if (event && (event.ctrlKey || event.metaKey)) {
                this.sm._toggleSelection(obj);
            } else {
                this.sm.select(obj);
            }
            // 自动切换到属性面板
            this._switchTab('inspector');
        } else {
            this.sm.deselect();
        }
        this.refresh();
    }

    // ============ 层级树 ============
    renderHierarchy() {
        const tree = this.hierarchyTree;
        tree.innerHTML = '';
        if (this.sm.objects.length === 0) {
            tree.innerHTML = '<div class="empty-state"><i class="fas fa-cube"></i><p>暂无对象</p><small>使用顶部工具栏添加图元</small></div>';
            return;
        }
        for (const obj of this.sm.objects) {
            this._renderTreeItem(tree, obj, 0, null);
        }
    }

    _renderTreeItem(parent, obj, depth, groupParent) {
        const item = document.createElement('div');
        item.className = 'tree-item';
        item.style.paddingLeft = (12 + depth * 16) + 'px';
        if (obj === this.sm.selected) item.classList.add('selected');
        if (this.sm._selectionSet.has(obj) && obj !== this.sm.selected) item.classList.add('multi-selected');

        if (obj.userData.type === 'group') {
            const hasChildren = obj.children.length > 0;
            const isCollapsed = obj.userData._treeCollapsed !== false; // 默认折叠
            const iconClass = isCollapsed ? 'fa-folder' : 'fa-folder-open';
            item.innerHTML = `<i class="fas ${iconClass}"></i><span class="tree-name">${this._esc(obj.userData.name || '未命名')}</span><span class="tree-child-count">(${obj.children.length})</span>${this._physGearIcon(obj)}`;
            parent.appendChild(item);
            item.addEventListener('click', (e) => {
                if (e.ctrlKey || e.metaKey) {
                    this.sm._toggleSelection(obj);
                } else {
                    this.sm.select(obj);
                }
                this._switchTab('inspector');
                this.refresh();
            });
            // 渲染子对象（折叠状态跟随 isCollapsed）
            if (hasChildren) {
                const childContainer = document.createElement('div');
                childContainer.className = 'tree-children';
                childContainer.style.display = isCollapsed ? 'none' : 'block';
                for (const child of obj.children) {
                    this._renderTreeItem(childContainer, child, depth + 1, obj);
                }
                parent.appendChild(childContainer);
                const iconEl = item.querySelector('i');
                if (iconEl) {
                    iconEl.style.cursor = 'pointer';
                    iconEl.addEventListener('click', (e) => {
                        e.stopPropagation();
                        obj.userData._treeCollapsed = !(obj.userData._treeCollapsed !== false);
                        this.renderHierarchy();
                    });
                }
                // 双击聚焦
                item.addEventListener('dblclick', (e) => {
                    e.stopPropagation();
                    this.sm.select(obj);
                    this.cc.focusOnObject(obj);
                    this.refresh();
                    this.showToast(`聚焦: ${obj.userData.name}`, 'success');
                });
            }
        } else {
            const type = obj.userData.primitiveType || obj.userData.type || 'unknown';
            const icon = PRIMITIVES[type] ? PRIMITIVES[type].icon : 'fa-cube';
            const indent = groupParent ? '└ ' : '';
            item.innerHTML = `<i class="fas ${icon}"></i><span class="tree-name">${indent}${this._esc(obj.userData.name || '未命名')}</span>${this._physGearIcon(obj)}`;
            parent.appendChild(item);
            item.addEventListener('click', (e) => {
                // 点击组合体子对象 → 选中父级组合体
                const target = groupParent || obj;
                if (e.ctrlKey || e.metaKey) {
                    this.sm._toggleSelection(target);
                } else {
                    this.sm.select(target);
                }
                this._switchTab('inspector');
                this.refresh();
            });
            // 双击聚焦
            item.addEventListener('dblclick', (e) => {
                e.stopPropagation();
                const target = groupParent || obj;
                this.sm.select(target);
                this.cc.focusOnObject(target);
                this.refresh();
                this.showToast(`聚焦: ${target.userData.name}`, 'success');
            });
        }
    }

    // ============ 纹理池 ============
    renderTexturePool() {
        const grid = this.texturePoolGrid;
        grid.innerHTML = '';
        for (const [name, entry] of this.sm.texturePool) {
            const item = document.createElement('div');
            item.className = 'texture-pool-item';
            item.innerHTML = `<img src="${entry.base64}" alt="${name}"><span class="tex-name">${name}</span><button class="tex-delete" data-name="${this._esc(name)}">×</button>`;
            item.addEventListener('click', e => {
                if (e.target.closest('.tex-delete')) return;
                if (this.sm.selected) {
                    this.sm.applyTextureToSelected(name);
                    this.renderInspector();
                    this.showToast(`纹理 "${name}" 已应用`, 'success');
                }
            });
            grid.appendChild(item);
        }
        // 添加按钮（不绑定独立 click 监听，由网格委托处理）
        const addBtn = document.createElement('div');
        addBtn.className = 'texture-pool-item texture-pool-add';
        addBtn.id = 'texture-pool-add';
        addBtn.innerHTML = '<i class="fas fa-plus"></i>';
        grid.appendChild(addBtn);
    }

    // ============ 属性检查器 ============
    renderInspector() {
        const body = this.inspectorBody;
        const obj = this.sm.selected;
        if (!obj) {
            body.innerHTML = '<div class="empty-state"><i class="fas fa-mouse-pointer"></i><p>未选中对象</p><small>点击场景中的物体或左侧层级树</small></div>';
            return;
        }
        const p = obj.position, r = obj.rotation, s = obj.scale;
        const mat = Array.isArray(obj.material) ? obj.material[0] : obj.material;
        const colorHex = mat && mat.color ? '#' + mat.color.getHexString() : '#888888';
        const currentTex = obj.userData.textureName || '';

        // 纹理选择器
        let texSelectorHTML = '<div class="texture-selector">';
        texSelectorHTML += `<div class="texture-selector-none" title="移除纹理"><i class="fas fa-times"></i></div>`;
        for (const [name, entry] of this.sm.texturePool) {
            const sel = name === currentTex ? ' selected' : '';
            texSelectorHTML += `<div class="texture-selector-item${sel}" data-tex="${this._esc(name)}" title="${this._esc(name)}"><img src="${entry.base64}"></div>`;
        }
        texSelectorHTML += '</div>';

        // 渲染（面剔除）选项
        const currentSide = mat ? mat.side : 0;
        const sideOptions = [
            { val: 2, label: '双面渲染' },
            { val: 1, label: '正面剔除' },
            { val: 0, label: '背面剔除' },
        ];
        let sideHTML = '<div class="inspector-section"><div class="inspector-section-title">渲染</div>';
        sideHTML += '<div class="inspector-row"><label>面剔除</label><select id="insp-side">';
        for (const opt of sideOptions) {
            sideHTML += `<option value="${opt.val}"${currentSide === opt.val ? ' selected' : ''}>${opt.label}</option>`;
        }
        sideHTML += '</select></div></div>';

        // 面可见性（仅 cube/cylinder/cone）
        let faceVisHTML = '';
        if (obj.userData.faceVisible && obj.userData.faceNames && obj.userData.primitiveType) {
            faceVisHTML = '<div class="inspector-section"><div class="inspector-section-title">面可见性</div>';
            faceVisHTML += '<div class="face-vis-grid">';
            const displayOrder = obj.userData.primitiveType === 'cube'
                ? [4, 2, 1, 5, 3, 0]  // 前, 上, 左, 后, 下, 右
                : [...Array(obj.userData.faceNames.length).keys()];
            for (const idx of displayOrder) {
                const active = obj.userData.faceVisible[idx] ? ' active' : '';
                faceVisHTML += `<button class="face-vis-btn${active}" id="insp-face-${idx}" data-face="${idx}">${this._esc(obj.userData.faceNames[idx])}</button>`;
            }
            faceVisHTML += '</div></div>';
        }

        const isGroup = obj.userData.type === 'group';

        body.innerHTML = `
            <div class="inspector-section">
                <div class="inspector-section-title">基本信息</div>
                <div class="inspector-row"><label>名称</label><input type="text" id="insp-name" value="${this._esc(obj.userData.name || '')}"></div>
                <div class="inspector-row"><label>类型</label><span style="font-size:13px;color:var(--text-dim)">${obj.userData.primitiveType || obj.userData.type || '未知'}</span></div>
            </div>
            <div class="inspector-section">
                <div class="inspector-section-title">位置</div>
                <div class="inspector-row"><label>X</label><input type="number" id="insp-pos-x" value="${p.x.toFixed(2)}" step="0.1"><label>Y</label><input type="number" id="insp-pos-y" value="${p.y.toFixed(2)}" step="0.1"><label>Z</label><input type="number" id="insp-pos-z" value="${p.z.toFixed(2)}" step="0.1"></div>
            </div>
            <div class="inspector-section">
                <div class="inspector-section-title">旋转</div>
                <div class="inspector-row"><label>X</label><input type="number" id="insp-rot-x" value="${(r.x * 180 / Math.PI).toFixed(1)}" step="1"><label>Y</label><input type="number" id="insp-rot-y" value="${(r.y * 180 / Math.PI).toFixed(1)}" step="1"><label>Z</label><input type="number" id="insp-rot-z" value="${(r.z * 180 / Math.PI).toFixed(1)}" step="1"></div>
            </div>
            <div class="inspector-section">
                <div class="inspector-section-title">缩放</div>
                <div class="inspector-row"><label>X</label><input type="number" id="insp-scl-x" value="${s.x.toFixed(2)}" step="0.1"><label>Y</label><input type="number" id="insp-scl-y" value="${s.y.toFixed(2)}" step="0.1"><label>Z</label><input type="number" id="insp-scl-z" value="${s.z.toFixed(2)}" step="0.1"></div>
            </div>
            ${isGroup ? '' : `
            <div class="inspector-section">
                <div class="inspector-section-title">材质</div>
                <div class="inspector-row"><label>颜色</label><input type="color" id="insp-color" value="${colorHex}"></div>
                <div class="inspector-row"><label>粗糙度</label><input type="range" id="insp-roughness" min="0" max="1" step="0.01" value="${(mat?.roughness != null) ? mat.roughness : 0.5}"><span style="font-size:11px;width:30px;text-align:right">${(mat?.roughness != null) ? mat.roughness.toFixed(2) : '0.50'}</span></div>
                <div class="inspector-row"><label>金属度</label><input type="range" id="insp-metalness" min="0" max="1" step="0.01" value="${(mat?.metalness != null) ? mat.metalness : 0.1}"><span style="font-size:11px;width:30px;text-align:right">${(mat?.metalness != null) ? mat.metalness.toFixed(2) : '0.10'}</span></div>
                <div class="inspector-row"><label>透明度</label><input type="range" id="insp-opacity" min="0" max="1" step="0.01" value="${(mat?.opacity != null) ? mat.opacity : 1}"><span style="font-size:11px;width:30px;text-align:right">${(mat?.opacity != null) ? mat.opacity.toFixed(2) : '1.00'}</span></div>
            </div>
            ${sideHTML}
            ${faceVisHTML}
            <div class="inspector-section">
                <div class="inspector-section-title">纹理</div>
                ${texSelectorHTML}
            </div>
            `}
            ${isGroup ? `
            <div class="inspector-section">
                <div class="inspector-section-title">旋转轴心</div>
                <div class="inspector-row"><label>X</label><input type="number" id="insp-pivot-x" value="${p.x.toFixed(2)}" step="0.1"><label>Y</label><input type="number" id="insp-pivot-y" value="${p.y.toFixed(2)}" step="0.1"><label>Z</label><input type="number" id="insp-pivot-z" value="${p.z.toFixed(2)}" step="0.1"></div>
            </div>
            <div class="inspector-section">
                <button class="btn-glass btn-glass-primary" id="insp-export-group" style="width:100%;height:52px;font-size:15px;font-weight:600"><i class="fas fa-box"></i> 导出组合体资产</button>
            </div>
            ` : `
            <div class="inspector-section">
                <button class="btn-glass btn-glass-primary" id="insp-export-model" style="width:100%;margin-bottom:8px;height:52px;font-size:15px;font-weight:600"><i class="fas fa-file-export"></i> 导出模型</button>
                <button class="btn-glass btn-glass-primary" id="insp-export-material" style="width:100%;height:52px;font-size:15px;font-weight:600"><i class="fas fa-palette"></i> 导出材质</button>
            </div>
            `}
        `;

        this._bindInspectorEvents(obj);
    }

    _bindInspectorEvents(obj) {
        const bindNum = (id, getter, setter) => {
            const el = document.getElementById(id);
            if (!el) return;
            el.addEventListener('input', () => {
                setter(parseFloat(el.value) || 0);
                this._scheduleAutoSave();
            });
        };

        const nameEl = document.getElementById('insp-name');
        if (nameEl) nameEl.addEventListener('input', () => { obj.userData.name = nameEl.value; this.renderHierarchy(); });

        bindNum('insp-pos-x', () => obj.position.x, v => obj.position.x = v);
        bindNum('insp-pos-y', () => obj.position.y, v => obj.position.y = v);
        bindNum('insp-pos-z', () => obj.position.z, v => obj.position.z = v);
        bindNum('insp-rot-x', () => obj.rotation.x * 180 / Math.PI, v => obj.rotation.x = v * Math.PI / 180);
        bindNum('insp-rot-y', () => obj.rotation.y * 180 / Math.PI, v => obj.rotation.y = v * Math.PI / 180);
        bindNum('insp-rot-z', () => obj.rotation.z * 180 / Math.PI, v => obj.rotation.z = v * Math.PI / 180);
        bindNum('insp-scl-x', () => obj.scale.x, v => obj.scale.x = v);
        bindNum('insp-scl-y', () => obj.scale.y, v => obj.scale.y = v);
        bindNum('insp-scl-z', () => obj.scale.z, v => obj.scale.z = v);

        // 颜色：支持多材质
        const colorEl = document.getElementById('insp-color');
        if (colorEl) {
            colorEl.addEventListener('input', () => {
                if (Array.isArray(obj.material)) {
                    obj.material.forEach(m => { m.color.set(colorEl.value); m.needsUpdate = true; });
                } else if (obj.material) {
                    obj.material.color.set(colorEl.value);
                }
                this._scheduleAutoSave();
            });
        }

        const bindRange = (id, prop) => {
            const el = document.getElementById(id);
            if (!el) return;
            el.addEventListener('input', () => {
                const val = parseFloat(el.value);
                if (Array.isArray(obj.material)) {
                    obj.material.forEach(m => { m[prop] = val; m.needsUpdate = true; });
                } else if (obj.material) {
                    obj.material[prop] = val;
                    obj.material.needsUpdate = true;
                }
                const span = el.nextElementSibling;
                if (span) span.textContent = val.toFixed(2);
                this._scheduleAutoSave();
            });
        };
        bindRange('insp-roughness', 'roughness');
        bindRange('insp-metalness', 'metalness');
        bindRange('insp-opacity', 'opacity');
        const opacityEl = document.getElementById('insp-opacity');
        if (opacityEl) {
            opacityEl.addEventListener('input', () => {
                const transparent = parseFloat(opacityEl.value) < 1;
                if (Array.isArray(obj.material)) {
                    obj.material.forEach(m => { m.transparent = transparent; m.needsUpdate = true; });
                } else if (obj.material) {
                    obj.material.transparent = transparent;
                    obj.material.needsUpdate = true;
                }
            });
        }

        // 面剔除（渲染）
        const sideEl = document.getElementById('insp-side');
        if (sideEl) {
            sideEl.addEventListener('change', () => {
                const side = parseInt(sideEl.value);
                if (Array.isArray(obj.material)) {
                    obj.material.forEach(m => { m.side = side; m.needsUpdate = true; });
                } else if (obj.material) {
                    obj.material.side = side;
                    obj.material.needsUpdate = true;
                }
                this._scheduleAutoSave();
            });
        }

        // 面可见性
        if (obj.userData.faceVisible && obj.userData.faceNames) {
            for (let i = 0; i < obj.userData.faceVisible.length; i++) {
                const btn = document.getElementById(`insp-face-${i}`);
                if (!btn) continue;
                btn.addEventListener('click', () => {
                    const visible = !obj.userData.faceVisible[i];
                    obj.userData.faceVisible[i] = visible;
                    btn.classList.toggle('active', visible);
                    if (Array.isArray(obj.material)) {
                        if (visible) {
                            if (obj.userData._faceMaterials && obj.userData._faceMaterials[i]) {
                                obj.material[i].copy(obj.userData._faceMaterials[i]);
                            }
                            obj.material[i].transparent = false;
                            obj.material[i].opacity = 1;
                            obj.material[i].depthWrite = true;
                        } else {
                            obj.material[i].transparent = true;
                            obj.material[i].opacity = 0;
                            obj.material[i].depthWrite = false;
                        }
                        obj.material[i].needsUpdate = true;
                    }
                    this._scheduleAutoSave();
                });
            }
        }

        // 旋转轴心（仅组合体）
        if (obj.userData.type === 'group') {
            // 辅助：移动轴心并保持子对象世界位置不变
            const _movePivot = (newPivot) => {
                // 保存所有子对象的世界位置
                const saved = [];
                for (const child of obj.children) {
                    if (child.isMesh || child.isGroup) {
                        saved.push({ child, worldPos: new THREE.Vector3() });
                        child.getWorldPosition(saved[saved.length - 1].worldPos);
                    }
                }
                // 移动轴心
                obj.position.copy(newPivot);
                obj.updateMatrixWorld();
                // 还原子对象世界位置
                for (const { child, worldPos } of saved) {
                    child.position.copy(obj.worldToLocal(worldPos.clone()));
                }
            };

            const bindPivot = (axis) => {
                const el = document.getElementById(`insp-pivot-${axis}`);
                if (!el) return;
                el.addEventListener('input', () => {
                    const newVal = parseFloat(el.value) || 0;
                    const newPivot = obj.position.clone();
                    newPivot[axis] = newVal;
                    _movePivot(newPivot);
                    this._scheduleAutoSave();
                });
            };
            bindPivot('x');
            bindPivot('y');
            bindPivot('z');
        }

        // 纹理选择器
        document.querySelectorAll('.texture-selector-item').forEach(el => {
            el.addEventListener('click', () => {
                const texName = el.dataset.tex;
                this.sm.applyTextureToSelected(texName);
                this.renderInspector();
                this.showToast(`纹理 "${texName}" 已应用`, 'success');
            });
        });
        const texNone = document.querySelector('.texture-selector-none');
        if (texNone) texNone.addEventListener('click', () => { this.sm.removeTextureFromSelected(); this.renderInspector(); this.showToast('纹理已移除', 'success'); });

        // 导出按钮
        const expGroup = document.getElementById('insp-export-group');
        if (expGroup) expGroup.addEventListener('click', () => { AssetManager.exportGroupAsset(obj); this.showToast('组合体资产已导出', 'success'); });
        const expModel = document.getElementById('insp-export-model');
        if (expModel) expModel.addEventListener('click', () => { AssetManager.exportModel(obj); this.showToast('模型已导出', 'success'); });
        const expMat = document.getElementById('insp-export-material');
        if (expMat) expMat.addEventListener('click', () => { AssetManager.exportMaterial(obj); this.showToast('材质已导出', 'success'); });
    }

    // ============ 状态栏 ============
    updateStatus(fps) {
        this.statusFps.textContent = fps;
        this.statusObjects.textContent = this.sm.objects.length;
        this.statusFaces.textContent = this.sm.getTotalTriangles().toLocaleString();
        // 实时摄像机坐标
        const pos = this.sm.camera.position;
        document.getElementById('status-cam').textContent = `${pos.x.toFixed(1)}, ${pos.y.toFixed(1)}, ${pos.z.toFixed(1)}`;
    }

    _updateStatusMode(mode) {
        if (this.statusMode) {
            this.statusMode.textContent = `模式: ${mode}`;
        }
    }

    showToast(msg, type) {
        if (this._toastTimer) clearTimeout(this._toastTimer);
        this.toast.textContent = msg;
        this.toast.className = 'toast visible ' + (type || '');
        this._toastTimer = setTimeout(() => this.toast.classList.remove('visible'), 2000);
    }

    refresh() {
        this.renderHierarchy();
        this.renderInspector();
        this.renderKeyframeList();
        this.cc._cameraSpaceEnabled = !this.sm.selected;
        this._scheduleAutoSave();
    }

    // ============ 关键帧管理 ============
    _kfAdd() {
        const idx = this.sm.addKeyframe();
        this.showToast(`关键帧 ${idx} 已创建`, 'success');
        this.refresh();
        this._scheduleAutoSave();
    }

    _kfDelete() {
        if (this.sm.keyframes.length === 0) {
            this.showToast('没有可删除的关键帧', 'error');
            return;
        }
        const lastIdx = this.sm.keyframes.length - 1;
        this.sm.deleteKeyframe(lastIdx);
        this.showToast(`关键帧 ${lastIdx} 已删除`, 'success');
        this.refresh();
        this._scheduleAutoSave();
    }

    _kfTogglePlay() {
        if (this.sm.keyframes.length < 2) {
            this.showToast('至少需要2个关键帧才能播放', 'error');
            return;
        }
        this.sm.isPlaying = !this.sm.isPlaying;
        if (this.sm.isPlaying) {
            this.sm._interpTimer = 0;
            this.sm._playFrameIdx = 0;
            this.sm.applyKeyframeState(0);
            this.btnKfPlay.innerHTML = '<i class="fas fa-pause"></i> 暂停';
            this.showToast('动画播放中...', 'success');
        } else {
            this.btnKfPlay.innerHTML = '<i class="fas fa-play"></i> 播放';
            this.showToast('动画已暂停', 'success');
        }
        this.refresh();
    }

    _kfStop() {
        this.sm.isPlaying = false;
        this.sm._interpTimer = 0;
        this.sm._playFrameIdx = 0;
        this.sm.applyKeyframeState(0);
        this.btnKfPlay.innerHTML = '<i class="fas fa-play"></i> 播放';
        this.showToast('动画已停止', 'success');
        this.refresh();
    }

    renderKeyframeList() {
        const kfs = this.sm.keyframes;
        if (kfs.length === 0) {
            this.keyframeList.innerHTML = '<div class="empty-state"><i class="fas fa-film"></i><p>无关键帧</p><small>点击"添加帧"创建</small></div>';
            return;
        }
        let html = '';
        kfs.forEach((kf, i) => {
            const st = kf.state;
            const parts = [];
            if (st.objects && st.objects.length > 0) parts.push(`${st.objects.length}个对象`);
            if (st.lighting && st.lighting.sunDir) parts.push('光照');
            const info = parts.length > 0 ? parts.join(', ') : '空';
            const active = (this.sm._playFrameIdx === i && this.sm.isPlaying) ? ' active' : '';
            html += `<div class="keyframe-list-item${active}" data-kf-idx="${i}">
                <span class="kf-index">#${i}</span>
                <span class="kf-info">${info}</span>
                <input type="number" class="kf-delay" value="${kf.delay}" min="0.1" max="30" step="0.1" title="延迟(秒)" data-kf-idx="${i}">
            </div>`;
        });
        this.keyframeList.innerHTML = html;

        // 点击关键帧项跳转
        this.keyframeList.querySelectorAll('.keyframe-list-item').forEach(item => {
            item.addEventListener('click', e => {
                if (e.target.classList.contains('kf-delay')) return; // 不拦截输入框点击
                const idx = parseInt(item.dataset.kfIdx);
                this.sm._playFrameIdx = idx;
                this.sm.applyKeyframeState(idx);
                this.refresh();
                this.showToast(`已跳转到关键帧 ${idx}`, 'success');
            });
        });
        // 每帧延迟时间修改
        this.keyframeList.querySelectorAll('.kf-delay').forEach(input => {
            input.addEventListener('input', () => {
                const idx = parseInt(input.dataset.kfIdx);
                this.sm.keyframes[idx].delay = Math.max(0.1, parseFloat(input.value) || 1.0);
                this._scheduleAutoSave();
            });
        });
    }

    // ============ 操作说明（选项卡式） ============
    _showHelpModal() {
        const overlay = document.getElementById('modal-overlay');
        const content = document.getElementById('modal-content');
        content.innerHTML = `
            <h3 style="margin-bottom:16px"><i class="fas fa-book-open"></i> 操作说明</h3>
            <div class="help-tabs">
                <button class="help-tab active" data-help-tab="mouse"><i class="fas fa-mouse"></i> 键鼠操作</button>
                <button class="help-tab" data-help-tab="keys"><i class="fas fa-keyboard"></i> 快捷键</button>
                <button class="help-tab" data-help-tab="physics"><i class="fas fa-atom"></i> 物理系统</button>
                <button class="help-tab" data-help-tab="features"><i class="fas fa-cubes"></i> 功能说明</button>
            </div>
            <div style="max-height:60vh;overflow-y:auto;font-size:13px;line-height:1.8">

                <div class="help-panel active" data-help-panel="mouse">
                    <p class="help-section-title">视角控制</p>
                    <p>左键拖拽: 旋转视角 | 右键拖拽 / 中键: 平移视角 | 滚轮: 缩放</p>
                    <p class="help-section-title">对象选择</p>
                    <p>左键点击: 选中对象 | <kbd>Ctrl</kbd>+点击: 多选 | 左键空白: 取消选中</p>
                    <p class="help-section-title">层级面板</p>
                    <p>双击元素 / 组合体: 聚焦显示该对象</p>
                    <p class="help-section-title">自由视角（无选中对象时）</p>
                    <p><kbd>Space</kbd> 抬升相机 | <kbd>Shift</kbd> 降低相机</p>
                </div>

                <div class="help-panel" data-help-panel="keys">
                    <p class="help-section-title">操作模式切换</p>
                    <p><kbd>1</kbd> 位移模式 | <kbd>2</kbd> 旋转模式 | <kbd>3</kbd> 缩放模式 | <kbd>4</kbd> 光照模式</p>
                    <p class="help-section-title">模式内操作（方向键 / 空格 / Shift）</p>
                    <p>位移: 方向键平移 XZ · <kbd>Space</kbd> 上升 · <kbd>Shift</kbd> 下降</p>
                    <p>旋转: 方向键绕轴 · <kbd>Space</kbd>/<kbd>Shift</kbd> 绕 Z 轴</p>
                    <p>缩放: 方向键缩放 XZ · <kbd>Space</kbd>/<kbd>Shift</kbd> 缩放 Y</p>
                    <p>光照: 方向键移动太阳 · <kbd>Space</kbd>/<kbd>Shift</kbd> 升降太阳</p>
                    <p class="help-section-title">选中对象快捷键</p>
                    <p><kbd>R</kbd> (按住) 环绕图元 | <kbd>W</kbd>/<kbd>A</kbd>/<kbd>S</kbd>/<kbd>D</kbd> 键盘平移视角</p>
                    <p class="help-section-title">通用快捷键</p>
                    <p><kbd>Delete</kbd> / <kbd>Backspace</kbd> 删除选中对象</p>
                    <p><kbd>Ctrl</kbd>+<kbd>G</kbd> 组合 | <kbd>Ctrl</kbd>+<kbd>Shift</kbd>+<kbd>G</kbd> 取消组合</p>
                </div>

                <div class="help-panel" data-help-panel="physics">
                    <p class="help-section-title">物理模拟</p>
                    <p><kbd>Q</kbd> 打开物理属性配置（锚点 / 自动旋转 / 初始动能 / 引力斥力）</p>
                    <p><kbd>E</kbd> 对全部图元施加 / 取消物理效果</p>
                    <p><kbd>Z</kbd> 切换碰撞体线框可视化（青色线框显示实际碰撞形状）</p>
                    <p class="help-section-title accent">物理操控模式（需先 E 启动模拟 + 选中动态图元）</p>
                    <p><kbd>X</kbd> 进入 / 退出越肩操控模式（相机聚焦并持续追踪选中图元）</p>
                    <p><kbd>W</kbd>/<kbd>S</kbd> 沿视角前 / 后施力 | <kbd>A</kbd>/<kbd>D</kbd> 沿视角左 / 右施力</p>
                    <p><kbd>Space</kbd> 跳跃（基于真实接地检测 + 冷却，落地前无法再次起跳）</p>
                    <p>鼠标拖拽可旋转越肩视角，操控方向实时跟随视角</p>
                    <p>退出时自动恢复默认视角与操控方式；移动过程冻结旋转确保平稳</p>
                </div>

                <div class="help-panel" data-help-panel="features">
                    <p class="help-section-title">关键帧动画</p>
                    <p>在"关键帧"选项卡中管理动画帧 | 每帧独立设置延迟时间</p>
                    <p>帧 0 记录初始状态，后续帧仅记录变化 | 帧间线性插值平滑过渡</p>
                    <p class="help-section-title">纹理系统</p>
                    <p>在"纹理"选项卡中导入与管理纹理资源，点击纹理池应用至选中对象</p>
                    <p class="help-section-title">场景设置</p>
                    <p>在"设置"选项卡调整天空盒配色、光照强度、地面尺寸、阴影与网格</p>
                    <p class="help-section-title">自动保存</p>
                    <p>场景变更后每 10 秒自动保存，下次启动可恢复上次工程</p>
                </div>

            </div>
            <div style="display:flex;gap:8px;margin-top:16px;justify-content:flex-end">
                <button class="btn-glass btn-glass-primary" id="help-close" style="padding:8px 24px;height:36px;font-size:14px">关闭</button>
            </div>
        `;
        overlay.classList.add('visible');

        // 选项卡切换逻辑
        content.querySelectorAll('.help-tab').forEach(tab => {
            tab.addEventListener('click', () => {
                const target = tab.dataset.helpTab;
                content.querySelectorAll('.help-tab').forEach(t => t.classList.toggle('active', t === tab));
                content.querySelectorAll('.help-panel').forEach(p => {
                    p.classList.toggle('active', p.dataset.helpPanel === target);
                });
            });
        });

        const close = () => overlay.classList.remove('visible');
        content.querySelector('#help-close').addEventListener('click', close);
        overlay.addEventListener('click', (e) => { if (e.target === overlay) close(); });
    }

    // ============ 自动保存 ============
    _scheduleAutoSave() {
        this._dirty = true;
    }

    _encodePath(filename) {
        const encoded = encodeURIComponent(filename);
        const decoded = encoded.replace(/%([0-9A-F]{2})/g, (_, p1) => String.fromCharCode(parseInt(p1, 16)));
        return btoa(decoded);
    }

    async _doAutoSave() {
        try {
            const data = {
                skybox: {
                    top: '#' + this.sm.skySphere.material.uniforms.topColor.value.getHexString(),
                    mid: '#' + this.sm.skySphere.material.uniforms.midColor.value.getHexString(),
                    bottom: '#' + this.sm.skySphere.material.uniforms.bottomColor.value.getHexString(),
                },
                lighting: {
                    ambient: this.sm.ambientLight.intensity,
                    hemi: this.sm.hemiLight.intensity,
                    sun: this.sm.sunLight.intensity,
                    sunDir: { x: this.sm.sunLight.position.x, y: this.sm.sunLight.position.y, z: this.sm.sunLight.position.z },
                    shadows: this.sm.sunLight.castShadow,
                },
                ground: {
                    gridVisible: this.sm.gridHelper.visible,
                    groundVisible: this.sm.groundPlane.visible,
                    size: this.sm.gridHelper.geometry.parameters?.size || 20,
                    color: this.sm.gridColor,
                },
                physics: this.pm ? {
                    gravity: this.pm.gravity,
                    groundY: this.pm.groundY,
                    massSingle: this.pm.massSingle,
                    linearDamping: this.pm.linearDamping,
                    angularDamping: this.pm.angularDamping,
                } : null,
                textures: [...this.sm.texturePool.values()].map(e => ({ name: e.name, base64: e.base64 })),
                objects: this.sm.objects.map(obj => AssetManager._serialize(obj)),
                keyframes: this.sm.keyframes,
            };
            const json = JSON.stringify({ version: '2.0', savedAt: new Date().toISOString(), ...data }, null, 2);
            console.log('[自动保存] 正在写入 status.json, 对象数:', this.sm.objects.length, '关键帧数:', this.sm.keyframes.length);
            const resp = await fetch('/file/write', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-File-Name': this._encodePath('package/lunar.studio.engine/status.json'),
                    'X-Overwrite': 'true',
                },
                body: json,
            });
            console.log('[自动保存] 完成, 状态:', resp.status, resp.ok ? 'OK' : 'FAIL');
        } catch (e) {
            console.error('[自动保存] 失败:', e.message || e);
        }
    }

    // ============ 刷新恢复模态框 ============
    async _showRecoveryModal() {
        let hasSaved = false;
        try {
            const resp = await fetch('/file/read/package/lunar.studio.engine/status.json');
            if (resp.ok) {
                const text = await resp.text();
                if (text && text.trim()) {
                    const data = JSON.parse(text);
                    if (data.objects && data.objects.length > 0) hasSaved = true;
                }
            }
        } catch (e) { /* 无保存 */ }

        if (!hasSaved) return;

        return new Promise((resolve) => {
            const overlay = document.createElement('div');
            overlay.style.cssText = 'position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,0.6);z-index:10000;display:flex;align-items:center;justify-content:center;';
            overlay.innerHTML = `
                <div class="glass-panel" style="padding:24px;max-width:420px;text-align:center;">
                    <h3 style="margin:0 0 8px"><i class="fas fa-history"></i> 检测到之前的工程</h3>
                    <p style="margin:0 0 20px;color:var(--text-dim);">发现上次未完成的工程状态，请选择如何处理？</p>
                    <div class="modal-option" id="recovery-opt1">
                        <div><div class="opt-label">继续之前的工程</div><div class="opt-desc">恢复上次保存的场景状态</div></div>
                        <i class="fas fa-chevron-right" style="color:var(--brand)"></i>
                    </div>
                    <div class="modal-option" id="recovery-opt2">
                        <div><div class="opt-label">新建工程并保存之前的工程状态</div><div class="opt-desc">将旧工程保存为文件，创建新工程</div></div>
                        <i class="fas fa-chevron-right" style="color:var(--brand)"></i>
                    </div>
                    <div class="modal-option" id="recovery-opt3">
                        <div><div class="opt-label">新建工程并放弃之前的工程状态</div><div class="opt-desc">直接丢弃旧工程，创建新工程</div></div>
                        <i class="fas fa-chevron-right" style="color:var(--brand)"></i>
                    </div>
                </div>
            `;
            document.body.appendChild(overlay);

            const cleanup = (choice) => {
                overlay.remove();
                resolve(choice);
            };

            overlay.querySelector('#recovery-opt1').addEventListener('click', () => cleanup(1));
            overlay.querySelector('#recovery-opt2').addEventListener('click', () => cleanup(2));
            overlay.querySelector('#recovery-opt3').addEventListener('click', () => cleanup(3));
        });
    }

    _esc(str) {
        const div = document.createElement('div'); div.textContent = str; return div.innerHTML;
    }

    _physGearIcon(obj) {
        const phys = obj.userData.physics;
        if (!phys) return '';
        const hasConfig = phys.anchored || phys.autoRotate?.enabled || phys.initialKinetic?.enabled || phys.attraction?.enabled || phys.repulsion?.enabled;
        return hasConfig ? ' <i class="fas fa-cog" style="color:#9d6bff;font-size:11px;margin-left:4px;" title="已配置物理属性"></i>' : '';
    }

    // ============ 物理属性配置模态框 ============
    _showPhysicsModal(mesh) {
        this._physModalTarget = mesh;
        const phys = mesh.userData.physics || {};
        const overlay = document.getElementById('physics-modal-overlay');
        document.getElementById('phys-modal-title').textContent = mesh.userData.name;

        // 锚定点
        const anchorBtn = document.getElementById('phys-anchor-toggle');
        this._setToggleBtn(anchorBtn, phys.anchored || false);

        // 自动旋转
        const autoRotateEnabled = phys.autoRotate?.enabled || false;
        const rotateBtn = document.getElementById('phys-rotate-toggle');
        this._setToggleBtn(rotateBtn, autoRotateEnabled);
        document.getElementById('phys-rotate-axis-row').style.display = autoRotateEnabled ? '' : 'none';
        document.getElementById('phys-rotate-speed-row').style.display = autoRotateEnabled ? '' : 'none';
        document.querySelectorAll('.phys-axis-btn').forEach(b => {
            b.classList.toggle('active', b.dataset.axis === (phys.autoRotate?.axis || 'y'));
        });
        document.getElementById('phys-rotate-speed').value = phys.autoRotate?.speed || 1;

        // 初始动能（与自动旋转互斥：KINEMATIC 刚体不受力）
        const kineticEnabled = phys.initialKinetic?.enabled && !autoRotateEnabled;
        const kineticBtn = document.getElementById('phys-kinetic-toggle');
        this._setToggleBtn(kineticBtn, kineticEnabled);
        document.getElementById('phys-kinetic-params').style.display = kineticEnabled ? '' : 'none';
        document.getElementById('phys-kinetic-vx').value = phys.initialKinetic?.velocity?.x || 0;
        document.getElementById('phys-kinetic-vy').value = phys.initialKinetic?.velocity?.y || 0;
        document.getElementById('phys-kinetic-vz').value = phys.initialKinetic?.velocity?.z || 0;

        // 引力
        const attractBtn = document.getElementById('phys-attract-toggle');
        this._setToggleBtn(attractBtn, phys.attraction?.enabled || false);
        document.getElementById('phys-attract-params').style.display = phys.attraction?.enabled ? '' : 'none';
        document.getElementById('phys-attract-strength').value = phys.attraction?.strength || 1;
        this._populatePhysTargets('phys-attract-target', phys.attraction?.targetId, mesh.userData.id);

        // 斥力
        const repelBtn = document.getElementById('phys-repel-toggle');
        this._setToggleBtn(repelBtn, phys.repulsion?.enabled || false);
        document.getElementById('phys-repel-params').style.display = phys.repulsion?.enabled ? '' : 'none';
        document.getElementById('phys-repel-strength').value = phys.repulsion?.strength || 1;
        this._populatePhysTargets('phys-repel-target', phys.repulsion?.targetId, mesh.userData.id);

        this._bindPhysModalEvents();
        overlay.classList.add('visible');
    }

    _setToggleBtn(btn, active) {
        btn.dataset.active = active ? 'true' : 'false';
        btn.textContent = active ? '开启' : '关闭';
        btn.classList.toggle('active', active);
    }

    _populatePhysTargets(selectId, selectedId, selfId) {
        const sel = document.getElementById(selectId);
        sel.innerHTML = '<option value="">-- 选择对象 --</option>';
        for (const obj of this.sm.objects) {
            if (obj.userData.id === selfId) continue;
            sel.innerHTML += `<option value="${obj.userData.id}" ${obj.userData.id === selectedId ? 'selected' : ''}>${this._esc(obj.userData.name)}</option>`;
        }
    }

    _bindPhysModalEvents() {
        const toggle = (btnId, showIds) => {
            const btn = document.getElementById(btnId);
            const active = btn.dataset.active === 'true';
            const newActive = !active;
            this._setToggleBtn(btn, newActive);
            showIds.forEach(id => {
                const el = document.getElementById(id);
                if (el) el.style.display = newActive ? '' : 'none';
            });
            // 锚定点与初始动能互斥
            if (btnId === 'phys-anchor-toggle' && newActive) {
                this._setToggleBtn(document.getElementById('phys-kinetic-toggle'), false);
                document.getElementById('phys-kinetic-params').style.display = 'none';
            }
            if (btnId === 'phys-kinetic-toggle' && newActive) {
                this._setToggleBtn(document.getElementById('phys-anchor-toggle'), false);
                // 初始动能与自动旋转互斥（KINEMATIC 刚体不受力）
                this._setToggleBtn(document.getElementById('phys-rotate-toggle'), false);
                document.getElementById('phys-rotate-axis-row').style.display = 'none';
                document.getElementById('phys-rotate-speed-row').style.display = 'none';
            }
            // 自动旋转与初始动能互斥（KINEMATIC 刚体不受力，初始动能无效）
            if (btnId === 'phys-rotate-toggle' && newActive) {
                this._setToggleBtn(document.getElementById('phys-kinetic-toggle'), false);
                document.getElementById('phys-kinetic-params').style.display = 'none';
                this.showToast('自动旋转已启用：物体转为 KINEMATIC 刚体，初始动能已自动关闭', 'info');
            }
        };

        document.getElementById('phys-anchor-toggle').onclick = () => toggle('phys-anchor-toggle', []);
        document.getElementById('phys-rotate-toggle').onclick = () => toggle('phys-rotate-toggle', ['phys-rotate-axis-row', 'phys-rotate-speed-row']);
        document.getElementById('phys-kinetic-toggle').onclick = () => toggle('phys-kinetic-toggle', ['phys-kinetic-params']);
        document.getElementById('phys-attract-toggle').onclick = () => toggle('phys-attract-toggle', ['phys-attract-params']);
        document.getElementById('phys-repel-toggle').onclick = () => toggle('phys-repel-toggle', ['phys-repel-params']);

        // 旋转轴按钮
        document.querySelectorAll('.phys-axis-btn').forEach(b => {
            b.onclick = () => {
                document.querySelectorAll('.phys-axis-btn').forEach(bb => bb.classList.remove('active'));
                b.classList.add('active');
            };
        });

        // 保存/取消
        document.getElementById('phys-modal-save').onclick = () => this._savePhysicsConfig();
        document.getElementById('phys-modal-cancel').onclick = () => this._hidePhysicsModal();

        // 点击遮罩关闭
        document.getElementById('physics-modal-overlay').onclick = (e) => {
            if (e.target === document.getElementById('physics-modal-overlay')) this._hidePhysicsModal();
        };
    }

    _hidePhysicsModal() {
        document.getElementById('physics-modal-overlay').classList.remove('visible');
        this._physModalTarget = null;
    }

    _savePhysicsConfig() {
        const mesh = this._physModalTarget;
        if (!mesh) return;
        if (!mesh.userData.physics) mesh.userData.physics = {};

        const phys = mesh.userData.physics;
        phys.enabled = true;
        phys.anchored = document.getElementById('phys-anchor-toggle').dataset.active === 'true';
        phys.autoRotate = {
            enabled: document.getElementById('phys-rotate-toggle').dataset.active === 'true',
            axis: document.querySelector('.phys-axis-btn.active')?.dataset?.axis || 'y',
            speed: parseFloat(document.getElementById('phys-rotate-speed').value) || 1,
        };
        phys.initialKinetic = {
            enabled: document.getElementById('phys-kinetic-toggle').dataset.active === 'true',
            velocity: {
                x: parseFloat(document.getElementById('phys-kinetic-vx').value) || 0,
                y: parseFloat(document.getElementById('phys-kinetic-vy').value) || 0,
                z: parseFloat(document.getElementById('phys-kinetic-vz').value) || 0,
            },
        };
        phys.attraction = {
            enabled: document.getElementById('phys-attract-toggle').dataset.active === 'true',
            targetId: document.getElementById('phys-attract-target').value || '',
            strength: parseFloat(document.getElementById('phys-attract-strength').value) || 1,
        };
        phys.repulsion = {
            enabled: document.getElementById('phys-repel-toggle').dataset.active === 'true',
            targetId: document.getElementById('phys-repel-target').value || '',
            strength: parseFloat(document.getElementById('phys-repel-strength').value) || 1,
        };

        // 如果启用了物理属性，(重新)创建刚体以应用最新配置（类型/碰撞体/角速度等）
        if (phys.enabled) {
            if (this.pm.bodies.has(mesh.userData.id)) {
                this.pm.removeBody(mesh);
            }
            this.pm._ensureGround();
            this.pm.createBody(mesh);
        }

        this._hidePhysicsModal();
        this.refresh();
        this._scheduleAutoSave();
        this.showToast(`物理配置已保存: ${mesh.userData.name}`, 'success');
    }
}

export { UIManager };