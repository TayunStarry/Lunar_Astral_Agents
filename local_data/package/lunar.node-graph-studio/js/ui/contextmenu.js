/**
 * 弹出菜单：顶栏下拉与画布右键菜单共用一套。
 *
 * 只负责「显示一组条目 → 回调命令」，条目的内容由调用方（main.js / toolbar.js）决定，
 * 因此画布菜单能直接复用命令层的全部操作。
 */

import { el } from "./dom.js";

let activeMenu = null;
let detach = null;

/**
 * 在屏幕坐标处弹出一组菜单项。
 * @param {Array<{label?:string, icon?:string, cmd?:string, onSelect?:Function, disabled?:boolean, danger?:boolean, separator?:boolean, hint?:string}>} items
 * @param {number} clientX
 * @param {number} clientY
 */
export function showMenu(items, clientX, clientY) {
	closeMenu();
	const menu = el("div", { class: "context-menu" });
	for (const item of items) {
		if (item.separator) {
			menu.appendChild(el("div", { class: "menu-sep" }));
			continue;
		}
		const row = el("button", {
			class: `menu-item${item.danger ? " menu-item-danger" : ""}${item.disabled ? " is-disabled" : ""}`,
		});
		row.disabled = !!item.disabled;
		row.appendChild(el("i", { class: `fas ${item.icon || "fa-angle-right"}` }));
		row.appendChild(el("span", { class: "menu-label", text: item.label || "" }));
		if (item.hint) row.appendChild(el("kbd", { class: "menu-hint", text: item.hint }));
		row.addEventListener("click", (event) => {
			event.stopPropagation();
			closeMenu();
			if (!item.disabled) item.onSelect?.(item.cmd);
		});
		menu.appendChild(row);
	}
	document.body.appendChild(menu);

	// 贴边修正：先渲染再量尺寸
	const rect = menu.getBoundingClientRect();
	const pad = 8;
	let left = clientX;
	let top = clientY;
	if (left + rect.width + pad > window.innerWidth) left = Math.max(pad, window.innerWidth - rect.width - pad);
	if (top + rect.height + pad > window.innerHeight) top = Math.max(pad, window.innerHeight - rect.height - pad);
	menu.style.left = `${Math.round(left)}px`;
	menu.style.top = `${Math.round(top)}px`;
	activeMenu = menu;

	const onPointerDown = (event) => {
		if (activeMenu && !activeMenu.contains(event.target)) closeMenu();
	};
	const onKey = (event) => {
		if (event.key === "Escape") closeMenu();
	};
	const onScroll = () => closeMenu();
	// 延后一帧再挂监听，避免触发本次右键/点击的同一个事件把我们关掉
	setTimeout(() => {
		document.addEventListener("pointerdown", onPointerDown, true);
		document.addEventListener("keydown", onKey, true);
		window.addEventListener("scroll", onScroll, true);
	}, 0);
	detach = () => {
		document.removeEventListener("pointerdown", onPointerDown, true);
		document.removeEventListener("keydown", onKey, true);
		window.removeEventListener("scroll", onScroll, true);
	};

	return menu;
}

export function closeMenu() {
	if (detach) {
		detach();
		detach = null;
	}
	if (activeMenu) {
		activeMenu.remove();
		activeMenu = null;
	}
}

export function isMenuOpen() {
	return !!activeMenu;
}
