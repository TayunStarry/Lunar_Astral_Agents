/**
 * 导出 / 导入。
 *
 *   · JSON —— 完整工程文件（含端口、连线样式、视口、设置），可再导入继续编辑；
 *   · SVG  —— 矢量图，适合贴进文档/PPT，箭头方向与线型完整保留；
 *   · PNG  —— 位图，按 2 倍分辨率绘制，适合直接分享。
 *
 * SVG/PNG 都是「无 DOM 依赖」的绘制：连线几何复用 core/router，文字换行复用 core/text，
 * 因此导出的图与画布上看到的完全一致。
 */

import { sanitizeDocument } from "../model/document.js";
import { computeFans, routeEdge, sampleRoute, arrowPath } from "../core/router.js";
import { wrapText, lineHeight } from "../core/text.js";
import { rectOf } from "../core/geometry.js";
import { downloadBlob, downloadText, pickFile, readFileText, toast } from "../ui/dom.js";

const EXPORT_PADDING = 48;
const NODE_TEXT_PADDING = 12;
const HEAD_HEIGHT = 34;

/** 工程文件 → JSON 下载 */
export function exportJSON(store) {
	const doc = store.snapshotDocument();
	const name = doc.name || "节点图";
	downloadText(`${safeName(name)}.json`, JSON.stringify(doc, null, "\t"));
	return name;
}

/** 导入 JSON 工程文件 */
export async function importJSON() {
	const file = await pickFile(".json,application/json");
	if (!file) return null;
	const text = await readFileText(file);
	let raw;
	try {
		raw = JSON.parse(text);
	} catch (err) {
		toast("导入失败：不是合法的 JSON 文件", "error");
		return null;
	}
	return sanitizeDocument(raw);
}

// ---------------------------------------------------------------- 布局计算

/** 计算导出所需的世界包围盒（含连线外扩） */
function exportBounds(graph) {
	let box = null;
	const include = (x, y) => {
		if (!box) box = { x, y, w: 0, h: 0 };
		else {
			const x2 = Math.max(box.x + box.w, x);
			const y2 = Math.max(box.y + box.h, y);
			box.x = Math.min(box.x, x);
			box.y = Math.min(box.y, y);
			box.w = x2 - box.x;
			box.h = y2 - box.y;
		}
	};
	for (const node of graph.nodes) {
		const r = rectOf(node);
		include(r.x, r.y);
		include(r.x + r.w, r.y + r.h);
	}
	for (const edge of graph.edges) {
		const route = routeFor(graph, edge);
		if (!route) continue;
		for (const p of route.points) include(p.x, p.y);
	}
	if (!box) box = { x: 0, y: 0, w: 320, h: 200 };
	return {
		x: box.x - EXPORT_PADDING,
		y: box.y - EXPORT_PADDING,
		w: box.w + EXPORT_PADDING * 2,
		h: box.h + EXPORT_PADDING * 2,
	};
}

function routeFor(graph, edge, fans) {
	return routeEdge(edge, (id) => graph.getNode(id), {
		style: edge.style,
		bend: fans ? fans.get(edge.id) || 0 : 0,
	});
}

// ---------------------------------------------------------------- SVG

/**
 * 导出 SVG 矢量图。
 * @param {import("../app/store.js").Store} store
 * @param {{background?:string}} [opts]
 */
export function buildSVG(store, opts = {}) {
	const graph = store.graph;
	const background = opts.background || "#12162a";
	const bounds = exportBounds(graph);
	const fans = computeFans(graph.edges);
	const parts = [];

	parts.push(`<?xml version="1.0" encoding="UTF-8"?>`);
	parts.push(
		`<svg xmlns="http://www.w3.org/2000/svg" width="${round(bounds.w)}" height="${round(bounds.h)}" `
		+ `viewBox="${round(bounds.x)} ${round(bounds.y)} ${round(bounds.w)} ${round(bounds.h)}">`,
	);
	parts.push(`<rect x="${round(bounds.x)}" y="${round(bounds.y)}" width="${round(bounds.w)}" height="${round(bounds.h)}" fill="${background}"/>`);
	parts.push(`<g font-family="'Microsoft YaHei','PingFang SC',system-ui,sans-serif">`);

	// 连线
	for (const edge of graph.edges) {
		const route = routeFor(graph, edge, fans);
		if (!route) continue;
		const color = edge.color || "#7f96c7";
		const dash = edge.dashed ? ` stroke-dasharray="8 6"` : "";
		parts.push(`<path d="${route.d}" fill="none" stroke="${color}" stroke-width="2" stroke-linecap="round"${dash}/>`);
		if (edge.label) {
			parts.push(
				`<text x="${round(route.mid.x)}" y="${round(route.mid.y)}" fill="#c9d6f2" font-size="12" `
				+ `text-anchor="middle" dominant-baseline="middle" paint-order="stroke" stroke="${background}" stroke-width="4">${escapeXML(edge.label)}</text>`,
			);
		}
		if (edge.arrow !== "none") {
			parts.push(arrowTag(route.end, route.endAngle, color));
		}
		if (edge.arrow === "both") {
			parts.push(arrowTag(route.start, route.startAngle + 180, color));
		}
	}

	// 节点
	for (const node of graph.nodes) {
		parts.push(nodeTag(node));
	}

	parts.push(`</g></svg>`);
	return parts.join("\n");
}

