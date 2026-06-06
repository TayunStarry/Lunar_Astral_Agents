/**
 * TTS语音合成模块
 *
 * 对文本进行清洗、分句，然后依次调用tts()合成语音
 */

/**
 * 清洗文本，去除Markdown标记、代码块、HTML标签、表情符号等不适合语音合成的内容
 *
 * @param {string} text - 原始文本
 * @returns {string} 清洗后的文本
 */
export function cleanTextForTTS(text: string): string {
    if (!text) return '';
    let processed = text;
    processed = processed.replace(/<think>[\s\S]*?<\/think>/gi, '');
    processed = processed.replace(/```[a-zA-Z][a-zA-Z0-9+#-]*[\s\S]*?```/g, '');
    processed = processed.replace(/```[\s\S]*?```/g, '');
    processed = processed.replace(/`[^`]*`/g, '');
    processed = processed.replace(/!\[.*?\]\(.*?\)/g, '');
    processed = processed.replace(/\[.*?\]\(.*?\)/g, '');
    processed = processed.replace(/<[^>]*>/g, '');
    processed = processed.replace(/[\u{1F600}-\u{1F64F}\u{1F300}-\u{1F5FF}\u{1F680}-\u{1F6FF}\u{1F900}-\u{1F9FF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}\u{FE00}-\u{FE0F}\u{1F1E0}-\u{1F1FF}\u{200D}\u{20E3}\u{FE0F}]/gu, '');
    processed = processed.replace(/\*/g, '');
    processed = processed.replace(/\r?\n/g, ' ');
    processed = processed.replace(/\（[^）]*\）/g, '');
    processed = processed.replace(/\([^)]*\)/g, '');
    const allowed = '\\u4e00-\\u9fff' + 'a-zA-Z0-9' + '\\s~' + '\uFF0C\u3002\uFF1F\uFF1A\uFF01\uFF1B\u3001\u2014\u2026\u300A\u300B\u201C\u201D\u2018\u2019\uFF08\uFF09\u3010\u3011' + ',.\'\"?:!';
    const whitelist = new RegExp(`[^${allowed}]`, 'g');
    processed = processed.replace(whitelist, '，');
    processed = processed.replace(/\s+/g, ' ');
    return processed.trim();
}

/**
 * 将清洗后的文本按目标长度30进行智能分句
 *
 * 在目标长度附近寻找最接近的标点符号位置切断，避免硬性截断
 * 优先在目标长度之后找标点，若找不到则在目标长度之前找
 *
 * @param {string} text - 清洗后的文本
 * @returns {string[]} 句子数组
 */
export function splitSentences(text: string): string[] {
    if (!text) return [];

    const TARGET_LENGTH = 30;
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

        const sentence = remaining.slice(0, splitPos).trim();
        if (sentence.length > 0) {
            sentences.push(sentence);
        }
        remaining = remaining.slice(splitPos);
    }

    return sentences;
}

/**
 * 对文本执行TTS语音合成：先清洗，再分句，依次合成
 *
 * @param {string} text - 要合成的原始文本
 */
export function synthesizeSpeech(text: string): void {
    const cleaned = cleanTextForTTS(text);
    if (!cleaned) return;

    const sentences = splitSentences(cleaned);
    if (sentences.length === 0) return;

    for (const sentence of sentences) {
        try {
            const [audio, err] = tts(sentence);
            if (err) {
                console.error(`TTS合成失败: [${sentence}]`, err);
            }
        }
        catch (e) {
            console.error(`TTS合成异常: [${sentence}]`, e);
        }
    }
}
