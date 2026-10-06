/**
 * 右栏属性检查器：按当前选择渲染节点 / 连线 / 多选 / 空选的属性面板。
 *
 * 焦点友好：仅在「选择集签名」变化时重建 DOM；签名不变时只回填数值，
 * 且跳过我正在输入的输入框 —— 于是连续改参数不会丢焦点。
 */

import { el } from "./dom.js";
import { COLOR_SWATCHES, FONT_SIZES, SHAPES, SHAPE_LABEL, NODE_TYPES } from "../model/palette.js";
import { EDGE_STYLES, EDGE_STYLE_LABEL, ARROW_MODES, ARROW_MODE_LABEL } from "../core/router.js";
import { SIDES, SIDE_LABEL, DIR_LABEL, portCount } from "../core/ports.js";
import { ALIGN_LABEL } from "../app/commands.js";

export class Inspector {
	/**
	 * @param {{host:HTMLElement, actions:object, store:object}} deps
	 */
	constructor({ host, actions, store }) {
		this.host = host;
		this.actions = actions;
		this.store = store;
		this.signature = "";
		this.fields = new Map();
		this.render(true);
	}

	/** 选择集签名：变了才重建 DOM */
	computeSignature() {
		const { nodes, edges } = this.store.selection;
		if (nodes.size === 0 && edges.size === 0) return "empty";
		if (nodes.size === 1 && edges.size === 0) return `node:${[...nodes][0]}`;
		if (edges.size === 1 && nodes.size === 0) return `edge:${[...edges][0]}`;
		return `multi:${[...nodes].sort().join(",")}|${[...edges].sort().join(",")}`;
	}

	render(force = false) {
		const signature = this.computeSignature();
		if (!force && signature === this.signature) {
			this.syncValues();
			return;
		}
		this.signature = signature;
		this.fields.clear();
		this.host.textContent = "";

		if (signature === "empty") this.renderEmpty();
		else if (signature.startsWith("node:")) this.renderNode(this.store.graph.getNode(signature.slice(5)));
		else if (signature.startsWith("edge:")) this.renderEdge(this.store.graph.getEdge(signature.slice(5)));
		else this.renderMulti();

		this.syncValues();
	}

	// ------------------------------------------------------------ 空选

	renderEmpty() {
		this.host.appendChild(this.section("fa-diagram-project", "画布设置", [
			this.row("文档名称", el("input", {
				class: "field-input", type: "text", value: this.store.graph.name,
				on: { change: (e) => this.actions.rename?.(e.target.value) },
			})),
			this.row("栅格尺寸", el("input", {
				class: "field-input", type: "number", min: "2", max: "200", step: "2", value: String(this.store.settings.grid),
				on: { change: (e) => this.actions.setSettings?.({ grid: Number(e.target.value) || 20 }) },
			})),
			this.row("默认线型", this.select(EDGE_STYLES.map((s) => [s, EDGE_STYLE_LABEL[s]]), this.store.ui.wireStyle, (v) => this.actions.setWireStyle?.(v))),
			this.row("默认箭头", this.select(ARROW_MODES.map((s) => [s, ARROW_MODE_LABEL[s]]), this.store.ui.arrowMode, (v) => this.actions.setArrowMode?.(v))),
			this.row("栅格吸附", this.toggle("snapGrid", this.store.settings.snapGrid)),
			this.row("对齐参考线", this.toggle("snapAlign", this.store.settings.snapAlign)),
			this.row("自动保存草稿", this.toggle("autosave", this.store.settings.autosave)),
		]));

		this.host.appendChild(this.section("fa-circle-info", "怎么用", [
			el("div", { class: "tip-list" }, [
				el("p", { text: "① 从左栏拖一个节点到画布，或单击新建。按住节点即可自由摆放；多选时可整组一起拖。" }),
				el("p", { text: "② 节点的上、下、左、右四侧都有输入口与输出口，从任意端口拖到另一个端口即可连线。" }),
				el("p", { text: "③ 双击节点改文字，双击节点标题改标题，双击连线加标签。" }),
				el("p", { text: "④ 连线箭头指向被连节点；可在选中连线后改成曲线 / 直角 / 直线，或翻转方向。" }),
				el("p", { text: "⑤ 拖动时的吸附：默认只吸附「与邻近节点对齐」并显示参考线；按住 Alt 可临时全部关闭。" }),
			]),
		]));
	}

	// ------------------------------------------------------------ 单节点

