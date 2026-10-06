/**
 * 缩略图：Canvas 2D 画全图轮廓 + 视口矩形，点击/拖动定位视口。
 *
 * 用 canvas 而不是 DOM，是为了在数百节点时也能保持流畅；
 * 节点只画色块（不画文字），连线按折线近似采样。
 */

import { rectOf } from "../core/geometry.js";
import { sampleRoute } from "../core/router.js";
import { capturePointer, releasePointer } from "../interaction.js";

const PADDING = 10;

export class Minimap {
	/**
	 * @param {import("../app/store.js").Store} store
	 * @param {import("./viewport.js").Viewport} viewport
	 * @param {{host:HTMLElement, canvas:HTMLCanvasElement}} dom
	 */
	constructor(store, viewport, dom) {
		this.store = store;
		this.viewport = viewport;
		this.host = dom.host;
		this.canvas = dom.canvas;
		this.ctx = this.canvas.getContext("2d");
		this.dragging = false;
		this._raf = 0;
		this.worldRect = null;

		this.bind();
	}

	bind() {
		this.canvas.addEventListener("pointerdown", (event) => {
			event.preventDefault();
			this.dragging = true;
			capturePointer(this.canvas, event.pointerId);
			this.centerOnEvent(event);
		});
		this.canvas.addEventListener("pointermove", (event) => {
			if (!this.dragging) return;
			this.centerOnEvent(event);
		});
		const stop = (event) => {
			this.dragging = false;
			releasePointer(this.canvas, event.pointerId);
		};
		this.canvas.addEventListener("pointerup", stop);
		this.canvas.addEventListener("pointercancel", stop);
	}

	centerOnEvent(event) {
		if (!this.worldRect) return;
		const rect = this.canvas.getBoundingClientRect();
		const sx = (event.clientX - rect.left - PADDING) / Math.max(1, rect.width - PADDING * 2);
		const sy = (event.clientY - rect.top - PADDING) / Math.max(1, rect.height - PADDING * 2);
		const wx = this.worldRect.x + sx * this.worldRect.w;
		const wy = this.worldRect.y + sy * this.worldRect.h;
		this.viewport.centerOn(wx, wy);
	}

	/** 合并多次请求，每帧最多画一次 */
	schedule() {
		if (this._raf) return;
		this._raf = requestAnimationFrame(() => {
			this._raf = 0;
			this.draw();
		});
	}

	draw() {
		if (!this.store.settings.showMinimap) return;
		const ctx = this.ctx;
		const rect = this.canvas.getBoundingClientRect();
		const dpr = window.devicePixelRatio || 1;
		const w = Math.max(1, Math.round(rect.width));
		const h = Math.max(1, Math.round(rect.height));
		if (this.canvas.width !== Math.round(w * dpr) || this.canvas.height !== Math.round(h * dpr)) {
			this.canvas.width = Math.round(w * dpr);
			this.canvas.height = Math.round(h * dpr);
		}
		ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
		ctx.clearRect(0, 0, w, h);

		const visible = this.viewport.visibleWorldRect();
		const graph = this.store.graph;
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
		if (!box) box = { ...visible };
		// 同时容纳当前视口，保证视口框始终可见
		{
			const x2 = Math.max(box.x + box.w, visible.x + visible.w);
			const y2 = Math.max(box.y + box.h, visible.y + visible.h);
			box.x = Math.min(box.x, visible.x);
			box.y = Math.min(box.y, visible.y);
			box.w = x2 - box.x;
			box.h = y2 - box.y;
		}
		// 留一点余量，避免贴边
		const margin = Math.max(box.w, box.h) * 0.04 + 20;
		box = { x: box.x - margin, y: box.y - margin, w: box.w + margin * 2, h: box.h + margin * 2 };
		this.worldRect = box;

		const scale = Math.min((w - PADDING * 2) / box.w, (h - PADDING * 2) / box.h);
		const ox = PADDING + ((w - PADDING * 2) - box.w * scale) / 2;
		const oy = PADDING + ((h - PADDING * 2) - box.h * scale) / 2;
		const toX = (x) => ox + (x - box.x) * scale;
		const toY = (y) => oy + (y - box.y) * scale;

		// 连线
		ctx.lineWidth = 1;
		ctx.strokeStyle = "rgba(140,168,220,0.5)";
		for (const edge of graph.edges) {
			const route = this.routeCache?.get(edge.id);
			const points = route ? sampleRoute(route, 8) : null;
			if (!points || points.length < 2) continue;
			ctx.beginPath();
			ctx.moveTo(toX(points[0].x), toY(points[0].y));
			for (let i = 1; i < points.length; i++) ctx.lineTo(toX(points[i].x), toY(points[i].y));
			ctx.stroke();
		}

		// 节点
		for (const node of graph.nodes) {
			const r = rectOf(node);
			ctx.fillStyle = node.color || "#5b8dff";
			ctx.globalAlpha = node.locked ? 0.45 : 0.9;
			roundRect(ctx, toX(r.x), toY(r.y), Math.max(2, r.w * scale), Math.max(2, r.h * scale), 2);
			ctx.fill();
		}
		ctx.globalAlpha = 1;

		// 视口框
		ctx.strokeStyle = "rgba(120,200,255,0.95)";
		ctx.lineWidth = 1.5;
		ctx.strokeRect(
			toX(visible.x),
			toY(visible.y),
			Math.max(2, visible.w * scale),
			Math.max(2, visible.h * scale),
		);
		ctx.fillStyle = "rgba(120,200,255,0.10)";
		ctx.fillRect(
			toX(visible.x),
			toY(visible.y),
			Math.max(2, visible.w * scale),
			Math.max(2, visible.h * scale),
		);
	}

	/** 渲染器把最近一次算出的连线几何交给缩略图复用（避免重复求解） */
	setRouteCache(routes) {
		this.routeCache = routes;
	}
}

function roundRect(ctx, x, y, w, h, r) {
	const radius = Math.min(r, w / 2, h / 2);
	ctx.beginPath();
	ctx.moveTo(x + radius, y);
	ctx.arcTo(x + w, y, x + w, y + h, radius);
	ctx.arcTo(x + w, y + h, x, y + h, radius);
	ctx.arcTo(x, y + h, x, y, radius);
	ctx.arcTo(x, y, x + w, y, radius);
	ctx.closePath();
}
