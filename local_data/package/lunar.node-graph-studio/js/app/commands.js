/**
 * 命令层：所有会改动节点图的操作都从这里走。
 *
 * 每个命令都是「改动 → 提交历史 → 广播重绘」三段式：
 *   · 改动只经 Graph；· 历史用整体快照（见 model/history.js）；
 *   · 广播经 Store，视图层只订阅、不反向改数据。
 *
 * 交互过程（拖拽/缩放）为了避免每帧入栈，直接改 Graph 后由交互层在结束时
 * 调用 commit(label) 一次性入栈 —— 因此本类对外暴露 commit()。
 */

import { edgeExists, invertPortRef, resolveConnection } from "../core/ports.js";
import { layeredLayout } from "../model/layout.js";
import { getNodeType } from "../model/palette.js";
import { rectOf } from "../core/geometry.js";
import { emptyDocument } from "../model/document.js";

const ALIGN_MODES = ["left", "centerX", "right", "top", "centerY", "bottom", "distributeX", "distributeY"];

export const ALIGN_LABEL = {
	left: "左对齐",
	centerX: "水平居中",
	right: "右对齐",
	top: "顶对齐",
	centerY: "垂直居中",
	bottom: "底对齐",
	distributeX: "水平等距",
	distributeY: "垂直等距",
};

export class Commands {
	/**
	 * @param {import("./store.js").Store} store
	 * @param {import("../model/history.js").History} history
	 * @param {(message:string, kind?:string)=>void} notify
	 */
	constructor(store, history, notify = () => {}) {
		this.store = store;
		this.history = history;
		this.notify = notify;
		this.clipboard = null;
	}

	// ------------------------------------------------------------ 基础

	/** 提交当前状态到历史（拖拽结束、就地编辑结束等场景） */
	commit(label) {
		this.history.commit(label);
		this.store.markDirty(true);
	}

	/** 执行一次可撤销改动 */
	run(label, mutator) {
		const result = mutator();
		if (result !== false) this.commit(label);
		this.store.emit("graph");
		return result;
	}

	// ------------------------------------------------------------ 节点

	/** 在世界坐标处新建节点 */
	addNode(typeKey, world, extra = {}) {
		const preset = getNodeType(typeKey);
		const x = Number.isFinite(world?.x) ? world.x - preset.w / 2 : 0;
		const y = Number.isFinite(world?.y) ? world.y - preset.h / 2 : 0;
		let node = null;
		this.run(`新建「${preset.label}」节点`, () => {
			node = this.store.graph.addNode({ type: preset.key, x, y, ...extra });
		});
		if (node) this.store.select({ nodes: [node.id] }, "replace");
		return node;
	}

	/** 批量删除选中的节点与连线 */
	deleteSelection() {
		const { nodes, edges } = this.store.selection;
		if (nodes.size === 0 && edges.size === 0) {
			this.notify("没有选中任何内容", "warn");
			return;
		}
		const locked = [...nodes].filter((id) => this.store.graph.getNode(id)?.locked);
		const removable = [...nodes].filter((id) => !locked.includes(id));
		const nodeCount = removable.length;
		const edgeCount = edges.size;
		this.run(nodeCount + edgeCount > 1 ? `删除 ${nodeCount + edgeCount} 项` : "删除", () => {
			this.store.graph.removeEdges([...edges]);
			this.store.graph.removeNodes(removable);
		});
		this.store.clearSelection();
		if (locked.length) this.notify(`${locked.length} 个锁定节点已跳过`, "warn");
	}

	/** 改节点字段（标题/正文/颜色/形状/字号/位置/尺寸/锁定等） */
	patchNodes(ids, patch, label = "修改节点") {
		const list = [...ids].filter((id) => this.store.graph.hasNode(id));
		if (list.length === 0) return;
		this.run(label, () => {
			for (const id of list) this.store.graph.updateNode(id, patch);
		});
	}

