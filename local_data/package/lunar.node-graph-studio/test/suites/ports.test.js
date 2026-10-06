/**
 * 端口模型测试：四向端口布局 + 连线合法性（「向四面八方连接」的规则层）。
 */

import { createSuite, assert, assertEqual, assertClose, makeNode } from "../harness.js";
import {
	SIDES, SIDE_VECTOR, defaultPorts, normalizePorts, portCount, sideSlots, portOffset, portAnchor,
	portKey, samePort, sanitizePortRef, allPortRefs, resolveConnection, edgeExists,
	reconcileEdgesForPorts, isPortRef,
} from "../../js/core/ports.js";

const getNodeFrom = (nodes) => (id) => nodes.find((n) => n.id === id);

export const portsSuite = createSuite("端口与连线规则（core/ports）")
	.test("默认端口：四侧各有 1 入 1 出（共 8 个，四面皆可连）", () => {
		const node = makeNode("A", { ports: defaultPorts() });
		for (const side of SIDES) {
			assertEqual(portCount(node, "in", side), 1, `${side} 侧输入口`);
			assertEqual(portCount(node, "out", side), 1, `${side} 侧输出口`);
		}
		assertEqual(allPortRefs(node).length, 8, "端口总数");
	})
	.test("端口归一化：非法值归零、四侧齐全", () => {
		const ports = normalizePorts({ in: { top: 3, right: -2, bottom: "x" }, out: { left: 1.9 } });
		assertEqual(ports.in.top, 3);
		assertEqual(ports.in.right, 0);
		assertEqual(ports.in.bottom, 0);
		assertEqual(ports.in.left, 0);
		assertEqual(ports.out.left, 1);
		assertEqual(ports.out.top, 0);
		assertEqual(normalizePorts(null).in.top, 0);
	})
	.test("同侧槽位：先排输入口再排输出口，互不重叠", () => {
		const node = makeNode("A", { ports: normalizePorts({ in: { top: 2 }, out: { top: 1 } }) });
		const slots = sideSlots(node, "top");
		assertEqual(slots.length, 3);
		assertEqual(slots[0].dir, "in");
		assertEqual(slots[0].index, 0);
		assertEqual(slots[1].dir, "in");
		assertEqual(slots[1].index, 1);
		assertEqual(slots[2].dir, "out");
		assertEqual(slots[2].index, 0);
	})
	.test("端口锚点落在节点四边上（上/下取顶底边，左/右取左右边）", () => {
		const node = makeNode("A", { x: 100, y: 200, w: 200, h: 100 });
		for (const side of SIDES) {
			for (const dir of ["in", "out"]) {
				const p = portAnchor(node, dir, side, 0);
				if (side === "top") assertClose(p.y, node.y, 1e-6, "上侧 y");
				if (side === "bottom") assertClose(p.y, node.y + node.h, 1e-6, "下侧 y");
				if (side === "left") assertClose(p.x, node.x, 1e-6, "左侧 x");
				if (side === "right") assertClose(p.x, node.x + node.w, 1e-6, "右侧 x");
				assert(p.x >= node.x - 1e-6 && p.x <= node.x + node.w + 1e-6, "锚点应落在节点横向范围内");
				assert(p.y >= node.y - 1e-6 && p.y <= node.y + node.h + 1e-6, "锚点应落在节点纵向范围内");
			}
		}
	})
	.test("同侧两个端口沿该侧均匀分开", () => {
		const node = makeNode("A", { x: 0, y: 0, w: 300, h: 150, ports: normalizePorts({ in: { top: 1 }, out: { top: 1 } }) });
		const pIn = portAnchor(node, "in", "top", 0);
		const pOut = portAnchor(node, "out", "top", 0);
		assertClose(pIn.x, 100, 1e-6, "第 1 槽位在 1/3 处");
		assertClose(pOut.x, 200, 1e-6, "第 2 槽位在 2/3 处");
		assert(pIn.x !== pOut.x, "同侧双向端口不应重叠");
	})
	.test("每侧的局部偏移方向正确（用于渲染定位）", () => {
		const node = makeNode("A", { w: 200, h: 100 });
		assertClose(portOffset(node, "out", "top", 0).y, 0, 1e-6);
		assertClose(portOffset(node, "out", "bottom", 0).y, 100, 1e-6);
		assertClose(portOffset(node, "out", "left", 0).x, 0, 1e-6);
		assertClose(portOffset(node, "out", "right", 0).x, 200, 1e-6);
		for (const side of SIDES) {
			assertClose(Math.hypot(SIDE_VECTOR[side].x, SIDE_VECTOR[side].y), 1, 1e-9, `${side} 法向应为单位向量`);
		}
	})
	.test("端口身份：键唯一、可判等、可清洗", () => {
		const a = { node: "A", dir: "out", side: "right", index: 0 };
		const b = { node: "A", dir: "out", side: "right", index: 0 };
		const c = { node: "A", dir: "out", side: "right", index: 1 };
		assert(samePort(a, b), "同端口应判等");
		assert(!samePort(a, c), "不同下标不应判等");
		assertEqual(portKey(a), "A|out|right|0");
		assert(isPortRef(a), "合法端口引用");
		assertEqual(sanitizePortRef({ node: "A", dir: "bogus", side: "right", index: 0 }), null);
		assertEqual(sanitizePortRef({ node: "A", dir: "out", side: "up", index: 0 }), null);
		assertEqual(sanitizePortRef(null), null);
		assertEqual(sanitizePortRef({ node: "A", dir: "out", side: "right", index: -5 }).index, 0);
	})
	.test("输出 → 输入：直接成线（尾部=输出，头部=输入）", () => {
		const a = makeNode("A");
		const b = makeNode("B", { x: 400 });
		const decision = resolveConnection(getNodeFrom([a, b]), [], { node: "A", dir: "out", side: "right", index: 0 }, { node: "B", dir: "in", side: "left", index: 0 });
		assert(decision.ok, decision.reason);
		assertEqual(decision.from.node, "A");
		assertEqual(decision.from.dir, "out");
		assertEqual(decision.to.node, "B");
		assertEqual(decision.to.dir, "in");
		assertEqual(decision.swapped, false);
	})
	.test("输入 → 输出：反着拖也能连，方向自动纠正", () => {
		const a = makeNode("A");
		const b = makeNode("B", { x: 400 });
		const decision = resolveConnection(getNodeFrom([a, b]), [], { node: "A", dir: "in", side: "right", index: 0 }, { node: "B", dir: "out", side: "left", index: 0 });
		assert(decision.ok, decision.reason);
		assertEqual(decision.from.node, "B", "尾部应是输出口所在的 B");
		assertEqual(decision.from.dir, "out");
		assertEqual(decision.to.node, "A", "头部应是输入口所在的 A");
		assertEqual(decision.to.dir, "in");
		assertEqual(decision.swapped, true);
	})
	.test("输出 → 输出：为落点节点同侧补一个输入口后成线", () => {
		const a = makeNode("A");
		const b = makeNode("B", { x: 400, ports: normalizePorts({ in: { right: 0 }, out: { right: 1 } }) });
		const decision = resolveConnection(getNodeFrom([a, b]), [], { node: "A", dir: "out", side: "right", index: 0 }, { node: "B", dir: "out", side: "right", index: 0 });
		assert(decision.ok, decision.reason);
		assert(decision.createdPort, "应报告需要新建端口");
		assertEqual(decision.createdPort.node, "B");
		assertEqual(decision.createdPort.dir, "in");
		assertEqual(decision.createdPort.side, "right");
		assertEqual(decision.createdPort.count, 1);
		assertEqual(decision.to.dir, "in");
	})
	.test("输出 → 输出（同侧已有输入口）：复用已有输入口，不新建", () => {
		const a = makeNode("A");
		const b = makeNode("B", { x: 400 });
		const decision = resolveConnection(getNodeFrom([a, b]), [], { node: "A", dir: "out", side: "top", index: 0 }, { node: "B", dir: "out", side: "left", index: 0 });
		assert(decision.ok, decision.reason);
		assertEqual(decision.createdPort, null, "已有输入口可复用");
		assertEqual(decision.to.dir, "in");
		assertEqual(decision.to.side, "left");
	})
	.test("输入 → 输入：为起点节点同侧补一个输出口后成线", () => {
		const a = makeNode("A", { ports: normalizePorts({ in: { bottom: 1 }, out: { bottom: 0 } }) });
		const b = makeNode("B", { x: 400 });
		const decision = resolveConnection(getNodeFrom([a, b]), [], { node: "A", dir: "in", side: "bottom", index: 0 }, { node: "B", dir: "in", side: "left", index: 0 });
		assert(decision.ok, decision.reason);
		assert(decision.createdPort, "应为 A 新建输出口");
		assertEqual(decision.createdPort.node, "A");
		assertEqual(decision.createdPort.dir, "out");
		assertEqual(decision.from.side, "bottom");
		assertEqual(decision.to.node, "B");
	})
	.test("拒绝自连与重复连线", () => {
		const a = makeNode("A");
		const same = resolveConnection(getNodeFrom([a]), [], { node: "A", dir: "out", side: "right", index: 0 }, { node: "A", dir: "out", side: "right", index: 0 });
		assert(!same.ok, "同一端口不应自连");
		const from = { node: "A", dir: "out", side: "right", index: 0 };
		const to = { node: "A", dir: "in", side: "left", index: 0 };
		const edges = [{ id: "e", from, to }];
		assert(edgeExists(edges, from, to), "应识别出重复连线");
		const dup = resolveConnection(getNodeFrom([a]), edges, from, to);
		assert(!dup.ok, "重复连线应被拒绝");
	})
	.test("同一个节点的不同端口可以自连（自环）", () => {
		const a = makeNode("A");
		const decision = resolveConnection(getNodeFrom([a]), [], { node: "A", dir: "out", side: "right", index: 0 }, { node: "A", dir: "in", side: "bottom", index: 0 });
		assert(decision.ok, decision.reason);
		assertEqual(decision.from.node, decision.to.node);
	})
	.test("缺失节点 / 非法端口一律拒绝", () => {
		const a = makeNode("A");
		assert(!resolveConnection(getNodeFrom([a]), [], { node: "A", dir: "out", side: "right", index: 0 }, { node: "Z", dir: "in", side: "left", index: 0 }).ok);
		assert(!resolveConnection(getNodeFrom([a]), [], null, { node: "A", dir: "in", side: "left", index: 0 }).ok);
	})
	.test("端口数量减少后：越界下标收敛、该向无端口的连线被删除", () => {
		const a = makeNode("A");
		const b = makeNode("B", { x: 400 });
		const edges = [
			{ id: "keep", from: { node: "A", dir: "out", side: "right", index: 1 }, to: { node: "B", dir: "in", side: "left", index: 0 } },
			{ id: "drop", from: { node: "A", dir: "out", side: "top", index: 0 }, to: { node: "B", dir: "in", side: "left", index: 0 } },
		];
		// A 的右侧输出口从 2 个减到 1 个 → keep 的下标 1 收敛为 0；上侧输出口清零 → drop 被删
		a.ports.out.right = 1;
		a.ports.out.top = 0;
		const map = getNodeFrom([a, b]);
		const drop = reconcileEdgesForPorts(edges, map);
		assertEqual(drop.length, 1);
		assertEqual(drop[0], "drop");
		assertEqual(edges[0].from.index, 0, "越界下标应被收敛到最后一个可用端口");
	})
	.test("枚举全部端口：数量与端口表一致", () => {
		const node = makeNode("A", { ports: normalizePorts({ in: { top: 2, left: 1 }, out: { bottom: 3 } }) });
		const refs = allPortRefs(node);
		assertEqual(refs.length, 6);
		assert(refs.every(isPortRef), "枚举结果应全部是合法端口引用");
	});
