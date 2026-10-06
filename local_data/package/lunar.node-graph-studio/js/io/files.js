/**
 * 琉璃本地文件 API 封装。
 *
 * 端点（见 docs/code-wiki/09-LTPX协议-月华工具包.md 与 03 章）：
 *   GET    /file/read/<path>       读文件
 *   POST   /file/write             写文件（X-File-Name: base64(相对路径)，X-Overwrite: true）
 *   POST   /file/list/<dir>        列目录
 *   DELETE /file/delete/<path>     删文件
 *
 * 全部返回 {ok, data?, error?}，永不抛出——调用方按 ok 分支处理，
 * 保证「后端不在/端点不可用」时应用仍可纯前端工作（localStorage + 下载导出）。
 */

const JSON_HEADERS = { "Content-Type": "application/json" };

/** 与琉璃前端一致的 base64 文件名编码（UTF-8 安全） */
export function encodeFileName(name) {
	try {
		return btoa(unescape(encodeURIComponent(name)));
	} catch (err) {
		return "";
	}
}

export async function readText(path) {
	try {
		const resp = await fetch(`/file/read/${encodePath(path)}`, { cache: "no-store" });
		if (!resp.ok) return { ok: false, error: `HTTP ${resp.status}` };
		return { ok: true, data: await resp.text() };
	} catch (err) {
		return { ok: false, error: err.message || String(err) };
	}
}

export async function readJSON(path) {
	const result = await readText(path);
	if (!result.ok) return result;
	try {
		return { ok: true, data: JSON.parse(result.data) };
	} catch (err) {
		return { ok: false, error: "JSON 解析失败" };
	}
}

export async function writeText(path, text, mime = "application/json") {
	try {
		const resp = await fetch("/file/write", {
			method: "POST",
			headers: { ...JSON_HEADERS, "X-File-Name": encodeFileName(path), "X-Overwrite": "true" },
			body: new Blob([text], { type: `${mime};charset=utf-8` }),
		});
		if (!resp.ok) return { ok: false, error: `HTTP ${resp.status}` };
		let data = null;
		try {
			data = await resp.json();
		} catch (err) {
			data = null;
		}
		return { ok: true, data };
	} catch (err) {
		return { ok: false, error: err.message || String(err) };
	}
}

export async function writeJSON(path, value) {
	return writeText(path, JSON.stringify(value, null, "\t"));
}

export async function listDir(dir) {
	try {
		const resp = await fetch(`/file/list/${encodePath(dir)}`, { method: "POST" });
		if (!resp.ok) return { ok: false, error: `HTTP ${resp.status}` };
		const data = await resp.json();
		return { ok: true, data: Array.isArray(data) ? data : [] };
	} catch (err) {
		return { ok: false, error: err.message || String(err) };
	}
}

export async function deleteFile(path) {
	try {
		const resp = await fetch(`/file/delete/${encodePath(path)}`, { method: "DELETE" });
		if (!resp.ok) return { ok: false, error: `HTTP ${resp.status}` };
		return { ok: true };
	} catch (err) {
		return { ok: false, error: err.message || String(err) };
	}
}

/** 逐段编码路径，保留分隔符 */
export function encodePath(path) {
	return String(path).split("/").map((seg) => encodeURIComponent(seg)).join("/");
}

/** 后端是否可用（用于决定是否显示「保存到本地」相关界面） */
export async function probeBackend() {
	const result = await listDir("package");
	return result.ok;
}
