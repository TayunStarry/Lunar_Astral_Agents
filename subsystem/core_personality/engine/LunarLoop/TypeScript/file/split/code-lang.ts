/** 代码语言定义与扩展名识别 */

/** 代码文件路径前缀（与 split.ts 的散文/MD 前缀保持一致，便于统一解析） */
export const CODE_PATH_PREFIX = '*标题> ';

/** 生成某段代码的路径前缀：`*标题> 路径*\n` */
export function codeHeader(path: string): string {
	return `${CODE_PATH_PREFIX}${path}*\n`;
}

/** 支持的代码语言类型（映射到统一小写扩展名） */
export type CodeLang = "ts" | "js" | "tsx" | "jsx" | "go" | "py" | "java" | "c" | "h" | "cpp" | "cxx" | "hpp" | "cs";

/** 语言是否为花括号风格（true=用 {} 界定作用域；false=用缩进，如 Python） */
export const BRACE_LANGS: Record<string, true> = {
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
