import type { CodeLang } from './code-lang';

/** 检测到的命名作用域（class / func / method） */
export type Scope = {
	kind: "class" | "func" | "method";
	name: string;
	/** 该作用域的开启层级（花括号深度 或 缩进数量） */
	openLevel: number;
	/** 声明行本身（用于单行函数捕获） */
	headerLine: string;
	/** 累积的正文行 */
	lines: string[];
	/** 栈路径 */
	path: string;
};

export type Section = { level: number; title: string; content: string; path: string };

/* ------------------------------------------------------------------
 * 过滤字符串与注释，便于可靠统计花括号
 * ------------------------------------------------------------------ */
export function stripStringsAndComments(line: string, lang: CodeLang): string {
	// 逐字符扫描，跳过字符串（单/双/反引号）与注释（//、#、/* */），
	// 其余字符保留，从而让花括号统计不受字符串/注释干扰。
	let out = "";
	let i = 0;
	let blockComment = false;
	const n = line.length;
	const pyHash = lang === "py";
	while (i < n) {
		const c = line[i];
		const next = line[i + 1];
		if (blockComment) {
			if (c === "*" && next === "/") { blockComment = false; i += 2; }
			else i++;
			continue;
		}
		// 行注释
		if ((c === "/" && next === "/") || (pyHash && c === "#")) break;
		if (c === "/" && next === "*") { blockComment = true; i += 2; continue; }
		// 字符串字面量
		if (c === '"' || c === "'" || c === "`") {
			const quote = c;
			i++;
			while (i < n && line[i] !== quote) {
				if (line[i] === "\\") i++;
				i++;
			}
			i++;
			continue;
		}
		// 普通字符：保留花括号与其它（仅统计花括号时实际上只需括号，但保留无妨）
		if (c === "{" || c === "}") out += c;
		i++;
	}
	return out;
}

/** 计算某行的花括号净增量 */
export function braceDelta(line: string, lang: CodeLang): number {
	const clean = stripStringsAndComments(line, lang);
	let d = 0;
	for (const ch of clean) {
		if (ch === "{") d++;
		else if (ch === "}") d--;
	}
	return d;
}

/** 计算行首缩进数量（制表符按 4 空格计） */
export function indentLevel(line: string): number {
	const m = /^[ \t]*/.exec(line);
	if (!m) return 0;
	return (m[0].match(/\t/g)?.length ?? 0) * 4 + m[0].length;
}

/** 是否为空白/纯注释/纯装饰行（Python 缩进跳过的辅助） */
export function isInsignificant(line: string, lang: CodeLang): boolean {
	if (!line.trim()) return true;
	if (lang === "py" && /^\s*#/.test(line)) return true;
	if (/^\s*(?:\/\/|\/\*|\*\/)/.test(line)) return true;
	return false;
}
