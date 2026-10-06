/**
 * 图模型、文档清洗、历史、吸附、布局、文字度量、命令层的测试。
 * 全部为纯逻辑（无 DOM），可在 Node 与浏览器中运行。
 */

import { createSuite, assert, assertEqual, assertClose, assertDeepEqual, makeNode } from "../harness.js";
import { Graph } from "../../js/model/graph.js";
import { sanitizeDocument, emptyDocument, createDocument, documentStats, DEFAULT_SETTINGS } from "../../js/model/document.js";
import { History } from "../../js/model/history.js";
import { snapToGrid, computeAlignSnap, rangesNear } from "../../js/core/snap.js";
import { layeredLayout } from "../../js/model/layout.js";
import { wrapText, textUnits, textBlockHeight, charUnits } from "../../js/core/text.js";
import { MIN_NODE_W, MIN_NODE_H } from "../../js/model/palette.js";
import { Store } from "../../js/app/store.js";
import { Commands, buildSampleDocument } from "../../js/app/commands.js";
import { portKey } from "../../js/core/ports.js";
import { rectOf } from "../../js/core/geometry.js";

function newGraph() {
	return new Graph(emptyDocument("测试"));
}

// ---------------------------------------------------------------- 图模型

export const graphSuite = createSuite("图模型（model/graph）")
	.test("新建节点：套用预设，端口默认四向齐全", () => {
		const graph = newGraph();
		const node = graph.addNode({ type: "decision", x: 40, y: 60 });
		assertEqual(node.type, "decision");
		assertEqual(node.x, 40);
		assertEqual(node.ports.in.top, 1);
		assertEqual(node.ports.out.left, 1);
		assert(node.w > 0 && node.h > 0, "应有默认尺寸");
	})
	.test("删除节点会连带删除相关连线", () => {
		const graph = newGraph();
		const a = graph.addNode({ id: "A" });
		const b = graph.addNode({ id: "B", x: 400 });
		graph.addEdge({ from: { node: "A", dir: "out", side: "right", index: 0 }, to: { node: "B", dir: "in", side: "left", index: 0 } });
		assertEqual(graph.edges.length, 1);
		const removed = graph.removeNodes(["A"]);
		assertEqual(removed.nodes, 1);
		assertEqual(removed.edges, 1);
		assertEqual(graph.edges.length, 0);
	})
	.test("端口数量清零会删除使用该端口的连线", () => {
		const graph = newGraph();
		graph.addNode({ id: "A" });
		graph.addNode({ id: "B", x: 400 });
		graph.addEdge({ from: { node: "A", dir: "out", side: "top", index: 0 }, to: { node: "B", dir: "in", side: "left", index: 0 } });
		graph.setPortCount("A", "out", "top", 0);
		assertEqual(graph.edges.length, 0, "上侧无输出口后，连线应被清理");
	})
	.test("反转连线：流向对调且恒保持 out → in（可在保存/撤销后存活）", () => {
		const graph = newGraph();
		graph.addNode({ id: "A" });
		graph.addNode({ id: "B", x: 400 });
		const edge = graph.addEdge({ from: { node: "A", dir: "out", side: "right", index: 0 }, to: { node: "B", dir: "in", side: "left", index: 0 } });
		const reversed = graph.reverseEdge(edge.id);
		assert(reversed, "应能翻转");
		assertEqual(reversed.from.node, "B", "新尾部应是原头部所在节点");
		assertEqual(reversed.to.node, "A", "新头部应是原尾部所在节点");
		assertEqual(reversed.from.dir, "out", "新尾部必须是输出口");
		assertEqual(reversed.to.dir, "in", "新头部必须是输入口");
		assertEqual(reversed.from.side, "left", "应取原头部所在节点的同侧输出口（走廊不变）");
		assertEqual(reversed.to.side, "right", "应取原尾部所在节点的同侧输入口（走廊不变）");
		// 关键回归：不能产生 in → out 的非法连线，否则保存/撤销时会被清洗掉
		const sanitized = sanitizeDocument(graph.toJSON());
		assertEqual(sanitized.edges.length, 1, "翻转后的连线必须能通过文档清洗");
		assertEqual(sanitized.edges[0].from.dir, "out");
		assertEqual(sanitized.edges[0].to.dir, "in");
	})
	.test("反转连线：对侧没有端口时不改动原连线", () => {
		const graph = newGraph();
		graph.addNode({ id: "A", ports: { in: { left: 1 }, out: { right: 1 } } });
		graph.addNode({ id: "B", x: 400, ports: { in: { left: 1 }, out: { right: 0 } } });
		const edge = graph.addEdge({ from: { node: "A", dir: "out", side: "right", index: 0 }, to: { node: "B", dir: "in", side: "left", index: 0 } });
		const before = JSON.stringify(edge);
		const result = graph.reverseEdge(edge.id);
		assertEqual(result, null, "B 没有可用的输出口，应放弃翻转");
		assertEqual(JSON.stringify(graph.edges[0]), before, "放弃时不得改动原连线");
	})
	.test("包围盒与区域查询", () => {
		const graph = newGraph();
		graph.addNode({ id: "A", x: 0, y: 0, w: 160, h: 120 });
		graph.addNode({ id: "B", x: 300, y: 200, w: 160, h: 120 });
		const bounds = graph.bounds();
		assertEqual(bounds.w, 460);
		assertEqual(bounds.h, 320);
		assertDeepEqual(graph.nodesInRect({ x: -10, y: -10, w: 200, h: 160 }).map((n) => n.id), ["A"]);
		assertEqual(graph.nodesInRect({ x: 1000, y: 1000, w: 10, h: 10 }).length, 0);
	})
	.test("度数统计：某端口上的连线数", () => {
		const graph = newGraph();
		graph.addNode({ id: "A" });
		graph.addNode({ id: "B", x: 300 });
		graph.addNode({ id: "C", x: 600 });
		const ref = { node: "A", dir: "out", side: "right", index: 0 };
		graph.addEdge({ from: { ...ref }, to: { node: "B", dir: "in", side: "left", index: 0 } });
		graph.addEdge({ from: { ...ref }, to: { node: "C", dir: "in", side: "left", index: 0 } });
		assertEqual(graph.degree(ref), 2);
	})
	.test("序列化往返：端口、连线样式、视口都保留", () => {
		const graph = newGraph();
		graph.addNode({ id: "A", ports: { in: { top: 2 }, out: { bottom: 3 } } });
		graph.addNode({ id: "B", x: 400 });
		graph.addEdge({
			from: { node: "A", dir: "out", side: "bottom", index: 2 },
			to: { node: "B", dir: "in", side: "left", index: 0 },
			style: "ortho",
			arrow: "both",
			label: "标签",
			dashed: true,
		});
		graph.viewport = { x: 12, y: 34, zoom: 1.5 };
		const clone = new Graph(graph.toJSON());
		assertEqual(clone.nodes.length, 2);
		assertEqual(clone.edges.length, 1);
		assertEqual(clone.getNode("A").ports.out.bottom, 3);
		assertEqual(clone.edges[0].style, "ortho");
		assertEqual(clone.edges[0].arrow, "both");
		assertEqual(clone.edges[0].label, "标签");
		assertEqual(clone.edges[0].dashed, true);
		assertEqual(clone.viewport.zoom, 1.5);
	})
	.test("删除连线 / 查询某节点的连线", () => {
		const graph = newGraph();
		graph.addNode({ id: "A" });
		graph.addNode({ id: "B", x: 300 });
		const edge = graph.addEdge({ from: { node: "A", dir: "out", side: "right", index: 0 }, to: { node: "B", dir: "in", side: "left", index: 0 } });
		assertEqual(graph.edgesOf("A").length, 1);
		assertEqual(graph.removeEdges([edge.id]), 1);
		assertEqual(graph.edgesOf("A").length, 0);
	});

