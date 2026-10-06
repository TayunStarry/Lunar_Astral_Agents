/**
 * 图模型：节点/连线/端口的增删改与不变量维护。
 *
 * 不变量：
 *   1. 连线恒为 out → in；
 *   2. 连线两端必须指向存在的节点、且端口下标在该侧该方向的端口数量范围内；
 *   3. 端口数量变化后，越界连线被收敛（下标夹取），该侧该向已无端口的连线被删除。
 *
 * 只有本类会改动 nodes/edges；一切修改都应经由此处，便于历史与渲染统一收口。
 * 不依赖 DOM。
 */

import { defaultPorts, invertPortRef, normalizePorts, portCount, reconcileEdgesForPorts, SIDES } from "../core/ports.js";
import { sanitizeDocument } from "./document.js";
import { getNodeType, MIN_NODE_W, MIN_NODE_H, MAX_NODE_W, MAX_NODE_H } from "./palette.js";
import { nodeId, edgeId } from "../core/ids.js";
import { rectOf, rectIntersects, rectUnion } from "../core/geometry.js";

export class Graph {
	constructor(doc) {
		this.nodes = [];
		this.edges = [];
		this.settings = {};
		this.name = "未命名节点图";
		this.viewport = { x: 0, y: 0, zoom: 1 };
		this.createdAt = "";
		this.updatedAt = "";
		this.load(doc);
	}

	/** 用一份（可能不可信的）文档替换全部内容 */
	load(doc) {
		const clean = sanitizeDocument(doc);
		this.nodes = clean.nodes;
		this.edges = clean.edges;
		this.settings = clean.settings;
		this.name = clean.name;
		this.viewport = clean.viewport;
		this.createdAt = clean.createdAt;
		this.updatedAt = clean.updatedAt;
		return this;
	}

	/** 导出为纯数据文档 */
	toJSON() {
		return {
			version: 1,
			name: this.name,
			nodes: this.nodes.map((n) => ({ ...n, ports: normalizePorts(n.ports) })),
			edges: this.edges.map((e) => ({ ...e, from: { ...e.from }, to: { ...e.to } })),
			settings: { ...this.settings },
			viewport: { ...this.viewport },
			createdAt: this.createdAt,
			updatedAt: this.updatedAt,
		};
	}

	// ------------------------------------------------------------ 节点

	getNode(id) {
		return this.nodes.find((n) => n.id === id);
	}

	hasNode(id) {
		return this.nodes.some((n) => n.id === id);
	}

	/** 新建节点；data 可覆盖预设字段 */
	addNode(data = {}) {
		const preset = getNodeType(data.type);
		const node = {
			id: data.id || nodeId(),
			type: data.type || preset.key,
			x: numOr(data.x, 0),
			y: numOr(data.y, 0),
			w: clamp(numOr(data.w, preset.w), MIN_NODE_W, MAX_NODE_W),
			h: clamp(numOr(data.h, preset.h), MIN_NODE_H, MAX_NODE_H),
			title: typeof data.title === "string" ? data.title : preset.title,
			text: typeof data.text === "string" ? data.text : preset.text,
			color: typeof data.color === "string" && data.color ? data.color : preset.color,
			shape: typeof data.shape === "string" && data.shape ? data.shape : preset.shape,
			fontSize: numOr(data.fontSize, preset.fontSize),
			ports: normalizePorts(data.ports || defaultPorts()),
			locked: data.locked === true,
			collapsed: data.collapsed === true,
		};
		node.w = Math.round(node.w);
		node.h = Math.round(node.h);
		this.nodes.push(node);
		return node;
	}

	/** 批量删除节点，并连带删除所有入射/出射连线 */
	removeNodes(ids) {
		const set = new Set(ids);
		if (set.size === 0) return { nodes: 0, edges: 0 };
		const before = this.nodes.length;
		this.nodes = this.nodes.filter((n) => !set.has(n.id));
		const beforeEdges = this.edges.length;
		this.edges = this.edges.filter((e) => !set.has(e.from.node) && !set.has(e.to.node));
		if (this.nodes.length !== before) this.touch();
		return { nodes: before - this.nodes.length, edges: beforeEdges - this.edges.length };
	}

	updateNode(id, patch) {
		const node = this.getNode(id);
		if (!node) return null;
		applyNodePatch(node, patch);
		this.touch();
		return node;
	}

	// ------------------------------------------------------------ 端口

	/** 设置某侧某方向的端口数量；会自动收敛受影响连线 */
	setPortCount(nodeIdValue, dir, side, count) {
		const node = this.getNode(nodeIdValue);
		if (!node || !SIDES.includes(side) || (dir !== "in" && dir !== "out")) return null;
		const next = clamp(Math.floor(numOr(count, 0)), 0, 64);
		node.ports = normalizePorts(node.ports);
		if (node.ports[dir][side] === next) return node;
		node.ports[dir][side] = next;
		this.reconcileEdges();
		this.touch();
		return node;
	}

	addPort(nodeIdValue, dir, side) {
		const node = this.getNode(nodeIdValue);
		if (!node) return null;
		return this.setPortCount(nodeIdValue, dir, side, portCount(node, dir, side) + 1);
	}

	removePort(nodeIdValue, dir, side) {
		const node = this.getNode(nodeIdValue);
		if (!node) return null;
		const count = portCount(node, dir, side);
		if (count <= 0) return node;
		return this.setPortCount(nodeIdValue, dir, side, count - 1);
	}

