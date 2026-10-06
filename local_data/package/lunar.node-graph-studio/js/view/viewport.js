/**
 * 视口：世界坐标 ⇄ 屏幕坐标换算、缩放平移、适配视图、栅格背景。
 *
 * 渲染策略：节点与连线都用「世界坐标」布局，缩放/平移只改 .world 上的一层
 * CSS transform: translate(vx,vy) scale(z)。因此节点文字始终是矢量渲染（不发虚），
 * 而栅格由 .canvas 的 background-size/position 跟随视口，同样是清晰的。
 */

import { rectOf, clamp } from "../core/geometry.js";

export const ZOOM_MIN = 0.15;
export const ZOOM_MAX = 4;

export class Viewport {
	/**
	 * @param {import("../app/store.js").Store} store
	 * @param {{canvas:HTMLElement, world:HTMLElement}} dom
	 */
	constructor(store, dom) {
		this.store = store;
		this.canvas = dom.canvas;
		this.world = dom.world;
	}

	apply() {
		const vp = this.store.viewport;
		this.world.style.transform = `translate(${vp.x}px, ${vp.y}px) scale(${vp.zoom})`;

		const settings = this.store.settings;
		const grid = Number(settings.grid) || 20;
		const size = grid * vp.zoom;
		const coarse = size * 4;
		if (settings.showGrid) {
			this.canvas.style.backgroundSize = `${size}px ${size}px, ${size}px ${size}px, ${coarse}px ${coarse}px, ${coarse}px ${coarse}px`;
			this.canvas.style.backgroundPosition = `${vp.x}px ${vp.y}px`;
			this.canvas.classList.remove("grid-off");
		} else {
			this.canvas.classList.add("grid-off");
		}
		this.canvas.style.setProperty("--grid-step", `${size}px`);
	}

	screenToWorld(clientX, clientY) {
		const rect = this.canvas.getBoundingClientRect();
		const vp = this.store.viewport;
		return {
			x: (clientX - rect.left - vp.x) / vp.zoom,
			y: (clientY - rect.top - vp.y) / vp.zoom,
		};
	}

	worldToScreen(x, y) {
		const vp = this.store.viewport;
		return { x: x * vp.zoom + vp.x, y: y * vp.zoom + vp.y };
	}

	panBy(dx, dy) {
		const vp = this.store.viewport;
		this.store.setViewport({ x: vp.x + dx, y: vp.y + dy });
	}

	/** 以屏幕上某点为锚点缩放（滚轮缩放的自然行为） */
	zoomAt(clientX, clientY, factor) {
		const vp = this.store.viewport;
		const next = clamp(vp.zoom * factor, ZOOM_MIN, ZOOM_MAX);
		if (next === vp.zoom) return;
		const rect = this.canvas.getBoundingClientRect();
		const sx = clientX - rect.left;
		const sy = clientY - rect.top;
		// 保持该屏幕点对应的世界点不动：w = (s - v) / z  ⇒  v' = s - w * z'
		const wx = (sx - vp.x) / vp.zoom;
		const wy = (sy - vp.y) / vp.zoom;
		this.store.setViewport({ zoom: next, x: sx - wx * next, y: sy - wy * next });
	}

	/** 以画布中心缩放（工具栏按钮/快捷键） */
	zoomBy(factor) {
		const rect = this.canvas.getBoundingClientRect();
		this.zoomAt(rect.left + rect.width / 2, rect.top + rect.height / 2, factor);
	}

	setZoomAtCenter(zoom) {
		const rect = this.canvas.getBoundingClientRect();
		const vp = this.store.viewport;
		const next = clamp(zoom, ZOOM_MIN, ZOOM_MAX);
		const cx = rect.width / 2;
		const cy = rect.height / 2;
		const wx = (cx - vp.x) / vp.zoom;
		const wy = (cy - vp.y) / vp.zoom;
		this.store.setViewport({ zoom: next, x: cx - wx * next, y: cy - wy * next });
	}

	/**
	 * 适配视图：把给定世界包围盒放进画布（留 padding）。
	 * @param {{x:number,y:number,w:number,h:number}|null} bounds
	 * @param {number} padding 屏幕像素留白
	 */
	fit(bounds, padding = 80) {
		const rect = this.canvas.getBoundingClientRect();
		if (!bounds || bounds.w <= 0 && bounds.h <= 0) {
			this.store.setViewport({ x: rect.width / 2, y: rect.height / 2, zoom: 1 });
			return;
		}
		const availW = Math.max(40, rect.width - padding * 2);
		const availH = Math.max(40, rect.height - padding * 2);
		const zoom = clamp(Math.min(availW / Math.max(bounds.w, 1), availH / Math.max(bounds.h, 1)), ZOOM_MIN, 1.6);
		const x = (rect.width - bounds.w * zoom) / 2 - bounds.x * zoom;
		const y = (rect.height - bounds.h * zoom) / 2 - bounds.y * zoom;
		this.store.setViewport({ x, y, zoom });
	}

	/** 缩放到 100% 并保持画布中心的世界点不动 */
	resetZoom() {
		this.setZoomAtCenter(1);
	}

	/** 把某个世界点带到画布中心 */
	centerOn(x, y) {
		const rect = this.canvas.getBoundingClientRect();
		const vp = this.store.viewport;
		this.store.setViewport({ x: rect.width / 2 - x * vp.zoom, y: rect.height / 2 - y * vp.zoom });
	}

	/** 当前可见区域（世界坐标），供缩略图与「可见性」判断使用 */
	visibleWorldRect() {
		const rect = this.canvas.getBoundingClientRect();
		const vp = this.store.viewport;
		return {
			x: -vp.x / vp.zoom,
			y: -vp.y / vp.zoom,
			w: rect.width / vp.zoom,
			h: rect.height / vp.zoom,
		};
	}

	/** 自动布局所需的「节点集合包围盒」（仅节点矩形） */
	graphBounds(graph) {
		let box = null;
		for (const node of graph.nodes) {
			const r = rectOf(node);
			if (!box) box = { ...r };
			else {
				const x2 = Math.max(box.x + box.w, r.x + r.w);
				const y2 = Math.max(box.y + box.h, r.y + r.h);
				box.x = Math.min(box.x, r.x);
				box.y = Math.min(box.y, r.y);
				box.w = x2 - box.x;
				box.h = y2 - box.y;
			}
		}
		return box;
	}
}
