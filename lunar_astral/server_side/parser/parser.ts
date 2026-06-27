/**
 * 正文切片数据结构
 *
 * display — 用于前端显示的文本（仅移除emoji，保留括号内容和原始符号）
 * tts     — 用于语音合成的文本（完整清洗：移除行内代码、图片标记、HTML标签等）
 */
export interface TextChunk {
	display: string;
	tts: string;
}

/**
 * 文本拆分结果数据结构
 *
 * thinkingBlocks — 从原始文本中提取的全部思考区内容（不参与语音合成）
 * codeBlocks     — 从原始文本中提取的全部代码块内容（不参与语音合成）
 * actionBlocks   — 从原始文本中提取的全部动作区内容（全角/半角括号包裹，不参与显示与语音合成）
 * emotionBlocks  — 从原始文本中提取的全部情感区内容（emoji表情，不参与显示与语音合成）
 * textChunks     — 清洗并切片后的正文内容，每个切片包含display和tts两个版本
 */
export interface ParsedContent {
	thinkingBlocks: string[];
	codeBlocks: string[];
	actionBlocks: string[];
	emotionBlocks: string[];
	textChunks: TextChunk[];
}

/**
 * emoji 表情正则（覆盖常见 emoji 区段，含变体选择符与 ZWJ）
 * 用于情感区提取及显示/TTS文本的兜底清洗
 */
const EMOJI_REGEX = /[\u{1F600}-\u{1F64F}\u{1F300}-\u{1F5FF}\u{1F680}-\u{1F6FF}\u{1F900}-\u{1F9FF}\u{1FA70}-\u{1FAFF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}\u{FE00}-\u{FE0F}\u{1F1E0}-\u{1F1FF}\u{200D}\u{20E3}\u{FE0F}]/gu;

/**
 * 从原始文本中提取思考区内容
 *
 * 匹配 标签，将每个匹配到的完整内容存入数组
 * 同时从原始文本中移除这些内容
 *
 * @param text - 原始文本
 * @returns [提取的思考区数组, 移除思考区后的文本]
 */
function extractThinkingBlocks(text: string): [string[], string] {
	const blocks: string[] = [];
	// 使用 exec 循环逐一提取，保留完整内容
	const regex = /<think>([\s\S]*?)<\/think>/gi;
	let match: RegExpExecArray | null;
	while ((match = regex.exec(text)) !== null) {
		const content = match[1].trim();
		if (content.length > 0) {
			blocks.push(content);
		}
	}
	// 从原文中移除思考区
	const remaining = text.replace(/<think>[\s\S]*?<\/think>/gi, '');
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
	const blocks: string[] = [];
	// 单一正则：匹配 ``` + 可选语言标识 + 内容 + ```
	// [a-zA-Z0-9+#-]* 匹配零个或多个语言标识字符，同时覆盖有/无语言标识的情况
	const codeBlockRegex = /```[a-zA-Z0-9+#-]*[\s\S]*?```/g;

	let match: RegExpExecArray | null;
	while ((match = codeBlockRegex.exec(text)) !== null) {
		// 保留完整匹配（含```标记），客户端需要标记来正确渲染代码块
		blocks.push(match[0]);
	}
	// 从原文中移除所有代码块
	const remaining = text.replace(/```[a-zA-Z0-9+#-]*[\s\S]*?```/g, '');
	return [blocks, remaining];
}

/**
 * 从原始文本中提取动作区内容（全角"（）"或半角"()"包裹的内容）
 *
 * 匹配最内层的括号对，内容不能包含括号字符本身（不支持嵌套）
 * 空括号或仅含空白的括号会被跳过
 * 同时从原始文本中移除这些括号片段
 *
 * @param text - 原始文本（应已移除思考区与代码块）
 * @returns [提取的动作区内容数组（仅括号内文字）, 移除动作区后的文本]
 */
function extractActionBlocks(text: string): [string[], string] {
	const blocks: string[] = [];
	// 匹配全角（）或半角()，内容为非括号字符（非贪婪）
	const regex = /[\(（]([^()（）]+?)[\)）]/g;
	let match: RegExpExecArray | null;
	while ((match = regex.exec(text)) !== null) {
		const content = match[1].trim();
		if (content.length > 0) {
			blocks.push(content);
		}
	}
	// 从原文中移除所有动作区括号片段
	const remaining = text.replace(/[\(（]([^()（）]*?)[\)）]/g, '');
	return [blocks, remaining];
}

/**
 * 从原始文本中提取情感区内容（emoji 表情）
 *
 * 连续的 emoji 字符会被合并为一条情感记录
 * 同时从原始文本中移除这些 emoji
 *
 * @param text - 原始文本（应已移除思考区、代码块、动作区）
 * @returns [提取的情感区内容数组（emoji字符串）, 移除emoji后的文本]
 */
