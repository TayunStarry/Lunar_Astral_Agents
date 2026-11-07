/**
 * 移动端连续语音对话：VAD / WAV 编码 / 节流来自共享层，
 * 这里只注入移动端的发送逻辑（QQ 适配器标准消息格式）。
 */
import { VoiceChatManager } from '/shared/core/voice-chat.js';
import { audioEnvelope } from '/shared/core/media.js';
import { AudioQueue } from './tts.js';
import { sendMessages } from './fetch.js';
import { getUserNamePrefix } from './username.js';

export { VoiceChatManager };

export const VoiceChat = new VoiceChatManager({
	audioQueue: AudioQueue,
	onUtterance: async ({ file, base64Data }) => {
		await sendMessages([{
			role: 'user',
			content: [
				{ type: 'text', text: getUserNamePrefix() },
				audioEnvelope(file, base64Data, 'wav')
			]
		}]);
	}
});