	/** 拖动过程中的位移（不入栈） */
	translateNodes(ids, dx, dy) {
		if (!dx && !dy) return;
		for (const id of ids) {
			const node = this.store.graph.getNode(id);
			if (!node || node.locked) continue;
			node.x = Math.round(node.x + dx);
			node.y = Math.round(node.y + dy);
		}
	}

	setNodeSize(id, w, h) {
		const node = this.store.graph.getNode(id);
		if (!node) return;
		node.w = Math.round(w);
		node.h = Math.round(h);
	}

	toggleLock(ids) {
		const list = [...ids].map((id) => this.store.graph.getNode(id)).filter(Boolean);
		if (!list.length) return;
		const next = !list.every((n) => n.locked);
		this.patchNodes(ids, { locked: next }, next ? "锁定节点" : "解锁节点");
	}

	// ------------------------------------------------------------ 连线

	/**
	 * 连接两个端口。落点同向时自动补端口（规则见 core/ports.js）。
	 * @returns {boolean} 是否成功
	 */
	connect(fromRef, toRef) {
		const graph = this.store.graph;
		const decision = resolveConnection((id) => graph.getNode(id), graph.edges, fromRef, toRef);
		if (!decision.ok) {
			if (decision.reason !== "不能连接到自己") this.notify(decision.reason, "warn");
			return false;
		}
		let edge = null;
		this.run("连接节点", () => {
			if (decision.createdPort) {
				const cp = decision.createdPort;
				graph.setPortCount(cp.node, cp.dir, cp.side, cp.count);
			}
			edge = graph.addEdge({
				from: decision.from,
				to: decision.to,
				style: this.store.ui.wireStyle,
				arrow: this.store.ui.arrowMode,
			});
		});
		if (edge) this.store.select({ edges: [edge.id] }, "replace");
		if (decision.swapped) this.notify("已按方向自动调整为 输出 → 输入", "info");
		else if (decision.createdPort) this.notify("落点同向：已自动补一个反向端口", "info");
		return true;
	}

	/** 把已有连线的一端改接到新端口（拖拽连线端点） */
	reconnect(edgeId, end, ref) {
		const graph = this.store.graph;
		const edge = graph.getEdge(edgeId);
		if (!edge) return false;
		const next = { ...edge, [end]: ref };
		const decision = resolveConnection((id) => graph.getNode(id), graph.edges.filter((e) => e.id !== edgeId), next.from, next.to);
		if (!decision.ok) {
			this.notify(decision.reason, "warn");
			return false;
		}
		this.run("重接连线", () => {
			if (decision.createdPort) {
				const cp = decision.createdPort;
				graph.setPortCount(cp.node, cp.dir, cp.side, cp.count);
			}
			const target = graph.getEdge(edgeId);
			if (!target) return;
			target.from = decision.from;
			target.to = decision.to;
		});
		return true;
	}

	reverseEdge(id) {
		const edge = this.store.graph.getEdge(id);
		if (!edge) return;
		if (edge.from.dir !== "out" || edge.to.dir !== "in") {
			this.notify("该连线两端方向异常，无法翻转", "warn");
			return;
		}
		// 预演一次，确认能翻且不会与既有连线重复
		const graph = this.store.graph;
		const getNode = (nodeId) => graph.getNode(nodeId);
		const mirroredFrom = invertPortRef(getNode, edge.to, "out");
		const mirroredTo = invertPortRef(getNode, edge.from, "in");
		if (!mirroredFrom || !mirroredTo) {
			this.notify("该节点的同侧/对侧没有可用端口，无法翻转（可先补一个端口）", "warn");
			return;
		}
		const others = graph.edges.filter((e) => e.id !== id);
		if (edgeExists(others, mirroredFrom, mirroredTo)) {
			this.notify("翻转后会与既有连线重复，已取消", "warn");
			return;
		}
		this.run("翻转连线方向", () => {
			graph.reverseEdge(id);
		});
	}

	patchEdges(ids, patch, label = "修改连线") {
		const list = [...ids].filter((id) => this.store.graph.getEdge(id));
		if (list.length === 0) return;
		this.run(label, () => {
			for (const id of list) this.store.graph.updateEdge(id, patch);
		});
	}