function extractEmotionBlocks(text: string): [string[], string] {
	const blocks: string[] = [];
	const regex = new RegExp(EMOJI_REGEX.source, 'gu');
	let match: RegExpExecArray | null;
	/** 上一次匹配结束位置，用于判断是否连续 emoji */
	let lastEnd = -1;
	/** 当前累积的连续 emoji 字符串 */
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
	const remaining = text.replace(new RegExp(EMOJI_REGEX.source, 'gu'), '');
	return [blocks, remaining];
}

/**
 * 清洗文本，去除Markdown标记、行内代码、HTML标签、表情符号等不适合语音合成的内容
 *
 * 注意：此函数应在 extractThinkingBlocks 和 extractCodeBlocks 之后调用，
 * 确保思考区和代码块已被移除
 *
 * @param text - 已移除思考区和代码块后的文本
 * @returns 清洗后的文本
 */
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
	// 白名单过滤：仅保留中文、英文、数字、常用中英文标点
	const allowed = '\\u4e00-\\u9fff' + 'a-zA-Z0-9' + '\\s_~\\-' + '\uFF0C\u3002\uFF1F\uFF1A\uFF01\uFF1B\u3001\u2014\u2026\u300A\u300B\u201C\u201D\u2018\u2019\uFF08\uFF09\u3010\u3011' + ',.\'\"?:!;';
	const whitelist = new RegExp(`[^${allowed}]`, 'g');
	processed = processed.replace(whitelist, ',');
	// 合并多余空格
	processed = processed.replace(/\s+/g, ' ');
	return processed.trim();
}

/**
 * 清洗用于显示的文本
 *
 * 仅移除emoji表情符号，保留括号内容、原始符号、HTML标签、Markdown标记等，
 * 确保前端能完整渲染原始文本内容
 *
 * @param text - 原始文本
 * @returns 仅移除emoji后的文本
 */
export function cleanTextForDisplay(text: string): string {
	if (!text) return '';
	// 只移除emoji表情符号
	return text.replace(/[\u{1F600}-\u{1F64F}\u{1F300}-\u{1F5FF}\u{1F680}-\u{1F6FF}\u{1F900}-\u{1F9FF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}\u{FE00}-\u{FE0F}\u{1F1E0}-\u{1F1FF}\u{200D}\u{20E3}\u{FE0F}]/gu, '');
}

/**
 * 将清洗后的文本进行二级智能分句
 *
 * 一级切片：基于语句中断标点（句号、问号、感叹号、破折号、波浪号）切分
 * 二级切片：对一级切片后超过 MAX_LENGTH 字符的片段，按二级标点（逗号、顿号、冒号、分号）
 *           采用最小拆分原则切分——每次在 MAX_LENGTH 范围内选择最靠后的二级标点作为切断点，
 *           使切出的片段尽量长，从而用最少的切分次数满足长度约束；
 *           若片段内无二级标点，则按 MAX_LENGTH 强制切分
 * 格式处理：切片开头不允许出现任何标点符号；切片末尾若为逗号（全角/半角），剔除该逗号
 *
 * @param text - 清洗后的文本
 * @returns 句子数组
 */
