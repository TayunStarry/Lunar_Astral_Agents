#!/usr/bin/env node
/**
 * Node 侧测试入口：node test/run.js
 *
 * 覆盖全部纯逻辑模块（几何 / 端口规则 / 连线路由方向 / 图模型 / 文档清洗 /
 * 历史 / 吸附 / 布局 / 文字 / 命令层），不需要浏览器环境。
 */

import { runSuites } from "./harness.js";
import { ALL_SUITES } from "./suites.js";
import { checkStaticImports } from "./check-imports.js";

const verbose = process.argv.includes("--verbose") || process.argv.includes("-v");

// 先做静态导入图检查：import 路径写错在浏览器里是整片 404，必须拦在最前面
const imports = checkStaticImports();
console.log(`静态导入检查：${imports.checked} 条相对导入`);
if (imports.problems.length) {
	console.log("");
	for (const problem of imports.problems) {
		console.log(`  ✗ ${problem.file}  →  ${problem.specifier}（${problem.reason}）`);
	}
	console.log("");
	console.log(`静态导入检查失败：${imports.problems.length} 条路径不可解析`);
	process.exit(1);
}
console.log("静态导入检查：全部可解析 ✓");
console.log("");

const { passed, failed, total, results, failures } = runSuites(ALL_SUITES, {
	onResult: (record) => {
		if (verbose || !record.ok) {
			const mark = record.ok ? "PASS" : "FAIL";
			console.log(`  [${mark}] ${record.suite} › ${record.name} (${record.duration.toFixed(1)}ms)`);
			if (!record.ok) console.log(`         ${record.error.message}`);
		}
	},
});

let currentSuite = "";
for (const record of results) {
	if (record.suite === currentSuite) continue;
	currentSuite = record.suite;
}
console.log("");
for (const suite of ALL_SUITES) {
	const own = results.filter((r) => r.suite === suite.name);
	const ok = own.filter((r) => r.ok).length;
	console.log(`  ${ok === own.length ? "✓" : "✗"} ${suite.name}  ${ok}/${own.length}`);
}
console.log("");
if (failures.length) {
	console.log("失败用例：");
	for (const failure of failures) {
		console.log(`  ✗ ${failure.suite} › ${failure.name}`);
		console.log(`    ${failure.error.stack ? failure.error.stack.split("\n").slice(0, 3).join("\n    ") : failure.error.message}`);
	}
	console.log("");
}
console.log(`合计 ${total} 项：通过 ${passed}，失败 ${failed}`);
process.exit(failed > 0 ? 1 : 0);
