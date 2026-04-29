import * as EntryAPI from '../EntryAPI/code';

/**
 * 提取文章的结论
 *
 * 该函数会尝试从给定的文本内容中提取文章的结论部分。
 *
 * 首先会尝试匹配包含 <think> 标签的推理过程部分，提取标签后的内容作为结论。
 *
 * 接着会尝试匹配 <|thought_start|>...<|thought_end|> 格式的思考过程，提取标签后的内容作为结论。
 *
 * 若未找到以上标签，会尝试匹配包含特定 HTML 类名 "conclusion" 的结论部分，提取其中的文本内容。
 *
 * 若仍然没有找到符合格式的结论部分，将返回原始文本内容。
 *
 * @param {string} content - 包含推理过程和结论的文本内容
 *
 * @returns {string} 提取到的结论文本内容
 */
export function extractConclusion(content: string): string {
    /**
     * 尝试匹配 <think> 标签及其内容，以及标签后的结论部分
     * 正则表达式解释：
     * - <think> 匹配 <think> 标签开头
     * - ([\s\S]*?) 匹配 <think> 和 </think> 之间的任意内容，非贪婪模式
     * - <\/think> 匹配 </think> 标签结尾
     * - ([\s\S]*) 匹配 </think> 标签后的所有内容
     */
    const thinkMatch = content.match(/<think>([\s\S]*?)<\/think>([\s\S]*)/);
    // 如果匹配成功，获取 </think> 标签后的内容并去除首尾空白后返回
    if (thinkMatch) return thinkMatch[2].trim();
    /**
     * 尝试匹配 <|thought_start|>...<|thought_end|> 格式的思考过程
     * 正则表达式解释：
     * - <\|thought_start\|> 匹配思考开始标签
     * - ([\s\S]*?) 匹配两个标签之间的任意内容，非贪婪模式
     * - <\|thought_end\|> 匹配思考结束标签
     * - ([\s\S]*) 匹配 <|thought_end|> 标签后的所有内容
     */
    const thoughtMatch = content.match(/<\|thought_start\|>([\s\S]*?)<\|thought_end\|>([\s\S]*)/);
    // 如果匹配成功，获取 <|thought_end|> 标签后的内容并去除首尾空白后返回
    if (thoughtMatch) return thoughtMatch[2].trim();
    /**
     * 尝试匹配类名为 "conclusion" 的 div 元素
     * 正则表达式解释：
     * - <div class="conclusion"> 匹配类名为 "conclusion" 的 div 标签开头
     * - ([\s\S]*?) 匹配 div 标签内的任意内容，非贪婪模式
     * - <\/div> 匹配 div 标签结尾
     * - i 标志表示不区分大小写
     */
    const conclusionMatch = content.match(/<div class="conclusion">([\s\S]*?)<\/div>/i);
    // 如果匹配成功，移除匹配到的内容中的 HTML 标签，去除首尾空白后返回
    if (conclusionMatch) return conclusionMatch[1].replace(/<[^>]*>/g, "").trim();
    // 如果以上三种匹配都未成功，说明没有找到特定格式的结论，返回原始文本内容
    return content;
};

/**
 * 处理文本中的思考标签，将其转换为特定格式的 HTML 结构
 *
 * 该函数会尝试匹配两种思考标签格式：
 * 1. <think>...</think> 格式
 * 2. <|thought_start|>...<|thought_end|> 格式
 *
 * 如果匹配成功，会将思考内容渲染并包装成特定的 HTML 结构，剩余内容作为结论处理。
 * 如果未匹配到思考标签，会直接解析内容并为表格元素添加 "markdown-table" 类名。
 *
 * @param {string} content - 包含思考标签的文本内容
 * @returns {string} 处理后的 HTML 内容
 */
