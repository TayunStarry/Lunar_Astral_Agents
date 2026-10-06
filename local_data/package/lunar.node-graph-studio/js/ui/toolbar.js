/**
 * 顶栏：按命令表声明式构建，按钮与处理函数一一对应（避免 HTML 里的 data-cmd 与 JS 脱节）。
 *
 * 命令表里的 cmd 名称就是 main.js 传入 actions 的键；缺处理函数会在控制台报警，
 * 这样界面与行为不会静默失联。
 */

import { el, toast } from "./dom.js";

/** 分隔符标记 */
const SEP = { sep: true };

export const TOOLBAR_GROUPS = [
	[
		{ cmd: "new", icon: "fa-file-circle-plus", title: "新建节点图（Ctrl+Alt+N）" },
		{ cmd: "loadSample", icon: "fa-wand-magic-sparkles", title: "载入示例节点图" },
		{ cmd: "openServer", icon: "fa-folder-open", title: "打开本地已保存的节点图" },
		{ cmd: "saveServer", icon: "fa-floppy-disk", title: "保存到 local_data（Ctrl+S）" },
	],
	[
		{ cmd: "importJSON", icon: "fa-file-import", title: "导入 JSON 工程文件" },
		{
			cmd: "exportMenu",
			icon: "fa-file-export",
			title: "导出",
			menu: [
				{ cmd: "exportJSON", icon: "fa-code", label: "导出 JSON 工程" },
				{ cmd: "exportSVG", icon: "fa-bezier-curve", label: "导出 SVG 矢量图" },
				{ cmd: "exportPNG", icon: "fa-image", label: "导出 PNG 位图" },
			],
		},
	],
	[
		{ cmd: "undo", icon: "fa-rotate-left", title: "撤销（Ctrl+Z）" },
		{ cmd: "redo", icon: "fa-rotate-right", title: "重做（Ctrl+Y）" },
	],
	[
		{ cmd: "copy", icon: "fa-copy", title: "复制（Ctrl+C）" },
		{ cmd: "paste", icon: "fa-paste", title: "粘贴（Ctrl+V）" },
		{ cmd: "duplicate", icon: "fa-clone", title: "再制（Ctrl+D）" },
		{ cmd: "delete", icon: "fa-trash-can", title: "删除选中（Delete）", danger: true },
	],
	[
		{
			cmd: "layoutMenu",
			icon: "fa-sitemap",
			title: "自动布局",
			menu: [
				{ cmd: "layoutTB", icon: "fa-arrow-down-long", label: "纵向分层（上→下）" },
				{ cmd: "layoutLR", icon: "fa-arrow-right-long", label: "横向分层（左→右）" },
				{ cmd: "layoutSelTB", icon: "fa-object-group", label: "仅对选中节点纵向分层" },
			],
		},
		{
			cmd: "alignMenu",
			icon: "fa-align-left",
			title: "对齐与分布",
			menu: [
				{ cmd: "align:left", icon: "fa-align-left", label: "左对齐" },
				{ cmd: "align:centerX", icon: "fa-align-center", label: "水平居中" },
				{ cmd: "align:right", icon: "fa-align-right", label: "右对齐" },
				{ cmd: "align:top", icon: "fa-arrow-up-long", label: "顶对齐" },
				{ cmd: "align:centerY", icon: "fa-arrows-up-down", label: "垂直居中" },
				{ cmd: "align:bottom", icon: "fa-arrow-down-long", label: "底对齐" },
				{ cmd: "align:distributeX", icon: "fa-arrows-left-right", label: "水平等距" },
				{ cmd: "align:distributeY", icon: "fa-arrows-up-down", label: "垂直等距" },
			],
		},
	],
	[
		{ cmd: "fitToContent", icon: "fa-text-height", title: "高度贴合文字：按内容自动收放（可增可减）" },
		{ cmd: "toggleLock", icon: "fa-lock", title: "锁定/解锁选中节点位置" },
		{ cmd: "bringToFront", icon: "fa-layer-group", title: "置于顶层" },
		{ cmd: "sendToBack", icon: "fa-layer-group", title: "置于底层", flip: true },
	],
];

