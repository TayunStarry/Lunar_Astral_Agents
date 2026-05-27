class OnlyData {
    static customConfig = { cloud: {}, server: {} };
    static toolCall = [];
    static imageFormatsExtensions = ['.jpg', '.jpeg', '.png', '.gif', '.bmp', '.webp'];
    static videoFormatsExtensions = ['.mp4', '.avi', '.mov', '.wmv', '.flv', '.mkv'];
    static fileValidExtensions = [
        '.txt', '.md', '.log', '.ini', '.conf',
        '.js', '.jsx', '.ts', '.tsx', '.py', '.java', '.c', '.cpp', '.h',
        '.cs', '.php', '.rb', '.go', '.rs', '.swift', '.kt', '.dart',
        '.json', '.csv', '.xml', '.yaml', '.yml',
        '.html', '.htm', '.css', '.scss', '.less', '.sass', '.styl',
        '.env', '.properties', '.toml',
        ...this.imageFormatsExtensions,
        ...this.videoFormatsExtensions
    ];
    static fileValidTypes = [
        'application/json',
        'application/xml',
        'application/x-yaml'
    ];
    static visionExtensions = [...this.imageFormatsExtensions, ...this.videoFormatsExtensions];
    static lunarToolPackageMap = new Map();
    static get systemUrl() {
        return url()[0] + '/v1';
    }
    ;
    static get fileServiceUrl() {
        return url()[0];
    }
    ;
    static get SystemKey() {
        return OnlyData.customConfig.cloud.cloud_model_key || 'key-520-1314-2000-02-18';
    }
    ;
    static get MultimodalName() {
        return OnlyData.customConfig.cloud.multimodal_model_name || "system-multimodal";
    }
    ;
    static get EmbeddingName() {
        return OnlyData.customConfig.cloud.embedding_model_name || "system-embedding";
    }
    ;
    static get userName() {
        return OnlyData.customConfig.server.user_name || "阁下";
    }
    ;
}

const ThinkType = [
    /<think>([\s\S]*?)<\/think>([\s\S]*)/,
    /<\|thought_start\|>([\s\S]*?)<\|thought_end\|>([\s\S]*)/,
];

function Clamp({ min, max }, value) {
    return Math.max(min, Math.min(max, value));
}
function RandomFloor(min, max) {
    return Math.floor(Math.random() * (max - min + 1) + min);
}
function RandomFloat(min, max, length = 2) {
    return Number((Math.random() * (max - min) + min).toFixed(length));
}
function CalculateMedian(numbers) {
    const sortedNumbers = [...numbers].sort((a, b) => a - b);
    const middleIndex = Math.floor(sortedNumbers.length / 2);
    if (sortedNumbers.length % 2 === 0)
        return (sortedNumbers[middleIndex - 1] + sortedNumbers[middleIndex]) / 2;
    else
        return sortedNumbers[middleIndex];
}
function CalculateModes(numbers) {
    const frequencyMap = new Map();
    let maxFrequency = 0;
    const modes = [];
    for (const number of numbers) {
        const frequency = (frequencyMap.get(number) || 0) + 1;
        frequencyMap.set(number, frequency);
        if (frequency > maxFrequency)
            maxFrequency = frequency;
    }
    frequencyMap.forEach((frequency, number) => {
        if (frequency === maxFrequency)
            modes.push(number);
    });
    return modes;
}

function getFileContent(path, removeNewLines = false) {
    let [content, size, mimeType, err] = readFile(path);
    if (err)
        throw err;
    const decodedContent = atob(String(content));
    if (removeNewLines)
        return decodedContent.replace(/[\r\n]+/g, '');
    return decodedContent.replace(/[ \t]+/g, ' ');
}
async function saveImageToServer(file) {
    try {
        const fileHash = await calculateFileHash(file);
        const fileExtension = file.name.slice(file.name.lastIndexOf('.')).toLowerCase();
        const newFileName = `${fileHash}${fileExtension}`;
        const base64FileName = toBtoaString('images/' + newFileName);
        const [_, __, err] = saveFile(base64FileName, true, file);
        if (!err)
            throw err;
        return `/read/images/${newFileName}`;
    }
    catch (error) {
        if (!(error instanceof Error))
            return '';
        return `${error.name} | ${error.message} | ${error.stack}`;
    }
}
async function fetchDocumentCallback(url, initializeContent = '{}', callback) {
    const defaultCallback = (content) => JSON.parse(content);
    const applyCallback = callback ?? defaultCallback;
    const fallback = async () => {
        saveFile(url.toString(), true, initializeContent);
        return applyCallback(initializeContent);
    };
    try {
        const filePath = url.toString().split(/[\/\\]/);
        const [list, err1] = fileList(filePath.slice(0, -1).join('/'));
        if (err1)
            return await fallback();
        const exists = list.some(item => item.name === filePath.slice(-filePath.length)[0] && !item.isDir);
        if (!exists)
            return await fallback();
        const [content, size, mimeType, err2] = readFile(url.toString());
        if (err2)
            return await fallback();
        const text = atob(String(content));
        if (!text)
            return await fallback();
        return applyCallback(text);
    }
    catch (error) {
        if (error instanceof Error)
            return await fallback();
    }
}