function arrowTag(point, angle, color) {
	return `<path d="${arrowPath(11)}" fill="${color}" transform="translate(${round(point.x)},${round(point.y)}) rotate(${round(angle)})"/>`;
}

function nodeTag(node) {
	const r = rectOf(node);
	const color = node.color || "#5b8dff";
	const radius = node.shape === "rect" ? 4 : node.shape === "pill" ? node.h / 2 : 12;
	const parts = [];
	parts.push(`<g>`);
	parts.push(
		`<rect x="${round(r.x)}" y="${round(r.y)}" width="${round(r.w)}" height="${round(r.h)}" rx="${round(radius)}" `
		+ `fill="#1b2138" stroke="${color}" stroke-width="1.6"/>`,
	);
	// 顶部强调条
	parts.push(
		`<path d="M ${round(r.x + radius)} ${round(r.y + 3)} H ${round(r.x + r.w - radius)}" stroke="${color}" stroke-width="3" stroke-linecap="round" fill="none"/>`,
	);
	// 标题
	parts.push(
		`<text x="${round(r.x + NODE_TEXT_PADDING)}" y="${round(r.y + HEAD_HEIGHT / 2 + 1)}" fill="#eaf0ff" font-size="${node.fontSize + 1}" `
		+ `font-weight="600" dominant-baseline="middle">${escapeXML(truncate(node.title, r.w - NODE_TEXT_PADDING * 2, node.fontSize + 1))}</text>`,
	);
	// 正文（多行）
	const lines = wrapText(node.text, r.w - NODE_TEXT_PADDING * 2, node.fontSize);
	const lh = lineHeight(node.fontSize);
	let y = r.y + HEAD_HEIGHT + NODE_TEXT_PADDING / 2 + lh * 0.5;
	const maxLines = Math.max(0, Math.floor((r.h - HEAD_HEIGHT - NODE_TEXT_PADDING) / lh));
	lines.slice(0, maxLines).forEach((line, i) => {
		const clipped = i === maxLines - 1 && lines.length > maxLines ? `${line}…` : line;
		parts.push(
			`<text x="${round(r.x + NODE_TEXT_PADDING)}" y="${round(y)}" fill="#c3cee6" font-size="${node.fontSize}" dominant-baseline="middle">${escapeXML(clipped)}</text>`,
		);
		y += lh;
	});
	parts.push(`</g>`);
	return parts.join("");
}

export function exportSVG(store) {
	const svg = buildSVG(store);
	const name = store.graph.name || "节点图";
	downloadText(`${safeName(name)}.svg`, svg, "image/svg+xml");
	toast("已导出 SVG 矢量图", "ok");
}

// ---------------------------------------------------------------- PNG

/**
 * 把节点图画到离屏 canvas（2 倍分辨率位图）。
 * 与 exportPNG 分离，便于自动化测试直接检查绘制结果而不触发下载。
 * @returns {HTMLCanvasElement|null}
 */