// ---------------------------------------------------------------- 文档清洗

export const documentSuite = createSuite("文档清洗（model/document）")
	.test("空文档：默认设置与视口齐备", () => {
		const doc = emptyDocument();
		assertEqual(doc.nodes.length, 0);
		assertEqual(doc.edges.length, 0);
		assertEqual(doc.settings.grid, DEFAULT_SETTINGS.grid);
		assertEqual(doc.viewport.zoom, 1);
		assertEqual(documentStats(doc).nodes, 0);
	})
	.test("垃圾输入不抛异常，输出始终结构化", () => {
		for (const bad of [null, undefined, 42, "x", [], { nodes: "nope", edges: 7 }]) {
			const doc = sanitizeDocument(bad);
			assert(Array.isArray(doc.nodes), "nodes 应为数组");
			assert(Array.isArray(doc.edges), "edges 应为数组");
			assert(typeof doc.name === "string" && doc.name.length > 0, "应有文档名");
		}
	})
	.test("数值越界被夹取、宽高有下限、非法枚举回落到默认", () => {
		const doc = sanitizeDocument({
			nodes: [{ id: "A", x: 1e12, y: -1e12, w: 1, h: 1, shape: "triangle", color: "javascript:alert(1)", fontSize: 999 }],
			edges: [],
		});
		const node = doc.nodes[0];
		assert(Math.abs(node.x) <= 1e6, "x 应被夹取");
		assert(node.w >= MIN_NODE_W, "宽度不应小于下限");
		assert(node.h >= MIN_NODE_H, "高度不应小于下限");
		assertEqual(node.shape, "round", "非法形状应回落");
		assert(node.color.startsWith("#"), "非法颜色应回落为预设色值");
		assert(!String(node.color).includes("javascript"), "不应保留危险字符串");
	})
	.test("重复 id 会被去重，悬空连线会被丢弃", () => {
		const doc = sanitizeDocument({
			nodes: [{ id: "A" }, { id: "A" }],
			edges: [{ from: { node: "A", dir: "out", side: "right", index: 0 }, to: { node: "ZZZ", dir: "in", side: "left", index: 0 } }],
		});
		assertEqual(doc.nodes.length, 2);
		assert(doc.nodes[0].id !== doc.nodes[1].id, "重复 id 应被改写");
		assertEqual(doc.edges.length, 0, "指向不存在节点的连线应被丢弃");
	})
	.test("端口不存在的连线会被丢弃、越界下标被收敛", () => {
		const doc = sanitizeDocument({
			nodes: [
				{ id: "A", ports: { in: { left: 0 }, out: { right: 1 } } },
				{ id: "B", ports: { in: { left: 1 }, out: { right: 0 } } },
			],
			edges: [
				{ id: "bad", from: { node: "A", dir: "out", side: "top", index: 0 }, to: { node: "B", dir: "in", side: "left", index: 0 } },
				{ id: "clamp", from: { node: "A", dir: "out", side: "right", index: 9 }, to: { node: "B", dir: "in", side: "left", index: 0 } },
				{ id: "wrongdir", from: { node: "A", dir: "in", side: "left", index: 0 }, to: { node: "B", dir: "in", side: "left", index: 0 } },
			],
		});
		assertEqual(doc.edges.length, 1, "只应留下可用的那条");
		assertEqual(doc.edges[0].id, "clamp");
		assertEqual(doc.edges[0].from.index, 0, "越界下标应被收敛");
	})
	.test("重复连线被去重；尺寸与缩放被夹取", () => {
		const doc = sanitizeDocument({
			nodes: [{ id: "A" }, { id: "B" }],
			edges: [
				{ from: { node: "A", dir: "out", side: "right", index: 0 }, to: { node: "B", dir: "in", side: "left", index: 0 } },
				{ from: { node: "A", dir: "out", side: "right", index: 0 }, to: { node: "B", dir: "in", side: "left", index: 0 } },
			],
			viewport: { x: 0, y: 0, zoom: 99 },
		});
		assertEqual(doc.edges.length, 1);
		assert(doc.viewport.zoom <= 4, "缩放应被夹取到上限");
	})
	.test("createDocument 与 emptyDocument 等价可用", () => {
		const doc = createDocument({ name: "我的图" });
		assertEqual(doc.name, "我的图");
		assertEqual(documentStats(doc).chars, 0);
	});

