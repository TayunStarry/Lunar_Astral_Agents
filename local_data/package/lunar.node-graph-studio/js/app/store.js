/**
 * 应用状态中心：图数据 + 选择集 + 视口 + 瞬时交互态，配一个极简主题事件总线。
 *
 * 主题（topic）约定：
 *   graph     图内容变化（节点/连线增删改）→ 重绘节点层与连线层
 *   selection 选择集变化                      → 更新选中样式与检查器
 *   viewport  平移/缩放变化                   → 更新世界变换、栅格、缩略图
 *   ui        瞬时交互态（悬停/参考线/框选/模式）→ 更新叠加层与状态栏
 *   settings  设置变化（栅格/吸附/缩略图）      → 工具栏与状态栏
 *   document  文档名/脏标记/文档列表变化        → 状态栏与文档列表
 */

import { Graph } from "../model/graph.js";
import { cloneState } from "../model/history.js";
import { rectOf, rectUnion, rectIntersects } from "../core/geometry.js";

export class Store {
	constructor(doc) {
		this.graph = new Graph(doc);
		this.viewport = { ...this.graph.viewport };
		this.selection = { nodes: new Set(), edges: new Set() };
		this.ui = {
			mode: "idle",            // idle | dragNode | dragWire | marquee | pan | resize
			wireStyle: this.graph.settings.defaultWireStyle,
			arrowMode: this.graph.settings.defaultArrowMode,
			hoverNode: null,
			hoverPort: null,
			activePort: null,        // 拖线起点端口引用
			activePortIsInput: false,
			wirePreview: null,       // 世界坐标
			guides: [],
			marquee: null,           // 屏幕坐标矩形
			editing: null,           // {kind:'title'|'text'|'label', id}
			cursor: { x: 0, y: 0 },  // 世界坐标
			dirty: false,
		};
		this._listeners = new Map();
	}

	// ------------------------------------------------------------ 事件

	on(topic, handler) {
		if (!this._listeners.has(topic)) this._listeners.set(topic, new Set());
		this._listeners.get(topic).add(handler);
		return () => this.off(topic, handler);
	}

	off(topic, handler) {
		const set = this._listeners.get(topic);
		if (set) set.delete(handler);
	}

	emit(topic, payload) {
		const set = this._listeners.get(topic);
		if (!set) return;
		for (const handler of [...set]) {
			try {
				handler(payload);
			} catch (err) {
				console.error(`[store] ${topic} 订阅者异常:`, err);
			}
		}
	}

	// ------------------------------------------------------------ 选择

	get selectionCount() {
		return this.selection.nodes.size + this.selection.edges.size;
	}

	isNodeSelected(id) {
		return this.selection.nodes.has(id);
	}

	isEdgeSelected(id) {
		return this.selection.edges.has(id);
	}

	/**
	 * 更新选择集。
	 * @param {{nodes?:Iterable<string>, edges?:Iterable<string>}} patch
	 * @param {"replace"|"add"|"toggle"|"remove"} [mode]
	 */
	select(patch, mode = "replace") {
		const nodes = patch.nodes ? [...patch.nodes] : [];
		const edges = patch.edges ? [...patch.edges] : [];

		if (mode === "replace") {
			const same = sameSet(this.selection.nodes, nodes) && sameSet(this.selection.edges, edges);
			if (same) return false;
			this.selection.nodes = new Set(nodes);
			this.selection.edges = new Set(edges);
		} else {
			for (const id of nodes) applyMembership(this.selection.nodes, id, mode);
			for (const id of edges) applyMembership(this.selection.edges, id, mode);
			// 选中连线时清掉节点，反之亦然（互斥，避免语义含混）
			if (nodes.length) this.selection.edges.clear();
			if (edges.length) this.selection.nodes.clear();
		}
		this.emit("selection");
		return true;
	}

	clearSelection() {
		if (this.selectionCount === 0) return false;
		this.selection.nodes.clear();
		this.selection.edges.clear();
		this.emit("selection");
		return true;
	}

	selectAll() {
		this.selection.nodes = new Set(this.graph.nodes.filter((n) => !n.locked).map((n) => n.id));
		this.selection.edges = new Set();
		this.emit("selection");
	}

