/**
 * 持久化：双通道保存，二者互不阻塞。
 *
 *   1. 浏览器 localStorage —— 自动保存（防丢），键含文档槽位，刷新/关闭包再打开都能恢复；
 *   2. 琉璃本地磁盘     —— 显式「保存到本地」写到 local_data/database/node_graph_studio/，
 *                          可列出、可打开、可删除，跨浏览器与跨机器都在。
 *
 * 磁盘通道失败（后端未启动、端点不可用）只提示不阻塞，界面继续可用。
 */

import { createDocument, sanitizeDocument } from "../model/document.js";
import * as files from "./files.js";

const STORAGE_PREFIX = "node-graph-studio:";
const AUTOSAVE_KEY = `${STORAGE_PREFIX}autosave`;
const SERVER_DIR = "database/node_graph_studio";

export class Persistence {
	/**
	 * @param {import("../app/store.js").Store} store
	 * @param {(message:string, kind?:string)=>void} notify
	 */
	constructor(store, notify = () => {}) {
		this.store = store;
		this.notify = notify;
		this.serverAvailable = null;
		this.autosaveTimer = 0;
		this.lastSavedAt = 0;
	}

	// ------------------------------------------------------------ 本地草稿

	/** 自动保存（节流）：任何改动后 1.2s 落盘到 localStorage */
	scheduleAutosave() {
		if (!this.store.settings.autosave) return;
		if (this.autosaveTimer) return;
		this.autosaveTimer = window.setTimeout(() => {
			this.autosaveTimer = 0;
			this.saveLocal();
		}, 1200);
	}

	saveLocal() {
		try {
			const doc = this.store.snapshotDocument();
			window.localStorage.setItem(AUTOSAVE_KEY, JSON.stringify(doc));
			this.lastSavedAt = Date.now();
			this.store.markDirty(false);
			return true;
		} catch (err) {
			// 超配额或隐私模式：只提示一次
			this.notify("本地草稿保存失败（浏览器存储不可用）", "warn");
			return false;
		}
	}

	loadLocal() {
		try {
			const raw = window.localStorage.getItem(AUTOSAVE_KEY);
			if (!raw) return null;
			return sanitizeDocument(JSON.parse(raw));
		} catch (err) {
			return null;
		}
	}

	clearLocal() {
		try {
			window.localStorage.removeItem(AUTOSAVE_KEY);
		} catch (err) {
			/* 忽略 */
		}
	}

	hasLocal() {
		try {
			return !!window.localStorage.getItem(AUTOSAVE_KEY);
		} catch (err) {
			return false;
		}
	}

	// ------------------------------------------------------------ 磁盘

	async probe() {
		if (this.serverAvailable !== null) return this.serverAvailable;
		this.serverAvailable = await files.probeBackend();
		return this.serverAvailable;
	}

	dir() {
		return SERVER_DIR;
	}

	/** 保存到 local_data/database/node_graph_studio/<name>.json */
	async saveToServer(name) {
		const doc = this.store.snapshotDocument();
		const fileName = sanitizeFileName(name || doc.name || "节点图");
		doc.name = fileName;
		this.store.graph.name = fileName;
		const path = `${SERVER_DIR}/${fileName}.json`;
		const result = await files.writeJSON(path, doc);
		if (!result.ok) {
			this.serverAvailable = false;
			this.notify(`保存到本地失败：${result.error}`, "error");
			return null;
		}
		this.serverAvailable = true;
		this.store.markDirty(false);
		this.store.emit("document");
		this.notify(`已保存到 local_data/${path}`, "ok");
		return path;
	}

	/** 列出磁盘上已保存的节点图 */
	async listServer() {
		const result = await files.listDir(SERVER_DIR);
		if (!result.ok) {
			this.serverAvailable = false;
			return null;
		}
		this.serverAvailable = true;
		return result.data
			.filter((item) => !item.isDir && /\.json$/i.test(item.name || ""))
			.map((item) => item.name)
			.sort((a, b) => a.localeCompare(b, "zh-Hans-CN"));
	}

	async loadFromServer(fileName) {
		const result = await files.readJSON(`${SERVER_DIR}/${fileName}`);
		if (!result.ok) {
			this.notify(`读取失败：${result.error}`, "error");
			return null;
		}
		return sanitizeDocument(result.data);
	}

	async deleteFromServer(fileName) {
		const result = await files.deleteFile(`${SERVER_DIR}/${fileName}`);
		if (!result.ok) {
			this.notify(`删除失败：${result.error}`, "error");
			return false;
		}
		return true;
	}

	/** 首次进入时的默认文档：有草稿就恢复，没有就给一张带示例的空白图 */
	initialDocument(sampleFactory) {
		const local = this.loadLocal();
		if (local && local.nodes.length > 0) return { doc: local, restored: true };
		return { doc: createDocument(sampleFactory ? sampleFactory() : {}), restored: false };
	}
}

export function sanitizeFileName(name) {
	const cleaned = String(name || "")
		.replace(/[\\/:*?"<>|]+/g, "-")
		.replace(/\s+/g, " ")
		.trim();
	return cleaned.slice(0, 80) || "节点图";
}
