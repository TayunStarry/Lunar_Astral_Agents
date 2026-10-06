/**
 * DOM 小工具：元素创建、事件绑定、提示条、文件下载/选择。
 * 只做最薄的一层封装，避免在业务模块里堆砌 document.createElement。
 */

/**
 * 创建元素。
 * @param {string} tag
 * @param {object} [props] class/text/html/title/dataset/style/attrs/on/type/value/placeholder...
 * @param {Array<Node|string>} [children]
 */
export function el(tag, props = {}, children = []) {
	const node = document.createElement(tag);
	applyProps(node, props);
	for (const child of children) {
		if (child == null) continue;
		node.append(child);
	}
	return node;
}

export function applyProps(node, props = {}) {
	for (const [key, value] of Object.entries(props)) {
		if (value == null) continue;
		switch (key) {
			case "class":
				node.className = value;
				break;
			case "text":
				node.textContent = String(value);
				break;
			case "html":
				node.innerHTML = value;
				break;
			case "dataset":
				for (const [k, v] of Object.entries(value)) node.dataset[k] = String(v);
				break;
			case "style":
				if (typeof value === "string") node.style.cssText = value;
				else for (const [k, v] of Object.entries(value)) node.style.setProperty(k, String(v));
				break;
			case "attrs":
				for (const [k, v] of Object.entries(value)) node.setAttribute(k, String(v));
				break;
			case "on":
				for (const [type, handler] of Object.entries(value)) node.addEventListener(type, handler);
				break;
			default:
				if (key in node) node[key] = value;
				else node.setAttribute(key, String(value));
		}
	}
	return node;
}

export function svgEl(tag, attrs = {}) {
	const node = document.createElementNS("http://www.w3.org/2000/svg", tag);
	for (const [k, v] of Object.entries(attrs)) {
		if (v == null) continue;
		node.setAttribute(k, String(v));
	}
	return node;
}

export function qs(selector, root = document) {
	return root.querySelector(selector);
}

export function clear(node) {
	while (node.firstChild) node.removeChild(node.firstChild);
	return node;
}

/** 事件委托：在容器上挂一个监听，按选择器分发 */
export function delegate(root, type, selector, handler, options) {
	const listener = (event) => {
		const target = event.target instanceof Element ? event.target.closest(selector) : null;
		if (!target || !root.contains(target)) return;
		handler(event, target);
	};
	root.addEventListener(type, listener, options);
	return () => root.removeEventListener(type, listener, options);
}

export function on(target, type, handler, options) {
	target.addEventListener(type, handler, options);
	return () => target.removeEventListener(type, handler, options);
}

// ---------------------------------------------------------------- 提示条

let toastHost = null;

export function initToasts(host) {
	toastHost = host;
}

/**
 * 右下角提示条。
 * @param {string} message
 * @param {"info"|"ok"|"warn"|"error"} [kind]
 */
export function toast(message, kind = "info") {
	if (!message) return;
	if (!toastHost) {
		console.log(`[toast:${kind}] ${message}`);
		return;
	}
	const icon = { info: "fa-circle-info", ok: "fa-circle-check", warn: "fa-triangle-exclamation", error: "fa-circle-xmark" }[kind] || "fa-circle-info";
	const node = el("div", { class: `toast toast-${kind}`, html: `<i class="fas ${icon}"></i><span></span>` });
	node.querySelector("span").textContent = message;
	toastHost.appendChild(node);
	requestAnimationFrame(() => node.classList.add("is-in"));
	const life = kind === "error" ? 5200 : 2800;
	setTimeout(() => {
		node.classList.remove("is-in");
		setTimeout(() => node.remove(), 320);
	}, life);
}

// ---------------------------------------------------------------- 文件

export function downloadBlob(filename, blob) {
	const url = URL.createObjectURL(blob);
	const a = el("a", { href: url, download: filename });
	document.body.appendChild(a);
	a.click();
	a.remove();
	setTimeout(() => URL.revokeObjectURL(url), 4000);
}

export function downloadText(filename, text, mime = "application/json") {
	downloadBlob(filename, new Blob([text], { type: `${mime};charset=utf-8` }));
}

