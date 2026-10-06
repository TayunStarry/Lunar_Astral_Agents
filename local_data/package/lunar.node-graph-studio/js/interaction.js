/**
 * 画布交互状态机。
 *
 * 一个指针（鼠标/触控/笔）从按下到松开只处于一种模式：
 *   dragNode 拖动节点   dragWire 拖出连线   marquee 框选   pan 平移   resize 调整尺寸
 * 模式在 pointerdown 时由「命中什么元素」决定，在 pointerup 时收尾并对可撤销的操作
 * 提交一次历史（拖拽过程本身不入栈，避免历史被每帧抖动填满）。
 *
 * 本次任务的关键行为都在这里落地：
 *   · 节点可自由拖放（多选整组移动、Shift 单轴、Alt 关吸附、Ctrl 抓取对齐参考线）
 *   · 任意端口起拖都能连线（上下左右四向），落在空白处还能「顺手新建节点并连上」
 */

import { rectFromPoints, clamp } from "./core/geometry.js";
import { computeAlignSnap, snapToGrid } from "./core/snap.js";
import { SIDES, portCount, resolveConnection } from "./core/ports.js";
import { MIN_NODE_W, MIN_NODE_H, NODE_TYPES } from "./model/palette.js";
import { showMenu } from "./ui/contextmenu.js";

const CLICK_TOLERANCE = 3;

export class Interaction {
	/**
	 * @param {object} deps
	 * @param {import("./app/store.js").Store} deps.store
	 * @param {import("./view/viewport.js").Viewport} deps.viewport
	 * @param {import("./view/renderer.js").Renderer} deps.renderer
	 * @param {import("./app/commands.js").Commands} deps.commands
	 * @param {import("./view/textedit.js").TextEditor} deps.textEditor
	 * @param {()=>void} deps.render
	 * @param {(items:Array, x:number, y:number)=>void} [deps.showMenu]
	 */
	constructor({ store, viewport, renderer, commands, textEditor, render, showMenu: menuFn }) {
		this.store = store;
		this.viewport = viewport;
		this.renderer = renderer;
		this.commands = commands;
		this.textEditor = textEditor;
		this.render = render;
		this.showMenu = menuFn || showMenu;

		this.canvas = viewport.canvas;
		this.state = null;
		this.spaceHeld = false;
		this.lastPointer = { x: 0, y: 0 };

		this.bind();
	}

	bind() {
		const canvas = this.canvas;
		canvas.addEventListener("pointerdown", (event) => this.onPointerDown(event));
		canvas.addEventListener("pointermove", (event) => this.onPointerMove(event));
		canvas.addEventListener("pointerup", (event) => this.onPointerUp(event));
		canvas.addEventListener("pointercancel", (event) => this.onPointerUp(event));
		canvas.addEventListener("wheel", (event) => this.onWheel(event), { passive: false });
		canvas.addEventListener("contextmenu", (event) => this.onContextMenu(event));
		canvas.addEventListener("dblclick", (event) => this.onDoubleClick(event));

		// 拖放新建：左栏节点类型拖进画布
		canvas.addEventListener("dragover", (event) => {
			const types = event.dataTransfer ? event.dataTransfer.types : [];
			if (Array.prototype.includes.call(types, "application/x-node-type")) {
				event.preventDefault();
				event.dataTransfer.dropEffect = "copy";
			}
		});
		canvas.addEventListener("drop", (event) => this.onDrop(event));

		// 空格 + 左键 = 平移（与设计软件一致）
		window.addEventListener("keydown", (event) => {
			if (event.code === "Space" && !isTextInput(event.target)) {
				this.spaceHeld = true;
				this.canvas.classList.add("is-pan-ready");
				event.preventDefault();
			}
		});
		window.addEventListener("keyup", (event) => {
			if (event.code === "Space") {
				this.spaceHeld = false;
				this.canvas.classList.remove("is-pan-ready");
			}
		});
		window.addEventListener("blur", () => {
			this.spaceHeld = false;
			this.canvas.classList.remove("is-pan-ready");
		});
	}

	// ------------------------------------------------------------ 指针按下

