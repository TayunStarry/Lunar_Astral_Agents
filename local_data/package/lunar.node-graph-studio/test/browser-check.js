#!/usr/bin/env node
/**
 * 无头浏览器自检：用 CDP 驱动本机 Edge/Chrome 跑 test/dom.html，
 * 采集用例结果、控制台异常，并对 index.html 的真实观感截图。
 *
 * 一条命令搞定（会自己拉起预览服务器与浏览器）：
 *   node test/browser-check.js
 *
 * 常用参数：
 *   --port <n>      预览服务器端口（默认 8123）
 *   --cdp <n>       CDP 调试端口（默认 9333）
 *   --browser <p>   指定浏览器可执行文件
 *   --shot <path>   截图输出路径（默认 <临时目录>/node-graph-studio.png）
 *   --keep-open     跑完不关闭浏览器（便于人工接管）
 *   --timeout <ms>  DOM 用例总超时（默认 90000）
 *
 * 注意：本脚本只用于开发/回归；正式运行由琉璃的 file_manager 提供静态资源。
 */

import { spawn } from "node:child_process";
import fs from "node:fs";
import fsp from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const pkgRoot = path.resolve(here, "..");

// ---------------------------------------------------------------- 参数

const argv = process.argv.slice(2);
function argValue(name, fallback) {
	const index = argv.indexOf(name);
	return index >= 0 && argv[index + 1] ? argv[index + 1] : fallback;
}
const SERVE_PORT = Number(argValue("--port", 8123));
const CDP_PORT = Number(argValue("--cdp", 9333));
const TIMEOUT_MS = Number(argValue("--timeout", 90000));
const KEEP_OPEN = argv.includes("--keep-open");
const verbose = argv.includes("--verbose") || argv.includes("-v");
/** --eval "<js>" / --eval-file <path>：只打开 --url 页面执行一段脚本并打印 JSON 结果（诊断用） */
const EVAL_FILE = argValue("--eval-file", null);
const EVAL = EVAL_FILE ? fs.readFileSync(EVAL_FILE, "utf8") : argValue("--eval", null);
const EVAL_URL = argValue("--url", null);
const SHOT = argValue("--shot", path.join(os.tmpdir(), "node-graph-studio.png"));

const BROWSER_CANDIDATES = [
	argValue("--browser", ""),
	"C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe",
	"C:\\Program Files\\Microsoft\\Edge\\Application\\msedge.exe",
	"C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe",
	"C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe",
	"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	"/usr/bin/google-chrome",
	"/usr/bin/chromium",
	"/usr/bin/chromium-browser",
].filter(Boolean);

function findBrowser() {
	for (const candidate of BROWSER_CANDIDATES) {
		if (fs.existsSync(candidate)) return candidate;
	}
	return null;
}

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

async function waitFor(fn, { timeout = 20000, interval = 150, label = "条件" } = {}) {
	const started = Date.now();
	let lastError = null;
	while (Date.now() - started < timeout) {
		try {
			const value = await fn();
			if (value) return value;
		} catch (err) {
			lastError = err;
		}
		await sleep(interval);
	}
	throw new Error(`等待${label}超时${lastError ? `：${lastError.message}` : ""}`);
}

// ---------------------------------------------------------------- 极简 CDP 客户端

class CDP {
	constructor(socket) {
		this.socket = socket;
		this.nextId = 1;
		this.pending = new Map();
		this.listeners = new Map();
		this.closed = false;
		socket.addEventListener("message", (event) => this.onMessage(event.data));
		socket.addEventListener("close", () => {
			this.closed = true;
			for (const { reject } of this.pending.values()) reject(new Error("CDP 连接已关闭"));
			this.pending.clear();
		});
	}

	static async connect(wsUrl) {
		const socket = new WebSocket(wsUrl);
		await new Promise((resolve, reject) => {
			socket.addEventListener("open", () => resolve(), { once: true });
			socket.addEventListener("error", (event) => reject(new Error(`WebSocket 连接失败：${event.message || "未知错误"}`)), { once: true });
		});
		return new CDP(socket);
	}

