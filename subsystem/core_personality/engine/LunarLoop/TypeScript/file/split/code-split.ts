/** 代码感知拆分入口 — 聚合语言识别/作用域/声明检测模块，保持公共 API 不变 */
export { CodeLang, resolveCodeLang } from './code-lang';
import { CODE_PATH_PREFIX, codeHeader, BRACE_LANGS } from './code-lang';
import type { CodeLang } from './code-lang';
import type { Section, Scope } from './code-scope';
import { isInsignificant, braceDelta, indentLevel } from './code-scope';
import { detectDeclaration, detectPython } from './code-detect';

/* ------------------------------------------------------------------
 * 核心拆分：把代码文件切成保留 class/function 单位的字符串片段
 * ------------------------------------------------------------------ */
export function splitCodeFile(content: string, lang: CodeLang, idealLen: number): string[] {
	const text = (content ?? "").replace(/\r\n/g, "\n");
	if (!text.trim()) return [];
	const lines = text.split("\n");
	const brace = !!(BRACE_LANGS[lang]);

	/** 预处理：每行的层级（花括号深度 或 缩进），以及声明与花括号增量 */
	type LineMeta = { lvl: number; braceDelta: number; decl: { kind: any; name: string } | null; text: string };
	const meta: LineMeta[] = [];
	let running = 0;
	for (const line of lines) {
		const startLvl = brace ? running : 0;
		const d = brace ? braceDelta(line, lang) : 0;
		if (brace) {
			meta.push({ lvl: startLvl, braceDelta: d, decl: detectDeclaration(line, lang), text: line });
			running += d;
		} else {
			const lvl = indentLevel(line);
			meta.push({ lvl, braceDelta: 0, decl: detectPython(line), text: line });
		}
	}

	const sections: Section[] = [];
	const preamble: string[] = [];
	/** 作用域栈：栈顶为当前最内层 */
	const stack: Scope[] = [];

	/** 终结栈顶作用域，生成一个 Section */
	const closeScope = (top: Scope) => {
		const content = top.lines.length > 0 ? top.lines.join("\n") : top.headerLine;
		sections.push({ level: stack.length, title: top.name, content: content.trimEnd() + "\n", path: top.path });
	};

	// 统一按行遍历（braced 用栈 pop 判定，python 用缩进判定）
	for (let i = 0; i < meta.length; i++) {
		const m = meta[i];

		// 关闭层级已降到作用域开启层级以下的作用域
		if (brace) {
			// 花括号结束判定：本行起始层级低于/等于栈顶 openLevel 则说明其块已闭合
			while (stack.length && m.lvl <= stack[stack.length - 1].openLevel) {
				const top = stack.pop()!;
				closeScope(top);
			}
		} else {
			// Python：跳过注释/空白，用非空行缩进判定块的结束
			if (!isInsignificant(m.text, "py")) {
				while (stack.length && m.lvl <= stack[stack.length - 1].openLevel) {
					const top = stack.pop()!;
					closeScope(top);
				}
			}
		}

		// 若是声明行：压入新作用域（声明行本身作为其头部，进入该作用域）
		if (m.decl) {
			const parentPath = stack.length ? stack[stack.length - 1].path : "";
			const path = parentPath ? `${parentPath} / ${m.decl.name}` : m.decl.name;
			stack.push({
				kind: m.decl.kind, name: m.decl.name,
				openLevel: m.lvl, headerLine: m.text, lines: [], path,
			});
			continue;
		}

		// 普通行：追加到栈顶（若无栈则归入文件头部 preamble）
		if (stack.length) stack[stack.length - 1].lines.push(m.text);
		else preamble.push(m.text);
	}
	// 收尾：关闭所有残留作用域；preamble 作为头部片段输出
	while (stack.length) closeScope(stack.pop()!);

	// 组装输出片段：每个 section 生成 header + content；超长则按行切分
	const output: string[] = [];

	// 若存在 preamble 且非空，单独输出为头部片段
	const pre = preamble.join("\n").trimEnd();
	if (pre) output.push((CODE_PATH_PREFIX + "*文件头\n" + pre).trimEnd() + "\n");

	const pushSection = (path: string, body: string) => {
		const header = codeHeader(path);
		if (body.length <= idealLen) {
			const piece = (header + body).trimEnd();
			if (piece.trim()) output.push(piece);
			return;
		}
		// 超长：按行切
		const chunks = splitLines(body, idealLen);
		for (const c of chunks) {
			const piece = (header + c).trimEnd();
			if (piece.trim()) output.push(piece);
		}
	};

	for (const s of sections) {
		// 跳过只有声明的空内容但保留签名（单行函数已存于 content=headerLine，这里非空）
		pushSection(s.path, s.content);
	}
	return output;
}

/** 行优先拆分（无智能断句，硬切长行） */
function splitLines(text: string, idealLen: number): string[] {
	const result: string[] = [];
	let buffer = "";
	const flush = () => { if (buffer.trim()) result.push(buffer.trimEnd() + "\n"); buffer = ""; };
	const ls = text.replace(/\r\n/g, "\n").split("\n");
	for (const line of ls) {
		const append = (buffer === "" ? "" : "\n") + line;
		if ((buffer + append).length <= idealLen) { buffer += append; continue; }
		if (buffer.trim()) flush();
		if (line.length > idealLen) {
			for (let o = 0; o < line.length; o += idealLen) {
				result.push(line.slice(o, o + idealLen).trimEnd() + "\n");
			}
		} else buffer = line;
	}
	if (buffer.trim()) flush();
	return result;
}

/* ------------------------------------------------------------------
 * 启发式标签：从代码路径/片段提取检索标签（供 memoryAddWithTags 使用）
 * ------------------------------------------------------------------ */
const STOP_WORDS = new Set([
	"the","a","an","this","that","with","from","for","and","or","of","to","in","on","is","are","at","by","as","be",
	"if","then","else","when","while","return","new","const","let","var","func","func ","function","class","struct",
	"type","package","import","export","default","interface","impl","for_import","base","main",
]);

/**
 * 从切片路径（如 `class User / func Login`）与源码片段提取标签。
 * 标签 = 路径中的层级标识 + 语言名。路径标识是天然的高频关键词，检索精准度高于梁覆盖度。
 */
export function extractCodeTags(path: string, lang: CodeLang): string[] {
	const tags: string[] = [lang];
	for (const seg of path.split("/")) {
		const token = seg.trim();
		if (!token) continue;
		// 去掉尾部括号参数，取标识符本体
		const name = token.split("(")[0].trim();
		// 标识符：允许字母数字下划线、泛型 <>、作用域 ::
		const idMatch = name.match(/[A-Za-z_][A-Za-z0-9_$]*((?:<[^>]*>)?|(?:::[\w]+)*)/);
		if (idMatch && !STOP_WORDS.has(idMatch[0].toLowerCase())) {
			tags.push(idMatch[0]);
			tags.push(idMatch[0].toLowerCase());
		} else if (name && !STOP_WORDS.has(name.toLowerCase())) {
			tags.push(name);
			tags.push(name.toLowerCase());
		}
	}
	return tags;
}