	onPointerDown(event) {
		if (event.button === 2) return; // 右键交给 contextmenu
		this.canvas.focus({ preventScroll: true });
		// 点击编辑器/浮层不参与画布交互
		if (event.target instanceof Element && event.target.closest(".node-editor, .edge-label-editor, .context-menu, .menu")) return;

		const point = this.viewport.screenToWorld(event.clientX, event.clientY);
		this.lastPointer = { x: event.clientX, y: event.clientY };
		capturePointer(this.canvas, event.pointerId);

		// 平移优先：中键 / 空格 / 只按在空白处的右键拖动
		if (event.button === 1 || this.spaceHeld) {
			this.beginPan(event);
			return;
		}

		const target = event.target instanceof Element ? event.target : null;
		const portEl = target?.closest(".port");
		if (portEl) {
			this.beginWire(event, portEl);
			return;
		}
		const resizeEl = target?.closest(".node-resize");
		if (resizeEl) {
			this.beginResize(event, resizeEl.closest(".node"));
			return;
		}
		const toolEl = target?.closest(".node-tool");
		if (toolEl) {
			const nodeEl = toolEl.closest(".node");
			const node = nodeEl ? this.store.graph.getNode(nodeEl.dataset.nodeId) : null;
			if (node) {
				// 折叠/展开要连带调整高度，交由组合根统一处理（需要渲染器测量文字高度）
				if (this.onToggleCollapse) this.onToggleCollapse(node.id);
				else this.commands.patchNodes([node.id], { collapsed: !node.collapsed }, node.collapsed ? "展开节点" : "折叠节点");
				this.store.emit("graph");
			}
			return;
		}
		const edgeEl = target?.closest(".edge") || target?.closest(".edge-label");
		if (edgeEl) {
			const edgeId = edgeEl.dataset.edgeId;
			if (edgeId) {
				if (event.shiftKey) this.store.select({ edges: [edgeId] }, "add");
				else if (!this.store.isEdgeSelected(edgeId)) this.store.select({ edges: [edgeId] }, "replace");
				this.state = { mode: "idle", edgeId };
				return;
			}
		}
		const nodeEl = target?.closest(".node");
		if (nodeEl) {
			this.beginNodeDrag(event, nodeEl, point);
			return;
		}
		this.beginMarquee(event);
	}

	beginPan(event) {
		this.state = {
			mode: "pan",
			pointerId: event.pointerId,
			startClient: { x: event.clientX, y: event.clientY },
			startViewport: { ...this.store.viewport },
		};
		this.canvas.classList.add("is-panning");
		this.store.setUI({ mode: "pan" });
	}

	beginWire(event, portEl) {
		const ref = {
			node: portEl.dataset.nodeId,
			dir: portEl.dataset.dir,
			side: portEl.dataset.side,
			index: Number(portEl.dataset.index),
		};
		const world = this.viewport.screenToWorld(event.clientX, event.clientY);
		this.state = { mode: "dragWire", pointerId: event.pointerId, anchor: ref, moved: false };
		this.store.setUI({
			mode: "dragWire",
			activePort: ref,
			activePortIsInput: ref.dir === "in",
			wirePreview: world,
			hoverPort: null,
		});
		this.store.emit("ui");
		this.render();
	}

	beginResize(event, nodeEl) {
		const node = nodeEl ? this.store.graph.getNode(nodeEl.dataset.nodeId) : null;
		if (!node) return;
		this.state = {
			mode: "resize",
			pointerId: event.pointerId,
			nodeId: node.id,
			startClient: { x: event.clientX, y: event.clientY },
			startSize: { w: node.w, h: node.h },
		};
		this.store.setUI({ mode: "resize" });
	}

	beginNodeDrag(event, nodeEl, point) {
		const nodeId = nodeEl.dataset.nodeId;
		const node = this.store.graph.getNode(nodeId);
		if (!node) return;

		// 选择语义：已选中项保持（可整组拖动）；Shift 追加/取消；否则单选
		if (event.shiftKey) {
			this.store.select({ nodes: [nodeId] }, "toggle");
		} else if (!this.store.isNodeSelected(nodeId)) {
			this.store.select({ nodes: [nodeId] }, "replace");
		}

		const ids = this.store.selection.nodes.size ? [...this.store.selection.nodes] : [nodeId];
		const origins = new Map();
		for (const id of ids) {
			const n = this.store.graph.getNode(id);
			if (n) origins.set(id, { x: n.x, y: n.y });
		}
		this.state = {
			mode: "dragNode",
			pointerId: event.pointerId,
			primaryId: nodeId,
			origins,
			startWorld: point,
			moved: false,
		};
		this.canvas.classList.add("is-node-dragging");
		this.store.setUI({ mode: "dragNode" });
	}

	beginMarquee(event) {
		const rect = this.canvas.getBoundingClientRect();
		this.state = {
			mode: "marquee",
			pointerId: event.pointerId,
			startClient: { x: event.clientX, y: event.clientY },
			additive: event.shiftKey,
			baseNodes: event.shiftKey ? [...this.store.selection.nodes] : [],
			canvasOrigin: { x: rect.left, y: rect.top },
		};
		if (!event.shiftKey) this.store.clearSelection();
		this.store.setUI({ mode: "marquee" });
	}

