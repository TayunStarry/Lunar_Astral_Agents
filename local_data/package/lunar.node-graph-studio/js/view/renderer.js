/**
 * 渲染器：把 Graph 投影成 DOM（节点层）+ SVG（连线层）+ HTML（连线标签层）。
 *
 * 采用「按 id 差量复用」：同一节点的 DOM 元素在多次渲染间保持不变，
 * 因此原地文字编辑、悬停态、CSS 过渡都不会被重绘打断。
 *
 * 连线方向由 core/router.js 决定：箭头多边形被 translate 到端口锚点、rotate 到
 * 曲线在该点的前进方向角，因此箭头永远指向被连节点。
 */

import { EDGE_STYLES, arrowPath, computeFans, routeDangling, routeEdge, sampleRoute } from "../core/router.js";
import { portOffset, sideSlots, SIDES } from "../core/ports.js";
import { HEADER_HEIGHT, MIN_NODE_H, getNodeType } from "../model/palette.js";
import { rectOf } from "../core/geometry.js";

const ARROW_SIZE = 11;

export class Renderer {
	/**
	 * @param {import("../app/store.js").Store} store
	 * @param {object} dom 全部相关容器
	 * @param {import("./viewport.js").Viewport} viewport
	 */
	constructor(store, dom, viewport) {
		this.store = store;
		this.viewport = viewport;
		this.dom = dom;
		/** @type {Map<string, HTMLElement>} */
		this.nodeEls = new Map();
		/** @type {Map<string, SVGGElement>} */
		this.edgeEls = new Map();
		/** @type {Map<string, HTMLElement>} */
		this.labelEls = new Map();
		/** @type {Map<string, object>} 最近一次渲染算出的连线几何，供命中测试复用 */
		this.routes = new Map();
		this.arrowD = arrowPath(ARROW_SIZE);

		this.tempWire = null;
		this.tempArrow = null;
		this.guideEls = [];
		this.marqueeEl = dom.marquee;

		this.selectionSignature = "";
	}

	// ------------------------------------------------------------ 图渲染

	renderGraph() {
		this.syncNodeOrder();
		this.renderNodes();
		this.renderEdges();
		this.renderSelection();
		this.renderTransient();
	}

	/** 节点数组顺序即层序，逐帧对齐 DOM 顺序 */
	syncNodeOrder() {
		const nodes = this.store.graph.nodes;
		for (let i = 0; i < nodes.length; i++) {
			const el = this.nodeEls.get(nodes[i].id);
			if (!el) continue;
			const current = this.dom.nodeLayer.children[i];
			if (current !== el) this.dom.nodeLayer.insertBefore(el, current || null);
		}
	}

	renderNodes() {
		const nodes = this.store.graph.nodes;
		const alive = new Set();
		for (const node of nodes) {
			alive.add(node.id);
			let el = this.nodeEls.get(node.id);
			if (!el) {
				el = this.createNodeEl(node);
				this.nodeEls.set(node.id, el);
				this.dom.nodeLayer.appendChild(el);
			}
			this.updateNodeEl(el, node);
		}
		for (const [id, el] of [...this.nodeEls]) {
			if (alive.has(id)) continue;
			el.remove();
			this.nodeEls.delete(id);
		}
	}

	createNodeEl(node) {
		const el = document.createElement("div");
		el.className = "node";
		el.dataset.nodeId = node.id;
		el.innerHTML = `
			<div class="node-accent"></div>
			<div class="node-head">
				<i class="node-icon fas"></i>
				<div class="node-title"></div>
				<button class="node-tool" data-act="collapse" title="折叠 / 展开正文"><i class="fas fa-chevron-up"></i></button>
			</div>
			<div class="node-body">
				<div class="node-text"></div>
			</div>
			<div class="node-ports"></div>
			<div class="node-lock" title="已锁定位置"><i class="fas fa-lock"></i></div>
			<div class="node-resize" title="拖动调整大小"></div>
		`;
		return el;
	}