	renderNode(node) {
		if (!node) {
			this.renderEmpty();
			return;
		}
		const preset = NODE_TYPES.find((t) => t.key === node.type);

		this.host.appendChild(this.section(preset ? preset.icon : "fa-square", `节点 · ${preset ? preset.label : node.type}`, [
			this.row("标题", this.textField("title", node.title, {
				title: "编辑节点标题",
				onCommit: (v) => this.actions.patchNodes?.([node.id], { title: v.trim() || node.title }, "编辑节点标题"),
			})),
			this.row("正文", this.textArea("text", node.text, "编辑节点文字"), true),
			this.row("颜色", this.swatches(node.color, (color) => this.actions.patchNodes?.([node.id], { color }, "修改节点颜色"))),
			this.row("形状", this.select(SHAPES.map((s) => [s, SHAPE_LABEL[s]]), node.shape, (v) => this.actions.patchNodes?.([node.id], { shape: v }, "修改节点形状"))),
			this.row("字号", this.select(FONT_SIZES.map((s) => [String(s), `${s}px`]), String(node.fontSize), (v) => this.actions.patchNodes?.([node.id], { fontSize: Number(v) }, "修改节点字号"))),
			this.row("位置", this.pair(
				this.numberField("x", node.x, (v) => this.actions.patchNodes?.([node.id], { x: v }, "移动节点")),
				this.numberField("y", node.y, (v) => this.actions.patchNodes?.([node.id], { y: v }, "移动节点")),
			)),
			this.row("尺寸", this.pair(
				this.numberField("w", node.w, (v) => this.actions.patchNodes?.([node.id], { w: v }, "调整节点尺寸"), 96),
				this.numberField("h", node.h, (v) => this.actions.patchNodes?.([node.id], { h: v }, "调整节点尺寸"), 56),
			)),
			el("div", { class: "field-actions" }, [
				el("button", {
					class: "btn btn-sm",
					html: '<i class="fas fa-text-height"></i> 高度贴合文字',
					title: "按正文内容自动收放这个节点的高度（文字多则变高、文字少则收紧）",
					on: { click: () => this.actions.fitToContent?.([node.id]) },
				}),
				el("button", {
					class: `btn btn-sm${node.locked ? " is-on" : ""}`,
					html: `<i class="fas fa-lock"></i> ${node.locked ? "已锁定" : "锁定位置"}`,
					title: "锁定后拖动不会移动该节点",
					on: { click: () => this.actions.toggleLock?.([node.id]) },
				}),
			]),
		]));

		// 端口矩阵：四侧 × 输入/输出，各有 -/+ 计数与提示
		const portRows = [];
		const grid = el("div", { class: "port-grid" });
		grid.appendChild(el("div", { class: "port-grid-head", text: "侧" }));
		grid.appendChild(el("div", { class: "port-grid-head", text: "输入" }));
		grid.appendChild(el("div", { class: "port-grid-head", text: "输出" }));
		for (const side of SIDES) {
			grid.appendChild(el("div", { class: "port-grid-side", text: SIDE_LABEL[side] }));
			for (const dir of ["in", "out"]) {
				const count = portCount(node, dir, side);
				grid.appendChild(el("div", { class: "port-cell" }, [
					el("button", { class: "stepper", text: "−", title: `减少${SIDE_LABEL[side]}侧${DIR_LABEL[dir]}端口`, on: { click: () => this.actions.removePort?.(node.id, dir, side) } }),
					el("span", { class: "port-count", text: String(count) }),
					el("button", { class: "stepper", text: "+", title: `增加${SIDE_LABEL[side]}侧${DIR_LABEL[dir]}端口`, on: { click: () => this.actions.addPort?.(node.id, dir, side) } }),
				]));
			}
		}
		portRows.push(grid);
		portRows.push(el("div", { class: "panel-hint", text: "默认四侧各有 1 入 1 出，因此可以向四面八方连线；同侧多个端口会沿该侧均匀排开。" }));

		this.host.appendChild(this.section("fa-plug", "端口", portRows));

		this.host.appendChild(el("div", { class: "danger-zone" }, [
			el("button", { class: "btn btn-danger btn-block", html: '<i class="fas fa-trash-can"></i> 删除该节点', on: { click: () => this.actions.deleteSelection?.() } }),
		]));
	}

	// ------------------------------------------------------------ 单连线