function splitTextToStrings(input, options = {}) {
    const option = {
        idealLen: options.idealLen ?? 1024,
        pathPrefix: options.pathPrefix ?? "*标题> ",
        pathOnNewLine: options.pathOnNewLine ?? true,
        skipTitleOnly: options.skipTitleOnly ?? true,
        includeOriginalTitle: options.includeOriginalTitle ?? false,
    };
    const text = (input ?? "").replace(/\r\n/g, "\n");
    if (!text.trim())
        return [];
    const isMarkdown = looksLikeMarkdown(text);
    if (!isMarkdown) {
        return splitPlainText(text, option.idealLen);
    }
    return splitMarkdown(text, option);
}
function splitPlainText(text, idealLen) {
    const results = [];
    let currentIndex = 0;
    const isPreferredBreak = (char) => char === "\n" ||
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
    while (currentIndex < text.length) {
        const remainingLength = text.length - currentIndex;
        if (remainingLength <= idealLen) {
            const tailText = text.slice(currentIndex).trim();
            if (tailText)
                results.push(tailText);
            break;
        }
        const endPosition = currentIndex + idealLen;
        const backwardWindow = Math.min(idealLen, 256);
        let cutPosition = -1;
        for (let position = endPosition; position >= Math.max(currentIndex + 1, endPosition - backwardWindow); position--) {
            const char = text[position - 1];
            if (isPreferredBreak(char)) {
                cutPosition = position;
                break;
            }
        }
        if (cutPosition === -1) {
            for (let position = endPosition; position > currentIndex; position--) {
                const char = text[position - 1];
                if (isPreferredBreak(char)) {
                    cutPosition = position;
                    break;
                }
            }
        }
        if (cutPosition === -1 || cutPosition <= currentIndex)
            cutPosition = endPosition;
        const chunkText = text.slice(currentIndex, cutPosition).trim();
        if (chunkText)
            results.push(chunkText);
        currentIndex = cutPosition;
    }
    return results;
}
function splitMarkdown(text, option) {
    const sections = parseMarkdownSections(text);
    if (sections.length === 0) {
        return splitPlainText(text, option.idealLen);
    }
    const output = [];
    for (const sec of sections) {
        if (option.skipTitleOnly && sec.content.trim() === '') {
            continue;
        }
        const header = formatPath(sec.path, option);
        const body = option.includeOriginalTitle
            ? (sec.title ? `#`.repeat(sec.level) + " " + sec.title + "\n" : "") + sec.content
            : sec.content;
        if (body.length <= option.idealLen) {
            const piece = (header + body).trimEnd();
            if (piece.trim())
                output.push(piece);
            continue;
        }
        const pieces = splitByNewlinePrefer(body, option.idealLen);
        for (const current of pieces) {
            const piece = (header + current).trimEnd();
            if (piece.trim())
                output.push(piece);
        }
    }
    return output;
}
function parseMarkdownSections(text) {
    const normalizedText = text.replace(/\r\n/g, "\n");
    const lines = normalizedText.split("\n");
    const headingRe = /^(#{1,6})\s+(.*)\s*$/;
    const sections = [];
    const stack = [];
    const headingIdx = [];
    for (let index = 0; index < lines.length; index++) {
        const line = lines[index];
        const match = line.match(headingRe);
        const isHeading = Boolean(match);
        if (isHeading && match) {
            headingIdx.push({ i: index, level: match[1].length, title: match[2].trim() });
        }
    }
    if (headingIdx.length === 0)
        return [];
    for (let k = 0; k < headingIdx.length; k++) {
        const cur = headingIdx[k];
        const next = headingIdx[k + 1];
        const startLine = cur.i;
        const endLine = next ? next.i : lines.length;
        while (stack.length && stack[stack.length - 1].level >= cur.level) {
            stack.pop();
        }
        stack.push({ level: cur.level, title: cur.title });
        const path = stack.map(s => s.title).join(" / ");
        const contentLines = lines.slice(startLine + 1, endLine);
        const content = contentLines.join("\n").trimEnd() + "\n";
        sections.push({ level: cur.level, title: cur.title, content, path, });
    }
    return sections;
}
function splitByNewlinePrefer(text, idealLen) {
    const result = [];
    let buffer = "";
    const flushBuffer = () => {
        const trimmed = buffer.trimEnd();
        if (trimmed.trim())
            result.push(trimmed + "\n");
        buffer = "";
    };
    const lines = text.replace(/\r\n/g, "\n").split("\n");
    for (let index = 0; index < lines.length; index++) {
        const currentLine = lines[index];
        const appendStr = (buffer === "" ? "" : "\n") + currentLine;
        if ((buffer + appendStr).length <= idealLen) {
            buffer += appendStr;
            continue;
        }
        if (buffer.trim().length > 0)
            flushBuffer();
        if (currentLine.length > idealLen) {
            let offset = 0;
            while (offset < currentLine.length) {
                const segment = currentLine.slice(offset, offset + idealLen);
                result.push(segment.trimEnd() + "\n");
                offset += idealLen;
            }
        }
        else {
            buffer = currentLine;
        }
    }
    if (buffer.trim().length > 0)
        flushBuffer();
    return result;
}
function formatPath(path, option) {
    const wholePath = `${option.pathPrefix}${path}*\n`;
    return option.pathOnNewLine ? wholePath : `${option.pathPrefix}${path}* `;
}
function looksLikeMarkdown(text) {
    const hasHeading = /(^|\n)#{1,6}\s+\S/.test(text);
    const hasFence = /(^|\n)```/.test(text);
    const hasList = /(^|\n)\s*([-*+]|\d+\.)\s+\S/.test(text);
    const hasQuote = /(^|\n)>\s+\S/.test(text);
    const hasTable = /(^|\n)\s*\|.*\|/.test(text);
    return hasHeading || hasFence || hasList || hasQuote || hasTable;
}

function toBtoaString(params) {
    const encodedParams = encodeURIComponent(params);
    const decodedParams = encodedParams.replace(/%([0-9A-F]{2})/g, (_, p1) => String.fromCharCode(parseInt(p1, 16)));
    return btoa(decodedParams);
}
async function FileToBase64(file) {
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = function (event) {
            const base64String = event.target?.result;
            if (!base64String)
                throw new Error("文件转 Base64 失败: 空字符串");
            resolve(base64String);
        };
        reader.onerror = function (error) {
            reject(new Error(`文件转 Base64 失败: ${error.target.error?.code}`));
        };
        reader.readAsDataURL(file);
    });
}
async function calculateFileHash(file) {
    function process(resolve) {
        const reader = new FileReader();
        reader.onload = async function (e) {
            try {
                const arrayBuffer = e.target?.result;
                const hashBuffer = await crypto.subtle.digest('SHA-256', arrayBuffer);
                const hashArray = Array.from(new Uint8Array(hashBuffer));
                const fullHash = hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
                const shortHash = fullHash.substring(0, 16);
                resolve(shortHash);
            }
            catch {
                resolve(toBtoaString(file.name).slice(-16));
            }
        };
        reader.onerror = async (error) => {
            if (!(error instanceof Error))
                return;
            resolve(`${error.name} | ${error.message} | ${error.stack}`);
        };
        reader.readAsArrayBuffer(file);
    }
    return new Promise(process);
}

function queryFromDatabase(operations, createTableOperation) {
    const requestBody = { operations, transaction: false };
    let [result, error] = database(requestBody);
    if (error)
        throw new Error('数据库查询失败');
    if (!result.success || !result.results[0].success) {
        const errorMessage = result.error || result.results[0].error || '';
        if (errorMessage.includes('no such table') && createTableOperation) {
            const createTableRequest = { operations: [createTableOperation], transaction: false };
            let [createTableResult, tableError] = database(createTableRequest);
            if (!tableError)
                throw new Error('创建表失败');
            if (!createTableResult.success)
                throw new Error('创建表失败');
            [result, error] = database(requestBody);
            if (!error)
                throw new Error('数据库查询失败');
            if (!result.success || !result.results[0].success)
                throw new Error('数据库查询失败');
        }
        else
            throw new Error('数据库查询失败');
    }
    return result;
}
function getPromptFromDatabase(key) {
    try {
        const operations = [
            {
                type: 'select',
                table: 'KeyPrompt',
                filter: {
                    IndexKey: key
                },
                limit: 1
            }
        ];
        const createTableOperation = {
            type: 'create',
            table: 'KeyPrompt',
            definition: {
                columns: [
                    { name: "ID", type: "INTEGER", primary_key: true, auto_increment: true },
                    { name: "IndexKey", type: "TEXT" },
                    { name: "Prompt", type: "TEXT" }
                ]
            }
        };
        const result = queryFromDatabase(operations, createTableOperation);
        if (result.success && result.results[0].success && result.results[0].rows) {
            return result.results[0].rows[0].Prompt;
        }
        return null;
    }
    catch (error) {
        return null;
    }
}
function savePromptToDatabase(key, prompt) {
    try {
        const existingPrompt = getPromptFromDatabase(key);
        const operations = [];
        if (existingPrompt)
            operations.push({ type: 'update', table: 'KeyPrompt', data: { Prompt: prompt }, filter: { IndexKey: key } });
        else
            operations.push({ type: 'insert', table: 'KeyPrompt', data: { IndexKey: key, Prompt: prompt } });
        const createTableOperation = {
            type: 'create',
            table: 'KeyPrompt',
            definition: {
                columns: [
                    { name: "ID", type: "INTEGER", primary_key: true, auto_increment: true },
                    { name: "IndexKey", type: "TEXT" },
                    { name: "Prompt", type: "TEXT" }
                ]
            }
        };
        const result = queryFromDatabase(operations, createTableOperation);
        return result.success && result.results[0].success;
    }
    catch (error) {
        console.error('向数据库存储提示词失败:', error);
        return false;
    }
}

let currentAddress = [];
class BaseConfig {
    isMultimodal = true;
    stream = false;
    enableTools = true;
    messages = [];
    systemPrompt = "你的名字叫做月华, 是一个女孩子";
    constructor() { }
}
class PromptProcessor extends BaseConfig {
    promptCompletion(prompt) {
        let addressText = "";
        if (currentAddress.length === 0) {
            const addressResult = address();
            currentAddress = addressResult[0];
            addressText = currentAddress.join(' ');
        }
        else
            addressText = currentAddress.join(' ');
        return prompt
            .replace(/{name}/g, OnlyData.userName)
            .replace(/{current-address}/g, addressText);
    }
    extractTextFromMessages(messages) {
        return messages.map(message => {
            if (typeof message.content === 'string') {
                return message.content;
            }
            else if (Array.isArray(message.content)) {
                const textContents = message.content
                    .filter(item => item.type === 'text')
                    .map(item => item.text);
                return textContents.join(' ');
            }
            return '';
        }).filter(text => text.trim() !== '');
    }
}
class ModeConfig extends PromptProcessor {
    useMultimodal(prompt) {
        this.systemPrompt = this.promptCompletion(prompt);
        this.isMultimodal = true;
        return this;
    }
    useEmbedding() {
        this.isMultimodal = false;
        return this;
    }
}
class ConfigModifier extends ModeConfig {
    setStream(stream = false) {
        this.stream = stream;
        return this;
    }
    setEnableTools(enable = true) {
        this.enableTools = enable;
        return this;
    }
    writeContext(context) {
        if (this.messages.length > 20)
            this.messages.slice(-20).push(context);
        else
            this.messages.push(context);
        return this;
    }
    coverContext(context) {
        this.messages = Array.isArray(context) ? context : [context];
        return this;
    }
}
class ModelBuilder extends ConfigModifier {
    get run() {
        if (this.isMultimodal)
            return this.runMultimodal();
        else
            return this.runEmbedding();
    }
    runMultimodal() {
        const isIncludesTools = this.messages.some((message) => message.role === 'tool');
        const requestBody = {
            model: OnlyData.MultimodalName,
            messages: [{ role: 'system', content: this.systemPrompt }, ...this.messages],
            stream: this.stream,
            tools: isIncludesTools ? [] : OnlyData.toolCall,
            tool_choice: isIncludesTools ? 'none' : 'auto',
        };
        if (!this.enableTools || !isIncludesTools) {
            delete requestBody.tool_choice;
            delete requestBody.tools;
        }
        const headers = {
            Authorization: `Bearer ${encodeURIComponent(OnlyData.SystemKey)}`,
            "Content-Type": "application/json",
        };
        const modelRequest = {
            method: "POST",
            crossDomain: true,
            headers,
            body: JSON.stringify(requestBody)
        };
        const endpoint = "/chat/completions";
        const [result, error] = syncFetch({ url: OnlyData.systemUrl + endpoint, execute: modelRequest });
        if (error)
            throw error;
        return result;
    }
    runEmbedding() {
        const validMessages = this.extractTextFromMessages(this.messages);
        const requestBody = {
            model: OnlyData.EmbeddingName,
            input: validMessages,
            stream: this.stream,
        };
        const headers = {
            Authorization: `Bearer ${encodeURIComponent(OnlyData.SystemKey)}`,
            "Content-Type": "application/json",
        };
        const modelRequest = {
            method: "POST",
            crossDomain: true,
            headers,
            body: JSON.stringify(requestBody)
        };
        const endpoint = "/embeddings";
        const [result, error] = syncFetch({ url: OnlyData.systemUrl + endpoint, execute: modelRequest });
        if (error)
            throw error;
        return result.data[0].embedding.slice(0, 256);
    }
    constructor() { super(); }
}

class ChatDialogueRole extends ModelBuilder {
    async callMultimediaAndToolParsing(cache, source) {
        try {
            await source.LiteImageFile();
            source.unreadContext.forEach(context => this.writeContext(context));
            source.unreadContext = [];
            this.formatHistoricalMessages(source);
            this.systemPrompt = this.systemPrompt.replace(/{current-time}/g, new Date().toLocaleString());
            const response = this.run;
            this.analyzeMessageResponse(response.body, cache, source);
            if (cache.toolCalls.length > 0) {
                const hasProcessedToolCalls = await this.batchExecutionToolCall(cache, source);
                if (hasProcessedToolCalls)
                    return await this.callMultimediaAndToolParsing(cache, source);
            }
            this.writeContext(response.body.choices?.[0]?.message);
        }
        catch (error) {
            console.error('请求处理错误:', error);
        }
        this.updateMessageContent(cache, source);
    }
    static MAX_TEXT_LENGTH = 512;
    formatHistoricalMessages(source) {
        if (this.messages.length === 0)
            return;
        const textMessageMap = new Set();
        const textMessages = [];
        const visionMessages = [];
        const formatMessages = [];
        for (const message of this.messages) {
            if (typeof message.content === 'string')
                textMessages.push(message);
            else
                for (let index = 0; index < message.content.length; index++) {
                    const content = message.content[index];
                    if (content.type == 'text')
                        textMessages.push({ role: message.role, content: content.text });
                    else
                        visionMessages.push({ role: message.role, content: [content] });
                }
        }
        for (const message of textMessages) {
            if (typeof message.content !== 'string' || textMessageMap.has(message.content))
                continue;
            formatMessages.push(message);
            textMessageMap.add(message.content);
        }
        if (visionMessages.length <= 10)
            formatMessages.push(...visionMessages);
        else
            for (let i = 0; i < visionMessages.length; i += 10) {
                const batchFrames = visionMessages.slice(i, i + 10);
                source.descriptionRole.coverContext(batchFrames);
                const summaryRequest = source.descriptionRole.run;
                const summary = summaryRequest.body?.choices?.[0]?.message?.content;
                if (summary && summary.trim().length > 0)
                    formatMessages.push({ role: 'user', content: summary });
            }
        this.messages = formatMessages;
        const latestRole = this.messages.slice(-1)[0].role;
        if (latestRole === 'user')
            return;
        this.writeContext({ role: 'user', content: '请继续之前的话题，或者对之前的内容进行优化完善。' });
    }
    analyzeMessageResponse(message, cache, source) {
        try {
            if (message.choices?.[0]?.message?.reasoning_content) {
                cache.thinkingContent = message.choices[0].message.reasoning_content;
            }
            if (message.timings?.predicted_per_second) {
                source.responseSpeed = message.timings.predicted_per_second;
            }
            if (message.choices?.[0]?.message?.tool_calls) {
                for (const toolCall of message.choices[0].message.tool_calls) {
                    try {
                        toolCall.function.arguments = JSON.parse(toolCall.function.arguments);
                        cache.toolCalls.push(toolCall);
                    }
                    catch (parseError) {
                        console.error('工具调用参数解析错误:', parseError);
                    }
                }
            }
            if (message.choices?.[0]?.message?.content) {
                cache.descriptionContent = message.choices[0].message.content;
            }
        }
        catch (error) {
            console.error('聊天消息响应处理错误:', error);
        }
    }
    async batchExecutionToolCall(state, source) {
        let hasToolCalls = false;
        for (const toolCall of state.toolCalls) {
            const functionName = toolCall.function.name;
            const functionArgs = toolCall.function.arguments;
            const lunarToolPackage = OnlyData.lunarToolPackageMap.get(functionName);
            if (!lunarToolPackage) {
                source.unreadContext.push({ role: "tool", content: `未找到工具包: ${functionName}`, tool_call_id: toolCall.id });
                continue;
            }
            try {
                const toolResult = await lunarToolPackage(functionArgs);
                source.unreadContext.push({ role: "tool", content: toolResult, tool_call_id: toolCall.id });
                hasToolCalls = true;
            }
            catch (error) {
                source.unreadContext.push({ role: "tool", content: `调用${functionName}失败: ${error}`, tool_call_id: toolCall.id });
            }
        }
        state.currentToolCallIndex = -1;
        state.currentFunctionArgs = "";
        state.currentFunctionName = "";
        state.currentToolCall = null;
        state.toolCalls = [];
        return hasToolCalls;
    }
    ;
    updateMessageContent(state, source) {
        if (state.thinkingContent.trim() !== "") {
            const newThinkTag = '<think>\n' + state.thinkingContent + '\n</think>\n';
            source.finalResponse = state.descriptionContent;
            console.log(newThinkTag);
        }
        else
            source.finalResponse = state.descriptionContent;
        return source.finalResponse;
    }
    constructor() {
        super();
        this.useMultimodal(fileView('prompts/chatRole.md')[0]);
    }
}

class PainterRole extends ModelBuilder {
    defaultExpressionPrompt = [
        '温柔的表情,开心的笑容,脸颊泛红',
        '害羞的表情,抿嘴微笑,眼神躲闪',
        '俏皮的表情,单眼眨眼,嘴角微微上扬,略带调皮的笑容',
        '平静的表情,眼神略微向下看向一侧,嘴唇轻抿,无笑容,若有所思',
        '惊讶的表情,双眼睁大,眉毛抬高,嘴巴微张成O形,脸颊泛红',
        '非常开心的表情,双眼弯成月牙形,张大嘴巴欢笑,脸颊泛红明显',
        '害羞的表情,眼神向下看,眉毛呈八字形,抿嘴微笑,脸颊大面积泛红',
        '自信的表情,眼神直视前方,眉毛微微上扬,嘴角带有一抹浅笑,眼神明亮'
    ];
    defaultPosturePrompt = [
        '一条腿轻轻抬起,俏皮姿势',
        '双手背在身后,身体微微前倾,双脚并拢',
        '一手插在外套口袋里,另一只手轻抬至脸颊旁比“V”字手势,身体略侧,双脚前后交叉站立',
        '双手自然垂放于身前,手指轻轻交握,双肩微微内收,双脚并拢,站姿端正',
        '手抬起至嘴前,十指轻轻触碰（做捂嘴状）,身体微微后仰,一条腿向后小半步,重心落在后脚',
        '双手举过头顶比心或张开五指,一条腿向后踢起,身体微微前倾,脚尖离地呈跳跃瞬间',
        '双手食指在胸前互点,头部微微低下,双膝内扣,两脚脚尖向内呈内八站姿',
        '双手叉腰,挺胸收腹,一条腿向侧方伸出,脚尖点地,身体笔直有力',
    ];
    selfAppearancePrompt = fileView('prompts/selfAppearance.md')[0];
    roleTool = [
        {
            type: "function",
            function: {
                name: "diffusion_generation",
                description: "根据文本描述生成图像。如需进行图像创作,请调用此函数",
                parameters: {
                    type: "object",
                    properties: {
                        "prompt": {
                            type: "string",
                            description: "图像生成的正向描述文本"
                        },
                        "negative_prompt": {
                            type: "string",
                            description: "负面提示文本,用于排除图像中不希望出现的元素"
                        },
                        "use_reference": {
                            type: "boolean",
                            description: "是否使用上一次生成的图像作为参考,默认值为 false"
                        },
                        "strength": {
                            type: "number",
                            description: "参考图像的影响强度,取值范围为 0 到 1,默认值为 0.65"
                        },
                        "cfg_scale": {
                            type: "number",
                            description: "提示词权重调节参数,取值范围为 0 到 2,默认值为 1.0"
                        }
                    },
                    required: [
                        "prompt"
                    ]
                }
            }
        },
        {
            type: "function",
            function: {
                name: "self_portrait",
                description: "生成自画像。调用此函数来创建自己的形象",
                parameters: {
                    type: "object",
                    properties: {
                        "expression": {
                            type: "string",
                            description: "表情提示词,描述想要展现的表情"
                        },
                        "posture": {
                            type: "string",
                            description: "动作提示词,描述想要展现的姿势或动作"
                        },
                        "environment": {
                            type: "string",
                            description: "环境提示词,描述背景环境或场景"
                        }
                    },
                    required: [
                        "expression",
                        "posture",
                        "environment"
                    ]
                }
            }
        }
    ];
    constructor() {
        super();
        this.useMultimodal(fileView('prompts/painterRole.md')[0]);
    }
    writeAppearancePrompt(expression, posture) {
        const currentExpression = expression || this.defaultExpressionPrompt[RandomFloor(0, this.defaultExpressionPrompt.length - 1)];
        const currentPosture = posture || this.defaultPosturePrompt[RandomFloor(0, this.defaultPosturePrompt.length - 1)];
        return this.selfAppearancePrompt.replace('{expression}', currentExpression).replace('{posture}', currentPosture);
    }
}

class AgentDefine {
    compilePlan = new ModelBuilder();
    queryKeywords = new ModelBuilder();
    emotionManager = new ModelBuilder();
    recorderRole = new ModelBuilder();
    summaryRole = new ModelBuilder();
    descriptionRole = new ModelBuilder();
    painterRole = new PainterRole();
    chatDialogueRole = new ChatDialogueRole();
    embedding = new ModelBuilder().useEmbedding();
    unreadContext = [];
    unreadVideoUrl = [];
    finalResponse = "";
    responseSpeed = 0;
    defaultAnswers = [
        '月华摔疼了，要等星光阁哥哥来修……',
        '糟糕啦，请告诉星光阁哥哥，月华遇到麻烦了！',
        '完蛋啦！快给星光阁哥哥传个信儿——月华碰上事儿啦，急得像热锅上的蚂蚁转圈圈呢！',
        '完犊子！快帮我给星光阁哥哥递句话——月华摊上事儿啦，十万火急',
        '救命！快给星光阁哥哥递个加急小纸条：月华那边遇到麻烦啦，速来捞人！',
    ];
    get randomDefaultMessage() {
        return this.defaultAnswers[RandomFloor(0, this.defaultAnswers.length)];
    }
    constructor() {
        this.compilePlan.useMultimodal(fileView('prompts/compilePlan.md')[0]);
        this.queryKeywords.useMultimodal(fileView('prompts/queryKeywords.md')[0]);
        this.emotionManager.useMultimodal(fileView('prompts/emotionManager.md')[0]);
        this.recorderRole.useMultimodal(fileView('prompts/recorderRole.md')[0]);
        this.summaryRole.useMultimodal(fileView('prompts/summaryRole.md')[0]);
        this.descriptionRole.useMultimodal(fileView('prompts/descriptionRole.md')[0]);
        fetchDocumentCallback('lunar_config.json').then(content => OnlyData.customConfig = content);
    }
    async analysisVideoFile(videoUrl, userNeeds) {
        const cachedPrompt = getPromptFromDatabase(videoUrl);
        if (cachedPrompt) {
            this.unreadContext.push({ role: 'user', content: cachedPrompt });
            return;
        }
        const [images, error] = keyframe(videoUrl, './cache');
        if (images.length === 0 || error)
            throw new Error('提取关键帧失败');
        const sandboxMessages = [];
        let videoSummary = '';
        const frameMessages = images.map(frame => ({ type: "image_url", image_url: { url: `data:image/jpeg;base64,${frame.data}` } }));
        for (let i = 0; i < frameMessages.length; i += 20) {
            const batchFrames = frameMessages.slice(i, i + 20);
            this.descriptionRole.coverContext({ role: 'user', content: batchFrames });
            const summaryRequest = this.descriptionRole.run;
            const summary = summaryRequest.body?.choices?.[0]?.message?.content;
            if (summary && summary.trim().length > 0)
                sandboxMessages.push({ type: 'text', text: summary });
        }
        if (sandboxMessages.length > 1) {
            this.summaryRole.coverContext({ role: 'user', content: sandboxMessages });
            const summaryRequest = this.summaryRole.run;
            videoSummary = summaryRequest.body?.choices?.[0]?.message?.content;
        }
        else if (sandboxMessages.length === 1)
            videoSummary = sandboxMessages[0].text;
        else
            videoSummary = this.defaultAnswers[RandomFloor(0, this.defaultAnswers.length - 1)];
        if (videoSummary)
            this.unreadContext.push({ role: 'user', content: videoSummary });
        if (userNeeds.trim().length > 0)
            this.unreadContext.push({ role: 'user', content: userNeeds });
        if (videoSummary)
            savePromptToDatabase(videoUrl, videoSummary);
    }
    async LiteImageFile() {
        for (let message of this.unreadContext) {
            if (typeof message.content === 'string')
                continue;
            const newContent = [];
            for (let item of message.content) {
                if (item.type == 'text')
                    newContent.push(item);
                else if (OnlyData.videoFormatsExtensions.some(format => item.image_url.url.toLowerCase().endsWith(format))) {
                    await this.analysisVideoFile(item.image_url.url, '');
                }
                else if (!item.image_url.url.startsWith("data:image")) {
                    const [response, error] = syncFetch({ url: item.image_url.url, execute: { crossDomain: true } });
                    if (error)
                        throw new Error('获取图片文件失败');
                    const [resizedBlob, error1] = resizeImage(response.body);
                    if (error1)
                        throw new Error('缩放图片失败');
                    newContent.push({ type: 'image_url', image_url: { url: resizedBlob.base64 } });
                }
            }
            message.content = newContent;
        }
    }
}

class LunarAgent extends AgentDefine {
    speakWeight = 1;
    async batchProcessVideoFiles(userNeeds) {
        if (this.unreadVideoUrl.length === 0)
            return;
        for (const videoUrl of this.unreadVideoUrl) {
            try {
                await this.analysisVideoFile(videoUrl, userNeeds || '');
                await new Promise(resolve => setTimeout(resolve, 1000));
            }
            catch (error) {
                continue;
            }
        }
        this.unreadVideoUrl = [];
    }
    async createChatMessage() {
        const cache = { currentToolCallIndex: -1, currentFunctionArgs: '', currentFunctionName: '', descriptionContent: '', thinkingContent: '', currentToolCall: null, toolCalls: [], };
        await this.chatDialogueRole.callMultimediaAndToolParsing(cache, this);
        this.speakWeight--;
        return this.finalResponse;
    }
    async thinkingChainProcess() {
        let errorCount = 0;
        while (true) {
            try {
                await this.pullExternalMessages();
                const messageLength = this.unreadContext.length + this.unreadVideoUrl.length;
                const messageType = messageLength === 0 ? 'response' : 'active';
                const allowSpeak = RandomFloor(15, 100) < this.speakWeight;
                if (messageLength === 0 && !allowSpeak) {
                    await new Promise(resolve => setTimeout(resolve, 1000));
                    continue;
                }
                else if (messageLength == 0 && allowSpeak)
                    this.speakWeight = 0;
                await this.batchProcessVideoFiles();
                await this.createChatMessage();
                const messageResponse = this.finalResponse.trim().length ? this.finalResponse : this.randomDefaultMessage;
                pushContext(messageType, messageResponse);
            }
            catch (error) {
                if (this.pushErrorMessage(error, errorCount))
                    break;
                errorCount++;
            }
        }
    }
    pushErrorMessage(error, errorCount) {
        console.error(error.message, ' || ', error.stack);
        if (errorCount < 3)
            return false;
        pushContext('active', this.defaultAnswers[RandomFloor(0, this.defaultAnswers.length - 1)]);
        return true;
    }
    async pullExternalMessages() {
        pullContext().forEach(message => this.writeMessage(message.role, message.content));
        pullVideoUrl().forEach(videoUrl => { this.writeVideoUrl(videoUrl); });
        await new Promise(resolve => setTimeout(resolve, 1000));
    }
    writeMessage(role, messages) {
        this.unreadContext.push({ role, content: messages });
        this.speakWeight += RandomFloor(1, 3);
        if (typeof messages === 'string')
            messages = [{ type: 'text', text: messages }];
        messages.forEach(message => { if (message.type === 'text')
            console.log(message.text); });
    }
    writeVideoUrl(videoUrl) {
        console.log('写入视频文件:' + videoUrl);
        this.unreadVideoUrl.push(videoUrl);
        this.speakWeight += RandomFloor(1, 3);
    }
    async testMessageWrite(role, messages, timeout) {
        await new Promise(resolve => setTimeout(resolve, timeout));
        if (messages.length > 0)
            this.writeMessage(role, messages);
    }
    constructor() { super(); this.thinkingChainProcess(); }
}
const AgentExample = new LunarAgent();
const message = [
    {
        type: 'text',
        text: '像与老朋友见面一样，打个招呼吧'
    }
];
AgentExample.testMessageWrite('user', message, 1500);

