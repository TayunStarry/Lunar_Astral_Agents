/**
 * 底栏状态条：左侧随模式变化的操作提示，中间统计，右侧缩放/坐标/保存状态。
 */

import { el } from "./dom.js";

const HINTS = {
	idle: "拖动节点自由摆放 · 从端口拖出连线（上下左右皆可） · 双击节点编辑文字 · 双击连线编辑标签",
	dragNode: "拖动中：按住 Shift 可沿单轴移动，按住 Alt 临时关闭吸附",
	dragWire: "连线中：松开在目标端口上完成连接；落在空白处可新建节点并连上",
	marquee: "框选中：松开完成选择，按住 Shift 追加选择",
	pan: "平移中：松开结束（也可用空格 + 拖动或鼠标中键）",
	resize: "调整尺寸中：节点尺寸会吸附到栅格",
};

export class StatusBar {
	constructor({ host, store }) {
		this.host = host;
		this.store = store;
		this.render();
	}

	render() {
		this.host.textContent = "";
		this.hintEl = el("div", { class: "status-hint" });
		this.statsEl = el("div", { class: "status-stats" });
		this.zoomEl = el("div", { class: "status-item status-zoom" });
		this.cursorEl = el("div", { class: "status-item status-cursor" });
		this.saveEl = el("div", { class: "status-item status-save" });
		this.host.append(
			this.hintEl,
			el("div", { class: "status-spacer" }),
			this.statsEl,
			this.zoomEl,
			this.cursorEl,
			this.saveEl,
		);
		this.refresh();
	}

	setHint(text) {
		if (this.hintEl.textContent !== text) this.hintEl.textContent = text;
	}

	refresh() {
		const store = this.store;
		const mode = store.ui.mode;
		this.setHint(HINTS[mode] || HINTS.idle);

		const { nodes, edges } = store.graph.stats();
		const selectedNodes = store.selection.nodes.size;
		const selectedEdges = store.selection.edges.size;
		const selected = selectedNodes + selectedEdges > 0
			? ` · 已选 ${selectedNodes ? `${selectedNodes} 节点` : ""}${selectedNodes && selectedEdges ? " / " : ""}${selectedEdges ? `${selectedEdges} 连线` : ""}`
			: "";
		const text = `节点 ${nodes} · 连线 ${edges}${selected}`;
		if (this.statsEl.textContent !== text) this.statsEl.textContent = text;

		this.zoomEl.textContent = `缩放 ${Math.round(store.viewport.zoom * 100)}%`;
		const cursor = store.ui.cursor;
		this.cursorEl.textContent = `${Math.round(cursor.x)}, ${Math.round(cursor.y)}`;

		const dirty = store.ui.dirty;
		this.saveEl.classList.toggle("is-dirty", dirty);
		this.saveEl.innerHTML = dirty
			? '<i class="fas fa-circle-dot"></i> 未保存'
			: '<i class="fas fa-circle-check"></i> 已保存';
		this.saveEl.title = dirty ? "有改动尚未保存（Ctrl+S 保存到本地，或等待自动保存草稿）" : "当前内容已保存";
	}
}
