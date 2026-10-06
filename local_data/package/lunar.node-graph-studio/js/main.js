/**
 * 组合根：把模型、视图、交互、界面接线在一起，并注册全部命令与快捷键。
 *
 * 数据流是单向的：
 *   Interaction / UI  →  Commands  →  Graph  →  Store 事件  →  Renderer / 面板刷新
 * 视图层从不直接改数据，模型层也从不触碰 DOM —— 这是本项目最容易维护的分界线。
 */

import { Store } from "./app/store.js";
import { Commands, buildSampleDocument } from "./app/commands.js";
import { History } from "./model/history.js";
import { Viewport } from "./view/viewport.js";
import { Renderer } from "./view/renderer.js";
import { Minimap } from "./view/minimap.js";
import { TextEditor } from "./view/textedit.js";
import { Interaction, isTextInput } from "./interaction.js";
import { Toolbar } from "./ui/toolbar.js";
import { Palette } from "./ui/palette.js";
import { Inspector } from "./ui/inspector.js";
import { StatusBar } from "./ui/statusbar.js";
import { showMenu, closeMenu } from "./ui/contextmenu.js";
import { toast, initToasts, askText, askConfirm } from "./ui/dom.js";
import { Persistence } from "./io/persistence.js";
import * as exporter from "./io/exportfile.js";
import { EDGE_STYLES, EDGE_STYLE_LABEL, ARROW_MODES, ARROW_MODE_LABEL } from "./core/router.js";
import { emptyDocument } from "./model/document.js";
import { SIDES, SIDE_LABEL, portCount } from "./core/ports.js";
import { NODE_TYPES } from "./model/palette.js";