	// ------------------------------------------------------------ 端口

	addPort(nodeId, dir, side) {
		const node = this.store.graph.getNode(nodeId);
		if (!node) return;
		this.run(`新增${side === "top" ? "上" : side === "bottom" ? "下" : side === "left" ? "左" : "右"}侧${dir === "in" ? "输入" : "输出"}端口`, () => {
			this.store.graph.addPort(nodeId, dir, side);
		});
	}

	removePort(nodeId, dir, side) {
		const node = this.store.graph.getNode(nodeId);
	if (!node) return;
		this.run("删除端口", () => {
			this.store.graph.removePort(nodeId, dir, side);
		});
		this.store.pruneSelection();
	}

	setPortCount(nodeId, dir, side, count) {
		this.run("调整端口数量", () => {
			this.store.graph.setPortCount(nodeId, dir, side, count);
		});
		this.store.pruneSelection();
	}

	// ------------------------------------------------------------ 复制粘贴

	copySelection() {
		const nodes = this.store.selectedNodes();
		if (nodes.length === 0) {
			this.notify("没有可复制的节点", "warn");
			return;
		}
		const ids = new Set(nodes.map((n) => n.id));
		const edges = this.store.graph.edges.filter((e) => ids.has(e.from.node) && ids.has(e.to.node));
		this.clipboard = JSON.parse(JSON.stringify({ nodes, edges }));
		this.notify(`已复制 ${nodes.length} 个节点`, "info");
	}

	cutSelection() {
		this.copySelection();
		this.deleteSelection();
	}

	/** 粘贴到世界坐标（at 为 null 时按固定偏移粘贴） */
	paste(at = null) {
		if (!this.clipboard || this.clipboard.nodes.length === 0) {
			this.notify("剪贴板为空", "warn");
			return;
		}
		const src = this.clipboard;
		// 以副本包围盒左上角为基准，决定落点
		const minX = Math.min(...src.nodes.map((n) => n.x));
		const minY = Math.min(...src.nodes.map((n) => n.y));
		const offset = at ? { x: at.x - minX, y: at.y - minY } : { x: 32, y: 32 };

		const idMap = new Map();
		const newIds = [];
		let newEdges = [];
		this.run("粘贴节点", () => {
			for (const node of src.nodes) {
				const copy = this.store.graph.addNode({
					...node,
					id: undefined,
					x: node.x + offset.x,
					y: node.y + offset.y,
					ports: JSON.parse(JSON.stringify(node.ports)),
				});
				idMap.set(node.id, copy.id);
				newIds.push(copy.id);
			}
			for (const edge of src.edges) {
				const from = idMap.get(edge.from.node);
				const to = idMap.get(edge.to.node);
				if (!from || !to) continue;
				newEdges.push(this.store.graph.addEdge({
					...edge,
					id: undefined,
					from: { ...edge.from, node: from },
					to: { ...edge.to, node: to },
				}).id);
			}
		});
		this.store.select({ nodes: newIds }, "replace");
		return { nodes: newIds, edges: newEdges };
	}

	duplicateSelection(offset = { x: 28, y: 28 }) {
		const nodes = this.store.selectedNodes();
		if (nodes.length === 0) {
			this.notify("没有可复制的节点", "warn");
			return;
		}
		const previous = this.clipboard;
		this.clipboard = JSON.parse(JSON.stringify({
			nodes,
			edges: this.store.graph.edges.filter((e) => {
				const ids = new Set(nodes.map((n) => n.id));
				return ids.has(e.from.node) && ids.has(e.to.node);
			}),
		}));
		this.paste(null);
		this.clipboard = previous;
		// 叠加自定义偏移
		const created = this.store.selectedNodes();
		const dx = offset.x - 32;
		const dy = offset.y - 32;
		if (dx || dy) {
			this.translateNodes(created.map((n) => n.id), dx, dy);
			this.commit("再制节点");
			this.store.emit("graph");
		}
	}

	// ------------------------------------------------------------ 对齐 / 分布 / 层序

