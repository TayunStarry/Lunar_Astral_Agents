/**
 * DOM 级端到端用例（在真实 Chromium 里加载 index.html 并驱动它）。
 * 用 test/dom.html 运行；也可用无头浏览器 --dump-dom 采集结果。
 */

import {
	DomRunner, fail, ok, eq, near, settle, firePointer, fireMouse, fireKey, centerOf,
	rotationOf, angleVector, alignment, placePair, dragWire, waitForApp,
	buildSVG, renderPNGCanvas, SIDES, SIDE_VECTOR,
} from "./dom-helpers.js";

const REPORT = { consoleErrors: [] };

/** 在 iframe 里创建两个节点、连一条线，返回后续断言需要的信息 */
function connectPair(win, app, fromSide, toSide) {
	const { a, b } = placePair(app);
	const nodeA = win.document.querySelector(`.node[data-node-id="${a.id}"]`);
	const nodeB = win.document.querySelector(`.node[data-node-id="${b.id}"]`);
	const outPort = nodeA.querySelector(`.port[data-dir="out"][data-side="${fromSide}"][data-index="0"]`);
	const inPort = nodeB.querySelector(`.port[data-dir="in"][data-side="${toSide}"][data-index="0"]`);
	if (!outPort || !inPort) fail(`找不到端口元素：out/${fromSide} 或 in/${toSide}`);
	return { a, b, nodeA, nodeB, outPort, inPort };
}

