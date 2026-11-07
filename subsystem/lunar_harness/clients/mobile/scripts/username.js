/**
 * 移动端用户称呼：覆盖层逻辑来自共享层，视觉沿用移动端设计令牌。
 */
import { UserName } from '/shared/core/username.js';

const MOBILE_NAME_STYLE = `
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

const userName = new UserName({ styleText: MOBILE_NAME_STYLE });

/** 启动时确保称呼可用：有缓存直接用，没有则显示覆盖层等待输入 */
export function initUserName() {
	return userName.init();
}

/** 构造发送给月华的用户名前缀 */
export function getUserNamePrefix() {
	return userName.prefix();
}
