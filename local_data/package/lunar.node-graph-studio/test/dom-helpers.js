/**
 * DOM 级端到端用例：在真实浏览器里加载 index.html（iframe，同源），
 * 用合成指针事件驱动画布，验证「拖放节点 / 四向连线 / 编辑文字 / 连线方向 / 导出」。
 *
 * 这一层刻意不复用 harness 的纯逻辑用例——它测的是「接线是否真的通」：
 * 事件 → 交互状态机 → 命令层 → 模型 → 渲染器 → DOM 属性。
 */

import { buildSVG, renderPNGCanvas } from "../js/io/exportfile.js";
import { SIDES, SIDE_VECTOR } from "../js/core/ports.js";

/** 把断言结果收集起来，不抛出（一条失败不影响后续用例） */
export class DomRunner {
	constructor() {
		this.results = [];
	}

	async test(name, fn) {
		const started = performance.now();
		try {
			await fn();
			this.results.push({ name, ok: true, ms: performance.now() - started });
		} catch (err) {
			this.results.push({ name, ok: false, ms: performance.now() - started, error: err });
		}
	}

	get passed() {
		return this.results.filter((r) => r.ok).length;
	}

	get failed() {
		return this.results.filter((r) => !r.ok).length;
	}
}

export function fail(message) {
	throw new Error(message);
}

export function ok(condition, message) {
	if (!condition) fail(message);
}

export function eq(actual, expected, message = "") {
	if (actual !== expected) fail(`${message || "值不相等"}：期望 ${JSON.stringify(expected)}，实际 ${JSON.stringify(actual)}`);
}

export function near(actual, expected, tolerance, message = "") {
	if (!(Math.abs(actual - expected) <= tolerance)) {
		fail(`${message || "数值不接近"}：期望 ${expected}±${tolerance}，实际 ${actual}`);
	}
}

/** 等待 n 帧，确保 rAF 调度的渲染已落地 */
export async function settle(win, frames = 3) {
	for (let i = 0; i < frames; i++) {
		await new Promise((resolve) => win.requestAnimationFrame(() => resolve()));
	}
	await new Promise((resolve) => setTimeout(resolve, 0));
}

function pointerEvent(win, type, init) {
	const options = {
		bubbles: true,
		cancelable: true,
		composed: true,
		pointerId: 1,
		pointerType: "mouse",
		isPrimary: true,
		button: 0,
		buttons: type === "pointerup" ? 0 : 1,
		clientX: init.clientX,
		clientY: init.clientY,
	};
	try {
		return new win.PointerEvent(type, options);
	} catch (err) {
		return new win.MouseEvent(type, options);
	}
}

function centerOf(element) {
	const rect = element.getBoundingClientRect();
	return { clientX: rect.left + rect.width / 2, clientY: rect.top + rect.height / 2 };
}

export { centerOf };

export function firePointer(win, target, type, point) {
	target.dispatchEvent(pointerEvent(win, type, point));
}

export function fireMouse(win, target, type, point = {}) {
	target.dispatchEvent(new win.MouseEvent(type, {
		bubbles: true,
		cancelable: true,
		composed: true,
		clientX: point.clientX || 0,
		clientY: point.clientY || 0,
	}));
}

export function fireKey(win, target, type, key, extra = {}) {
	target.dispatchEvent(new win.KeyboardEvent(type, { key, bubbles: true, cancelable: true, ...extra }));
}

/** 解析 SVG transform 里的旋转角（度） */
export function rotationOf(element) {
	const transform = element.getAttribute("transform") || "";
	const match = /rotate\(([-\d.]+)/.exec(transform);
	if (!match) fail(`箭头缺少 rotate：${transform}`);
	return Number(match[1]);
}

export function angleVector(deg) {
	const rad = (deg * Math.PI) / 180;
	return { x: Math.cos(rad), y: Math.sin(rad) };
}

/** 两向量夹角对齐度（点积，1 表示同向） */
export function alignment(a, b) {
	return a.x * b.x + a.y * b.y;
}

/**
 * 在 iframe 里创建两个节点并摆到屏幕可视区域。
 * 视口刻意下移，避免上侧端口被顶栏遮住（顶栏会挡住 elementFromPoint）。
 * @returns {{a:object,b:object,idA:string,idB:string}}
 */
export function placePair(app, worldA = { x: 0, y: 0 }, worldB = { x: 460, y: 40 }) {
	const a = app.commands.addNode("process", worldA);
	const b = app.commands.addNode("decision", worldB);
	app.store.setViewport({ x: 190, y: 170, zoom: 1 });
	app.renderer.renderGraph();
	return { a, b, idA: a.id, idB: b.id };
}

/** 通过合成指针从 outPort 拖到 inPort，完成一次连线 */
export async function dragWire(win, outPort, inPort) {
	const from = centerOf(outPort);
	const to = centerOf(inPort);
	firePointer(win, outPort, "pointerdown", from);
	firePointer(win, win.document.getElementById("canvas"), "pointermove", to);
	firePointer(win, win.document.getElementById("canvas"), "pointerup", to);
	await settle(win, 2);
}

/** iframe 里的应用是否已就绪 */
export function appOf(iframe) {
	return iframe.contentWindow && iframe.contentWindow.NodeGraphStudio;
}

export async function waitForApp(iframe, timeoutMs = 15000) {
	const started = Date.now();
	while (Date.now() - started < timeoutMs) {
		const app = appOf(iframe);
		if (app) return app;
		await new Promise((resolve) => setTimeout(resolve, 60));
	}
	fail("应用未在超时内完成初始化（window.NodeGraphStudio 未出现）");
	return null;
}

export { buildSVG, renderPNGCanvas, SIDES, SIDE_VECTOR };
