/**
 * 自动布局：分层（Sugiyama-lite）排布。
 *
 * 依据连线方向（输出 → 输入）计算层级，同层内按前驱重心排序以减少交叉，
 * 再把每层沿主轴依次排开、垂直方向居中。纯函数，返回 id → 位置 的映射。
 */

import { rectOf } from "../core/geometry.js";

/**
 * @param {Array} nodes 节点数组（读取 id/x/y/w/h）
 * @param {Array} edges 连线数组（读取 from.node/to.node）
 * @param {{gapMain?:number, gapCross?:number, direction?:"TB"|"LR", originX?:number, originY?:number}} [opts]
 * @returns {Map<string,{x:number,y:number}>}
 */
export function layeredLayout(nodes, edges, opts = {}) {
	const gapMain = opts.gapMain ?? 90;
	const gapCross = opts.gapCross ?? 56;
	const direction = opts.direction === "LR" ? "LR" : "TB";
	const originX = opts.originX ?? 0;
	const originY = opts.originY ?? 0;

	const list = nodes.filter((n) => n && typeof n.id === "string");
	const byId = new Map(list.map((n) => [n.id, n]));
	const result = new Map();
	if (list.length === 0) return result;

	// 邻接（去重、忽略自环与缺失端点）
	const outgoing = new Map(list.map((n) => [n.id, []]));
	const incoming = new Map(list.map((n) => [n.id, []]));
	const seen = new Set();
	for (const edge of edges || []) {
		const a = edge?.from?.node;
		const b = edge?.to?.node;
		if (!byId.has(a) || !byId.has(b) || a === b) continue;
		const key = `${a}->${b}`;
		if (seen.has(key)) continue;
		seen.add(key);
		outgoing.get(a).push(b);
		incoming.get(b).push(a);
	}

	// 最长路径分层：Kahn 拓扑序，残留（环）节点接在最大层之后
	const layer = new Map();
	const indeg = new Map(list.map((n) => [n.id, incoming.get(n.id).length]));
	const queue = list
		.filter((n) => indeg.get(n.id) === 0)
		.sort(stableOrder)
		.map((n) => n.id);
	for (const id of queue) layer.set(id, 0);

	const order = [];
	const pending = new Map(indeg);
	const fifo = queue.slice();
	while (fifo.length) {
		const id = fifo.shift();
		order.push(id);
		const base = layer.get(id) || 0;
		for (const next of outgoing.get(id)) {
			layer.set(next, Math.max(layer.get(next) ?? 0, base + 1));
			pending.set(next, (pending.get(next) || 0) - 1);
			// pending 只会单调减到 0，因此每个节点最多入队一次
			if ((pending.get(next) || 0) <= 0) fifo.push(next);
		}
	}

	let maxLayer = 0;
	for (const v of layer.values()) maxLayer = Math.max(maxLayer, v);
	const leftOver = list.filter((n) => !layer.has(n.id)).sort(stableOrder);
	for (const node of leftOver) {
		maxLayer += 1;
		layer.set(node.id, maxLayer);
	}
	for (const node of list) {
		if (!layer.has(node.id)) layer.set(node.id, 0);
	}

	// 分组
	const groups = new Map();
	for (const node of list) {
		const l = layer.get(node.id);
		if (!groups.has(l)) groups.set(l, []);
		groups.get(l).push(node);
	}
	const layers = [...groups.keys()].sort((a, b) => a - b);

	// 一次重心排序：按前驱所在位置的平均次序
	const positionInLayer = new Map();
	for (const l of layers) {
		const arr = groups.get(l).sort(stableOrder);
		arr.forEach((node, i) => positionInLayer.set(node.id, i));
	}
	for (const l of layers) {
		if (l === layers[0]) continue;
		const arr = groups.get(l);
		arr.sort((a, b) => {
			const ba = barycenter(a.id, incoming, positionInLayer);
			const bb = barycenter(b.id, incoming, positionInLayer);
			if (ba !== bb) return ba - bb;
			return stableOrder(a, b);
		});
		arr.forEach((node, i) => positionInLayer.set(node.id, i));
	}

	// 主轴尺寸：每层取该层节点的最大主向尺寸
	const mainSize = (l) => groups.get(l).reduce((acc, n) => (direction === "TB" ? Math.max(acc, n.h) : Math.max(acc, n.w)), 0);

	let cursorMain = direction === "TB" ? originY : originX;
	for (const l of layers) {
		const arr = groups.get(l);
		const crossSizes = arr.map((n) => (direction === "TB" ? n.w : n.h));
		const totalCross = crossSizes.reduce((a, b) => a + b, 0) + gapCross * Math.max(0, arr.length - 1);
		let cursorCross = -(totalCross / 2);
		arr.forEach((node, i) => {
			const size = crossSizes[i];
			if (direction === "TB") {
				result.set(node.id, {
					x: round(originX + cursorCross),
					y: round(cursorMain),
				});
			} else {
				result.set(node.id, {
					x: round(cursorMain),
					y: round(originY + cursorCross),
				});
			}
			cursorCross += size + gapCross;
		});
		cursorMain += mainSize(l) + gapMain;
	}

	// 平移修正：让整组结果的最小顶点成为 (originX, originY)
	let minX = Infinity;
	let minY = Infinity;
	for (const pos of result.values()) {
		minX = Math.min(minX, pos.x);
		minY = Math.min(minY, pos.y);
	}
	if (Number.isFinite(minX) && Number.isFinite(minY)) {
		for (const [id, pos] of result) {
			result.set(id, { x: round(pos.x - minX + originX), y: round(pos.y - minY + originY) });
		}
	}
	return result;
}

function barycenter(id, incoming, positionInLayer) {
	const preds = incoming.get(id) || [];
	if (preds.length === 0) return Number.MAX_SAFE_INTEGER / 2;
	let sum = 0;
	let count = 0;
	for (const p of preds) {
		if (positionInLayer.has(p)) {
			sum += positionInLayer.get(p);
			count++;
		}
	}
	return count ? sum / count : Number.MAX_SAFE_INTEGER / 2;
}

function stableOrder(a, b) {
	if (a.y !== b.y) return a.y - b.y;
	if (a.x !== b.x) return a.x - b.x;
	return String(a.id).localeCompare(String(b.id));
}

function round(v) {
	return Math.round(v);
}

/** 让整图（含 origin）落回正坐标区，避免长期编辑后跑到极大负值 */
export function normalizeToOrigin(nodes, padding = 80) {
	if (!nodes.length) return new Map();
	let minX = Infinity;
	let minY = Infinity;
	for (const node of nodes) {
		const r = rectOf(node);
		minX = Math.min(minX, r.x);
		minY = Math.min(minY, r.y);
	}
	const dx = minX === Infinity ? 0 : padding - minX;
	const dy = minY === Infinity ? 0 : padding - minY;
	return new Map(nodes.map((n) => [n.id, { x: Math.round(n.x + dx), y: Math.round(n.y + dy) }]));
}
