/**
 * 端口模型 —— 「节点可向四面八方连接」的全部规则都在这里。
 *
 * 设计要点：
 *   · 端口是**纯身份**（node + dir + side + index），位置由节点几何 + 该侧端口数量推导，
 *     因此移动/缩放节点后端口自动跟随，不存在需要同步的冗余坐标。
 *   · 每个节点的每一侧都同时拥有输入口与输出口，四侧齐全 → 上下左右任意方向都能连。
 *   · 连线语义恒为 out → in（尾部=输出，头部=输入）；用户从任意一端起拖都能连成，
 *     同向投放时自动为落点节点补一个反向端口（见 resolveConnection）。
 *
 * 本模块不依赖 DOM。
 */

export const SIDES = ["top", "right", "bottom", "left"];

/** 各侧的朝外单位法向量 */
export const SIDE_VECTOR = {
	top: { x: 0, y: -1 },
	right: { x: 1, y: 0 },
	bottom: { x: 0, y: 1 },
	left: { x: -1, y: 0 },
};

/** 输入 ↔ 输出 互反 */
export function oppositeDir(dir) {
	return dir === "in" ? "out" : "in";
}

export function isSide(value) {
	return SIDES.includes(value);
}

export const SIDE_LABEL = {
	top: "上",
	right: "右",
	bottom: "下",
	left: "左",
};

export const DIR_LABEL = {
	in: "输入",
	out: "输出",
};

/** 默认端口配置：四侧各有 1 入 1 出 → 开箱即可向任意方向连线 */
export function defaultPorts() {
	return {
		in: { top: 1, right: 1, bottom: 1, left: 1 },
		out: { top: 1, right: 1, bottom: 1, left: 1 },
	};
}

/** 归一化端口表：补齐缺失键、夹取为 >=0 的整数，保证后续下标运算安全 */
export function normalizePorts(raw) {
	const base = { in: { top: 0, right: 0, bottom: 0, left: 0 }, out: { top: 0, right: 0, bottom: 0, left: 0 } };
	if (!raw || typeof raw !== "object") return base;
	for (const dir of ["in", "out"]) {
		const src = raw[dir];
		if (!src || typeof src !== "object") continue;
		for (const side of SIDES) {
			const v = Number(src[side]);
			base[dir][side] = Number.isFinite(v) && v > 0 ? Math.floor(v) : 0;
		}
	}
	return base;
}

export function portCount(node, dir, side) {
	const ports = node && node.ports ? node.ports[dir] : null;
	const v = ports ? Number(ports[side]) : 0;
	return Number.isFinite(v) && v > 0 ? v : 0;
}

export function portRef(node, dir, side, index = 0) {
	return { node, dir, side, index };
}

export function portKey(ref) {
	if (!ref) return "";
	return `${ref.node}|${ref.dir}|${ref.side}|${ref.index}`;
}

export function parsePortKey(key) {
	const [node, dir, side, index] = String(key || "").split("|");
	if (!node || !dir || !side) return null;
	return { node, dir, side, index: Number(index) || 0 };
}

export function samePort(a, b) {
	return !!a && !!b && portKey(a) === portKey(b);
}

export function isPortRef(value) {
	return !!value
		&& typeof value.node === "string"
		&& (value.dir === "in" || value.dir === "out")
		&& SIDES.includes(value.side)
		&& Number.isFinite(Number(value.index));
}

export function sanitizePortRef(ref) {
	if (!ref || typeof ref !== "object") return null;
	if (typeof ref.node !== "string" || !ref.node) return null;
	if (ref.dir !== "in" && ref.dir !== "out") return null;
	if (!SIDES.includes(ref.side)) return null;
	const index = Number(ref.index);
	return { node: ref.node, dir: ref.dir, side: ref.side, index: Number.isFinite(index) && index > 0 ? Math.floor(index) : 0 };
}

/**
 * 某一侧的端口槽位顺序：先排全部输入口，再接全部输出口（同侧两向不重叠）。
 * @returns {Array<{dir:"in"|"out", index:number}>}
 */
export function sideSlots(node, side) {
	const slots = [];
	const inCount = portCount(node, "in", side);
	const outCount = portCount(node, "out", side);
	for (let i = 0; i < inCount; i++) slots.push({ dir: "in", index: i });
	for (let i = 0; i < outCount; i++) slots.push({ dir: "out", index: i });
	return slots;
}

/** 端口在节点**局部坐标系**（左上角为原点）下的位置 */
export function portOffset(node, dir, side, index = 0) {
	const slots = sideSlots(node, side);
	const slot = slots.findIndex((s) => s.dir === dir && s.index === index);
	const total = Math.max(slots.length, 1);
	// 未登记的端口（例如数量已被改为 0）退化为该侧中点，避免出现 NaN 坐标
	const ratio = slot >= 0 ? (slot + 1) / (total + 1) : 0.5;
	const w = node.w;
	const h = node.h;
	switch (side) {
		case "top":
			return { x: w * ratio, y: 0 };
		case "bottom":
			return { x: w * ratio, y: h };
		case "left":
			return { x: 0, y: h * ratio };
		case "right":
		default:
			return { x: w, y: h * ratio };
	}
}

/** 端口在**世界坐标系**下的位置 */
export function portAnchor(node, dir, side, index = 0) {
	const off = portOffset(node, dir, side, index);
	return { x: node.x + off.x, y: node.y + off.y };
}

export function anchorOfRef(getNode, ref) {
	const node = getNode(ref.node);
	if (!node) return null;
	return portAnchor(node, ref.dir, ref.side, ref.index);
}

/** 端口朝外的单位方向（用于连线出线方向；输入口朝外＝背离节点，与输出口一致） */
export function portOutward(side) {
	return SIDE_VECTOR[side] || { x: 0, y: 0 };
}

