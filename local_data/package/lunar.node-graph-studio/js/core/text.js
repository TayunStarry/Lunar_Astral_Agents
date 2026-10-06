/**
 * 文字度量与换行：SVG / PNG 导出没有 DOM 可测，节点也不该为测高触发重排，
 * 因此统一用一套「按字符宽度估算」的度量：中文/全角按 1 个字宽，其余按 0.55。
 *
 * 纯函数，浏览器与 Node 通用（导出与渲染共用同一套换行结果，所见即所得）。
 */

const FULL_WIDTH = /[\u1100-\u115F\u2E80-\u303E\u3041-\u33FF\u3400-\u4DBF\u4E00-\u9FFF\uA000-\uA4CF\uAC00-\uD7A3\uF900-\uFAFF\uFE30-\uFE4F\uFF00-\uFF60\uFFE0-\uFFE6]/;

/** 单个字符的宽度（单位：字宽） */
export function charUnits(ch) {
	if (ch === "\t") return 2;
	if (ch === " ") return 0.3;
	return FULL_WIDTH.test(ch) ? 1 : 0.55;
}

/** 整段文字（不含换行）的宽度（单位：字宽） */
export function textUnits(text) {
	let sum = 0;
	for (const ch of String(text || "")) sum += charUnits(ch);
	return sum;
}

/** 估算行高 */
export function lineHeight(fontSize) {
	return Math.round((fontSize || 13) * 1.5);
}

/**
 * 按像素宽度换行；显式换行符优先。
 * @param {string} text
 * @param {number} maxWidth 可用像素宽度
 * @param {number} fontSize
 * @returns {string[]}
 */
export function wrapText(text, maxWidth, fontSize = 13) {
	const limit = Math.max(1, maxWidth / Math.max(1, fontSize));
	const lines = [];
	for (const paragraph of String(text == null ? "" : text).split("\n")) {
		if (paragraph === "") {
			lines.push("");
			continue;
		}
		let current = "";
		let units = 0;
		for (const ch of paragraph) {
			const u = charUnits(ch);
			if (units + u > limit && current !== "") {
				lines.push(current);
				current = "";
				units = 0;
			}
			current += ch;
			units += u;
		}
		lines.push(current);
	}
	return lines;
}

/** 文字块所需高度（像素） */
export function textBlockHeight(text, maxWidth, fontSize = 13) {
	return wrapText(text, maxWidth, fontSize).length * lineHeight(fontSize);
}