	updateNodeEl(el, node) {
		el.style.left = `${node.x}px`;
		el.style.top = `${node.y}px`;
		el.style.width = `${node.w}px`;
		el.style.height = `${node.h}px`;
		el.style.setProperty("--node-color", node.color);
		el.style.setProperty("--node-font", `${node.fontSize}px`);
		el.dataset.shape = node.shape || "round";
		el.dataset.type = node.type || "process";
		el.classList.toggle("is-locked", !!node.locked);
		el.classList.toggle("is-collapsed", !!node.collapsed);

		const preset = getNodeType(node.type);
		const icon = el.querySelector(".node-icon");
		const iconClass = `node-icon fas ${preset.icon}`;
		if (icon.className !== iconClass) icon.className = iconClass;

		const title = el.querySelector(".node-title");
		if (title.textContent !== node.title) title.textContent = node.title;
		title.title = node.title;

		const textEl = el.querySelector(".node-text");
		if (textEl.textContent !== node.text) textEl.textContent = node.text;
		textEl.title = node.text;

		const collapseBtn = el.querySelector('[data-act="collapse"]');
		collapseBtn.querySelector("i").className = node.collapsed ? "fas fa-chevron-down" : "fas fa-chevron-up";

		this.syncPorts(el, node);
	}

	/** 端口元素只在「端口签名」变化时重建（拖拽/渲染期间不打断悬停态） */
	syncPorts(el, node) {
		const signature = SIDES.map((side) => `${side}:${sideSlots(node, side).length}`).join(",")
			+ "|" + JSON.stringify(node.ports);
		if (el.__portSig === signature) {
			this.positionPorts(el, node);
			return;
		}
		el.__portSig = signature;
		const host = el.querySelector(".node-ports");
		host.textContent = "";
		for (const side of SIDES) {
			for (const slot of sideSlots(node, side)) {
				const port = document.createElement("div");
				port.className = "port";
				port.dataset.nodeId = node.id;
				port.dataset.dir = slot.dir;
				port.dataset.side = side;
				port.dataset.index = String(slot.index);
				port.title = `${slot.dir === "in" ? "输入" : "输出"} · ${sideLabel(side)} #${slot.index}（拖动即可连线）`;
				port.innerHTML = '<span class="port-dot"></span>';
				host.appendChild(port);
			}
		}
		this.positionPorts(el, node);
	}

	positionPorts(el, node) {
		const ports = el.querySelectorAll(".port");
		for (const port of ports) {
			const dir = port.dataset.dir;
			const side = port.dataset.side;
			const index = Number(port.dataset.index);
			const off = portOffset(node, dir, side, index);
			port.style.left = `${off.x}px`;
			port.style.top = `${off.y}px`;
			port.dataset.side = side;
		}
	}

	renderEdges() {
		const graph = this.store.graph;
		const fans = computeFans(graph.edges);
		const alive = new Set();
		this.routes.clear();

		for (const edge of graph.edges) {
			const route = routeEdge(edge, (id) => graph.getNode(id), {
				style: EDGE_STYLES.includes(edge.style) ? edge.style : "bezier",
				bend: fans.get(edge.id) || 0,
			});
			if (!route) continue;
			alive.add(edge.id);
			this.routes.set(edge.id, route);

			let group = this.edgeEls.get(edge.id);
			if (!group) {
				group = this.createEdgeEl(edge);
				this.edgeEls.set(edge.id, group);
				this.dom.edgeRoot.appendChild(group);
			}
			this.updateEdgeEl(group, edge, route);

			let label = this.labelEls.get(edge.id);
			if (!label) {
				label = document.createElement("div");
				label.className = "edge-label";
				label.dataset.edgeId = edge.id;
				this.labelEls.set(edge.id, label);
				this.dom.edgeLabels.appendChild(label);
			}
			this.updateLabelEl(label, edge, route);
		}

		for (const [id, el] of [...this.edgeEls]) {
			if (alive.has(id)) continue;
			el.remove();
			this.edgeEls.delete(id);
		}
		for (const [id, el] of [...this.labelEls]) {
			if (alive.has(id)) continue;
			el.remove();
			this.labelEls.delete(id);
		}
	}

	createEdgeEl(edge) {
		const NS = "http://www.w3.org/2000/svg";
		const group = document.createElementNS(NS, "g");
		group.setAttribute("class", "edge");
		group.dataset.edgeId = edge.id;

		const hit = document.createElementNS(NS, "path");
		hit.setAttribute("class", "edge-hit");
		const line = document.createElementNS(NS, "path");
		line.setAttribute("class", "edge-line");
		const arrowTo = document.createElementNS(NS, "path");
		arrowTo.setAttribute("class", "edge-arrow edge-arrow-to");
		arrowTo.setAttribute("d", this.arrowD);
		const arrowFrom = document.createElementNS(NS, "path");
		arrowFrom.setAttribute("class", "edge-arrow edge-arrow-from");
		arrowFrom.setAttribute("d", this.arrowD);

		group.append(hit, line, arrowFrom, arrowTo);
		return group;
	}

