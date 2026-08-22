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
	/** 动作区内容（全角/半角括号包裹，不参与显示与语音合成） */
	actionBlocks: string[];
	/** 情感区内容（emoji表情与颜文字，不参与显示与语音合成） */
	emotionBlocks: string[];
	/** 清洗并切片后的正文内容，每个切片包含display和tts两个版本 */
	textChunks: TextChunk[];
}

/** emoji 表情正则（覆盖常见 emoji 区段，含变体选择符与 ZWJ） */
const EMOJI_REGEX = /[\u{1F600}-\u{1F64F}\u{1F300}-\u{1F5FF}\u{1F680}-\u{1F6FF}\u{1F900}-\u{1F9FF}\u{1FA70}-\u{1FAFF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}\u{FE00}-\u{FE0F}\u{1F1E0}-\u{1F1FF}\u{200D}\u{20E3}\u{FE0F}]/gu;

/** 颜文字特征字符（眼/嘴/泪滴/手等高频符号，用于区分颜文字与普通括号动作） */
const KAOMOJI_MARKS = '_^･・。.、；;～~ノﾉゞヾω￣▽△□○●☆★°´｀♪♫＞＜><｡一≧≦∇∀ﾟ⌒⌣◕';
/**
 * 颜文字正则：
 * - 括号包裹型：(^_^)、(>_<)、(T_T)、(・ω・)、（￣▽￣）、<(￣︶￣)> 等，
 *   内层须含颜文字特征字符且不含中文句子，避免误吞 (挥手) 等动作区
 * - 裸脸型：>_<、^_^、T_T、o_o 等三段式眼-口-眼
 */
const KAOMOJI_REGEX = new RegExp(
	`[<＜＼]?[（(](?:[^\\s（()）\u4e00-\u9fff]|\u4e00){0,5}[${KAOMOJI_MARKS}](?:[^\\s（()）\u4e00-\u9fff]|\u4e00){0,5}[）)](?:[<＜>＞／\u30ce\u309e\u266a\u266b]*)?` +
	`|[>＜^TtOo0vV][_\\-=^><。.・oO][<＞^TtOo0vV]`,
	'gu'
);
/** 情感区正则（emoji 表情 + 颜文字） */
const EMOTION_REGEX = new RegExp(`${EMOJI_REGEX.source}|${KAOMOJI_REGEX.source}`, 'gu');

/** 从原始文本中提取思考区内容 */
function extractThinkingBlocks(text: string): [string[], string] {
	/** 思考区内容数组 */
	const blocks: string[] = [];
	/** 思考区正则表达式 */
	const regex = /<think>([\s\S]*?)<\/think>/gi;
	/** 匹配结果 */
	let match: RegExpExecArray | null;
	/** 从文本中提取思考区内容 */
	while ((match = regex.exec(text)) !== null) {
		/** 提取思考区内容 */
		const content = match[1].trim();
		// 如果思考区内容不为空,则添加到结果数组中
		if (content.length > 0) blocks.push(content);
	}
	/** 从原文中移除思考区 */
	const remaining = text.replace(/<think>[\s\S]*?<\/think>/gi, '');
	// 返回提取的思考区内容数组和移除思考区后的文本
	return [blocks, remaining];
}

/**
 * 从原始文本中提取代码块内容
 *
 * 使用单一正则匹配所有代码块（带或不带语言标识），避免重复提取
 * 保留完整的代码块标记（```...```），确保客户端能正确渲染
 *
 * @param text - 原始文本
 * @returns [提取的代码块数组（含```标记）, 移除代码块后的文本]
 */
function extractCodeBlocks(text: string): [string[], string] {
	/** 代码块内容数组 */
	const blocks: string[] = [];
	/** 代码块正则表达式 */
	const codeBlockRegex = /```[a-zA-Z0-9+#-]*[\s\S]*?```/g;
	/** 匹配结果 */
	let match: RegExpExecArray | null;
	// 遍历所有匹配到的代码块
	while ((match = codeBlockRegex.exec(text)) !== null) {
		// 保留完整匹配（含```标记），客户端需要标记来正确渲染代码块
		blocks.push(match[0]);
	}
	/** 从原文中移除所有代码块 */
	const remaining = text.replace(/```[a-zA-Z0-9+#-]*[\s\S]*?```/g, '');
	// 返回提取的代码块内容数组和移除代码块后的文本
	return [blocks, remaining];
}

