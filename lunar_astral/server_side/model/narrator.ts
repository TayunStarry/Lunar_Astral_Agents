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
 * 将清洗后的文本进行二级智能分句
 *
 * 一级切片：基于语句中断标点（句号、冒号、感叹号、问号、破折号、波浪号）切分
 * 二级切片：对一级切片后超过35字符的片段，基于逗号进一步切分
 * 若片段中不存在可用于分段的标点符号，则保持原片段不切片
 * 标点符号始终位于切片末尾，切片顺序与原文完全一致
 *
 * @param text - 清洗后的文本
 * @returns 句子数组
 */
export function splitSentences(text: string): string[] {
    if (!text) return [];

    // 一级切片：语句中断标点
    const LEVEL1_PUNCT = /[。：？！—～:?!]/;
    // 二级切片：逗号类标点
    const LEVEL2_PUNCT = /[，,、；;]/;
    const MAX_LENGTH = 35;

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

    // 二级切片：对超过35字符的片段，在逗号处选择性地切断，确保每段不超过35字符
    const result: string[] = [];
    for (const fragment of level1) {
        if (fragment.length <= MAX_LENGTH) {
            result.push(fragment);
            continue;
        }

        // 在逗号处切分，但只在必要时切断，确保每段 ≤ 35字符
        let remaining = fragment;
        while (remaining.length > MAX_LENGTH) {
            // 在 MAX_LENGTH 范围内找最后一个逗号位置作为切断点
            let splitPos = -1;
            for (let i = Math.min(remaining.length - 1, MAX_LENGTH - 1); i >= 0; i--) {
                if (LEVEL2_PUNCT.test(remaining[i])) {
                    // 将连续逗号归入当前切片
                    let end = i + 1;
                    while (end < remaining.length && LEVEL2_PUNCT.test(remaining[end])) {
                        end++;
                    }
                    splitPos = end;
                    break;
                }
            }

            // 无逗号可切，保持原片段不再切分
            if (splitPos === -1) {
                break;
            }

            const slice = remaining.slice(0, splitPos).trim();
            if (slice.length > 0) {
                result.push(slice);
            }
            remaining = remaining.slice(splitPos);
        }

        // 处理剩余部分
        const tail = remaining.trim();
        if (tail.length > 0) {
            result.push(tail);
        }
    }

    return result;
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
