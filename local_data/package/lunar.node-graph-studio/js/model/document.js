/**
 * 文档层：节点图的持久化形态与「信任边界」。
 *
 * 外部来源（磁盘 JSON / 剪贴板 / 导入文件）一律先经 sanitizeDocument 洗一遍：
 * 数值夹取、枚举校验、悬空连线剔除、端口下标收敛、重复 id 去重。
 * 这样下游（Graph / 渲染 / 交互）可以放心假设数据是干净的。
 */

import { defaultPorts, normalizePorts, portCount, SIDES } from "../core/ports.js";
import { EDGE_STYLES, ARROW_MODES } from "../core/router.js";
import { SHAPES, FONT_SIZES, MIN_NODE_W, MIN_NODE_H, MAX_NODE_W, MAX_NODE_H, getNodeType } from "./palette.js";
import { uid } from "../core/ids.js";

export const DOC_VERSION = 1;

export const DEFAULT_SETTINGS = {
	grid: 20,
	// 默认「自由摆放 + 对齐参考线」：栅格只作为视觉参考，不做位置量化。
	// 位置量化会让拖动按 20px 一跳，手感发涩；需要量化时顶栏「吸附」或右栏开关一键开启。
	snapGrid: false,
	snapAlign: true,
	showGrid: true,
	showMinimap: true,
	defaultWireStyle: "bezier",
	defaultArrowMode: "to",
	autosave: true,
};

export const DEFAULT_VIEWPORT = { x: 80, y: 60, zoom: 1 };

const ZOOM_MIN = 0.15;
const ZOOM_MAX = 4;
const COORD_LIMIT = 1e6;
const TEXT_LIMIT = 20000;

/** 由部分字段构造一份完整、已清洗的文档 */
export function createDocument(partial = {}) {
	return sanitizeDocument({ version: DOC_VERSION, ...partial });
}

/** 新建空白文档 */
export function emptyDocument(name = "未命名节点图") {
	return createDocument({
		name,
		nodes: [],
		edges: [],
		settings: { ...DEFAULT_SETTINGS },
		viewport: { ...DEFAULT_VIEWPORT },
	});
}

/**
 * 清洗/规整一份文档数据。
 * @param {any} raw
 * @returns {{version:number,name:string,nodes:Array,edges:Array,settings:object,viewport:object,createdAt:string,updatedAt:string}}
 */
export function sanitizeDocument(raw) {
	const src = raw && typeof raw === "object" ? raw : {};
	const now = new Date().toISOString();

	const nodes = [];
	const usedIds = new Set();
	for (const item of asArray(src.nodes)) {
		const node = sanitizeNode(item, usedIds);
		if (node) nodes.push(node);
	}

	const nodeIds = new Set(nodes.map((n) => n.id));
	const byId = new Map(nodes.map((n) => [n.id, n]));
	const edges = [];
	const edgeKeys = new Set();
	for (const item of asArray(src.edges)) {
		const edge = sanitizeEdge(item, nodeIds, byId);
		if (!edge) continue;
		const key = `${edge.from.node}|${edge.from.dir}|${edge.from.side}|${edge.from.index}->${edge.to.node}|${edge.to.dir}|${edge.to.side}|${edge.to.index}`;
		if (edgeKeys.has(key)) continue;
		edgeKeys.add(key);
		edges.push(edge);
	}

	return {
		version: DOC_VERSION,
		name: clampText(src.name, 120) || "未命名节点图",
		nodes,
		edges,
		settings: sanitizeSettings(src.settings),
		viewport: sanitizeViewport(src.viewport),
		createdAt: typeof src.createdAt === "string" && src.createdAt ? src.createdAt : now,
		updatedAt: now,
	};
}

export function sanitizeSettings(raw) {
	const src = raw && typeof raw === "object" ? raw : {};
	const out = { ...DEFAULT_SETTINGS };
	const grid = Number(src.grid);
	if (Number.isFinite(grid)) out.grid = Math.min(200, Math.max(2, Math.round(grid)));
	for (const key of ["snapGrid", "snapAlign", "showGrid", "showMinimap", "autosave"]) {
		if (typeof src[key] === "boolean") out[key] = src[key];
	}
	if (EDGE_STYLES.includes(src.defaultWireStyle)) out.defaultWireStyle = src.defaultWireStyle;
	if (ARROW_MODES.includes(src.defaultArrowMode)) out.defaultArrowMode = src.defaultArrowMode;
	return out;
}