export function renderPNGCanvas(store, scale = 2) {
	const graph = store.graph;
	const bounds = exportBounds(graph);
	const width = Math.max(1, Math.round(bounds.w * scale));
	const height = Math.max(1, Math.round(bounds.h * scale));
	if (width * height > 60e6) return null;
	const canvas = document.createElement("canvas");
	canvas.width = width;
	canvas.height = height;
	const ctx = canvas.getContext("2d");
	if (!ctx) return null;

	ctx.fillStyle = "#12162a";
	ctx.fillRect(0, 0, width, height);
	ctx.save();
	ctx.scale(scale, scale);
	ctx.translate(-bounds.x, -bounds.y);

	const fans = computeFans(graph.edges);

	// 连线 + 箭头
	for (const edge of graph.edges) {
		const route = routeFor(graph, edge, fans);
		if (!route) continue;
		const color = edge.color || "#7f96c7";
		ctx.strokeStyle = color;
		ctx.lineWidth = 2;
		ctx.setLineDash(edge.dashed ? [8, 6] : []);
		const points = route.style === "bezier" ? sampleRoute(route, 24) : route.points;
		ctx.beginPath();
		ctx.moveTo(points[0].x, points[0].y);
		for (let i = 1; i < points.length; i++) ctx.lineTo(points[i].x, points[i].y);
		ctx.stroke();
		ctx.setLineDash([]);
		if (edge.arrow !== "none") drawArrow(ctx, route.end, route.endAngle, color, 11);
		if (edge.arrow === "both") drawArrow(ctx, route.start, route.startAngle + 180, color, 11);
		if (edge.label) {
			ctx.font = "12px 'Microsoft YaHei',sans-serif";
			ctx.textAlign = "center";
			ctx.textBaseline = "middle";
			const w = ctx.measureText(edge.label).width + 10;
			ctx.fillStyle = "rgba(18,22,42,0.85)";
			ctx.fillRect(route.mid.x - w / 2, route.mid.y - 9, w, 18);
			ctx.fillStyle = "#c9d6f2";
			ctx.fillText(edge.label, route.mid.x, route.mid.y);
		}
	}

	// 节点
	for (const node of graph.nodes) {
		const r = rectOf(node);
		const color = node.color || "#5b8dff";
		const radius = node.shape === "rect" ? 4 : node.shape === "pill" ? r.h / 2 : 12;
		roundRect(ctx, r.x, r.y, r.w, r.h, radius);
		ctx.fillStyle = "#1b2138";
		ctx.fill();
		ctx.strokeStyle = color;
		ctx.lineWidth = 1.6;
		ctx.stroke();

		ctx.fillStyle = color;
		ctx.fillRect(r.x + radius, r.y + 2.5, Math.max(0, r.w - radius * 2), 3);

		ctx.textAlign = "left";
		ctx.textBaseline = "middle";
		ctx.fillStyle = "#eaf0ff";
		ctx.font = `600 ${node.fontSize + 1}px 'Microsoft YaHei',sans-serif`;
		ctx.fillText(truncate(node.title, r.w - NODE_TEXT_PADDING * 2, node.fontSize + 1), r.x + NODE_TEXT_PADDING, r.y + HEAD_HEIGHT / 2 + 1);

		ctx.fillStyle = "#c3cee6";
		ctx.font = `${node.fontSize}px 'Microsoft YaHei',sans-serif`;
		const lines = wrapText(node.text, r.w - NODE_TEXT_PADDING * 2, node.fontSize);
		const lh = lineHeight(node.fontSize);
		const maxLines = Math.max(0, Math.floor((r.h - HEAD_HEIGHT - NODE_TEXT_PADDING) / lh));
		let y = r.y + HEAD_HEIGHT + NODE_TEXT_PADDING / 2 + lh * 0.5;
		lines.slice(0, maxLines).forEach((line, i) => {
			const clipped = i === maxLines - 1 && lines.length > maxLines ? `${line}…` : line;
			ctx.fillText(clipped, r.x + NODE_TEXT_PADDING, y);
			y += lh;
		});
	}

	ctx.restore();
	return canvas;
}

/** 导出 PNG（2 倍分辨率位图） */
export async function exportPNG(store, scale = 2) {
	const canvas = renderPNGCanvas(store, scale);
	if (!canvas) {
		toast("图幅过大，建议缩小节点范围后再导出 PNG（或改用 SVG）", "warn");
		return;
	}
	const blob = await new Promise((resolve) => canvas.toBlob((b) => resolve(b), "image/png"));
	if (!blob) {
		toast("PNG 生成失败", "error");
		return;
	}
	downloadBlob(`${safeName(store.graph.name || "节点图")}.png`, blob);
	toast("已导出 PNG 位图", "ok");
}

function drawArrow(ctx, point, angle, color, size) {
	ctx.save();
	ctx.translate(point.x, point.y);
	ctx.rotate((angle * Math.PI) / 180);
	ctx.beginPath();
	ctx.moveTo(0, 0);
	ctx.lineTo(-size, -size * 0.46);
	ctx.lineTo(-size * 0.72, 0);
	ctx.lineTo(-size, size * 0.46);
	ctx.closePath();
	ctx.fillStyle = color;
	ctx.fill();
	ctx.restore();
}

function roundRect(ctx, x, y, w, h, r) {
	const radius = Math.max(0, Math.min(r, w / 2, h / 2));
	ctx.beginPath();
	ctx.moveTo(x + radius, y);
	ctx.arcTo(x + w, y, x + w, y + h, radius);
	ctx.arcTo(x + w, y + h, x, y + h, radius);
	ctx.arcTo(x, y + h, x, y, radius);
	ctx.arcTo(x, y, x + w, y, radius);
	ctx.closePath();
}

// ---------------------------------------------------------------- 工具

function truncate(text, maxWidth, fontSize) {
	const lines = wrapText(text, maxWidth, fontSize);
	if (lines.length <= 1) return lines[0] || "";
	return lines[0];
}

function round(v) {
	return Math.round(v * 100) / 100;
}

function escapeXML(text) {
	return String(text == null ? "" : text)
		.replace(/&/g, "&amp;")
		.replace(/</g, "&lt;")
		.replace(/>/g, "&gt;")
		.replace(/"/g, "&quot;");
}

function safeName(name) {
	return String(name || "节点图").replace(/[\\/:*?"<>|]+/g, "-").trim().slice(0, 80) || "节点图";
}
