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
        static LTPfunction = new Map();
        static LTPdefinition = [];
        static get systemUrl() {
            return url()[0] + '/v1';
        }
        ;
        static get fileServiceUrl() {
            return url()[0];
        }
        ;
        static get SystemKey() {
            return OnlyData.customConfig?.cloud?.cloud_model_key || 'key-520-1314-2000-02-18';
        }
        ;
        static get MultimodalName() {
            return OnlyData.customConfig?.cloud?.multimodal_model_name || "system-multimodal";
        }
        ;
        static get EmbeddingName() {
            return OnlyData.customConfig?.cloud?.embedding_model_name || "system-embedding";
        }
        ;
        static get userName() {
            return OnlyData.customConfig?.server?.user_name || "阁下";
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
            if (err)
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

    function queryFromKnowledge(operations, createTableOperation) {
        const requestBody = { operations, transaction: false };
        let [result, error] = knowledge(requestBody);
        if (error)
            throw new Error('知识库查询失败');
        if (!result.success || !result.results[0].success) {
            const errorMessage = result.error || result.results[0].error || '';
            if (errorMessage.includes('no such table') && createTableOperation) {
                const createTableRequest = { operations: [createTableOperation], transaction: false };
                let [createTableResult, tableError] = knowledge(createTableRequest);
                if (tableError)
                    throw tableError;
                if (!createTableResult.success)
                    throw new Error('创建表失败');
                [result, error] = knowledge(requestBody);
                if (error)
                    throw error;
                if (!result.success || !result.results[0].success)
                    throw new Error('知识库查询失败');
            }
            else
                throw new Error('知识库查询失败');
        }
        return result;
    }
    function getPromptFromKnowledge(key) {
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
            const result = queryFromKnowledge(operations, createTableOperation);
            if (result.success && result.results[0].success && result.results[0].rows) {
                return result.results[0].rows[0].Prompt;
            }
            return null;
        }
        catch (error) {
            return null;
        }
    }
    function savePromptToKnowledge(key, prompt) {
        try {
            const existingPrompt = getPromptFromKnowledge(key);
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
            const result = queryFromKnowledge(operations, createTableOperation);
            return result.success && result.results[0].success;
        }
        catch (error) {
            console.error('向知识库存储提示词失败:', error);
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
        static memoryReady = false;
        constructor() { }
        static initMemory() {
            if (BaseConfig.memoryReady)
                return;
            const [_, err] = memoryInit(OnlyData.systemUrl, OnlyData.SystemKey, OnlyData.EmbeddingName, 'lunar_messages');
            if (err)
                console.error('记忆库初始化失败:', err);
            else
                BaseConfig.memoryReady = true;
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
            const cleaned = this.stripReasoningContent(context);
            if (this.messages.length >= 20) {
                const discarded = this.messages.slice(0, this.messages.length - 19);
                this.messages = this.messages.slice(-19).concat(cleaned);
                OnlyData.unreadRecords.push(...discarded);
            }
            else
                this.messages.push(cleaned);
            return this;
        }
        stripReasoningContent(message) {
            if ('reasoning_content' in message) {
                const { reasoning_content, ...rest } = message;
                return rest;
            }
            return message;
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
            const userMessages = this.getLatestUserMessages();
            if (userMessages.length === 0)
                return this;
            if (!BaseConfig.memoryReady)
                BaseConfig.initMemory();
            if (!BaseConfig.memoryReady)
                return this;
            const allResults = [];
            for (const userMessage of userMessages) {
                const [results, error] = memoryQuery('lunar_messages', userMessage, 5);
                if (error) {
                    console.error('记忆库查询失败:', error);
                    continue;
                }
                if (results && results.length > 0) {
                    allResults.push(...results);
                }
            }
            if (allResults.length === 0)
                return this;
            const seen = new Map();
            for (const r of allResults) {
                const existing = seen.get(r.content);
                if (!existing || r.similarity > existing.similarity) {
                    seen.set(r.content, r);
                }
            }
            const uniqueResults = Array.from(seen.values()).sort((a, b) => b.similarity - a.similarity);
            console.log(`[RAG] 查询到 ${uniqueResults.length} 条相关消息，相似度范围: ${uniqueResults[0]?.similarity?.toFixed(4) ?? 'N/A'} ~ ${uniqueResults[uniqueResults.length - 1]?.similarity?.toFixed(4) ?? 'N/A'}`);
            this.ragMessages = uniqueResults.map(r => ({ role: r.role, content: r.content }));
            return this;
        }
        getLatestUserMessages() {
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
            return userTexts;
        }
        constructor(prompt) {
            super();
            this.systemPrompt = this.promptCompletion(prompt);
        }
    }

    class CreativeRoleBase extends ModelBuilder {
        _history = [];
        DIALOGUE_HISTORY_LIMIT = 15;
        OWN_HISTORY_LIMIT = 5;
        MAX_ITERATIONS = 3;
        UNREAD_CHECK_COUNT = 10;
        constructor(prompt) {
            super(prompt);
        }
        consumeHistory() {
            const result = [...this._history];
            this._history = [];
            return result;
        }
        createCreativeWork(dialogueMessages, unreadContext, count = this.UNREAD_CHECK_COUNT) {
            const dialogueHistory = dialogueMessages.slice(-this.DIALOGUE_HISTORY_LIMIT);
            const ownHistory = this._history.slice(-this.OWN_HISTORY_LIMIT);
            this.coverContext([...dialogueHistory, ...ownHistory, ...unreadContext]);
            const unreadTexts = this.extractUnreadTexts(unreadContext, count);
            if (!this.matchKeywords(unreadTexts))
                return true;
            const details = [];
            for (let i = 0; i < this.MAX_ITERATIONS; i++) {
                console.log(`[${this.roleName}] 第 ${i + 1} 轮推理`);
                let response;
                try {
                    response = this.run([], this.getToolDefinitions());
                }
                catch (error) {
                    console.error(`[${this.roleName}] 第 ${i + 1} 轮推理失败:`, error);
                    break;
                }
                const choice = response.body?.choices?.[0];
                if (!choice) {
                    console.log(`[${this.roleName}] 模型返回空结果，结束循环`);
                    break;
                }
                const toolCalls = choice.message?.tool_calls;
                if (!toolCalls || toolCalls.length === 0)
                    break;
                this.writeContext(choice.message);
                for (const toolCall of toolCalls) {
                    console.log(`[${this.roleName}] 执行工具: ${toolCall.function.name}`);
                    const result = this.executeTool(toolCall);
                    this.writeContext({ role: 'tool', content: result, tool_call_id: toolCall.id });
                    this.collectDetail(toolCall, details);
                }
            }
            if (details.length > 0) {
                const summary = this.buildSummary(details);
                this._history.push({ role: 'user', content: summary });
                console.log(`[${this.roleName}] 已将 ${details.length} 件作品详情写入历史`);
            }
            return false;
        }
        extractUnreadTexts(unreadContext, count) {
            const texts = [];
            for (const message of unreadContext.slice(-count)) {
                if (typeof message.content === 'string')
                    texts.push(message.content);
                else
                    message.content.forEach(item => { if (item.type === 'text')
                        texts.push(item.text); });
            }
            return texts;
        }
    }

    class DialogueRole extends ModelBuilder {
        async callMultimediaAndToolParsing(cache, source) {
            try {
                await source.LiteImageFile();
                source.researcherRole.consumeHistory().forEach(msg => source.unreadContext.push(msg));
                source.painterRole.consumeHistory().forEach(msg => source.unreadContext.push(msg));
                source.musicianRole.consumeHistory().forEach(msg => source.unreadContext.push(msg));
                source.unreadContext.forEach(context => this.writeContext(context));
                source.unreadContext = [];
                this.formatHistoricalMessages(source);
                this.runtimeMessages = [{ role: 'user', content: `当前时间: ${new Date().toLocaleString()}` }];
                this.queryRagMessages();
                const response = this.run(this.ragMessages, [...OnlyData.LTPdefinition]);
                this.analyzeMessageResponse(response.body, cache);
                if (cache.toolCalls.length > 0) {
                    this.writeContext(response.body.choices?.[0]?.message);
                    const hasProcessedToolCalls = await this.batchExecutionToolCall(cache);
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
            const isAudioMessage = (c) => c.type === 'input_audio';
            for (const message of this.messages) {
                if (typeof message.content !== 'string') {
                    if (message.content.some(isAudioMessage)) {
                        flattenedMessages.push(message);
                        continue;
                    }
                    for (const content of message.content) {
                        if (content.type === 'text')
                            flattenedMessages.push({ role: message.role, content: content.text });
                        else
                            flattenedMessages.push({ role: message.role, content: [content] });
                    }
                }
                else
                    flattenedMessages.push(message);
            }
            const visionCount = flattenedMessages.filter(m => { if (!Array.isArray(m.content) || m.content.some(isAudioMessage))
                return false; }).length;
            if (visionCount <= 10)
                this.messages = flattenedMessages;
            else {
                const processedMessages = [];
                let visionBuffer = [];
                for (const message of flattenedMessages) {
                    const isVisionMessage = Array.isArray(message.content) && !message.content.some(isAudioMessage);
                    if (isVisionMessage)
                        visionBuffer.push(message);
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
            const latestRole = this.messages.slice(-1)[0].role;
            if (latestRole === 'user' || latestRole === 'tool')
                return;
            const continuationPrompts = [
                '请延续当前话题，继续展开讨论。',
                '请完善当前话题，对已有内容进行补充和优化。',
                '请将话题转向旅行，聊聊旅行相关的见闻或计划。',
                '请将话题转向游戏，聊聊最近有趣的游戏体验。',
                '请将话题转向音乐，聊聊最近在听的音乐或音乐推荐。',
                '请将话题转向电影，聊聊最近看过或想看的电影。',
                '请将话题转向书籍，聊聊最近在读或推荐的书籍。',
                '请将话题转向动漫，聊聊最近在追或推荐的动漫。',
            ];
            const prompt = continuationPrompts[Math.floor(Math.random() * continuationPrompts.length)];
            this.writeContext({ role: 'user', content: prompt });
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
        analyzeMessageResponse(message, cache) {
            try {
                if (message.choices?.[0]?.message?.reasoning_content) {
                    cache.thinkingContent = message.choices[0].message.reasoning_content;
                }
                if (message.timings?.predicted_per_second) {
                    console.log(`词元生成速度: ${message.timings.predicted_per_second}`);
                }
                if (message.timings?.cache_n !== undefined) {
                    console.log(`缓存命中数量: ${message.timings.cache_n}`);
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
        async batchExecutionToolCall(state) {
            let hasToolCalls = false;
            for (const toolCall of state.toolCalls) {
                const functionName = toolCall.function.name;
                const functionArgs = toolCall.function.arguments;
                const lunarToolPackage = OnlyData.LTPfunction.get(functionName);
                if (!lunarToolPackage) {
                    this.messages.push({ role: "tool", content: `未找到工具包: ${functionName}`, tool_call_id: toolCall.id });
                    continue;
                }
                try {
                    const toolResult = await lunarToolPackage(functionArgs);
                    const textContent = Array.isArray(toolResult) ? toolResult[0] : String(toolResult);
                    const base64Image = Array.isArray(toolResult) ? toolResult[1] : '';
                    this.messages.push({ role: "tool", content: textContent, tool_call_id: toolCall.id });
                    if (base64Image && typeof base64Image === 'string' && base64Image.length > 0) {
                        this.messages.push({ role: "tool", content: [{ type: "image_url", image_url: { url: base64Image } }], tool_call_id: toolCall.id });
                        console.log(`[工具调用] ${functionName} 返回图片数据，长度=${base64Image.length} 字节`);
                    }
                    hasToolCalls = true;
                }
                catch (error) {
                    this.messages.push({ role: "tool", content: `调用${functionName}失败: ${error}`, tool_call_id: toolCall.id });
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

    class PainterRole extends CreativeRoleBase {
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
        defaultOutfitPrompt = '穿着宽松的奶油白色针织连帽拉链外套，敞开拉链，里面是纯白色圆领T恤，高腰深蓝和白色格纹百褶迷你裙，侧腰位置悬挂着白色和深蓝的大缎带蝴蝶结，饰有圆润的白色珍珠装饰和金色高光，白色短袜，黑色系带低帮帆布鞋';
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
                            "outfit": {
                                type: "string",
                                description: "服装提示词,描述想要穿着的服装样式。如果不提供则使用默认服装"
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
        imageKeywords = [
            /画(?:一(?:张|幅|个))?(?:图|画|图片|图像|插画|插图)/,
            /生成(?:一(?:张|幅|个))?(?:图|画|图片|图像|插画|插图)/,
            /绘制(?:一(?:张|幅|个))?(?:图|画|图片|图像|插画|插图)/,
            /创作(?:一(?:张|幅|个))?(?:图|画|图片|图像|插画|插图)/,
            /(?:帮我|给我|为我)(?:画|绘制|生成|创作|做|弄|整)(?:一(?:张|幅|个))?(?:图|画|图片|图像|插画|插图)?/,
            /(?:做|弄|整)(?:一(?:张|幅|个))?(?:图|画|图片|图像|插画|插图)/,
            /来(?:一(?:张|幅|个))?(?:图|画|图片|图像|插画|插图)/,
            /自画像/,
            /画(?:一(?:张|幅|个))?自画像/,
        ];
        constructor() {
            super(fileView('prompts/painterRole.md')[0]);
        }
        get roleName() { return '画家'; }
        matchKeywords(texts) {
            return texts.some(text => this.imageKeywords.some(keyword => keyword.test(text)));
        }
        getToolDefinitions() { return this.roleTool; }
        executeTool(toolCall) {
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
        collectDetail(toolCall, paintings) {
            try {
                const args = typeof toolCall.function.arguments === 'string'
                    ? JSON.parse(toolCall.function.arguments)
                    : toolCall.function.arguments;
                if (toolCall.function.name === 'self_portrait') {
                    paintings.push({
                        toolName: 'self_portrait',
                        promptSummary: '自画像',
                        expression: args.expression || '',
                        posture: args.posture || '',
                        environment: args.environment || '',
                    });
                }
                else if (toolCall.function.name === 'diffusion_generation') {
                    const prompt = args.prompt || '';
                    paintings.push({
                        toolName: 'diffusion_generation',
                        promptSummary: prompt.length > 100 ? prompt.slice(0, 97) + '...' : prompt,
                    });
                }
            }
            catch {
            }
        }
        buildSummary(paintings) {
            const parts = [];
            parts.push('[绘画创作记录] 你（月华）刚刚完成了以下图像作品创作：');
            for (let i = 0; i < paintings.length; i++) {
                const p = paintings[i];
                const detailLines = [];
                if (p.toolName === 'self_portrait') {
                    detailLines.push(`作品${i + 1}：自画像`);
                    if (p.expression)
                        detailLines.push(`  - 表情：${p.expression}`);
                    if (p.posture)
                        detailLines.push(`  - 姿势：${p.posture}`);
                    if (p.environment)
                        detailLines.push(`  - 环境：${p.environment}`);
                }
                else {
                    detailLines.push(`作品${i + 1}：扩散生成图像`);
                    detailLines.push(`  - 画面内容：${p.promptSummary}`);
                }
                parts.push(detailLines.join('\n'));
            }
            parts.push('\n注意：请基于以上真实创作信息向用户介绍图像作品，切勿编造画面内容。图像已通过前端推送给用户。');
            return parts.join('\n');
        }
        writeAppearancePrompt(expression, posture, outfit, environment) {
            const currentExpression = expression || this.defaultExpressionPrompt[RandomFloor(0, this.defaultExpressionPrompt.length - 1)];
            const currentPosture = posture || this.defaultPosturePrompt[RandomFloor(0, this.defaultPosturePrompt.length - 1)];
            const currentOutfit = outfit || this.defaultOutfitPrompt;
            return this.selfAppearancePrompt.replace('{expression}', currentExpression).replace('{posture}', currentPosture).replace('{outfit}', currentOutfit).replace('{environment}', environment || '');
        }
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
                console.log(`服装: "${args.outfit}"`);
                console.log(`环境: "${args.environment}"`);
                console.log(`负面提示词: "${args.negative_prompt}"`);
                console.log(`提示词引导系数: "${args.cfg_scale}"`);
                const fullPrompt = this.writeAppearancePrompt(args.expression, args.posture, args.outfit, args.environment);
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
    }

    class MusicianRole extends CreativeRoleBase {
        musicTool = [
            {
                type: "function",
                function: {
                    name: "compose_music",
                    description: "创作音乐作品并生成ABC记谱法格式的乐谱。ABC记谱法是一种基于文本的音乐记谱格式，使用字母表示音符。请务必生成完整的、可直接播放的ABC乐谱，确保音符时值、小节线和调号正确。",
                    parameters: {
                        type: "object",
                        properties: {
                            "title": {
                                type: "string",
                                description: "音乐作品标题"
                            },
                            "instruments": {
                                type: "string",
                                description: "使用的乐器列表，多个乐器用逗号分隔，如'钢琴,小提琴'"
                            },
                            "tempo": {
                                type: "number",
                                description: "演奏速度（BPM），默认120"
                            },
                            "structure": {
                                type: "string",
                                description: "音乐段落结构描述，例如：'前奏(4小节)-主旋律(8小节)-副歌(8小节)-尾声(4小节)'"
                            },
                            "key": {
                                type: "string",
                                description: "调式，如 C、G、D、F、a、e、d"
                            },
                            "meter": {
                                type: "string",
                                description: "拍号，如 4/4、3/4、6/8"
                            },
                            "abc_notation": {
                                type: "string",
                                description: `ABC记谱法格式的完整乐谱。必须严格遵循ABC记谱法规范：

标题行格式:
X:1
T:作品标题
M:拍号
L:默认音符时值(如 1/8 表示八分音符)
Q:速度标记(如 1/4=120)
K:调号

音符与音高: 使用CDEFGAB表示音名，小写字母表示高八度，后面跟逗号表示低八度(如 C, D, E,)。升半音用^前缀(如 ^C)，降半音用_前缀(如 _B)。
时值: 数字后缀表示时值倍数，如 C2 表示两倍时值的C音，C/2 表示一半时值。
小节线: | 分隔小节，|| 表示双小节线，|] 表示结束。
休止符: z 表示休止符，时值规则同音符。

示例:
X:1
T:月光小夜曲
M:4/4
L:1/8
Q:1/4=100
K:C
C2 E2 G2 c'2 | e'2 d'2 c'2 G2 | E2 C2 D2 E2 | C8 |]`
                            }
                        },
                        required: [
                            "title",
                            "abc_notation"
                        ]
                    }
                }
            }
        ];
        musicKeywords = [
            /创作(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|乐谱|钢琴曲|古典乐|轻音乐)/,
            /生成(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|乐谱|钢琴曲|古典乐|轻音乐)/,
            /写(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|乐谱|钢琴曲|古典乐|轻音乐)/,
            /制作(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|乐谱|钢琴曲|古典乐|轻音乐)/,
            /编(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|乐谱|曲|钢琴曲|古典乐|轻音乐)/,
            /(?:帮我|给我|为我)(?:创作|生成|写|制作|编|做|弄|整)(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|乐谱|钢琴曲|古典乐|轻音乐)?/,
            /(?:做|弄|整)(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|乐谱|钢琴曲|古典乐|轻音乐)/,
            /来(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|乐谱|钢琴曲|古典乐|轻音乐)/,
            /作曲/,
            /编曲/,
            /谱写/,
            /演奏(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|钢琴曲|古典乐|轻音乐)/,
            /(?:弹|拉|吹)(?:一(?:首|段|曲))?.*(?:钢琴|小提琴|吉他|笛子|古筝|曲子|音乐|旋律)/,
        ];
        constructor() {
            super(fileView('prompts/musicianRole.md')[0]);
        }
        get roleName() { return '音乐家'; }
        matchKeywords(texts) {
            return texts.some(text => this.musicKeywords.some(keyword => keyword.test(text)));
        }
        getToolDefinitions() { return this.musicTool; }
        executeTool(toolCall) {
            const funcName = toolCall.function.name;
            let args = {};
            try {
                args = typeof toolCall.function.arguments === 'string'
                    ? JSON.parse(toolCall.function.arguments)
                    : toolCall.function.arguments;
            }
            catch (parseError) {
                console.error(`[音乐家] 工具调用参数解析失败:`, toolCall.function.arguments);
                return `工具调用参数解析失败，请确保传入合法的 JSON 字符串。错误: ${parseError}`;
            }
            switch (funcName) {
                case 'compose_music': return this.handleComposeMusic(args);
                default: return `未知工具: ${funcName}，可用工具为 compose_music`;
            }
        }
        collectDetail(toolCall, pieces) {
            try {
                const args = typeof toolCall.function.arguments === 'string'
                    ? JSON.parse(toolCall.function.arguments)
                    : toolCall.function.arguments;
                if (args.title) {
                    pieces.push({
                        title: args.title,
                        instruments: args.instruments || '',
                        tempo: args.tempo || 0,
                        structure: args.structure || '',
                        key: args.key || '',
                        meter: args.meter || '',
                        abcLength: (args.abc_notation || '').length,
                    });
                }
            }
            catch {
            }
        }
        buildSummary(pieces) {
            const parts = [];
            parts.push('[音乐创作记录] 你（月华）刚刚完成了以下音乐作品创作：');
            for (let i = 0; i < pieces.length; i++) {
                const p = pieces[i];
                const detailLines = [];
                detailLines.push(`作品${i + 1}：《${p.title}》`);
                if (p.instruments)
                    detailLines.push(`  - 乐器配置：${p.instruments}`);
                if (p.key)
                    detailLines.push(`  - 调式：${p.key}${p.key === p.key.toLowerCase() ? '小调' : '大调'}`);
                if (p.tempo > 0)
                    detailLines.push(`  - 速度：${p.tempo} BPM`);
                if (p.meter)
                    detailLines.push(`  - 拍号：${p.meter}`);
                if (p.structure)
                    detailLines.push(`  - 段落结构：${p.structure}`);
                detailLines.push(`  - 乐谱长度：${p.abcLength} 字符`);
                parts.push(detailLines.join('\n'));
            }
            parts.push('\n注意：请基于以上真实创作信息向用户介绍音乐作品，切勿编造不存在的曲名、乐器或结构。乐谱已通过音乐播放器推送给用户，可以引导用户查看和播放。');
            return parts.join('\n');
        }
        handleComposeMusic(args) {
            try {
                const title = args.title || '未命名作品';
                const abcNotation = args.abc_notation || '';
                const instruments = (args.instruments || '').trim();
                console.log(`[音乐家] 创作音乐: "${title}"`);
                if (instruments)
                    console.log(`  乐器: ${instruments}`);
                if (args.tempo)
                    console.log(`  速度: ${args.tempo} BPM`);
                if (args.structure)
                    console.log(`  结构: ${args.structure}`);
                if (!abcNotation.trim()) {
                    return '音乐创作失败：ABC记谱法乐谱为空';
                }
                const enrichedAbc = this.injectInstrumentDirective(abcNotation, instruments);
                const hasX = /^X:\s*\d+/m.test(enrichedAbc);
                const hasT = /^T:\s*.+/m.test(enrichedAbc);
                const hasK = /^K:\s*.+/m.test(enrichedAbc);
                if (!hasX || !hasK) {
                    console.warn('[音乐家] ABC乐谱缺少必要字段 (X:/K:)，尝试自动补充');
                    let fixedAbc = enrichedAbc;
                    if (!hasX)
                        fixedAbc = 'X:1\n' + fixedAbc;
                    if (!hasT)
                        fixedAbc = fixedAbc.replace(/^(X:\s*\d+\n)/m, `$1T:${title}\n`);
                    if (!hasK)
                        fixedAbc = fixedAbc.replace(/^(T:.*\n)/m, `$1K:C\n`);
                    const pushSuccess = pushContext('music', fixedAbc, '');
                    if (!pushSuccess) {
                        console.warn('[音乐家] 推送乐谱到前端失败');
                    }
                    return `音乐作品"${title}"创作成功（已自动补全格式）。乐谱已推送到前端进行渲染播放。`;
                }
                const pushSuccess = pushContext('music', enrichedAbc, '');
                if (!pushSuccess) {
                    console.warn('[音乐家] 推送乐谱到前端失败');
                }
                console.log(`[音乐家] 乐谱推送成功，长度: ${enrichedAbc.length} 字符，乐器: ${instruments || '默认'}`);
                return `音乐作品"${title}"创作成功。乐谱已推送到前端进行渲染播放。`;
            }
            catch (error) {
                console.error('[音乐家] 音乐创作处理异常:', error);
                return `音乐创作异常: ${error}`;
            }
        }
        injectInstrumentDirective(abcNotation, instruments) {
            if (!instruments)
                return abcNotation;
            const cleaned = instruments
                .replace(/，/g, ',')
                .split(',')
                .map(s => s.trim())
                .filter(Boolean)
                .join(',');
            if (!cleaned)
                return abcNotation;
            if (/^%%instrument/m.test(abcNotation))
                return abcNotation;
            const directive = `%%instrument ${cleaned}\n`;
            const xMatch = abcNotation.match(/^X:\s*\d+/m);
            if (xMatch && xMatch.index !== undefined) {
                const before = abcNotation.substring(0, xMatch.index);
                const after = abcNotation.substring(xMatch.index);
                return before + directive + after;
            }
            return directive + abcNotation;
        }
    }

    class ResearcherRole extends ModelBuilder {
        _history = [];
        DIALOGUE_HISTORY_LIMIT = 15;
        OWN_HISTORY_LIMIT = 5;
        MAX_ITERATIONS = 5;
        UNREAD_CHECK_COUNT = 10;
        webSearchInitialized = false;
        assemblyMemoryInjected = false;
        researchTools = [
            {
                type: "function",
                function: {
                    name: "web_search",
                    description: "执行网络搜索，获取实时信息。当需要联网获取实时数据、最新资讯、事实查询等信息时使用。支持四种模式：simple（轻量摘要）、webpage（网页搜索，默认）、depth（深度研究，子问题拆解并行搜索）、assembly（大会辩论式深度研究，维新派vs守旧派多轮辩论，综合网络与记忆库信息生成报告，适合复杂争议性问题）。",
                    parameters: {
                        type: "object",
                        properties: {
                            query: {
                                type: "string",
                                description: "搜索查询关键词或问题"
                            },
                            mode: {
                                type: "string",
                                description: "搜索模式：simple（轻量摘要）、webpage（网页搜索，默认）、depth（深度研究）或 assembly（大会辩论式深度研究，综合网络搜索与记忆库）",
                                enum: ["simple", "webpage", "depth", "assembly"]
                            }
                        },
                        required: ["query"]
                    }
                }
            },
            {
                type: "function",
                function: {
                    name: "memory_query",
                    description: "查询内部记忆库中的历史对话和事件记录。用于回忆过去的对话内容、查找用户偏好、追溯历史事件等需要从内部记忆中检索信息的场景。",
                    parameters: {
                        type: "object",
                        properties: {
                            query: {
                                type: "string",
                                description: "查询文本，用于在记忆库中搜索相关记录"
                            },
                            top_k: {
                                type: "number",
                                description: "返回的最相关结果数量，默认10"
                            }
                        },
                        required: ["query"]
                    }
                }
            },
            {
                type: "function",
                function: {
                    name: "process_links",
                    description: "处理消息中的链接。自动识别链接类型：网页链接抓取内容并总结、图片链接使用视觉模型识别、下载链接自动下载文件。处理后将原始链接替换为摘要标签。",
                    parameters: {
                        type: "object",
                        properties: {
                            text: {
                                type: "string",
                                description: "包含链接的文本内容，工具会自动提取并处理其中的所有链接"
                            }
                        },
                        required: ["text"]
                    }
                }
            }
        ];
        researchKeywords = [
            /查(?:一查|一下|询|找|找找|看看)/,
            /搜索/,
            /搜(?:一搜|一下)/,
            /搜索(?:一搜|一下)/,
            /(?:帮我|给我|为我|替我)(?:查|搜索|找|调查|研究|检索|查询)/,
            /研究(?:一(?:下|研究))/,
            /调查(?:一(?:下|调查))?/,
            /思考(?:一(?:下|思考))?/,
            /回忆(?:一(?:下|回忆))?/,
            /想想?(?:看|起|到)/,
            /记不记得/,
            /还记得/,
            /以前(?:说过|聊过|讨论过|提过|提到)/,
            /上次(?:说|聊|讨论|提|提到)/,
            /了解(?:一(?:下|了解))?/,
            /(?:是|到底(?:是)|究竟(?:是))什么/,
            /(?:怎么|为什么|怎么回事)/,
            /(?:最新|最近|当前|目前|今天|现在).*(?:消息|新闻|情况|状态|动态|信息|数据)/,
            /(?:有没有|是否).*(?:相关|关于)/,
            /深入(?:了解|分析|研究)/,
            /详细(?:了解|分析|说明|解释)/,
            /分析(?:一(?:下|分析))?/,
            /核实/,
            /验证/,
            /(?:真|假|正确|错误|靠谱|可靠)/,
            /(?:资料|文献|论文|报告|数据|统计)/,
        ];
        constructor() {
            super(fileView('prompts/researcherRole.md')[0]);
        }
        consumeHistory() {
            const result = [...this._history];
            this._history = [];
            return result;
        }
        executeResearch(dialogueMessages, unreadContext, count = this.UNREAD_CHECK_COUNT) {
            const dialogueHistory = dialogueMessages.slice(-this.DIALOGUE_HISTORY_LIMIT);
            const ownHistory = this._history.slice(-this.OWN_HISTORY_LIMIT);
            this.coverContext([...dialogueHistory, ...ownHistory, ...unreadContext]);
            const unreadTexts = this.extractUnreadTexts(unreadContext, count);
            if (!this.matchKeywords(unreadTexts))
                return true;
            const details = [];
            for (let i = 0; i < this.MAX_ITERATIONS; i++) {
                console.log(`[研究者] 第 ${i + 1} 轮推理`);
                let response;
                try {
                    response = this.run([], this.researchTools);
                }
                catch (error) {
                    console.error(`[研究者] 第 ${i + 1} 轮推理失败:`, error);
                    break;
                }
                const choice = response.body?.choices?.[0];
                if (!choice) {
                    console.log(`[研究者] 模型返回空结果，结束循环`);
                    break;
                }
                const toolCalls = choice.message?.tool_calls;
                if (!toolCalls || toolCalls.length === 0)
                    break;
                this.writeContext(choice.message);
                for (const toolCall of toolCalls) {
                    console.log(`[研究者] 执行工具: ${toolCall.function.name}`);
                    const result = this.executeTool(toolCall);
                    this.writeContext({ role: 'tool', content: result, tool_call_id: toolCall.id });
                    this.collectDetail(toolCall, details);
                }
            }
            if (details.length > 0) {
                const report = this.synthesizeReport();
                this._history.push({ role: 'user', content: report });
                console.log(`[研究者] 已将研究报告写入历史（${details.length} 条工具调用记录）`);
            }
            return false;
        }
        matchKeywords(texts) {
            return texts.some(text => this.researchKeywords.some(keyword => keyword.test(text)));
        }
        executeTool(toolCall) {
            const funcName = toolCall.function.name;
            let args = {};
            try {
                args = typeof toolCall.function.arguments === 'string'
                    ? JSON.parse(toolCall.function.arguments)
                    : toolCall.function.arguments;
            }
            catch (parseError) {
                console.error(`[研究者] 工具调用参数解析失败:`, toolCall.function.arguments);
                return `工具调用参数解析失败，请确保传入合法的 JSON 字符串。错误: ${parseError}`;
            }
            switch (funcName) {
                case 'web_search': return this.handleWebSearch(args);
                case 'memory_query': return this.handleMemoryQuery(args);
                case 'process_links': return this.handleProcessLinks(args);
                default: return `未知工具: ${funcName}，可用工具为 web_search、memory_query 和 process_links`;
            }
        }
        handleWebSearch(args) {
            try {
                const query = args.query || '';
                if (!query.trim())
                    return '搜索失败：查询关键词不能为空';
                const mode = args.mode || 'webpage';
                if (!this.webSearchInitialized) {
                    const initResult = this.initWebSearch();
                    if (!initResult)
                        return '搜索失败：网络检索子系统初始化失败';
                }
                console.log(`[研究者] 网络搜索: query="${query}", mode="${mode}"`);
                let result;
                let error = null;
                switch (mode) {
                    case 'assembly':
                        if (!this.assemblyMemoryInjected) {
                            this.injectMemoryProvider();
                        }
                        [result, error] = webSearchAssembly(query.trim());
                        break;
                    case 'depth':
                        [result, error] = webSearchDepth(query.trim());
                        break;
                    case 'webpage':
                        [result, error] = webSearchWebpage(query.trim());
                        break;
                    case 'simple':
                    default:
                        [result, error] = webSearchSimple(query.trim());
                        break;
                }
                if (error) {
                    console.error(`[研究者] 网络搜索失败: ${error.message || String(error)}`);
                    return `搜索失败：${error.message || String(error)}`;
                }
                const textResult = result || '未找到相关搜索结果';
                console.log(`[研究者] 搜索结果长度: ${textResult.length} 字符`);
                return textResult;
            }
            catch (error) {
                console.error('[研究者] 网络搜索处理异常:', error);
                return `网络搜索异常: ${error}`;
            }
        }
        handleMemoryQuery(args) {
            try {
                const query = args.query || '';
                if (!query.trim())
                    return '查询失败：查询文本不能为空';
                const topK = args.top_k || 10;
                if (!BaseConfig.memoryReady) {
                    BaseConfig.initMemory();
                    if (!BaseConfig.memoryReady)
                        return '查询失败：记忆库未就绪';
                }
                console.log(`[研究者] 记忆库查询: query="${query}", topK=${topK}`);
                const [results, error] = memoryQuery('lunar_messages', query.trim(), topK);
                if (error) {
                    console.error(`[研究者] 记忆库查询失败: ${error}`);
                    return `记忆库查询失败：${error}`;
                }
                if (!results || results.length === 0)
                    return '记忆库中未找到相关记录';
                const formattedResults = results.map((r, i) => `[记录${i + 1}] 相似度:${(r.similarity * 100).toFixed(1)}% | 内容:${r.content}`).join('\n');
                console.log(`[研究者] 查询到 ${results.length} 条相关记录`);
                return formattedResults;
            }
            catch (error) {
                console.error('[研究者] 记忆库查询处理异常:', error);
                return `记忆库查询异常: ${error}`;
            }
        }
        handleProcessLinks(args) {
            try {
                const text = args.text || '';
                if (!text.trim())
                    return '处理失败：文本内容不能为空';
                if (!this.webSearchInitialized) {
                    const initResult = this.initWebSearch();
                    if (!initResult)
                        return '处理失败：网络检索子系统初始化失败';
                }
                console.log(`[研究者] 处理链接: 文本长度=${text.length}`);
                const [replacedText, descriptions, error] = webSearchProcessLinks(text);
                if (error) {
                    console.error(`[研究者] 链接处理失败: ${error}`);
                    return `链接处理失败：${error}`;
                }
                if (!descriptions || descriptions.length === 0)
                    return '未检测到链接';
                const result = `替换后文本:\n${replacedText}\n\n链接详情:\n${descriptions.join('\n')}`;
                console.log(`[研究者] 处理了 ${descriptions.length} 个链接`);
                return result;
            }
            catch (error) {
                console.error('[研究者] 链接处理异常:', error);
                return `链接处理异常: ${error}`;
            }
        }
        initWebSearch() {
            if (this.webSearchInitialized)
                return true;
            try {
                const [success, err] = webSearchInit(OnlyData.systemUrl, OnlyData.SystemKey, OnlyData.MultimodalName, 4096, 0.7);
                if (err) {
                    console.error('[研究者] 网络检索初始化失败:', err);
                    return false;
                }
                this.webSearchInitialized = true;
                console.log('[研究者] 网络检索子系统初始化成功');
                return true;
            }
            catch (e) {
                console.error('[研究者] 网络检索初始化异常:', e);
                return false;
            }
        }
        injectMemoryProvider() {
            try {
                const [success, err] = webSearchSetMemoryProvider();
                if (err) {
                    console.warn('[研究者] 注入记忆库提供者失败:', err);
                    return;
                }
                this.assemblyMemoryInjected = true;
                console.log('[研究者] 已为大会辩论模式注入记忆库提供者');
            }
            catch (e) {
                console.warn('[研究者] 注入记忆库提供者异常:', e);
            }
        }
        collectDetail(toolCall, details) {
            try {
                const args = typeof toolCall.function.arguments === 'string'
                    ? JSON.parse(toolCall.function.arguments)
                    : toolCall.function.arguments;
                const query = args.query || args.text || '';
                const keyFindings = query ? `查询: ${query}` : '';
                if (toolCall.function.name === 'web_search') {
                    details.push({
                        toolName: 'web_search',
                        query,
                        mode: args.mode || 'webpage',
                        keyFindings,
                    });
                }
                else if (toolCall.function.name === 'memory_query') {
                    details.push({
                        toolName: 'memory_query',
                        query,
                        keyFindings,
                    });
                }
                else if (toolCall.function.name === 'process_links') {
                    details.push({
                        toolName: 'process_links',
                        query,
                        keyFindings,
                    });
                }
            }
            catch {
            }
        }
        synthesizeReport() {
            try {
                const response = this.run([], []);
                const content = response.body?.choices?.[0]?.message?.content || '';
                if (content.trim().length === 0) {
                    console.warn('[研究者] 综合分析返回空结果');
                    return '[研究报告] 研究过程中未能生成有效报告，请稍后重试。';
                }
                return content;
            }
            catch (error) {
                console.error('[研究者] 综合分析失败:', error);
                return '[研究报告] 研究报告生成失败，请稍后重试。';
            }
        }
        extractUnreadTexts(unreadContext, count) {
            const texts = [];
            for (const message of unreadContext.slice(-count)) {
                if (typeof message.content === 'string')
                    texts.push(message.content);
                else
                    message.content.forEach(item => { if (item.type === 'text')
                        texts.push(item.text); });
            }
            return texts;
        }
    }

    class Prompt extends ModelBuilder {
        currentLocation = '';
        getCurrentLocation() {
            if (this.currentLocation)
                return this.currentLocation;
            const [addressResult, error] = address();
            if (error || !addressResult || addressResult.length === 0) {
                this.currentLocation = '未知地点';
            }
            else {
                this.currentLocation = addressResult.join(' ');
            }
            return this.currentLocation;
        }
        getCurrentTime() {
            return new Date().toLocaleString('zh-CN', {
                year: 'numeric',
                month: '2-digit',
                day: '2-digit',
                hour: '2-digit',
                minute: '2-digit',
                second: '2-digit',
                hour12: false
            });
        }
        buildSummarizePrompt(records) {
            const recordTexts = records.map((msg, idx) => {
                const content = typeof msg.content === 'string'
                    ? msg.content
                    : JSON.stringify(msg.content);
                const preview = content.length > 500 ? content.slice(0, 500) + '...' : content;
                return `[事件${idx + 1}] 角色:${msg.role} | 内容:${preview}`;
            });
            const currentTime = this.getCurrentTime();
            const currentLocation = this.getCurrentLocation();
            return `请将以下 ${records.length} 条历史事件数据，每个事件独立摘要为一个简洁准确的记忆点摘要。

【事件数据】
${recordTexts.join('\n')}

【系统上下文】
- 当前时间: ${currentTime}（若事件未包含时间信息，使用此时间）
- 当前位置: ${currentLocation}（若事件未包含地点信息，使用此位置）

【处理规则】
1. 每个事件独立生成一条记忆点摘要，不要跨事件合并
2. 若事件明确包含时间信息，保留原时间；否则使用上述当前时间
3. 若事件明确包含地点信息，保留原地点；否则使用上述当前位置
4. 摘要内容简洁准确，聚焦核心事实

【输出格式】
请输出 JSON 数组，每个元素对应一个事件的记忆点摘要：
\`\`\`json
[
  {
    "time": "时间信息（从事件中提取，若无则使用当前时间）",
    "location": "地点信息（从事件中提取，若无则使用当前位置）",
    "content": "事件的核心内容摘要，简洁准确",
    "topic": "事件的关键词或主题"
  }
]
\`\`\`

仅输出 JSON 数组，不要包含其他说明文字。`;
        }
        buildDecisionPrompt(summary, existingRecords) {
            return `请针对以下记忆点摘要，判断应执行的操作：

【当前摘要】
- 时间: ${summary.time}
- 地点: ${summary.location}
- 内容: ${summary.content}
- 话题: ${summary.topic}

【已有相关记录】
${existingRecords}

【决策要求】
请判断以下三项：
1. 是否需要与已有记录合并？若合并，需提供合并后的完整内容（合并方式：删除旧记录，写入新合并记录）
2. 是否需要删除某些已有记录？列出要删除的记录ID
3. 是否需要将当前摘要持久化存储到数据库？

【决策原则】
- 完全重复的信息：删除旧的，不写入新的（should_store=false）
- 语义关联可合并：删除旧的，写入合并后的新内容（should_store=true，store_content为合并后内容）
- 无相似记录：直接写入新摘要（should_store=true，store_content为原摘要格式化内容）
- 已有记录过时但新摘要无价值：仅删除旧的（should_store=false）

【输出格式】
请输出 JSON 对象：
\`\`\`json
{
  "delete_ids": ["需要删除的记录ID列表"],
  "should_store": true或false,
  "store_content": "若存储，使用的内容（合并时为合并后内容，否则为原摘要格式化内容）"
}
\`\`\`

仅输出 JSON 对象，不要包含其他说明文字。`;
        }
        formatSummaryAsRecord(summary) {
            return `[${summary.time}] 地点:${summary.location} | 事件:${summary.content} | 话题:${summary.topic}`;
        }
        ensureTimestampInRecord(content) {
            const timestampRegex = /^\[([^\]]+)\]/;
            if (timestampRegex.test(content))
                return content;
            return `[${this.getCurrentTime()}] ${content}`;
        }
    }
    class Toolchain extends Prompt {
        queryExistingRecords(queryText, topK = 10) {
            if (!queryText || queryText.trim().length === 0)
                return [];
            const [results, error] = memoryQuery('lunar_messages', queryText.trim(), topK);
            if (error) {
                console.error('[编纂者] 记忆库查询失败:', error);
                return [];
            }
            return results || [];
        }
        executeBatchActions(decisions) {
            const allDeleteIds = [];
            for (const decision of decisions) {
                allDeleteIds.push(...decision.deleteIds);
            }
            const uniqueDeleteIds = [...new Set(allDeleteIds)];
            console.log(`[编纂者] 准备删除 ${uniqueDeleteIds.length} 条旧记录`);
            for (const id of uniqueDeleteIds) {
                const trimmedId = id.trim();
                if (!trimmedId)
                    continue;
                const [, error] = memoryDelete('lunar_messages', trimmedId);
                if (error)
                    console.error(`[编纂者] 删除记录 ${trimmedId} 失败:`, error);
                else
                    console.log(`[编纂者] 已删除记录 ${trimmedId}`);
            }
            const toStore = decisions.filter(d => d.shouldStore);
            console.log(`[编纂者] 准备写入 ${toStore.length} 条新记录`);
            for (const decision of toStore) {
                if (!decision.storeContent || decision.storeContent.trim().length === 0)
                    continue;
                const finalContent = this.ensureTimestampInRecord(decision.storeContent.trim());
                const [, error] = memoryAdd('lunar_messages', 'assistant', finalContent);
                if (error)
                    console.error('[编纂者] 写入记录失败:', error);
                else
                    console.log('[编纂者] 已写入新记录');
            }
        }
        parseJsonResponse(content) {
            try {
                const jsonMatch = content.match(/```(?:json)?\s*([\s\S]*?)```/);
                const jsonStr = jsonMatch ? jsonMatch[1].trim() : content.trim();
                return JSON.parse(jsonStr);
            }
            catch (error) {
                console.error('[编纂者] JSON 解析失败:', error, '原始内容:', content.slice(0, 200));
                return null;
            }
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
            if (!BaseConfig.memoryReady) {
                BaseConfig.initMemory();
                if (!BaseConfig.memoryReady) {
                    console.warn('[编纂者] 记忆库未就绪，保留未读记录待下次整理');
                    return;
                }
            }
            try {
                const summaries = this.generateMemorySummaries(OnlyData.unreadRecords);
                if (summaries.length === 0) {
                    console.log('[编纂者] 未生成有效摘要，结束整理');
                    return;
                }
                console.log(`[编纂者] 阶段一完成，生成 ${summaries.length} 条记忆点摘要`);
                const decisions = this.processSummaries(summaries);
                console.log(`[编纂者] 阶段二完成，生成 ${decisions.length} 条决策`);
                this.executeBatchActions(decisions);
                console.log('[编纂者] 历史记录组织完成');
                OnlyData.unreadRecords = [];
            }
            catch (error) {
                console.error('[编纂者] 组织历史记录失败，保留未读记录待下次重试:', error);
            }
        }
        generateMemorySummaries(records) {
            const prompt = this.buildSummarizePrompt(records);
            this.coverContext({ role: 'user', content: prompt });
            this.runtimeMessages = [];
            let response;
            try {
                response = this.run([], []);
            }
            catch (error) {
                console.error('[编纂者] 阶段一模型推理失败:', error);
                return [];
            }
            const content = response.body?.choices?.[0]?.message?.content || '';
            const summaries = this.parseJsonResponse(content);
            if (!summaries || !Array.isArray(summaries))
                return [];
            return summaries.filter(s => s && s.content && s.content.trim().length > 0);
        }
        processSummaries(summaries) {
            const decisions = [];
            for (const summary of summaries) {
                const decision = this.processMemorySummary(summary);
                decisions.push(decision);
            }
            return decisions;
        }
        processMemorySummary(summary) {
            const queryText = summary.topic || summary.content.slice(0, 50);
            const existing = this.queryExistingRecords(queryText, 10);
            const existingText = existing.length === 0
                ? '无相关已有记录'
                : existing.map((r, i) => `[记录${i + 1}] ID:${r.id} | 相似度:${(r.similarity * 100).toFixed(1)}% | 内容:${r.content}`).join('\n');
            const prompt = this.buildDecisionPrompt(summary, existingText);
            this.coverContext({ role: 'user', content: prompt });
            this.runtimeMessages = [];
            let response;
            try {
                response = this.run([], []);
            }
            catch (error) {
                console.error('[编纂者] 阶段二模型推理失败，使用默认决策（存储原摘要）:', error);
                return this.buildFallbackDecision(summary, existing);
            }
            const content = response.body?.choices?.[0]?.message?.content || '';
            const decision = this.parseJsonResponse(content);
            if (!decision) {
                return this.buildFallbackDecision(summary, existing);
            }
            let storeContent = decision.store_content || '';
            if (decision.should_store && !storeContent) {
                storeContent = this.formatSummaryAsRecord(summary);
            }
            return {
                summary,
                deleteIds: decision.delete_ids || [],
                shouldStore: !!decision.should_store,
                storeContent
            };
        }
        buildFallbackDecision(summary, existing) {
            const SIMILARITY_THRESHOLD = 0.85;
            const deleteIds = existing
                .filter(r => r.similarity >= SIMILARITY_THRESHOLD)
                .map(r => r.id);
            return {
                summary,
                deleteIds,
                shouldStore: true,
                storeContent: this.formatSummaryAsRecord(summary)
            };
        }
        persistDiscardedMessages(discarded) {
            console.log('[编纂者] 开始持久化被抛弃的消息');
            if (!BaseConfig.memoryReady)
                BaseConfig.initMemory();
            if (!BaseConfig.memoryReady)
                return;
            for (const message of discarded) {
                const content = typeof message.content === 'string' ? message.content : JSON.stringify(message.content);
                memoryAdd('lunar_messages', message.role, content);
            }
        }
        queryHistoricalRecords(queryText, topK = 10) {
            if (!BaseConfig.memoryReady)
                BaseConfig.initMemory();
            if (!BaseConfig.memoryReady)
                return [];
            const [results, error] = memoryQuery('lunar_messages', queryText, topK);
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
    }

    class AgentDefine {
        queryKeywords = new ModelBuilder(fileView('prompts/queryKeywords.md')[0]);
        emotionManager = new ModelBuilder(fileView('prompts/emotionManager.md')[0]);
        summaryRole = new ModelBuilder(fileView('prompts/summaryRole.md')[0]);
        descriptionRole = new ModelBuilder(fileView('prompts/descriptionRole.md')[0]);
        dialogueRole = new DialogueRole();
        researcherRole = new ResearcherRole();
        painterRole = new PainterRole();
        musicianRole = new MusicianRole();
        organizeRole = new OrganizeRole();
        unreadContext = [];
        unreadVideoUrl = [];
        finalResponse = "";
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
            const cachedPrompt = getPromptFromKnowledge(videoUrl);
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
                savePromptToKnowledge(videoUrl, videoSummary);
        }
        async LiteImageFile() {
            for (let message of this.unreadContext) {
                if (typeof message.content === 'string')
                    continue;
                const newContent = [];
                for (let item of message.content) {
                    if (item.type == 'text' || item.type == 'input_audio')
                        newContent.push(item);
                    else if (item.image_url && OnlyData.videoFormatsExtensions.some(format => item.image_url.url.toLowerCase().endsWith(format))) {
                        await this.analysisVideoFile(item.image_url.url, '');
                    }
                    else if (item.image_url && !item.image_url.url.startsWith("data:image")) {
                        console.log(item.image_url.url);
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
        silenceCount = 0;
        errorCount = 0;
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
            while (true) {
                try {
                    this.syncLTPXToolStatus();
                    await this.pullExternalMessages();
                    const dueItems = checkDueItems();
                    for (const item of dueItems) {
                        this.unreadContext.push({ role: 'user', content: `[计划提醒] 预约时间已到，请执行以下计划：${item.content}` });
                    }
                    const messageLength = this.unreadContext.length + this.unreadVideoUrl.length;
                    const messageType = messageLength === 0 ? 'response' : 'active';
                    const allowSpeak = RandomFloor(15, 100) < this.speakWeight;
                    if (messageLength === 0 && !allowSpeak) {
                        this.silenceCount = Math.min(this.silenceCount + 1, 100);
                        await new Promise(resolve => setTimeout(resolve, 1000));
                        continue;
                    }
                    if (messageLength === 0 && allowSpeak && this.silenceCount < 30) {
                        this.silenceCount = Math.min(this.silenceCount + 1, 100);
                        await new Promise(resolve => setTimeout(resolve, 1000));
                        continue;
                    }
                    this.silenceCount = 0;
                    if (messageLength === 0)
                        this.speakWeight = 0;
                    await this.batchProcessVideoFiles();
                    const currentUnreadContext = [...this.unreadContext];
                    this.researcherRole.executeResearch(this.dialogueRole.messages, currentUnreadContext);
                    this.painterRole.createCreativeWork(this.dialogueRole.messages, currentUnreadContext);
                    this.musicianRole.createCreativeWork(this.dialogueRole.messages, currentUnreadContext);
                    await this.createChatMessage();
                    if (!this.finalResponse.trim().length)
                        throw new Error('消息响应为空');
                    else
                        this.errorCount = 0;
                    if (OnlyData.unreadRecords.length > 10) {
                        setTimeout(() => this.organizeRole.organizeHistoricalRecords(), 0);
                    }
                    const { thinkingBlocks, codeBlocks, actionBlocks, emotionBlocks, textChunks } = parseContent(this.finalResponse);
                    if (!textChunks.length)
                        throw new Error('清洗后的文本为空');
                    if (actionBlocks.length)
                        console.log('[动作区]', actionBlocks.join(' | '));
                    if (emotionBlocks.length)
                        console.log('[情感区]', emotionBlocks.join(' | '));
                    for (const thinking of thinkingBlocks) {
                        pushContext(messageType, thinking, '');
                    }
                    for (const code of codeBlocks) {
                        pushContext(messageType, code, '');
                    }
                    for (const chunk of textChunks) {
                        let audio = '';
                        try {
                            const [audioData, err] = tts(chunk.tts);
                            if (!err && audioData)
                                audio = audioData;
                        }
                        catch (e) {
                            console.error(`TTS合成异常: [${chunk.tts}]`, e);
                        }
                        pushContext(messageType, chunk.display, audio);
                    }
                }
                catch (error) {
                    const [promptSound, , , readErr] = readFile('audios/cartoon-fail.mp3');
                    if (readErr)
                        console.error('读取提示音失败:', readErr);
                    console.error(error.message, ' || ', error.stack);
                    this.errorCount++;
                    pushContext('active', this.randomDefaultMessage, promptSound);
                    if (this.errorCount >= 3) {
                        this.resetAgentState();
                        this.errorCount = 0;
                        continue;
                    }
                }
            }
        }
        resetAgentState() {
            this.queryKeywords.coverContext([]);
            this.emotionManager.coverContext([]);
            this.summaryRole.coverContext([]);
            this.descriptionRole.coverContext([]);
            this.dialogueRole.coverContext([]);
            this.researcherRole.coverContext([]);
            this.painterRole.coverContext([]);
            this.musicianRole.coverContext([]);
            this.organizeRole.coverContext([]);
            this.unreadContext = [];
            this.unreadVideoUrl = [];
        }
        syncLTPXToolStatus() {
            try {
                const statusJSON = getLTPXToolStatus();
                if (!statusJSON || statusJSON === '{}')
                    return;
                const status = JSON.parse(statusJSON);
                if ((status.pendingLoads && status.pendingLoads.length > 0) ||
                    (status.pendingUnloads && status.pendingUnloads.length > 0)) {
                    processLTPXChanges(statusJSON);
                }
            }
            catch (e) {
                console.error('LTPX 工具状态同步失败:', e);
            }
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
    const AgentRuntime = new LunarAgent();
    const message = [
        {
            type: 'text',
            text: '你好呀~'
        }
    ];
    AgentRuntime.testMessageWrite('user', message, 1500);

    const SCHEDULE_FILE_PATH = 'database/schedule.json';
    const scheduleTools = [
        {
            type: "function",
            function: {
                name: "create_schedule",
                description: "在计划表中创建新的计划项，指定执行时间点和对应的工作内容。时间格式为 ISO 8601 (如 '2026-06-14T15:30:00') 或中文日期时间格式 (如 '2026年6月14日 15:30')。",
                parameters: {
                    type: "object",
                    properties: {
                        time: {
                            type: "string",
                            description: "计划执行的时间点，支持 ISO 8601 格式或中文日期时间格式，例如 '2026-06-14T15:30:00' 或 '2026年6月14日 15:30'"
                        },
                        content: {
                            type: "string",
                            description: "计划执行的工作内容描述，应清晰说明需要完成的事项"
                        }
                    },
                    required: ["time", "content"]
                }
            }
        },
        {
            type: "function",
            function: {
                name: "edit_schedule",
                description: "编辑计划表中已存在的计划项，可修改其执行时间和/或工作内容。需要提供计划项的ID，至少提供 time 或 content 中的一个进行修改。",
                parameters: {
                    type: "object",
                    properties: {
                        id: {
                            type: "string",
                            description: "要编辑的计划项ID，从 query_schedule 返回结果中获得"
                        },
                        time: {
                            type: "string",
                            description: "修改后的执行时间点，不修改则留空"
                        },
                        content: {
                            type: "string",
                            description: "修改后的工作内容描述，不修改则留空"
                        }
                    },
                    required: ["id"]
                }
            }
        },
        {
            type: "function",
            function: {
                name: "delete_schedule",
                description: "从计划表中删除指定的计划项。需要提供计划项的ID。",
                parameters: {
                    type: "object",
                    properties: {
                        id: {
                            type: "string",
                            description: "要删除的计划项ID，从 query_schedule 返回结果中获得"
                        }
                    },
                    required: ["id"]
                }
            }
        },
        {
            type: "function",
            function: {
                name: "query_schedule",
                description: "查询计划表中已有的计划项列表，可按关键词筛选。返回所有匹配的计划项及其ID、时间和内容。",
                parameters: {
                    type: "object",
                    properties: {
                        keyword: {
                            type: "string",
                            description: "用于筛选计划项的关键词，留空则返回全部计划项"
                        }
                    },
                    required: []
                }
            }
        }
    ];
    let scheduleCache = [];
    function loadSchedulesFromDisk() {
        const [fileData, , , readErr] = readFile(SCHEDULE_FILE_PATH);
        if (readErr) {
            return [];
        }
        try {
            const jsonStr = atob(fileData);
            const data = JSON.parse(jsonStr);
            return Array.isArray(data) ? data : [];
        }
        catch (e) {
            console.error('[计划表] 计划表数据解析失败:', e);
            return [];
        }
    }
    function saveSchedulesToDisk(schedules) {
        try {
            const jsonStr = JSON.stringify(schedules, null, 2);
            const [, , saveErr] = saveFile(SCHEDULE_FILE_PATH, true, jsonStr);
            if (saveErr) {
                console.error('[计划表] 保存计划表失败:', saveErr);
                return false;
            }
            return true;
        }
        catch (e) {
            console.error('[计划表] 序列化计划表失败:', e);
            return false;
        }
    }
    const CN_DATE_REGEX = /^(\d{4})年(\d{1,2})月(\d{1,2})日\s+(\d{1,2}):(\d{2})(?::(\d{2}))?$/;
    function normalizeTime(raw) {
        if (!raw || raw.trim().length === 0) {
            return null;
        }
        const trimmed = raw.trim();
        const stdDate = new Date(trimmed);
        if (!isNaN(stdDate.getTime())) {
            return formatISO(stdDate);
        }
        const cnMatch = trimmed.match(CN_DATE_REGEX);
        if (cnMatch) {
            const [, year, month, day, hour, minute, second] = cnMatch;
            return `${year.padStart(4, '0')}-${month.padStart(2, '0')}-${day.padStart(2, '0')}T${hour.padStart(2, '0')}:${minute.padStart(2, '0')}:${(second || '00').padStart(2, '0')}`;
        }
        return null;
    }
    function formatISO(date) {
        const y = date.getFullYear();
        const mo = String(date.getMonth() + 1).padStart(2, '0');
        const d = String(date.getDate()).padStart(2, '0');
        const h = String(date.getHours()).padStart(2, '0');
        const mi = String(date.getMinutes()).padStart(2, '0');
        const s = String(date.getSeconds()).padStart(2, '0');
        return `${y}-${mo}-${d}T${h}:${mi}:${s}`;
    }
    function initSchedules() {
        const raw = loadSchedulesFromDisk();
        if (raw.length === 0) {
            saveSchedulesToDisk([]);
            scheduleCache = [];
            console.log('[计划表] 初始化完成，计划表为空');
            return;
        }
        let needsRewrite = false;
        for (const item of raw) {
            const normalized = normalizeTime(item.time);
            if (normalized && normalized !== item.time) {
                item.time = normalized;
                needsRewrite = true;
            }
        }
        scheduleCache = raw;
        if (needsRewrite) {
            saveSchedulesToDisk(scheduleCache);
            console.log('[计划表] 已修正历史数据中的非标准时间格式');
        }
        console.log(`[计划表] 初始化完成，共加载 ${scheduleCache.length} 个计划项`);
    }
    function generateId() {
        return `schedule_${Date.now()}_${Math.random().toString(36).slice(2, 9)}`;
    }
    function parseArgs$1(args) {
        return typeof args === 'string' ? JSON.parse(args) : (args || {});
    }
    async function handleCreateSchedule(args) {
        const { time, content } = parseArgs$1(args);
        if (!time || time.trim().length === 0) {
            return ['创建计划项失败：执行时间不能为空，请提供有效的时间点', ''];
        }
        if (!content || content.trim().length === 0) {
            return ['创建计划项失败：工作内容不能为空，请提供具体的计划内容', ''];
        }
        const normalizedTime = normalizeTime(time);
        if (!normalizedTime) {
            return [`创建计划项失败：无法解析时间格式 "${time}"，请使用 ISO 8601 格式 (如 "2026-06-14T15:30:00") 或中文格式 (如 "2026年6月14日 15:30")`, ''];
        }
        const newItem = {
            id: generateId(),
            time: normalizedTime,
            content: content.trim()
        };
        scheduleCache.push(newItem);
        if (!saveSchedulesToDisk(scheduleCache)) {
            scheduleCache.pop();
            return ['创建计划项失败：保存到磁盘时出错，请稍后重试', ''];
        }
        console.log(`[计划表] 创建成功: [${newItem.id}] ${newItem.time} - ${newItem.content}`);
        return [`计划项创建成功：ID为 ${newItem.id}，执行时间: ${newItem.time}，内容: ${newItem.content}`, ''];
    }
    async function handleEditSchedule(args) {
        const { id, time, content } = parseArgs$1(args);
        if (!id || id.trim().length === 0) {
            return ['编辑计划项失败：计划项ID不能为空，请从 query_schedule 获取有效ID', ''];
        }
        if ((!time || time.trim().length === 0) && (!content || content.trim().length === 0)) {
            return ['编辑计划项失败：至少需要提供 time 或 content 中的一个进行修改', ''];
        }
        const index = scheduleCache.findIndex(item => item.id === id.trim());
        if (index === -1) {
            return [`编辑计划项失败：未找到ID为 ${id} 的计划项，请使用 query_schedule 确认正确的ID`, ''];
        }
        const snapshot = { ...scheduleCache[index] };
        if (time && time.trim().length > 0) {
            const normalizedTime = normalizeTime(time);
            if (!normalizedTime) {
                return [`编辑计划项失败：无法解析时间格式 "${time}"，请使用 ISO 8601 或中文日期时间格式`, ''];
            }
            scheduleCache[index].time = normalizedTime;
        }
        if (content && content.trim().length > 0) {
            scheduleCache[index].content = content.trim();
        }
        if (!saveSchedulesToDisk(scheduleCache)) {
            scheduleCache[index] = snapshot;
            return ['编辑计划项失败：保存到磁盘时出错，请稍后重试', ''];
        }
        console.log(`[计划表] 编辑成功: [${id}] -> ${scheduleCache[index].time} - ${scheduleCache[index].content}`);
        return [`计划项编辑成功：ID为 ${id}，已更新为 执行时间: ${scheduleCache[index].time}，内容: ${scheduleCache[index].content}`, ''];
    }
    async function handleDeleteSchedule(args) {
        const { id } = parseArgs$1(args);
        if (!id || id.trim().length === 0) {
            return ['删除计划项失败：计划项ID不能为空', ''];
        }
        const index = scheduleCache.findIndex(item => item.id === id.trim());
        if (index === -1) {
            return [`删除计划项失败：未找到ID为 ${id} 的计划项`, ''];
        }
        const deletedItem = scheduleCache[index];
        scheduleCache.splice(index, 1);
        if (!saveSchedulesToDisk(scheduleCache)) {
            scheduleCache.splice(index, 0, deletedItem);
            return ['删除计划项失败：保存到磁盘时出错，请稍后重试', ''];
        }
        console.log(`[计划表] 删除成功: [${id}] ${deletedItem.time} - ${deletedItem.content}`);
        return [`计划项删除成功：已移除 [${deletedItem.id}] ${deletedItem.time} - ${deletedItem.content}`, ''];
    }
    async function handleQuerySchedule(args) {
        const { keyword } = parseArgs$1(args);
        if (scheduleCache.length === 0) {
            return ['当前计划表为空，没有任何计划项，可以放心创建新计划。', ''];
        }
        const sorted = [...scheduleCache].sort((a, b) => new Date(a.time).getTime() - new Date(b.time).getTime());
        const filtered = keyword && keyword.trim().length > 0
            ? sorted.filter(item => item.content.includes(keyword.trim()) || item.time.includes(keyword.trim()))
            : sorted;
        if (filtered.length === 0) {
            return [`未找到包含关键词 "${keyword}" 的计划项，当前共有 ${scheduleCache.length} 个计划项。`, ''];
        }
        return [`当前共有 ${scheduleCache.length} 个计划项` + (keyword ? `，匹配 "${keyword}" 的有 ${filtered.length} 条` : '') + ':\n' +
                filtered.map((item, i) => `[计划项${i + 1}] ID:${item.id} | 时间:${item.time} | 内容:${item.content}`).join('\n'), ''];
    }
    function checkDueItems() {
        if (scheduleCache.length === 0)
            return [];
        const now = new Date();
        const dueItems = [];
        const remaining = [];
        for (const item of scheduleCache) {
            const itemTime = new Date(item.time);
            if (isNaN(itemTime.getTime())) {
                console.warn(`[计划表] 无效的时间格式，跳过: [${item.id}] ${item.time}`);
                remaining.push(item);
                continue;
            }
            if (now >= itemTime) {
                dueItems.push(item);
                console.log(`[计划表] 触发到期计划项: [${item.id}] ${item.time} - ${item.content}`);
            }
            else {
                remaining.push(item);
            }
        }
        if (dueItems.length > 0) {
            scheduleCache = remaining;
            saveSchedulesToDisk(scheduleCache);
        }
        return dueItems;
    }
    initSchedules();
    OnlyData.LTPfunction.set('create_schedule', handleCreateSchedule);
    OnlyData.LTPfunction.set('edit_schedule', handleEditSchedule);
    OnlyData.LTPfunction.set('delete_schedule', handleDeleteSchedule);
    OnlyData.LTPfunction.set('query_schedule', handleQuerySchedule);
    OnlyData.LTPdefinition.push(...scheduleTools);

    let webSearchInitialized = false;
    function initWebSearch() {
        if (webSearchInitialized) {
            console.log('[网络检索] 子系统已初始化，跳过重复初始化');
            return true;
        }
        const config = {
            baseURL: OnlyData.systemUrl,
            apiKey: OnlyData.SystemKey,
            model: OnlyData.MultimodalName,
            maxTokens: 4096,
            temperature: 0.7,
        };
        try {
            const [success, err] = webSearchInit(config.baseURL, config.apiKey, config.model, config.maxTokens, config.temperature);
            if (err) {
                console.error('[网络检索] 初始化失败:', err);
                return false;
            }
            webSearchInitialized = true;
            console.log('[网络检索] 子系统初始化成功');
            return true;
        }
        catch (e) {
            console.error('[网络检索] 初始化异常:', e);
            return false;
        }
    }
    function isWebSearchReady() {
        return webSearchInitialized && webSearchIsReady();
    }
    function executeWebSearch(query, mode = 'webpage') {
        try {
            if (mode === 'depth') {
                const [result, err] = webSearchDepth(query);
                if (err)
                    return ['', err];
                return [result, null];
            }
            if (mode === 'webpage') {
                const [result, err] = webSearchWebpage(query);
                if (err)
                    return ['', err];
                return [result, null];
            }
            const [result, err] = webSearchSimple(query);
            if (err)
                return ['', err];
            return [result, null];
        }
        catch (e) {
            return ['', e];
        }
    }

    const screenshotTools = [
        {
            type: "function",
            function: {
                name: "screenshot",
                description: "截取当前屏幕画面。当用户要求查看屏幕内容、确认屏幕显示状态、或需要获取当前屏幕画面时，应使用此工具。支持指定显示器索引、截取区域、缩放比例和图片格式。截取的图片会自动缩放处理并展示给用户。",
                parameters: {
                    type: "object",
                    properties: {
                        display_index: {
                            type: "number",
                            description: "显示器索引：0 表示主显示器，-1 表示截取所有显示器拼接画面，默认为 0"
                        },
                        region: {
                            type: "string",
                            description: "截图区域，格式为 'x,y,width,height'，例如 '100,200,800,600'。留空则截取整个显示器"
                        },
                        scale: {
                            type: "string",
                            description: "缩放参数：可以是比例（如 '0.5'）或指定宽高（如 '800,600'）。留空则自动缩放"
                        },
                        format: {
                            type: "string",
                            description: "输出图片格式：'png' 或 'jpg'，默认为 'png'",
                            enum: ["png", "jpg"]
                        }
                    },
                    required: []
                }
            }
        }
    ];
    async function handleScreenshot(args) {
        console.log(`[截图] ========== 开始处理截图工具调用 ==========`);
        console.log(`[截图] 原始参数: ${JSON.stringify(args)}`);
        const parsed = typeof args === 'string' ? JSON.parse(args) : (args || {});
        const { display_index, region, scale, format } = parsed;
        console.log(`[截图] 参数解析完成: display_index=${display_index}, region=${region}, scale=${scale}, format=${format}`);
        const displayIndex = display_index ?? 0;
        const captureFormat = format || 'png';
        console.log(`[截图] 最终参数: display=${displayIndex}, region="${region || ''}", scale="${scale || ''}", format="${captureFormat}"`);
        console.log(`[截图] 准备执行截图操作...`);
        const [result, captureErr] = screenshotCapture(displayIndex, region || '', scale || '', captureFormat, 0);
        if (captureErr) {
            console.error(`[截图] 截图失败: ${captureErr.message || String(captureErr)}`);
            console.log(`[截图] ========== 截图工具调用结束(失败) ==========`);
            return [`截图失败：${captureErr.message || String(captureErr)}`, ''];
        }
        console.log(`[截图] 截图处理成功: ${result?.width}x${result?.height}, 格式=${result?.format}`);
        if (!result || !result.base64) {
            console.error(`[截图] 截图失败: 未获取到截图数据`);
            console.log(`[截图] ========== 截图工具调用结束(失败) ==========`);
            return ['截图失败：未获取到截图数据', ''];
        }
        pushImage([result.base64]);
        console.log(`[截图] 图片已推送: ${result.width}x${result.height}, 格式=${result.format}, 数据长度=${result.base64.length} 字节`);
        const sizeInfo = `${result.width}x${result.height}`;
        const textResponse = `截图完成，已获取当前屏幕画面（${sizeInfo}），图片已展示给用户。`;
        console.log(`[截图] 返回响应: ${sizeInfo}`);
        console.log(`[截图] ========== 截图工具调用结束(成功) ==========`);
        return [textResponse, result.base64];
    }
    OnlyData.LTPfunction.set('screenshot', handleScreenshot);
    OnlyData.LTPdefinition.push(...screenshotTools);

    const agentControlTools = [
        {
            type: "function",
            function: {
                name: "play_action",
                description: "让智能体执行预设动作。可用动作：荡秋千（需要鼠标追踪）、翻花绳（需要鼠标追踪）。执行动作时会自动切换鼠标追踪状态。",
                parameters: {
                    type: "object",
                    properties: {
                        action_name: {
                            type: "string",
                            description: "动作名称，可选值：荡秋千、翻花绳",
                            enum: ["荡秋千", "翻花绳"]
                        }
                    },
                    required: ["action_name"]
                }
            }
        },
        {
            type: "function",
            function: {
                name: "agent_movement",
                description: "控制智能体移动到指定位置。移动期间会自动关闭鼠标追踪，移动结束后可选恢复。移动有10秒超时限制。",
                parameters: {
                    type: "object",
                    properties: {
                        x: {
                            type: "number",
                            description: "目标X坐标"
                        },
                        y: {
                            type: "number",
                            description: "目标Y坐标（地面为0）"
                        },
                        z: {
                            type: "number",
                            description: "目标Z坐标"
                        },
                        resume_tracking: {
                            type: "boolean",
                            description: "移动结束后是否恢复鼠标追踪，默认为 true"
                        }
                    },
                    required: ["x", "y", "z"]
                }
            }
        }
    ];
    const ALLOWED_ACTIONS = ['荡秋千', '翻花绳'];
    function parseArgs(args) {
        return typeof args === 'string' ? JSON.parse(args) : (args || {});
    }
    async function handlePlayAction(args) {
        const { action_name } = parseArgs(args);
        if (!action_name || typeof action_name !== 'string' || action_name.trim().length === 0) {
            return ['执行动作失败：动作名称不能为空，请提供有效的动作名称', ''];
        }
        if (!ALLOWED_ACTIONS.includes(action_name)) {
            return [`执行动作失败：不支持的动作 "${action_name}"，可用动作为：${ALLOWED_ACTIONS.join('、')}`, ''];
        }
        pushContext('action', JSON.stringify({ type: 'action', action: action_name }), '');
        console.log(`[智能体控制] 执行动作: ${action_name}`);
        return [`已执行动作：${action_name}`, ''];
    }
    async function handleAgentMovement(args) {
        const { x, y, z, resume_tracking } = parseArgs(args);
        if (typeof x !== 'number' || typeof y !== 'number' || typeof z !== 'number') {
            return ['移动失败：坐标参数 x、y、z 必须为数字', ''];
        }
        if (isNaN(x) || isNaN(y) || isNaN(z)) {
            return ['移动失败：坐标参数 x、y、z 不能为 NaN', ''];
        }
        const resumeTracking = resume_tracking !== false;
        pushContext('movement', JSON.stringify({ type: 'movement', position: { x, y, z }, resumeTracking }), '');
        console.log(`[智能体控制] 移动到 (${x}, ${y}, ${z})，恢复鼠标追踪: ${resumeTracking}`);
        return [`正在移动到 (${x}, ${y}, ${z})`, ''];
    }
    OnlyData.LTPfunction.set('play_action', handlePlayAction);
    OnlyData.LTPfunction.set('agent_movement', handleAgentMovement);
    OnlyData.LTPdefinition.push(...agentControlTools);

    const EMOJI_REGEX = /[\u{1F600}-\u{1F64F}\u{1F300}-\u{1F5FF}\u{1F680}-\u{1F6FF}\u{1F900}-\u{1F9FF}\u{1FA70}-\u{1FAFF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}\u{FE00}-\u{FE0F}\u{1F1E0}-\u{1F1FF}\u{200D}\u{20E3}\u{FE0F}]/gu;
    function extractThinkingBlocks(text) {
        const blocks = [];
        const regex = /<think>([\s\S]*?)<\/think>/gi;
        let match;
        while ((match = regex.exec(text)) !== null) {
            const content = match[1].trim();
            if (content.length > 0) {
                blocks.push(content);
            }
        }
        const remaining = text.replace(/<think>[\s\S]*?<\/think>/gi, '');
        return [blocks, remaining];
    }
    function extractCodeBlocks(text) {
        const blocks = [];
        const codeBlockRegex = /```[a-zA-Z0-9+#-]*[\s\S]*?```/g;
        let match;
        while ((match = codeBlockRegex.exec(text)) !== null) {
            blocks.push(match[0]);
        }
        const remaining = text.replace(/```[a-zA-Z0-9+#-]*[\s\S]*?```/g, '');
        return [blocks, remaining];
    }
    function extractActionBlocks(text) {
        const blocks = [];
        const stack = [];
        const ranges = [];
        for (let i = 0; i < text.length; i++) {
            const ch = text[i];
            if (ch === '(' || ch === '\uFF08') {
                stack.push(i);
            }
            else if (ch === ')' || ch === '\uFF09') {
                if (stack.length === 0)
                    continue;
                const start = stack.pop();
                if (stack.length === 0) {
                    const content = text.slice(start + 1, i).trim();
                    if (content.length > 0) {
                        blocks.push(content);
                    }
                    ranges.push([start, i + 1]);
                }
            }
        }
        let remaining = '';
        let lastEnd = 0;
        for (const [start, end] of ranges) {
            remaining += text.slice(lastEnd, start);
            lastEnd = end;
        }
        remaining += text.slice(lastEnd);
        return [blocks, remaining];
    }
    function extractEmotionBlocks(text) {
        const blocks = [];
        const regex = new RegExp(EMOJI_REGEX.source, 'gu');
        let match;
        let lastEnd = -1;
        let current = '';
        while ((match = regex.exec(text)) !== null) {
            if (current.length > 0 && match.index === lastEnd) {
                current += match[0];
            }
            else {
                if (current.length > 0)
                    blocks.push(current);
                current = match[0];
            }
            lastEnd = match.index + match[0].length;
        }
        if (current.length > 0)
            blocks.push(current);
        const remaining = text.replace(new RegExp(EMOJI_REGEX.source, 'gu'), '');
        return [blocks, remaining];
    }
    function cleanTextForTTS(text) {
        if (!text)
            return '';
        let processed = text;
        processed = processed.replace(/`[^`]*`/g, '');
        processed = processed.replace(/!\[.*?\]\(.*?\)/g, '');
        processed = processed.replace(/\[([^\]]*)\]\(.*?\)/g, '$1');
        processed = processed.replace(/<[^>]*>/g, '');
        processed = processed.replace(/[\u{1F600}-\u{1F64F}\u{1F300}-\u{1F5FF}\u{1F680}-\u{1F6FF}\u{1F900}-\u{1F9FF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}\u{FE00}-\u{FE0F}\u{1F1E0}-\u{1F1FF}\u{200D}\u{20E3}\u{FE0F}]/gu, '');
        processed = processed.replace(/\*/g, '');
        processed = processed.replace(/\r?\n/g, ' ');
        processed = processed.replace(/\（[^）]*\）/g, '');
        processed = processed.replace(/\([^)]*\)/g, '');
        const allowed = '\\u4e00-\\u9fff' + 'a-zA-Z0-9' + '\\s_~\\-' + '\uFF0C\u3002\uFF1F\uFF1A\uFF01\uFF1B\u3001\u2014\u2026\u300A\u300B\u201C\u201D\u2018\u2019\uFF08\uFF09\u3010\u3011' + ',.\'\"?:!;';
        const whitelist = new RegExp(`[^${allowed}]`, 'g');
        processed = processed.replace(whitelist, ',');
        processed = processed.replace(/\s+/g, ' ');
        return processed.trim();
    }
    function cleanTextForDisplay(text) {
        if (!text)
            return '';
        return text.replace(/[\u{1F600}-\u{1F64F}\u{1F300}-\u{1F5FF}\u{1F680}-\u{1F6FF}\u{1F900}-\u{1F9FF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}\u{FE00}-\u{FE0F}\u{1F1E0}-\u{1F1FF}\u{200D}\u{20E3}\u{FE0F}]/gu, '');
    }
    function splitSentences(text) {
        if (!text)
            return [];
        const LEVEL1_PUNCT = /[。？！—～?!]/;
        const LEVEL2_PUNCT = /[，,、：；:;]/;
        const MAX_LENGTH = 35;
        const LEADING_PUNCT = /^[。，、：；:;,?!？！—～"'""''()（）\[\]【】{}<>…\s]+/;
        const TRAILING_COMMA = /[，,]+$/;
        function formatChunk(chunk) {
            let result = chunk.trim();
            result = result.replace(LEADING_PUNCT, '');
            result = result.replace(TRAILING_COMMA, '');
            return result.trim();
        }
        function splitByPunct(source, punctRegex) {
            const result = [];
            let start = 0;
            for (let i = 0; i < source.length; i++) {
                if (punctRegex.test(source[i])) {
                    let end = i + 1;
                    while (end < source.length && punctRegex.test(source[end])) {
                        end++;
                    }
                    const fragment = source.slice(start, end).trim();
                    if (fragment.length > 0) {
                        result.push(fragment);
                    }
                    start = end;
                    i = end - 1;
                }
            }
            if (start < source.length) {
                const fragment = source.slice(start).trim();
                if (fragment.length > 0) {
                    result.push(fragment);
                }
            }
            return result;
        }
        const level1 = splitByPunct(text, LEVEL1_PUNCT);
        const result = [];
        function isInsideBracket(source, pos) {
            let depth = 0;
            for (let i = 0; i < pos; i++) {
                if (source[i] === '\uFF08' || source[i] === '(')
                    depth++;
                else if (source[i] === '\uFF09' || source[i] === ')')
                    depth--;
            }
            return depth > 0;
        }
        for (const fragment of level1) {
            if (fragment.length <= MAX_LENGTH) {
                const formatted = formatChunk(fragment);
                if (formatted.length > 0)
                    result.push(formatted);
                continue;
            }
            let remaining = fragment;
            while (remaining.length > MAX_LENGTH) {
                let splitPos = -1;
                for (let i = Math.min(remaining.length - 1, MAX_LENGTH - 1); i >= 0; i--) {
                    if (LEVEL2_PUNCT.test(remaining[i]) && !isInsideBracket(remaining, i)) {
                        let end = i + 1;
                        while (end < remaining.length && LEVEL2_PUNCT.test(remaining[end])) {
                            end++;
                        }
                        splitPos = end;
                        break;
                    }
                }
                if (splitPos === -1) {
                    splitPos = MAX_LENGTH;
                }
                const slice = formatChunk(remaining.slice(0, splitPos));
                if (slice.length > 0) {
                    result.push(slice);
                }
                remaining = remaining.slice(splitPos);
            }
            const tail = formatChunk(remaining);
            if (tail.length > 0) {
                result.push(tail);
            }
        }
        return result;
    }
    function parseContent(rawText) {
        if (!rawText)
            return { thinkingBlocks: [], codeBlocks: [], actionBlocks: [], emotionBlocks: [], textChunks: [] };
        const [thinkingBlocks, textAfterThinking] = extractThinkingBlocks(rawText);
        const [codeBlocks, textAfterCode] = extractCodeBlocks(textAfterThinking);
        const [actionBlocks, textAfterAction] = extractActionBlocks(textAfterCode);
        const [emotionBlocks, textAfterEmotion] = extractEmotionBlocks(textAfterAction);
        const displayText = cleanTextForDisplay(textAfterEmotion);
        const displayChunks = splitSentences(displayText);
        const textChunks = displayChunks.map(chunk => ({
            display: chunk,
            tts: cleanTextForTTS(chunk),
        }));
        return { thinkingBlocks, codeBlocks, actionBlocks, emotionBlocks, textChunks };
    }

    exports.AgentDefine = AgentDefine;
    exports.BaseConfig = BaseConfig;
    exports.CalculateMedian = CalculateMedian;
    exports.CalculateModes = CalculateModes;
    exports.Clamp = Clamp;
    exports.CreativeRoleBase = CreativeRoleBase;
    exports.DialogueRole = DialogueRole;
    exports.FileToBase64 = FileToBase64;
    exports.ModelBuilder = ModelBuilder;
    exports.MusicianRole = MusicianRole;
    exports.OnlyData = OnlyData;
    exports.OrganizeRole = OrganizeRole;
    exports.PainterRole = PainterRole;
    exports.RandomFloat = RandomFloat;
    exports.RandomFloor = RandomFloor;
    exports.ResearcherRole = ResearcherRole;
    exports.ThinkType = ThinkType;
    exports.agentControlTools = agentControlTools;
    exports.calculateFileHash = calculateFileHash;
    exports.checkDueItems = checkDueItems;
    exports.cleanTextForDisplay = cleanTextForDisplay;
    exports.cleanTextForTTS = cleanTextForTTS;
    exports.executeWebSearch = executeWebSearch;
    exports.fetchDocumentCallback = fetchDocumentCallback;
    exports.getFileContent = getFileContent;
    exports.getPromptFromKnowledge = getPromptFromKnowledge;
    exports.initSchedules = initSchedules;
    exports.initWebSearch = initWebSearch;
    exports.isWebSearchReady = isWebSearchReady;
    exports.parseContent = parseContent;
    exports.queryFromKnowledge = queryFromKnowledge;
    exports.saveImageToServer = saveImageToServer;
    exports.savePromptToKnowledge = savePromptToKnowledge;
    exports.scheduleTools = scheduleTools;
    exports.screenshotTools = screenshotTools;
    exports.splitSentences = splitSentences;
    exports.splitTextToStrings = splitTextToStrings;
    exports.toBtoaString = toBtoaString;

    return exports;

})({});
