/**
 * 移动端视图切换器：双击输入框在「模型区 / 历史记录」之间切换
 *
 * 判定规则：
 * - 两次有效点击间隔 ≤ 300ms 才算双击；超过则重新计时
 * - 单次按压位移 > 10px（滚动/拖拽）或时长 ≥ 400ms（长按）不计入点击
 * - 第二次按下即拦截默认行为，避免输入框聚焦与软键盘弹出；切换后再 blur 兜底
 * - 鼠标双击与触屏双击共用同一出口，并以 400ms 去抖避免合成事件重复触发
 */

const DOUBLE_TAP_WINDOW = 300;
const MOVE_TOLERANCE = 10;
const LONG_PRESS = 400;
const TOGGLE_DEBOUNCE = 400;
const HINT_KEY = 'lunar.mobile.historyHint';

/**
 * @param {object} options
 * @param {HTMLElement} options.panel 主面板（承载 history-mode 类）
 * @param {HTMLTextAreaElement} options.input 输入框
 * @param {Function} [options.onShow] 切到历史视图后回调（用于按需加载数据）
 * @param {Function} [options.onHide] 切回模型视图后回调
 */
export function createViewSwitcher({ panel, input, onShow, onHide }) {
	let historyMode = false;
	let lastTapAt = 0;
	let pendingSecond = false;
	let pressAt = 0;
	let startX = 0;
	let startY = 0;
	let moved = false;
	let lastToggleAt = 0;

	function applyMode(next) {
		if (historyMode === next) return;
		historyMode = next;
		panel.classList.toggle('history-mode', historyMode);
		panel.dataset.view = historyMode ? 'history' : 'stage';
		hideHistoryHint();
		// 双击不应留下输入焦点与软键盘
		input.blur();
		if (historyMode) onShow?.();
		else onHide?.();
	}

	function toggle() {
		const now = Date.now();
		// 触屏双击后浏览器可能补发 dblclick，这里统一去抖
		if (now - lastToggleAt < TOGGLE_DEBOUNCE) return;
		lastToggleAt = now;
		applyMode(!historyMode);
	}

	if (input) {
		input.addEventListener('touchstart', (event) => {
			const touch = event.touches[0];
			if (!touch || event.touches.length > 1) {
				pendingSecond = false;
				lastTapAt = 0;
				return;
			}
			const now = Date.now();
			pressAt = now;
			startX = touch.clientX;
			startY = touch.clientY;
			moved = false;
			// 距上次点击在窗口内：按第二次点击处理，立即拦截聚焦与软键盘
			if (now - lastTapAt <= DOUBLE_TAP_WINDOW) {
				event.preventDefault();
				pendingSecond = true;
			} else {
				pendingSecond = false;
			}
		}, { passive: false });

		input.addEventListener('touchmove', (event) => {
			const touch = event.touches[0];
			if (!touch) return;
			if (Math.abs(touch.clientX - startX) > MOVE_TOLERANCE
				|| Math.abs(touch.clientY - startY) > MOVE_TOLERANCE) {
				moved = true;
				pendingSecond = false;
			}
		}, { passive: true });

		input.addEventListener('touchend', () => {
			const now = Date.now();
			const duration = now - pressAt;
			const isTap = !moved && duration < LONG_PRESS;
			if (pendingSecond && isTap) {
				pendingSecond = false;
				lastTapAt = 0;
				toggle();
				return;
			}
			pendingSecond = false;
			lastTapAt = isTap ? now : 0;
		});

		input.addEventListener('touchcancel', () => {
			pendingSecond = false;
			moved = true;
			lastTapAt = 0;
		});

		input.addEventListener('dblclick', (event) => {
			event.preventDefault();
			toggle();
		});
	}

	return {
		isHistoryMode: () => historyMode,
		show: () => applyMode(true),
		hide: () => applyMode(false)
	};
}

/** 首次进入时展示一次性的双击提示（localStorage 记忆，不新增常驻入口） */
export function maybeShowHistoryHint(hintEl) {
	if (!hintEl) return;
	let seen = false;
	try {
		seen = localStorage.getItem(HINT_KEY) === '1';
	} catch (error) {
		// 本地存储不可用时不打扰用户
		seen = true;
	}
	if (seen) return;
	try {
		localStorage.setItem(HINT_KEY, '1');
	} catch (error) {
		// 忽略写入失败：最多下次再提示一次
	}
	setTimeout(() => hintEl.classList.add('visible'), 1400);
	setTimeout(() => hintEl.classList.remove('visible'), 6200);
}

/** 进入历史视图时立即收起提示 */
export function hideHistoryHint() {
	const hint = document.getElementById('historyHint');
	if (hint) hint.classList.remove('visible');
}