export function splitSentences(text: string): string[] {
	if (!text) return [];

	// 一级切片：语句中断标点（句号、问号、感叹号、破折号、波浪号）
	const LEVEL1_PUNCT = /[。？！—～?!]/;
	// 二级切片：逗号、顿号、冒号、分号（中英文）
	const LEVEL2_PUNCT = /[，,、：；:;]/;
	// 切片最大长度阈值
	const MAX_LENGTH = 35;
	// 全部中英文标点（用于剔除切片开头标点）
	const LEADING_PUNCT = /^[。，、：；:;,?!？！—～"'""''()（）\[\]【】{}<>…\s]+/;
	// 末尾逗号（全角/半角）
	const TRAILING_COMMA = /[，,]+$/;

	/**
	 * 格式化切片：
	 * 1. 剔除开头的任意标点符号
	 * 2. 若末尾为逗号（全角/半角），剔除该逗号
	 */
	function formatChunk(chunk: string): string {
		let result = chunk.trim();
		// 剔除开头的标点符号
		result = result.replace(LEADING_PUNCT, '');
		// 剔除末尾的逗号
		result = result.replace(TRAILING_COMMA, '');
		return result.trim();
	}

	/**
	 * 按指定标点正则切分文本，标点归入前一个切片末尾
	 * 连续标点也一并归入前一个切片
	 */
	function splitByPunct(source: string, punctRegex: RegExp): string[] {
		const result: string[] = [];
		let start = 0;

		for (let i = 0; i < source.length; i++) {
			if (punctRegex.test(source[i])) {
				// 将当前标点及紧随的连续同类标点归入当前切片
				let end = i + 1;
				while (end < source.length && punctRegex.test(source[end])) {
					end++;
				}
				const fragment = source.slice(start, end).trim();
				if (fragment.length > 0) {
					result.push(fragment);
				}
				start = end;
				i = end - 1; // for 循环会 i++，所以设为 end - 1
			}
		}

		// 处理末尾无标点的残余文本
		if (start < source.length) {
			const fragment = source.slice(start).trim();
			if (fragment.length > 0) {
				result.push(fragment);
			}
		}

		return result;
	}

	// 一级切片
	const level1 = splitByPunct(text, LEVEL1_PUNCT);

	// 二级切片：对超过 MAX_LENGTH 的片段，按二级标点采用最小拆分原则切分
	// 括号内的二级标点不计入切分点（切分无意义）
	const result: string[] = [];

	/**
	 * 判断指定位置是否处于括号内部（中英文括号均支持）
	 */
	function isInsideBracket(source: string, pos: number): boolean {
		let depth = 0;
		for (let i = 0; i < pos; i++) {
			if (source[i] === '\uFF08' || source[i] === '(') depth++;
			else if (source[i] === '\uFF09' || source[i] === ')') depth--;
		}
		return depth > 0;
	}

	for (const fragment of level1) {
		if (fragment.length <= MAX_LENGTH) {
			const formatted = formatChunk(fragment);
			if (formatted.length > 0) result.push(formatted);
			continue;
		}

		let remaining = fragment;
		while (remaining.length > MAX_LENGTH) {
			// 最小拆分原则：在 MAX_LENGTH 范围内找最后一个二级标点作为切断点（跳过括号内的标点）
			// 这样切出的片段尽量长，切分次数最少
			let splitPos = -1;
			for (let i = Math.min(remaining.length - 1, MAX_LENGTH - 1); i >= 0; i--) {
				if (LEVEL2_PUNCT.test(remaining[i]) && !isInsideBracket(remaining, i)) {
					// 将连续二级标点归入当前切片
					let end = i + 1;
					while (end < remaining.length && LEVEL2_PUNCT.test(remaining[end])) {
						end++;
					}
					splitPos = end;
					break;
				}
			}

			// 无二级标点可切：强制按 MAX_LENGTH 长度切分，避免产生超长片段
			if (splitPos === -1) {
				splitPos = MAX_LENGTH;
			}

			const slice = formatChunk(remaining.slice(0, splitPos));
			if (slice.length > 0) {
				result.push(slice);
			}
			remaining = remaining.slice(splitPos);
		}

		// 处理剩余部分
		const tail = formatChunk(remaining);
		if (tail.length > 0) {
			result.push(tail);
		}
	}

	return result;
}

/**
 * 完整的文本解析流程：多维度提取 → 切片 → 双版本清洗
 *
 * 处理流程：
 * 1. 提取全部思考区内容 → thinkingBlocks（不参与显示与语音合成）
 * 2. 提取全部代码块内容 → codeBlocks（不参与显示与语音合成）
 * 3. 提取全部动作区内容（全角/半角括号包裹）→ actionBlocks（不参与显示与语音合成）
 * 4. 提取全部情感区内容（emoji表情）→ emotionBlocks（不参与显示与语音合成）
 * 5. 对剩余正文执行智能切片
 * 6. 清洗用于显示的文本 → display
 * 7. 清洗用于TTS的文本（移除行内代码、图片标记、HTML标签等）→ tts
 *
 * @param rawText - 原始文本
 * @returns ParsedContent 包含五个独立数组，textChunks中每个切片包含display和tts两个版本
 */
export function parseContent(rawText: string): ParsedContent {
	if (!rawText) return { thinkingBlocks: [], codeBlocks: [], actionBlocks: [], emotionBlocks: [], textChunks: [] };

	// 第一步：提取思考区内容
	const [thinkingBlocks, textAfterThinking] = extractThinkingBlocks(rawText);

	// 第二步：提取代码块内容
	const [codeBlocks, textAfterCode] = extractCodeBlocks(textAfterThinking);

	// 第三步：提取动作区内容（全角/半角括号包裹）
	const [actionBlocks, textAfterAction] = extractActionBlocks(textAfterCode);

	// 第四步：提取情感区内容（emoji表情）
	const [emotionBlocks, textAfterEmotion] = extractEmotionBlocks(textAfterAction);

	// 第五步：清洗用于显示的文本（兜底移除残留emoji）并智能切片
	const displayText = cleanTextForDisplay(textAfterEmotion);
	const displayChunks = splitSentences(displayText);

	// 第六步 & 第七步：为每个切片生成显示文本和TTS文本
	const textChunks: TextChunk[] = displayChunks.map(chunk => ({
		display: chunk,
		tts: cleanTextForTTS(chunk),
	}));

	return { thinkingBlocks, codeBlocks, actionBlocks, emotionBlocks, textChunks };
}
