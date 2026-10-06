/**
 * 连线路径求解 —— 「正确显示节点连线的显示方向」的核心。
 *
 * 每种样式都返回统一的路由结果：
 *   { style, d, points, start, end, startAngle, endAngle, mid }
 * 其中
 *   · start = 尾部（输出端口）锚点，end = 头部（输入端口）锚点；
 *   · endAngle  = 曲线在头部处的前进方向角（度）：箭头按此角旋转 → 箭头必然指进被连节点；
 *   · startAngle = 曲线在尾部处的前进方向角（度）：双向箭头用它 +180° 画尾箭头。
 *
 * 出线方向由端口所在侧决定（右侧端口先向右走、上方端口从上方进入），
 * 因此上/下/左/右四个方向的连线都不会穿进节点内部。
 *
 * 折线样式在「入线方向会折返」时自动改走外侧绕行，保证末段方向始终与端口法向一致。
 *
 * 本模块不依赖 DOM（SVG 路径字符串亦为纯计算），可在 Node 中测试。
 */

import {
	angleOf, bezierPoint, bezierTangent, clamp, dist, n, normalize, perp,
	polylineMidpoint, rectOf, simplifyPolyline, sub, mul, add, EPS,
} from "./geometry.js";

export const EDGE_STYLES = ["bezier", "ortho", "straight"];

export const EDGE_STYLE_LABEL = {
	bezier: "曲线",
	ortho: "直角",
	straight: "直线",
};

export const ARROW_MODES = ["to", "both", "none"];

export const ARROW_MODE_LABEL = {
	to: "终点箭头",
	both: "双向箭头",
	none: "无箭头",
};

const DEFAULT_STUB = 26;
/** 自环时控制点外伸的最小长度，保证环画得开、看得清 */
const SELF_LOOP_MIN_K = 110;

function sideVec(side) {
	switch (side) {
		case "top": return { x: 0, y: -1 };
		case "bottom": return { x: 0, y: 1 };
		case "left": return { x: -1, y: 0 };
		case "right":
		default: return { x: 1, y: 0 };
	}
}

/** 角度：向量退化时回退到 fallback（度） */
function safeAngle(v, fallback) {
	if (!v) return fallback;
	if (Math.abs(v.x) < EPS && Math.abs(v.y) < EPS) return fallback;
	return angleOf(v);
}

function sign(v) {
	if (v > EPS) return 1;
	if (v < -EPS) return -1;
	return 0;
}

/**
 * 求解一条连线的几何。
 *
 * @param {{from:object, to:object}} edge 连线（读取 from/to 端口引用）
 * @param {(id:string)=>object|undefined} getNode 节点查询
 * @param {{style?:string, bend?:number, stub?:number}} [opts]
 */
export function routeEdge(edge, getNode, opts = {}) {
	const fromNode = getNode(edge.from.node);
	const toNode = getNode(edge.to.node);
	if (!fromNode || !toNode) return null;

	const style = EDGE_STYLES.includes(opts.style) ? opts.style : "bezier";
	const bend = Number.isFinite(opts.bend) ? opts.bend : 0;
	const stub = Number.isFinite(opts.stub) ? opts.stub : DEFAULT_STUB;

	const start = anchorOn(fromNode, edge.from.dir, edge.from.side, edge.from.index);
	const end = anchorOn(toNode, edge.to.dir, edge.to.side, edge.to.index);
	const vStart = sideVec(edge.from.side);
	const vEnd = sideVec(edge.to.side);
	const selfLoop = edge.from.node === edge.to.node;

	if (style === "straight") return routeStraight(start, end, bend);
	if (style === "ortho") return routeOrtho(start, end, vStart, vEnd, stub, selfLoop, bend, rectOf(fromNode), rectOf(toNode));
	return routeBezier(start, end, vStart, vEnd, stub, selfLoop, bend);
}

/** 端口锚点：与 core/ports.js 的 portAnchor 同构（此处内联实现以避免模块循环依赖） */
function anchorOn(node, dir, side, index) {
	const ratios = sideRatios(node, side);
	const ratio = Number.isFinite(ratios[`${dir}:${index}`]) ? ratios[`${dir}:${index}`] : 0.5;
	switch (side) {
		case "top": return { x: node.x + node.w * ratio, y: node.y };
		case "bottom": return { x: node.x + node.w * ratio, y: node.y + node.h };
		case "left": return { x: node.x, y: node.y + node.h * ratio };
		case "right":
		default: return { x: node.x + node.w, y: node.y + node.h * ratio };
	}
}