// ---------------------------------------------------------------- 历史

export const historySuite = createSuite("撤销 / 重做（model/history）")
	.test("提交后可撤销、可重做，并回到正确状态", () => {
		let state = { value: 0 };
		const history = new History({ snapshot: () => JSON.parse(JSON.stringify(state)), apply: (s) => { state = JSON.parse(JSON.stringify(s)); } });
		history.reset();
		state.value = 1;
		history.commit("加一");
		state.value = 2;
		history.commit("再加一");
		assertEqual(history.undoLabel(), "再加一");
		assert(history.canUndo(), "应可撤销");
		history.undo();
		assertEqual(state.value, 1);
		assert(history.canRedo(), "应可重做");
		history.redo();
		assertEqual(state.value, 2);
	})
	.test("撤销后再提交会丢弃「未来」分支", () => {
		let state = { value: 0 };
		const history = new History({ snapshot: () => ({ ...state }), apply: (s) => { state = { ...s }; } });
		history.reset();
		state.value = 1;
		history.commit("一");
		state.value = 2;
		history.commit("二");
		history.undo();
		assertEqual(state.value, 1);
		state.value = 99;
		history.commit("改道");
		assert(!history.canRedo(), "重做分支应被丢弃");
		assertEqual(history.redoLabel(), "");
	})
	.test("无变化的提交被忽略（不污染历史）", () => {
		let state = { value: 0 };
		const history = new History({ snapshot: () => ({ ...state }), apply: (s) => { state = { ...s }; } });
		history.reset();
		history.commit("没变");
		assertEqual(history.length, 1);
		state.value = 5;
		history.commit("变了");
		assertEqual(history.length, 2);
	})
	.test("超出上限时丢弃最早的记录，且仍可撤销", () => {
		let state = { value: 0 };
		const history = new History({ snapshot: () => ({ ...state }), apply: (s) => { state = { ...s }; }, limit: 3 });
		history.reset();
		for (let i = 1; i <= 6; i++) {
			state.value = i;
			history.commit(`第 ${i} 步`);
		}
		assertEqual(history.length, 3);
		history.undo();
		history.undo();
		assertEqual(state.value, 4);
		assert(!history.canUndo(), "已到栈底");
	})
	.test("初始状态不可撤销", () => {
		const history = new History({ snapshot: () => ({}), apply: () => { } });
		history.reset();
		assert(!history.canUndo());
		assert(!history.undo());
	});