	updateEdgeEl(group, edge, route) {
		const [hit, line, arrowFrom, arrowTo] = group.children;
		hit.setAttribute("d", route.d);
		line.setAttribute("d", route.d);
		group.dataset.style = route.style;
		group.classList.toggle("is-dashed", !!edge.dashed);

		if (edge.color) group.style.setProperty("--edge-color", edge.color);
		else group.style.removeProperty("--edge-color");

		const showTo = edge.arrow !== "none";
		const showFrom = edge.arrow === "both";
		arrowTo.style.display = showTo ? "" : "none";
		arrowFrom.style.display = showFrom ? "" : "none";
		if (showTo) {
			arrowTo.setAttribute("transform", `translate(${round(route.end.x)},${round(route.end.y)}) rotate(${round(route.endAngle)})`);
		}
		if (showFrom) {
			// 尾箭头朝向来路：起点前进方向 + 180°
			arrowFrom.setAttribute("transform", `translate(${round(route.start.x)},${round(route.start.y)}) rotate(${round(route.startAngle + 180)})`);
		}
	}

	updateLabelEl(label, edge, route) {
		const hasText = !!edge.label;
		label.classList.toggle("is-empty", !hasText);
		if (!hasText) {
			label.textContent = "";
			return;
		}
		if (label.textContent !== edge.label) label.textContent = edge.label;
		label.style.left = `${route.mid.x}px`;
		label.style.top = `${route.mid.y}px`;
	}

	renderSelection() {
		const { nodes, edges } = this.store.selection;
		for (const [id, el] of this.nodeEls) el.classList.toggle("is-selected", nodes.has(id));
		for (const [id, el] of this.edgeEls) el.classList.toggle("is-selected", edges.has(id));
		for (const [id, el] of this.labelEls) el.classList.toggle("is-selected", edges.has(id));
	}

	// ------------------------------------------------------------ 瞬时态

	renderTransient() {
		const ui = this.store.ui;
		this.renderTempWire(ui);
		this.renderGuides(ui.guides);
		this.renderMarquee(ui.marquee);
		this.renderEditing(ui.editing);
	}

	renderTempWire(ui) {
		const anchor = ui.activePort;
		const free = ui.wirePreview;
		if (!anchor || !free) {
			if (this.tempWire) this.tempWire.style.display = "none";
			if (this.tempArrow) this.tempArrow.style.display = "none";
			return;
		}
		const node = this.store.graph.getNode(anchor.node);
		if (!node) return;
		const off = portOffset(node, anchor.dir, anchor.side, anchor.index);
		const anchorPoint = { x: node.x + off.x, y: node.y + off.y };

		if (!this.tempWire) {
			const NS = "http://www.w3.org/2000/svg";
			const group = document.createElementNS(NS, "g");
			group.setAttribute("class", "temp-edge");
			this.tempWire = document.createElementNS(NS, "path");
			this.tempWire.setAttribute("class", "temp-line");
			this.tempArrow = document.createElementNS(NS, "path");
			this.tempArrow.setAttribute("class", "temp-arrow");
			this.tempArrow.setAttribute("d", this.arrowD);
			group.append(this.tempWire, this.tempArrow);
			this.dom.tempRoot.appendChild(group);
			this.tempGroup = group;
		}
		this.tempWire.style.display = "";
		this.tempArrow.style.display = "";

		// 复用与正式连线完全相同的路由，保证「拖的时候什么样、连完就是什么样」
		const route = routeDangling(anchor.side, anchorPoint, free, ui.wireStyle);
		if (!route) return;
		this.tempWire.setAttribute("d", route.d);

		if (anchor.dir === "out") {
			// 从输出口起拖：箭头跟手，表示「将要连到鼠标处」
			this.tempArrow.setAttribute("transform", `translate(${round(route.end.x)},${round(route.end.y)}) rotate(${round(route.endAngle)})`);
		} else {
			// 从输入口起拖：箭头画在端口端，表示「连线将指入本节点」
			this.tempArrow.setAttribute("transform", `translate(${round(route.start.x)},${round(route.start.y)}) rotate(${round(route.endAngle + 180)})`);
		}
	}

