/**
 * 星月智能 · 桌面端共享层适配入口
 *
 * 桌面端仍是经典 script + 全局变量架构，这里以单个 ES Module 入口
 * 把共享模块挂到 window.LunarShared，供后续 defer 脚本按全局名调用。
 * 移动端不需要此文件，直接 import 共享模块。
 */
import * as constants from './core/constants.js';
import * as utils from './core/utils.js';
import * as files from './core/files.js';
import * as media from './core/media.js';
import * as api from './core/api.js';
import * as markdown from './core/markdown.js';
import * as renderer from './core/renderer.js';
import * as messageView from './core/message-view.js';
import { WebSocketClient } from './core/socket.js';
import { AudioQueueManager, createAudioQueue } from './core/audio-queue.js';
import { ToastController } from './core/toast.js';
import { UserName } from './core/username.js';
import { VoiceChatManager } from './core/voice-chat.js';

window.LunarShared = {
	constants,
	utils,
	files,
	media,
	api,
	markdown,
	renderer,
	messageView,
	WebSocketClient,
	AudioQueueManager,
	createAudioQueue,
	ToastController,
	UserName,
	VoiceChatManager
};

// 显式广播就绪信号：后续经典脚本不依赖「网络快就先生效」这类隐含时序，
// 由 js/shared-boot.js 负责等待 / 缺失时报错。
window.dispatchEvent(new CustomEvent('lunar-shared-ready'));