	renderEdge(edge) {
		if (!edge) {
			this.renderEmpty();
			return;
		}
		const fromNode = this.store.graph.getNode(edge.from.node);
		const toNode = this.store.graph.getNode(edge.to.node);

		this.host.appendChild(this.section("fa-bezier-curve", "连线", [
			el("div", { class: "edge-endpoints" }, [
				el("div", { class: "endpoint endpoint-from" }, [
					el("span", { class: "endpoint-tag", text: "尾部（输出）" }),
					el("span", { class: "endpoint-name", text: fromNode ? fromNode.title : "?" }),
					el("span", { class: "endpoint-port", text: `${DIR_LABEL[edge.from.dir]}·${SIDE_LABEL[edge.from.side]}#${edge.from.index}` }),
				]),
				el("i", { class: "fas fa-arrow-down-long edge-arrow-hint" }),
				el("div", { class: "endpoint endpoint-to" }, [
					el("span", { class: "endpoint-tag", text: "头部（输入）" }),
					el("span", { class: "endpoint-name", text: toNode ? toNode.title : "?" }),
					el("span", { class: "endpoint-port", text: `${DIR_LABEL[edge.to.dir]}·${SIDE_LABEL[edge.to.side]}#${edge.to.index}` }),
				]),
			]),
			this.row("线型", this.segmented(EDGE_STYLES, EDGE_STYLE_LABEL, edge.style, (v) => this.actions.patchEdges?.([edge.id], { style: v }, "修改线型"))),
			this.row("箭头", this.segmented(ARROW_MODES, ARROW_MODE_LABEL, edge.arrow, (v) => this.actions.patchEdges?.([edge.id], { arrow: v }, "修改箭头"))),
			this.row("虚线", this.toggleValue(!!edge.dashed, (on) => this.actions.patchEdges?.([edge.id], { dashed: on }, "切换虚线"))),
			this.row("标签", this.textField("label", edge.label, {
				title: "编辑连线标签",
				onCommit: (v) => this.actions.patchEdges?.([edge.id], { label: v }, "编辑连线标签"),
			})),
			this.row("颜色", this.swatches(edge.color || "#7f96c7", (color) => this.actions.patchEdges?.([edge.id], { color }, "修改连线颜色"), true)),
			el("div", { class: "field-actions" }, [
				el("button", { class: "btn btn-sm", html: '<i class="fas fa-right-left"></i> 翻转方向', title: "交换尾部与头部（箭头随之反向）", on: { click: () => this.actions.reverseEdge?.(edge.id) } }),
			]),
		]));

		this.host.appendChild(el("div", { class: "danger-zone" }, [
			el("button", { class: "btn btn-danger btn-block", html: '<i class="fas fa-trash-can"></i> 删除该连线', on: { click: () => this.actions.deleteSelection?.() } }),
		]));
	}

	// ------------------------------------------------------------ 多选

	renderMulti() {
		const nodes = this.store.selection.nodes.size;
		const edges = this.store.selection.edges.size;
		this.host.appendChild(this.section("fa-object-group", `多选 · ${nodes} 节点 / ${edges} 连线`, [
			el("div", { class: "field-actions field-actions-wrap" }, Object.entries(ALIGN_LABEL).map(([mode, label]) =>
				el("button", { class: "btn btn-sm", text: label, on: { click: () => this.actions.align?.(mode) } }))),
			this.row("批量颜色", this.swatches(null, (color) => this.actions.patchNodes?.(this.store.selection.nodes, { color }, "修改节点颜色"))),
			this.row("批量字号", this.select(FONT_SIZES.map((s) => [String(s), `${s}px`]), "", (v) => this.actions.patchNodes?.(this.store.selection.nodes, { fontSize: Number(v) }, "修改节点字号"))),
			this.row("批量线型", this.segmented(EDGE_STYLES, EDGE_STYLE_LABEL, "", (v) => this.actions.patchEdges?.(this.store.selection.edges, { style: v }, "修改线型"))),
			el("div", { class: "field-actions" }, [
				el("button", { class: "btn btn-sm", html: '<i class="fas fa-clone"></i> 再制', on: { click: () => this.actions.duplicate?.() } }),
				el("button", { class: "btn btn-sm", html: '<i class="fas fa-sitemap"></i> 自动布局', on: { click: () => this.actions.autoLayout?.("TB", true) } }),
			]),
		]));

		this.host.appendChild(el("div", { class: "danger-zone" }, [
			el("button", { class: "btn btn-danger btn-block", html: '<i class="fas fa-trash-can"></i> 删除选中项', on: { click: () => this.actions.deleteSelection?.() } }),
		]));
	}

	// ------------------------------------------------------------ 片段构造

	section(icon, title, children) {
		const box = el("section", { class: "panel" }, [
			el("header", { class: "panel-head", html: `<i class="fas ${icon}"></i><span></span>` }),
		]);
		box.querySelector("span").textContent = title;
		const body = el("div", { class: "panel-body" });
		for (const child of children) if (child) body.appendChild(child);
		box.appendChild(body);
		return box;
	}

	row(label, control, stacked = false) {
		return el("div", { class: `field${stacked ? " field-stacked" : ""}` }, [
			el("label", { class: "field-label", text: label }),
			el("div", { class: "field-control" }, [control]),
		]);
	}

	register(name, node) {
		this.fields.set(name, node);
		return node;
	}

