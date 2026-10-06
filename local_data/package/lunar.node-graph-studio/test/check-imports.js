/**
 * 静态导入图检查（Node 专用，由 test/run.js 在跑用例前先执行）。
 *
 * 解决的问题很具体：`node --check` 只验语法，不验 import 路径；
 * 而 js/ 顶层的模块（如 interaction.js）改成 "../core/x.js" 就会指到包外面去，
 * 浏览器里表现为一整片 404、应用完全不启动。这里逐文件解析相对 import 并核对文件是否存在，
 * 让这类错误在提交前就被拦住。
 */

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const pkgRoot = path.resolve(here, "..");

const IMPORT_RE = /(?:^|\n)\s*(?:import|export)[^'"\n]*?from\s*['"]([^'"]+)['"]/g;
const DYNAMIC_RE = /\bimport\s*\(\s*['"]([^'"]+)['"]\s*\)/g;

function listJsFiles(dir, out = []) {
	for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
		if (entry.name === "node_modules" || entry.name.startsWith(".")) continue;
		const full = path.join(dir, entry.name);
		if (entry.isDirectory()) listJsFiles(full, out);
		else if (entry.name.endsWith(".js")) out.push(full);
	}
	return out;
}

/**
 * 去掉块注释（含 JSDoc）。
 * JSDoc 里的 `import("../x.js").Type` 只是类型标注，不是运行时导入，
 * 若一并检查会把「文档路径写错」误报成「应用会 404」。
 * （本项目不含 `/*` 字样的正则字面量，因此整体剥离块注释是安全的。）
 */
function stripBlockComments(source) {
	return source.replace(/\/\*[\s\S]*?\*\//g, "");
}

/**
 * @returns {{checked:number, problems:Array<{file:string, specifier:string, reason:string}>}}
 */
export function checkStaticImports() {
	const files = listJsFiles(pkgRoot);
	const problems = [];
	let checked = 0;

	for (const file of files) {
		const source = stripBlockComments(fs.readFileSync(file, "utf8"));
		const specifiers = new Set();
		for (const match of source.matchAll(IMPORT_RE)) specifiers.add(match[1]);
		for (const match of source.matchAll(DYNAMIC_RE)) specifiers.add(match[1]);

		for (const specifier of specifiers) {
			if (!specifier.startsWith(".")) continue; // 裸模块名（node: 内置等）跳过
			checked += 1;
			const resolved = path.resolve(path.dirname(file), specifier);
			if (!fs.existsSync(resolved)) {
				problems.push({
					file: path.relative(pkgRoot, file),
					specifier,
					reason: `解析到包外或不存在：${path.relative(pkgRoot, resolved)}`,
				});
				continue;
			}
			// 目标必须仍在包目录内
			const relative = path.relative(pkgRoot, resolved);
			if (relative.startsWith("..")) {
				problems.push({
					file: path.relative(pkgRoot, file),
					specifier,
					reason: "指向包目录之外（浏览器会 404）",
				});
			}
		}
	}

	return { checked, problems };
}
