/**
 * 纯几何工具：向量、矩形、贝塞尔/折线采样与角度。
 *
 * 本模块不依赖 DOM，可在浏览器与 Node 中同时运行（供 test/suites 直接测试）。
 */

export const EPS = 1e-6;

/** 构造向量（避免调用方到处写字面量对象） */
export function vec(x = 0, y = 0) {
	return { x, y };
}

export function add(a, b) {
	return { x: a.x + b.x, y: a.y + b.y };
}

export function sub(a, b) {
	return { x: a.x - b.x, y: a.y - b.y };
}

export function mul(a, k) {
	return { x: a.x * k, y: a.y * k };
}

export function len(a) {
	return Math.hypot(a.x, a.y);
}

export function dist(a, b) {
	return Math.hypot(a.x - b.x, a.y - b.y);
}

/** 单位化；零向量返回 (0,0)，调用方需自行判空 */
export function normalize(a) {
	const l = len(a);
	if (l < EPS) return { x: 0, y: 0 };
	return { x: a.x / l, y: a.y / l };
}

/** 逆时针 90°（屏幕坐标 y 向下时视觉上为顺时针，但仅用于法线偏移，语义一致即可） */
export function perp(a) {
	return { x: -a.y, y: a.x };
}

export function dot(a, b) {
	return a.x * b.x + a.y * b.y;
}

export function lerp(a, b, t) {
	return { x: a.x + (b.x - a.x) * t, y: a.y + (b.y - a.y) * t };
}

export function clamp(v, lo, hi) {
	if (Number.isNaN(v)) return lo;
	return Math.min(hi, Math.max(lo, v));
}

/** 向量方位角（度）。0° 指向 +x（右），顺时针增长（与屏幕坐标一致） */
export function angleOf(a) {
	return (Math.atan2(a.y, a.x) * 180) / Math.PI;
}

/** 按角度旋转向量（度） */
export function rotate(a, deg) {
	const r = (deg * Math.PI) / 180;
	const c = Math.cos(r);
	const s = Math.sin(r);
	return { x: a.x * c - a.y * s, y: a.x * s + a.y * c };
}

// ---------------------------------------------------------------- 矩形

export function rect(x, y, w, h) {
	return { x, y, w, h };
}

export function rectOf(node) {
	return { x: node.x, y: node.y, w: node.w, h: node.h };
}

export function rectFromPoints(p1, p2) {
	const x = Math.min(p1.x, p2.x);
	const y = Math.min(p1.y, p2.y);
	const w = Math.abs(p1.x - p2.x);
	const h = Math.abs(p1.y - p2.y);
	return { x, y, w, h };
}

export function rectContains(r, p) {
	return p.x >= r.x && p.x <= r.x + r.w && p.y >= r.y && p.y <= r.y + r.h;
}

export function rectIntersects(a, b) {
	if (a.x + a.w < b.x || b.x + b.w < a.x) return false;
	if (a.y + a.h < b.y || b.y + b.h < a.y) return false;
	return true;
}

export function rectUnion(a, b) {
	if (!a) return { ...b };
	if (!b) return { ...a };
	const x = Math.min(a.x, b.x);
	const y = Math.min(a.y, b.y);
	const x2 = Math.max(a.x + a.w, b.x + b.w);
	const y2 = Math.max(a.y + a.h, b.y + b.h);
	return { x, y, w: x2 - x, h: y2 - y };
}

export function rectCenter(r) {
	return { x: r.x + r.w / 2, y: r.y + r.h / 2 };
}

/** 矩形是否与「以中心缩放后的自身」等价的最小可容纳矩形（用于外扩命中区） */
export function inflateRect(r, pad) {
	return { x: r.x - pad, y: r.y - pad, w: r.w + pad * 2, h: r.h + pad * 2 };
}

// ---------------------------------------------------------------- 采样

/** 点到线段距离（用于连线命中测试） */
export function pointSegmentDistance(p, a, b) {
	const ab = sub(b, a);
	const l2 = dot(ab, ab);
	if (l2 < EPS) return dist(p, a);
	let t = dot(sub(p, a), ab) / l2;
	t = clamp(t, 0, 1);
	const proj = add(a, mul(ab, t));
	return dist(p, proj);
}

/** 折线总长度 */
export function polylineLength(points) {
	let total = 0;
	for (let i = 1; i < points.length; i++) total += dist(points[i - 1], points[i]);
	return total;
}

/** 折线中点（按弧长） */
export function polylineMidpoint(points) {
	return polylinePointAt(points, 0.5);
}

export function bezierPoint(p0, p1, p2, p3, t) {
	const u = 1 - t;
	const a = u * u * u;
	const b = 3 * u * u * t;
	const c = 3 * u * t * t;
	const d = t * t * t;
	return {
		x: a * p0.x + b * p1.x + c * p2.x + d * p3.x,
		y: a * p0.y + b * p1.y + c * p2.y + d * p3.y,
	};
}

/** 三次贝塞尔切线（未单位化） */
export function bezierTangent(p0, p1, p2, p3, t) {
	const u = 1 - t;
	const a = 3 * u * u;
	const b = 6 * u * t;
	const c = 3 * t * t;
	return {
		x: a * (p1.x - p0.x) + b * (p2.x - p1.x) + c * (p3.x - p2.x),
		y: a * (p1.y - p0.y) + b * (p2.y - p1.y) + c * (p3.y - p2.y),
	};
}

/** 沿折线按比例取点（ortho/straight 用，用于标签定位） */
export function polylinePointAt(points, ratio) {
	if (points.length === 0) return { x: 0, y: 0 };
	if (points.length === 1) return { ...points[0] };
	const total = polylineLength(points);
	if (total < EPS) return { ...points[0] };
	const target = total * clamp(ratio, 0, 1);
	let acc = 0;
	for (let i = 1; i < points.length; i++) {
		const seg = dist(points[i - 1], points[i]);
		if (acc + seg >= target) {
			const t = seg < EPS ? 0 : (target - acc) / seg;
			return lerp(points[i - 1], points[i], t);
		}
		acc += seg;
	}
	return { ...points[points.length - 1] };
}

/**
 * 去掉重复点与共线冗余点。
 *
 * 注意：只有「同向续行」的共线点才可删；折返回来的共线点（角度 180°）必须保留，
 * 否则会把折线拉直、破坏端口处的出/入方向（直角连线尤其依赖这个折点）。
 */
export function simplifyPolyline(points) {
	const out = [];
	for (const p of points) {
		const last = out[out.length - 1];
		if (last && dist(last, p) < 0.5) continue;
		out.push({ x: p.x, y: p.y });
	}
	if (out.length < 3) return out;
	const res = [out[0]];
	for (let i = 1; i < out.length - 1; i++) {
		const a = res[res.length - 1];
		const b = out[i];
		const c = out[i + 1];
		const ab = sub(b, a);
		const bc = sub(c, b);
		const cross = ab.x * bc.y - ab.y * bc.x;
		const sameDirection = dot(ab, bc) > 0;
		if (Math.abs(cross) < 0.5 && sameDirection) continue; // 同向共线，丢弃中间点
		res.push(b);
	}
	res.push(out[out.length - 1]);
	return res;
}

/** 数值保留小数（SVG 路径字符串瘦身） */
export function n(v, digits = 2) {
	if (!Number.isFinite(v)) return "0";
	const s = v.toFixed(digits);
	return s.replace(/\.?0+$/, "") || "0";
}