	/**
	 * 单行文本框。
	 * @param {string} name 回填用的字段名
	 * @param {string} value 初值
	 * @param {{title?:string, onCommit?:Function}} [options] onCommit 在 change 时调用
	 */
	textField(name, value, options = {}) {
		const input = el("input", { class: "field-input", type: "text", value: value || "", title: options.title || "" });
		if (options.onCommit) input.addEventListener("change", () => options.onCommit(input.value));
		return this.register(name, input);
	}

	textArea(name, value, commitLabel = "") {
		const area = el("textarea", { class: "field-input field-area", spellcheck: false, title: commitLabel });
		area.value = value || "";
		area.addEventListener("change", () => {
			const ids = [...this.store.selection.nodes];
			if (ids.length === 1) this.actions.patchNodes?.(ids, { text: area.value }, commitLabel || "编辑节点文字");
		});
		return this.register(name, area);
	}

	numberField(name, value, onCommit, min = null) {
		const input = el("input", { class: "field-input field-number", type: "number", value: String(Math.round(value)) });
		if (min != null) input.min = String(min);
		input.addEventListener("change", () => {
			const v = Number(input.value);
			if (Number.isFinite(v)) onCommit(v);
		});
		return this.register(name, input);
	}

	pair(a, b) {
		return el("div", { class: "field-pair" }, [a, b]);
	}

	select(options, value, onChange) {
		const select = el("select", { class: "field-input" });
		for (const [val, label] of options) {
			const option = el("option", { value: val, text: label });
			select.appendChild(option);
		}
		select.value = value;
		select.addEventListener("change", () => onChange(select.value));
		return select;
	}

	segmented(values, labels, current, onChange) {
		const box = el("div", { class: "segmented" });
		for (const value of values) {
			const button = el("button", { class: `segment${value === current ? " is-on" : ""}`, text: labels[value] || value });
			button.addEventListener("click", () => {
				onChange(value);
				for (const sibling of box.children) sibling.classList.toggle("is-on", sibling === button);
			});
			box.appendChild(button);
		}
		return box;
	}

	swatches(current, onPick, allowClear = false) {
		const box = el("div", { class: "swatches" });
		for (const color of COLOR_SWATCHES) {
			const button = el("button", {
				class: `swatch${current && current.toLowerCase() === color ? " is-on" : ""}`,
				style: { "--swatch": color },
				title: color,
			});
			button.addEventListener("click", () => {
				onPick(color);
				for (const sibling of box.children) sibling.classList.toggle("is-on", sibling === button);
			});
			box.appendChild(button);
		}
		if (allowClear) {
			const clear = el("button", { class: "swatch swatch-clear", title: "恢复默认颜色" });
			clear.addEventListener("click", () => onPick(null));
			box.appendChild(clear);
		}
		return box;
	}

	toggle(name, value) {
		const box = el("label", { class: "switch" });
		const input = el("input", { type: "checkbox" });
		input.checked = !!value;
		input.addEventListener("change", () => this.actions.setSettings?.({ [name]: input.checked }));
		box.append(input, el("span", { class: "switch-track" }), el("span", { class: "switch-thumb" }));
		return this.register(name, input);
	}

	toggleValue(value, onChange) {
		const box = el("label", { class: "switch" });
		const input = el("input", { type: "checkbox" });
		input.checked = !!value;
		input.addEventListener("change", () => onChange(input.checked));
		box.append(input, el("span", { class: "switch-track" }), el("span", { class: "switch-thumb" }));
		return input;
	}

	// ------------------------------------------------------------ 数值回填

	/** 不重建 DOM，只把模型值写回控件（跳过正在编辑的控件） */
	syncValues() {
		const signature = this.signature;
		const active = document.activeElement;
		const setValue = (input, value) => {
			if (!input || input === active) return;
			const next = String(value);
			if (input.value !== next) input.value = next;
		};

		if (signature.startsWith("node:")) {
			const node = this.store.graph.getNode(signature.slice(5));
			if (!node) return;
			setValue(this.fields.get("title"), node.title);
			setValue(this.fields.get("text"), node.text);
			setValue(this.fields.get("x"), Math.round(node.x));
			setValue(this.fields.get("y"), Math.round(node.y));
			setValue(this.fields.get("w"), Math.round(node.w));
			setValue(this.fields.get("h"), Math.round(node.h));
		} else if (signature.startsWith("edge:")) {
			const edge = this.store.graph.getEdge(signature.slice(5));
			if (!edge) return;
			setValue(this.fields.get("label"), edge.label);
		} else if (signature === "empty") {
			setValue(this.fields.get("snapGrid"), this.store.settings.snapGrid);
			setValue(this.fields.get("snapAlign"), this.store.settings.snapAlign);
			setValue(this.fields.get("autosave"), this.store.settings.autosave);
		}
	}
}