/** 某侧端口沿边的比例位置：先排输入口、再排输出口（与 ports.sideSlots 一致） */
function sideRatios(node, side) {
	const ports = node.ports || {};
	const inCount = countOf(ports.in, side);
	const outCount = countOf(ports.out, side);
	const total = inCount + outCount;
	const ratios = {};
	let slot = 0;
	for (let i = 0; i < inCount; i++) {
		ratios[`in:${i}`] = total > 0 ? (slot + 1) / (total + 1) : 0.5;
		slot++;
	}
	for (let i = 0; i < outCount; i++) {
		ratios[`out:${i}`] = total > 0 ? (slot + 1) / (total + 1) : 0.5;
		slot++;
	}
	return ratios;
}

function countOf(sideMap, side) {
	const v = sideMap ? Number(sideMap[side]) : 0;
	return Number.isFinite(v) && v > 0 ? Math.floor(v) : 0;
}

function finish(style, points, startAngle, endAngle, mid, selfLoop, d) {
	return {
		style,
		points,
		start: points[0],
		end: points[points.length - 1],
		startAngle,
		endAngle,
		mid,
		selfLoop: !!selfLoop,
		d,
	};
}

// ---------------------------------------------------------------- 直线

function routeStraight(start, end, bend) {
	// 直线样式下 bend 以法向整体平移线段（多线并行时避免完全重叠）
	const offset = mul(normalize(perp(sub(end, start))), bend);
	const s = add(start, offset);
	const e = add(end, offset);
	const angle = safeAngle(sub(e, s), 0);
	return finish(
		"straight",
		[s, e],
		angle,
		angle,
		{ x: (s.x + e.x) / 2, y: (s.y + e.y) / 2 },
		false,
		`M ${n(s.x)} ${n(s.y)} L ${n(e.x)} ${n(e.y)}`,
	);
}

// ---------------------------------------------------------------- 贝塞尔

function routeBezier(start, end, vStart, vEnd, stub, selfLoop, bend) {
	const d = dist(start, end);
	// 控制点外伸长度随距离增长；自环额外放大，环才画得开
	const k = selfLoop ? clamp(d * 0.8, SELF_LOOP_MIN_K, 320) : clamp(d * 0.42, stub, 240);
	const c1 = add(start, mul(vStart, k));
	const c2 = add(end, mul(vEnd, k));

	if (bend) {
		// 并行线：沿连线法向整体弯折（平移两个控制点），端口处切线方向不受影响
		const shift = mul(normalize(perp(sub(end, start))), bend);
		c1.x += shift.x;
		c1.y += shift.y;
		c2.x += shift.x;
		c2.y += shift.y;
	}

	const startAngle = safeAngle(bezierTangent(start, c1, c2, end, 0), angleOf(vStart));
	const endAngle = safeAngle(bezierTangent(start, c1, c2, end, 1), angleOf(vEnd));
	const mid = bezierPoint(start, c1, c2, end, 0.5);

	return finish(
		"bezier",
		[start, c1, c2, end],
		startAngle,
		endAngle,
		mid,
		selfLoop,
		`M ${n(start.x)} ${n(start.y)} C ${n(c1.x)} ${n(c1.y)}, ${n(c2.x)} ${n(c2.y)}, ${n(end.x)} ${n(end.y)}`,
	);
}

// ---------------------------------------------------------------- 直角折线

/**
 * 直角折线路由。
 *
 * 方向不变量（由测试逐项验证）：
 *   首段 = vStart（从端口朝外出发）；末段的走向 = −vEnd（正对着端口驶入节点）。
 * 在满足方向不变量的前提下，按四种朝向组合挑一条好看的走法：
 *   · 横出横入：优先在两条出/入桩之间折一个竖弯；桩位交叉时改从入线侧外侧绕。
 *   · 横出竖入 / 竖出横入：优先一次拐角直达；拐角会走反时改走外侧车道。
 *   · 竖出竖入：一律走「外侧竖车道」（先横移到节点外侧，再竖直跨越，最后拐进端口），
 *     避免竖线从自己的节点里穿过去。
 */