	/** 对齐或等距分布（作用于选中的多个节点） */
	align(mode) {
		if (!ALIGN_MODES.includes(mode)) return;
		const nodes = this.store.selectedNodes().filter((n) => !n.locked);
		if (nodes.length < 2) {
			this.notify("请先选中至少 2 个节点", "warn");
			return;
		}
		const boxes = nodes.map((n) => rectOf(n));
		const left = Math.min(...boxes.map((b) => b.x));
		const right = Math.max(...boxes.map((b) => b.x + b.w));
		const top = Math.min(...boxes.map((b) => b.y));
		const bottom = Math.max(...boxes.map((b) => b.y + b.h));
		const centerX = (left + right) / 2;
		const centerY = (top + bottom) / 2;

		const moves = new Map();
		if (mode === "distributeX" || mode === "distributeY") {
			const horizontal = mode === "distributeX";
			const sorted = [...nodes].sort((a, b) => (horizontal ? a.x - b.x : a.y - b.y));
			if (sorted.length < 3) {
				this.notify("等距分布需要至少 3 个节点", "warn");
				return;
			}
			const totalSize = sorted.reduce((acc, n) => acc + (horizontal ? n.w : n.h), 0);
			const span = horizontal ? right - left : bottom - top;
			const gap = (span - totalSize) / (sorted.length - 1);
			let cursor = horizontal ? left : top;
			for (const node of sorted) {
				if (horizontal) moves.set(node.id, { x: cursor, y: node.y });
				else moves.set(node.id, { x: node.x, y: cursor });
				cursor += (horizontal ? node.w : node.h) + gap;
			}
		} else {
			for (const node of nodes) {
				let x = node.x;
				let y = node.y;
				switch (mode) {
					case "left": x = left; break;
					case "right": x = right - node.w; break;
					case "centerX": x = centerX - node.w / 2; break;
					case "top": y = top; break;
					case "bottom": y = bottom - node.h; break;
					case "centerY": y = centerY - node.h / 2; break;
					default: break;
				}
				moves.set(node.id, { x, y });
			}
		}

		this.run(ALIGN_LABEL[mode] || "对齐", () => {
			for (const [id, pos] of moves) {
				this.store.graph.updateNode(id, { x: Math.round(pos.x), y: Math.round(pos.y) });
			}
		});
	}

	bringToFront(ids) {
		const set = new Set(ids);
		const picked = this.store.graph.nodes.filter((n) => set.has(n.id));
		if (picked.length === 0) return;
		this.run("置于顶层", () => {
			const rest = this.store.graph.nodes.filter((n) => !set.has(n.id));
			this.store.graph.nodes = [...rest, ...picked];
		});
	}

	sendToBack(ids) {
		const set = new Set(ids);
		const picked = this.store.graph.nodes.filter((n) => set.has(n.id));
		if (picked.length === 0) return;
		this.run("置于底层", () => {
			const rest = this.store.graph.nodes.filter((n) => !set.has(n.id));
			this.store.graph.nodes = [...picked, ...rest];
		});
	}

	// ------------------------------------------------------------ 布局

	/** 依据连线方向自动分层排布全部节点（或选中节点） */
	autoLayout(direction = "TB", onlySelection = false) {
		const scope = onlySelection && this.store.selection.nodes.size > 1
			? this.store.selectedNodes()
			: this.store.graph.nodes;
		if (scope.length < 2) {
			this.notify("节点太少，无需自动布局", "warn");
			return;
		}
		const ids = new Set(scope.map((n) => n.id));
		const edges = this.store.graph.edges.filter((e) => ids.has(e.from.node) && ids.has(e.to.node));
		const box = scope.reduce((acc, n) => ({
			x: Math.min(acc.x, n.x),
			y: Math.min(acc.y, n.y),
		}), { x: Infinity, y: Infinity });
		const layout = layeredLayout(scope, edges, {
			direction,
			originX: Number.isFinite(box.x) ? box.x : 0,
			originY: Number.isFinite(box.y) ? box.y : 0,
		});
		this.run("自动布局", () => {
			for (const [id, pos] of layout) this.store.graph.updateNode(id, pos);
		});
	}

