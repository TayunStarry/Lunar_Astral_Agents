/** 从原始文本中提取思考区 / 代码块 / 动作区 / 情感区（各返回 [提取内容, 剩余文本]） */

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
export function extractThinkingBlocks(text: string): [string[], string] {
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
export function extractCodeBlocks(text: string): [string[], string] {
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
export function extractActionBlocks(text: string): [string[], string] {
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
export function extractEmotionBlocks(text: string): [string[], string] {
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