function boot() {
	const dom = {
		canvas: document.getElementById("canvas"),
		world: document.getElementById("world"),
		nodeLayer: document.getElementById("nodeLayer"),
		edgeRoot: document.getElementById("edgeRoot"),
		tempRoot: document.getElementById("tempRoot"),
		edgeLabels: document.getElementById("edgeLabels"),
		overlayLayer: document.getElementById("overlayLayer"),
		marquee: document.getElementById("marquee"),
		minimap: document.getElementById("minimap"),
		minimapCanvas: document.getElementById("minimapCanvas"),
		toastHost: document.getElementById("toastHost"),
		appBar: document.getElementById("appBar"),
		paletteHost: document.getElementById("paletteHost"),
		inspectorHost: document.getElementById("inspectorHost"),
		statusBar: document.getElementById("statusBar"),
	};
	const missing = Object.entries(dom).filter(([, value]) => !value).map(([key]) => key);
	if (missing.length) {
		console.error("[node-graph-studio] 缺少必需的 DOM 容器:", missing);
		return;
	}

	initToasts(dom.toastHost);

	const persistence = new Persistence(null, toast);
	// 首次进入：优先恢复浏览器草稿（不覆盖磁盘文件），否则给一份空白表
	const initial = persistence.loadLocal();
	const store = new Store(initial && initial.nodes.length ? initial : emptyDocument("未命名节点图"));
	persistence.store = store;

	const history = new History({
		snapshot: () => store.snapshot(),
		apply: (state) => store.restore(state),
		limit: 200,
	});
	history.reset();

	const commands = new Commands(store, history, toast);
	const viewport = new Viewport(store, dom);
	const renderer = new Renderer(store, dom, viewport);
	const minimap = new Minimap(store, viewport, { host: dom.minimap, canvas: dom.minimapCanvas });
	const textEditor = new TextEditor({ store, renderer, commands });

	// ------------------------------------------------------------ 渲染调度
	//
	// 全量渲染与「只画瞬时态」的渲染必须共用同一次 rAF，但**瞬时态请求可以被全量请求升级**。
	// 否则先到的瞬时态请求会占住这一帧、把紧随其后的全量渲染挤掉（`if (rafId) return`），
	// 表现为「拖节点时节点纹丝不动、一松手才跳过去」——交互层每帧都会先广播 ui 再请求重绘，
	// 正好踩中这个坑。
	let rafId = 0;
	let needFullRender = false;

	function scheduleRender(full) {
		if (full) needFullRender = true;
		if (rafId) return;
		rafId = requestAnimationFrame(() => {
			rafId = 0;
			const doFull = needFullRender;
			needFullRender = false;
			if (doFull) {
				renderer.renderGraph();
				minimap.setRouteCache(renderer.routes);
				minimap.schedule();
			} else {
				renderer.renderTransient();
			}
			statusbar.refresh();
			if (doFull) {
				inspector.render();
				toolbar.refresh();
			}
		});
	}

	function render() {
		scheduleRender(true);
	}

	function renderOverlayOnly() {
		scheduleRender(false);
	}

	// ------------------------------------------------------------ UI 装配

	const actions = {};
	const toolbar = new Toolbar({ host: dom.appBar, actions, store });
	const palette = new Palette({ host: dom.paletteHost, actions, store });
	const inspector = new Inspector({ host: dom.inspectorHost, actions, store });
	const statusbar = new StatusBar({ host: dom.statusBar, store });
	const interaction = new Interaction({
		store,
		viewport,
		renderer,
		commands,
		textEditor,
		render,
		showMenu,
	});

	// ------------------------------------------------------------ 命令实现

	Object.assign(actions, {
		rename: (name) => {
			const next = String(name || "").trim() || "未命名节点图";
			if (next === store.graph.name) return;
			store.graph.name = next;
			store.markDirty(true);
			store.emit("document");
			render();
		},
		newDocument: async () => {
			if (store.ui.dirty) {
				const ok = await askConfirm({ title: "新建节点图", message: "当前内容还有未保存的改动，确定要新建吗？", okText: "新建" });
				if (!ok) return;
			}
			commands.newDocument();
			persistence.clearLocal();
			viewport.fit(viewport.graphBounds(store.graph), 120);
			render();
			toast("已新建空白节点图", "ok");
		},
		loadSample: () => {
			commands.loadSample();
			viewport.fit(viewport.graphBounds(store.graph), 120);
			render();
		},
		saveServer: async () => {
			const name = await askText({ title: "保存到本地", value: store.graph.name, okText: "保存" });
			if (name === null) return;
			await persistence.saveToServer(name);
			toolbar.refresh();
			palette.refreshDocs();
		},
		openServer: async () => {
			const names = await persistence.listServer();
			if (!names) {
				toast("本地文件接口不可用", "warn");
				return;
			}
			if (!names.length) {
				toast("还没有保存过节点图", "warn");
				return;
			}
			showMenu(
				names.map((fileName) => ({
					label: fileName.replace(/\.json$/i, ""),
					icon: "fa-file-lines",
					cmd: fileName,
					onSelect: (target) => openDoc(target),
				})),
				Math.round(window.innerWidth / 2),
				Math.round(window.innerHeight / 3),
			);
		},
		listDocs: () => persistence.listServer(),
		openDoc: (fileName) => openDoc(fileName),
		deleteDoc: async (fileName, row) => {
			const ok = await askConfirm({
				title: "删除本地节点图",
				message: `确定删除「${fileName.replace(/\.json$/i, "")}」？该操作不可撤销。`,
				okText: "删除",
				danger: true,
			});
			if (!ok) return;
			const done = await persistence.deleteFromServer(fileName);
			if (done) {
				row?.remove();
				toast("已删除", "ok");
			}
		},
		exportJSON: () => {
			const name = exporter.exportJSON(store);
			toast(`已导出 ${name}.json`, "ok");
		},
		exportSVG: () => exporter.exportSVG(store),
		exportPNG: () => exporter.exportPNG(store),
		importJSON: async () => {
			const doc = await exporter.importJSON();
			if (!doc) return;
			commands.loadDocument(doc);
			viewport.fit(viewport.graphBounds(store.graph), 120);
			render();
			toast(`已导入「${doc.name}」`, "ok");
		},
		undo: () => {
			commands.undo();
			render();
		},
		redo: () => {
			commands.redo();
			render();
		},
		copy: () => commands.copySelection(),
		cut: () => commands.cutSelection(),
		paste: () => {
			commands.paste(centerWorld());
			render();
		},
		duplicate: () => {
			commands.duplicateSelection();
			render();
		},
		delete: () => {
			textEditor.finish("commit");
			commands.deleteSelection();
			render();
		},
		selectAll: () => {
			store.selectAll();
			render();
		},
		addNodeAtCenter: (typeKey) => {
			commands.addNode(typeKey, centerWorld());
			render();
		},
		patchNodes: (ids, patch, label) => {
			commands.patchNodes(ids, patch, label);
			render();
		},
		patchEdges: (ids, patch, label) => {
			commands.patchEdges(ids, patch, label);
			render();
		},
		addPort: (nodeId, dir, side) => {
			commands.addPort(nodeId, dir, side);
			render();
		},
		removePort: (nodeId, dir, side) => {
			commands.removePort(nodeId, dir, side);
			render();
		},
		align: (mode) => {
			commands.align(mode);
			render();
		},
		autoLayout: (direction, onlySelection) => {
			commands.autoLayout(direction, onlySelection);
			viewport.fit(viewport.graphBounds(store.graph), 120);
			render();
		},
		layoutTB: () => actions.autoLayout("TB", false),
		layoutLR: () => actions.autoLayout("LR", false),
		layoutSelTB: () => actions.autoLayout("TB", true),
		bringToFront: () => {
			commands.bringToFront([...store.selection.nodes]);
			render();
		},
		sendToBack: () => {
			commands.sendToBack([...store.selection.nodes]);
			render();
		},
		reverseEdge: (edgeId) => {
			commands.reverseEdge(edgeId);
			render();
		},
		toggleLock: (ids) => {
			commands.toggleLock(ids && ids.length ? ids : [...store.selection.nodes]);
			render();
		},
		/**
		 * 高度贴合文字：可增可减。
		 * 判据是「测量值 vs 当前高度」的差，超过 1px 就应用 —— 早先写成只处理
		 * `目标 > 当前`，于是它只会把卡片撑大，永远收不回来。
		 */
		fitToContent: (ids) => {
			const list = ids && ids.length ? ids : [...store.selection.nodes];
			const updates = [];
			for (const id of list) {
				const node = store.graph.getNode(id);
				if (!node) continue;
				const target = renderer.measureContentHeight(node);
				if (Math.abs(target - node.h) >= 1) updates.push([id, target]);
			}
			if (!updates.length) {
				toast("高度已经和文字吻合，无需调整", "info");
				return;
			}
			const grown = updates.filter(([id, h]) => h > store.graph.getNode(id).h).length;
			const shrunk = updates.length - grown;
			commands.run("高度贴合文字", () => {
				for (const [id, h] of updates) store.graph.updateNode(id, { h });
			});
			const parts = [];
			if (grown) parts.push(`变高 ${grown}`);
			if (shrunk) parts.push(`收紧 ${shrunk}`);
			toast(`已按文字调整高度（${parts.join("，")}）`, "ok");
			render();
		},
		/**
		 * 折叠/展开。折叠时收成一条标题栏，展开时重新按内容量一次高度 ——
		 * 否则折叠后卡片仍留着整块空白，展开后又可能裁掉文字。
		 */
		toggleCollapse: (nodeId) => {
			const node = store.graph.getNode(nodeId);
			if (!node) return;
			if (!node.collapsed) {
				commands.patchNodes([nodeId], { collapsed: true, h: renderer.collapsedHeight(node) }, "折叠节点");
			} else {
				commands.patchNodes([nodeId], { collapsed: false, h: renderer.measureContentHeight(node) }, "展开节点");
			}
			render();
		},
		setSettings: (patch) => {
			store.setSettings(patch);
			viewport.apply();
			render();
		},
		setWireStyle: (style) => {
			store.ui.wireStyle = EDGE_STYLES.includes(style) ? style : "bezier";
			store.setSettings({ defaultWireStyle: store.ui.wireStyle });
			inspector.render(true);
		},
		setArrowMode: (mode) => {
			store.ui.arrowMode = ARROW_MODES.includes(mode) ? mode : "to";
			store.setSettings({ defaultArrowMode: store.ui.arrowMode });
			inspector.render(true);
		},
		toggleGrid: () => {
			store.setSettings({ showGrid: !store.settings.showGrid });
			viewport.apply();
			toolbar.refresh();
		},
		toggleSnap: () => {
			const on = !(store.settings.snapGrid || store.settings.snapAlign);
			store.setSettings({ snapGrid: on, snapAlign: on });
			toolbar.refresh();
			toast(on ? "已开启吸附（栅格 + 对齐参考线）" : "已关闭吸附", "info");
		},
		toggleMinimap: () => {
			const on = !store.settings.showMinimap;
			store.setSettings({ showMinimap: on });
			dom.minimap.classList.toggle("is-hidden", !on);
			if (on) minimap.schedule();
			toolbar.refresh();
		},
		zoomIn: () => viewport.zoomBy(1.2),
		zoomOut: () => viewport.zoomBy(1 / 1.2),
		zoomReset: () => viewport.resetZoom(),
		fit: () => viewport.fit(viewport.graphBounds(store.graph), 120),
		deleteSelection: () => actions.delete(),
	});

	toolbar.attach({ history, commands });

	async function openDoc(fileName) {
		if (store.ui.dirty) {
			const ok = await askConfirm({ title: "打开本地节点图", message: "当前内容还有未保存的改动，确定要打开吗？", okText: "打开" });
			if (!ok) return;
		}
		const doc = await persistence.loadFromServer(fileName);
		if (!doc) return;
		commands.loadDocument(doc);
		viewport.fit(viewport.graphBounds(store.graph), 120);
		render();
		toast(`已打开「${doc.name}」`, "ok");
	}

	function centerWorld() {
		const rect = dom.canvas.getBoundingClientRect();
		return viewport.screenToWorld(rect.left + rect.width / 2, rect.top + rect.height / 2);
	}

	// ------------------------------------------------------------ 右键菜单

	// 折叠/展开需要渲染器测量文字高度，因此由组合根提供给交互层
	interaction.onToggleCollapse = (nodeId) => actions.toggleCollapse(nodeId);

	interaction.onContextMenuRequest = (event) => {
		const edgeId = interaction.hitEdgeAt(event.clientX, event.clientY);
		const nodeId = interaction.hitNodeAt(event.clientX, event.clientY);
		const portEl = event.target instanceof Element ? event.target.closest(".port") : null;

		if (portEl) {
			const nodeRef = portEl.dataset.nodeId;
			const dir = portEl.dataset.dir;
			const side = portEl.dataset.side;
			showMenu([
				{ label: `本侧再加一个${dir === "in" ? "输入" : "输出"}端口`, icon: "fa-plus", onSelect: () => actions.addPort(nodeRef, dir, side) },
				{ label: "删除本侧该方向的一个端口", icon: "fa-minus", onSelect: () => actions.removePort(nodeRef, dir, side) },
				{ separator: true },
				{ label: "清空该节点全部端口连线", icon: "fa-unlink", danger: true, onSelect: () => removeEdgesOfNode(nodeRef) },
			], event.clientX, event.clientY);
			return;
		}

		if (edgeId) {
			if (!store.isEdgeSelected(edgeId)) store.select({ edges: [edgeId] }, "replace");
			const edge = store.graph.getEdge(edgeId);
			const items = [
				{ label: "编辑连线标签", icon: "fa-tag", hint: "双击", onSelect: () => textEditor.beginLabel(edgeId) },
				{ label: "翻转方向（尾部 ⇄ 头部）", icon: "fa-right-left", onSelect: () => actions.reverseEdge(edgeId) },
				{ separator: true },
			];
			for (const style of EDGE_STYLES) {
				items.push({
					label: `线型：${EDGE_STYLE_LABEL[style]}`,
					icon: "fa-bezier-curve",
					disabled: edge?.style === style,
					onSelect: () => actions.patchEdges([edgeId], { style }, "修改线型"),
				});
			}
			for (const mode of ARROW_MODES) {
				items.push({
					label: `箭头：${ARROW_MODE_LABEL[mode]}`,
					icon: "fa-arrow-right",
					disabled: edge?.arrow === mode,
					onSelect: () => actions.patchEdges([edgeId], { arrow: mode }, "修改箭头"),
				});
			}
			items.push(
				{ label: edge?.dashed ? "取消虚线" : "改为虚线", icon: "fa-ellipsis", onSelect: () => actions.patchEdges([edgeId], { dashed: !edge?.dashed }, "切换虚线") },
				{ separator: true },
				{ label: "删除连线", icon: "fa-trash-can", danger: true, onSelect: () => actions.delete() },
			);
			showMenu(items, event.clientX, event.clientY);
			return;
		}

		if (nodeId) {
			if (!store.isNodeSelected(nodeId)) store.select({ nodes: [nodeId] }, "replace");
			const node = store.graph.getNode(nodeId);
			showMenu([
				{ label: "编辑节点文字", icon: "fa-font", hint: "双击", onSelect: () => textEditor.beginText(nodeId) },
				{ label: "编辑节点标题", icon: "fa-heading", onSelect: () => textEditor.beginTitle(nodeId) },
				{ separator: true },
				{ label: "再制一份", icon: "fa-clone", hint: "Ctrl+D", onSelect: () => actions.duplicate() },
				{ label: "复制", icon: "fa-copy", hint: "Ctrl+C", onSelect: () => actions.copy() },
				{ separator: true },
				{ label: "加宽 20", icon: "fa-arrows-left-right", onSelect: () => actions.patchNodes([nodeId], { w: node.w + 20 }, "调整宽度") },
				{ label: "加高 20", icon: "fa-arrows-up-down", onSelect: () => actions.patchNodes([nodeId], { h: node.h + 20 }, "调整高度") },
				{ label: "高度贴合文字", icon: "fa-text-height", onSelect: () => actions.fitToContent([nodeId]) },
				{ label: node?.collapsed ? "展开正文" : "折叠正文", icon: "fa-chevron-up", onSelect: () => actions.toggleCollapse(nodeId) },
				{ label: node?.locked ? "解锁位置" : "锁定位置", icon: "fa-lock", onSelect: () => actions.toggleLock([nodeId]) },
				{ separator: true },
				{ label: "添加端口…", icon: "fa-plug", onSelect: () => addPortMenu(nodeId, event.clientX, event.clientY) },
				{ label: "置于顶层", icon: "fa-layer-group", onSelect: () => actions.bringToFront() },
				{ label: "置于底层", icon: "fa-layer-group", onSelect: () => actions.sendToBack() },
				{ separator: true },
				{ label: "删除节点", icon: "fa-trash-can", danger: true, hint: "Delete", onSelect: () => actions.delete() },
			], event.clientX, event.clientY);
			return;
		}

		const world = viewport.screenToWorld(event.clientX, event.clientY);
		showMenu([
			{ label: "在此新建节点", icon: "fa-plus", onSelect: () => createMenuAt(world, event.clientX, event.clientY) },
			{ separator: true },
			{ label: "粘贴", icon: "fa-paste", hint: "Ctrl+V", disabled: !commands.clipboard, onSelect: () => actions.paste() },
			{ label: "全选", icon: "fa-object-group", hint: "Ctrl+A", onSelect: () => actions.selectAll() },
			{ separator: true },
			{ label: "自动布局（纵向）", icon: "fa-sitemap", onSelect: () => actions.layoutTB() },
			{ label: "自动布局（横向）", icon: "fa-sitemap", onSelect: () => actions.layoutLR() },
			{ label: "适应全部节点", icon: "fa-expand", hint: "F", onSelect: () => actions.fit() },
		], event.clientX, event.clientY);
	};

	function addPortMenu(nodeId, clientX, clientY) {
		const node = store.graph.getNode(nodeId);
		const items = [];
		for (const side of SIDES) {
			for (const dir of ["in", "out"]) {
				items.push({
					label: `${SIDE_LABEL[side]}侧 + 1 个${dir === "in" ? "输入" : "输出"}端口（现有 ${portCount(node, dir, side)}）`,
					icon: dir === "in" ? "fa-arrow-right-to-bracket" : "fa-arrow-right-from-bracket",
					onSelect: () => actions.addPort(nodeId, dir, side),
				});
			}
		}
		showMenu(items, clientX, clientY);
	}

	function createMenuAt(world, clientX, clientY) {
		showMenu(
			NODE_TYPES.map((type) => ({
				label: `新建「${type.label}」`,
				icon: type.icon,
				onSelect: () => {
					commands.addNode(type.key, world);
					render();
				},
			})),
			clientX,
			clientY,
		);
	}

	function removeEdgesOfNode(nodeId) {
		const ids = store.graph.edgesOf(nodeId).map((e) => e.id);
		if (!ids.length) {
			toast("该节点还没有连线", "info");
			return;
		}
		commands.run(`清空 ${ids.length} 条连线`, () => store.graph.removeEdges(ids));
		render();
	}

	// ------------------------------------------------------------ 快捷键

	window.addEventListener("keydown", (event) => {
		if (isTextInput(event.target)) return;
		if (textEditor.isEditing) return;
		const mod = event.ctrlKey || event.metaKey;
		const key = event.key;

		if (key === "Escape") {
			closeMenu();
			store.clearSelection();
			if (store.ui.activePort) {
				store.setUI({ activePort: null, wirePreview: null, mode: "idle" });
				store.emit("ui");
			}
			render();
			return;
		}
		if (!mod && (key === "Delete" || key === "Backspace")) {
			event.preventDefault();
			actions.delete();
			return;
		}
		if (mod && key.toLowerCase() === "z" && !event.shiftKey) {
			event.preventDefault();
			actions.undo();
			return;
		}
		if (mod && (key.toLowerCase() === "y" || (key.toLowerCase() === "z" && event.shiftKey))) {
			event.preventDefault();
			actions.redo();
			return;
		}
		if (mod && key.toLowerCase() === "a") {
			event.preventDefault();
			actions.selectAll();
			return;
		}
		if (mod && key.toLowerCase() === "c") {
			event.preventDefault();
			actions.copy();
			return;
		}
		if (mod && key.toLowerCase() === "x") {
			event.preventDefault();
			actions.cut();
			render();
			return;
		}
		if (mod && key.toLowerCase() === "v") {
			event.preventDefault();
			actions.paste();
			return;
		}
		if (mod && key.toLowerCase() === "d") {
			event.preventDefault();
			actions.duplicate();
			return;
		}
		if (mod && key.toLowerCase() === "s") {
			event.preventDefault();
			actions.saveServer();
			return;
		}
		if (mod && (key === "0")) {
			event.preventDefault();
			actions.zoomReset();
			render();
			return;
		}
		if (mod && (key === "=" || key === "+")) {
			event.preventDefault();
			actions.zoomIn();
			return;
		}
		if (mod && key === "-") {
			event.preventDefault();
			actions.zoomOut();
			return;
		}
		if (mod && key === "1") {
			event.preventDefault();
			actions.fit();
			return;
		}
		if (!mod && (key === "f" || key === "F")) {
			event.preventDefault();
			actions.fit();
			return;
		}
		if (!mod && (key === "l" || key === "L")) {
			event.preventDefault();
			actions.toggleLock();
			return;
		}
		if (!mod && (key === "g" || key === "G")) {
			event.preventDefault();
			actions.toggleGrid();
			return;
		}
		if (key.startsWith("Arrow")) {
			if (store.selection.nodes.size === 0) return;
			event.preventDefault();
			const step = event.shiftKey ? 10 : 1;
			const dx = key === "ArrowLeft" ? -step : key === "ArrowRight" ? step : 0;
			const dy = key === "ArrowUp" ? -step : key === "ArrowDown" ? step : 0;
			commands.translateNodes([...store.selection.nodes], dx, dy);
			commands.commit("移动节点");
			render();
		}
	});

	// ------------------------------------------------------------ 状态订阅

	store.on("graph", () => {
		persistence.scheduleAutosave();
		render();
	});
	store.on("selection", () => {
		render();
	});
	store.on("settings", () => {
		viewport.apply();
		toolbar.refresh();
	});
	store.on("document", () => {
		statusbar.refresh();
		toolbar.refresh();
	});
	store.on("viewport", () => {
		viewport.apply();
		minimap.schedule();
		statusbar.refresh();
		toolbar.refresh();
	});
	store.on("ui", () => {
		renderOverlayOnly();
	});

	// 面板与检查器在每次渲染后自动刷新，这里只需保证初始视图正确
	viewport.apply();
	dom.minimap.classList.toggle("is-hidden", !store.settings.showMinimap);
	if (store.graph.nodes.length > 0) {
		viewport.fit(viewport.graphBounds(store.graph), 120);
	}
	render();
	palette.refreshDocs();

	// 关闭页面前落一次草稿
	window.addEventListener("beforeunload", () => {
		persistence.saveLocal();
	});

	// 暴露给调试与自动化（也方便在琉璃 DevTools 里验证几何计算）
	window.NodeGraphStudio = { store, commands, history, renderer, viewport, minimap, textEditor, interaction, persistence, actions };

	toast("节点图工坊已就绪：拖节点、连端口、双击写字", "ok");
}

if (document.readyState === "loading") {
	document.addEventListener("DOMContentLoaded", boot);
} else {
	boot();
}
