var agentSystem = (function (exports) {
    'use strict';

    class OnlyData {
        static customConfig = { cloud: {}, server: {} };
        static unreadRecords = [];
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
            return `/file/read/images/${newFileName}`;
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
        stream = false;
        enableTools = true;
        messages = [];
        ragMessages = [];
        runtimeMessages = [];
        systemPrompt = "你的名字叫做月华, 是一个女孩子";
        static chromemReady = false;
        constructor() { }
        static initChromem() {
            if (BaseConfig.chromemReady)
                return;
            const [_, err] = chromemInit(OnlyData.systemUrl, OnlyData.SystemKey, OnlyData.EmbeddingName);
            if (err)
                console.error('chromem 初始化失败:', err);
            else
                BaseConfig.chromemReady = true;
        }
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
    class ConfigModifier extends PromptProcessor {
        setStream(stream = false) {
            this.stream = stream;
            return this;
        }
        setEnableTools(enable = true) {
            this.enableTools = enable;
            return this;
        }
        writeContext(context) {
            if (this.messages.length >= 20) {
                const discarded = this.messages.slice(0, this.messages.length - 19);
                this.messages = this.messages.slice(-19).concat(context);
                OnlyData.unreadRecords.push(...discarded);
            }
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
        run(appendContext, toolCall) {
            const messages = [
                { role: 'system', content: this.systemPrompt },
                ...appendContext,
                ...this.messages.slice(0, -1),
                ...this.runtimeMessages,
                ...this.messages.slice(-1)
            ];
            const requestBody = {
                model: OnlyData.MultimodalName,
                messages: messages,
                stream: this.stream,
                tools: toolCall,
                tool_choice: 'auto',
            };
            if (!this.enableTools || toolCall.length === 0) {
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
        queryRagMessages() {
            const latestUserMessage = this.getLatestUserMessageContent();
            if (!latestUserMessage)
                return this;
            if (!BaseConfig.chromemReady)
                BaseConfig.initChromem();
            if (!BaseConfig.chromemReady)
                return this;
            const [results, error] = chromemQuery(latestUserMessage, 10);
            if (error) {
                console.error('chromem 查询失败:', error);
                return this;
            }
            if (results && results.length > 0) {
                this.ragMessages = results.map((r) => ({ role: r.role, content: r.content, }));
                this.ragMessages.forEach((message) => console.log(message.content));
            }
            return this;
        }
        getLatestUserMessageContent() {
            const userTexts = [];
            for (let i = this.messages.length - 1; i >= 0 && userTexts.length < 5; i--) {
                const message = this.messages[i];
                if (message.role === 'user') {
                    if (typeof message.content === 'string') {
                        userTexts.unshift(message.content);
                    }
                    else if (Array.isArray(message.content)) {
                        const textContent = message.content
                            .filter(item => item.type === 'text')
                            .map(item => item.text)
                            .join(' ');
                        if (textContent.trim())
                            userTexts.unshift(textContent);
                    }
                }
            }
            return userTexts.length > 0 ? userTexts.join(' ') : null;
        }
        constructor(prompt) {
            super();
            this.systemPrompt = this.promptCompletion(prompt);
        }
    }

    class DialogueRole extends ModelBuilder {
        async callMultimediaAndToolParsing(cache, source) {
            try {
                await source.LiteImageFile();
                source.unreadContext.forEach(context => this.writeContext(context));
                source.unreadContext = [];
                this.formatHistoricalMessages(source);
                this.runtimeMessages = [{ role: 'user', content: `当前时间: ${new Date().toLocaleString()}` }];
                this.queryRagMessages();
                const response = this.run(this.ragMessages, []);
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
        formatHistoricalMessages(source) {
            if (this.messages.length === 0)
                return;
            const flattenedMessages = [];
            for (const message of this.messages) {
                if (typeof message.content === 'string') {
                    flattenedMessages.push(message);
                }
                else {
                    for (const content of message.content) {
                        if (content.type === 'text')
                            flattenedMessages.push({ role: message.role, content: content.text });
                        else
                            flattenedMessages.push({ role: message.role, content: [content] });
                    }
                }
            }
            const visionCount = flattenedMessages.filter(m => Array.isArray(m.content)).length;
            if (visionCount <= 10) {
                this.messages = flattenedMessages;
                return;
            }
            const processedMessages = [];
            let visionBuffer = [];
            for (const message of flattenedMessages) {
                const isVisionMessage = Array.isArray(message.content);
                if (isVisionMessage) {
                    visionBuffer.push(message);
                }
                else {
                    if (visionBuffer.length > 0) {
                        this.processVisionBuffer(visionBuffer, processedMessages, source);
                        visionBuffer = [];
                    }
                    processedMessages.push(message);
                }
            }
            if (visionBuffer.length > 0) {
                this.processVisionBuffer(visionBuffer, processedMessages, source);
            }
            this.messages = processedMessages;
        }
        processVisionBuffer(buffer, output, source) {
            if (buffer.length <= 10) {
                output.push(...buffer);
                return;
            }
            for (let i = 0; i < buffer.length; i += 10) {
                const batchFrames = buffer.slice(i, i + 10);
                source.descriptionRole.coverContext(batchFrames);
                const summaryRequest = source.descriptionRole.run([], []);
                const summary = summaryRequest.body?.choices?.[0]?.message?.content;
                if (summary && summary.trim().length > 0) {
                    output.push({ role: 'user', content: summary });
                }
            }
        }
        analyzeMessageResponse(message, cache, source) {
            try {
                if (message.choices?.[0]?.message?.reasoning_content) {
                    cache.thinkingContent = message.choices[0].message.reasoning_content;
                }
                if (message.timings?.predicted_per_second) {
                    source.responseSpeed = message.timings.predicted_per_second;
                    console.log(`词元生成速度: ${message.timings.predicted_per_second}`);
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
            super(fileView('prompts/dialogueRole.md')[0]);
        }
    }

    let Prompt$1 = class Prompt extends ModelBuilder {
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
            '一手插在外套口袋里,另一只手轻抬至脸颊旁比"V"字手势,身体略侧,双脚前后交叉站立',
            '双手自然垂放于身前,手指轻轻交握,双肩微微内收,双脚并拢,站姿端正',
            '手抬起至嘴前,十指轻轻触碰"做捂嘴状",身体微微后仰,一条腿向后小半步,重心落在后脚',
            '双手举过头顶比心或张开五指,一条腿向后踢起,身体微微前倾,脚尖离地呈跳跃瞬间',
            '双手食指在胸前互点,头部微微低下,双膝内扣,两脚脚尖向内呈内八站姿',
            '双手叉腰,挺胸收腹,一条腿向侧方伸出,脚尖点地,身体笔直有力',
        ];
        selfAppearancePrompt = fileView('prompts/selfAppearance.md')[0];
        writeAppearancePrompt(expression, posture, environment) {
            const currentExpression = expression || this.defaultExpressionPrompt[RandomFloor(0, this.defaultExpressionPrompt.length - 1)];
            const currentPosture = posture || this.defaultPosturePrompt[RandomFloor(0, this.defaultPosturePrompt.length - 1)];
            return this.selfAppearancePrompt.replace('{expression}', currentExpression).replace('{posture}', currentPosture).replace('{environment}', environment || '');
        }
    };
    let Toolchain$1 = class Toolchain extends Prompt$1 {
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
                            },
                            "negative_prompt": {
                                type: "string",
                                description: "负面提示文本,用于排除图像中不希望出现的元素"
                            },
                            "cfg_scale": {
                                type: "number",
                                description: "提示词权重调节参数,取值范围为 0 到 2,默认值为 1.0"
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
        handleDiffusionGeneration(args) {
            try {
                const prompt = args.prompt || '';
                if (!prompt.trim())
                    return '扩散生成失败：正向提示词不能为空';
                console.log(`[画家] 扩散生成 - 正向提示词: ${prompt.slice(0, 100)}...`);
                const imageParams = {
                    prompt: prompt,
                    negativePrompt: args.negative_prompt || '',
                    cfgScale: args.cfg_scale ?? 1.0,
                };
                const [result, error] = generateImage(imageParams);
                if (error) {
                    console.error('[画家] 图像生成失败:', error);
                    return `扩散图像生成失败: ${error}`;
                }
                if (!result || !result.base64) {
                    return '扩散图像生成失败：引擎返回空结果';
                }
                console.log(`[画家] 扩散图像生成成功，尺寸: ${result.width}x${result.height}`);
                const pushSuccess = pushImage([result.base64]);
                if (!pushSuccess) {
                    console.warn('[画家] 推送图片到前端失败');
                }
                return `扩散图像生成成功。图片尺寸: ${result.width}x${result.height}，seed: ${result.seed}`;
            }
            catch (error) {
                console.error('[画家] 扩散生成处理异常:', error);
                return `扩散图像生成异常: ${error}`;
            }
        }
        handleSelfPortrait(args) {
            try {
                console.log(`[画家] -> 自画像生成`);
                console.log(`表情: "${args.expression}"`);
                console.log(`姿势: "${args.posture}"`);
                console.log(`环境: "${args.environment}"`);
                console.log(`负面提示词: "${args.negative_prompt}"`);
                console.log(`提示词引导系数: "${args.cfg_scale}"`);
                const fullPrompt = this.writeAppearancePrompt(args.expression, args.posture, args.environment);
                const defaultNegativePrompt = '低分辨率, 糙噪点, 超现实主义, 丑陋的面部特征, 失真表情, 模糊轮廓, 颜色失衡, 不均匀光影, 强烈对比度, 过曝或欠曝, 杂乱背景, 像素化, 彩虹效果, 畸形肢体, 错位比例, 低质感纹理';
                const imageParams = {
                    prompt: fullPrompt,
                    negativePrompt: args.negative_prompt || defaultNegativePrompt,
                    cfgScale: args.cfg_scale ?? 1.0,
                };
                const [result, error] = generateImage(imageParams);
                if (error) {
                    console.error('[画家] 自画像生成失败:', error);
                    return `自画像生成失败: ${error}`;
                }
                if (!result || !result.base64) {
                    return '自画像生成失败：引擎返回空结果';
                }
                console.log(`[画家] 自画像生成成功，尺寸: ${result.width}x${result.height}`);
                const pushSuccess = pushImage([result.base64]);
                if (!pushSuccess) {
                    console.warn('[画家] 推送自画像到前端失败');
                }
                return `自画像生成成功。图片尺寸: ${result.width}x${result.height}，seed: ${result.seed}`;
            }
            catch (error) {
                console.error('[画家] 自画像生成处理异常:', error);
                return `自画像生成异常: ${error}`;
            }
        }
    };
    class PainterRole extends Toolchain$1 {
        constructor() {
            super(fileView('prompts/painterRole.md')[0]);
        }
        createImageRendering(source, count = 10) {
            this.coverContext([...source.dialogueRole.messages, ...source.unreadContext]);
            const unreadTexts = [];
            for (const message of source.unreadContext.slice(-count)) {
                if (typeof message.content === 'string')
                    unreadTexts.push(message.content);
                else
                    message.content.forEach(item => { if (item.type === 'text')
                        unreadTexts.push(item.text); });
            }
            let allowGeneration = false;
            const imageKeywords = [
                /画(?:一(?:张|幅|个))?/,
                /生成(?:一(?:张|幅|个))?.*(?:图|画|图片|图像)/,
                /图片/,
                /绘画/,
                /画图/,
                /自画像/,
                /画像/,
                /绘制/,
                /创作.*(?:图|画)/,
                /帮我.*画/,
                /给我.*画/,
                /来(?:一(?:张|幅|个))?.*(?:图|画)/,
                /draw/,
                /paint/,
                /image|picture|portrait/,
                /generate.*image/,
                /create.*(?:image|picture)/,
                /插图/,
                /插画/,
                /(?:做|弄|整)(?:一(?:张|幅|个))?.*(?:图|画)/,
            ];
            unreadTexts.forEach(text => imageKeywords.forEach(keyword => { if (keyword.test(text))
                allowGeneration = true; }));
            if (!allowGeneration)
                return true;
            const MAX_ITERATIONS = 3;
            for (let i = 0; i < MAX_ITERATIONS; i++) {
                console.log(`[画家] 第 ${i + 1} 轮绘画推理`);
                let response;
                try {
                    response = this.run([], this.roleTool);
                }
                catch (error) {
                    console.error(`[画家] 第 ${i + 1} 轮推理失败:`, error);
                    break;
                }
                const choice = response.body?.choices?.[0];
                if (!choice) {
                    console.log('[画家] 模型返回空结果，结束绘画循环');
                    break;
                }
                const toolCalls = choice.message?.tool_calls;
                if (!toolCalls || toolCalls.length === 0) {
                    const replyContent = choice.message?.content || '';
                    if (replyContent)
                        source.unreadContext.push({ role: 'user', content: `[画家反馈] ${replyContent}` });
                    break;
                }
                this.writeContext(choice.message);
                for (const toolCall of toolCalls) {
                    console.log(`[画家] 执行工具: ${toolCall.function.name}`);
                    const result = this.executePaintingTool(toolCall);
                    this.writeContext({ role: 'tool', content: result, tool_call_id: toolCall.id });
                }
            }
            return false;
        }
        executePaintingTool(toolCall) {
            const funcName = toolCall.function.name;
            let args = {};
            try {
                args = typeof toolCall.function.arguments === 'string' ? JSON.parse(toolCall.function.arguments) : toolCall.function.arguments;
            }
            catch (parseError) {
                console.error(`[画家] 工具调用参数解析失败:`, toolCall.function.arguments);
                return `工具调用参数解析失败，请确保传入合法的 JSON 字符串。错误: ${parseError}`;
            }
            switch (funcName) {
                case 'diffusion_generation': return this.handleDiffusionGeneration(args);
                case 'self_portrait': return this.handleSelfPortrait(args);
                default: return `未知工具: ${funcName}，可用工具为 diffusion_generation 和 self_portrait`;
            }
        }
    }

    class Prompt extends ModelBuilder {
        buildOrganizePrompt(records) {
            const now = new Date();
            const recordTexts = records.map((msg, idx) => {
                const content = typeof msg.content === 'string'
                    ? msg.content
                    : JSON.stringify(msg.content);
                const preview = content.length > 300 ? content.slice(0, 300) + '...' : content;
                const timestamp = now.toLocaleString('zh-CN', {
                    year: 'numeric',
                    month: '2-digit',
                    day: '2-digit',
                    hour: '2-digit',
                    minute: '2-digit',
                    second: '2-digit',
                    hour12: false
                });
                return `[记录${idx + 1}] 时间:${timestamp} | 角色:${msg.role} | 内容:${preview}`;
            });
            return `请整理以下 ${records.length} 条对话记录:\n\n${recordTexts.join('\n')}\n\n【重要原则】合并优先，新增为辅！请严格按照以下流程操作：\n1. 先用 query_existing_records 充分查询已有档案（建议 top_k=10），确认是否存在相似记录\n2. 如果找到语义相似的已有记录，必须使用 merge_existing_record 合并到已有记录中，而非创建新条目\n3. 仅当确认无任何相似记录时，才使用 store_organized_record 新增\n4. 完全重复的信息直接跳过，不存储\n\n每条记录必须严格遵循格式：[时间戳] 地点:{地点} | 人物:{参与者} | 事件:{事件摘要} | 话题:{关键词}。完成后请输出整理报告。`;
        }
        ensureTimestampInRecord(content) {
            const timestampRegex = /^\[([^\]]+)\]/;
            if (timestampRegex.test(content)) {
                return content;
            }
            const now = new Date();
            const timestamp = now.toLocaleString('zh-CN', {
                year: 'numeric',
                month: '2-digit',
                day: '2-digit',
                hour: '2-digit',
                minute: '2-digit',
                second: '2-digit',
                hour12: false
            });
            return `[${timestamp}] ${content}`;
        }
    }
    class Toolchain extends Prompt {
        organizeTools = [
            {
                type: "function",
                function: {
                    name: "query_existing_records",
                    description: "查询向量数据库中已存在的历史档案记录，用于查重和关联。在生成新记录前，必须先调用此工具确认是否已有相似记录。建议使用较大的 top_k 值（如10）以确保充分查重。",
                    parameters: {
                        type: "object",
                        properties: {
                            query_text: {
                                type: "string",
                                description: "用于语义检索的查询关键词或描述文本，建议使用多个关键词组合查询"
                            },
                            top_k: {
                                type: "integer",
                                description: "返回最相关的记录数量，建议设为10以确保充分查重，最大不超过20条"
                            }
                        },
                        required: ["query_text"]
                    }
                }
            },
            {
                type: "function",
                function: {
                    name: "merge_existing_record",
                    description: "将新内容合并到已有的历史档案记录中。当新内容与已有记录存在语义关联（同一话题延续、同一事件更新、内容补充等）时，必须使用此工具而非 store_organized_record。操作会删除旧记录并存储合并后的新记录。",
                    parameters: {
                        type: "object",
                        properties: {
                            id: {
                                type: "string",
                                description: "要合并的已有记录ID，从 query_existing_records 返回结果中获得"
                            },
                            merged_content: {
                                type: "string",
                                description: "合并后的完整记录内容，必须包含旧记录和新记录的所有关键信息，严格遵循格式：[时间戳] 地点:{地点} | 人物:{参与者} | 事件:{事件摘要} | 话题:{关键词}"
                            }
                        },
                        required: ["id", "merged_content"]
                    }
                }
            },
            {
                type: "function",
                function: {
                    name: "delete_existing_record",
                    description: "删除向量数据库中已存在的重复或过时的历史档案记录。在合并或更新已有记录时，应先删除旧记录再存储新记录。",
                    parameters: {
                        type: "object",
                        properties: {
                            id: {
                                type: "string",
                                description: "要删除的历史记录ID，从 query_existing_records 返回结果中获得"
                            }
                        },
                        required: ["id"]
                    }
                }
            },
            {
                type: "function",
                function: {
                    name: "store_organized_record",
                    description: "将整理好的结构化记录存储到向量数据库。仅当通过 query_existing_records 确认无相似记录时才可使用此工具。如果存在相似记录，应使用 merge_existing_record 合并而非新建。每条记录必须严格遵循格式：[时间戳] 地点:{地点} | 人物:{参与者} | 事件:{事件摘要} | 话题:{关键词}",
                    parameters: {
                        type: "object",
                        properties: {
                            content: {
                                type: "string",
                                description: "结构化记录内容，严格遵循指定格式，包含时间、地点、人物、事件、话题五个维度的完整信息"
                            }
                        },
                        required: ["content"]
                    }
                }
            }
        ];
        executeOrganizeTool(toolCall) {
            const funcName = toolCall.function.name;
            let args = {};
            try {
                args = typeof toolCall.function.arguments === 'string'
                    ? JSON.parse(toolCall.function.arguments)
                    : toolCall.function.arguments;
            }
            catch {
                console.error(`[编纂者] 工具调用参数解析失败:`, toolCall.function.arguments);
                return `工具调用参数解析失败，请确保传入合法的 JSON 字符串`;
            }
            console.log(`[编纂者] 执行工具: ${funcName}`);
            switch (funcName) {
                case 'query_existing_records':
                    return this.handleQueryRecords(args.query_text || '', args.top_k || 10);
                case 'merge_existing_record':
                    return this.handleMergeRecord(args.id || '', args.merged_content || '');
                case 'delete_existing_record':
                    return this.handleDeleteRecord(args.id || '');
                case 'store_organized_record':
                    return this.handleStoreRecord(args.content || '');
                default:
                    console.warn(`[编纂者] 未知工具: ${funcName}`);
                    return `未知工具: ${funcName}，可用工具为 query_existing_records、merge_existing_record、delete_existing_record 和 store_organized_record`;
            }
        }
        handleQueryRecords(queryText, topK) {
            if (!queryText || queryText.trim().length === 0) {
                return '查询文本为空，请提供有效的查询关键词';
            }
            const [results, error] = chromemQuery(queryText.trim(), topK);
            if (error) {
                console.error('[编纂者] chromem 查询失败:', error);
                return `向量数据库查询失败: ${error}`;
            }
            if (!results || results.length === 0) {
                return '未找到相关历史记录，可以放心创建新档案';
            }
            return '找到以下相关历史记录:\n' + results
                .map((r, i) => `[已有记录${i + 1}] ID:${r.id} | 内容:${r.content}`)
                .join('\n');
        }
        handleMergeRecord(id, mergedContent) {
            if (!id || id.trim().length === 0) {
                return '记录ID为空，无法合并，请提供从 query_existing_records 获取的记录ID';
            }
            if (!mergedContent || mergedContent.trim().length === 0) {
                return '合并内容为空，已跳过合并';
            }
            if (!BaseConfig.chromemReady) {
                BaseConfig.initChromem();
                if (!BaseConfig.chromemReady) {
                    return '向量数据库未就绪，合并失败，请稍后重试';
                }
            }
            const [deleteResult, deleteError] = chromemDelete(id.trim());
            if (deleteError) {
                console.error('[编纂者] 合并时删除旧记录失败:', deleteError);
                return `合并失败：删除旧记录 ${id} 时出错: ${deleteError}`;
            }
            console.log(`[编纂者] 合并：已删除旧记录 ${id}`);
            const finalContent = this.ensureTimestampInRecord(mergedContent.trim());
            const [addResult, addError] = chromemAdd('assistant', finalContent);
            if (addError) {
                console.error('[编纂者] 合并时存储新记录失败:', addError);
                return `合并失败：旧记录 ${id} 已删除，但存储合并内容时出错: ${addError}。合并内容: ${finalContent.slice(0, 200)}`;
            }
            console.log(`[编纂者] 合并成功：旧记录 ${id} 已替换为合并内容`);
            return `记录合并成功：已将旧记录 ${id} 替换为合并后的内容`;
        }
        handleDeleteRecord(id) {
            if (!id || id.trim().length === 0) {
                return '记录ID为空，已跳过删除';
            }
            if (!BaseConfig.chromemReady) {
                BaseConfig.initChromem();
                if (!BaseConfig.chromemReady) {
                    return '向量数据库未就绪，删除失败，请稍后重试';
                }
            }
            const [result, error] = chromemDelete(id.trim());
            if (error) {
                console.error('[编纂者] chromem 删除失败:', error);
                return `向量数据库删除失败: ${error}`;
            }
            return result ? `记录 ${id} 已成功从向量数据库删除` : `删除操作已完成但未返回确认信息`;
        }
        handleStoreRecord(content) {
            if (!content || content.trim().length === 0) {
                return '记录内容为空，已跳过存储';
            }
            if (!BaseConfig.chromemReady) {
                BaseConfig.initChromem();
                if (!BaseConfig.chromemReady) {
                    return '向量数据库未就绪，存储失败，请稍后重试';
                }
            }
            const trimmedContent = content.trim();
            const topicMatch = trimmedContent.match(/话题[:：](.+)/);
            const eventMatch = trimmedContent.match(/事件[:：](.+?)[|｜]/);
            const checkQuery = topicMatch ? topicMatch[1].trim() : eventMatch ? eventMatch[1].trim() : trimmedContent.slice(0, 50);
            if (checkQuery) {
                const [existingResults] = chromemQuery(checkQuery, 5);
                if (existingResults && existingResults.length > 0) {
                    const similarRecords = existingResults
                        .map((r, i) => `[相似记录${i + 1}] ID:${r.id} | 内容:${r.content}`)
                        .join('\n');
                    console.warn('[编纂者] 存储前发现相似记录，建议合并而非新增');
                    return `⚠️ 检测到可能存在相似的历史记录，建议使用 merge_existing_record 合并而非新建：\n${similarRecords}\n\n如果确认这些记录与新内容无关，请再次调用 store_organized_record 并说明理由。`;
                }
            }
            const finalContent = this.ensureTimestampInRecord(trimmedContent);
            const [result, error] = chromemAdd('assistant', finalContent);
            if (error) {
                console.error('[编纂者] chromem 存储失败:', error);
                return `向量数据库存储失败: ${error}`;
            }
            return result ? '记录已成功存储到向量数据库' : '存储操作已完成但未返回确认信息';
        }
    }
    class OrganizeRole extends Toolchain {
        constructor() {
            super(fileView('prompts/organizeRole.md')[0]);
        }
        organizeHistoricalRecords() {
            console.log('[编纂者] 开始组织历史记录');
            if (OnlyData.unreadRecords.length === 0) {
                console.log('[编纂者] 没有未读记录需要整理');
                return;
            }
            if (!BaseConfig.chromemReady) {
                BaseConfig.initChromem();
                if (!BaseConfig.chromemReady) {
                    console.warn('[编纂者] 向量数据库未就绪，保留未读记录待下次整理');
                    return;
                }
            }
            try {
                const organizePrompt = this.buildOrganizePrompt(OnlyData.unreadRecords);
                this.coverContext({ role: 'user', content: organizePrompt });
                this.runtimeMessages = [
                    { role: 'user', content: `当前时间: ${new Date().toLocaleString()}` }
                ];
                this.executeOrganizeLoop();
                console.log('[编纂者] 历史记录组织完成');
                OnlyData.unreadRecords = [];
            }
            catch (error) {
                console.error('[编纂者] 组织历史记录失败，保留未读记录待下次重试:', error);
            }
        }
        persistDiscardedMessages(discarded) {
            console.log('[编纂者] 开始持久化被抛弃的消息');
            if (!BaseConfig.chromemReady)
                BaseConfig.initChromem();
            if (!BaseConfig.chromemReady)
                return;
            for (const message of discarded) {
                const content = typeof message.content === 'string' ? message.content : JSON.stringify(message.content);
                chromemAdd(message.role, content);
            }
        }
        queryHistoricalRecords(queryText, topK = 10) {
            if (!BaseConfig.chromemReady)
                BaseConfig.initChromem();
            if (!BaseConfig.chromemReady)
                return [];
            const [results, error] = chromemQuery(queryText, topK);
            if (error) {
                console.error('[编纂者] 查询历史记录失败:', error);
                return [];
            }
            if (!results || results.length === 0)
                return [];
            return results.map((r) => ({
                id: r.id,
                role: r.role,
                content: r.content
            }));
        }
        getHistoricalContext(maxResults = 5) {
            const records = this.queryHistoricalRecords('近期对话 重要事件', maxResults);
            if (records.length === 0)
                return '';
            return records.map(r => r.content).join('\n');
        }
        executeOrganizeLoop() {
            const MAX_ITERATIONS = 8;
            for (let i = 0; i < MAX_ITERATIONS; i++) {
                console.log(`[编纂者] 第 ${i + 1} 轮模型推理`);
                let response;
                try {
                    response = this.run([], this.organizeTools);
                }
                catch (error) {
                    console.error(`[编纂者] 第 ${i + 1} 轮推理失败:`, error);
                    break;
                }
                const choice = response.body?.choices?.[0];
                if (!choice) {
                    console.log('[编纂者] 模型返回空结果，结束循环');
                    break;
                }
                const toolCalls = choice.message?.tool_calls;
                if (!toolCalls || toolCalls.length === 0) {
                    const replyContent = choice.message?.content || '';
                    console.log('[编纂者] 模型完成整理:', replyContent.slice(0, 300));
                    if (replyContent) {
                        this.writeContext(choice.message);
                    }
                    break;
                }
                console.log(`[编纂者] 第 ${i + 1} 轮工具调用, 共 ${toolCalls.length} 个工具`);
                this.writeContext(choice.message);
                for (const toolCall of toolCalls) {
                    const result = this.executeOrganizeTool(toolCall);
                    this.writeContext({
                        role: 'tool',
                        content: result,
                        tool_call_id: toolCall.id
                    });
                }
            }
        }
    }

    class AgentDefine {
        queryKeywords = new ModelBuilder(fileView('prompts/queryKeywords.md')[0]);
        emotionManager = new ModelBuilder(fileView('prompts/emotionManager.md')[0]);
        recorderRole = new ModelBuilder(fileView('prompts/recorderRole.md')[0]);
        summaryRole = new ModelBuilder(fileView('prompts/summaryRole.md')[0]);
        descriptionRole = new ModelBuilder(fileView('prompts/descriptionRole.md')[0]);
        dialogueRole = new DialogueRole();
        painterRole = new PainterRole();
        organizeRole = new OrganizeRole();
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
            return this.defaultAnswers[RandomFloor(0, this.defaultAnswers.length - 1)];
        }
        constructor() {
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
                const summaryRequest = this.descriptionRole.run([], []);
                const summary = summaryRequest.body?.choices?.[0]?.message?.content;
                if (summary && summary.trim().length > 0)
                    sandboxMessages.push({ type: 'text', text: summary });
            }
            if (sandboxMessages.length > 1) {
                this.summaryRole.coverContext({ role: 'user', content: sandboxMessages });
                const summaryRequest = this.summaryRole.run([], []);
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
            await this.dialogueRole.callMultimediaAndToolParsing(cache, this);
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
                    this.painterRole.createImageRendering(this);
                    await this.createChatMessage();
                    if (!this.finalResponse.trim().length)
                        throw new Error('消息响应为空');
                    else
                        errorCount = 0;
                    if (OnlyData.unreadRecords.length > 10) {
                        setTimeout(() => this.organizeRole.organizeHistoricalRecords(), 0);
                    }
                    const cleanedResponse = cleanTextForTTS(this.finalResponse);
                    if (!cleanedResponse.trim().length)
                        throw new Error('清洗后的文本为空');
                    const chunks = splitSentences(cleanedResponse);
                    for (const chunk of chunks) {
                        let audio = '';
                        try {
                            const [audioData, err] = tts(chunk);
                            if (!err && audioData)
                                audio = audioData;
                        }
                        catch (e) {
                            console.error(`TTS合成异常: [${chunk}]`, e);
                        }
                        pushContext(messageType, chunk, audio);
                    }
                }
                catch (error) {
                    const [promptSound, , , readErr] = readFile('audios/cartoon-fail.mp3');
                    if (readErr)
                        console.error('读取提示音失败:', readErr);
                    console.error(error.message, ' || ', error.stack);
                    errorCount++;
                    pushContext('active', this.randomDefaultMessage, promptSound);
                    if (errorCount >= 3) {
                        this.resetAgentState();
                        errorCount = 0;
                        continue;
                    }
                }
            }
        }
        resetAgentState() {
            this.queryKeywords.coverContext([]);
            this.emotionManager.coverContext([]);
            this.recorderRole.coverContext([]);
            this.summaryRole.coverContext([]);
            this.descriptionRole.coverContext([]);
            this.dialogueRole.coverContext([]);
            this.painterRole.coverContext([]);
            this.organizeRole.coverContext([]);
            this.unreadContext = [];
            this.unreadVideoUrl = [];
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
            text: '你好呀~'
        }
    ];
    AgentExample.testMessageWrite('user', message, 1500);

    function cleanTextForTTS(text) {
        if (!text)
            return '';
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
    function splitSentences(text) {
        if (!text)
            return [];
        const TARGET_LENGTH = 30;
        const PUNCTUATION = /[。？！…，、；：,;:\.\?!]/;
        const sentences = [];
        let remaining = text;
        while (remaining.length > 0) {
            if (remaining.length <= TARGET_LENGTH * 1.5) {
                sentences.push(remaining.trim());
                break;
            }
            let splitPos = -1;
            const searchEnd = Math.min(remaining.length, TARGET_LENGTH + Math.floor(TARGET_LENGTH * 0.5));
            for (let i = TARGET_LENGTH; i < searchEnd; i++) {
                if (PUNCTUATION.test(remaining[i])) {
                    splitPos = i + 1;
                    break;
                }
            }
            if (splitPos === -1) {
                const searchStart = Math.max(0, TARGET_LENGTH - Math.floor(TARGET_LENGTH * 0.5));
                for (let i = TARGET_LENGTH - 1; i >= searchStart; i--) {
                    if (PUNCTUATION.test(remaining[i])) {
                        splitPos = i + 1;
                        break;
                    }
                }
            }
            if (splitPos === -1 || splitPos === 0) {
                splitPos = TARGET_LENGTH;
            }
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
    function synthesizeSpeech(text) {
        const cleaned = cleanTextForTTS(text);
        if (!cleaned)
            return;
        const sentences = splitSentences(cleaned);
        if (sentences.length === 0)
            return;
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

    exports.AgentDefine = AgentDefine;
    exports.BaseConfig = BaseConfig;
    exports.CalculateMedian = CalculateMedian;
    exports.CalculateModes = CalculateModes;
    exports.Clamp = Clamp;
    exports.DialogueRole = DialogueRole;
    exports.FileToBase64 = FileToBase64;
    exports.ModelBuilder = ModelBuilder;
    exports.OnlyData = OnlyData;
    exports.OrganizeRole = OrganizeRole;
    exports.PainterRole = PainterRole;
    exports.RandomFloat = RandomFloat;
    exports.RandomFloor = RandomFloor;
    exports.ThinkType = ThinkType;
    exports.calculateFileHash = calculateFileHash;
    exports.cleanTextForTTS = cleanTextForTTS;
    exports.fetchDocumentCallback = fetchDocumentCallback;
    exports.getFileContent = getFileContent;
    exports.getPromptFromDatabase = getPromptFromDatabase;
    exports.queryFromDatabase = queryFromDatabase;
    exports.saveImageToServer = saveImageToServer;
    exports.savePromptToDatabase = savePromptToDatabase;
    exports.splitSentences = splitSentences;
    exports.splitTextToStrings = splitTextToStrings;
    exports.synthesizeSpeech = synthesizeSpeech;
    exports.toBtoaString = toBtoaString;

    return exports;

})({});