	// ------------------------------------------------------------ 指针移动

	onPointerMove(event) {
		const world = this.viewport.screenToWorld(event.clientX, event.clientY);
		this.store.ui.cursor = world;

		if (!this.state) {
			this.store.emit("ui", { cursor: true });
			return;
		}

		switch (this.state.mode) {
			case "pan":
				this.updatePan(event);
				break;
			case "dragNode":
				this.updateNodeDrag(event, world);
				break;
			case "dragWire":
				this.updateWire(event, world);
				break;
			case "marquee":
				this.updateMarquee(event);
				break;
			case "resize":
				this.updateResize(event);
				break;
			default:
				break;
		}
	}

	updatePan(event) {
		const state = this.state;
		const dx = event.clientX - state.startClient.x;
		const dy = event.clientY - state.startClient.y;
		this.store.setViewport({ x: state.startViewport.x + dx, y: state.startViewport.y + dy });
	}

	updateNodeDrag(event, world) {
		const state = this.state;
		const settings = this.store.settings;
		let dx = world.x - state.startWorld.x;
		let dy = world.y - state.startWorld.y;
		if (Math.abs(dx) > CLICK_TOLERANCE || Math.abs(dy) > CLICK_TOLERANCE) state.moved = true;
		if (!state.moved) return;

		// Shift：沿位移较大的单轴移动
		if (event.shiftKey) {
			if (Math.abs(dx) >= Math.abs(dy)) dy = 0;
			else dx = 0;
		}

		const primaryOrigin = state.origins.get(state.primaryId) || { x: 0, y: 0 };
		let targetX = primaryOrigin.x + dx;
		let targetY = primaryOrigin.y + dy;

		const snapGrid = settings.snapGrid && !event.altKey;
		const snapAlign = settings.snapAlign && !event.altKey;

		if (snapGrid) {
			targetX = snapToGrid(targetX, settings.grid, true);
			targetY = snapToGrid(targetY, settings.grid, true);
		}

		let correctionX = targetX - (primaryOrigin.x + dx);
		let correctionY = targetY - (primaryOrigin.y + dy);

		// 对齐参考线：与其它节点对齐时给出更强的吸附
		if (snapAlign) {
			const primary = this.store.graph.getNode(state.primaryId);
			if (primary) {
				const draggedRect = {
					x: primaryOrigin.x + dx + correctionX,
					y: primaryOrigin.y + dy + correctionY,
					w: primary.w,
					h: primary.h,
				};
				const others = this.store.graph.nodes
					.filter((n) => !state.origins.has(n.id))
					.map((n) => ({ x: n.x, y: n.y, w: n.w, h: n.h }));
				const snap = computeAlignSnap(draggedRect, others, 6);
				correctionX += snap.dx;
				correctionY += snap.dy;
				this.store.ui.guides = snap.guides;
			}
		} else if (this.store.ui.guides.length) {
			this.store.ui.guides = [];
		}

		dx += correctionX;
		dy += correctionY;

		for (const [id, origin] of state.origins) {
			const node = this.store.graph.getNode(id);
			if (!node || node.locked) continue;
			node.x = Math.round(origin.x + dx);
			node.y = Math.round(origin.y + dy);
		}
		// 直接请求全量重绘：节点位置变了就必须重排（参考线由 renderGraph 内的瞬时态渲染一并处理）
		this.render();
	}

	updateWire(event, world) {
		const state = this.state;
		state.moved = true;
		this.store.ui.wirePreview = world;

		// 指针捕获期间不会自动派发 hover，手动做一次命中测试
		const hovered = this.hoveredPort(event.clientX, event.clientY);
		const previous = this.store.ui.hoverPort;
		const changed = !sameRef(previous, hovered);
		if (changed) this.store.ui.hoverPort = hovered;

		if (hovered) {
			const decision = resolveConnection(
				(id) => this.store.graph.getNode(id),
				this.store.graph.edges,
				state.anchor,
				hovered,
			);
			this.renderer.markDropTarget(hovered, decision.ok);
		} else if (changed) {
			this.renderer.markDropTarget(null, false);
		}
		this.render();
	}