	/** 端口数量变化后修正连线：越界下标收敛，无端口可用的连线删除 */
	reconcileEdges() {
		const getNode = (id) => this.getNode(id);
		const drop = reconcileEdgesForPorts(this.edges, getNode);
		if (drop.length) {
			const set = new Set(drop);
			this.edges = this.edges.filter((e) => !set.has(e.id));
		}
		// 端点节点已消失的连线（节点被删时已处理，这里兜底）
		this.edges = this.edges.filter((e) => getNode(e.from.node) && getNode(e.to.node));
		return drop.length;
	}

	// ------------------------------------------------------------ 连线

	addEdge(data) {
		const edge = {
			id: data.id || edgeId(),
			from: { ...data.from },
			to: { ...data.to },
			style: data.style || "bezier",
			arrow: data.arrow || "to",
			dashed: data.dashed === true,
			label: typeof data.label === "string" ? data.label : "",
			color: data.color || null,
		};
		this.edges.push(edge);
		this.touch();
		return edge;
	}

	getEdge(id) {
		return this.edges.find((e) => e.id === id);
	}

	removeEdges(ids) {
		const set = new Set(ids);
		if (set.size === 0) return 0;
		const before = this.edges.length;
		this.edges = this.edges.filter((e) => !set.has(e.id));
		if (this.edges.length !== before) this.touch();
		return before - this.edges.length;
	}

	updateEdge(id, patch) {
		const edge = this.getEdge(id);
		if (!edge) return null;
		if (typeof patch.style === "string") edge.style = patch.style;
		if (typeof patch.arrow === "string") edge.arrow = patch.arrow;
		if (typeof patch.dashed === "boolean") edge.dashed = patch.dashed;
		if (typeof patch.label === "string") edge.label = patch.label;
		if ("color" in patch) edge.color = patch.color || null;
		this.touch();
		return edge;
	}

	/**
	 * 反转一条连线（流向对调，但恒保持 out → in）。
	 *
	 * 不能简单地把 from/to 互换——那会得到一条 in → out 的连线，
	 * 既违反不变量，又会在保存/撤销回放时被 sanitizeDocument 当成非法连线丢弃。
	 * 正确做法：在「原头部所在节点」上取同侧的输出口作为新尾部，在「原尾部所在节点」上
	 * 取同侧的输入口作为新头部，于是 A.right(out) → B.left(in) 变成 B.left(out) → A.right(in)：
	 * 连线走廊不变、箭头调头。
	 *
	 * @returns {object|null} 反转后的连线；任一端找不到可用端口时返回 null（不修改）
	 */
	reverseEdge(id) {
		const edge = this.getEdge(id);
		if (!edge) return null;
		const getNode = (nodeId) => this.getNode(nodeId);
		const from = invertPortRef(getNode, edge.to, "out");
		const to = invertPortRef(getNode, edge.from, "in");
		if (!from || !to) return null;
		edge.from = from;
		edge.to = to;
		this.touch();
		return edge;
	}

	/** 与某节点相连的全部连线 */
	edgesOf(nodeIdValue) {
		return this.edges.filter((e) => e.from.node === nodeIdValue || e.to.node === nodeIdValue);
	}

	// ------------------------------------------------------------ 查询

	nodesInRect(rect) {
		return this.nodes.filter((n) => rectIntersects(rectOf(n), rect));
	}

	/** 全部节点包围盒；空图返回 null */
	bounds() {
		let box = null;
		for (const node of this.nodes) box = rectUnion(box, rectOf(node));
		return box;
	}

	/** 全部连线端点也纳入包围盒（自环会伸出节点外） */
	boundsWithEdges(routeOf) {
		let box = this.bounds();
		if (typeof routeOf !== "function") return box;
		for (const edge of this.edges) {
			const route = routeOf(edge);
			if (!route) continue;
			for (const p of route.points) {
				box = rectUnion(box, { x: p.x, y: p.y, w: 0, h: 0 });
			}
		}
		return box;
	}

	/** 统计某端口的连接数（端口角标提示用） */
	degree(ref) {
		return this.edges.filter(
			(e) => (e.from.node === ref.node && e.from.dir === ref.dir && e.from.side === ref.side && e.from.index === ref.index)
				|| (e.to.node === ref.node && e.to.dir === ref.dir && e.to.side === ref.side && e.to.index === ref.index),
		).length;
	}

	stats() {
		return { nodes: this.nodes.length, edges: this.edges.length };
	}

	touch() {
		this.updatedAt = new Date().toISOString();
	}
}

function applyNodePatch(node, patch) {	if (typeof patch.title === "string") node.title = patch.title;
	if (typeof patch.text === "string") node.text = patch.text;
	if (typeof patch.color === "string" && patch.color) node.color = patch.color;
	if (typeof patch.shape === "string") node.shape = patch.shape;
	if (Number.isFinite(patch.fontSize)) node.fontSize = patch.fontSize;
	if (Number.isFinite(patch.x)) node.x = Math.round(patch.x);
	if (Number.isFinite(patch.y)) node.y = Math.round(patch.y);
	if (Number.isFinite(patch.w)) node.w = Math.round(clamp(patch.w, MIN_NODE_W, MAX_NODE_W));
	if (Number.isFinite(patch.h)) node.h = Math.round(clamp(patch.h, MIN_NODE_H, MAX_NODE_H));
	if (typeof patch.locked === "boolean") node.locked = patch.locked;
	if (typeof patch.collapsed === "boolean") node.collapsed = patch.collapsed;
	if (patch.ports) node.ports = normalizePorts(patch.ports);
	node.updatedAt = new Date().toISOString();
}

function numOr(value, fallback) {
	const v = Number(value);
	return Number.isFinite(v) ? v : fallback;
}

function clamp(v, lo, hi) {
	return Math.min(hi, Math.max(lo, v));
}