/** 供状态栏/提示使用的中文描述 */
export function describePort(ref) {
	if (!ref) return "";
	return `${DIR_LABEL[ref.dir] || ref.dir}·${SIDE_LABEL[ref.side] || ref.side}#${ref.index}`;
}

/** 枚举一个节点上的全部端口引用 */
export function allPortRefs(node) {
	const out = [];
	for (const side of SIDES) {
		for (const dir of ["in", "out"]) {
			const count = portCount(node, dir, side);
			for (let i = 0; i < count; i++) out.push(portRef(node.id, dir, side, i));
		}
	}
	return out;
}

// ---------------------------------------------------------------- 连线合法性

export function edgeExists(edges, from, to) {
	const fk = portKey(from);
	const tk = portKey(to);
	return edges.some((e) => portKey(e.from) === fk && portKey(e.to) === tk);
}

/**
 * 解析一次「把 a 拖到 b」的连线意图。
 *
 * @param {(id:string)=>object|undefined} getNode
 * @param {Array} edges 现有连线（用于查重）
 * @param {object} a 起点端口引用
 * @param {object} b 落点端口引用
 * @returns {{ok:boolean, reason?:string, from?:object, to?:object,
 *            swapped?:boolean, createdPort?:{node:string,dir:string,side:string,index:number,count:number}}}
 */
export function resolveConnection(getNode, edges, a, b) {
	const ra = sanitizePortRef(a);
	const rb = sanitizePortRef(b);
	if (!ra || !rb) return { ok: false, reason: "端口信息不完整" };

	const na = getNode(ra.node);
	const nb = getNode(rb.node);
	if (!na || !nb) return { ok: false, reason: "端口所属节点不存在" };

	if (samePort(ra, rb)) return { ok: false, reason: "不能连接到自己" };

	let from;
	let to;
	let createdPort = null;
	let swapped = false;

	if (ra.dir !== rb.dir) {
		// 一入一出：谁拖谁落都能连，恒为 out → in
		if (ra.dir === "out") {
			from = ra;
			to = rb;
		} else {
			from = rb;
			to = ra;
			swapped = true;
		}
	} else if (ra.dir === "out") {
		// 输出→输出：给落点节点同一侧补一个输入口
		const ensured = ensureCounterpart(nb, "in", rb.side, rb.index);
		from = ra;
		to = ensured.ref;
		createdPort = ensured.createdPort;
	} else {
		// 输入→输入：给起点节点同一侧补一个输出口
		const ensured = ensureCounterpart(na, "out", ra.side, ra.index);
		from = ensured.ref;
		to = rb;
		createdPort = ensured.createdPort;
	}

	if (from.dir !== "out" || to.dir !== "in") {
		return { ok: false, reason: "连线方向解析失败" };
	}
	if (samePort(from, to)) return { ok: false, reason: "不能连接到自己" };
	if (edgeExists(edges, from, to)) return { ok: false, reason: "这两个端口之间已有连线" };

	return { ok: true, from, to, swapped, createdPort };
}

/**
 * 取同侧的反向端口：已有则复用最近的下标，没有则给出「需要新建」的端口信息。
 * 纯函数——不修改节点，由调用方按 createdPort 应用端口数量变更。
 */
function ensureCounterpart(node, dir, side, preferredIndex) {
	const count = portCount(node, dir, side);
	if (count > 0) {
		const index = Math.min(Math.max(0, Math.floor(preferredIndex) || 0), count - 1);
		return { ref: portRef(node.id, dir, side, index), createdPort: null };
	}
	return {
		ref: portRef(node.id, dir, side, 0),
		createdPort: { node: node.id, dir, side, index: 0, count: 1 },
	};
}

/** 对侧映射：左右互换、上下互换 */
export const MIRROR_SIDE = { top: "bottom", bottom: "top", left: "right", right: "left" };

/**
 * 取一个「流向相反」的端口引用（用于翻转连线方向）。
 *
 * 取端口顺序：**同侧同下标** → 对侧同下标 → 任意可用端口；都没有则返回 null。
 * 优先同侧是刻意的：A.right(out) → B.left(in) 翻转后得到 B.left(out) → A.right(in)，
 * 连线仍走原来那条走廊，只有箭头调头——这正是用户按「翻转方向」时期待的结果；
 * 若改成翻到对侧，直线样式会横穿节点自身。
 *
 * @param {(id:string)=>object|undefined} getNode
 * @param {{node:string,dir:"in"|"out",side:string,index:number}} ref
 * @param {"in"|"out"} dir 目标方向
 */
export function invertPortRef(getNode, ref, dir) {
	const node = getNode(ref.node);
	if (!node) return null;
	const mirroredSide = MIRROR_SIDE[ref.side] || ref.side;
	const candidates = [ref.side, mirroredSide, ...SIDES.filter((s) => s !== ref.side && s !== mirroredSide)];
	for (const side of candidates) {
		const count = portCount(node, dir, side);
		if (count > 0) return { node: ref.node, dir, side, index: Math.min(ref.index, count - 1) };
	}
	return null;
}

/**
 * 端口数量变更后，修正既有连线对新端口表的引用。
 * 返回需要删除的连线 id 列表（该侧该方向已无端口可用）。
 */
export function reconcileEdgesForPorts(edges, getNode) {
	const drop = [];
	for (const edge of edges) {
		for (const ref of [edge.from, edge.to]) {
			const node = getNode(ref.node);
			if (!node) {
				drop.push(edge.id);
				break;
			}
			const count = portCount(node, ref.dir, ref.side);
			if (count <= 0) {
				drop.push(edge.id);
				break;
			}
			if (ref.index > count - 1) ref.index = count - 1;
		}
	}
	return [...new Set(drop)];
}
