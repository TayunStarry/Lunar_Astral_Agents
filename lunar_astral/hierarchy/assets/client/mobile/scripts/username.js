/**
 * 后端用户名模块
 *
 * 从 lunar_config.json 的 server.user_name 读取用户名（QQ 适配器同款标注，
 * 仅用于发送给月华，本地展示不使用）；未定义或读取失败时默认「子幽」。
 * 与电脑端 preferences.js 的 loadBackendUserName 逻辑一致。
 */

let userName = '子幽';

/** 启动时加载一次后端用户名（异步，失败保持默认） */
export async function initUserName() {
    try {
        const res = await fetch('/file/read/lunar_config.json');
        if (!res.ok) return;
        const config = await res.json();
        const name = config && config.server ? config.server.user_name : null;
        if (typeof name === 'string' && name.trim()) {
            userName = name.trim();
        }
    } catch (e) {
        // 配置读取失败：保持默认用户名
    }
}

/** 构造发送给月华的用户名前缀，与 QQ 适配器的入站格式一致 */
export function getUserNamePrefix() {
    return `[用户: ${userName}]: `;
}
