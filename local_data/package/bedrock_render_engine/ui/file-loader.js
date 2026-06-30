// ==== file-loader.js — 全局拖拽加载器 ====

/**
 * FileLoader — 全局拖拽文件自动分类加载
 *
 * 监听 document 的 dragover/drop 事件，根据文件扩展名和内容自动分类：
 *   - .bbmodel → 模型
 *   - .json 且含 "animation_groups" 字段 → 动画组配置（新）
 *   - .json 且含 "animations" 字段 → 动画
 *   - .json 且含 "animation_controllers" 字段 → 旧版控制器（已弃用）
 *   - .png → 纹理（暂未实现）
 *
 * 拖拽时显示遮罩层提示用户释放文件
 */
export class FileLoader {
    /**
     * @param {{
     *   onLoadBbmodel: (file: File) => void|Promise<void>,
     *   onLoadAnimations: (files: File[]) => void|Promise<void>,
     *   onLoadControllers?: (files: File[]) => void|Promise<void>,
     *   onLoadAnimGroupConfig?: (files: File[]) => void|Promise<void>,
     *   onToast?: (msg: string, type?: string) => void
     * }} handlers
     */
    constructor(handlers) {
        this.handlers = handlers;
        this._overlay = null;
        this._dragCounter = 0;
        this._bind();
    }

    /**
     * 创建拖拽遮罩
     * @private
     */
    _ensureOverlay() {
        if (this._overlay) return;
        const overlay = document.createElement('div');
        overlay.className = 'drop-overlay';
        overlay.innerHTML = `
            <div class="drop-overlay-inner glass-panel">
                <i class="fas fa-cloud-upload-alt"></i>
                <p>释放文件以加载</p>
                <small>.bbmodel · 动画 .json · 控制器 .json</small>
            </div>
        `;
        overlay.style.display = 'none';
        document.body.appendChild(overlay);
        this._overlay = overlay;
    }

    _bind() {
        this._ensureOverlay();

        // 计数器避免子元素 dragenter/dragleave 抖动
        document.addEventListener('dragenter', (e) => {
            if (!e.dataTransfer || !Array.from(e.dataTransfer.types || []).includes('Files')) return;
            e.preventDefault();
            this._dragCounter++;
            this._overlay.style.display = 'flex';
        });

        document.addEventListener('dragover', (e) => {
            if (!e.dataTransfer || !Array.from(e.dataTransfer.types || []).includes('Files')) return;
            e.preventDefault();
            e.dataTransfer.dropEffect = 'copy';
        });

        document.addEventListener('dragleave', (e) => {
            if (!Array.from(e.dataTransfer?.types || []).includes('Files')) return;
            this._dragCounter--;
            if (this._dragCounter <= 0) {
                this._dragCounter = 0;
                this._overlay.style.display = 'none';
            }
        });

        document.addEventListener('drop', async (e) => {
            if (!e.dataTransfer || !Array.from(e.dataTransfer.types || []).includes('Files')) return;
            e.preventDefault();
            this._dragCounter = 0;
            this._overlay.style.display = 'none';

            const files = Array.from(e.dataTransfer.files);
            if (files.length === 0) return;

            await this._classifyAndLoad(files);
        });
    }

    /**
     * 分类并加载文件
     * @param {File[]} files
     * @private
     */
    async _classifyAndLoad(files) {
        const models = [];
        const animations = [];
        const controllers = [];
        const groupConfigs = [];
        const unknown = [];

        for (const file of files) {
            const lower = file.name.toLowerCase();
            if (lower.endsWith('.bbmodel')) {
                models.push(file);
            } else if (lower.endsWith('.json')) {
                const kind = await this._sniffJson(file);
                if (kind === 'animation') animations.push(file);
                else if (kind === 'controller') controllers.push(file);
                else if (kind === 'anim_group_config') groupConfigs.push(file);
                else unknown.push(file);
            } else {
                unknown.push(file);
            }
        }

        // 顺序：模型 → 动画 → 动画组配置 → 旧版控制器（依赖关系）
        let loaded = 0;
        for (const f of models) {
            try { await this.handlers.onLoadBbmodel(f); loaded++; } catch (e) { console.error(e); }
        }
        if (animations.length > 0) {
            try { await this.handlers.onLoadAnimations(animations); loaded += animations.length; } catch (e) { console.error(e); }
        }
        if (groupConfigs.length > 0 && this.handlers.onLoadAnimGroupConfig) {
            try { await this.handlers.onLoadAnimGroupConfig(groupConfigs); loaded += groupConfigs.length; } catch (e) { console.error(e); }
        }
        if (controllers.length > 0 && this.handlers.onLoadControllers) {
            try { await this.handlers.onLoadControllers(controllers); loaded += controllers.length; } catch (e) { console.error(e); }
        }

        if (unknown.length > 0) {
            this.handlers.onToast?.(`跳过 ${unknown.length} 个未知文件`, 'warning');
        }
        if (loaded > 0) {
            this.handlers.onToast?.(`导入完成：${loaded} 个文件`, 'success');
        }
    }

    /**
     * 嗅探 JSON 文件类型（仅读取前 4KB 判断关键字段）
     * 优先级：animation_groups > animation_controllers > animations
     * @param {File} file
     * @returns {Promise<'animation'|'controller'|'anim_group_config'|'unknown'>}
     * @private
     */
    async _sniffJson(file) {
        try {
            // 仅读取前 4KB 避免 IO 浪费
            const slice = file.slice(0, 4096);
            const text = await slice.text();
            // 优先识别新的动画组配置格式
            if (/\"animation_groups\"\s*:/.test(text)) return 'anim_group_config';
            if (/\"animation_controllers\"\s*:/.test(text)) return 'controller';
            if (/\"animations\"\s*:/.test(text)) return 'animation';
            return 'unknown';
        } catch {
            return 'unknown';
        }
    }
}
