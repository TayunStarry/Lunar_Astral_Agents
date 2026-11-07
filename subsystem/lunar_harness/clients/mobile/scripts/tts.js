/**
 * 移动端音频播放队列：共享队列 + 移动端音色增强链路。
 * 队列播空时通知连续语音模块恢复监听判定。
 */
import { AudioQueueManager } from '/shared/core/audio-queue.js';

export { AudioQueueManager };

export const AudioQueue = new AudioQueueManager({
	enhance: true,
	onIdle: () => {
		import('./voice.js')
			.then(({ VoiceChat }) => VoiceChat.onAudioPlaybackChange())
			.catch(() => { /* voice.js 可能未加载 */ });
	}
});
