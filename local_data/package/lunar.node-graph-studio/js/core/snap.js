/**
 * 拖拽吸附：栅格吸附 + 节点间对齐参考线。
 *
 * 纯函数，无 DOM；返回「需要施加的位移」与「应绘制的参考线」，由调用方决定如何使用。
 */

import { rectOf, clamp } from "./geometry.js";

/**
 * 栅格吸附：把数值吸附到最近的 grid 倍数。
 * @param {number} value 待吸附坐标
 * @param {number} grid 栅格尺寸
 * @param {boolean} enabled 是否启用
 */
export function snapToGrid(value, grid, enabled) {
	if (!enabled || !(grid > 0)) return value;
	return Math.round(value / grid) * grid;
}

/**
 * 计算拖拽中的对齐吸附。
 *
 * 以被拖拽节点的 3 条纵向候选线（左/中/右）与 3 条横向候选线（上/中/下）
 * 去和其余节点的同类候选线比对，取距离最小且在阈值内的偏移量。
 *
 * **只在「横向/纵向上确实靠近」的节点之间吸附**：否则画布另一头某个节点的左边缘
 * 一旦恰好落在你的 6px 内，就会把节点猛拽过去一下——拖起来像在跟人抢鼠标。
 *
 * @param {{x:number,y:number,w:number,h:number}} dragged 拖拽矩形（已含本次位移）
 * @param {Array<{x:number,y:number,w:number,h:number}>} others 参照矩形
 * @param {number} threshold 吸附阈值（世界单位）
 * @param {number} proximity 垂直方向上的「算作相邻」距离（世界单位）
 * @returns {{dx:number, dy:number, guides:Array<{axis:"x"|"y", pos:number, from:number, to:number}>}}
 */
export function computeAlignSnap(dragged, others, threshold = 6, proximity = 90) {
	const d = rectOf(dragged);
	const draggedV = [d.x, d.x + d.w / 2, d.x + d.w];
	const draggedH = [d.y, d.y + d.h / 2, d.y + d.h];

	let bestX = null;
	let bestY = null;

	for (const o of others) {
		const r = rectOf(o);
		// 纵向对齐只在两者纵向范围接近时才有意义，横向对齐同理
		const verticalNeighbour = rangesNear(d.y, d.y + d.h, r.y, r.y + r.h, proximity);
		const horizontalNeighbour = rangesNear(d.x, d.x + d.w, r.x, r.x + r.w, proximity);
		const otherV = [r.x, r.x + r.w / 2, r.x + r.w];
		const otherH = [r.y, r.y + r.h / 2, r.y + r.h];

		if (verticalNeighbour) {
			for (const dv of draggedV) {
				for (const ov of otherV) {
					const delta = ov - dv;
					if (Math.abs(delta) > threshold) continue;
					if (!bestX || Math.abs(delta) < Math.abs(bestX.delta)) {
						bestX = {
							delta,
							pos: ov,
							from: Math.min(d.y, r.y),
							to: Math.max(d.y + d.h, r.y + r.h),
						};
					}
				}
			}
		}
		if (horizontalNeighbour) {
			for (const dh of draggedH) {
				for (const oh of otherH) {
					const delta = oh - dh;
					if (Math.abs(delta) > threshold) continue;
					if (!bestY || Math.abs(delta) < Math.abs(bestY.delta)) {
						bestY = {
							delta,
							pos: oh,
							from: Math.min(d.x, r.x),
							to: Math.max(d.x + d.w, r.x + r.w),
						};
					}
				}
			}
		}
	}

	const guides = [];
	if (bestX) guides.push({ axis: "x", pos: bestX.pos, from: bestX.from, to: bestX.to });
	if (bestY) guides.push({ axis: "y", pos: bestY.pos, from: bestY.from, to: bestY.to });

	return {
		dx: bestX ? bestX.delta : 0,
		dy: bestY ? bestY.delta : 0,
		guides,
	};
}

/** 两个区间是否重叠，或间距不超过 slack */
export function rangesNear(a1, a2, b1, b2, slack = 90) {
	if (a2 < b1) return b1 - a2 <= slack;
	if (b2 < a1) return a1 - b2 <= slack;
	return true;
}

/** 限制矩形进入给定可视范围（保证节点不丢失在无穷远处） */
export function clampRect(rect, bounds) {
	const r = rectOf(rect);
	const x = clamp(r.x, bounds.x, bounds.x + bounds.w - r.w);
	const y = clamp(r.y, bounds.y, bounds.y + bounds.h - r.h);
	return { x, y };
}
