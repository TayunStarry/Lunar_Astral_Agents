/**
 * 用户名模块
 *
 * 首次访问/缓存缺失时显示称呼覆盖层，输入后缓存到 localStorage；
 * 刷新时直接复用缓存，不再从 lunar_config.json 读取。
 * 与电脑端 preferences.js 使用同一个存储键，电脑端与手机端共用称呼。
 */

const USER_NAME_STORAGE_KEY = 'lunar_user_name';

let userName = '子幽';

function readCachedUserName() {
	try {
		const saved = localStorage.getItem(USER_NAME_STORAGE_KEY);
		return saved && saved.trim() ? saved.trim() : '';
	} catch (e) {
		return '';
	}
}

function writeCachedUserName(name) {
	try {
		localStorage.setItem(USER_NAME_STORAGE_KEY, name);
	} catch (e) {
		// 隐私模式等场景下写入失败：本次会话仍可使用
	}
}

function injectUserNameOverlayStyle() {
	if (document.getElementById('lunarUserNameStyle')) return;
	const style = document.createElement('style');
	style.id = 'lunarUserNameStyle';
	style.textContent = `
		.lunar-name-overlay { position: fixed; inset: 0; z-index: 100000; display: flex; align-items: center; justify-content: center; padding: 20px; background: rgba(10, 14, 24, 0.42); backdrop-filter: blur(8px); -webkit-backdrop-filter: blur(8px); }
		.lunar-name-card { width: min(92vw, 420px); padding: 28px 24px; border-radius: 20px; background: var(--glass-bg, rgba(255, 255, 255, 0.9)); border: 1px solid var(--glass-border, rgba(255, 255, 255, 0.45)); box-shadow: var(--shadow, 0 8px 32px rgba(0, 0, 0, 0.18)); color: var(--text-0f1e0f, #0f1e0f); font-family: inherit; }
		.lunar-name-card h2 { margin: 0 0 10px; font-size: 1.2rem; font-weight: 600; }
		.lunar-name-card p { margin: 0 0 18px; color: var(--text-secondary, #6b7280); font-size: 0.9rem; line-height: 1.5; }
		.lunar-name-input { width: 100%; box-sizing: border-box; padding: 12px 14px; border-radius: 12px; border: 1px solid var(--border-e1e6f0, rgba(0, 0, 0, 0.12)); background: rgba(255, 255, 255, 0.72); color: inherit; font-size: 1rem; outline: none; }
		.lunar-name-input:focus { border-color: var(--primary-color, #6c9bcf); box-shadow: 0 0 0 3px rgba(108, 155, 207, 0.18); }
		.lunar-name-actions { display: flex; justify-content: flex-end; margin-top: 18px; }
		.lunar-name-confirm { padding: 10px 22px; border: none; border-radius: 12px; background: var(--primary-color, #6c9bcf); color: #fff; font-size: 0.95rem; cursor: pointer; }
		.lunar-name-confirm:disabled { opacity: 0.5; cursor: not-allowed; }
	`;
	document.head.appendChild(style);
}

/** 启动时确保称呼可用：有缓存直接用，没有则显示覆盖层等待输入 */
export function initUserName() {
	const cached = readCachedUserName();
	if (cached) {
		userName = cached;
		return Promise.resolve();
	}
	injectUserNameOverlayStyle();
	return new Promise((resolve) => {
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
			writeCachedUserName(name);
			userName = name;
			overlay.remove();
			resolve();
		};
		input.addEventListener('input', () => {
			button.disabled = !input.value.trim();
		});
		input.addEventListener('keydown', (event) => {
			if (event.key === 'Enter') {
				event.preventDefault();
				submit();
			}
		});
		button.addEventListener('click', submit);
		requestAnimationFrame(() => input.focus());
	});
}

/** 构造发送给月华的用户名前缀，与 QQ 适配器的入站格式一致 */
export function getUserNamePrefix() {
	return `[用户: ${userName}]: `;
}