// ---------------------------------------------------------------- 吸附

export const snapSuite = createSuite("吸附（core/snap）")
	.test("栅格吸附：按倍数取整，关闭时原样返回", () => {
		assertEqual(snapToGrid(23, 20, true), 20);
		assertEqual(snapToGrid(31, 20, true), 40);
		assertEqual(snapToGrid(-9, 20, true), -0);
		assertEqual(snapToGrid(23, 20, false), 23);
		assertEqual(snapToGrid(23, 0, true), 23);
	})
	.test("对齐吸附：命中中线时给出位移与参考线", () => {
		const dragged = { x: 103, y: 50, w: 100, h: 40 };
		// 纵向范围重叠（50..90 与 40..80）→ 算邻近，纵向候选线互不命中，只产生一条纵向参考线
		const others = [{ x: 100, y: 40, w: 100, h: 40 }];
		const result = computeAlignSnap(dragged, others, 6);
		assertEqual(result.dx, -3, "应把左边缘吸到 100");
		assertEqual(result.dy, 0, "纵向无对齐");
		assertEqual(result.guides.length, 1);
		assertEqual(result.guides[0].axis, "x");
		assertEqual(result.guides[0].pos, 100);
	})
	.test("距离超过阈值时不吸附（纵向也错开时不产生任何参考线）", () => {
		const result = computeAlignSnap({ x: 140, y: 300, w: 100, h: 40 }, [{ x: 100, y: 0, w: 100, h: 40 }], 6);
		assertEqual(result.dx, 0);
		assertEqual(result.dy, 0);
		assertEqual(result.guides.length, 0);
	})
	.test("只与「邻近」节点吸附：画布另一头的节点不会把拖动中的节点猛拽一下", () => {
		const dragged = { x: 103, y: 50, w: 100, h: 40 };
		// 左边缘恰好落在 6px 阈值内，但纵向相隔 2000px —— 不该吸附
		const farAway = { x: 100, y: 2050, w: 100, h: 40 };
		const far = computeAlignSnap(dragged, [farAway], 6);
		assertEqual(far.dx, 0, "纵向相隔极远时不应发生纵向对齐吸附");
		assertEqual(far.guides.length, 0, "不应画出跨半张图的参考线");
		// 纵向相邻（重叠）时照常吸附
		const nearby = { x: 100, y: 40, w: 100, h: 40 };
		const near = computeAlignSnap(dragged, [nearby], 6);
		assertEqual(near.dx, -3, "邻近节点仍应吸附到左边缘对齐");
		assert(near.guides.some((g) => g.axis === "x"), "应画出纵向参考线");
		assert(rangesNear(0, 40, 2000, 2040, 90) === false, "区间距离远超 slack 时应判为不相邻");
		assert(rangesNear(0, 40, 130, 170, 90) === true, "区间间距在 slack 内应判为相邻");
		assert(rangesNear(0, 40, 20, 60, 90) === true, "区间重叠应判为相邻");
	});

