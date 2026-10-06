/**
 * 连线方向与四向连接的验收测试 —— 本次任务的核心要求。
 *
 * 断言的是「不变量」而不是具体坐标，因此重构路由实现也不会误判：
 *   ① 曲线必须从尾部端口锚点出发、在头部端口锚点结束；
 *   ② 尾部处的切线方向必须与该端口的朝外法向一致（线是「出」来的）；
 *   ③ 头部处的切线方向必须与朝内法向一致（箭头指进被连节点）；
 *   ④ 上/下/左/右 任意两侧组合（16 种）在三种线型下都成立。
 */

import { createSuite, assert, assertClose, assertEqual, makeNode } from "../harness.js";
import { routeEdge, routeDangling, hitTestRoute, arrowPath, orthoPath, computeFans } from "../../js/core/router.js";
import { portAnchor, SIDES, SIDE_VECTOR } from "../../js/core/ports.js";
import { dot, normalize, sub, angleOf, pointSegmentDistance } from "../../js/core/geometry.js";

const STYLES = ["bezier", "ortho", "straight"];

/** 构造两个节点：A 在左、B 在右，便于观察方向 */
function pair() {
	const a = makeNode("A", { x: 0, y: 0, w: 200, h: 100 });
	const b = makeNode("B", { x: 420, y: 260, w: 200, h: 100 });
	const map = new Map([[a.id, a], [b.id, b]]);
	return { a, b, getNode: (id) => map.get(id) };
}

function edgeOf(a, b, fromSide, toSide, style = "bezier") {
	return {
		id: "e",
		from: { node: a.id, dir: "out", side: fromSide, index: 0 },
		to: { node: b.id, dir: "in", side: toSide, index: 0 },
		style,
	};
}

