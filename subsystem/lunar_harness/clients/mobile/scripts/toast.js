/**
 * 通用悬浮消息组件 (Toast)
 *
 * 队列、生命周期与自动消失逻辑在 /shared/core/toast.js；
 * 本文件只提供移动端单例，视觉完全沿用 styles/toast.css。
 *
 * 消息类型：
 * - info     : 蓝色信息提示，3秒后自动消失
 * - success  : 绿色成功提示，3秒后自动消失
 * - warning  : 黄色警告提示，5秒后自动消失
 * - error    : 红色错误提示，5秒后自动消失
 * - voice    : 语音识别常驻状态，手动关闭或调用 dismissVoice()
 */

import { ToastController, TOAST_TYPES } from '/shared/core/toast.js';

export { ToastController as ToastManager, TOAST_TYPES };

/** 全局 Toast 单例 */
export const Toast = new ToastController();
