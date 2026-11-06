// ==== view-control.js — 显示层视图变换 ====
//
// 与 Live2D 引擎（lunar.live2d_engine/engine.js）保持同一套交互约定：
//   - 平移：Pointer Events + setPointerCapture，鼠标左键 / 触摸单指拖动
//   - 缩放：滚轮（按 deltaMode 换算）+ 双指捏合，围绕指针 / 双指中点
//   - 上下限：0.4 – 4 倍；平移至少保留 40px 画面在视口内
//   - 复位：双击 / 鼠标中键（不新增其他入口）
// 变换只写 #gif-layer 的 CSS transform，舞台、动画名提示与加载遮罩保持不动。

/** 与 Live2D 引擎一致的缩放上下限 */
const MIN_ZOOM = 0.4;
const MAX_ZOOM = 4;
/** 平移后至少保留的画面像素 */
const KEEP_VISIBLE = 40;

export class GifViewController {
    /** 交互舞台（占满 iframe） */
    #stage;
    /** 承载 img 的变换层 */
    #layer;
    /** 动图元素，用于取原始尺寸 */
    #image;
    /** 视频元素（WebM 模式），用于取原始尺寸 */
    #video;
    #zoom = 1;
    #panX = 0;
    #panY = 0;
    /** 按下中的指针（pointerId → 坐标） */
    #pointers = new Map();
    #dragOrigin = null;
    #pinch = null;
    #bound = false;

    constructor({ stage, layer, image, video }) {
        this.#stage = stage;
        this.#layer = layer;
        this.#image = image;
        this.#video = video || null;
    }

