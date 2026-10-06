/**
 * 浏览器侧测试渲染：把与 Node 相同的用例跑一遍并画到页面上。
 * 在琉璃里打开 包目录/test/index.html 即可自查，不依赖任何构建。
 */

import { runSuites } from "./harness.js";
import { ALL_SUITES } from "./suites.js";

const summary = document.getElementById("summary");
const list = document.getElementById("list");
const failedOnly = document.getElementById("failedOnly");

let last = null;

function render() {
	const filter = failedOnly.checked;
	list.textContent = "";
	let currentSuite = "";
	for (const record of last.results) {
		if (filter && record.ok) continue;
		if (record.suite !== currentSuite) {
			currentSuite = record.suite;
			list.appendChild(node("div", "suite", currentSuite));
		}
		const row = node("div", `case ${record.ok ? "ok" : "bad"}`);
		row.appendChild(node("span", "mark", record.ok ? "✓" : "✗"));
		row.appendChild(node("span", "name", record.name));
		row.appendChild(node("span", "ms", `${record.duration.toFixed(1)}ms`));
		list.appendChild(row);
		if (!record.ok) {
			list.appendChild(node("pre", "err", record.error.stack || record.error.message));
		}
	}
	summary.textContent = `合计 ${last.total} 项 · 通过 ${last.passed} · 失败 ${last.failed}`;
	summary.className = last.failed ? "summary bad" : "summary ok";
}

function node(tag, className, text) {
	const el = document.createElement(tag);
	if (className) el.className = className;
	if (text != null) el.textContent = text;
	return el;
}

document.getElementById("rerun").addEventListener("click", run);
failedOnly.addEventListener("change", () => {
	if (last) render();
});

function run() {
	last = runSuites(ALL_SUITES);
	render();
}

run();