// ---------------------------------------------------------------- 布局

export const layoutSuite = createSuite("自动布局（model/layout）")
	.test("链式结构按方向分层：后一个节点的 y 严格递增", () => {
		const nodes = [
			makeNode("A", { x: 0, y: 0, w: 200, h: 80 }),
			makeNode("B", { x: 0, y: 0, w: 200, h: 80 }),
			makeNode("C", { x: 0, y: 0, w: 200, h: 80 }),
		];
		const edges = [
			{ from: { node: "A", dir: "out", side: "bottom", index: 0 }, to: { node: "B", dir: "in", side: "top", index: 0 } },
			{ from: { node: "B", dir: "out", side: "bottom", index: 0 }, to: { node: "C", dir: "in", side: "top", index: 0 } },
		];
		const layout = layeredLayout(nodes, edges, { direction: "TB", gapMain: 90 });
		assert(layout.get("A").y < layout.get("B").y, "A 应在 B 上方");
		assert(layout.get("B").y < layout.get("C").y, "B 应在 C 上方");
	})
	.test("横向分层：x 递增且同层不重叠", () => {
		const nodes = [
			makeNode("A", { x: 0, y: 0, w: 200, h: 80 }),
			makeNode("B", { x: 0, y: 0, w: 200, h: 80 }),
			makeNode("C", { x: 0, y: 0, w: 200, h: 80 }),
		];
		const edges = [
			{ from: { node: "A", dir: "out", side: "right", index: 0 }, to: { node: "B", dir: "in", side: "left", index: 0 } },
			{ from: { node: "A", dir: "out", side: "right", index: 0 }, to: { node: "C", dir: "in", side: "left", index: 0 } },
		];
		const layout = layeredLayout(nodes, edges, { direction: "LR", gapCross: 40 });
		assert(layout.get("A").x < layout.get("B").x, "A 应在 B 左侧");
		assert(layout.get("A").x < layout.get("C").x, "A 应在 C 左侧");
		assertEqual(layout.get("B").x, layout.get("C").x, "同层应对齐");
		assert(Math.abs(layout.get("B").y - layout.get("C").y) >= 80, "同层节点不应重叠");
	})
	.test("全部节点都被放置；环状结构不会死循环", () => {
		const nodes = [makeNode("A"), makeNode("B"), makeNode("C")];
		const edges = [
			{ from: { node: "A", dir: "out", side: "right", index: 0 }, to: { node: "B", dir: "in", side: "left", index: 0 } },
			{ from: { node: "B", dir: "out", side: "right", index: 0 }, to: { node: "C", dir: "in", side: "left", index: 0 } },
			{ from: { node: "C", dir: "out", side: "right", index: 0 }, to: { node: "A", dir: "in", side: "left", index: 0 } },
		];
		const layout = layeredLayout(nodes, edges);
		assertEqual(layout.size, 3);
		for (const node of nodes) {
			const pos = layout.get(node.id);
			assert(Number.isFinite(pos.x) && Number.isFinite(pos.y), `${node.id} 坐标应为有限数`);
		}
	})
	.test("结果整体最小顶点落在 origin 上", () => {
		const nodes = [makeNode("A", { x: -500, y: -400 }), makeNode("B", { x: -500, y: -400 })];
		const layout = layeredLayout(nodes, [], { originX: 40, originY: 60 });
		const xs = [...layout.values()].map((p) => p.x);
		const ys = [...layout.values()].map((p) => p.y);
		assertEqual(Math.min(...xs), 40);
		assertEqual(Math.min(...ys), 60);
	});

// ---------------------------------------------------------------- 文字