/** 弹出文件选择框 */
export function pickFile(accept = ".json") {
	return new Promise((resolve) => {
		const input = el("input", { type: "file", accept, style: "display:none" });
		document.body.appendChild(input);
		input.addEventListener("change", () => {
			const file = input.files && input.files[0] ? input.files[0] : null;
			input.remove();
			resolve(file);
		});
		input.click();
	});
}

export function readFileText(file) {
	return new Promise((resolve, reject) => {
		const reader = new FileReader();
		reader.onload = () => resolve(String(reader.result || ""));
		reader.onerror = () => reject(reader.error || new Error("读取文件失败"));
		reader.readAsText(file, "utf-8");
	});
}

/** 简易输入弹窗（替代 window.prompt，样式统一且可取消） */
export function askText({ title = "请输入", value = "", placeholder = "", okText = "确定" } = {}) {
	return new Promise((resolve) => {
		const overlay = el("div", { class: "modal-mask" });
		const input = el("input", { class: "field-input", type: "text", value, placeholder });
		const ok = el("button", { class: "btn btn-primary", text: okText });
		const cancel = el("button", { class: "btn", text: "取消" });
		const box = el("div", { class: "modal" }, [
			el("div", { class: "modal-title", text: title }),
			el("div", { class: "modal-body" }, [input]),
			el("div", { class: "modal-foot" }, [cancel, ok]),
		]);
		overlay.appendChild(box);
		document.body.appendChild(overlay);
		const close = (result) => {
			overlay.remove();
			document.removeEventListener("keydown", onKey, true);
			resolve(result);
		};
		const onKey = (event) => {
			if (event.key === "Escape") {
				event.stopPropagation();
				close(null);
			} else if (event.key === "Enter") {
				event.stopPropagation();
				close(input.value);
			}
		};
		cancel.addEventListener("click", () => close(null));
		ok.addEventListener("click", () => close(input.value));
		overlay.addEventListener("pointerdown", (event) => {
			if (event.target === overlay) close(null);
		});
		document.addEventListener("keydown", onKey, true);
		input.focus();
		input.select();
	});
}

/** 简易确认弹窗 */
export function askConfirm({ title = "确认", message = "", okText = "确定", danger = false } = {}) {
	return new Promise((resolve) => {
		const overlay = el("div", { class: "modal-mask" });
		const ok = el("button", { class: `btn ${danger ? "btn-danger" : "btn-primary"}`, text: okText });
		const cancel = el("button", { class: "btn", text: "取消" });
		const box = el("div", { class: "modal" }, [
			el("div", { class: "modal-title", text: title }),
			el("div", { class: "modal-body", text: message }),
			el("div", { class: "modal-foot" }, [cancel, ok]),
		]);
		overlay.appendChild(box);
		document.body.appendChild(overlay);
		const close = (result) => {
			overlay.remove();
			document.removeEventListener("keydown", onKey, true);
			resolve(result);
		};
		const onKey = (event) => {
			if (event.key === "Escape") {
				event.stopPropagation();
				close(false);
			}
		};
		cancel.addEventListener("click", () => close(false));
		ok.addEventListener("click", () => close(true));
		overlay.addEventListener("pointerdown", (event) => {
			if (event.target === overlay) close(false);
		});
		document.addEventListener("keydown", onKey, true);
		ok.focus();
	});
}

let tooltipEl = null;
let tooltipTimer = 0;

/** 极简悬浮提示（用于画布元素的自定义提示） */
export function showTooltip(text, clientX, clientY) {
	if (!text) return;
	if (!tooltipEl) {
		tooltipEl = el("div", { class: "tooltip" });
		document.body.appendChild(tooltipEl);
	}
	tooltipEl.textContent = text;
	tooltipEl.classList.add("is-on");
	const pad = 14;
	tooltipEl.style.left = `${clientX + pad}px`;
	tooltipEl.style.top = `${clientY + pad}px`;
	clearTimeout(tooltipTimer);
}

export function hideTooltip() {
	if (!tooltipEl) return;
	clearTimeout(tooltipTimer);
	tooltipTimer = setTimeout(() => {
		if (tooltipEl) tooltipEl.classList.remove("is-on");
	}, 60);
}