	// ------------------------------------------------------------ 历史 / 文档

	undo() {
		if (!this.history.canUndo()) {
			this.notify("没有可撤销的操作", "warn");
			return;
		}
		const label = this.history.undoLabel();
		this.history.undo();
		this.store.pruneSelection();
		this.store.markDirty(true);
		this.store.emit("graph");
		this.notify(`已撤销：${label}`, "info");
	}

	redo() {
		if (!this.history.canRedo()) {
			this.notify("没有可重做的操作", "warn");
			return;
		}
		const label = this.history.redoLabel();
		this.history.redo();
		this.store.pruneSelection();
		this.store.markDirty(true);
		this.store.emit("graph");
		this.notify(`已重做：${label}`, "info");
	}

	/** 新建文档（内容清空，历史重置） */
	newDocument(name = "未命名节点图") {
		this.store.loadDocument(emptyDocument(name));
		this.history.reset();
		this.store.emit("document");
	}

	/** 载入文档（来自磁盘/导入/示例） */
	loadDocument(doc) {
		this.store.loadDocument(doc);
		this.history.reset();
		this.store.emit("document");
	}

	/** 用示例内容填充当前空文档，让用户一进来就能看到连线方向与四向端口 */
	loadSample() {
		this.loadDocument(buildSampleDocument());
		this.notify("已载入示例节点图", "info");
	}
}

/**
 * 示例文档：覆盖「上下左右四个方向」的端口连线与三种线型，
 * 也是人工验收「连线方向显示」的现成样本。
 */
export function buildSampleDocument() {
	const base = emptyDocument("示例：四向连线");
	const r = { x: 120, y: 240, w: 200, h: 96 };
	const nodes = [
		{ id: "n_start", type: "start", x: r.x, y: r.y, title: "开始", text: "双击可编辑文字" },
		{ id: "n_fetch", type: "io", x: r.x + 360, y: 96, title: "读取输入", text: "从上方端口接入\n从下方端口送出" },
		{ id: "n_check", type: "decision", x: r.x + 360, y: 440, title: "校验通过？", text: "是 → 右侧\n否 → 下方" },
		{ id: "n_left", type: "data", x: r.x + 720, y: 440, title: "左侧入线", text: "从本节点左侧输入" },
		{ id: "n_right", type: "process", x: r.x + 720, y: 108, title: "右侧出线", text: "从本节点右侧输出\n（箭头指向被连节点）" },
		{ id: "n_note", type: "note", x: r.x + 40, y: 460, title: "说明", text: "每个节点的上下左右四侧都带输入口与输出口，可向任意方向连线。" },
	];
	base.nodes = nodes.map((n) => ({ ...n }));
	base.edges = [
		{ id: "e1", from: { node: "n_start", dir: "out", side: "right", index: 0 }, to: { node: "n_fetch", dir: "in", side: "left", index: 0 }, style: "bezier", arrow: "to", label: "曲线" },
		{ id: "e2", from: { node: "n_fetch", dir: "out", side: "bottom", index: 0 }, to: { node: "n_check", dir: "in", side: "top", index: 0 }, style: "ortho", arrow: "to", label: "直角（下→上）" },
		{ id: "e3", from: { node: "n_check", dir: "out", side: "bottom", index: 0 }, to: { node: "n_left", dir: "in", side: "bottom", index: 0 }, style: "bezier", arrow: "both", label: "双向", dashed: true },
		{ id: "e4", from: { node: "n_left", dir: "out", side: "right", index: 0 }, to: { node: "n_right", dir: "in", side: "right", index: 0 }, style: "straight", arrow: "to", label: "直线（右→右）" },
		{ id: "e5", from: { node: "n_right", dir: "out", side: "top", index: 0 }, to: { node: "n_start", dir: "in", side: "top", index: 0 }, style: "bezier", arrow: "to", label: "回环（上→上）" },
	];
	base.viewport = { x: 60, y: 40, zoom: 0.85 };
	return base;
}