export class Toolbar {
	/**
	 * @param {{host:HTMLElement, actions:Record<string,Function>, store:object}} deps
	 */
	constructor({ host, actions, store }) {
		this.host = host;
		this.actions = actions;
		this.store = store;
		this.buttons = new Map();
		this.openMenu = null;
		this.render();
	}

	render() {
		this.host.textContent = "";

		const left = el("div", { class: "bar-group bar-left" }, [
			el("div", { class: "brand", html: '<i class="fas fa-diagram-project"></i><span>节点图工坊</span>' }),
		]);

		const nameInput = el("input", { class: "doc-name", type: "text", title: "文档名称", value: this.store.graph.name });
		nameInput.addEventListener("change", () => this.actions.rename?.(nameInput.value));
		nameInput.addEventListener("keydown", (event) => {
			event.stopPropagation();
			if (event.key === "Enter") nameInput.blur();
		});
		nameInput.addEventListener("focus", () => nameInput.select());
		this.nameInput = nameInput;
		left.appendChild(nameInput);

		const center = el("div", { class: "bar-group bar-center" });
		for (const group of TOOLBAR_GROUPS) {
			const box = el("div", { class: "bar-cluster" });
			for (const item of group) box.appendChild(this.buildButton(item));
			center.appendChild(box);
		}

		const right = el("div", { class: "bar-group bar-right" }, [
			this.buildToggle("toggleGrid", "fa-border-all", "显示/隐藏栅格"),
			this.buildToggle("toggleSnap", "fa-magnet", "吸附：栅格 + 节点对齐"),
			this.buildToggle("toggleMinimap", "fa-map", "显示/隐藏缩略图"),
			el("div", { class: "bar-cluster" }, [
				el("button", { class: "btn btn-icon", title: "缩小（Ctrl+-）", html: '<i class="fas fa-magnifying-glass-minus"></i>', on: { click: () => this.actions.zoomOut?.() } }),
				el("button", { class: "btn btn-icon zoom-label", title: "缩放到 100%（Ctrl+0）", text: "100%", on: { click: () => this.actions.zoomReset?.() } }),
				el("button", { class: "btn btn-icon", title: "放大（Ctrl+=）", html: '<i class="fas fa-magnifying-glass-plus"></i>', on: { click: () => this.actions.zoomIn?.() } }),
				el("button", { class: "btn btn-icon", title: "适应全部节点（F / Ctrl+1）", html: '<i class="fas fa-expand"></i>', on: { click: () => this.actions.fit?.() } }),
			]),
		]);

		this.host.append(left, center, right);
		this.zoomLabel = right.querySelector(".zoom-label");
		this.refresh();
	}

	buildButton(item) {
		if (item.sep) return el("div", { class: "bar-sep" });
		const button = el("button", {
			class: `btn btn-icon${item.danger ? " btn-danger-ghost" : ""}`,
			title: item.title || item.label || item.cmd,
			html: `<i class="fas ${item.icon}"></i>${item.menu ? '<i class="fas fa-caret-down caret"></i>' : ""}`,
		});
		if (item.menu) {
			button.dataset.menu = item.cmd;
			button.addEventListener("click", (event) => {
				event.stopPropagation();
				this.toggleMenu(button, item.menu);
			});
		} else {
			button.addEventListener("click", () => this.invoke(item.cmd));
		}
		this.buttons.set(item.cmd, button);
		return button;
	}

	buildToggle(cmd, icon, title) {
		const button = el("button", { class: "btn btn-icon", title, html: `<i class="fas ${icon}"></i>` });
		button.addEventListener("click", () => this.invoke(cmd));
		this.buttons.set(cmd, button);
		return button;
	}

	invoke(cmd) {
		if (cmd.startsWith("align:")) {
			this.actions.align?.(cmd.slice(6));
			return;
		}
		const handler = this.actions[cmd];
		if (typeof handler === "function") handler();
		else toast(`未实现的命令：${cmd}`, "warn");
		this.closeMenu();
	}