	updateMarquee(event) {
		const state = this.state;
		const rect = this.canvas.getBoundingClientRect();
		const a = { x: state.startClient.x - rect.left, y: state.startClient.y - rect.top };
		const b = { x: event.clientX - rect.left, y: event.clientY - rect.top };
		const screenRect = rectFromPoints(a, b);
		this.store.ui.marquee = screenRect;

		const worldA = this.viewport.screenToWorld(state.startClient.x, state.startClient.y);
		const worldB = this.viewport.screenToWorld(event.clientX, event.clientY);
		const worldRect = rectFromPoints(worldA, worldB);
		const hits = this.store.nodeIdsInRect(worldRect);
		const ids = state.additive ? [...new Set([...state.baseNodes, ...hits])] : hits;
		this.store.selection.nodes = new Set(ids);
		this.store.selection.edges.clear();
		this.store.emit("selection");
		this.render();
	}

	updateResize(event) {
		const state = this.state;
		const node = this.store.graph.getNode(state.nodeId);
		if (!node) return;
		const scale = this.store.viewport.zoom;
		const dx = (event.clientX - state.startClient.x) / scale;
		const dy = (event.clientY - state.startClient.y) / scale;
		let w = clamp(state.startSize.w + dx, MIN_NODE_W, 2400);
		let h = clamp(state.startSize.h + dy, MIN_NODE_H, 1800);
		if (this.store.settings.snapGrid && !event.altKey) {
			w = snapToGrid(w, this.store.settings.grid, true);
			h = snapToGrid(h, this.store.settings.grid, true);
		}
		node.w = Math.round(w);
		node.h = Math.round(h);
		this.render();
	}

	// ------------------------------------------------------------ 指针松开

	onPointerUp(event) {
		const state = this.state;
		this.state = null;
		if (!state) return;
		releasePointer(this.canvas, event.pointerId);
		this.canvas.classList.remove("is-node-dragging", "is-panning");

		switch (state.mode) {
			case "dragNode":
				if (state.moved) this.commands.commit("移动节点");
				this.store.ui.guides = [];
				this.store.setUI({ mode: "idle" });
				this.store.emit("graph");
				break;
			case "dragWire":
				this.finishWire(event, state);
				break;
			case "marquee":
				this.store.ui.marquee = null;
				this.store.setUI({ mode: "idle" });
				break;
			case "pan":
				this.store.setUI({ mode: "idle" });
				break;
			case "resize":
				this.commands.commit("调整节点尺寸");
				this.store.setUI({ mode: "idle" });
				this.store.emit("graph");
				break;
			default:
				this.store.setUI({ mode: "idle" });
				break;
		}
		this.render();
	}

	finishWire(event, state) {
		const hovered = this.hoveredPort(event.clientX, event.clientY);
		this.renderer.markDropTarget(null, false);

		if (hovered) {
			this.commands.connect(state.anchor, hovered);
		} else {
			const world = this.viewport.screenToWorld(event.clientX, event.clientY);
			// 落在空白处：让用户顺手选一个节点类型，新建后自动连上
			this.openCreateMenu(world, state.anchor, event.clientX, event.clientY);
		}

		this.store.setUI({
			mode: "idle",
			activePort: null,
			activePortIsInput: false,
			wirePreview: null,
			hoverPort: null,
		});
		this.store.emit("ui");
	}

	/** 松手处不在端口上：弹出节点类型菜单，创建后自动连接 */
	openCreateMenu(world, anchorPort, clientX, clientY) {
		const items = NODE_TYPES.map((type) => ({
			label: `新建「${type.label}」节点并连上`,
			icon: type.icon,
			cmd: type.key,
			onSelect: (key) => this.createAndConnect(key, world, anchorPort),
		}));
		this.showMenu(items, clientX, clientY);
	}

	createAndConnect(typeKey, world, anchorPort) {
		const node = this.commands.addNode(typeKey, world);
		if (!node) return;
		if (anchorPort) {
			const ref = this.pickConnectablePort(anchorPort, node);
			if (ref) this.commands.connect(anchorPort, ref);
		}
		this.store.emit("graph");
		this.render();
	}

	/**
	 * 为新节点挑一个能连的端口：优先朝向来源的那一侧，方向相反者优先。
	 */
	pickConnectablePort(anchorPort, node) {
		const opposite = { top: "bottom", bottom: "top", left: "right", right: "left" };
		const preferredSides = [opposite[anchorPort.side], ...SIDES.filter((s) => s !== opposite[anchorPort.side])];
		const preferredDir = anchorPort.dir === "out" ? "in" : "out";
		const candidates = [];
		for (const side of preferredSides) {
			for (const dir of [preferredDir, preferredDir === "in" ? "out" : "in"]) {
				const count = portCount(node, dir, side);
				for (let i = 0; i < count; i++) candidates.push({ node: node.id, dir, side, index: i });
			}
		}
		for (const ref of candidates) {
			const decision = resolveConnection((id) => this.store.graph.getNode(id), this.store.graph.edges, anchorPort, ref);
			if (decision.ok || decision.createdPort) return ref;
		}
		return null;
	}