export function processThinkTags(content: string): string {
    // 遍历所有模式，尝试匹配思考标签
    for (const pattern of EntryAPI.ThinkType) {
        /**
         * 尝试匹配当前模式的思考标签
         */
        const match = content.match(pattern);
        // 如果匹配成功，执行以下操作
        if (match) {
            /**
             * 提取思考内容
             */
            const thinkContent = match[1];
            /**
             * 提取思考标签后的剩余内容，若不存在则为空字符串
             */
            const remainingContent = match[2] || '';
            /**
             * 对思考内容进行 Markdown 解析，去除首尾空白
             */
            const renderedThink = (window as any).marked.parse(thinkContent.trim());
            /**
             * 对剩余内容进行 Markdown 解析，若剩余内容为空则不解析，去除首尾空白
             */
            const renderedRemaining = remainingContent.trim() ? (window as any).marked.parse(remainingContent.trim()) : '';
            // 返回包装好的 HTML 结构
            return [
                '<div class="think-block">',
                '<div class="think-header">',
                '<span><i class="fas fa-lightbulb think-icon"></i> 深度思考</span>',
                '<button class="chat-action-button toggle_think_button" style="font-size: 1.35rem;">',
                '<i class="fas fa-angle-down"></i>',
                '</button>',
                '</div>',
                '<div class="think-content">',
                renderedThink,
                '</div>',
                '</div>',
                '<div class="conclusion">',
                renderedRemaining,
                '</div>',
            ].join('');
        }
    };
    /**
     * 没有匹配到思考标签时，直接对原始内容进行 Markdown 解析
     */
    let processedContent = (window as any).marked.parse(content);
    // 为所有表格元素添加 "markdown-table" 类名，方便样式控制
    processedContent = processedContent.replace(/<table(\s[^>]*)?>/gi, (_: string, attrs: string) => `<table class="markdown-table"${attrs ? ' ' + attrs.trim() : ''}>`);
    // 返回处理后的 HTML 内容
    return processedContent;
};

/**
 * 清理文本，用于语音合成
 *
 * @param {string} text - 输入的文本
 *
 * @returns {string} - 清理后的文本
 */
export function cleanTextForTTS(text: string): string {
    // 如果输入文本为空，直接返回空字符串
    if (!text) return "";
    // 移除英文括号及其内的内容
    let cleanedText = text.replace(/\([^)]*\)/g, "");
    // 移除中文括号及其内的内容
    cleanedText = cleanedText.replace(/\（[^）]*\）/g, "");
    // 移除 HTML 标签
    cleanedText = cleanedText.replace(/<[^>]+>/g, "");
    // 移除代码块（由 ``` 包裹的内容）
    cleanedText = cleanedText.replace(/```[\s\S]*?```/g, "");
    // 移除 Markdown 图片语法
    cleanedText = cleanedText.replace(/!\[.*?\]\(.*?\)/g, "");
    // 移除 Emoji 表情符号
    cleanedText = cleanedText.replace(/[\uD800-\uDBFF][\uDC00-\uDFFF]|[\u2600-\u27FF]|[\uD83C][\uDF00-\uDFFF]|[\uD83D][\uDC00-\uDDFF]/g, "");
    // 移除单星号包裹的内容（非加粗或斜体语法）
    cleanedText = cleanedText.replace(/(?<!\*)\*[^*]*\*(?!\*)/g, "");
    // 移除 * 与 # 字符
    cleanedText = cleanedText.replace(/[*#]/g, "");
    // 将多个连续空格替换为单个空格，并去除首尾空格
    return cleanedText.replace(/\s{2,}/g, " ").trim();
};

/**
 * 移除代码中的注释并处理单引号转双引号
 *
 * @param {string} codeContent - 包含注释的代码内容
 * @returns {string} - 移除注释并处理单引号后的代码内容
 */
export function removeCodeComments(codeContent: string): string {
    /**
     * 提取 shebang 行（如果存在），shebang 行通常以 #! 开头
     */
    const shebang = codeContent.startsWith('#!')
        ? codeContent.match(/^#!.*?\n/)?.[0] || ''
        : '';
    // 若存在 shebang 行，则从代码内容中移除它
    codeContent = shebang ? codeContent.slice(shebang.length) : codeContent;
    /**
     * 移除特定格式的注释：
     * 1. 移除行首以 %%% 开头和结尾的多行注释
     * 2. 移除行首以 %% 开头到行尾的单行注释
     * 3. 移除行首以 # 开头的单行注释
     */
    codeContent = codeContent
        .replace(/%%[^\n]*\n?/g, '')
        .replace(/%%%[\s\S]*?%%%/g, '')
        .replace(/^\s*#[^\n]*\n/gm, '');
    /**
     * 移除常见的 JavaScript 注释：
     * 1. 移除 // 开头的单行注释
     * 2. 移除 /* *\/ 包裹的多行注释
     */
    codeContent = codeContent
        .replace(/\/\/[^\n]*\n/g, "")
        .replace(/\/\*[\s\S]*?\*\//g, "");
    /**
     * 处理单引号转双引号：
     * 1. 将转义的单引号 \' 替换为十六进制表示 \x27
     * 2. 将单引号包裹的字符串替换为双引号包裹
     * 3. 将十六进制表示的单引号 \x27 恢复为转义的单引号 \'
     */
    codeContent = codeContent
        .replace(/\\'/g, '\x27')
        .replace(/'((?:[^'\\]|\\.)*?)'/g, '"$1"')
        .replace(/\x27/g, "\\'");

    // 将提取的 shebang 行重新添加到代码内容开头并返回
    return shebang + codeContent;
};