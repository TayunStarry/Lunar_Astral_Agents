/** 文本分割选项 */
type SplitOptions = {
	/** 理想的分割长度，默认从 SliderAPI.messageSliceLengthSlider 获取 */
	idealLen?: number;
	/** 路径前缀格式，默认 "[类别]: " */
	pathPrefix?: string;
	/** 是否将路径单独放在一行，默认 true */
	pathOnNewLine?: boolean;
	/** 是否跳过只有标题没有内容的片段，默认 true */
	skipTitleOnly?: boolean;
	/** 是否在内容中包含原始标题，默认 false */
	includeOriginalTitle?: boolean;
};

/** 解析后的 Markdown 标题段落 */
type MdSection = {
	/** 标题层级 1..6 */
	level: number;
	/** 标题文本（不含#） */
	title: string;
	/** 标题下的正文（直到下一个同级/更高级/任何标题） */
	content: string;
	/** 组合后的层级路径 */
	path: string;
};

/**
 * 将输入文本按指定长度拆分成若干字符串片段。
 *
 * 若文本像 Markdown，则按标题层级拆分并附带路径前缀；
 *
 * 否则按普通文本规则拆分。
 *
 * @param {string} input - 原始文本
 *
 * @param {SplitOptions} options - 拆分行为选项
 *
 * @returns {string[]} - 拆分后的字符串数组
 */
export function splitTextToStrings(input: string, options: SplitOptions = {}): string[] {
	/** 合并默认选项，确保后续逻辑一定能拿到完整配置 */
	const option: Required<SplitOptions> = {
		idealLen: options.idealLen ?? 1024,
		pathPrefix: options.pathPrefix ?? "*标题> ",
		pathOnNewLine: options.pathOnNewLine ?? true,
		skipTitleOnly: options.skipTitleOnly ?? true,
		includeOriginalTitle: options.includeOriginalTitle ?? false,
	};
	/** 统一换行符，避免 Windows 换行导致后续处理不一致 */
	const text = (input ?? "").replace(/\r\n/g, "\n");
	// 空文本直接返回空数组，避免无意义处理
	if (!text.trim()) return [];
	/** 判断是否为 Markdown：通过常见语法特征快速识别 */
	const isMarkdown = looksLikeMarkdown(text);
	// 按类型分流：普通文本直接按长度拆分；Markdown 需保留结构
	if (!isMarkdown) {
		return splitPlainText(text, option.idealLen);
	}
	// 按 Markdown 标题层级拆分
	return splitMarkdown(text, option);
};

/** ---------------- Plain Text ---------------- */

/**
 * 将普通文本按指定长度拆分成若干字符串片段。
 *
 * @param {string} text - 原始普通文本
 *
 * @param {number} idealLen - 理想单段长度
 *
 * @returns {string[]} - 拆分后的字符串数组
 */
function splitPlainText(text: string, idealLen: number): string[] {
	/** 存储最终结果 */
	const results: string[] = [];
	/** 当前处理位置 */
	let currentIndex = 0;
	/** 定义 Preferred Break 字符集 */
	const isPreferredBreak = (char: string) =>
		char === "\n" ||
		char === "。" ||
		char === "；" ||
		char === ";" ||
		char === "." ||
		char === "!" ||
		char === "?" ||
		char === "？" ||
		char === "！" ||
		char === "…" ||
		char === "、" ||
		char === ":" ||
		char === "：";
	// 主循环 - 按理想长度遍历文本
	while (currentIndex < text.length) {
		/** 计算当前剩余长度 */
		const remainingLength = text.length - currentIndex;
		// 若剩余长度小于等于理想长度，直接作为最后一段处理
		if (remainingLength <= idealLen) {
			/** 直接截取剩余部分作为最后一段 */
			const tailText = text.slice(currentIndex).trim();
			// 若最后一段非空，加入结果
			if (tailText) results.push(tailText);
			break;
		}
		/** 计算理想结束位置 */
		const endPosition = currentIndex + idealLen;
		/** 定义回退窗口，避免超出文本边界 */
		const backwardWindow = Math.min(idealLen, 256);
		/** 从理想结束位置开始回退，找 Preferred Break */
		let cutPosition = -1;
		// 从理想结束位置开始回退，找 Preferred Break
		for (let position = endPosition; position >= Math.max(currentIndex + 1, endPosition - backwardWindow); position--) {
			/** 当前字符 */
			const char = text[position - 1];
			// 若当前字符为 Preferred Break，记录位置
			if (isPreferredBreak(char)) {
				cutPosition = position;
				break;
			}
		}
		// 若回退窗口内未找到 Preferred Break，从理想结束位置开始继续回退
		if (cutPosition === -1) {
			// 若回退窗口内未找到 Preferred Break，从理想结束位置开始继续回退，找普通 Break
			for (let position = endPosition; position > currentIndex; position--) {
				/** 当前字符 */
				const char = text[position - 1];
				// 若当前字符为普通 Break，记录位置
				if (isPreferredBreak(char)) {
					cutPosition = position;
					break;
				}
			}
		}
		// 若回退窗口内未找到普通 Break，或普通 Break 位置在当前索引之前，直接取理想结束位置
		if (cutPosition === -1 || cutPosition <= currentIndex) cutPosition = endPosition;
		/** 当前片段文本 */
		const chunkText = text.slice(currentIndex, cutPosition).trim();
		// 若当前片段非空，加入结果
		if (chunkText) results.push(chunkText);
		// 更新当前索引为下一段开始位置
		currentIndex = cutPosition;
	}
	// 返回拆分后的字符串数组
	return results;
};