export function sanitizeViewport(raw) {
	const src = raw && typeof raw === "object" ? raw : {};
	const x = Number(src.x);
	const y = Number(src.y);
	const zoom = Number(src.zoom);
	return {
		x: Number.isFinite(x) ? clampNum(x, -COORD_LIMIT, COORD_LIMIT) : DEFAULT_VIEWPORT.x,
		y: Number.isFinite(y) ? clampNum(y, -COORD_LIMIT, COORD_LIMIT) : DEFAULT_VIEWPORT.y,
		zoom: Number.isFinite(zoom) ? clampNum(zoom, ZOOM_MIN, ZOOM_MAX) : 1,
	};
}

export function sanitizeNode(raw, usedIds) {
	if (!raw || typeof raw !== "object") return null;
	let id = typeof raw.id === "string" && raw.id ? raw.id : uid("n");
	if (usedIds && usedIds.has(id)) id = uid("n");
	if (usedIds) usedIds.add(id);

	const preset = getNodeType(raw.type);
	const w = clampNum(numOr(raw.w, preset.w), MIN_NODE_W, MAX_NODE_W);
	const h = clampNum(numOr(raw.h, preset.h), MIN_NODE_H, MAX_NODE_H);

	return {
		id,
		type: typeof raw.type === "string" && raw.type ? raw.type : preset.key,
		x: clampNum(numOr(raw.x, 0), -COORD_LIMIT, COORD_LIMIT),
		y: clampNum(numOr(raw.y, 0), -COORD_LIMIT, COORD_LIMIT),
		w: Math.round(w),
		h: Math.round(h),
		title: clampText(raw.title, 400) || preset.title,
		text: clampText(raw.text, TEXT_LIMIT),
		color: sanitizeColor(raw.color) || preset.color,
		shape: SHAPES.includes(raw.shape) ? raw.shape : preset.shape,
		fontSize: FONT_SIZES.includes(Number(raw.fontSize)) ? Number(raw.fontSize) : preset.fontSize,
		ports: normalizePorts(raw.ports ?? defaultPorts()),
		locked: raw.locked === true,
		collapsed: raw.collapsed === true,
	};
}

export function sanitizeEdge(raw, nodeIds, byId) {
	if (!raw || typeof raw !== "object") return null;
	const from = sanitizeRef(raw.from, nodeIds, byId, "out");
	const to = sanitizeRef(raw.to, nodeIds, byId, "in");
	if (!from || !to) return null;
	if (from.node === to.node && from.dir === to.dir && from.side === to.side && from.index === to.index) return null;
	if (from.dir !== "out" || to.dir !== "in") return null;

	return {
		id: typeof raw.id === "string" && raw.id ? raw.id : uid("e"),
		from,
		to,
		style: EDGE_STYLES.includes(raw.style) ? raw.style : "bezier",
		arrow: ARROW_MODES.includes(raw.arrow) ? raw.arrow : "to",
		dashed: raw.dashed === true,
		label: clampText(raw.label, 200),
		color: sanitizeColor(raw.color) || null,
	};
}

function sanitizeRef(raw, nodeIds, byId, expectDir) {
	if (!raw || typeof raw !== "object") return null;
	const node = typeof raw.node === "string" ? raw.node : "";
	if (!node || !nodeIds.has(node)) return null;
	const side = SIDES.includes(raw.side) ? raw.side : expectDir === "out" ? "right" : "left";
	const index = Math.max(0, Math.floor(numOr(raw.index, 0)));
	const dir = raw.dir === "out" || raw.dir === "in" ? raw.dir : expectDir;
	const nodeObj = byId ? byId.get(node) : null;
	if (nodeObj) {
		const count = portCount(nodeObj, dir, side);
		if (count <= 0) return null;
		return { node, dir, side, index: Math.min(index, count - 1) };
	}
	return { node, dir, side, index };
}

export function sanitizeColor(value) {
	if (typeof value !== "string") return null;
	const v = value.trim();
	if (/^#[0-9a-fA-F]{3}$/.test(v) || /^#[0-9a-fA-F]{6}$/.test(v) || /^#[0-9a-fA-F]{8}$/.test(v)) return v.toLowerCase();
	if (/^rgba?\([0-9.,\s%]+\)$/.test(v)) return v;
	return null;
}

function asArray(value) {
	return Array.isArray(value) ? value : [];
}

function numOr(value, fallback) {
	const v = Number(value);
	return Number.isFinite(v) ? v : fallback;
}

function clampNum(v, lo, hi) {
	return Math.min(hi, Math.max(lo, v));
}

function clampText(value, max) {
	if (typeof value !== "string") return "";
	return value.length > max ? value.slice(0, max) : value;
}

/** 统计信息（状态栏/文档列表用） */
export function documentStats(doc) {
	const nodes = asArray(doc?.nodes);
	const edges = asArray(doc?.edges);
	let chars = 0;
	for (const node of nodes) chars += String(node.title || "").length + String(node.text || "").length;
	return { nodes: nodes.length, edges: edges.length, chars };
}