	onMessage(raw) {
		let message;
		try {
			message = JSON.parse(raw);
		} catch (err) {
			return;
		}
		if (message.id && this.pending.has(message.id)) {
			const { resolve, reject } = this.pending.get(message.id);
			this.pending.delete(message.id);
			if (message.error) reject(new Error(`${message.error.message}${message.error.data ? ` (${message.error.data})` : ""}`));
			else resolve(message.result);
			return;
		}
		if (message.method) {
			const handlers = this.listeners.get(message.method) || [];
			for (const handler of handlers) {
				try {
					handler(message.params, message.sessionId);
				} catch (err) {
					console.error(`[cdp] ${message.method} 处理器异常:`, err);
				}
			}
		}
	}

	send(method, params = {}, sessionId) {
		if (this.closed) return Promise.reject(new Error("CDP 连接已关闭"));
		const id = this.nextId++;
		const payload = { id, method, params };
		if (sessionId) payload.sessionId = sessionId;
		this.socket.send(JSON.stringify(payload));
		return new Promise((resolve, reject) => {
			this.pending.set(id, { resolve, reject });
			setTimeout(() => {
				if (this.pending.has(id)) {
					this.pending.delete(id);
					reject(new Error(`CDP ${method} 超时`));
				}
			}, 30000);
		});
	}

	on(method, handler) {
		if (!this.listeners.has(method)) this.listeners.set(method, []);
		this.listeners.get(method).push(handler);
	}

	close() {
		try {
			this.socket.close();
		} catch (err) {
			/* 忽略 */
		}
	}
}

/** 在指定会话上求值并返回 JSON 结果 */
async function evaluate(cdp, sessionId, expression) {
	const result = await cdp.send("Runtime.evaluate", {
		expression,
		returnByValue: true,
		awaitPromise: true,
		allowUnsafeEvalBlockedByCSP: false,
	}, sessionId);
	if (result.exceptionDetails) {
		const text = result.exceptionDetails.exception?.description || result.exceptionDetails.text;
		throw new Error(`页面求值异常：${text}`);
	}
	return result.result ? result.result.value : undefined;
}

// ---------------------------------------------------------------- 主流程