export const routerSuite = createSuite("连线路由与方向（core/router）")
	.test("曲线端点严格落在端口锚点上", () => {
		const { a, b, getNode } = pair();
		for (const style of STYLES) {
			for (const fromSide of SIDES) {
				for (const toSide of SIDES) {
					const route = routeEdge(edgeOf(a, b, fromSide, toSide, style), getNode, { style });
					assert(route, `${style}/${fromSide}->${toSide} 应能求解`);
					const start = portAnchor(a, "out", fromSide, 0);
					const end = portAnchor(b, "in", toSide, 0);
					assertClose(route.start.x, start.x, 1e-6, `${style}/${fromSide}->${toSide} 起点 x`);
					assertClose(route.start.y, start.y, 1e-6, `${style}/${fromSide}->${toSide} 起点 y`);
					assertClose(route.end.x, end.x, 1e-6, `${style}/${fromSide}->${toSide} 终点 x`);
					assertClose(route.end.y, end.y, 1e-6, `${style}/${fromSide}->${toSide} 终点 y`);
				}
			}
		}
	})
	.test("尾端切线朝外：与端口朝外法向一致（四向 × 曲线/直角）", () => {
		const { a, b, getNode } = pair();
		// 直线样式的走向由两点连线决定（不保证垂直出线），故只对曲线与直角两条线型断言
		for (const style of ["bezier", "ortho"]) {
			for (const fromSide of SIDES) {
				for (const toSide of SIDES) {
					const route = routeEdge(edgeOf(a, b, fromSide, toSide, style), getNode, { style });
					const along = { x: Math.cos((route.startAngle * Math.PI) / 180), y: Math.sin((route.startAngle * Math.PI) / 180) };
					const outward = SIDE_VECTOR[fromSide];
					const alignment = dot(normalize(along), outward);
					assert(
						alignment > 0.95,
						`${style}/${fromSide}->${toSide}：尾端出线方向应与 ${fromSide} 侧朝外法向一致，实测对齐度 ${alignment.toFixed(4)}`,
					);
				}
			}
		}
	})
	.test("头端切线朝内：箭头必然指进被连节点（四向 × 曲线/直角）", () => {
		const { a, b, getNode } = pair();
		for (const style of ["bezier", "ortho"]) {
			for (const fromSide of SIDES) {
				for (const toSide of SIDES) {
					const route = routeEdge(edgeOf(a, b, fromSide, toSide, style), getNode, { style });
					const along = { x: Math.cos((route.endAngle * Math.PI) / 180), y: Math.sin((route.endAngle * Math.PI) / 180) };
					const inward = { x: -SIDE_VECTOR[toSide].x, y: -SIDE_VECTOR[toSide].y };
					const alignment = dot(normalize(along), inward);
					assert(
						alignment > 0.95,
						`${style}/${fromSide}->${toSide}：箭头方向应指向 ${toSide} 侧节点内部，实测对齐度 ${alignment.toFixed(4)}`,
					);
				}
			}
		}
	})
	.test("直线样式：箭头沿连线指向终点（必然指到端口）", () => {
		const { a, b, getNode } = pair();
		for (const fromSide of SIDES) {
			for (const toSide of SIDES) {
				const route = routeEdge(edgeOf(a, b, fromSide, toSide, "straight"), getNode, { style: "straight" });
				const expected = angleOf(sub(route.end, route.start));
				assertClose(route.endAngle, expected, 1e-6, `straight/${fromSide}->${toSide} 箭头方向应沿连线`);
				assertEqual(route.points.length, 2, "直线样式只有一段");
			}
		}
	})
	.test("箭头几何：朝 +x、尖端在原点", () => {
		const d = arrowPath(11);
		assert(d.startsWith("M 0 0"), "箭头尖端应在原点");
		assert(d.trim().endsWith("Z"), "箭头应是闭合多边形");
	})
	.test("直角折线的首末段方向与端口法向一致", () => {
		const { a, b, getNode } = pair();
		for (const fromSide of SIDES) {
			for (const toSide of SIDES) {
				const route = routeEdge(edgeOf(a, b, fromSide, toSide, "ortho"), getNode, { style: "ortho" });
				const first = sub(route.points[1], route.points[0]);
				const last = sub(route.points[route.points.length - 1], route.points[route.points.length - 2]);
				const outward = dot(normalize(first), SIDE_VECTOR[fromSide]);
				const inward = dot(normalize(last), { x: -SIDE_VECTOR[toSide].x, y: -SIDE_VECTOR[toSide].y });
				assert(outward > 0.95, `ortho ${fromSide}->${toSide} 首段应对齐朝外法向（${outward.toFixed(4)}）`);
				assert(inward > 0.95, `ortho ${fromSide}->${toSide} 末段应对齐朝内法向（${inward.toFixed(4)}）`);
			}
		}
	})
	.test("直角折线不含冗余点，且各段均为水平或竖直", () => {
		const { a, b, getNode } = pair();
		const route = routeEdge(edgeOf(a, b, "right", "top", "ortho"), getNode, { style: "ortho" });
		for (let i = 1; i < route.points.length; i++) {
			const p = route.points[i - 1];
			const q = route.points[i];
			const horizontal = Math.abs(p.y - q.y) < 1e-6;
			const vertical = Math.abs(p.x - q.x) < 1e-6;
			assert(horizontal || vertical, `第 ${i} 段既非水平也非竖直：${JSON.stringify([p, q])}`);
		}
	})
	.test("同侧自环（右→右）仍能从右侧出、从右侧进", () => {
		const a = makeNode("A", { x: 0, y: 0, w: 200, h: 120 });
		const getNode = () => a;
		const edge = {
			id: "loop",
			from: { node: "A", dir: "out", side: "right", index: 0 },
			to: { node: "A", dir: "in", side: "right", index: 0 },
		};
		const route = routeEdge(edge, getNode, { style: "bezier" });
		assert(route, "自环应能求解");
		const inward = dot(
			normalize({ x: Math.cos((route.endAngle * Math.PI) / 180), y: Math.sin((route.endAngle * Math.PI) / 180) }),
			{ x: -1, y: 0 },
		);
		assert(inward > 0.95, `自环箭头应指向节点内部（右侧入线），实测 ${inward.toFixed(4)}`);
		assert(route.end.x > route.start.x - 1e-6, "自环应向外伸出后再回到节点右侧");
	})
	.test("三点共线的折线会被化简", () => {
		const d = orthoPath([{ x: 0, y: 0 }, { x: 10, y: 0 }, { x: 20, y: 0 }], 8);
		assert(d.includes("L 20 0"), `共线点应被直接连线：${d}`);
	})
	.test("命中测试：线上命中、远处不命中", () => {
		const { a, b, getNode } = pair();
		const route = routeEdge(edgeOf(a, b, "right", "left", "bezier"), getNode, { style: "bezier" });
		const mid = route.mid;
		assert(hitTestRoute(mid, route, 6), "中点应命中");
		assert(!hitTestRoute({ x: mid.x + 400, y: mid.y + 400 }, route, 6), "远处点不应命中");
	})
	.test("临时连线（拖拽中）方向仍然正确", () => {
		const anchor = { x: 200, y: 50 };
		const free = { x: 520, y: 300 };
		for (const style of STYLES) {
			const route = routeDangling("right", anchor, free, style);
			assert(route, `${style} 临时连线应能求解`);
			if (style !== "straight") {
				const outward = dot(
					normalize({ x: Math.cos((route.startAngle * Math.PI) / 180), y: Math.sin((route.startAngle * Math.PI) / 180) }),
					{ x: 1, y: 0 },
				);
				assert(outward > 0.95, `${style} 临时连线应从右侧朝外出发（${outward.toFixed(4)}）`);
			}
			assertClose(route.start.x, anchor.x, 1e-6, `${style} 临时连线起点`);
			assertClose(route.end.x, free.x, 1e-6, `${style} 临时连线终点`);
			// 从输出口起拖时箭头跟手：末角方向应背离起点
			const towardsFree = angleOf(sub(free, anchor));
			const delta = Math.abs(((route.endAngle - towardsFree + 540) % 360) - 180);
			assert(delta < 75, `${style} 箭头应大致朝拖动方向（偏差 ${delta.toFixed(1)}°）`);
		}
	})
	.test("一对节点间的多条连线会扇开（不重叠）", () => {
		const edges = [0, 1, 2].map((i) => ({
			id: `e${i}`,
			from: { node: "A", dir: "out", side: "right", index: 0 },
			to: { node: "B", dir: "in", side: "left", index: 0 },
		}));
		const fans = computeFans(edges);
		assertEqual(fans.get("e0"), -26, "第一条应向左偏");
		assertEqual(fans.get("e1"), 0, "中间不偏");
		assertEqual(fans.get("e2"), 26, "第三条应向右偏");
	})
	.test("折线点到线段距离：垂足在段内", () => {
		const d = pointSegmentDistance({ x: 5, y: 3 }, { x: 0, y: 0 }, { x: 10, y: 0 });
		assertClose(d, 3, 1e-6, "垂距");
	});