    /** 绑定交互事件（只绑定一次） */
    bind() {
        const stage = this.#stage;
        if (this.#bound || !stage) return;
        this.#bound = true;
        stage.addEventListener('wheel', this.#onWheel, { passive: false });
        stage.addEventListener('pointerdown', this.#onPointerDown);
        stage.addEventListener('pointermove', this.#onPointerMove);
        stage.addEventListener('pointerup', this.#onPointerUp);
        stage.addEventListener('pointercancel', this.#onPointerUp);
        stage.addEventListener('dblclick', this.#onDblClick);
        stage.addEventListener('mousedown', this.#onMouseDown);
    }

    /** 解绑并清空指针状态（切走引擎 / 页面卸载时调用） */
    destroy() {
        const stage = this.#stage;
        if (stage && this.#bound) {
            stage.removeEventListener('wheel', this.#onWheel);
            stage.removeEventListener('pointerdown', this.#onPointerDown);
            stage.removeEventListener('pointermove', this.#onPointerMove);
            stage.removeEventListener('pointerup', this.#onPointerUp);
            stage.removeEventListener('pointercancel', this.#onPointerUp);
            stage.removeEventListener('dblclick', this.#onDblClick);
            stage.removeEventListener('mousedown', this.#onMouseDown);
        }
        this.#bound = false;
        this.#pointers.clear();
        this.#dragOrigin = null;
        this.#pinch = null;
        if (stage) stage.classList.remove('dragging');
    }

    /** 复位：回到居中、1 倍 */
    reset() {
        this.#zoom = 1;
        this.#panX = 0;
        this.#panY = 0;
        this.#apply();
    }

    /**
     * 以舞台内某点为锚点缩放（factor > 1 放大，< 1 缩小）。
     * @param {number} factor 缩放系数
     * @param {number} px 锚点 x（舞台内坐标）
     * @param {number} py 锚点 y（舞台内坐标）
     */
    zoomAt(factor, px, py) {
        const nextZoom = clamp(this.#zoom * factor, MIN_ZOOM, MAX_ZOOM);
        if (nextZoom === this.#zoom) return;
        const { w, h } = this.#size();
        const k = nextZoom / this.#zoom;
        // 让锚点在缩放前后保持不动：p' = p + k * (pan - p)
        const ax = px - w / 2;
        const ay = py - h / 2;
        this.#panX = ax + (this.#panX - ax) * k;
        this.#panY = ay + (this.#panY - ay) * k;
        this.#zoom = nextZoom;
        this.#apply();
    }

    /** 按舞台像素平移 */
    panBy(dx, dy) {
        if (!dx && !dy) return;
        this.#panX += dx;
        this.#panY += dy;
        this.#apply();
    }

    /** 当前视图状态（调试 / 宿主读取） */
    getView() {
        return { zoom: this.#zoom, panX: this.#panX, panY: this.#panY };
    }

    // ==== 内部 ====

    #size() {
        return {
            w: this.#stage ? this.#stage.clientWidth : 0,
            h: this.#stage ? this.#stage.clientHeight : 0,
        };
    }

    /** 当前画面按 object-fit: contain 铺开后的实际尺寸（视频优先） */
    #contentSize(w, h) {
        const nw = this.#video?.videoWidth || this.#image?.naturalWidth || w;
        const nh = this.#video?.videoHeight || this.#image?.naturalHeight || h;
        if (!w || !h || !nw || !nh) return { cw: w, ch: h };
        const ratio = Math.min(w / nw, h / nh);
        return { cw: nw * ratio, ch: nh * ratio };
    }

    /** 把平移限制在「至少保留 40px 画面可见」的范围，并写入 transform */
    #apply() {
        const { w, h } = this.#size();
        if (!w || !h) return;
        const { cw, ch } = this.#contentSize(w, h);
        const limitX = Math.max(0, w / 2 + (cw * this.#zoom) / 2 - KEEP_VISIBLE);
        const limitY = Math.max(0, h / 2 + (ch * this.#zoom) / 2 - KEEP_VISIBLE);
        this.#panX = clamp(this.#panX, -limitX, limitX);
        this.#panY = clamp(this.#panY, -limitY, limitY);
        if (this.#layer) {
            this.#layer.style.transform =
                `translate(${this.#panX}px, ${this.#panY}px) scale(${this.#zoom})`;
        }
    }

    #localPoint(event) {
        const rect = this.#stage.getBoundingClientRect();
        return { x: event.clientX - rect.left, y: event.clientY - rect.top };
    }

    #onWheel = (event) => {
        event.preventDefault();
        const point = this.#localPoint(event);
        const unit = event.deltaMode === 1 ? 0.05 : event.deltaMode === 2 ? 1 : 0.0012;
        this.zoomAt(Math.exp(-event.deltaY * unit), point.x, point.y);
    };

    #onPointerDown = (event) => {
        const stage = this.#stage;
        if (event.pointerType === 'mouse' && event.button === 1) {
            // 中键：复位，同时屏蔽浏览器自动滚动
            event.preventDefault();
            this.reset();
            return;
        }
        if (event.pointerType === 'mouse' && event.button !== 0) return;
        event.preventDefault(); // 拖动期间不选中文字
        try { stage.setPointerCapture(event.pointerId); } catch (e) { /* 不支持时忽略 */ }
        this.#pointers.set(event.pointerId, { x: event.clientX, y: event.clientY });
        if (this.#pointers.size === 1) {
            this.#dragOrigin = { x: event.clientX, y: event.clientY };
            stage.classList.add('dragging');
        } else if (this.#pointers.size === 2) {
            const [a, b] = [...this.#pointers.values()];
            this.#pinch = { dist: distance(a, b), mid: midpoint(a, b) };
            this.#dragOrigin = null;
            stage.classList.add('dragging');
        }
    };

    #onPointerMove = (event) => {
        if (!this.#pointers.has(event.pointerId)) return;
        this.#pointers.set(event.pointerId, { x: event.clientX, y: event.clientY });
        if (this.#pointers.size >= 2 && this.#pinch) {
            const [a, b] = [...this.#pointers.values()];
            const dist = distance(a, b);
            const mid = midpoint(a, b);
            // 先跟随双指中点平移，再以新中点为锚缩放：
            // 保证「缩放前中点下的画面」正好落到「缩放后的中点」，指尖下画面不漂移
            if (this.#pinch.dist > 0 && dist > 0) {
                this.panBy(mid.x - this.#pinch.mid.x, mid.y - this.#pinch.mid.y);
                const point = this.#localPoint({ clientX: mid.x, clientY: mid.y });
                this.zoomAt(dist / this.#pinch.dist, point.x, point.y);
            }
            this.#pinch = { dist, mid };
            return;
        }
        if (!this.#dragOrigin) return;
        this.panBy(event.clientX - this.#dragOrigin.x, event.clientY - this.#dragOrigin.y);
        this.#dragOrigin = { x: event.clientX, y: event.clientY };
    };

    #onPointerUp = (event) => {
        this.#pointers.delete(event.pointerId);
        const stage = this.#stage;
        if (this.#pointers.size === 0) {
            this.#dragOrigin = null;
            this.#pinch = null;
            stage.classList.remove('dragging');
            return;
        }
        const [rest] = [...this.#pointers.values()];
        this.#dragOrigin = { x: rest.x, y: rest.y };
        this.#pinch = null;
    };

    #onDblClick = (event) => {
        event.preventDefault();
        this.reset();
    };

    #onMouseDown = (event) => {
        if (event.button === 1) event.preventDefault(); // 屏蔽中键自动滚动
    };
}

function distance(a, b) {
    return Math.hypot(a.x - b.x, a.y - b.y);
}

function midpoint(a, b) {
    return { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 };
}

function clamp(value, min, max) {
    return Math.min(max, Math.max(min, value));
}
