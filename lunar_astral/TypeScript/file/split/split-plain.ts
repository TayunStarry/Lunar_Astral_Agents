/**
 * 将普通文本按指定长度拆分成若干字符串片段。
 *
 * @param {string} text - 原始普通文本
 *
 * @param {number} idealLen - 理想单段长度
 *
 * @returns {string[]} - 拆分后的字符串数组
 */
export function splitPlainText(text: string, idealLen: number): string[] {
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