/** 从原始文本中提取动作区内容 */
function extractActionBlocks(text: string): [string[], string] {
	/** 动作区内容数组 */
	const blocks: string[] = [];
	/** 开括号位置栈（全角/半角混用同一栈，支持混合配对） */
	const stack: number[] = [];
	/** 已闭合的最外层括号区间 [start, end)（end 为闭括号索引+1） */
	const ranges: Array<[number, number]> = [];
	// 遍历文本中的每个字符
	for (let i = 0; i < text.length; i++) {
		const ch = text[i];
		// 开括号入栈
		if (ch === '(' || ch === '\uFF08') stack.push(i);
		// 闭括号出栈
		else if (ch === ')' || ch === '\uFF09') {
			// 多余的闭括号，忽略
			if (stack.length === 0) continue;
			/** 开始括号位置（不包含括号） */
			const start = stack.pop()!;
			// 忽略未闭合的括号对
			if (stack.length !== 0) continue;
			/** 提取动作区内容（不包含括号，仅包含内层动作括号） */
			const content = text.slice(start + 1, i).trim();
			// 如果动作区内容不为空,则添加到结果数组中
			if (content.length > 0) blocks.push(content);
			// 记录当前括号对的区间
			ranges.push([start, i + 1]);
		}
	}
	/** 根据区间构建 remaining：移除所有最外层括号片段，保留其余原文 */
	let remaining = '';
	/** 上一次区间结束位置，用于拼接剩余文本 */
	let lastEnd = 0;
	// 遍历所有记录的括号对区间
	for (const [start, end] of ranges) {
		// 合并当前括号对之间的文本（不包含括号）
		remaining += text.slice(lastEnd, start);
		// 更新上一次匹配结束位置
		lastEnd = end;
	}
	// 合并剩余文本（不包含括号）
	remaining += text.slice(lastEnd);
	// 返回提取的动作区内容数组和移除最外层括号片段后的文本
	return [blocks, remaining];
}

/**
 * 从原始文本中提取情感区内容（emoji 表情与颜文字）
 *
 * 连续的 emoji/颜文字字符会被合并为一条情感记录
 * 同时从原始文本中移除这些情感内容
 *
 * @param text - 原始文本（应已移除思考区、代码块）
 * @returns [提取的情感区内容数组, 移除情感内容后的文本]
 */
function extractEmotionBlocks(text: string): [string[], string] {
	const blocks: string[] = [];
	const regex = new RegExp(EMOTION_REGEX.source, 'gu');
	let match: RegExpExecArray | null;
	/** 上一次匹配结束位置，用于判断是否连续 */
	let lastEnd = -1;
	/** 当前累积的连续情感字符串 */
	let current = '';
	while ((match = regex.exec(text)) !== null) {
		// 若本次匹配紧跟上次匹配（中间无其他字符），合并为同一条情感记录
		if (current.length > 0 && match.index === lastEnd) {
			current += match[0];
		} else {
			// 不连续，先存入已累积的记录
			if (current.length > 0) blocks.push(current);
			current = match[0];
		}
		lastEnd = match.index + match[0].length;
	}
	// 存入最后一条累积记录
	if (current.length > 0) blocks.push(current);
	const remaining = text.replace(regex, '');
	return [blocks, remaining];
}

