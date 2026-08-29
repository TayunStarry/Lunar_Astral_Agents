/** 文本清洗与智能断句（TTS 清洗 / emoji 移除 / 句子切分） */

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