	renderGuides(guides) {
		const list = Array.isArray(guides) ? guides : [];
		while (this.guideEls.length < list.length) {
			const el = document.createElement("div");
			el.className = "guide";
			this.dom.overlayLayer.appendChild(el);
			this.guideEls.push(el);
		}
		this.guideEls.forEach((el, i) => {
			const guide = list[i];
			if (!guide) {
				el.style.display = "none";
				return;
			}
			el.style.display = "";
			el.dataset.axis = guide.axis;
			const from = Math.min(guide.from, guide.to) - 24;
			const to = Math.max(guide.from, guide.to) + 24;
			if (guide.axis === "x") {
				el.style.left = `${guide.pos}px`;
				el.style.top = `${from}px`;
				el.style.width = "1px";
				el.style.height = `${Math.max(1, to - from)}px`;
			} else {
				el.style.left = `${from}px`;
				el.style.top = `${guide.pos}px`;
				el.style.width = `${Math.max(1, to - from)}px`;
				el.style.height = "1px";
			}
		});
	}

	renderMarquee(rect) {
		if (!this.marqueeEl) return;
		if (!rect || rect.w < 1 && rect.h < 1) {
			this.marqueeEl.classList.remove("is-on");
			return;
		}
		this.marqueeEl.classList.add("is-on");
		this.marqueeEl.style.left = `${rect.x}px`;
		this.marqueeEl.style.top = `${rect.y}px`;
		this.marqueeEl.style.width = `${rect.w}px`;
		this.marqueeEl.style.height = `${rect.h}px`;
	}

	/** 就地编辑期间给节点加标记（编辑器元素由 textedit.js 负责插入/移除） */
	renderEditing(editing) {
		// 拖动中的节点抬起来一点，给「正在搬运」的即时反馈；
		// renderTransient 每帧都会跑，所以拖动状态一变化就能立刻反映到样式上。
		const dragging = this.store.ui.mode === "dragNode";
		for (const [id, el] of this.nodeEls) {
			const forThis = !!editing && editing.id === id;
			el.classList.toggle("is-editing", forThis);
			el.classList.toggle("is-editing-text", forThis && editing.kind === "text");
			el.classList.toggle("is-editing-title", forThis && editing.kind === "title");
			el.classList.toggle("is-dragging", dragging && this.store.selection.nodes.has(id));
		}
	}

	/** 拖线时标记落点端口的可行性（每次悬停变化调用一次） */
	markDropTarget(ref, ok) {
		for (const el of this.dom.nodeLayer.querySelectorAll(".port.is-drop-ok, .port.is-drop-bad")) {
			el.classList.remove("is-drop-ok", "is-drop-bad");
		}
		if (!ref) return;
		const el = this.portElement(ref);
		if (!el) return;
		el.classList.add(ok ? "is-drop-ok" : "is-drop-bad");
	}

	portElement(ref) {
		if (!ref) return null;
		const nodeEl = this.nodeEls.get(ref.node);
		if (!nodeEl) return null;
		return nodeEl.querySelector(`.port[data-dir="${ref.dir}"][data-side="${ref.side}"][data-index="${ref.index}"]`);
	}

	elementAt(clientX, clientY) {
		return document.elementFromPoint(clientX, clientY);
	}

	// ------------------------------------------------------------ 查询与测量

	/** 最近一次渲染的连线几何 */
	routeOf(edgeId) {
		return this.routes.get(edgeId) || null;
	}

	/** 世界坐标下某点的连线命中（返回最近的一条） */
	hitEdge(worldPoint, tolerance) {
		let best = null;
		let bestDistance = Infinity;
		for (const [id, route] of this.routes) {
			const points = route.style === "bezier" ? sampleRoute(route, 20) : route.points;
			for (let i = 1; i < points.length; i++) {
				const d = segmentDistance(worldPoint, points[i - 1], points[i]);
				if (d <= tolerance && d < bestDistance) {
					bestDistance = d;
					best = id;
				}
			}
		}
		return best;
	}