/** ---------------- Markdown ---------------- */

/**
 * 将 Markdown 文本按标题层级拆分成若干片段，每段不超过理想长度。
 *
 * @param {string} text 原始 Markdown 文本
 *
 * @param {Required<SplitOptions>} option  已合并默认值的拆分选项
 *
 * @returns {string[]} 拆分后的字符串数组
 */
function splitMarkdown(text: string, option: Required<SplitOptions>): string[] {
	/** 解析 Markdown 标题段落 */
	const sections = parseMarkdownSections(text);
	// 若无标题，退化为普通文本拆分
	if (sections.length === 0) {
		return splitPlainText(text, option.idealLen);
	}
	/** 存储最终结果 */
	const output: string[] = [];
	// 逐段处理
	for (const sec of sections) {
		// 如果启用了跳过只有标题的选项，并且内容为空，则跳过
		if (option.skipTitleOnly && sec.content.trim() === '') {
			continue;
		}
		/** 生成路径前缀 */
		const header = formatPath(sec.path, option);
		/** 拼接标题与内容：根据选项决定是否包含原始标题 */
		const body = option.includeOriginalTitle
			? (sec.title ? `#`.repeat(sec.level) + " " + sec.title + "\n" : "") + sec.content
			: sec.content;
		// 若 body 本身不超过理想长度，直接输出
		if (body.length <= option.idealLen) {
			/** 拼接路径前缀与正文 */
			const piece = (header + body).trimEnd();
			// 若拼接结果非空，加入结果
			if (piece.trim()) output.push(piece);
			continue;
		}
		/** 按行优先策略拆分正文 */
		const pieces = splitByNewlinePrefer(body, option.idealLen);
		// 处理每一行
		for (const current of pieces) {
			/** 拼接路径前缀与当前行 */
			const piece = (header + current).trimEnd();
			// 若拼接结果非空，加入结果
			if (piece.trim()) output.push(piece);
		}
	}
	// 返回 拆分后的字符串数组
	return output;
};

/**
 * 解析 Markdown 文本，将其按标题层级拆分成若干段落。
 *
 * @param {string} text 原始 Markdown 文本
 *
 * @returns {MdSection[]} 解析后的段落数组，每个段落包含层级、标题、内容和路径
 */
