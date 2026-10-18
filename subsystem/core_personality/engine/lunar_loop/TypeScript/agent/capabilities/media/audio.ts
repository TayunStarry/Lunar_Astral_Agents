import { GlobalConfig } from '../../../config/global';
import { getPromptFromKnowledge, savePromptToKnowledge } from '../../../file/io/knowledge';
import { admitMedia, existingSummary, mediaName, storeMediaSummary } from './admission';

/** 音频转写失败时的兜底置换文本 */
export const AUDIO_FALLBACK_TEXT = '月华听不懂这段语音呢';

/**
 * 理解音频（两段式：ASR 转写 → 返回转写文本）
 *
 * 返回文本含【语音转写】前缀；转换/转写失败时抛出异常。
 * URL 形式的音频按地址缓存（data URI 为一次性录音/上传，跳过缓存）。
 */
export async function understandAudio(audioSource: string): Promise<string> {
	// 缓存检查
	const cacheable = !audioSource.startsWith('data:');
	const cachedPrompt = cacheable ? getPromptFromKnowledge(audioSource) : '';
	if (cachedPrompt) {
		console.log('[感知者] 命中音频缓存，直接返回');
		return cachedPrompt;
	}
	// 第一步：将音频本地化并转换为 16kHz 单声道 WAV
	console.log('[感知者] 开始转换音频...');
	const [wavBase64, audioError] = audioWav(audioSource);
	if (!wavBase64 || audioError) {
		console.error('[感知者] 音频转换失败:', audioError);
		throw new Error('音频转换失败');
	}
	// 第二步：调用语音识别模型转写
	console.log('[感知者] 开始语音转写...');
	const transcript = await transcribeAudio(wavBase64);
	if (!transcript.trim()) {
		console.error('[感知者] 语音转写为空');
		throw new Error('语音转写为空');
	}
	console.log(`[感知者] 语音转写完成: ${transcript.substring(0, 50)}`);
	// 第三步：转写文本直接作为音频内容的完整表达，并写入缓存
	const text = `【语音转写】${transcript}`;
	if (cacheable) {
		savePromptToKnowledge(audioSource, text);
		console.log('[感知者] 语音转写文本已缓存');
	}
	return text;
}

/**
 * 调用语音识别模型（system-asr）转写 16kHz WAV 音频
 *
 * @param wavBase64 16kHz 单声道 WAV 音频的 base64 编码
 * @returns 转写文本（已剥离 ASR 输出的 "language xxx<asr_text>" 前缀）
 */
async function transcribeAudio(wavBase64: string): Promise<string> {
	const [result, error] = syncFetch({
		url: GlobalConfig.MultimodalUrl + '/chat/completions',
		execute: {
			method: 'POST',
			crossDomain: true,
			headers: {
				Authorization: `Bearer ${encodeURIComponent(GlobalConfig.MultimodalKey)}`,
				'Content-Type': 'application/json',
			},
			body: JSON.stringify({
				model: GlobalConfig.AsrName,
				messages: [{
					role: 'user',
					content: [{ type: 'input_audio', input_audio: { data: wavBase64, format: 'wav' } }]
				}],
				temperature: 0,
			}),
		},
	});
	if (error) {
		console.error('[感知者] 语音转写请求失败:', error.message);
		return '';
	}
	/** ASR 原始输出（形如 "language Chinese<asr_text>转写内容"） */
	const raw = result?.body?.choices?.[0]?.message?.content || '';
	// 剥离语言标记前缀，仅保留转写正文
	const marker = '<asr_text>';
	return raw.includes(marker) ? raw.substring(raw.indexOf(marker) + marker.length).trim() : raw.trim();
}

/**
 * 音频资源化：转写 → 登记 → 写回结论 → 产出带句柄文本
 *
 * @param originalSource 原始音频来源
 * @param name 建议文件名
 * @param origin 来源语义
 * @returns 形如 `@A.002【语音转写】…` 的文本；失败返回空串由调用方兜底
 */
export async function resourceAudio(
	originalSource: string,
	name: string,
	origin: 'client-upload' | 'channel'
): Promise<string> {
	const media = admitMedia(originalSource, name, origin, 'A');
	if (!media) return '';
	const cached = existingSummary(media);
	if (cached) return `${media.handle}${cached}`;
	try {
		const transcript = await understandAudio(originalSource);
		storeMediaSummary(media, transcript);
		return `${media.handle}${transcript}`;
	}
	catch (error) {
		console.error('[感知者] 音频理解失败（句柄已保留）:', error);
		return `${media.handle}${AUDIO_FALLBACK_TEXT}`;
	}
}

/** 转写音频并返回转写文本；失败时返回兜底提示（调用方原位置换） */
export async function understandAudioOrFallback(audioSource: string): Promise<string> {
	try {
		return await understandAudio(audioSource);
	}
	catch (error) {
		console.error('[感知者] 音频理解失败，以兜底文本置换:', error);
		return AUDIO_FALLBACK_TEXT;
	}
}

/**
 * 音频资源化包装：收进资源目录后返回「句柄 + 转写文本」
 *
 * @param audioSource 音频来源
 * @returns 置换用文本
 */
export async function resourceAudioOrFallback(audioSource: string): Promise<string> {
	try {
		const text = await resourceAudio(audioSource, mediaName(audioSource, '.wav'), audioSource.startsWith('data:') ? 'client-upload' : 'channel');
		if (text) return text;
	}
	catch (error) {
		console.error('[感知者] 音频资源化失败，退化为纯转写文本:', error);
	}
	return understandAudioOrFallback(audioSource);
}