function routeOrtho(start, end, vStart, vEnd, stub, selfLoop, bend, rectA, rectB) {
	const s = clamp(stub, 12, 80);
	const O = add(start, mul(vStart, s)); // 出线桩
	const N = add(end, mul(vEnd, s));     // 入线桩
	const hStart = Math.abs(vStart.x) > EPS;
	const hEnd = Math.abs(vEnd.x) > EPS;

	let raw;
	if (hStart && hEnd) {
		const midX = (O.x + N.x) / 2;
		if (sign(end.x - midX) === -sign(vEnd.x)) {
			raw = [start, O, { x: midX, y: O.y }, { x: midX, y: end.y }, end];
		} else {
			// 中折会折返：改在入线侧外侧竖直通过
			raw = [start, O, { x: N.x, y: O.y }, N, end];
		}
	} else if (hStart && !hEnd) {
		if (sign(end.y - O.y) === -sign(vEnd.y)) {
			raw = [start, O, { x: end.x, y: O.y }, end];
		} else {
			raw = [start, O, { x: O.x, y: N.y }, { x: end.x, y: N.y }, end];
		}
	} else if (!hStart && hEnd) {
		if (sign(O.x - end.x) === sign(vEnd.x)) {
			raw = [start, O, { x: O.x, y: end.y }, end];
		} else {
			raw = [start, O, { x: N.x, y: O.y }, N, end];
		}
	} else {
		// 竖出竖入：先横移到两个节点之外，再竖直跨越
		const laneX = outerLaneX(rectA, rectB, s, O.x, end.x);
		raw = [start, O, { x: laneX, y: O.y }, { x: laneX, y: N.y }, { x: end.x, y: N.y }, end];
	}

	if (bend) {
		const shift = mul(normalize(perp(sub(end, start))), bend);
		for (const p of raw) {
			p.x += shift.x;
			p.y += shift.y;
		}
	}

	const points = simplifyPolyline(raw);
	const firstSeg = points.length > 1 ? sub(points[1], points[0]) : vStart;
	const lastSeg = points.length > 1 ? sub(points[points.length - 1], points[points.length - 2]) : vEnd;

	return finish(
		"ortho",
		points,
		safeAngle(firstSeg, angleOf(vStart)),
		safeAngle(lastSeg, angleOf(vEnd)),
		polylineMidpoint(points),
		selfLoop,
		orthoPath(points, 8),
	);
}

/** 取一条位于两个节点之外（更靠近给定 x）的竖直车道 */
function outerLaneX(rectA, rectB, s, xa, xb) {
	const center = (xa + xb) / 2;
	if (!rectA || !rectB) return (xa >= xb ? Math.max(xa, xb) : Math.min(xa, xb)) + (xa >= xb ? s : -s);
	const rightLane = Math.max(rectA.x + rectA.w, rectB.x + rectB.w) + s;
	const leftLane = Math.min(rectA.x, rectB.x) - s;
	return Math.abs(rightLane - center) <= Math.abs(center - leftLane) ? rightLane : leftLane;
}

/** 折线 → 带圆角的 SVG 路径（半径受相邻半段长度约束，不会画过头） */
export function orthoPath(points, radius = 8) {
	if (points.length < 2) return "";
	if (points.length === 2) {
		return `M ${n(points[0].x)} ${n(points[0].y)} L ${n(points[1].x)} ${n(points[1].y)}`;
	}
	let d = `M ${n(points[0].x)} ${n(points[0].y)}`;
	for (let i = 1; i < points.length - 1; i++) {
		const prev = points[i - 1];
		const cur = points[i];
		const next = points[i + 1];
		const r = Math.min(radius, dist(prev, cur) / 2, dist(cur, next) / 2);
		if (r < 0.6) {
			d += ` L ${n(cur.x)} ${n(cur.y)}`;
			continue;
		}
		const p1 = add(cur, mul(normalize(sub(prev, cur)), r));
		const p2 = add(cur, mul(normalize(sub(next, cur)), r));
		d += ` L ${n(p1.x)} ${n(p1.y)} Q ${n(cur.x)} ${n(cur.y)} ${n(p2.x)} ${n(p2.y)}`;
	}
	const last = points[points.length - 1];
	return `${d} L ${n(last.x)} ${n(last.y)}`;
}

// ---------------------------------------------------------------- 拖拽中的临时连线

/**
 * 临时连线：anchor 是真实端口锚点，free 是鼠标位置。
 *
 * 曲线从真实端口按其所在侧朝外出发，末端朝「背离 anchor」的方向延伸，
 * 因此 endAngle 就是拖拽前进方向 —— 从输出口起拖时箭头跟手，从输入口起拖时
 * 由调用方把箭头画在 anchor 端（指向端口）。
 */