	// ------------------------------------------------------------ 滚轮 / 右键 / 双击

	onWheel(event) {
		event.preventDefault();
		if (event.shiftKey) {
			this.viewport.panBy(-event.deltaY, 0);
			return;
		}
		const factor = Math.exp(-event.deltaY * 0.0016);
		this.viewport.zoomAt(event.clientX, event.clientY, factor);
	}

	onContextMenu(event) {
		event.preventDefault();
		if (this.state) return;
		this.onContextMenuRequest?.(event);
	}

	onDoubleClick(event) {
		if (event.target instanceof Element && event.target.closest(".node-editor, .edge-label-editor")) return;
		const target = event.target instanceof Element ? event.target : null;

		const portEl = target?.closest(".port");
		if (portEl) {
			// 双击端口：为同侧同向再加一个端口，随后就能从新端口连线
			const nodeId = portEl.dataset.nodeId;
			this.commands.addPort(nodeId, portEl.dataset.dir, portEl.dataset.side);
			return;
		}
		const titleEl = target?.closest(".node-title");
		if (titleEl) {
			this.textEditor.beginTitle(titleEl.closest(".node").dataset.nodeId);
			return;
		}
		const textEl = target?.closest(".node-text");
		if (textEl) {
			this.textEditor.beginText(textEl.closest(".node").dataset.nodeId);
			return;
		}
		const nodeEl = target?.closest(".node");
		if (nodeEl) {
			this.textEditor.beginText(nodeEl.dataset.nodeId);
			return;
		}
		const edgeEl = target?.closest(".edge") || target?.closest(".edge-label");
		if (edgeEl && edgeEl.dataset.edgeId) {
			this.textEditor.beginLabel(edgeEl.dataset.edgeId);
			return;
		}
		// 空白处双击：新建默认类型节点
		const world = this.viewport.screenToWorld(event.clientX, event.clientY);
		this.openCreateMenu(world, null, event.clientX, event.clientY);
	}

	// ------------------------------------------------------------ 拖放新建

	onDrop(event) {
		const key = event.dataTransfer?.getData("application/x-node-type") || event.dataTransfer?.getData("text/plain");
		if (!key) return;
		event.preventDefault();
		const world = this.viewport.screenToWorld(event.clientX, event.clientY);
		this.commands.addNode(key, world);
		this.render();
	}

	// ------------------------------------------------------------ 命中辅助

	hoveredPort(clientX, clientY) {
		const element = document.elementFromPoint(clientX, clientY);
		const portEl = element instanceof Element ? element.closest(".port") : null;
		if (!portEl) return null;
		return {
			node: portEl.dataset.nodeId,
			dir: portEl.dataset.dir,
			side: portEl.dataset.side,
			index: Number(portEl.dataset.index),
		};
	}

	/** 世界坐标下的连线命中（供右键菜单选中连线） */
	hitEdgeAt(clientX, clientY) {
		const world = this.viewport.screenToWorld(clientX, clientY);
		const tolerance = 10 / this.store.viewport.zoom;
		return this.renderer.hitEdge(world, tolerance);
	}

	/** 世界坐标下的节点命中（DOM 顺序即层序，取最上面一个） */
	hitNodeAt(clientX, clientY) {
		const element = document.elementFromPoint(clientX, clientY);
		const nodeEl = element instanceof Element ? element.closest(".node") : null;
		return nodeEl ? nodeEl.dataset.nodeId : null;
	}
}

function sameRef(a, b) {
	if (!a && !b) return true;
	if (!a || !b) return false;
	return a.node === b.node && a.dir === b.dir && a.side === b.side && a.index === b.index;
}

/**
 * 指针捕获的安全包装：合成事件（自动化测试/脚本触发）没有真实指针，
 * 直接调用 setPointerCapture 会抛 NotFoundError，这里退化为「不捕获」。
 */
export function capturePointer(element, pointerId) {
	try {
		element.setPointerCapture(pointerId);
	} catch (err) {
		/* 合成事件或指针已失效：无需捕获 */
	}
}

export function releasePointer(element, pointerId) {
	try {
		if (element.hasPointerCapture && element.hasPointerCapture(pointerId)) {
			element.releasePointerCapture(pointerId);
		}
	} catch (err) {
		/* 指针已释放 */
	}
}

export function isTextInput(target) {
	if (!(target instanceof Element)) return false;
	const tag = target.tagName;
	return tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || target.isContentEditable;
}