export async function runDomSuite(iframe) {
	const runner = new DomRunner();
	const win = iframe.contentWindow;
	const doc = win.document;
	const $ = (sel) => doc.querySelector(sel);
	const $$ = (sel) => [...doc.querySelectorAll(sel)];

	const app = await waitForApp(iframe);
	const { store, commands, actions, renderer, viewport, textEditor, persistence } = app;

	// ------------------------------------------------------------ 启动与结构

	await runner.test("应用启动：核心装配齐备", async () => {
		for (const key of ["store", "commands", "history", "renderer", "viewport", "minimap", "textEditor", "interaction", "persistence", "actions"]) {
			ok(app[key], `NodeGraphStudio.${key} 应存在`);
		}
		ok($("#canvas"), "画布容器存在");
		ok($("#edgeLayer"), "连线 SVG 层存在");
		ok($("#nodeLayer"), "节点层存在");
		ok($("#minimapCanvas"), "缩略图 canvas 存在");
	});

	await runner.test("顶栏与两侧面板都已构建", async () => {
		const buttons = $$("#appBar .btn");
		ok(buttons.length >= 20, `顶栏按钮应足够多，实际 ${buttons.length}`);
		eq($$("#paletteHost .type-item").length, 7, "左栏节点类型应有 7 种");
		ok($("#inspectorHost .panel"), "右栏检查器应有面板");
		ok($("#statusBar .status-hint"), "状态栏应有提示文本");
	});

	await runner.test("启动期间没有脚本错误", async () => {
		eq(REPORT.consoleErrors.length, 0, `捕获到脚本错误：${REPORT.consoleErrors.join(" | ")}`);
	});

	// ------------------------------------------------------------ 拖放节点

	await runner.test("新建节点：DOM 元素与四向端口都生成", async () => {
		commands.newDocument("DOM 测试");
		await settle(win);
		const node = commands.addNode("process", { x: 200, y: 160 });
		await settle(win);
		const el = doc.querySelector(`.node[data-node-id="${node.id}"]`);
		ok(el, "应渲染出节点元素");
		eq(el.querySelectorAll(".port").length, 8, "应渲染四侧 × 输入输出 = 8 个端口");
		for (const side of SIDES) {
			eq(el.querySelectorAll(`.port[data-side="${side}"][data-dir="in"]`).length, 1, `${side} 侧应有 1 个输入口`);
			eq(el.querySelectorAll(`.port[data-side="${side}"][data-dir="out"]`).length, 1, `${side} 侧应有 1 个输出口`);
		}
		// 端口锚点应贴合节点四边
		const nodeRect = el.getBoundingClientRect();
		for (const side of SIDES) {
			const port = el.querySelector(`.port[data-side="${side}"][data-dir="out"]`);
			const r = port.getBoundingClientRect();
			const cx = r.left + r.width / 2;
			const cy = r.top + r.height / 2;
			if (side === "top") near(cy, nodeRect.top, 1.5, "上侧端口应贴在顶边");
			if (side === "bottom") near(cy, nodeRect.bottom, 1.5, "下侧端口应贴在底边");
			if (side === "left") near(cx, nodeRect.left, 1.5, "左侧端口应贴在左边");
			if (side === "right") near(cx, nodeRect.right, 1.5, "右侧端口应贴在右边");
		}
	});

	await runner.test("拖拽过程中节点实时跟随指针（而不是松手才跳过去）", async () => {
		commands.newDocument("拖动跟随");
		await settle(win);
		// 先关掉吸附，专测「跟随」本身
		store.setSettings({ snapGrid: false, snapAlign: false });
		const node = commands.addNode("process", { x: 0, y: 0 });
		store.setViewport({ x: 320, y: 280, zoom: 1 });
		await settle(win, 2);
		const el = doc.querySelector(`.node[data-node-id="${node.id}"]`);
		const head = el.querySelector(".node-head");
		const base = el.getBoundingClientRect();
		const start = { clientX: base.left + 60, clientY: base.top + 16 };

		firePointer(win, head, "pointerdown", start);
		const steps = [40, 80, 120];
		const observed = [];
		for (const step of steps) {
			firePointer(win, $("#canvas"), "pointermove", { clientX: start.clientX + step, clientY: start.clientY + step / 2 });
			await settle(win, 2);
			const rect = el.getBoundingClientRect();
			observed.push({ step, dx: rect.left - base.left, dy: rect.top - base.top });
		}
		firePointer(win, $("#canvas"), "pointerup", { clientX: start.clientX + 120, clientY: start.clientY + 60 });
		await settle(win, 2);

		const trail = observed.map((o) => `${o.step}px→${o.dx.toFixed(1)}px`).join("，");
		ok(observed[0].dx > 20, `第一次移动后节点就应跟着走（实测位移 ${observed[0].dx.toFixed(1)}px；${trail}）`);
		ok(observed[1].dx > observed[0].dx + 20, `继续移动应继续跟随（${trail}）`);
		for (const o of observed) {
			near(o.dx, o.step, 2, `指针右移 ${o.step}px，节点应右移约 ${o.step}px（实测 ${o.dx.toFixed(1)}px；${trail}）`);
			near(o.dy, o.step / 2, 2, `指针下移 ${o.step / 2}px，节点应下移约 ${o.step / 2}px（实测 ${o.dy.toFixed(1)}px）`);
		}
	});

	await runner.test("开启栅格吸附后仍逐帧跟随（只是按栅格量化，不会停在原地）", async () => {
		commands.newDocument("栅格拖动");
		await settle(win);
		store.setSettings({ snapGrid: true, snapAlign: false });
		const node = commands.addNode("process", { x: 0, y: 0 });
		store.setViewport({ x: 320, y: 280, zoom: 1 });
		await settle(win, 2);
		const el = doc.querySelector(`.node[data-node-id="${node.id}"]`);
		const head = el.querySelector(".node-head");
		const base = el.getBoundingClientRect();
		const start = { clientX: base.left + 60, clientY: base.top + 16 };
		const grid = store.settings.grid;

		firePointer(win, head, "pointerdown", start);
		const gridTrail = [];
		for (const step of [grid, grid * 2, grid * 3]) {
			firePointer(win, $("#canvas"), "pointermove", { clientX: start.clientX + step, clientY: start.clientY });
			await settle(win, 2);
			gridTrail.push({ step, dx: el.getBoundingClientRect().left - base.left });
		}
		firePointer(win, $("#canvas"), "pointerup", { clientX: start.clientX + grid * 3, clientY: start.clientY });
		await settle(win, 2);

		const trail = gridTrail.map((o) => `${o.step}px→${o.dx.toFixed(1)}px`).join("，");
		gridTrail.forEach((o, i) => {
			ok(o.dx > grid * (i + 0.4), `第 ${i + 1} 次移动后应已按栅格前进（${trail}）`);
			near(o.dx, o.step, grid, `栅格吸附下的位移应与指针接近且不超过一个栅格（${trail}）`);
		});
		store.setSettings({ snapGrid: false, snapAlign: true });
	});

	await runner.test("拖角改尺寸时实时跟随指针", async () => {
		commands.newDocument("尺寸跟随");
		await settle(win);
		store.setSettings({ snapGrid: false, snapAlign: false });
		const node = commands.addNode("process", { x: 0, y: 0 });
		store.setViewport({ x: 320, y: 280, zoom: 1 });
		await settle(win, 2);
		const el = doc.querySelector(`.node[data-node-id="${node.id}"]`);
		const handle = el.querySelector(".node-resize");
		ok(handle, "应有右下角尺寸手柄");
		const base = el.getBoundingClientRect();
		const grip = { clientX: base.right, clientY: base.bottom };
		const w0 = node.w;
		const h0 = node.h;

		firePointer(win, handle, "pointerdown", grip);
		firePointer(win, $("#canvas"), "pointermove", { clientX: grip.clientX + 60, clientY: grip.clientY + 40 });
		await settle(win, 2);
		const during = el.getBoundingClientRect();
		ok(during.width > base.width + 40, `拖动中宽度就应跟着变（${base.width.toFixed(0)}→${during.width.toFixed(0)}）`);
		ok(during.height > base.height + 25, `拖动中高度就应跟着变（${base.height.toFixed(0)}→${during.height.toFixed(0)}）`);
		near(node.w, w0 + 60, 3, "模型宽度应约等于指针位移增量");
		near(node.h, h0 + 40, 3, "模型高度应约等于指针位移增量");

		firePointer(win, $("#canvas"), "pointerup", { clientX: grip.clientX + 60, clientY: grip.clientY + 40 });
		await settle(win, 2);
		ok(el.getBoundingClientRect().width > base.width + 40, "松手后尺寸应保持");
	});

	await runner.test("拖拽落位可撤销：一次拖动只进一条历史", async () => {
		commands.newDocument("拖动撤销");
		await settle(win);
		const node = commands.addNode("process", { x: 0, y: 0 });
		store.setViewport({ x: 320, y: 280, zoom: 1 });
		await settle(win, 2);
		const el = doc.querySelector(`.node[data-node-id="${node.id}"]`);
		const head = el.querySelector(".node-head");
		const rect = el.getBoundingClientRect();
		const start = { clientX: rect.left + 50, clientY: rect.top + 16 };
		const origin = { x: node.x, y: node.y };
		const historyBefore = app.history.length;

		firePointer(win, head, "pointerdown", start);
		for (const step of [30, 60, 90, 120]) {
			firePointer(win, $("#canvas"), "pointermove", { clientX: start.clientX + step, clientY: start.clientY + step });
			await settle(win, 1);
		}
		ok(app.history.length === historyBefore, `拖动过程不应写入历史（拖动中新增 ${app.history.length - historyBefore} 条）`);
		firePointer(win, $("#canvas"), "pointerup", { clientX: start.clientX + 120, clientY: start.clientY + 120 });
		await settle(win, 2);
		eq(app.history.length, historyBefore + 1, "整次拖动只应写入 1 条历史");
		ok(node.x !== origin.x || node.y !== origin.y, "节点应已移动");

		actions.undo();
		await settle(win, 2);
		eq(store.graph.getNode(node.id).x, origin.x, "撤销应恢复 x");
		eq(store.graph.getNode(node.id).y, origin.y, "撤销应恢复 y");
	});

	// ------------------------------------------------------------ 四向连线（核心要求）

	await runner.test("四向端口两两连线：16 种侧向组合都能连成且方向为 out→in", async () => {
		commands.newDocument("四向连线");
		await settle(win);
		for (const fromSide of SIDES) {
			for (const toSide of SIDES) {
				const { a, b, outPort, inPort } = connectPair(win, app, fromSide, toSide);
				const drop = centerOf(inPort);
				const hit = doc.elementFromPoint(drop.clientX, drop.clientY);
				const hitPort = hit && hit.closest ? hit.closest(".port") : null;
				const diagnosis = `落点(${Math.round(drop.clientX)},${Math.round(drop.clientY)}) 命中=${describeElement(hit)}`
					+ ` 命中端口=${hitPort ? JSON.stringify(hitPort.dataset) : "无"}`
					+ ` DOM 节点数=${$$(".node").length} 模型节点数=${store.graph.nodes.length}`
					+ ` 视口=${JSON.stringify(store.viewport)} 模式=${store.ui.mode}`;
				await dragWire(win, outPort, inPort);
				const edges = store.graph.edges;
				eq(edges.length, 1, `${fromSide}→${toSide} 应恰好产生 1 条连线（${diagnosis}）`);
				const edge = edges[0];
				eq(edge.from.dir, "out", `${fromSide}→${toSide} 尾部必须是输出口`);
				eq(edge.to.dir, "in", `${fromSide}→${toSide} 头部必须是输入口`);
				eq(edge.from.side, fromSide, `${fromSide}→${toSide} 尾部侧向应保持`);
				eq(edge.to.side, toSide, `${fromSide}→${toSide} 头部侧向应保持`);
				// 清理，进入下一组合
				store.select({ nodes: [a.id, b.id] }, "replace");
				commands.deleteSelection();
				await settle(win, 2);
				eq(store.graph.edges.length, 0, "清理后不应残留连线");
			}
		}
	});

	await runner.test("连线方向显示：箭头旋转角与「指进目标端口」一致（16 种组合）", async () => {
		commands.newDocument("箭头方向");
		await settle(win);
		let checked = 0;
		for (const fromSide of SIDES) {
			for (const toSide of SIDES) {
				const { a, b, outPort, inPort } = connectPair(win, app, fromSide, toSide);
				await dragWire(win, outPort, inPort);
				const edge = store.graph.edges[0];
				ok(edge, `${fromSide}→${toSide} 应有连线`);
				const group = doc.querySelector(`.edge[data-edge-id="${edge.id}"]`);
				ok(group, "应渲染出连线分组");
				const line = group.querySelector(".edge-line");
				ok(line && line.getAttribute("d"), "连线应有路径 d");
				const arrow = group.querySelector(".edge-arrow-to");
				ok(arrow, "应有终点箭头元素");
				ok(arrow.style.display !== "none", "终点箭头应可见");
				eq(arrow.getAttribute("d"), group.querySelector(".edge-arrow-from").getAttribute("d"), "箭头多边形共用同一路径");
				const angle = rotationOf(arrow);
				const inward = { x: -SIDE_VECTOR[toSide].x, y: -SIDE_VECTOR[toSide].y };
				const dot = alignment(angleVector(angle), inward);
				ok(dot > 0.95, `${fromSide}→${toSide}：箭头应对准 ${toSide} 侧法向，实测对齐度 ${dot.toFixed(4)}`);
				checked += 1;
				store.select({ nodes: [a.id, b.id] }, "replace");
				commands.deleteSelection();
				await settle(win, 1);
			}
		}
		eq(checked, 16, "应覆盖全部 16 种侧向组合");
	});

	await runner.test("连线其他形式：双向箭头、虚线、直角与直线样式都能落到 DOM", async () => {
		commands.newDocument("线型");
		await settle(win);
		const { a, b, outPort, inPort } = connectPair(win, app, "right", "left");
		await dragWire(win, outPort, inPort);
		const edge = store.graph.edges[0];
		const group = () => doc.querySelector(`.edge[data-edge-id="${edge.id}"]`);

		actions.patchEdges([edge.id], { arrow: "both" }, "箭头");
		await settle(win);
		ok(group().querySelector(".edge-arrow-from").style.display !== "none", "双向箭头应显示尾箭头");
		ok(group().querySelector(".edge-arrow-to").style.display !== "none", "双向箭头应显示头箭头");

		actions.patchEdges([edge.id], { arrow: "none" }, "箭头");
		await settle(win);
		eq(group().querySelector(".edge-arrow-to").style.display, "none", "无箭头模式应隐藏头箭头");

		actions.patchEdges([edge.id], { arrow: "to", dashed: true, style: "ortho" }, "线型");
		await settle(win);
		ok(group().classList.contains("is-dashed"), "应加上虚线样式类");
		eq(group().dataset.style, "ortho", "应切换到直角样式");

		actions.patchEdges([edge.id], { style: "straight" }, "线型");
		await settle(win);
		eq(group().dataset.style, "straight", "应切换到直线样式");

		// 翻转方向：流向对调、恒保持 out→in；端口取同侧，因此走廊不变、只有箭头调头
		const beforeFrom = { ...edge.from };
		const beforeTo = { ...edge.to };
		actions.reverseEdge(edge.id);
		await settle(win);
		const reversed = store.graph.edges[0];
		eq(reversed.from.node, beforeTo.node, "翻转后尾部应是原头部所在节点");
		eq(reversed.from.side, beforeTo.side, "翻转后尾部应取同侧端口（走廊不变）");
		eq(reversed.from.dir, "out", "翻转后尾部仍应是输出口");
		eq(reversed.to.node, beforeFrom.node, "翻转后头部应是原尾部所在节点");
		eq(reversed.to.side, beforeFrom.side, "翻转后头部应取同侧端口（走廊不变）");
		eq(reversed.to.dir, "in", "翻转后头部仍应是输入口");
		// 箭头随之调头：直线样式下箭头必须沿连线指向头部，且落在指向节点内部的一侧
		{
			const groupNow = group();
			const arrow = groupNow.querySelector(".edge-arrow-to");
			const angle = rotationOf(arrow);
			const route = renderer.routeOf(reversed.id);
			ok(route, "应能取到连线几何");
			const expected = Math.atan2(route.end.y - route.start.y, route.end.x - route.start.x) * (180 / Math.PI);
			ok(angleDelta(angle, expected) < 0.5, `直线样式的箭头应沿连线指向头部（实测 ${angle.toFixed(2)}°，期望 ${expected.toFixed(2)}°）`);
			const inward = { x: -SIDE_VECTOR[reversed.to.side].x, y: -SIDE_VECTOR[reversed.to.side].y };
			// 直线不保证垂直入线，但绝不能反向：必须指向节点内部那一侧
			ok(alignment(angleVector(angle), inward) > 0, "翻转后箭头必须指向新头部节点内部，不得反向");
			// 箭头落点就是新头部端口锚点
			const head = portAnchorOf(store, reversed.to);
			near(route.end.x, head.x, 0.5, "箭头应落在头部端口锚点 x 上");
			near(route.end.y, head.y, 0.5, "箭头应落在头部端口锚点 y 上");
		}

		store.select({ nodes: [a.id, b.id] }, "replace");
		commands.deleteSelection();
	});

	// ------------------------------------------------------------ 文字编辑（核心要求）

	await runner.test("「高度贴合文字」可增可减，且重复调用不再变化（不会越点越大）", async () => {
		commands.newDocument("贴合高度");
		await settle(win);
		const node = commands.addNode("process", { x: 0, y: 0 });
		store.select({ nodes: [node.id] }, "replace");
		await settle(win, 2);
		const preset = node.h;

		// 正文只有一行，贴合后应当收紧
		const heights = [];
		for (let i = 0; i < 4; i++) {
			actions.fitToContent([node.id]);
			await settle(win, 2);
			heights.push(node.h);
		}
		const trail = heights.join(" → ");
		ok(heights[0] < preset, `一行文字应把预设高度收紧（预设 ${preset} → ${heights[0]}；${trail}）`);
		ok(heights[1] === heights[0] && heights[3] === heights[0], `重复调用高度必须稳定不变（${trail}）`);
		ok(node.h <= 34 + 16 + 6 + 20 + 2, `一行文字的节点高度应贴近一行的高度（实测 ${node.h}）`);

		// 文字变多 → 变高；再点 → 稳定
		actions.patchNodes([node.id], { text: "第一行\n第二行\n第三行\n第四行\n第五行" }, "改文字");
		await settle(win, 2);
		actions.fitToContent([node.id]);
		await settle(win, 2);
		const tall = node.h;
		ok(tall > heights[0], `文字变多后应变高（${heights[0]} → ${tall}）`);
		actions.fitToContent([node.id]);
		await settle(win, 2);
		ok(node.h === tall, `文字未变时再次贴合不应继续变高（${tall} → ${node.h}）`);

		// 文字变少 → 变矮
		actions.patchNodes([node.id], { text: "短" }, "改文字");
		await settle(win, 2);
		actions.fitToContent([node.id]);
		await settle(win, 2);
		ok(node.h < tall, `文字变少后应变矮（${tall} → ${node.h}）`);
	});

	await runner.test("就地编辑正文：高度随内容收放，不会每敲一下就往上顶", async () => {
		commands.newDocument("编辑收放");
		await settle(win);
		const node = commands.addNode("process", { x: 0, y: 0 });
		await settle(win, 2);
		const el = doc.querySelector(`.node[data-node-id="${node.id}"]`);
		fireMouse(win, el.querySelector(".node-text"), "dblclick", { clientX: 0, clientY: 0 });
		await settle(win, 1);
		const area = el.querySelector(".node-editor-text");
		ok(area, "应进入编辑态");

		// 内容不变、反复触发 input：高度必须稳定（旧实现每帧 +2px）
		const feed = async (value, times) => {
			for (let i = 0; i < times; i++) {
				area.value = value;
				area.dispatchEvent(new win.Event("input", { bubbles: true }));
				await settle(win, 2);
			}
			return node.h;
		};
		const baseline = await feed("一行文字", 10);
		const again = await feed("一行文字", 10);
		ok(again === baseline, `内容不变时反复输入不应持续长高（${baseline} → ${again}）`);

		// 多行 → 变高
		const tall = await feed("一\n二\n三\n四\n五\n六", 1);
		ok(tall > baseline, `多行文字应变高（${baseline} → ${tall}）`);

		// 删回一行 → 变矮
		const shrunk = await feed("一行", 1);
		ok(shrunk < tall, `删掉多行后应变矮（${tall} → ${shrunk}）`);

		fireKey(win, area, "keydown", "Escape");
		await settle(win, 1);
	});

	await runner.test("折叠收成一条标题栏，展开后按内容恢复高度", async () => {
		commands.newDocument("折叠高度");
		await settle(win);
		const node = commands.addNode("process", { x: 0, y: 0 });
		await settle(win, 2);
		actions.fitToContent([node.id]);
		await settle(win, 2);
		const expanded = node.h;

		const tool = doc.querySelector(`.node[data-node-id="${node.id}"] .node-tool[data-act="collapse"]`);
		ok(tool, "应有折叠按钮");
		// 真实鼠标先派发 pointerdown（画布交互挂在 pointerdown 上），再派发 click
		firePointer(win, tool, "pointerdown", { clientX: 0, clientY: 0 });
		await settle(win, 2);
		eq(node.collapsed, true, "应进入折叠态");
		ok(node.h < expanded, `折叠后应收成一条标题栏（${expanded} → ${node.h}）`);
		ok(node.h <= 40, `折叠高度应贴近标题栏高度（实测 ${node.h}）`);
		const collapsedRect = doc.querySelector(`.node[data-node-id="${node.id}"]`).getBoundingClientRect();
		ok(collapsedRect.height < expanded, "折叠后的 DOM 高度也应跟着变小");

		firePointer(win, doc.querySelector(`.node[data-node-id="${node.id}"] .node-tool[data-act="collapse"]`), "pointerdown", { clientX: 0, clientY: 0 });
		await settle(win, 2);
		eq(node.collapsed, false, "应恢复展开态");
		eq(node.h, expanded, `展开后应回到内容高度（期望 ${expanded}，实际 ${node.h}）`);
	});

	await runner.test("双击节点正文：出现就地编辑器，提交后模型与 DOM 同步", async () => {
		commands.newDocument("文字编辑");
		await settle(win);
		const node = commands.addNode("process", { x: 220, y: 180 });
		await settle(win);
		const el = doc.querySelector(`.node[data-node-id="${node.id}"]`);
		const textEl = el.querySelector(".node-text");
		fireMouse(win, textEl, "dblclick", { clientX: 0, clientY: 0 });
		await settle(win, 1);
		const area = el.querySelector(".node-editor-text");
		ok(area, "双击正文后应出现 textarea 编辑器");
		eq(doc.activeElement, area, "编辑器应获得焦点");

		const longText = "第一行文字\n第二行：换行也要保留\n第三行用于触发自动增高，让节点长高一些以容纳更多内容。";
		const beforeH = node.h;
		area.value = longText;
		area.dispatchEvent(new win.Event("input", { bubbles: true }));
		await settle(win, 3);
		ok(node.h > beforeH, `文字变长后节点应自动增高（${beforeH} → ${node.h}）`);

		fireKey(win, area, "keydown", "Enter", { ctrlKey: true });
		await settle(win, 2);
		eq(store.graph.getNode(node.id).text, longText, "模型文字应更新");
		eq(el.querySelector(".node-text").textContent, longText, "DOM 文字应更新");
		eq(el.querySelector(".node-editor-text"), null, "编辑器应已移除");
		ok(!el.classList.contains("is-editing-text"), "编辑态样式应被移除");
		// 可撤销
		actions.undo();
		await settle(win, 2);
		eq(store.graph.getNode(node.id).text, "双击节点即可编辑这段文字", "撤销应回到原始文案");
	});

	await runner.test("双击节点标题：单行编辑并提交", async () => {
		const node = store.graph.nodes[0];
		const el = doc.querySelector(`.node[data-node-id="${node.id}"]`);
		fireMouse(win, el.querySelector(".node-title"), "dblclick", { clientX: 0, clientY: 0 });
		await settle(win, 1);
		const input = el.querySelector(".node-editor-title");
		ok(input, "双击标题后应出现输入框");
		input.value = "改过的标题";
		fireKey(win, input, "keydown", "Enter");
		await settle(win, 2);
		eq(store.graph.getNode(node.id).title, "改过的标题", "模型标题应更新");
		eq(el.querySelector(".node-title").textContent, "改过的标题", "DOM 标题应更新");
	});

	await runner.test("双击连线：编辑连线标签", async () => {
		const { a, b, outPort, inPort } = connectPair(win, app, "right", "left");
		await dragWire(win, outPort, inPort);
		const edge = store.graph.edges[0];
		const group = doc.querySelector(`.edge[data-edge-id="${edge.id}"]`);
		ok(group, "应有连线分组");
		fireMouse(win, group.querySelector(".edge-line"), "dblclick", { clientX: 0, clientY: 0 });
		await settle(win, 1);
		const input = doc.querySelector(".edge-label-editor");
		ok(input, "双击连线后应出现标签输入框");
		input.value = "是";
		fireKey(win, input, "keydown", "Enter");
		await settle(win, 2);
		eq(store.graph.getEdge(edge.id).label, "是", "模型标签应更新");
		const label = doc.querySelector(`.edge-label[data-edge-id="${edge.id}"]`);
		ok(label, "应渲染标签元素");
		ok(!label.classList.contains("is-empty"), "有标签时不应是空态");
		eq(label.textContent, "是", "标签文本应更新");
		store.select({ nodes: [a.id, b.id] }, "replace");
		commands.deleteSelection();
	});

	await runner.test("端口增删：同侧多端口沿边均匀排开且不重叠", async () => {
		commands.newDocument("端口");
		await settle(win);
		const node = commands.addNode("process", { x: 200, y: 160 });
		await settle(win);
		actions.addPort(node.id, "out", "right");
		await settle(win, 2);
		const el = doc.querySelector(`.node[data-node-id="${node.id}"]`);
		const ports = [...el.querySelectorAll('.port[data-side="right"]')];
		eq(ports.length, 3, "右侧应有 1 入 2 出共 3 个端口");
		const ys = ports.map((p) => {
			const r = p.getBoundingClientRect();
			return r.top + r.height / 2;
		});
		eq(new Set(ys.map((v) => Math.round(v))).size, 3, "同侧端口不应重叠");
		ok(Math.min(...ys) > el.getBoundingClientRect().top, "端口应落在边的范围内");
		ok(Math.max(...ys) < el.getBoundingClientRect().bottom, "端口应落在边的范围内");
	});

	// ------------------------------------------------------------ 选择 / 平移 / 缩放 / 快捷键

	await runner.test("框选：拖出选择框后选中被覆盖的节点", async () => {
		commands.newDocument("框选");
		await settle(win);
		const a = commands.addNode("process", { x: 120, y: 120 });
		const b = commands.addNode("process", { x: 400, y: 120 });
		store.clearSelection();
		await settle(win);
		const canvas = $("#canvas");
		const startPoint = { clientX: 4, clientY: 4 };
		firePointer(win, canvas, "pointerdown", startPoint);
		firePointer(win, canvas, "pointermove", { clientX: 700, clientY: 400 });
		await settle(win, 1);
		ok($("#marquee").classList.contains("is-on"), "框选期间应显示选择框");
		firePointer(win, canvas, "pointerup", { clientX: 700, clientY: 400 });
		await settle(win, 1);
		ok(store.selection.nodes.size >= 1, `框选应选中节点（实际 ${store.selection.nodes.size}）`);
		ok(!$("#marquee").classList.contains("is-on"), "松开后选择框应隐藏");
		ok(store.graph.getNode(a.id) && store.graph.getNode(b.id), "节点应仍在");
	});

	await runner.test("滚轮缩放与快捷缩放", async () => {
		const before = store.viewport.zoom;
		$("#canvas").dispatchEvent(new win.WheelEvent("wheel", { deltaY: -240, clientX: 400, clientY: 300, bubbles: true, cancelable: true }));
		await settle(win, 1);
		ok(store.viewport.zoom > before, "向上滚应放大");
		actions.zoomReset();
		await settle(win, 1);
		near(store.viewport.zoom, 1, 1e-6, "重置后应回到 100%");
		actions.fit();
		await settle(win, 1);
		ok(store.viewport.zoom > 0, "适应视图后缩放应为正");
	});

	await runner.test("空格拖动平移画布", async () => {
		const beforeX = store.viewport.x;
		fireKey(win, doc.body, "keydown", " ", { code: "Space" });
		const canvas = $("#canvas");
		firePointer(win, canvas, "pointerdown", { clientX: 500, clientY: 400 });
		firePointer(win, canvas, "pointermove", { clientX: 620, clientY: 460 });
		firePointer(win, canvas, "pointerup", { clientX: 620, clientY: 460 });
		fireKey(win, doc.body, "keyup", " ", { code: "Space" });
		await settle(win, 1);
		ok(store.viewport.x !== beforeX, "空格拖动应平移视图");
	});

	await runner.test("Delete 删除选中节点，Ctrl+Z 可恢复", async () => {
		commands.newDocument("删除");
		await settle(win);
		const node = commands.addNode("process", { x: 200, y: 160 });
		store.select({ nodes: [node.id] }, "replace");
		await settle(win, 1);
		fireKey(win, doc.body, "keydown", "Delete");
		await settle(win, 2);
		eq(store.graph.nodes.length, 0, "节点应被删除");
		fireKey(win, doc.body, "keydown", "z", { ctrlKey: true });
		await settle(win, 2);
		eq(store.graph.nodes.length, 1, "撤销应恢复节点");
	});

	// ------------------------------------------------------------ 菜单 / 检查器 / 状态栏

	await runner.test("右键节点弹出上下文菜单，Esc 关闭", async () => {
		commands.newDocument("右键菜单");
		await settle(win);
		const node = commands.addNode("process", { x: 0, y: 0 });
		store.setViewport({ x: 300, y: 200, zoom: 1 });
		await settle(win, 2);
		const el = doc.querySelector(`.node[data-node-id="${node.id}"]`);
		ok(el, "应有节点元素");
		const rect = el.getBoundingClientRect();
		const point = { clientX: rect.left + rect.width / 2, clientY: rect.top + rect.height / 2 };
		el.dispatchEvent(new win.MouseEvent("contextmenu", { bubbles: true, cancelable: true, ...point }));
		await settle(win, 1);
		const menu = doc.querySelector(".context-menu");
		ok(menu, "应弹出右键菜单");
		const labels = [...menu.querySelectorAll(".menu-label")].map((n) => n.textContent);
		ok(labels.some((t) => t.includes("编辑节点文字")), `节点菜单应含节点专属操作，实际 ${labels.join("/")}`);
		ok(menu.querySelectorAll(".menu-item").length >= 8, `节点菜单条目应足够多，实际 ${menu.querySelectorAll(".menu-item").length}`);
		fireKey(win, doc, "keydown", "Escape");
		await settle(win, 1);
		eq(doc.querySelector(".context-menu"), null, "Esc 应关闭菜单");
	});

	await runner.test("右键空白画布弹出菜单（含新建节点入口）", async () => {
		const canvas = $("#canvas");
		canvas.dispatchEvent(new win.MouseEvent("contextmenu", { bubbles: true, cancelable: true, clientX: 900, clientY: 600 }));
		await settle(win, 1);
		const menu = doc.querySelector(".context-menu");
		ok(menu, "应弹出空白处菜单");
		const labels = [...menu.querySelectorAll(".menu-label")].map((n) => n.textContent);
		ok(labels.some((t) => t.includes("新建节点")), `应包含新建节点入口，实际 ${labels.join("/")}`);
		fireMouse(win, doc.body, "pointerdown", { clientX: 5, clientY: 5 });
		await settle(win, 1);
	});

	await runner.test("顶栏导出菜单可展开", async () => {
		const trigger = doc.querySelector('[data-menu="exportMenu"]');
		ok(trigger, "应存在导出按钮");
		trigger.dispatchEvent(new win.MouseEvent("click", { bubbles: true, cancelable: true }));
		await settle(win, 1);
		const menu = doc.querySelector(".menu");
		ok(menu, "应弹出下拉菜单");
		const labels = [...menu.querySelectorAll(".menu-item span")].map((n) => n.textContent);
		ok(labels.length === 3, `导出菜单应有 3 项，实际 ${labels.length}`);
		fireMouse(win, doc.body, "pointerdown", { clientX: 5, clientY: 5 });
		await settle(win, 1);
	});

	await runner.test("检查器：单选节点显示属性，改标题即时生效", async () => {
		commands.newDocument("检查器");
		await settle(win);
		const node = commands.addNode("process", { x: 200, y: 160 });
		store.select({ nodes: [node.id] }, "replace");
		await settle(win, 2);
		const host = $("#inspectorHost");
		const titleInput = host.querySelector(".field-input");
		ok(titleInput, "检查器应出现输入控件");
		eq(host.querySelectorAll(".port-cell").length, 8, "端口矩阵应有 8 个格子（四侧 × 输入输出）");
		// 通过检查器改标题
		const titleField = [...host.querySelectorAll(".field")].find((f) => f.querySelector(".field-label")?.textContent === "标题");
		ok(titleField, "应有「标题」字段");
		const input = titleField.querySelector("input");
		input.value = "检查器改的标题";
		input.dispatchEvent(new win.Event("change", { bubbles: true }));
		await settle(win, 2);
		eq(store.graph.getNode(node.id).title, "检查器改的标题", "检查器修改应写入模型");
		// 端口 + 按钮
		const plus = host.querySelectorAll(".port-cell .stepper")[1];
		plus.dispatchEvent(new win.MouseEvent("click", { bubbles: true }));
		await settle(win, 2);
		eq(store.graph.getNode(node.id).ports.in.top, 2, "点击 + 应增加端口");
	});

	await runner.test("状态栏反映统计与选中数量", async () => {
		commands.newDocument("状态栏");
		await settle(win);
		const node = commands.addNode("process", { x: 200, y: 160 });
		store.select({ nodes: [node.id] }, "replace");
		await settle(win, 2);
		const text = $("#statusBar .status-stats").textContent;
		ok(text.includes("节点 1"), `状态栏应显示节点数，实际「${text}」`);
		ok(text.includes("已选"), `状态栏应显示选中数，实际「${text}」`);
		ok($("#statusBar .status-zoom").textContent.includes("缩放"), "状态栏应显示缩放");
	});

	await runner.test("最小缩略图能画出内容", async () => {
		commands.newDocument("缩略图");
		await settle(win);
		commands.addNode("process", { x: 200, y: 160 });
		await settle(win, 2);
		app.minimap.draw();
		const canvas = $("#minimapCanvas");
		const ctx = canvas.getContext("2d");
		const data = ctx.getImageData(0, 0, canvas.width, canvas.height).data;
		let painted = 0;
		for (let i = 3; i < data.length; i += 4) if (data[i] > 0) painted += 1;
		ok(painted > 100, `缩略图应绘制出节点轮廓（非透明像素 ${painted}）`);
	});

	// ------------------------------------------------------------ 示例 / 导出 / 持久化

	await runner.test("示例文档：覆盖四向连线与三种线型", async () => {
		actions.loadSample();
		await settle(win, 3);
		ok(store.graph.nodes.length >= 5, "示例应有多个节点");
		ok(store.graph.edges.length >= 4, "示例应有多条连线");
		const styles = new Set(store.graph.edges.map((e) => e.style));
		eq(styles.size, 3, "示例应包含三种线型");
		const sides = new Set();
		for (const edge of store.graph.edges) sides.add(edge.from.side), sides.add(edge.to.side);
		eq(sides.size, 4, "示例应覆盖上下左右四个方向");
		eq($$(".edge").length, store.graph.edges.length, "DOM 连线数应与模型一致");
		eq($$(".node").length, store.graph.nodes.length, "DOM 节点数应与模型一致");
		for (const arrow of $$(".edge-arrow-to")) {
			ok(/rotate\([-\d.]+/.test(arrow.getAttribute("transform") || ""), "每条连线都应有箭头旋转角");
		}
	});

	await runner.test("导出 SVG：包含节点、连线与箭头", async () => {
		const svg = buildSVG(store);
		ok(svg.startsWith("<?xml"), "SVG 应有 XML 头");
		ok(svg.includes("<svg"), "应包含 svg 根元素");
		ok(svg.includes("</svg>"), "SVG 应闭合");
		for (const node of store.graph.nodes) {
			ok(svg.includes(escapeForCheck(node.title)), `SVG 应包含节点标题「${node.title}」`);
		}
		ok((svg.match(/<path d="M 0 0 L -11/g) || []).length >= store.graph.edges.length, "每条连线应带箭头多边形");
		ok(svg.includes("<rect"), "应绘制节点矩形");
	});

	await runner.test("导出 PNG：离屏画布尺寸正确且有内容", async () => {
		const canvas = renderPNGCanvas(store, 1);
		ok(canvas, "应生成离屏画布");
		ok(canvas.width > 100 && canvas.height > 100, `画布尺寸应合理（${canvas.width}×${canvas.height}）`);
		const ctx = canvas.getContext("2d");
		const data = ctx.getImageData(0, 0, canvas.width, canvas.height).data;
		const first = [data[0], data[1], data[2]];
		let different = 0;
		for (let i = 0; i < data.length; i += 4 * 97) {
			if (Math.abs(data[i] - first[0]) > 12 || Math.abs(data[i + 1] - first[1]) > 12) different += 1;
		}
		ok(different > 20, `PNG 应画出节点与连线（异色采样 ${different}）`);
	});

	await runner.test("持久化：保存到本地后能在列表中读到，并写进浏览器草稿", async () => {
		const local = persistence.saveLocal();
		ok(local, "本地草稿应写入成功");
		ok(win.localStorage.getItem("node-graph-studio:autosave"), "localStorage 应有草稿键");
		const restored = persistence.loadLocal();
		ok(restored && restored.nodes.length === store.graph.nodes.length, "草稿应能读回同样的节点数");
		const path = await persistence.saveToServer("DOM 自检图");
		eq(path, "database/node_graph_studio/DOM 自检图.json", "应写到约定的本地目录");
		const names = await persistence.listServer();
		ok(names && names.includes("DOM 自检图.json"), `列表应包含刚保存的文件，实际 ${JSON.stringify(names)}`);
		const doc = await persistence.loadFromServer("DOM 自检图.json");
		ok(doc && doc.nodes.length === store.graph.nodes.length, "应能从本地读回");
		await persistence.deleteFromServer("DOM 自检图.json");
		const after = await persistence.listServer();
		ok(after && !after.includes("DOM 自检图.json"), "删除后列表不应再包含它");
	});

	await runner.test("文档名修改同步到顶栏与状态栏", async () => {
		actions.rename("重命名验证");
		await settle(win, 2);
		eq(store.graph.name, "重命名验证", "模型名称应更新");
		eq(doc.querySelector(".doc-name").value, "重命名验证", "顶栏输入框应同步");
	});

	// 收尾：把示例载入，便于截图查看真实观感
	actions.loadSample();
	await settle(win, 4);

	return runner.results;
}

function escapeForCheck(text) {
	return String(text).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

/** 断言失败时用的元素描述（定位「落点命中了什么」） */
function describeElement(element) {
	if (!element) return "null";
	const cls = element.className && element.className.baseVal !== undefined ? element.className.baseVal : element.className;
	return `${element.tagName}.${String(cls || "").split(" ")[0]}`;
}

/** 两个角度之间的最小差值（度） */
function angleDelta(a, b) {
	return Math.abs(((a - b + 540) % 360) - 180);
}

/** 端口锚点（世界坐标） */
function portAnchorOf(store, ref) {
	const node = store.graph.getNode(ref.node);
	const slots = [];
	for (const dir of ["in", "out"]) {
		const count = node.ports[dir][ref.side] || 0;
		for (let i = 0; i < count; i++) slots.push({ dir, index: i });
	}
	const slot = slots.findIndex((s) => s.dir === ref.dir && s.index === ref.index);
	const ratio = slot >= 0 ? (slot + 1) / (slots.length + 1) : 0.5;
	if (ref.side === "top") return { x: node.x + node.w * ratio, y: node.y };
	if (ref.side === "bottom") return { x: node.x + node.w * ratio, y: node.y + node.h };
	if (ref.side === "left") return { x: node.x, y: node.y + node.h * ratio };
	return { x: node.x + node.w, y: node.y + node.h * ratio };
}

export { REPORT };