	/** 测量节点内容所需的总高度（用于「适应内容」） */
	/**
	 * 内容真实需要的高度（不含"当前有多高"这个信息）。
	 *
	 * 关键：**绝不能读 .node-text 的 scrollHeight**。它是被 flex 拉伸填满当前高度
	 * 的元素，scrollHeight 天然 ≥ 它现在的可视高度，也就是 ≥ 节点当前高度减去头部与内边距；
	 * 于是"所需高度"必然大于当前高度，每调用一次就把卡片撑高十几像素 ——
	 * 「高度适应文字」越点越大、打字时每帧长 2px，都是这一处造成的。
	 *
	 * 改成用离屏探针量真实排版高度后，结果只取决于「文字 + 可用宽度 + 字号」，
	 * 与节点当前高度无关，因此可增可减且不会自激。
	 *
	 * @param {object} node
	 * @param {string} [textOverride] 就地编辑中传入 textarea 的当前值
	 */
	measureContentHeight(node, textOverride) {
		const el = this.nodeEls.get(node.id);
		const headEl = el ? el.querySelector(".node-head") : null;
		const bodyEl = el ? el.querySelector(".node-body") : null;
		const textEl = el ? el.querySelector(".node-text") : null;
		const headH = headEl ? headEl.offsetHeight : HEADER_HEIGHT;
		const bodyStyle = bodyEl ? getComputedStyle(bodyEl) : null;
		const textStyle = textEl ? getComputedStyle(textEl) : null;
		const bodyPadV = bodyStyle ? cssPx(bodyStyle.paddingTop) + cssPx(bodyStyle.paddingBottom) : 16;
		const bodyPadH = bodyStyle ? cssPx(bodyStyle.paddingLeft) + cssPx(bodyStyle.paddingRight) : 10;
		const textPadV = textStyle ? cssPx(textStyle.paddingTop) + cssPx(textStyle.paddingBottom) : 6;
		const textPadH = textStyle ? cssPx(textStyle.paddingLeft) + cssPx(textStyle.paddingRight) : 12;
		const text = textOverride != null ? String(textOverride) : String(node.text || "");
		const width = Math.max(40, node.w - bodyPadH - textPadH);
		const textH = this.measureTextHeight(text, width, node.fontSize);
		return Math.max(MIN_NODE_H, Math.ceil(headH + bodyPadV + textPadV + textH));
	}

	/** 折叠态应有的高度：只留一条标题栏 */
	collapsedHeight(node) {
		const el = this.nodeEls.get(node.id);
		const headH = el ? (el.querySelector(".node-head")?.offsetHeight || HEADER_HEIGHT) : HEADER_HEIGHT;
		return Math.max(MIN_NODE_H, Math.ceil(headH));
	}

	/** 离屏探针测量：只依赖「文字 + 宽度 + 字号」，与节点当前尺寸无关 */
	measureTextHeight(text, width, fontSize) {
		if (!this.probeEl) {
			this.probeEl = document.createElement("div");
			this.probeEl.className = "measure-probe";
			this.dom.world.appendChild(this.probeEl);
		}
		const probe = this.probeEl;
		probe.style.width = `${width}px`;
		probe.style.fontSize = `${fontSize}px`;
		probe.textContent = text;
		return probe.offsetHeight;
	}

	/** 节点 DOM 的世界矩形（直接读模型，避免与渲染时序耦合） */
	nodeRect(node) {
		return rectOf(node);
	}

	/** 供 textedit 使用：节点正文/标题元素 */
	getTextEl(nodeId) {
		return this.nodeEls.get(nodeId)?.querySelector(".node-text") || null;
	}

	getTitleEl(nodeId) {
		return this.nodeEls.get(nodeId)?.querySelector(".node-title") || null;
	}

	getNodeEl(nodeId) {
		return this.nodeEls.get(nodeId) || null;
	}
}

// ---------------------------------------------------------------- 工具

function sideLabel(side) {
	return { top: "上", right: "右", bottom: "下", left: "左" }[side] || side;
}

/** 解析 CSS 长度（"7px" → 7），拿不到就返回 0 */
function cssPx(value) {
	const n = parseFloat(value);
	return Number.isFinite(n) ? n : 0;
}

function round(v) {
	return Math.round(v * 100) / 100;
}

function segmentDistance(p, a, b) {
	const vx = b.x - a.x;
	const vy = b.y - a.y;
	const l2 = vx * vx + vy * vy;
	if (l2 < 1e-9) return Math.hypot(p.x - a.x, p.y - a.y);
	let t = ((p.x - a.x) * vx + (p.y - a.y) * vy) / l2;
	t = Math.max(0, Math.min(1, t));
	return Math.hypot(p.x - (a.x + vx * t), p.y - (a.y + vy * t));
}