	/** 丢弃已不存在的选择项（撤销/删除后调用） */
	pruneSelection() {
		let changed = false;
		for (const id of [...this.selection.nodes]) {
			if (!this.graph.hasNode(id)) {
				this.selection.nodes.delete(id);
				changed = true;
			}
		}
		for (const id of [...this.selection.edges]) {
			if (!this.graph.getEdge(id)) {
				this.selection.edges.delete(id);
				changed = true;
			}
		}
		// 连线两端被删时，Graph 已删除该连线，这里同步选择集
		for (const id of [...this.selection.edges]) {
			const edge = this.graph.getEdge(id);
			if (!edge || !this.graph.hasNode(edge.from.node) || !this.graph.hasNode(edge.to.node)) {
				this.selection.edges.delete(id);
				changed = true;
			}
		}
		if (changed) this.emit("selection");
		return changed;
	}

	selectedNodes() {
		return this.graph.nodes.filter((n) => this.selection.nodes.has(n.id));
	}

	selectedEdges() {
		return this.graph.edges.filter((e) => this.selection.edges.has(e.id));
	}

	/** 选择集包围盒（世界坐标） */
	selectionBounds() {
		let box = null;
		for (const node of this.selectedNodes()) box = rectUnion(box, rectOf(node));
		return box;
	}

	/** 与给定世界矩形相交的节点 id */
	nodeIdsInRect(rect) {
		return this.graph.nodes.filter((n) => rectIntersects(rectOf(n), rect)).map((n) => n.id);
	}

	// ------------------------------------------------------------ 视口 / 设置

	setViewport(patch) {
		let changed = false;
		if (Number.isFinite(patch.x) && patch.x !== this.viewport.x) {
			this.viewport.x = patch.x;
			changed = true;
		}
		if (Number.isFinite(patch.y) && patch.y !== this.viewport.y) {
			this.viewport.y = patch.y;
			changed = true;
		}
		if (Number.isFinite(patch.zoom)) {
			const z = Math.min(4, Math.max(0.15, patch.zoom));
			if (z !== this.viewport.zoom) {
				this.viewport.zoom = z;
				changed = true;
			}
		}
		if (changed) this.emit("viewport");
		return changed;
	}

	get settings() {
		return this.graph.settings;
	}

	setSettings(patch) {
		let changed = false;
		for (const [key, value] of Object.entries(patch)) {
			if (this.graph.settings[key] !== value) {
				this.graph.settings[key] = value;
				changed = true;
			}
		}
		if (changed) {
			this.emit("settings");
			this.markDirty();
		}
		return changed;
	}

	setUI(patch, emit = true) {
		let changed = false;
		for (const [key, value] of Object.entries(patch)) {
			if (this.ui[key] !== value) {
				this.ui[key] = value;
				changed = true;
			}
		}
		if (changed && emit) this.emit("ui", patch);
		return changed;
	}

	markDirty(dirty = true) {
		if (this.ui.dirty === dirty) return;
		this.ui.dirty = dirty;
		this.emit("document");
	}

	// ------------------------------------------------------------ 快照（供历史）

	/** 仅节点与连线参与撤销栈（视图/设置不进历史） */
	snapshot() {
		return cloneState({ nodes: this.graph.nodes, edges: this.graph.edges });
	}

	restore(state) {
		if (!state) return;
		const doc = this.graph.toJSON();
		doc.nodes = state.nodes;
		doc.edges = state.edges;
		this.graph.load(doc);
		this.pruneSelection();
		this.markDirty(true);
		this.emit("graph");
		this.emit("selection");
	}

	/** 完整文档（保存/导出用） */
	snapshotDocument() {
		this.graph.viewport = { ...this.viewport };
		return this.graph.toJSON();
	}

	loadDocument(doc) {
		this.graph.load(doc);
		this.viewport = { ...this.graph.viewport };
		this.selection.nodes.clear();
		this.selection.edges.clear();
		this.ui.editing = null;
		this.ui.hoverPort = null;
		this.ui.activePort = null;
		this.ui.wirePreview = null;
		this.ui.guides = [];
		this.ui.marquee = null;
		this.markDirty(false);
		this.emit("graph");
		this.emit("selection");
		this.emit("viewport");
		this.emit("settings");
		this.emit("document");
	}
}

function applyMembership(set, id, mode) {
	if (mode === "add") set.add(id);
	else if (mode === "remove") set.delete(id);
	else if (mode === "toggle") {
		if (set.has(id)) set.delete(id);
		else set.add(id);
	}
}

function sameSet(set, list) {
	if (set.size !== list.length) return false;
	for (const id of list) if (!set.has(id)) return false;
	return true;
}