export function routeDangling(anchorSide, anchorPoint, freePoint, style = "bezier") {
	const vStart = sideVec(anchorSide);
	const incoming = sub(anchorPoint, freePoint);
	const vEnd = normalize(incoming);
	if (Math.abs(vEnd.x) < EPS && Math.abs(vEnd.y) < EPS) {
		// 鼠标压在端口上：退化法向，避免 NaN 路径
		return routeBezier(anchorPoint, freePoint, vStart, { x: -vStart.x, y: -vStart.y }, 26, false, 0);
	}
	const syntheticEnd = freePoint;
	if (style === "straight") return routeStraight(anchorPoint, syntheticEnd, 0);
	if (style === "ortho") return routeOrtho(anchorPoint, syntheticEnd, vStart, vEnd, 26, false, 0, null, null);
	return routeBezier(anchorPoint, syntheticEnd, vStart, vEnd, 26, false, 0);
}

// ---------------------------------------------------------------- 命中与采样

/** 世界坐标点是否落在连线上（点选连线）；tolerance 为世界单位容差 */
export function hitTestRoute(point, route, tolerance = 6) {
	if (!route || !route.points || route.points.length < 2) return false;
	if (route.style === "bezier") {
		const [p0, p1, p2, p3] = route.points;
		let prev = p0;
		for (let i = 1; i <= 24; i++) {
			const cur = bezierPoint(p0, p1, p2, p3, i / 24);
			if (segmentHit(point, prev, cur, tolerance)) return true;
			prev = cur;
		}
		return false;
	}
	for (let i = 1; i < route.points.length; i++) {
		if (segmentHit(point, route.points[i - 1], route.points[i], tolerance)) return true;
	}
	return false;
}

function segmentHit(p, a, b, tolerance) {
	const ab = sub(b, a);
	const l2 = ab.x * ab.x + ab.y * ab.y;
	if (l2 < EPS) return dist(p, a) <= tolerance;
	const t = clamp(((p.x - a.x) * ab.x + (p.y - a.y) * ab.y) / l2, 0, 1);
	return dist(p, { x: a.x + ab.x * t, y: a.y + ab.y * t }) <= tolerance;
}

/** 折线近似采样（缩略图/位图导出用） */
export function sampleRoute(route, steps = 16) {
	if (!route) return [];
	if (route.style !== "bezier") return route.points.slice();
	const [p0, p1, p2, p3] = route.points;
	const out = [];
	for (let i = 0; i <= steps; i++) out.push(bezierPoint(p0, p1, p2, p3, i / steps));
	return out;
}

/** 连线中点（标签定位） */
export function routeMidpoint(route) {
	if (!route) return { x: 0, y: 0 };
	if (route.mid) return route.mid;
	if (route.style === "bezier" && route.points.length === 4) {
		const [p0, p1, p2, p3] = route.points;
		return bezierPoint(p0, p1, p2, p3, 0.5);
	}
	return polylineMidpoint(route.points);
}

/**
 * 箭头多边形：默认朝 +x，尖端位于原点；调用方 translate 到端点并 rotate(angle) 摆放。
 * 尾部内凹（0.72）让箭头显得更利落。
 */
export function arrowPath(size = 11) {
	const half = size * 0.46;
	return `M 0 0 L ${n(-size)} ${n(-half)} L ${n(-size * 0.72)} 0 L ${n(-size)} ${n(half)} Z`;
}

/** 同一对节点之间的多条连线沿法向扇开，避免完全重叠 */
export const FAN_GAP = 26;

/**
 * 计算每条连线的扇开偏移量。
 * @param {Array} edges
 * @returns {Map<string, number>} edgeId → bend
 */
export function computeFans(edges) {
	const groups = new Map();
	for (const edge of edges) {
		const a = edge.from.node;
		const b = edge.to.node;
		const key = a < b ? `${a}::${b}` : `${b}::${a}`;
		if (!groups.has(key)) groups.set(key, []);
		groups.get(key).push(edge);
	}
	const out = new Map();
	for (const list of groups.values()) {
		if (list.length < 2) {
			for (const edge of list) out.set(edge.id, 0);
			continue;
		}
		list.forEach((edge, i) => {
			out.set(edge.id, (i - (list.length - 1) / 2) * FAN_GAP);
		});
	}
	return out;
}
