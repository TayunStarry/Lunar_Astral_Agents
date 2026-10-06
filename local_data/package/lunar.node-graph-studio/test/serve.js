#!/usr/bin/env node
/**
 * 开发预览服务器（仅用于本地联调 / 自动化验证）。
 *
 * 正式运行时页面由琉璃的 file_manager 通过 /file/read/package/<目录>/ 提供，
 * 本脚本只是把这套端点在本机复刻一份，方便：
 *   · 直接用浏览器打开 node test/serve.js 后打印的地址做手工验收；
 *   · 用无头浏览器跑 test/dom.html 做 DOM 级回归。
 *
 * 用法：
 *   node test/serve.js [--port 8099] [--quiet]
 *
 * 端点：
 *   GET    /                     → 包目录静态文件（index.html、css/、js/…）
 *   GET    /file/read/<path>     → 先查预览写入区，再回落到真实 local_data（图标库等）
 *   POST   /file/write           → 写进系统临时目录（不污染仓库）
 *   POST   /file/list/<dir>      → 列出预览写入区里的目录
 *   DELETE /file/delete/<path>   → 删除预览写入区里的文件
 */

import http from "node:http";
import fs from "node:fs";
import fsp from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const pkgRoot = path.resolve(here, "..");
// 仓库里的 local_data 根目录（用于把 /file/read/... 映射到真实资源）
const localDataRoot = path.resolve(pkgRoot, "..", "..");
const storeRoot = path.join(os.tmpdir(), "node-graph-studio-preview");

const args = process.argv.slice(2);
const portArgIndex = args.indexOf("--port");
const PORT = Number(portArgIndex >= 0 ? args[portArgIndex + 1] : process.env.PORT || 8099);
const quiet = args.includes("--quiet");

const MIME = {
	".html": "text/html; charset=utf-8",
	".htm": "text/html; charset=utf-8",
	".css": "text/css; charset=utf-8",
	".js": "application/javascript; charset=utf-8",
	".mjs": "application/javascript; charset=utf-8",
	".json": "application/json; charset=utf-8",
	".svg": "image/svg+xml",
	".png": "image/png",
	".jpg": "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
	".gif": "image/gif",
	".ico": "image/x-icon",
	".woff": "font/woff",
	".woff2": "font/woff2",
	".ttf": "font/ttf",
	".txt": "text/plain; charset=utf-8",
	".md": "text/markdown; charset=utf-8",
	".wasm": "application/wasm",
};

function mimeOf(file) {
	return MIME[path.extname(file).toLowerCase()] || "application/octet-stream";
}

/** 防目录穿越：确保解析结果仍在允许的根目录下 */
function safeJoin(root, relative) {
	const target = path.resolve(root, "." + path.sep + decodeURIComponent(relative));
	const normalizedRoot = path.resolve(root);
	if (target !== normalizedRoot && !target.startsWith(normalizedRoot + path.sep)) return null;
	return target;
}

async function sendFile(res, file, fallbackStatus = 404) {
	try {
		const stat = await fsp.stat(file);
		if (stat.isDirectory()) {
			const index = path.join(file, "index.html");
			return sendFile(res, index, fallbackStatus);
		}
		res.writeHead(200, {
			"Content-Type": mimeOf(file),
			"Content-Length": stat.size,
			"Cache-Control": "no-store",
		});
		fs.createReadStream(file).pipe(res);
		return true;
	} catch (err) {
		res.writeHead(fallbackStatus, { "Content-Type": "text/plain; charset=utf-8" });
		res.end(`${fallbackStatus === 404 ? "未找到" : "错误"}: ${path.relative(process.cwd(), file)}`);
		return false;
	}
}

async function readBody(req) {
	const chunks = [];
	for await (const chunk of req) chunks.push(chunk);
	return Buffer.concat(chunks);
}

function decodeFileName(header) {
	if (!header) return null;
	try {
		const name = Buffer.from(String(header), "base64").toString("utf8");
		return name || null;
	} catch (err) {
		return null;
	}
}

const server = http.createServer(async (req, res) => {
	const parsed = new URL(req.url, `http://127.0.0.1:${PORT}`);
	const pathname = parsed.pathname;

	try {
		// ---- 读文件（先预览写入区，再真实 local_data） ----
		if (pathname.startsWith("/file/read/")) {
			const relative = pathname.slice("/file/read/".length);
			const stored = safeJoin(storeRoot, relative);
			if (stored && fs.existsSync(stored)) return void (await sendFile(res, stored));
			const real = safeJoin(localDataRoot, relative);
			if (real && fs.existsSync(real)) return void (await sendFile(res, real));
			res.writeHead(404, { "Content-Type": "text/plain; charset=utf-8" });
			res.end("未找到");
			return;
		}

		// ---- 写文件 ----
		if (pathname === "/file/write" && req.method === "POST") {
			const name = decodeFileName(req.headers["x-file-name"]);
			if (!name) {
				res.writeHead(400, { "Content-Type": "application/json" });
				res.end(JSON.stringify({ error: "缺少文件名" }));
				return;
			}
			const target = safeJoin(storeRoot, name);
			if (!target) {
				res.writeHead(403, { "Content-Type": "application/json" });
				res.end(JSON.stringify({ error: "无效文件名" }));
				return;
			}
			await fsp.mkdir(path.dirname(target), { recursive: true });
			await fsp.writeFile(target, await readBody(req));
			res.writeHead(200, { "Content-Type": "application/json" });
			res.end(JSON.stringify({ filename: name, path: target, overwrite: "true" }));
			return;
		}

		// ---- 列目录 ----
		if (pathname.startsWith("/file/list/") && req.method === "POST") {
			const relative = pathname.slice("/file/list/".length);
			const dir = safeJoin(storeRoot, relative);
			let items = [];
			try {
				const entries = await fsp.readdir(dir, { withFileTypes: true });
				items = entries.map((e) => ({ name: e.name, isDir: e.isDirectory() }));
			} catch (err) {
				items = [];
			}
			res.writeHead(200, { "Content-Type": "application/json" });
			res.end(JSON.stringify(items));
			return;
		}

		// ---- 删文件 ----
		if (pathname.startsWith("/file/delete/") && req.method === "DELETE") {
			const relative = pathname.slice("/file/delete/".length);
			const target = safeJoin(storeRoot, relative);
			if (target) await fsp.rm(target, { force: true });
			res.writeHead(200, { "Content-Type": "application/json" });
			res.end(JSON.stringify({ ok: true }));
			return;
		}

		// ---- 其它：包目录静态文件 ----
		const relative = pathname === "/" ? "index.html" : pathname.slice(1);
		const file = safeJoin(pkgRoot, relative);
		if (!file) {
			res.writeHead(403, { "Content-Type": "text/plain; charset=utf-8" });
			res.end("拒绝访问");
			return;
		}
		await sendFile(res, file);
	} catch (err) {
		res.writeHead(500, { "Content-Type": "text/plain; charset=utf-8" });
		res.end(`服务器错误: ${err.message}`);
	}
});

server.listen(PORT, "127.0.0.1", () => {
	if (!quiet) {
		console.log(`[节点图工坊] 预览地址 http://127.0.0.1:${PORT}/index.html`);
		console.log(`[节点图工坊] DOM 自检 http://127.0.0.1:${PORT}/test/dom.html`);
		console.log(`[节点图工坊] 逻辑自检 http://127.0.0.1:${PORT}/test/index.html`);
		console.log(`[节点图工坊] 预览写入区 ${storeRoot}`);
	}
});
