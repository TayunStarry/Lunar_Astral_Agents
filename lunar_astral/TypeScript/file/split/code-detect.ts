import type { CodeLang } from './code-lang';

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
export function detectPython(line: string): { kind: any; name: string } | null {
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
export function detectDeclaration(line: string, lang: CodeLang): { kind: any; name: string } | null {
	switch (lang) {
		case "ts": case "js": case "tsx": case "jsx": return detectTsJs(line);
		case "go": return detectGo(line);
		case "py": return detectPython(line);
		case "java": case "cs": return detectBraceClassLang(line);
		case "c": case "h": case "cpp": case "cxx": case "hpp": return detectCpp(line);
		default: return null;
	}
}