	toggleMenu(anchor, items) {
		if (this.openMenu && this.openMenu.dataset.for === anchor.dataset.menu) {
			this.closeMenu();
			return;
		}
		this.closeMenu();
		const menu = el("div", { class: "menu" });
		menu.dataset.for = anchor.dataset.menu;
		for (const item of items) {
			const row = el("button", { class: "menu-item", html: `<i class="fas ${item.icon}"></i><span></span>` });
			row.querySelector("span").textContent = item.label;
			row.addEventListener("click", (event) => {
				event.stopPropagation();
				this.invoke(item.cmd);
			});
			menu.appendChild(row);
		}
		const rect = anchor.getBoundingClientRect();
		menu.style.left = `${Math.round(rect.left)}px`;
		menu.style.top = `${Math.round(rect.bottom + 6)}px`;
		document.body.appendChild(menu);
		this.openMenu = menu;
		setTimeout(() => {
			document.addEventListener("pointerdown", this.onDocPointerDown, true);
		}, 0);
	}

	onDocPointerDown = (event) => {
		if (this.openMenu && !this.openMenu.contains(event.target)) this.closeMenu();
	};

	closeMenu() {
		if (!this.openMenu) return;
		this.openMenu.remove();
		this.openMenu = null;
		document.removeEventListener("pointerdown", this.onDocPointerDown, true);
	}

	/** 按当前状态刷新按钮可用性与高亮 */
	refresh() {
		const store = this.store;
		const { history, commands } = this.deps || {};
		const hasNodes = store.graph.nodes.length > 0;
		const hasSelection = store.selectionCount > 0;
		const hasNodeSelection = store.selection.nodes.size > 0;

		this.setEnabled("delete", hasSelection);
		this.setEnabled("copy", hasNodeSelection);
		this.setEnabled("duplicate", hasNodeSelection);
		this.setEnabled("toggleLock", hasNodeSelection);
		this.setEnabled("bringToFront", hasNodeSelection);
		this.setEnabled("sendToBack", hasNodeSelection);
		this.setEnabled("fitToContent", hasNodeSelection);
		this.setEnabled("layoutMenu", hasNodes);
		this.setEnabled("alignMenu", store.selection.nodes.size > 1);
		this.setEnabled("undo", !!history?.canUndo?.());
		this.setEnabled("redo", !!history?.canRedo?.());

		this.setToggle("toggleGrid", !!store.settings.showGrid);
		this.setToggle("toggleSnap", !!store.settings.snapGrid || !!store.settings.snapAlign);
		this.setToggle("toggleMinimap", !!store.settings.showMinimap);

		if (this.zoomLabel) this.zoomLabel.textContent = `${Math.round(store.viewport.zoom * 100)}%`;
		if (this.nameInput && document.activeElement !== this.nameInput && this.nameInput.value !== store.graph.name) {
			this.nameInput.value = store.graph.name;
		}
		// 撤销/重做按钮的提示里带上操作名
		const undoBtn = this.buttons.get("undo");
		if (undoBtn) undoBtn.title = history?.canUndo?.() ? `撤销：${history.undoLabel()}（Ctrl+Z）` : "没有可撤销的操作";
		const redoBtn = this.buttons.get("redo");
		if (redoBtn) redoBtn.title = history?.canRedo?.() ? `重做：${history.redoLabel()}（Ctrl+Y）` : "没有可重做的操作";
	}

	/** 由 main 注入 history/commands，便于 refresh 读取状态 */
	attach(deps) {
		this.deps = deps;
		this.refresh();
	}

	setEnabled(cmd, enabled) {
		const button = this.buttons.get(cmd);
		if (!button) return;
		button.disabled = !enabled;
		button.classList.toggle("is-disabled", !enabled);
	}

	setToggle(cmd, on) {
		const button = this.buttons.get(cmd);
		if (!button) return;
		button.classList.toggle("is-on", !!on);
	}
}
