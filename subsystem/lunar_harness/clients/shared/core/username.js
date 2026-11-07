/**
 * 星月智能 · 桌面端 / 移动端共享用户称呼
 *
 * 逻辑完全一致：有缓存直接用；缺失时显示覆盖层，写入 localStorage。
 * 覆盖层视觉由各端传入 styleText（保留桌面 / 移动各自的设计令牌）。
 */
import { DEFAULT_USER_NAME, USER_NAME_STORAGE_KEY } from './constants.js';

export class UserName {
	constructor(options = {}) {
		this.storageKey = options.storageKey || USER_NAME_STORAGE_KEY;
		this.fallback = options.fallback || DEFAULT_USER_NAME;
		this.styleText = options.styleText || '';
		this.name = this.read() || this.fallback;
	}

	read() {
		try {
			const saved = localStorage.getItem(this.storageKey);
			return saved && saved.trim() ? saved.trim() : '';
		} catch {
			return '';
		}
	}

	write(name) {
		try {
			localStorage.setItem(this.storageKey, name);
		} catch {
			// 隐私模式等场景下写入失败：本次会话仍可使用
		}
	}

	injectStyle() {
		if (!this.styleText || document.getElementById('lunarUserNameStyle')) return;
		const style = document.createElement('style');
		style.id = 'lunarUserNameStyle';
		style.textContent = this.styleText;
		document.head.appendChild(style);
	}

	/** 启动时确保称呼可用；返回最终称呼 */
	init() {
		const cached = this.read();
		if (cached) {
			this.name = cached;
			return Promise.resolve(this.name);
		}
		this.injectStyle();
		return new Promise(resolve => {
			const overlay = document.createElement('div');
			overlay.className = 'lunar-name-overlay';
			overlay.innerHTML = `
				<div class="lunar-name-card" role="dialog" aria-modal="true" aria-labelledby="lunarNameTitle">
					<h2 id="lunarNameTitle">希望月华如何称呼您？</h2>
					<p>这个名字会用于对话与消息气泡，并只保存在当前浏览器。</p>
					<input class="lunar-name-input" id="lunarNameInput" type="text" maxlength="24" placeholder="请输入您的称呼" autocomplete="nickname">
					<div class="lunar-name-actions">
						<button class="lunar-name-confirm" id="lunarNameConfirm" type="button" disabled>确认</button>
					</div>
				</div>
			`;
			document.body.appendChild(overlay);

			const input = overlay.querySelector('#lunarNameInput');
			const button = overlay.querySelector('#lunarNameConfirm');
			const submit = () => {
				const name = input.value.trim();
				if (!name) {
					input.focus();
					return;
				}
				this.write(name);
				this.name = name;
				overlay.remove();
				resolve(name);
			};
			input.addEventListener('input', () => {
				button.disabled = !input.value.trim();
			});
			input.addEventListener('keydown', event => {
				if (event.key === 'Enter') {
					event.preventDefault();
					submit();
				}
			});
			button.addEventListener('click', submit);
			requestAnimationFrame(() => input.focus());
		});
	}

	/** 当前称呼 */
	get() {
		return this.name;
	}

	/** 发送给月华的用户名前缀（与 QQ 适配器入站格式一致） */
	prefix() {
		return `[用户: ${this.name}]: `;
	}
}
