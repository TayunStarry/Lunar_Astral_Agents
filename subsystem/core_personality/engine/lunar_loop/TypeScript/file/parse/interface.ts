// ============================================================
//  文本解析入口 — 聚合提取/清洗/断句模块，保持公共 API 不变
// ============================================================

export { cleanTextForTTS, removeEmojiSymbols, splitSentences } from './parse-text';
import { extractThinkingBlocks, extractCodeBlocks, extractEmotionBlocks, extractActionBlocks } from './parse-blocks';
import { cleanTextForTTS, removeEmojiSymbols, splitSentences } from './parse-text';

/** 正文切片数据结构 */
export interface TextChunk {
	/** 用于前端显示的文本 */
	display: string;
	/** 用于语音合成的文本 */
	tts: string;
}

/** 解析后的文本数据结构 */
export interface ParsedContent {
	/** 思考区内容（不参与显示与语音合成） */
	thinkingBlocks: string[];
	/** 代码块内容（不参与显示与语音合成） */
	codeBlocks: string[];
	/** 行动区内容（不参与显示与语音合成） */
	actionBlocks: string[];
	/** 清洗并切片后的正文内容，每个切片包含display和tts两个版本 */
	textChunks: TextChunk[];
}

/** 解析文本内容 */
export function parseContent(rawText: string): ParsedContent {
	// 如果原始文本为空，直接返回空对象
	if (!rawText) return { thinkingBlocks: [], codeBlocks: [], actionBlocks: [], textChunks: [] };
	/** 提取思考区内容（不参与显示与语音合成） */
	const [thinkingBlocks, textAfterThinking] = extractThinkingBlocks(rawText);
	/** 提取代码块内容（不参与显示与语音合成） */
	const [codeBlocks, textAfterCode] = extractCodeBlocks(textAfterThinking);
	/** 先提取情感区内容（emoji表情与颜文字），避免颜文字的括号被动作区误吞（不参与显示与语音合成） */
	const [emotionBlocks, textAfterEmotion] = extractEmotionBlocks(textAfterCode);
	/** 再提取动作区内容（全角/半角括号包裹）（不参与显示与语音合成） */
	const [actionZoneBlocks, textAfterAction] = extractActionBlocks(textAfterEmotion);
	/** 合并动作区与情感区为统一的行动分区（不参与显示与语音合成） */
	const actionBlocks = [...actionZoneBlocks, ...emotionBlocks];
	/** 清洗用于显示的文本（兜底移除残留emoji） */
	const displayText = removeEmojiSymbols(textAfterAction);
	/** 对清洗后的文本执行智能切片 */
	const displayChunks = splitSentences(displayText);
	/** 为每个切片生成显示文本和TTS文本 */
	const textChunks: TextChunk[] = displayChunks.map(chunk => ({ display: chunk, tts: cleanTextForTTS(chunk), }));
	// 返回解析结果
	return { thinkingBlocks, codeBlocks, actionBlocks, textChunks };
}