function parseMarkdownSections(text: string): MdSection[] {
	/** 替换所有 Windows 换行符为 Unix 换行符 */
	const normalizedText = text.replace(/\r\n/g, "\n");
	/** 按换行符拆分文本行 */
	const lines = normalizedText.split("\n");
	/** 标题正则：# 到 ######，后面至少一个空格或直接文本（兼容常见写法） */
	const headingRe = /^(#{1,6})\s+(.*)\s*$/;
	/** 存储解析后的段落 */
	const sections: MdSection[] = [];
	/** 维护标题层级栈 */
	const stack: { level: number; title: string }[] = [];
	/** 存储所有标题行索引 */
	const headingIdx: { i: number; level: number; title: string }[] = [];
	// 找到所有标题行索引
	for (let index = 0; index < lines.length; index++) {
		/** 当前行 */
		const line = lines[index];
		/** 当前行是否为标题 */
		const match = line.match(headingRe);
		/** 当前行是否为标题 */
		const isHeading = Boolean(match);
		// 若当前行是标题，记录索引、层级和标题文本
		if (isHeading && match) {
			headingIdx.push({ i: index, level: match[1].length, title: match[2].trim() });
		}
	}
	// 若无标题行，直接返回空数组
	if (headingIdx.length === 0) return [];
	// 遍历所有标题行，构建段落
	for (let k = 0; k < headingIdx.length; k++) {
		/** 当前标题行 */
		const cur = headingIdx[k];
		/** 下一个标题行 */
		const next = headingIdx[k + 1];
		/** 当前段落起始行索引 */
		const startLine = cur.i;
		/** 当前段落结束行索引（下一个标题行或文本结束） */
		const endLine = next ? next.i : lines.length;
		// 维护层级栈：遇到更浅/同级就弹
		while (stack.length && stack[stack.length - 1].level >= cur.level) {
			stack.pop();
		}
		// 加入当前标题到栈
		stack.push({ level: cur.level, title: cur.title });
		/** 当前段落路径（标题层级路径） */
		const path = stack.map(s => s.title).join(" / ");
		/** 当前段落内容（标题行之后到下个标题之前） */
		const contentLines = lines.slice(startLine + 1, endLine);
		/** 当前段落内容（标题行之后到下个标题之前，trimEnd 后 + 换行符） */
		const content = contentLines.join("\n").trimEnd() + "\n";
		// 加入当前段落
		sections.push({ level: cur.level, title: cur.title, content, path, });
	}
	// 返回 解析后的段落数组
	return sections;
};

/**
 * 按行优先策略拆分文本，尝试将文本拆分成长度不超过理想值的段落。
 *
 * @param {string} text 原始文本
 *
 * @param {number} idealLen 理想段落长度
 *
 * @returns {string[]} 拆分后的字符串数组
 */
function splitByNewlinePrefer(text: string, idealLen: number): string[] {
	/** 存储最终结果 */
	const result: string[] = [];
	/** 缓冲区：当前正在构建的段落 */
	let buffer = "";
	/** 刷新缓冲区：将当前段落加入结果，清空缓冲区 */
	const flushBuffer = () => {
		/** 缓冲区内容（trimEnd 后） */
		const trimmed = buffer.trimEnd();
		if (trimmed.trim()) result.push(trimmed + "\n");
		buffer = "";
	};
	/** 按换行符拆分文本行 */
	const lines = text.replace(/\r\n/g, "\n").split("\n");
	// 遍历所有行
	for (let index = 0; index < lines.length; index++) {
		/** 当前行 */
		const currentLine = lines[index];
		/** 加入当前行到缓冲区 */
		const appendStr = (buffer === "" ? "" : "\n") + currentLine;
		// 若加入当前行后长度不超过理想值，直接加入缓冲区
		if ((buffer + appendStr).length <= idealLen) {
			buffer += appendStr;
			continue;
		}
		// buffer 已经有内容就先flush，再处理当前行
		if (buffer.trim().length > 0) flushBuffer();
		// 单行就超长：硬切该行
		if (currentLine.length > idealLen) {
			let offset = 0;
			while (offset < currentLine.length) {
				/** 当前子段落 */
				const segment = currentLine.slice(offset, offset + idealLen);
				result.push(segment.trimEnd() + "\n");
				offset += idealLen;
			}
		}
		else {
			buffer = currentLine;
		}
	}
	// 最后检查缓冲区是否有剩余内容
	if (buffer.trim().length > 0) flushBuffer();
	return result;
};

/**
 * 根据配置将路径字符串格式化为最终输出前缀。
 *
 * @param {string} path 当前段落的层级路径（如“一级标题 / 二级标题”）
 *
 * @param {Required<SplitOptions>} option  已合并默认值的拆分选项，决定前缀格式与换行行为
 *
 * @returns {string} 格式化后的路径前缀，可能以换行符或空格结尾
 */
function formatPath(path: string, option: Required<SplitOptions>): string {
	/** 完整路径：前缀 + 路径 + 换行 */
	const wholePath = `${option.pathPrefix}${path}*\n`;
	// 若不要求路径独占一行，则去掉换行符并追加一个空格，使路径与正文同行
	return option.pathOnNewLine ? wholePath : `${option.pathPrefix}${path}* `;
};

/**
 * 判断文本是否看起来像 Markdown 格式。
 *
 * @param {string} text 原始文本
 *
 * @returns {boolean} 是否看起来像 Markdown 格式
 */
function looksLikeMarkdown(text: string): boolean {
	/** 是否包含标题行 */
	const hasHeading = /(^|\n)#{1,6}\s+\S/.test(text);
	/** 是否包含代码块围栏 */
	const hasFence = /(^|\n)```/.test(text);
	/** 是否包含列表项 */
	const hasList = /(^|\n)\s*([-*+]|\d+\.)\s+\S/.test(text);
	/** 是否包含引用块 */
	const hasQuote = /(^|\n)>\s+\S/.test(text);
	/** 是否包含表格 */
	const hasTable = /(^|\n)\s*\|.*\|/.test(text);
	// 返回判断结果
	return hasHeading || hasFence || hasList || hasQuote || hasTable;
};