/**
 * 就地文字编辑：节点正文、节点标题、连线标签都能双击直接改。
 *
 * 编辑期间只往 DOM 里插入一个编辑器元素（渲染器按 id 复用节点/连线元素，
 * 不会被重绘打断）；被编辑的原元素靠 CSS 隐藏，而不是从 DOM 里摘掉——
 * 这样编辑前后结构完全一致，不需要任何「还原替换」的补丁逻辑。
 *
 * 正文输入时节点会随内容自动增高：写不下就长高，是最自然也最省心的行为。
 */

import { el } from "../ui/dom.js";

export class TextEditor {
	/**
	 * @param {object} deps
	 * @param {import("../app/store.js").Store} deps.store
	 * @param {import("./renderer.js").Renderer} deps.renderer
	 * @param {import("../app/commands.js").Commands} deps.commands
	 */
	constructor({ store, renderer, commands }) {
		this.store = store;
		this.renderer = renderer;
		this.commands = commands;
		/** @type {{kind:string,id:string,original:string,el:HTMLElement}|null} */
		this.active = null;
		this._raf = 0;
	}

	get isEditing() {
		return !!this.active;
	}

	/** 双击节点正文 → 编辑多行文字 */
	beginText(nodeId) {
		const node = this.store.graph.getNode(nodeId);
		const nodeEl = this.renderer.getNodeEl(nodeId);
		const body = nodeEl ? nodeEl.querySelector(".node-body") : null;
		if (!node || !body) return null;
		this.finish("commit");

		const area = el("textarea", {
			class: "node-editor node-editor-text",
			spellcheck: false,
			placeholder: "输入节点文字…（Ctrl+Enter 结束）",
		});
		area.value = node.text || "";
		body.appendChild(area);

		this.activate({ kind: "text", id: nodeId, original: node.text || "", el: area });
		area.focus();
		const end = area.value.length;
		try {
			area.setSelectionRange(end, end);
		} catch (err) {
			/* 某些输入类型不支持选区，忽略 */
		}
		return area;
	}

	/** 双击节点标题 → 单行编辑 */
	beginTitle(nodeId) {
		const node = this.store.graph.getNode(nodeId);
		const titleEl = this.renderer.getTitleEl(nodeId);
		if (!node || !titleEl) return null;
		this.finish("commit");

		const input = el("input", { class: "node-editor node-editor-title", type: "text", placeholder: "节点标题" });
		input.value = node.title || "";
		titleEl.after(input);

		this.activate({ kind: "title", id: nodeId, original: node.title || "", el: input });
		input.focus();
		input.select();
		return input;
	}

	/** 双击连线 → 编辑连线标签 */
	beginLabel(edgeId) {
		const edge = this.store.graph.getEdge(edgeId);
		if (!edge) return null;
		this.finish("commit");

		const route = this.renderer.routeOf(edgeId);
		const host = this.renderer.dom.edgeLabels;
		const input = el("input", { class: "edge-label-editor", type: "text", placeholder: "连线标签" });
		input.value = edge.label || "";
		if (route) {
			input.style.left = `${route.mid.x}px`;
			input.style.top = `${route.mid.y}px`;
		}
		host.appendChild(input);

		this.activate({ kind: "label", id: edgeId, original: edge.label || "", el: input });
		input.focus();
		input.select();
		return input;
	}

	activate(state) {
		this.active = state;
		const editor = state.el;
		editor.addEventListener("keydown", (event) => this.onKeyDown(event));
		editor.addEventListener("blur", () => {
			if (this.active === state) this.finish("commit");
		});
		if (state.kind === "text") {
			editor.addEventListener("input", () => this.onTextInput(editor, state));
		}
		// 编辑器内部不触发画布交互
		for (const type of ["pointerdown", "pointerup", "dblclick", "wheel", "contextmenu"]) {
			editor.addEventListener(type, (event) => event.stopPropagation());
		}
		this.store.setUI({ editing: { kind: state.kind, id: state.id } });
	}