async function main() {
	const browserPath = findBrowser();
	if (!browserPath) {
		console.error("未找到 Edge/Chrome，可用 --browser <可执行文件> 指定。");
		process.exit(2);
	}

	// 1) 预览服务器
	const serverLog = [];
	const server = spawn(process.execPath, [path.join(pkgRoot, "test", "serve.js"), "--port", String(SERVE_PORT), "--quiet"], {
		stdio: ["ignore", "pipe", "pipe"],
	});
	server.stdout.on("data", (chunk) => serverLog.push(String(chunk)));
	server.stderr.on("data", (chunk) => serverLog.push(String(chunk)));
	const baseUrl = `http://127.0.0.1:${SERVE_PORT}`;
	await waitFor(async () => {
		const resp = await fetch(`${baseUrl}/index.html`, { method: "HEAD" });
		return resp.ok;
	}, { label: "预览服务器就绪", timeout: 15000 });
	console.log(`预览服务器：${baseUrl}`);

	// 2) 浏览器
	const profile = path.join(os.tmpdir(), `ngs-browser-check-${Date.now()}`);
	const browser = spawn(browserPath, [
		"--headless=new",
		"--disable-gpu",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-extensions",
		"--disable-background-networking",
		"--hide-scrollbars",
		"--window-size=1680,1000",
		`--user-data-dir=${profile}`,
		`--remote-debugging-port=${CDP_PORT}`,
		"about:blank",
	], { stdio: "ignore" });

	const cleanup = () => {
		if (!KEEP_OPEN) {
			try {
				browser.kill();
			} catch (err) {
				/* 忽略 */
			}
		}
		try {
			server.kill();
		} catch (err) {
			/* 忽略 */
		}
	};
	process.on("exit", cleanup);
	process.on("SIGINT", () => {
		cleanup();
		process.exit(130);
	});

	const version = await waitFor(async () => {
		const resp = await fetch(`http://127.0.0.1:${CDP_PORT}/json/version`);
		if (!resp.ok) return null;
		const data = await resp.json();
		return data.webSocketDebuggerUrl ? data : null;
	}, { label: "浏览器 CDP 就绪", timeout: 30000 });
	console.log(`浏览器：${version.Browser}`);

	const cdp = await CDP.connect(version.webSocketDebuggerUrl);

	const consoleProblems = [];
	const attach = async (url) => {
		const { targetId } = await cdp.send("Target.createTarget", { url: "about:blank" });
		const { sessionId } = await cdp.send("Target.attachToTarget", { targetId, flatten: true });
		cdp.on("Runtime.exceptionThrown", (params, sid) => {
			if (sid !== sessionId) return;
			const details = params.exceptionDetails || {};
			reportProblem(`未捕获异常：${details.exception?.description || details.text}`);
		});
		cdp.on("Runtime.consoleAPICalled", (params, sid) => {
			if (sid !== sessionId) return;
			if (params.type !== "error" && params.type !== "warning") return;
			const text = (params.args || []).map((a) => a.value ?? a.description ?? a.type).join(" ");
			reportProblem(`console.${params.type}：${text}`);
		});
		cdp.on("Log.entryAdded", (params, sid) => {
			if (sid !== sessionId) return;
			if (params.entry?.level !== "error") return;
			reportProblem(`日志错误：${params.entry.text}`);
		});
		cdp.on("Network.loadingFailed", (params, sid) => {
			if (sid !== sessionId) return;
			reportProblem(`资源加载失败：${params.errorText}（${params.type}）`);
		});
		cdp.on("Network.responseReceived", (params, sid) => {
			if (sid !== sessionId) return;
			if (params.response.status >= 400) reportProblem(`HTTP ${params.response.status}：${params.response.url}`);
		});
		await cdp.send("Runtime.enable", {}, sessionId);
		await cdp.send("Log.enable", {}, sessionId);
		await cdp.send("Page.enable", {}, sessionId);
		await cdp.send("Network.enable", {}, sessionId);
		const loaded = new Promise((resolve) => {
			const handler = (params, sid) => {
				if (sid === sessionId && params.name === "load") resolve();
			};
			const existing = cdp.listeners.get("Page.loadEventFired") || [];
			existing.push(handler);
			cdp.listeners.set("Page.loadEventFired", existing);
		});
		await cdp.send("Page.navigate", { url }, sessionId);
		await Promise.race([loaded, sleep(20000)]);
		return { sessionId, targetId };
	};

	/** 页面问题实时打印，方便一眼定位「模块没跑起来」这类问题 */
	function reportProblem(message) {
		if (consoleProblems.includes(message)) return;
		consoleProblems.push(message);
		if (verbose) console.log(`  [页面] ${message}`);
	}

	// 3) 诊断模式：只求值一段脚本
	if (EVAL) {
		const target = EVAL_URL || `${baseUrl}/index.html`;
		const session = await attach(target);
		await waitFor(() => evaluate(cdp, session.sessionId, "document.readyState === 'complete'"), { label: "页面加载完成", timeout: 20000 });
		// 只有被测页面是应用本体时才等 NodeGraphStudio
		await waitFor(() => evaluate(cdp, session.sessionId, "location.pathname !== '/index.html' || !!window.NodeGraphStudio"), { label: "应用可用", timeout: 20000 });
		const value = await evaluate(cdp, session.sessionId, `(async () => { ${EVAL} })()`);
		console.log(typeof value === "string" ? value : JSON.stringify(value, null, 2));
		if (consoleProblems.length) {
			console.log("");
			console.log("页面问题：");
			for (const problem of consoleProblems) console.log(`  · ${problem}`);
		}
		cdp.close();
		cleanup();
		process.exit(0);
	}

	// 4) DOM 端到端用例
	console.log("运行 DOM 端到端用例…");
	const suiteSession = await attach(`${baseUrl}/test/dom.html`);
	const payload = await waitFor(async () => {
		const value = await evaluate(cdp, suiteSession.sessionId, "window.__DOM_RESULTS__ ? JSON.stringify({title: document.title, results: window.__DOM_RESULTS__.results.map(r => ({name: r.name, ok: r.ok, error: r.error ? String(r.error.message || r.error) : null})), consoleErrors: window.__DOM_RESULTS__.consoleErrors}) : null");
		return value || null;
	}, { label: "DOM 用例完成", timeout: TIMEOUT_MS, interval: 400 }).catch(async (err) => {
		const title = await evaluate(cdp, suiteSession.sessionId, "document.title").catch(() => "?");
		const text = await evaluate(cdp, suiteSession.sessionId, "document.getElementById('details') ? document.getElementById('details').textContent : ''").catch(() => "");
		return JSON.stringify({ title, results: [], consoleErrors: [String(err), text ? text.slice(0, 2000) : ""] });
	});

	const report = JSON.parse(payload);
	const failures = (report.results || []).filter((r) => !r.ok);

	// 4) 真实观感截图（独立页面，载入示例图并自适应视图）
	console.log("生成界面截图…");
	const shotSession = await attach(`${baseUrl}/index.html`);
	await waitFor(() => evaluate(cdp, shotSession.sessionId, "!!window.NodeGraphStudio"), { label: "应用初始化", timeout: 20000 });
	const probe = await evaluate(cdp, shotSession.sessionId, `(() => {
		const app = window.NodeGraphStudio;
		app.actions.loadSample();
		app.actions.fit();
		app.store.select({ nodes: app.store.graph.nodes.slice(1, 3).map(n => n.id) }, 'replace');
		app.renderer.renderGraph();
		app.minimap.draw();
		const nodes = [...document.querySelectorAll('.node')].map(n => {
			const r = n.getBoundingClientRect();
			return { title: n.querySelector('.node-title').textContent, ports: n.querySelectorAll('.port').length, x: Math.round(r.left), y: Math.round(r.top), w: Math.round(r.width), h: Math.round(r.height) };
		});
		const edges = [...document.querySelectorAll('.edge')].map(g => ({ style: g.dataset.style, line: !!g.querySelector('.edge-line').getAttribute('d'), arrow: g.querySelector('.edge-arrow-to').getAttribute('transform') }));
		return JSON.stringify({
			title: document.title,
			docName: document.querySelector('.doc-name') ? document.querySelector('.doc-name').value : null,
			toolbarButtons: document.querySelectorAll('#appBar .btn').length,
			nodeTypes: document.querySelectorAll('#paletteHost .type-item').length,
			nodes, edges,
			inspectorFields: document.querySelectorAll('#inspectorHost .field').length,
			status: document.querySelector('#statusBar .status-stats') ? document.querySelector('#statusBar .status-stats').textContent : null,
			zoom: app.store.viewport.zoom,
		});
	})()`);
	await sleep(600);

	const shot = await cdp.send("Page.captureScreenshot", { format: "png" }, shotSession.sessionId);
	await fsp.writeFile(SHOT, Buffer.from(shot.data, "base64"));

	// 5) 汇总
	console.log("");
	console.log("════════════════ DOM 端到端用例 ════════════════");
	for (const result of report.results || []) {
		console.log(`  ${result.ok ? "✓" : "✗"} ${result.name}${result.ok ? "" : `\n      ${result.error}`}`);
	}
	if (!report.results || report.results.length === 0) {
		console.log(`  （未取得用例结果：${report.title}）`);
	}
	console.log("");
	console.log(`合计 ${(report.results || []).length} 项，失败 ${failures.length}`);
	if (report.consoleErrors && report.consoleErrors.length) {
		console.log("页面错误：");
		for (const item of report.consoleErrors) console.log(`  · ${item}`);
	}
	console.log("");
	console.log("════════════════ 真实页面观感探针 ════════════════");
	console.log(probe);
	console.log("");
	console.log(`截图：${SHOT}`);
	console.log(`预览地址（浏览器仍开着时可访问）：${baseUrl}/index.html`);

	cdp.close();
	cleanup();
	process.exit(failures.length || (report.consoleErrors || []).length ? 1 : 0);
}

main().catch((err) => {
	console.error("浏览器自检失败：", err);
	process.exit(3);
});
