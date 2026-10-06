/**
 * 左栏：节点类型面板 + 本地文档列表。
 *
 * 节点类型支持两种新建方式：单击（落在画布中心）与拖拽（落在鼠标松开处）。
 * 文档列表直接读写 local_data/database/node_graph_studio/，后端不可用时给出降级提示。
 */

import { NODE_TYPES } from "../model/palette.js";
import { el, toast } from "./dom.js";

export class Palette {
	/**
	 * @param {{host:HTMLElement, actions:object, store:object}} deps
	 */
	constructor({ host, actions, store }) {
		this.host = host;
		this.actions = actions;
		this.store = store;
		this.docs = [];
		this.render();
	}

	render() {
		this.host.textContent = "";

		// ---- 节点类型 ----
		const typeSection = el("section", { class: "panel" }, [
			el("header", { class: "panel-head", html: '<i class="fas fa-shapes"></i><span>节点类型</span>' }),
			el("div", { class: "panel-hint", text: "单击新建到画布中心，或拖到画布任意位置" }),
		]);
		const list = el("div", { class: "type-list" });
		for (const type of NODE_TYPES) {
			const item = el("div", {
				class: "type-item",
				draggable: true,
				title: `新建「${type.label}」节点`,
				dataset: { type: type.key },
			}, [
				el("span", { class: "type-icon", style: { "--node-color": type.color }, html: `<i class="fas ${type.icon}"></i>` }),
				el("span", { class: "type-label", text: type.label }),
				el("span", { class: "type-size", text: `${type.w}×${type.h}` }),
			]);
			item.addEventListener("click", () => this.actions.addNodeAtCenter?.(type.key));
			item.addEventListener("dragstart", (event) => {
				event.dataTransfer.effectAllowed = "copy";
				try {
					event.dataTransfer.setData("application/x-node-type", type.key);
					event.dataTransfer.setData("text/plain", type.key);
				} catch (err) {
					/* 某些环境限制 setData，退化为单击新建 */
				}
			});
			item.addEventListener("dblclick", () => this.actions.addNodeAtCenter?.(type.key));
			list.appendChild(item);
		}
		typeSection.appendChild(list);
		this.host.appendChild(typeSection);

		// ---- 快捷键提示 ----
		this.host.appendChild(this.buildShortcutSection());

		// ---- 本地文档 ----
		const docSection = el("section", { class: "panel panel-grow" }, [
			el("header", { class: "panel-head" }, [
				el("i", { class: "fas fa-folder-tree" }),
				el("span", { text: "本地文档" }),
				el("button", { class: "panel-head-btn", title: "刷新列表", html: '<i class="fas fa-rotate"></i>', on: { click: () => this.refreshDocs() } }),
			]),
		]);
		this.docList = el("div", { class: "doc-list" }, [el("div", { class: "list-empty", text: "正在读取…" })]);
		docSection.appendChild(this.docList);
		this.host.appendChild(docSection);
	}

	buildShortcutSection() {
		const rows = [
			["拖拽节点", "自由摆放（可多选一起拖）"],
			["端口拖拽", "上下左右任意方向连线"],
			["双击节点", "编辑文字 / 双击标题改标题"],
			["双击连线", "编辑连线标签"],
			["右键", "上下文菜单"],
			["空格 / 中键拖动", "平移画布"],
			["滚轮", "以光标为中心缩放"],
			["Ctrl+Z / Ctrl+Y", "撤销 / 重做"],
			["Ctrl+D / Ctrl+C / Ctrl+V", "再制 / 复制 / 粘贴"],
			["Ctrl+S", "保存到本地"],
			["Ctrl+0 / F", "100% / 适应视图"],
			["Delete", "删除选中"],
		];
		const body = el("div", { class: "shortcut-body" });
		for (const [key, desc] of rows) {
			body.appendChild(el("div", { class: "shortcut-row" }, [
				el("kbd", { text: key }),
				el("span", { text: desc }),
			]));
		}
		const section = el("section", { class: "panel panel-collapsed" }, [
			el("header", { class: "panel-head panel-head-clickable" }, [
				el("i", { class: "fas fa-keyboard" }),
				el("span", { text: "操作速查" }),
				el("i", { class: "fas fa-chevron-down panel-head-caret" }),
			]),
			body,
		]);
		section.querySelector(".panel-head").addEventListener("click", () => {
			section.classList.toggle("panel-collapsed");
		});
		return section;
	}

	// ------------------------------------------------------------ 文档列表

	async refreshDocs() {
		if (!this.docList) return;
		this.docList.textContent = "";
		this.docList.appendChild(el("div", { class: "list-empty", text: "正在读取…" }));
		const names = await this.actions.listDocs?.();
		this.docList.textContent = "";
		if (names === null) {
			this.docList.appendChild(el("div", { class: "list-empty", text: "本地文件接口不可用；当前使用浏览器草稿自动保存。" }));
			return;
		}
		this.docs = names;
		if (names.length === 0) {
			this.docList.appendChild(el("div", { class: "list-empty", text: "还没有保存过节点图。" }));
			return;
		}
		for (const name of names) {
			const row = el("div", { class: "doc-row" }, [
				el("i", { class: "fas fa-file-lines doc-icon" }),
				el("span", { class: "doc-name", text: name.replace(/\.json$/i, ""), title: name }),
				el("button", { class: "doc-btn", title: "打开", html: '<i class="fas fa-folder-open"></i>' }),
				el("button", { class: "doc-btn doc-btn-danger", title: "删除", html: '<i class="fas fa-trash-can"></i>' }),
			]);
			const [openBtn, delBtn] = row.querySelectorAll(".doc-btn");
			openBtn.addEventListener("click", () => this.actions.openDoc?.(name));
			delBtn.addEventListener("click", () => this.actions.deleteDoc?.(name, row));
			row.addEventListener("dblclick", () => this.actions.openDoc?.(name));
			this.docList.appendChild(row);
		}
	}

	setDocsHint(text) {
		if (!this.docList) return;
		this.docList.textContent = "";
		this.docList.appendChild(el("div", { class: "list-empty", text }));
	}

	notify(message, kind = "info") {
		toast(message, kind);
	}
}