/** 清洗文本，去除Markdown标记、行内代码、HTML标签、表情符号等不适合语音合成的内容 */
export function cleanTextForTTS(text: string): string {
	if (!text) return '';
	let processed = text;
	// 移除行内代码
	processed = processed.replace(/`[^`]*`/g, '');
	// 移除图片标记 ![alt](url)
	processed = processed.replace(/!\[.*?\]\(.*?\)/g, '');
	// 移除链接标记 [text](url)，保留链接文字
	processed = processed.replace(/\[([^\]]*)\]\(.*?\)/g, '$1');
	// 移除HTML标签
	processed = processed.replace(/<[^>]*>/g, '');
	// 移除emoji表情符号
	processed = processed.replace(/[\u{1F600}-\u{1F64F}\u{1F300}-\u{1F5FF}\u{1F680}-\u{1F6FF}\u{1F900}-\u{1F9FF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}\u{FE00}-\u{FE0F}\u{1F1E0}-\u{1F1FF}\u{200D}\u{20E3}\u{FE0F}]/gu, '');
	// 移除星号（Markdown加粗/斜体标记）
	processed = processed.replace(/\*/g, '');
	// 将换行符替换为空格
	processed = processed.replace(/\r?\n/g, ' ');
	// 移除中文括号内的内容（通常是注释或补充说明）
	processed = processed.replace(/\（[^）]*\）/g, '');
	// 移除英文括号内的内容
	processed = processed.replace(/\([^)]*\)/g, '');
	/** 白名单过滤：仅保留中文、英文、数字、常用中英文标点（其余字符替换为逗号） */
	const allowed = '\\u4e00-\\u9fff' + 'a-zA-Z0-9' + '\\s_~\\-' + '\uFF0C\u3002\uFF1F\uFF1A\uFF01\uFF1B\u3001\u2014\u2026\u300A\u300B\u3008\u3009\u201C\u201D\u2018\u2019\uFF08\uFF09\u3010\u3011' + ',.\'\"?:!;()\\[\\]';
	/** 白名单正则表达式 */
	const whitelist = new RegExp(`[^${allowed}]`, 'g');
	// 替换非白名单字符为逗号
	processed = processed.replace(whitelist, ',');
	// 合并连续逗号，避免出现一连串逗号
	processed = processed.replace(/,{2,}/g, ',');
	// 合并多余空格
	processed = processed.replace(/\s+/g, ' ');
	// 移除首尾空格
	return processed.trim();
}

/** 移除文本中的emoji表情符号 */
export function removeEmojiSymbols(text: string): string {
	return text.replace(/[\u{1F600}-\u{1F64F}\u{1F300}-\u{1F5FF}\u{1F680}-\u{1F6FF}\u{1F900}-\u{1F9FF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}\u{FE00}-\u{FE0F}\u{1F1E0}-\u{1F1FF}\u{200D}\u{20E3}\u{FE0F}]/gu, '');
}

/** 用于格式化文本，移除开头和结尾的标点符号 */
function formatChunk(chunk: string): string {
	/** 用于移除文本开头的标点符号 */
	const LEADING_PUNCT = /^[。，、：；:;,?!？！—～"'""''()（）\[\]【】{}<>…\s]+/;
	/** 用于移除文本结尾的标点符号 */
	const TRAILING_COMMA = /[，,]+$/;
	/** 移除文本开头和结尾的标点符号 */
	const result = chunk.replace(LEADING_PUNCT, '').replace(TRAILING_COMMA, '');
	//返回格式化后的清除首尾空格的文本
	return result.trim();
}
/** 基于标点将文本内容拆分为片段 */
function splitByPunct(source: string, punctRegex: RegExp): string[] {
	/** 拆分结果数组 */
	const result: string[] = [];
	/** 当前片段的起始位置 */
	let start = 0;
	// 遍历文本，查找标点符号位置
	for (let i = 0; i < source.length; i++) {
		// 如果当前字符不是标点符号，继续遍历
		if (!punctRegex.test(source[i])) continue;
		/** 当前片段的结束位置 */
		let end = i + 1;
		// 找到当前片段的结束位置
		while (end < source.length && punctRegex.test(source[end])) end++;
		/** 当前片段的文本 */
		const fragment = source.slice(start, end).trim();
		// 如果当前片段非空，添加到结果数组
		if (fragment.length > 0) result.push(fragment);
		// 更新当前片段的起始位置
		start = end;
		// 跳过当前标点符号
		i = end - 1;
	}
	// 如果文本末尾不是标点符号，添加最后一个片段
	if (start < source.length) {
		/** 最后一个片段的文本 */
		const fragment = source.slice(start).trim();
		// 如果最后一个片段非空，添加到结果数组
		if (fragment.length > 0) result.push(fragment);
	}
	// 返回拆分结果数组
	return result;
}
/** 检查文本是否在括号内 */
function isInsideBracket(source: string, pos: number): boolean {
	/** 括号的深度 */
	let depth = 0;
	// 遍历文本，查找括号位置
	for (let i = 0; i < pos; i++) {
		// 如果当前字符是左括号，深度加1
		if (source[i] === '\uFF08' || source[i] === '(') depth++;
		// 如果当前字符是右括号，深度减1
		else if (source[i] === '\uFF09' || source[i] === ')') depth--;
	}
	// 返回是否在括号内的结果
	return depth > 0;
}
/** 智能切分文本为句子 */
export function splitSentences(text: string): string[] {
	// 如果文本为空，直接返回空数组
	if (!text) return [];
	/** 用于拆分文本内容的一级标点符号 */
	const LEVEL1_PUNCT = /[。？！—～?!]/;
	/** 用于拆分文本内容的二级标点符号 */
	const LEVEL2_PUNCT = /[，,、：；:;]/;
	/** 每个切片的理想最大长度 */
	const IDEAL_MAXIMUM_LENGTH = 35;
	/** 一级标点符号拆分结果数组 */
	const level1 = splitByPunct(text, LEVEL1_PUNCT);
	/** 最终切分结果数组 */
	const result: string[] = [];
	// 遍历一级标点符号拆分结果数组
	for (let content of level1) {
		// 如果当前片段的长度小于等于理想最大长度，直接添加到结果数组
		if (content.length <= IDEAL_MAXIMUM_LENGTH) {
			/** 格式化后的当前片段文本 */
			const formatted = formatChunk(content);
			// 如果格式化后的文本非空，添加到结果数组
			if (formatted.length > 0) result.push(formatted);
			// 继续处理下一个片段
			continue;
		}
		// 遍历当前片段，查找二级标点符号位置
		while (content.length > IDEAL_MAXIMUM_LENGTH) {
			/** 当前片段的二级标点符号位置 */
			let splitPos = -1;
			// 遍历当前片段，查找二级标点符号位置
			for (let i = Math.min(content.length - 1, IDEAL_MAXIMUM_LENGTH - 1); i >= 0; i--) {
				// 如果当前字符不是二级标点符号，或在括号内，继续遍历
				if (!LEVEL2_PUNCT.test(content[i]) || isInsideBracket(content, i)) continue;
				/** 当前片段的二级标点符号位置 */
				let end = i + 1;
				// 找到当前片段的二级标点符号位置
				while (end < content.length && LEVEL2_PUNCT.test(content[end])) end++;
				// 更新当前片段的二级标点符号位置
				splitPos = end;
				// 找到二级标点符号位置，跳出循环
				break;
			}
			// 如果没有找到二级标点符号位置，跳出循环
			if (splitPos === -1) break;
			/** 格式化后的当前片段文本 */
			const slice = formatChunk(content.slice(0, splitPos));
			// 如果格式化后的文本非空，添加到结果数组
			if (slice.length > 0) result.push(slice);
			// 更新当前片段为剩余文本
			content = content.slice(splitPos);
		}
		/** 格式化后的当前片段文本 */
		const tail = formatChunk(content);
		// 如果格式化后的文本非空，添加到结果数组
		if (tail.length > 0) result.push(tail);
	}
	return result;
}

/** 解析文本内容 */
export function parseContent(rawText: string): ParsedContent {
	// 如果原始文本为空，直接返回空对象
	if (!rawText) return { thinkingBlocks: [], codeBlocks: [], actionBlocks: [], emotionBlocks: [], textChunks: [] };
	/** 提取思考区内容（不参与显示与语音合成） */
	const [thinkingBlocks, textAfterThinking] = extractThinkingBlocks(rawText);
	/** 提取代码块内容（不参与显示与语音合成） */
	const [codeBlocks, textAfterCode] = extractCodeBlocks(textAfterThinking);
	/** 先提取情感区内容（emoji表情与颜文字），避免颜文字的括号被动作区误吞（不参与显示与语音合成） */
	const [emotionBlocks, textAfterEmotion] = extractEmotionBlocks(textAfterCode);
	/** 再提取动作区内容（全角/半角括号包裹）（不参与显示与语音合成） */
	const [actionBlocks, textAfterAction] = extractActionBlocks(textAfterEmotion);
	/** 清洗用于显示的文本（兜底移除残留emoji） */
	const displayText = removeEmojiSymbols(textAfterAction);
	/** 对清洗后的文本执行智能切片 */
	const displayChunks = splitSentences(displayText);
	/** 为每个切片生成显示文本和TTS文本 */
	const textChunks: TextChunk[] = displayChunks.map(chunk => ({ display: chunk, tts: cleanTextForTTS(chunk), }));
	// 返回解析结果
	return { thinkingBlocks, codeBlocks, actionBlocks, emotionBlocks, textChunks };
}