	onKeyDown(event) {
		if (event.key === "Escape") {
			event.preventDefault();
			event.stopPropagation();
			this.finish("cancel");
			return;
		}
		if (event.key === "Enter" && (event.ctrlKey || event.metaKey || this.active?.kind !== "text")) {
			event.preventDefault();
			event.stopPropagation();
			this.finish("commit");
			return;
		}
		// 其余按键不冒泡给画布快捷键（否则打字会触发删除/移动）
		event.stopPropagation();
	}

	onTextInput(area, state) {
		if (this._raf) return;
		this._raf = requestAnimationFrame(() => {
			this._raf = 0;
			if (this.active !== state) return;
			this.growToFit(area, state.id);
		});
	}

	/**
	 * 正文高度随内容收放。
	 *
	 * 用渲染器的离屏探针量「文字真实需要的高度」——它只取决于文字/宽度/字号，
	 * 与节点当前高度无关，所以既能在加行时变高、也能在删行时变矮，而且不会自激。
	 * （早先读 textarea 的 scrollHeight：它被 flex 拉伸成当前高度，读回来天然 ≥
	 * 当前高度，等于每敲一下就把卡片往上顶 2px。）
	 */
	growToFit(area, nodeId) {
		const node = this.store.graph.getNode(nodeId);
		if (!node) return;
		const target = this.renderer.measureContentHeight(node, area.value);
		if (Math.abs(target - node.h) < 1) return;
		node.h = Math.min(1800, Math.max(40, target));
		this.renderer.renderGraph();
		const again = this.renderer.getNodeEl(nodeId)?.querySelector(".node-editor-text");
		if (again && again !== area) {
			again.focus();
			const end = again.value.length;
			try {
				again.setSelectionRange(end, end);
			} catch (err) {
				/* 忽略 */
			}
		}
	}

	/**
	 * 结束编辑。
	 * @param {"commit"|"cancel"} mode
	 * @returns {boolean} 是否有实际改动
	 */
	finish(mode = "commit") {
		const state = this.active;
		if (!state) return false;
		this.active = null; // 先清空，避免 blur 造成重入
		if (this._raf) {
			cancelAnimationFrame(this._raf);
			this._raf = 0;
		}

		const value = state.el.value;
		const node = this.store.graph.getNode(state.id);
		// 记录编辑期间可能被直接改动的尺寸（growToFit 只改模型，不提交历史）
		const sizeBefore = node ? { w: node.w, h: node.h } : null;
		let changed = false;

		if (mode === "commit") {
			if (state.kind === "text" && node) {
				if (value !== state.original) {
					this.commands.patchNodes([state.id], { text: value }, "编辑节点文字");
					changed = true;
				}
			} else if (state.kind === "title" && node) {
				const next = value.trim() || state.original;
				if (next !== state.original) {
					this.commands.patchNodes([state.id], { title: next }, "编辑节点标题");
					changed = true;
				}
			} else if (state.kind === "label") {
				const edge = this.store.graph.getEdge(state.id);
				if (edge && value !== state.original) {
					this.commands.patchEdges([state.id], { label: value }, "编辑连线标签");
					changed = true;
				}
			}
		}

		state.el.remove();
		// 编辑期间「只长高不提交」的尺寸变化在这里补一次历史（与上面的提交同状态时会被忽略）
		if (sizeBefore && node && (sizeBefore.h !== node.h || sizeBefore.w !== node.w)) {
			this.commands.commit("调整节点高度");
		}
		this.store.setUI({ editing: null });
		// 完整重绘：把编辑期间直接改过的尺寸同步进 DOM，并隐藏/显示正确的元素
		this.renderer.renderGraph();
		return changed || !!(sizeBefore && node && (sizeBefore.h !== node.h || sizeBefore.w !== node.w));
	}
}
