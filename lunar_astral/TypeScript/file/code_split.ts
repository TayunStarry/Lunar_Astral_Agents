/** 代码感知拆分器 */

/** 代码文件路径前缀（与 split.ts 的散文/MD 前缀保持一致，便于统一解析） */
const CODE_PATH_PREFIX = '*标题> ';

/** 生成某段代码的路径前缀：`*标题> 路径*\n` */
function codeHeader(path: string): string {
	return `${CODE_PATH_PREFIX}${path}*\n`;
}

/** 支持的代码语言类型（映射到统一小写扩展名） */
export type CodeLang = "ts" | "js" | "tsx" | "jsx" | "go" | "py" | "java" | "c" | "h" | "cpp" | "cxx" | "hpp" | "cs";

/** 语言是否为花括号风格（true=用 {} 界定作用域；false=用缩进，如 Python） */
const BRACE_LANGS: Record<string, true> = {
	ts: true, js: true, tsx: true, jsx: true, go: true,
	java: true, c: true, h: true, cpp: true, cxx: true, hpp: true, cs: true,
};

/** 语言识别：根据扩展名归一化为 CodeLang；不是代码则返回 null */
export function resolveCodeLang(fileType: string): CodeLang | null {
	const t = (fileType || "").toLowerCase().replace(/^\./, "");
	if (BRACE_LANGS[t]) return t as CodeLang;
	if (t === "python") return "py";
	return null;
}

/** 检测到的命名作用域（class / func / method） */
type Scope = {
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

type Section = { level: number; title: string; content: string; path: string };

/* ------------------------------------------------------------------
 * 过滤字符串与注释，便于可靠统计花括号
 * ------------------------------------------------------------------ */
function stripStringsAndComments(line: string, lang: CodeLang): string {
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
function braceDelta(line: string, lang: CodeLang): number {
	const clean = stripStringsAndComments(line, lang);
	let d = 0;
	for (const ch of clean) {
		if (ch === "{") d++;
		else if (ch === "}") d--;
	}
	return d;
}

/** 计算行首缩进数量（制表符按 4 空格计） */
function indentLevel(line: string): number {
	const m = /^[ \t]*/.exec(line);
	if (!m) return 0;
	return (m[0].match(/\t/g)?.length ?? 0) * 4 + m[0].length;
}

/** 是否为空白/纯注释/纯装饰行（Python 缩进跳过的辅助） */
function isInsignificant(line: string, lang: CodeLang): boolean {
	if (!line.trim()) return true;
	if (lang === "py" && /^\s*#/.test(line)) return true;
	if (/^\s*(?:\/\/|\/\*|\*\/)/.test(line)) return true;
	return false;
}

/* ------------------------------------------------------------------
 * 各语言声明检测：返回 {kind, name} 或 null
 * ------------------------------------------------------------------ */

/** TS/JS/TSX/JSX */
function detectTsJs(line: string): { kind: any; name: string } | null {
	const t = line.trim();
	// class / interface / type / enum / namespace
	let m = /^(?:export\s+)?(?:abstract\s+|default\s+)?(?:class|interface|type|enum|namespace)\s+(\w+)/.exec(t);
	if (m) return { kind: "class", name: m[1] };
	// 具名函数 function foo(
	m = /^(?:export\s+)?(?:async\s+)?function\s*(\w+)\s*\(/.exec(t);
	if (m) return { kind: "func", name: m[1] };
	// const x = (...) => {  或  const x = function (
	m = /^(?:export\s+)?const\s+(\w+)\s*=\s*(?:async\s+)?(?:\([^)]*\)|[\w]+)\s*=>/.exec(t);
	if (m) return { kind: "func", name: m[1] };
	// 类内方法：访问修饰符/async + 标识符( + 非流控；作为 method
	m = /^(?:(?:public|private|protected|static|async|readonly|abstract)\s+)*(\w+)\s*\([^)]*\)\s*[:{]/.exec(t);
	if (m && !/^(if|for|while|switch|catch|function|return|new|typeof|instanceof|delete|void)\b/.test(t)) {
		return { kind: "method", name: m[1] };
	}
	return null;
}

/** Go：包级函数、方法（接收者）、类型声明 */
function detectGo(line: string): { kind: any; name: string } | null {
	const t = line.trim();
	// func
	let m = /^func\s+(?:\([^)]*\)\s+)?(\w+)\s*\(/.exec(t);
	if (m) return { kind: "func", name: m[1] };
	// type X struct/interface (分号可忽略，单行匹配)
	m = /^type\s+(\w+)\s+(?:struct|interface)/.exec(t);
	if (m) return { kind: "class", name: m[1] };
	// type X = ...  单行别名不独立成块，忽略
	return null;
}

/** Python：def / class，含缩进方法 */
function detectPython(line: string): { kind: any; name: string } | null {
	const t = line.trim();
	if (t.startsWith("#")) return null;
	let m = /^def\s+(\w+)\s*\(/.exec(t);
	if (m) return { kind: "func", name: m[1] };
	m = /^class\s+(\w+)/.exec(t);
	if (m) return { kind: "class", name: m[1] };
	return null;
}

/** Java / C#（类与方法共用庆祝） */
function detectBraceClassLang(line: string): { kind: any; name: string } | null {
	const t = line.trim();
	// class / interface / enum / struct / record
	let m = /^(?:(?:public|private|protected|static|final|abstract|sealed|internal|readonly)\s+)*(?:class|interface|enum|@\w+\s+)?(?:class|interface|enum|struct|record)\s+(\w+)/.exec(t);
	if (m) return { kind: "class", name: m[1] };
	// 方法：访问修饰符/类型 + 标识符(... type ...) {
	m = /^(?:(?:public|private|protected|static|final|synchronized|native|abstract|virtual|override|async|internal)\s+)*[\w<>\[\],?.\s]+\s+(\w+)\s*\([^)]*\)\s*\{/.exec(t);
	if (m && !/^(if|for|while|switch|catch|return|new|else|do|try|synchronized)\b/.test(t)) {
		return { kind: "method", name: m[1] };
	}
	return null;
}

/** C / C++：使用 :: 的方法、函数、class / struct */
function detectCpp(line: string): { kind: any; name: string } | null {
	const t = line.trim();
	// class / struct / namespace / 前向
	let m = /^(?:template\s*<[^>]*>\s*)?(?:class|struct|union|namespace)\s+(\w+)/.exec(t);
	if (m) return { kind: "class", name: m[1] };
	// 方法：返回类型 + Class::Method(
	m = /^[\w:*&<>{},\s]+\s+(\w+)::(\w+)\s*\(/.exec(t);
	if (m) return { kind: "method", name: `${m[1]}::${m[2]}` };
	// 顶层函数
	m = /^(?:static\s+|inline\s+|extern\s+|virtual\s+|const\s+|unsigned\s+|signed\s+)*[\w:*&<>{},\s]+\s+(\w+)\s*\([^)]*\)\s*(?:const\s*)?\{/.exec(t);
	if (m && !/^(if|for|while|switch|catch|return|new|sizeof|else|do|try)\b/.test(t)) {
		return { kind: "func", name: m[1] };
	}
	return null;
}

/** 按语言分发声明检测 */
function detectDeclaration(line: string, lang: CodeLang): { kind: any; name: string } | null {
	switch (lang) {
		case "ts": case "js": case "tsx": case "jsx": return detectTsJs(line);
		case "go": return detectGo(line);
		case "py": return detectPython(line);
		case "java": case "cs": return detectBraceClassLang(line);
		case "c": case "h": case "cpp": case "cxx": case "hpp": return detectCpp(line);
		default: return null;
	}
}

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