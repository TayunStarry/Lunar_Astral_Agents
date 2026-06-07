/**
 * 文本拆分结果数据结构
 *
 * thinkingBlocks — 从原始文本中提取的全部思考区内容（不参与语音合成）
 * codeBlocks     — 从原始文本中提取的全部代码块内容（不参与语音合成）
 * textChunks     — 清洗并切片后的正文内容（参与语音合成）
 */
export interface ParsedContent {
    thinkingBlocks: string[];
    codeBlocks: string[];
    textChunks: string[];
}

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
    processed = processed.replace(whitelist, '，');
    // 合并多余空格
    processed = processed.replace(/\s+/g, ' ');
    return processed.trim();
}

/**
 * 将清洗后的文本按目标长度进行智能分句
 *
 * 在目标长度附近寻找最接近的标点符号位置切断，避免硬性截断
 * 优先在目标长度之后找标点，若找不到则在目标长度之前找
 *
 * @param text - 清洗后的文本
 * @param targetLength - 目标切片长度，默认30
 * @returns 句子数组
 */
export function splitSentences(text: string, targetLength: number = 30): string[] {
    if (!text) return [];

    const TARGET_LENGTH = targetLength;
    // 可作为分句断点的标点符号
    const PUNCTUATION = /[。？！…，、；：,;:\.\?!]/;
    const sentences: string[] = [];
    let remaining = text;

    while (remaining.length > 0) {
        // 短文本直接作为一句
        if (remaining.length <= TARGET_LENGTH * 1.5) {
            sentences.push(remaining.trim());
            break;
        }

        // 在目标长度附近寻找标点符号断点
        let splitPos = -1;

        // 优先从目标位置向后搜索标点（允许超出目标长度）
        const searchEnd = Math.min(remaining.length, TARGET_LENGTH + Math.floor(TARGET_LENGTH * 0.5));
        for (let i = TARGET_LENGTH; i < searchEnd; i++) {
            if (PUNCTUATION.test(remaining[i])) {
                splitPos = i + 1; // 包含标点符号
                break;
            }
        }

        // 向后未找到，则从目标位置向前搜索
        if (splitPos === -1) {
            const searchStart = Math.max(0, TARGET_LENGTH - Math.floor(TARGET_LENGTH * 0.5));
            for (let i = TARGET_LENGTH - 1; i >= searchStart; i--) {
                if (PUNCTUATION.test(remaining[i])) {
                    splitPos = i + 1;
                    break;
                }
            }
        }

        // 前后都找不到标点，在目标长度处硬切
        if (splitPos === -1 || splitPos === 0) {
            splitPos = TARGET_LENGTH;
        }

        // 确保标点符号在句尾而非下一句开头：将切分点后的连续标点归入当前句
        while (splitPos < remaining.length && PUNCTUATION.test(remaining[splitPos])) {
            splitPos++;
        }

        const sentence = remaining.slice(0, splitPos).trim();
        if (sentence.length > 0) {
            sentences.push(sentence);
        }
        remaining = remaining.slice(splitPos);
    }

    return sentences;
}

/**
 * 完整的文本解析流程：拆分 → 清洗 → 切片
 *
 * 处理流程：
 * 1. 提取全部思考区内容 → thinkingBlocks（不参与语音合成）
 * 2. 提取全部代码块内容 → codeBlocks（不参与语音合成）
 * 3. 对剩余文本执行清洗操作
 * 4. 对清洗后的文本执行智能切片 → textChunks（参与语音合成）
 *
 * @param rawText - 原始文本
 * @returns ParsedContent 包含三个独立数组
 */
export function parseContent(rawText: string): ParsedContent {
    if (!rawText) return { thinkingBlocks: [], codeBlocks: [], textChunks: [] };

    // 第一步：提取思考区内容
    const [thinkingBlocks, textAfterThinking] = extractThinkingBlocks(rawText);

    // 第二步：提取代码块内容
    const [codeBlocks, textAfterCode] = extractCodeBlocks(textAfterThinking);

    // 第三步：清洗剩余文本
    const cleanedText = cleanTextForTTS(textAfterCode);

    // 第四步：智能切片
    const textChunks = splitSentences(cleanedText);

    return { thinkingBlocks, codeBlocks, textChunks };
}