export const textSuite = createSuite("文字度量与换行（core/text）")
	.test("中文按整字、西文按半字估算", () => {
		assertEqual(charUnits("中"), 1);
		assert(charUnits("a") < 0.7, "西文字符更窄");
		assertEqual(charUnits("\t"), 2);
		assertClose(textUnits("中文"), 2, 1e-6);
	})
	.test("按宽度换行，显式换行优先", () => {
		// 中文每字 1 个字宽：宽 30px / 字号 10px ⇒ 每行 3 个字
		const lines = wrapText("一二三四五六\n第二行", 30, 10);
		assertEqual(lines[0], "一二三");
		assertEqual(lines[1], "四五六");
		assertEqual(lines[lines.length - 1], "第二行");
		assert(wrapText("abcdefghij", 50, 10).length > 1, "西文也该在宽度用尽时折行");
	})
	.test("空文本也返回一行", () => {
		assertEqual(wrapText("", 100, 13).length, 1);
		assert(textBlockHeight("中文\n中文", 100, 13) > 0, "文字块高度应为正");
	});

// ---------------------------------------------------------------- 命令层（含 Store 集成）

export const commandsSuite = createSuite("命令层与状态（app/commands + app/store）")
	.test("新建节点自动选中，并进入撤销栈", () => {
		const store = new Store(emptyDocument("T"));
		const history = new History({ snapshot: () => store.snapshot(), apply: (s) => store.restore(s) });
		history.reset();
		const commands = new Commands(store, history, () => { });
		const node = commands.addNode("process", { x: 100, y: 100 });
		assertEqual(store.graph.nodes.length, 1);
		assert(store.isNodeSelected(node.id), "新节点应被选中");
		assert(history.canUndo(), "新建应可撤销");
		history.undo();
		assertEqual(store.graph.nodes.length, 0, "撤销后节点应消失");
		history.redo();
		assertEqual(store.graph.nodes.length, 1, "重做后节点应恢复");
	})
	.test("拖拽位移不逐帧入栈，结束时一次提交", () => {
		const store = new Store(emptyDocument("T"));
		const history = new History({ snapshot: () => store.snapshot(), apply: (s) => store.restore(s) });
		history.reset();
		const commands = new Commands(store, history, () => { });
		const node = commands.addNode("process", { x: 0, y: 0 });
		const originX = store.graph.getNode(node.id).x;
		const originY = store.graph.getNode(node.id).y;
		const before = history.length;
		commands.translateNodes([node.id], 10, 5);
		commands.translateNodes([node.id], 10, 5);
		assertEqual(history.length, before, "位移过程不应写入历史");
		commands.commit("移动节点");
		assertEqual(history.length, before + 1, "结束后只写一条");
		history.undo();
		assertEqual(store.graph.getNode(node.id).x, originX, "撤销应回到拖动前");
		assertEqual(store.graph.getNode(node.id).y, originY, "撤销应回到拖动前");
	})
	.test("connect 会用 out→in 语义建线，并保留四向能力", () => {
		const store = new Store(emptyDocument("T"));
		const history = new History({ snapshot: () => store.snapshot(), apply: (s) => store.restore(s) });
		history.reset();
		const commands = new Commands(store, history, () => { });
		const a = commands.addNode("process", { x: 0, y: 0 });
		const b = commands.addNode("process", { x: 600, y: 400 });
		// 从 A 的「上侧输出」连到 B 的「右侧输入」——两个非默认方向
		const ok = commands.connect(
			{ node: a.id, dir: "out", side: "top", index: 0 },
			{ node: b.id, dir: "in", side: "right", index: 0 },
		);
		assert(ok, "应连接成功");
		assertEqual(store.graph.edges.length, 1);
		assertEqual(store.graph.edges[0].from.side, "top");
		assertEqual(store.graph.edges[0].to.side, "right");
		assertEqual(store.graph.edges[0].from.dir, "out");
		assertEqual(store.graph.edges[0].to.dir, "in");
	})
	.test("同向投放会自动补端口并成线", () => {
		const store = new Store(emptyDocument("T"));
		const history = new History({ snapshot: () => store.snapshot(), apply: (s) => store.restore(s) });
		history.reset();
		const commands = new Commands(store, history, () => { });
		const a = commands.addNode("process", { x: 0, y: 0 });
		const b = commands.addNode("process", { x: 500, y: 0 });
		// 先把 B 左侧输入口清零，制造「同向需要补端口」的场景
		commands.setPortCount(b.id, "in", "left", 0);
		const ok = commands.connect(
			{ node: a.id, dir: "out", side: "right", index: 0 },
			{ node: b.id, dir: "out", side: "left", index: 0 },
		);
		assert(ok, "应通过自动补端口完成连接");
		assertEqual(store.graph.getNode(b.id).ports.in.left, 1, "应为落点节点补上输入口");
		assertEqual(store.graph.edges.length, 1);
	})
	.test("重复连接被拒绝，不产生第二条连线", () => {
		const store = new Store(emptyDocument("T"));
		const history = new History({ snapshot: () => store.snapshot(), apply: (s) => store.restore(s) });
		history.reset();
		const commands = new Commands(store, history, () => { });
		const a = commands.addNode("process", { x: 0, y: 0 });
		const b = commands.addNode("process", { x: 500, y: 0 });
		const from = { node: a.id, dir: "out", side: "right", index: 0 };
		const to = { node: b.id, dir: "in", side: "left", index: 0 };
		assert(commands.connect(from, to), "首次应成功");
		assert(!commands.connect(from, to), "重复应被拒绝");
		assertEqual(store.graph.edges.length, 1);
	})
	.test("删除选中会连带删除连线，并可从历史恢复", () => {
		const store = new Store(emptyDocument("T"));
		const history = new History({ snapshot: () => store.snapshot(), apply: (s) => store.restore(s) });
		history.reset();
		const commands = new Commands(store, history, () => { });
		const a = commands.addNode("process", { x: 0, y: 0 });
		const b = commands.addNode("process", { x: 500, y: 0 });
		commands.connect({ node: a.id, dir: "out", side: "right", index: 0 }, { node: b.id, dir: "in", side: "left", index: 0 });
		store.select({ nodes: [a.id] }, "replace");
		const before = store.graph.edges.length;
		commands.deleteSelection();
		assertEqual(store.graph.edges.length, before - 1, "相关连线应被删除");
		history.undo();
		assertEqual(store.graph.edges.length, before, "撤销后应恢复");
	})
	.test("复制粘贴：节点与内部连线同时复制，且端口保持", () => {
		const store = new Store(emptyDocument("T"));
		const history = new History({ snapshot: () => store.snapshot(), apply: (s) => store.restore(s) });
		history.reset();
		const commands = new Commands(store, history, () => { });
		const a = commands.addNode("process", { x: 0, y: 0 });
		const b = commands.addNode("process", { x: 400, y: 0 });
		commands.connect({ node: a.id, dir: "out", side: "right", index: 0 }, { node: b.id, dir: "in", side: "left", index: 0 });
		store.select({ nodes: [a.id, b.id] }, "replace");
		commands.copySelection();
		const pasted = commands.paste({ x: 1000, y: 1000 });
		assertEqual(pasted.nodes.length, 2);
		assertEqual(pasted.edges.length, 1, "内部连线应一并复制");
		assertEqual(store.graph.nodes.length, 4);
		assertEqual(store.graph.edges.length, 2);
	})
	.test("对齐：左对齐把全部选中节点贴到最左边缘", () => {
		const store = new Store(emptyDocument("T"));
		const history = new History({ snapshot: () => store.snapshot(), apply: (s) => store.restore(s) });
		history.reset();
		const commands = new Commands(store, history, () => { });
		const a = commands.addNode("process", { x: 0, y: 0 });
		const b = commands.addNode("process", { x: 300, y: 200 });
		const c = commands.addNode("process", { x: 700, y: 400 });
		const expected = Math.min(
			store.graph.getNode(a.id).x,
			store.graph.getNode(b.id).x,
			store.graph.getNode(c.id).x,
		);
		store.select({ nodes: [a.id, b.id, c.id] }, "replace");
		commands.align("left");
		assertEqual(store.graph.getNode(a.id).x, expected);
		assertEqual(store.graph.getNode(b.id).x, expected);
		assertEqual(store.graph.getNode(c.id).x, expected);
	})
	.test("等距分布：中间节点被均匀排开", () => {
		const store = new Store(emptyDocument("T"));
		const history = new History({ snapshot: () => store.snapshot(), apply: (s) => store.restore(s) });
		history.reset();
		const commands = new Commands(store, history, () => { });
		const a = commands.addNode("process", { x: 0, y: 0 });
		const b = commands.addNode("process", { x: 100, y: 0 });
		const c = commands.addNode("process", { x: 900, y: 0 });
		store.select({ nodes: [a.id, b.id, c.id] }, "replace");
		commands.align("distributeX");
		const xa = store.graph.getNode(a.id).x;
		const xb = store.graph.getNode(b.id).x;
		const xc = store.graph.getNode(c.id).x;
		assert(xa < xb && xb < xc, "顺序应保持");
		assertClose(xb, (xa + xc) / 2, 1, "中间节点应居中");
	})
	.test("自动布局会重排全部节点并保持连线", () => {
		const store = new Store(emptyDocument("T"));
		const history = new History({ snapshot: () => store.snapshot(), apply: (s) => store.restore(s) });
		history.reset();
		const commands = new Commands(store, history, () => { });
		const a = commands.addNode("process", { x: 500, y: 500 });
		const b = commands.addNode("process", { x: 10, y: 900 });
		commands.connect({ node: a.id, dir: "out", side: "right", index: 0 }, { node: b.id, dir: "in", side: "left", index: 0 });
		commands.autoLayout("TB", false);
		assertEqual(store.graph.edges.length, 1, "连线不应丢失");
		assert(store.graph.getNode(a.id).y < store.graph.getNode(b.id).y, "A 应被排到 B 上方");
	})
	.test("层序调整：置顶后被放到数组末尾（DOM 顺序即层序）", () => {
		const store = new Store(emptyDocument("T"));
		const history = new History({ snapshot: () => store.snapshot(), apply: (s) => store.restore(s) });
		history.reset();
		const commands = new Commands(store, history, () => { });
		const a = commands.addNode("process", { x: 0, y: 0 });
		const b = commands.addNode("process", { x: 10, y: 10 });
		commands.bringToFront([a.id]);
		assertEqual(store.graph.nodes[store.graph.nodes.length - 1].id, a.id);
		commands.sendToBack([a.id]);
		assertEqual(store.graph.nodes[0].id, a.id);
		assertEqual(store.graph.nodes.length, 2);
		if (b) assert(true);
	})
	.test("选择集在撤销后会剔除已不存在的节点", () => {
		const store = new Store(emptyDocument("T"));
		const history = new History({ snapshot: () => store.snapshot(), apply: (s) => store.restore(s) });
		history.reset();
		const commands = new Commands(store, history, () => { });
		const a = commands.addNode("process", { x: 0, y: 0 });
		store.select({ nodes: [a.id] }, "replace");
		commands.deleteSelection();
		history.undo();
		history.redo();
		assertEqual(store.selection.nodes.size, 0, "选择集不应包含已删除节点");
	})
	.test("示例文档自洽：连线方向正确、覆盖四向与三种线型", () => {
		const doc = buildSampleDocument();
		const graph = new Graph(doc);
		assert(graph.nodes.length >= 5, "示例应包含足够节点");
		assert(graph.edges.length >= 4, "示例应包含多条连线");
		const styles = new Set(graph.edges.map((e) => e.style));
		assert(styles.has("bezier") && styles.has("ortho") && styles.has("straight"), "示例应包含三种线型");
		const sides = new Set();
		for (const edge of graph.edges) {
			assertEqual(edge.from.dir, "out", "尾部必须是输出口");
			assertEqual(edge.to.dir, "in", "头部必须是输入口");
			assert(graph.getNode(edge.from.node), "尾部节点应存在");
			assert(graph.getNode(edge.to.node), "头部节点应存在");
			sides.add(edge.from.side);
			sides.add(edge.to.side);
		}
		assertEqual(sides.size, 4, "示例应覆盖上下左右四个方向");
		assert(new Set(graph.edges.map((e) => portKey(e.from) + portKey(e.to))).size === graph.edges.length, "不应有重复连线");
	})
	.test("文档快照含视口；恢复后节点矩形可读", () => {
		const store = new Store(emptyDocument("T"));
		store.setViewport({ x: 40, y: 50, zoom: 1.4 });
		const doc = store.snapshotDocument();
		assertEqual(doc.viewport.zoom, 1.4);
		assert(rectOf(makeNode("X")).w === 200, "矩形读取应正常");
	});
