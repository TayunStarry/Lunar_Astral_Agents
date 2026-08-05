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
            return "system-embedding";
        }
        ;
        static get userName() {
            return OnlyData.customConfig?.server?.user_name || "阁下";
        }
        ;
        static get debugMode() {
            return OnlyData.customConfig?.server?.debug_mode ?? false;
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
            if (this.messages.length >= 40) {
                const discarded = this.messages.slice(0, this.messages.length - 39);
                this.messages = this.messages.slice(-39).concat(cleaned);
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
            const rawMessages = [
                { role: 'system', content: this.systemPrompt },
                { role: 'user', content: '[上下文]' },
                ...appendContext,
                ...this.messages.slice(0, -1),
                ...this.runtimeMessages,
                ...this.messages.slice(-1)
            ];
            const requestBody = {
                model: OnlyData.MultimodalName,
                messages: rawMessages,
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
            if (result?.body?.error) {
                const errMsg = typeof result.body.error === 'string'
                    ? result.body.error
                    : result.body.error.message || JSON.stringify(result.body.error);
                throw new Error(`模型服务错误 [${result.status}]: ${errMsg}`);
            }
            if (!result?.body?.choices) {
                throw new Error(`模型响应异常: status=${result?.status}, body=${JSON.stringify(result?.body)?.substring(0, 200)}`);
            }
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
        dumpContext(roleName, outputPath) {
            const timestamp = new Date().toLocaleString('zh-CN', {
                year: 'numeric', month: '2-digit', day: '2-digit',
                hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false
            });
            const snapshot = {
                timestamp,
                role: roleName,
                systemPrompt: this.systemPrompt,
                messagesCount: this.messages.length,
                messages: this.messages.map((msg, idx) => {
                    const content = typeof msg.content === 'string'
                        ? msg.content
                        : JSON.stringify(msg.content);
                    return {
                        index: idx,
                        role: msg.role,
                        contentPreview: content.length > 500 ? content.slice(0, 500) + '...' : content,
                        contentLength: content.length,
                    };
                }),
                ragMessagesCount: this.ragMessages.length,
                ragMessages: this.ragMessages.map((msg, idx) => ({
                    index: idx,
                    role: msg.role,
                    contentPreview: typeof msg.content === 'string'
                        ? (msg.content.length > 300 ? msg.content.slice(0, 300) + '...' : msg.content)
                        : JSON.stringify(msg.content).slice(0, 300),
                })),
                runtimeMessagesCount: this.runtimeMessages.length,
                runtimeMessages: this.runtimeMessages.map((msg, idx) => ({
                    index: idx,
                    role: msg.role,
                    contentPreview: typeof msg.content === 'string'
                        ? (msg.content.length > 300 ? msg.content.slice(0, 300) + '...' : msg.content)
                        : JSON.stringify(msg.content).slice(0, 300),
                })),
                stream: this.stream,
                enableTools: this.enableTools,
            };
            const path = outputPath || `agent_debug_${roleName}.json`;
            const [, error] = saveDebugFile(path, JSON.stringify(snapshot, null, 2));
            if (error) {
                console.error(`[${roleName}] 导出上下文失败:`, error);
                return '';
            }
            console.log(`[${roleName}] 上下文快照已导出: ${path}`);
            return path;
        }
    }

    class CreativeRoleBase extends ModelBuilder {
        OWN_HISTORY_LIMIT = 5;
        MAX_ITERATIONS = 3;
        constructor(prompt) {
            super(prompt);
        }
        consumeHistory() {
            const result = [...this.messages];
            this.messages = [];
            return result;
        }
        writeContext(context) {
            const cleaned = this.stripReasoningContent(context);
            if (this.messages.length >= 40) {
                this.messages = this.messages.slice(-39).concat(cleaned);
            }
            else
                this.messages.push(cleaned);
            return this;
        }
        async createCreativeWork(taskDescription) {
            this.writeContext({ role: 'user', content: taskDescription });
            const details = [];
            let rejectionReason = '';
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
                if (!toolCalls || toolCalls.length === 0) {
                    if (choice.message?.content && choice.message.content.trim()) {
                        rejectionReason = choice.message.content;
                    }
                    this.writeContext(choice.message);
                    break;
                }
                this.writeContext(choice.message);
                for (const toolCall of toolCalls) {
                    console.log(`[${this.roleName}] 执行工具: ${toolCall.function.name}`);
                    const result = this.executeTool(toolCall);
                    this.writeContext({ role: 'tool', content: result, tool_call_id: toolCall.id });
                    this.collectDetail(toolCall, details);
                }
            }
            if (details.length === 0) {
                const reason = rejectionReason || '月华认为此次无需进行创作';
                console.log(`[${this.roleName}] 未产出作品，原因: ${reason}`);
                return reason;
            }
            const summary = this.buildSummary(details);
            console.log(`[${this.roleName}] 已完成 ${details.length} 件作品创作`);
            return summary;
        }
    }

    class DialogueRole extends ModelBuilder {
        async callMultimediaAndToolParsing(cache, source) {
            try {
                await source.LiteImageFile();
                source.learnerRole.consumeHistory().forEach(msg => source.unreadContext.push(msg));
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
                    const hasText = message.content.some((c) => c.type === 'text');
                    const hasImage = message.content.some((c) => c.type === 'image_url');
                    if (hasText && hasImage) {
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
            const prompt = this.buildContinuationPrompt();
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
                    if (base64Image && typeof base64Image === 'string' && base64Image.length > 0) {
                        const message = {
                            role: "user",
                            content: [
                                { type: "text", text: textContent },
                                { type: "image_url", image_url: { url: base64Image } }
                            ]
                        };
                        this.messages.push(message);
                        console.log(`[工具调用] ${functionName} 返回图片数据，长度=${base64Image.length} 字节`);
                    }
                    else
                        this.messages.push({ role: "tool", content: textContent, tool_call_id: toolCall.id });
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
        buildContinuationPrompt() {
            const pick = (arr) => arr[Math.floor(Math.random() * arr.length)];
            const interests = [
                '旅行', '游戏', '音乐', '电影', '书籍', '动漫', '美食', '运动',
                '摄影', '绘画', '手工', '编程', '天文', '历史', '哲学', '科技',
                '宠物', '园艺', '穿搭', '舞蹈', '乐器', '写作', '钓鱼', '骑行',
            ];
            const doing = [
                '正在喝一杯热茶', '正在窗边发呆', '刚刚整理完房间', '正在浏览网页',
                '正在听一首新歌', '刚刚看完一段视频', '正在翻看旧照片', '正在写日记',
                '正在做手工', '正在画一幅画', '正在弹琴', '正在做一道菜',
                '正在散步', '正在看窗外的风景', '正在刷手机', '正在整理书架',
            ];
            const wantTo = [
                '想去海边看日落', '想学一门新乐器', '想去看一场演唱会', '想去爬山',
                '想养一只猫', '想尝试做一道新菜', '想去看极光', '想去逛博物馆',
                '想学画画', '想去露营', '想写一首诗', '想去看一场电影',
                '想去游乐园', '想学跳舞', '想去看樱花', '想开一家小店',
            ];
            const location = [
                '坐在窗边的书桌前', '窝在沙发里', '躺在草地上', '站在阳台上',
                '靠在床头', '坐在咖啡馆的角落', '在公园的长椅上', '在图书馆里',
                '在厨房里', '在工作室里', '在花园里', '在天台上',
            ];
            const action = [
                '伸了个懒腰', '托着下巴', '揉了揉眼睛', '转着手里的笔',
                '轻轻哼着歌', '翘着二郎腿', '抱着抱枕', '拨弄着头发',
                '用手指敲着桌面', '晃着双脚', '靠在椅背上', '侧着头',
            ];
            const mood = [
                '心情很放松', '觉得有点无聊', '心情特别好', '有点小期待',
                '感觉懒洋洋的', '很平静', '有点好奇', '心情不错',
                '稍微有点困', '精神很好', '有点怀旧', '感觉很温暖',
            ];
            const pools = [
                { label: '兴趣', items: interests },
                { label: '正在做', items: doing },
                { label: '想做', items: wantTo },
                { label: '位置', items: location },
                { label: '动作', items: action },
                { label: '心情', items: mood },
            ];
            const count = 2 + Math.floor(Math.random() * 2);
            const shuffled = [...pools].sort(() => Math.random() - 0.5);
            const selected = shuffled.slice(0, count);
            const parts = selected.map(p => pick(p.items));
            const prefix = pick(['你', '现在你', '此刻你', '这会儿你',]);
            const suffix = pick(['，聊点什么吧~', '，来聊聊吧~', '，说说看吧~', '，展开聊聊？', '，有什么想说的吗？', '，分享一下呗~',]);
            return `${prefix}${parts.join('，')}${suffix}`;
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
        constructor() {
            super(fileView('prompts/painterRole.md')[0]);
        }
        get roleName() { return '画家'; }
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
            if (paintings.length === 0)
                return '月华没有绘制任何作品';
            const parts = [];
            for (let i = 0; i < paintings.length; i++) {
                const p = paintings[i];
                if (p.toolName === 'self_portrait') {
                    let desc = '月华绘制了一幅自画像';
                    if (p.expression)
                        desc += `，展现了${p.expression}`;
                    if (p.environment)
                        desc += `，背景是${p.environment}`;
                    parts.push(desc + '。');
                }
                else {
                    parts.push(`月华绘制了一幅图像：${p.promptSummary}。`);
                }
            }
            parts.push('图像已通过前端推送给用户。');
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
        MAX_ITERATIONS = 5;
        musicTool = [
            {
                type: "function",
                function: {
                    name: "compose_music",
                    description: "创作音乐作品并生成ABC记谱法格式的乐谱，后端将通过SoundFont专业音色库渲染为真实乐器音色的音频。必须生成完整、可直接播放的ABC乐谱，包含和弦伴奏与多声部编配。",
                    parameters: {
                        type: "object",
                        properties: {
                            "title": {
                                type: "string",
                                description: "音乐作品标题"
                            },
                            "instruments": {
                                type: "string",
                                description: "使用的乐器列表，多个乐器用逗号分隔。优先使用琴类乐器：钢琴(piano)、竖琴(harp)、吉他(guitar)、大提琴(cello)、小提琴(violin)。也可以使用长笛(flute)、单簧管(clarinet)、双簧管(oboe)、小号(trumpet)。推荐组合：'钢琴'独奏、'钢琴,大提琴'二重奏、'竖琴,小提琴'等。"
                            },
                            "tempo": {
                                type: "number",
                                description: "演奏速度（BPM）。抒情曲建议60-80，轻快曲建议90-120，激昂曲建议130-150。默认100"
                            },
                            "structure": {
                                type: "string",
                                description: "音乐段落结构，如：'前奏(4小节)-A段主旋律(8小节)-B段展开(8小节)-A'再现(8小节)-尾声(4小节)'"
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
                                description: `ABC记谱法格式的完整乐谱。后端使用FluidSynth+SoundFont渲染真实乐器音色，GM标准乐器编号确保音色正确。

=== 基础格式 ===
X:1
T:作品标题
M:拍号
L:默认音符时值(如 1/8)
Q:速度标记(如 1/4=100)
K:调号

=== 音符规则 ===
音名: C D E F G A B（大写=中低音区, 小写cdefgab=高八度, 加逗号=低八度如C,D,）
升降号: ^升半音(如^C)  _降半音(如_B)
时值: 数字后缀=倍数(C2=两倍)  /数字=分数(C/2=一半)
小节线: | 分隔  || 双线  |] 结束
休止符: z

=== 和弦伴奏（核心要求！必须包含！） ===
和弦用方括号包裹同时发音的音符，如 [CEG] 表示C大三和弦同时演奏。
和弦必须贯穿全曲，形成完整的伴奏织体：

1. 柱式和弦: [C,,E,,G,,]2 [C,,E,,G,,]2 | [F,,A,,C,]2 [G,,B,,D,]2 |
2. 分解和弦(琶音): C,,2 E,2 G,2 c2 | F,,2 A,2 C2 f2 |
3. 阿尔贝蒂低音: C,2 G,2 E,2 G,2 | F,2 C2 A,2 C2 |

=== 多声部记谱（多乐器时必须使用） ===
[V:1] = 第一个乐器（通常是旋律声部）
[V:2] = 第二个乐器（通常是和弦伴奏/低音声部）
各声部小节对齐、同步演奏。

=== 表情记号（使音乐富有表现力！） ===
力度: !pp!极弱 !p!弱 !mp!中弱 !mf!中强 !f!强 !ff!极强
运音法: .断奏 >重音 -保持

=== GM乐器编号（通过 %%prog 声部 程序号 指定） ===
钢琴0  竖琴46  吉他24  大提琴42  小提琴40
长笛73  单簧管71  双簧管68  小号56  合成器80
示例: %%prog 1 0  %%prog 2 42

=== 完整示例：钢琴独奏（含和弦伴奏） ===
%%prog 1 0
X:1
T:晨光曲
M:4/4
L:1/8
Q:1/4=90
K:C
!mp! [V:1] c2 e2 g2 e2 | f2 a2 g2 e2 | d2 f2 e2 d2 | c4 z4 |
!mf! [V:2] [C,,E,,G,,]4 | [F,,A,,C,]4 | [G,,B,,D,]4 | [C,,E,,G,,]4 |

=== 完整示例：钢琴+大提琴二重奏 ===
%%prog 1 0
%%prog 2 42
X:1
T:夜色温柔
M:4/4
L:1/8
Q:1/4=80
K:Am
!mp! [V:1] e2 a2 c'2 a2 | d2 f2 e2 d2 | c2 e2 d2 ^c2 | A4 z4 |
!p!   [V:2] [A,,2E,2A,2]2 | [D,,2A,,2D,2]2 | [E,,2B,,2E,2]2 | [A,,,2E,,2A,,2]2 |

关键原则:
- 必须包含和弦伴奏，不可只有单音旋律线
- 左手/第二声部使用和弦或分解和弦提供和声支撑
- 合理使用力度变化（开头mp、高潮f、结尾p）
- 旋律要有乐句呼吸感（每4-8小节一个乐句，句末用稍长时值或休止）`
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
        constructor() {
            super(fileView('prompts/musicianRole.md')[0]);
        }
        get roleName() { return '音乐家'; }
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
            if (pieces.length === 0)
                return '月华没有演奏任何作品';
            const parts = [];
            for (let i = 0; i < pieces.length; i++) {
                const p = pieces[i];
                let desc = `月华演奏了《${p.title}》`;
                const details = [];
                if (p.instruments)
                    details.push(`使用${p.instruments}`);
                if (p.key)
                    details.push(`${p.key}${p.key === p.key.toLowerCase() ? '小调' : '大调'}`);
                if (p.tempo > 0)
                    details.push(`${p.tempo}BPM`);
                if (p.structure)
                    details.push(`结构为${p.structure}`);
                if (details.length > 0)
                    desc += `（${details.join('，')}）`;
                parts.push(desc + '。');
            }
            parts.push('乐谱已通过音乐播放器推送给用户，可以查看和播放。');
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
                let enrichedAbc = this.injectInstrumentDirective(abcNotation, instruments);
                const hasX = /^X:\s*\d+/m.test(enrichedAbc);
                const hasT = /^T:\s*.+/m.test(enrichedAbc);
                const hasK = /^K:\s*.+/m.test(enrichedAbc);
                if (!hasX || !hasK) {
                    console.warn('[音乐家] ABC乐谱缺少必要字段 (X:/K:)，尝试自动补充');
                    if (!hasX)
                        enrichedAbc = 'X:1\n' + enrichedAbc;
                    if (!hasT)
                        enrichedAbc = enrichedAbc.replace(/^(X:\s*\d+\n)/m, `$1T:${title}\n`);
                    if (!hasK)
                        enrichedAbc = enrichedAbc.replace(/^(T:.*\n)/m, `$1K:C\n`);
                }
                const pushSuccess = pushContext('music', enrichedAbc, '');
                if (!pushSuccess) {
                    console.warn('[音乐家] 推送乐谱到前端失败');
                }
                console.log(`[音乐家] 乐谱推送成功，长度: ${enrichedAbc.length} 字符，乐器: ${instruments || '默认'}，后端音频渲染已自动触发`);
                return `音乐作品"${title}"创作成功。乐谱已推送到前端展示，音频正在通过 SoundFont 专业音色库渲染，稍后将自动播放。`;
            }
            catch (error) {
                console.error('[音乐家] 音乐创作处理异常:', error);
                return `音乐创作异常: ${error}`;
            }
        }
        injectInstrumentDirective(abcNotation, instruments) {
            if (!instruments)
                return abcNotation;
            const list = instruments
                .replace(/，/g, ',')
                .split(',')
                .map(s => s.trim())
                .filter(Boolean);
            if (list.length === 0)
                return abcNotation;
            if (/^%%(?:voice|prog)\s+/m.test(abcNotation))
                return abcNotation;
            const gmPrograms = {
                '钢琴': 0, 'piano': 0,
                '竖琴': 46, 'harp': 46,
                '吉他': 24, 'guitar': 24,
                '大提琴': 42, 'cello': 42,
                '小提琴': 40, 'violin': 40,
                '长笛': 73, 'flute': 73,
                '单簧管': 71, 'clarinet': 71,
                '双簧管': 68, 'oboe': 68,
                '小号': 56, 'trumpet': 56,
                '合成器': 80, 'synth': 80,
            };
            const directives = [];
            for (let i = 0; i < list.length; i++) {
                const inst = list[i];
                const voiceNum = i + 1;
                const prog = gmPrograms[inst.toLowerCase()] ?? gmPrograms[inst] ?? 0;
                directives.push(`%%prog ${voiceNum} ${prog}`);
                directives.push(`%%voice ${voiceNum} ${inst}`);
            }
            const directive = directives.join('\n') + '\n';
            const xMatch = abcNotation.match(/^X:\s*\d+/m);
            if (xMatch && xMatch.index !== undefined) {
                const before = abcNotation.substring(0, xMatch.index);
                const after = abcNotation.substring(xMatch.index);
                return before + directive + after;
            }
            return directive + abcNotation;
        }
    }

    const learnerKeywords = [
        /回忆(?:一(?:下|回忆))?/, /想想?(?:看|起|到)/, /记不记得/,
        /还记得/, /以前(?:说过|聊过|讨论过|提过|提到)/,
        /上次(?:说|聊|讨论|提|提到)/,
        /搜索/, /搜(?:一搜|一下)/, /深入(?:了解|分析|研究)/,
        /详细(?:了解|分析|说明|解释)/, /分析(?:一(?:下|分析))?/,
        /(?:资料|文献|论文|报告|数据|统计)/, /调查(?:一(?:下|调查))?/,
        /核实/, /验证/,
        /查(?:一查|一下|询|找|找找|看看)/,
        /(?:帮我|给我|为我|替我)(?:查|搜索|找|调查|研究|检索|查询)/,
        /(?:真|假|正确|错误|靠谱|可靠)/,
    ];
    const recallKeywords = [
        /回忆(?:一(?:下|回忆))?/, /想想?(?:看|起|到)/, /记不记得/,
        /还记得/, /以前(?:说过|聊过|讨论过|提过|提到)/,
        /上次(?:说|聊|讨论|提|提到)/,
    ];
    function isRecallIntent(texts) {
        return texts.some(text => recallKeywords.some(kw => kw.test(text)));
    }
    let learnerInitialized = false;
    function ensureLearnerInitialized() {
        if (learnerInitialized)
            return true;
        if (!learnerIsReady()) {
            const [success, err] = learnerInit(OnlyData.systemUrl, OnlyData.SystemKey, OnlyData.MultimodalName, 4096, 0.7, OnlyData.systemUrl, OnlyData.SystemKey, OnlyData.EmbeddingName);
            if (err) {
                console.error('[学习者] 初始化失败:', err);
                return false;
            }
        }
        learnerInitialized = true;
        console.log('[学习者] 初始化完成');
        return true;
    }
    class LearnerRole {
        messages = [];
        consumeHistory() {
            const result = [...this.messages];
            this.messages = [];
            return result;
        }
        executeLearner(unreadContext) {
            const unreadTexts = this.extractTexts(unreadContext);
            if (!unreadTexts.some(text => learnerKeywords.some(keyword => keyword.test(text)))) {
                return true;
            }
            if (!ensureLearnerInitialized())
                return true;
            const mode = isRecallIntent(unreadTexts) ? 'recall' : 'full';
            console.log('[学习者] 开始执行研究, 模式:', mode);
            const [report, error] = learnerExecute(unreadTexts, mode);
            if (error) {
                console.error('[学习者] 执行失败:', error);
                return true;
            }
            if (report && report.trim().length > 0) {
                this.messages.push({ role: 'user', content: report });
                console.log('[学习者] 已将研究报告写入历史');
            }
            return false;
        }
        extractTexts(messages) {
            const texts = [];
            for (const msg of messages) {
                if (typeof msg.content === 'string') {
                    texts.push(msg.content);
                }
                else if (Array.isArray(msg.content)) {
                    msg.content.forEach((item) => {
                        if (item.type === 'text')
                            texts.push(item.text);
                    });
                }
            }
            return texts;
        }
        dumpContext(dialogueMessages, unreadContext, outputPath) {
            const path = outputPath || 'agent_debug_学习者.json';
            const timestamp = new Date().toLocaleString('zh-CN', {
                year: 'numeric', month: '2-digit', day: '2-digit',
                hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false
            });
            const unreadTexts = this.extractTexts(unreadContext);
            const mode = isRecallIntent(unreadTexts) ? 'recall' : 'full';
            const snapshot = {
                timestamp,
                role: '学习者',
                mode,
                ownMessagesCount: this.messages.length,
                ownMessages: this.messages.map((msg, idx) => {
                    const content = typeof msg.content === 'string'
                        ? msg.content
                        : JSON.stringify(msg.content);
                    return {
                        index: idx,
                        role: msg.role,
                        contentPreview: content.length > 500 ? content.slice(0, 500) + '...' : content,
                        contentLength: content.length,
                    };
                }),
                dialogueMessagesCount: dialogueMessages.length,
                dialogueMessages: dialogueMessages.slice(-15).map((msg, idx) => {
                    const content = typeof msg.content === 'string'
                        ? msg.content
                        : JSON.stringify(msg.content);
                    return {
                        index: idx,
                        role: msg.role,
                        contentPreview: content.length > 300 ? content.slice(0, 300) + '...' : content,
                    };
                }),
                unreadContextCount: unreadContext.length,
                unreadContext: unreadContext.slice(-10).map((msg, idx) => {
                    const content = typeof msg.content === 'string'
                        ? msg.content
                        : JSON.stringify(msg.content);
                    return {
                        index: idx,
                        role: msg.role,
                        contentPreview: content.length > 300 ? content.slice(0, 300) + '...' : content,
                    };
                }),
                learnerInitialized,
            };
            const [, error] = saveDebugFile(path, JSON.stringify(snapshot, null, 2));
            if (error) {
                console.error('[学习者] 导出 TS 层上下文失败:', error);
                return '';
            }
            if (learnerInitialized) {
                const goPath = path.replace('.json', '_go.json');
                const [, goError] = learnerDumpContext(unreadTexts, mode, goPath);
                if (goError) {
                    console.error('[学习者] 导出 Go 层上下文失败:', goError);
                }
            }
            console.log('[学习者] 上下文快照已导出:', path);
            return path;
        }
    }

    const PERSON_PREFIX = '[人物档案 - ';
    const EVENT_PREFIX = '[事件档案 - ';
    const SELF_PREFIX = '[自我档案]';
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
        formatMessages(records) {
            return records.map((msg, idx) => {
                const content = typeof msg.content === 'string'
                    ? msg.content
                    : JSON.stringify(msg.content);
                const preview = content.length > 600 ? content.slice(0, 600) + '...' : content;
                return `[消息${idx + 1}] 角色:${msg.role} | ${preview}`;
            }).join('\n');
        }
        buildPersonExtractPrompt(records) {
            const currentTime = this.getCurrentTime();
            const currentLocation = this.getCurrentLocation();
            return `请从以下对话消息中提取所有人物信息，为每个人物生成一份档案。

【对话消息】
${this.formatMessages(records)}

【系统上下文】
- 当前时间: ${currentTime}
- 当前位置: ${currentLocation}

【提取规则】
1. 从消息中识别所有被提及的人物（包括说话者自身），为每个独立人物生成一份档案
2. 同一个人物不要重复提取
3. 若某字段在消息中未提及，则留空（不要编造）
4. 字段说明：
   - name: 人物名称（必填，如"月华"、"星光阁"）
   - nickname: 外号或别名
   - gender: 性别
   - personality: 性格特征描述
   - clothing: 服饰特点描述
   - location: 当前所在地点
   - dietaryPrefs: 饮食偏好
   - currentActivity: 当前正在进行的活动

【输出格式】
请输出 JSON 对象，包含 items 数组：
\`\`\`json
{
  "items": [
    {
      "name": "人物名称",
      "nickname": "外号",
      "gender": "性别",
      "personality": "性格特征",
      "clothing": "服饰特点",
      "location": "所在地点",
      "dietaryPrefs": "饮食偏好",
      "currentActivity": "当前活动"
    }
  ]
}
\`\`\`

仅输出 JSON，不要包含其他说明文字。`;
        }
        buildPersonMergePrompt(existingArchive, newInfo) {
            return `请将以下人物档案的新信息合并到已有档案中，补充和更新档案内容。

【已有档案】
${existingArchive}

【新信息】
${JSON.stringify(newInfo, null, 2)}

【合并规则】
1. 保留已有档案中所有仍然有效的信息
2. 用新信息补充和更新对应字段
3. 若新信息与已有信息冲突，以新信息为准（新信息更有时效性）
4. 不要删除已有档案中未与新信息冲突的字段

【输出格式】
请输出合并后的完整档案 JSON：
\`\`\`json
{
  "name": "人物名称",
  "nickname": "外号",
  "gender": "性别",
  "personality": "性格特征",
  "clothing": "服饰特点",
  "location": "所在地点",
  "dietaryPrefs": "饮食偏好",
  "currentActivity": "当前活动"
}
\`\`\`

仅输出 JSON，不要包含其他说明文字。`;
        }
        buildEventExtractPrompt(records) {
            const currentTime = this.getCurrentTime();
            const currentLocation = this.getCurrentLocation();
            return `请从以下对话消息中提取所有事件信息，为每个独立事件生成一份档案。

【对话消息】
${this.formatMessages(records)}

【系统上下文】
- 当前时间: ${currentTime}
- 当前位置: ${currentLocation}

【提取规则】
1. 从消息中识别所有已发生或正在发生的事件
2. 仅提取具有明确信息的事件，不要编造
3. 字段说明：
   - name: 事件简称（必填，如"星月祭典"、"代码审查"）
   - type: 事件类型（如"社交活动"、"工作会议"、"个人事务"）
   - time: 发生时间（从消息中提取，若无则使用当前时间）
   - location: 发生地点
   - keyNotes: 关键注意事项或重要细节

【输出格式】
请输出 JSON 对象，包含 items 数组：
\`\`\`json
{
  "items": [
    {
      "name": "事件简称",
      "type": "事件类型",
      "time": "发生时间",
      "location": "发生地点",
      "keyNotes": "关键注意事项"
    }
  ]
}
\`\`\`

仅输出 JSON，不要包含其他说明文字。`;
        }
        buildEventMergePrompt(existingArchive, newInfo) {
            return `请将以下事件档案的新信息合并到已有档案中，补充和更新档案内容。

【已有档案】
${existingArchive}

【新信息】
${JSON.stringify(newInfo, null, 2)}

【合并规则】
1. 保留已有档案中所有仍然有效的信息
2. 用新信息补充和更新对应字段
3. 若新信息与已有信息冲突，以新信息为准

【输出格式】
请输出合并后的完整档案 JSON：
\`\`\`json
{
  "name": "事件简称",
  "type": "事件类型",
  "time": "发生时间",
  "location": "发生地点",
  "keyNotes": "关键注意事项"
}
\`\`\`

仅输出 JSON，不要包含其他说明文字。`;
        }
        buildSelfExtractPrompt(records) {
            return `请从以下对话消息中提取关于"月华"（即说话者自身）的当前状态信息。

【对话消息】
${this.formatMessages(records)}

【提取规则】
1. 仅提取关于月华自身的信息
2. 字段说明：
   - mood: 当前心情状态（如"开心"、"疲惫"、"专注"）
   - clothing: 当前服饰描述
   - activity: 正在进行的活动
   - needs: 当前需求或期望获取的物品/信息
3. 若某字段在消息中未提及，则留空

【输出格式】
请输出 JSON 对象：
\`\`\`json
{
  "mood": "心情状态",
  "clothing": "服饰描述",
  "activity": "正在进行的活动",
  "needs": "当前需求"
}
\`\`\`

仅输出 JSON，不要包含其他说明文字。`;
        }
        buildSelfMergePrompt(existingArchive, newInfo) {
            return `请将以下自我档案的新信息合并到已有档案中，补充和更新档案内容。

【已有档案】
${existingArchive}

【新信息】
${JSON.stringify(newInfo, null, 2)}

【合并规则】
1. 保留已有档案中所有仍然有效的信息
2. 用新信息补充和更新对应字段
3. 若新信息与已有信息冲突，以新信息为准

【输出格式】
请输出合并后的完整档案 JSON：
\`\`\`json
{
  "mood": "心情状态",
  "clothing": "服饰描述",
  "activity": "正在进行的活动",
  "needs": "当前需求"
}
\`\`\`

仅输出 JSON，不要包含其他说明文字。`;
        }
        formatPersonArchive(archive) {
            const fields = [];
            if (archive.name)
                fields.push(`名称: ${archive.name}`);
            if (archive.nickname)
                fields.push(`外号: ${archive.nickname}`);
            if (archive.gender)
                fields.push(`性别: ${archive.gender}`);
            if (archive.personality)
                fields.push(`性格: ${archive.personality}`);
            if (archive.clothing)
                fields.push(`服饰: ${archive.clothing}`);
            if (archive.location)
                fields.push(`地点: ${archive.location}`);
            if (archive.dietaryPrefs)
                fields.push(`饮食: ${archive.dietaryPrefs}`);
            if (archive.currentActivity)
                fields.push(`活动: ${archive.currentActivity}`);
            return `${PERSON_PREFIX}${archive.name}]\n${fields.join('\n')}`;
        }
        formatEventArchive(archive) {
            const fields = [];
            if (archive.name)
                fields.push(`事件: ${archive.name}`);
            if (archive.type)
                fields.push(`类型: ${archive.type}`);
            if (archive.time)
                fields.push(`时间: ${archive.time}`);
            if (archive.location)
                fields.push(`地点: ${archive.location}`);
            if (archive.keyNotes)
                fields.push(`注意事项: ${archive.keyNotes}`);
            return `${EVENT_PREFIX}${archive.name}]\n${fields.join('\n')}`;
        }
        formatSelfArchive(archive) {
            const fields = [];
            if (archive.mood)
                fields.push(`心情: ${archive.mood}`);
            if (archive.clothing)
                fields.push(`服饰: ${archive.clothing}`);
            if (archive.activity)
                fields.push(`活动: ${archive.activity}`);
            if (archive.needs)
                fields.push(`需求: ${archive.needs}`);
            return `${SELF_PREFIX}\n${fields.join('\n')}`;
        }
        parsePersonArchive(content) {
            try {
                const result = { name: '' };
                const lines = content.replace(PERSON_PREFIX, '').replace(/\]$/, '').split('\n');
                const headerMatch = content.match(/\[人物档案 - (.+?)\]/);
                if (headerMatch)
                    result.name = headerMatch[1];
                for (const line of lines) {
                    if (line.startsWith('名称: '))
                        result.name = result.name || line.slice(4);
                    else if (line.startsWith('外号: '))
                        result.nickname = line.slice(4);
                    else if (line.startsWith('性别: '))
                        result.gender = line.slice(4);
                    else if (line.startsWith('性格: '))
                        result.personality = line.slice(4);
                    else if (line.startsWith('服饰: '))
                        result.clothing = line.slice(4);
                    else if (line.startsWith('地点: '))
                        result.location = line.slice(4);
                    else if (line.startsWith('饮食: '))
                        result.dietaryPrefs = line.slice(4);
                    else if (line.startsWith('活动: '))
                        result.currentActivity = line.slice(4);
                }
                return result.name ? result : null;
            }
            catch {
                return null;
            }
        }
        parseEventArchive(content) {
            try {
                const result = { name: '' };
                const headerMatch = content.match(/\[事件档案 - (.+?)\]/);
                if (headerMatch)
                    result.name = headerMatch[1];
                const lines = content.replace(EVENT_PREFIX, '').replace(/\]$/, '').split('\n');
                for (const line of lines) {
                    if (line.startsWith('事件: '))
                        result.name = result.name || line.slice(4);
                    else if (line.startsWith('类型: '))
                        result.type = line.slice(4);
                    else if (line.startsWith('时间: '))
                        result.time = line.slice(4);
                    else if (line.startsWith('地点: '))
                        result.location = line.slice(4);
                    else if (line.startsWith('注意事项: '))
                        result.keyNotes = line.slice(4);
                }
                return result.name ? result : null;
            }
            catch {
                return null;
            }
        }
        parseSelfArchive(content) {
            try {
                const result = {};
                const lines = content.replace(SELF_PREFIX, '').split('\n');
                for (const line of lines) {
                    if (line.startsWith('心情: '))
                        result.mood = line.slice(4);
                    else if (line.startsWith('服饰: '))
                        result.clothing = line.slice(4);
                    else if (line.startsWith('活动: '))
                        result.activity = line.slice(4);
                    else if (line.startsWith('需求: '))
                        result.needs = line.slice(4);
                }
                return (result.mood || result.clothing || result.activity || result.needs) ? result : null;
            }
            catch {
                return null;
            }
        }
    }
    class Toolchain extends Prompt {
        queryMemory(queryText, topK = 10) {
            if (!queryText || queryText.trim().length === 0)
                return [];
            const [results, error] = memoryQuery('lunar_messages', queryText.trim(), topK);
            if (error) {
                console.error('[编纂者] 记忆库查询失败:', error);
                return [];
            }
            return results || [];
        }
        queryArchiveByPrefix(prefix, topK = 50) {
            const allResults = this.queryMemory(prefix, topK);
            return allResults.filter(r => r.content.startsWith(prefix));
        }
        deleteRecords(ids) {
            const uniqueIds = [...new Set(ids.filter(id => id && id.trim()))];
            if (uniqueIds.length === 0)
                return;
            console.log(`[编纂者] 删除 ${uniqueIds.length} 条旧档案`);
            for (const id of uniqueIds) {
                const [, error] = memoryDelete('lunar_messages', id.trim());
                if (error)
                    console.error(`[编纂者] 删除记录 ${id} 失败:`, error);
                else
                    console.log(`[编纂者] 已删除记录 ${id}`);
            }
        }
        writeArchive(content) {
            if (!content || content.trim().length === 0)
                return;
            const [, error] = memoryAdd('lunar_messages', 'assistant', content.trim());
            if (error)
                console.error('[编纂者] 写入档案失败:', error);
            else
                console.log(`[编纂者] 已写入档案: ${content.slice(0, 60)}...`);
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
        runLLM(prompt) {
            this.coverContext({ role: 'user', content: prompt });
            this.runtimeMessages = [];
            try {
                const response = this.run([], []);
                return response.body?.choices?.[0]?.message?.content || '';
            }
            catch (error) {
                console.error('[编纂者] LLM 推理失败:', error);
                return '';
            }
        }
        runMergeLLM(prompt) {
            const content = this.runLLM(prompt);
            if (!content)
                return null;
            return this.parseJsonResponse(content);
        }
    }
    class OrganizeRole extends Toolchain {
        ARCHIVE_QUERY_TOPK = 50;
        constructor() {
            super(fileView('prompts/organizeRole.md')[0]);
        }
        organizeHistoricalRecords() {
            console.log('[编纂者] 开始档案收集与整理');
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
            const records = [...OnlyData.unreadRecords];
            try {
                this.processPersonArchives(records);
                this.processEventArchives(records);
                this.processSelfArchive(records);
                console.log('[编纂者] 档案整理完成');
                OnlyData.unreadRecords = [];
            }
            catch (error) {
                console.error('[编纂者] 档案整理失败，保留未读记录待下次重试:', error);
            }
        }
        processPersonArchives(records) {
            console.log('[编纂者] === 阶段一：人物档案处理 ===');
            const prompt = this.buildPersonExtractPrompt(records);
            const content = this.runLLM(prompt);
            if (!content) {
                console.log('[编纂者] 人物档案提取未获得有效结果');
                return;
            }
            const result = this.parseJsonResponse(content);
            if (!result || !result.items || result.items.length === 0) {
                console.log('[编纂者] 未提取到人物信息');
                return;
            }
            console.log(`[编纂者] 提取到 ${result.items.length} 个人物档案`);
            for (const person of result.items) {
                if (!person.name)
                    continue;
                this.processSinglePersonArchive(person);
            }
        }
        processSinglePersonArchive(newInfo) {
            const prefix = `${PERSON_PREFIX}${newInfo.name}]`;
            console.log(`[编纂者] 处理人物档案: ${newInfo.name}`);
            const existingRecords = this.queryArchiveByPrefix(prefix, this.ARCHIVE_QUERY_TOPK);
            if (existingRecords.length === 0) {
                const archiveText = this.formatPersonArchive(newInfo);
                this.writeArchive(archiveText);
                console.log(`[编纂者] 新增人物档案: ${newInfo.name}`);
                return;
            }
            console.log(`[编纂者] 发现 ${newInfo.name} 的旧档案 ${existingRecords.length} 条，执行合并`);
            for (const record of existingRecords) {
                const oldArchive = this.parsePersonArchive(record.content);
                if (!oldArchive) {
                    this.deleteRecords([record.id]);
                    continue;
                }
                const mergePrompt = this.buildPersonMergePrompt(record.content, newInfo);
                const merged = this.runMergeLLM(mergePrompt);
                if (merged && merged.name) {
                    this.deleteRecords([record.id]);
                    const archiveText = this.formatPersonArchive(merged);
                    this.writeArchive(archiveText);
                    console.log(`[编纂者] 合并更新人物档案: ${merged.name}`);
                }
                else {
                    const archiveText = this.formatPersonArchive(newInfo);
                    this.writeArchive(archiveText);
                    console.log(`[编纂者] 合并失败，新信息作为补充写入: ${newInfo.name}`);
                }
            }
        }
        processEventArchives(records) {
            console.log('[编纂者] === 阶段二：事件档案处理 ===');
            const prompt = this.buildEventExtractPrompt(records);
            const content = this.runLLM(prompt);
            if (!content) {
                console.log('[编纂者] 事件档案提取未获得有效结果');
                return;
            }
            const result = this.parseJsonResponse(content);
            if (!result || !result.items || result.items.length === 0) {
                console.log('[编纂者] 未提取到事件信息');
                return;
            }
            console.log(`[编纂者] 提取到 ${result.items.length} 个事件档案`);
            for (const event of result.items) {
                if (!event.name)
                    continue;
                this.processSingleEventArchive(event);
            }
        }
        processSingleEventArchive(newInfo) {
            const prefix = `${EVENT_PREFIX}${newInfo.name}]`;
            console.log(`[编纂者] 处理事件档案: ${newInfo.name}`);
            const existingRecords = this.queryArchiveByPrefix(prefix, this.ARCHIVE_QUERY_TOPK);
            if (existingRecords.length === 0) {
                const archiveText = this.formatEventArchive(newInfo);
                this.writeArchive(archiveText);
                console.log(`[编纂者] 新增事件档案: ${newInfo.name}`);
                return;
            }
            console.log(`[编纂者] 发现 ${newInfo.name} 的旧档案 ${existingRecords.length} 条，执行合并`);
            for (const record of existingRecords) {
                const mergePrompt = this.buildEventMergePrompt(record.content, newInfo);
                const merged = this.runMergeLLM(mergePrompt);
                if (merged && merged.name) {
                    this.deleteRecords([record.id]);
                    const archiveText = this.formatEventArchive(merged);
                    this.writeArchive(archiveText);
                    console.log(`[编纂者] 合并更新事件档案: ${merged.name}`);
                }
                else {
                    const archiveText = this.formatEventArchive(newInfo);
                    this.writeArchive(archiveText);
                    console.log(`[编纂者] 合并失败，新信息作为补充写入: ${newInfo.name}`);
                }
            }
        }
        processSelfArchive(records) {
            console.log('[编纂者] === 阶段三：自我档案处理 ===');
            const prompt = this.buildSelfExtractPrompt(records);
            const content = this.runLLM(prompt);
            if (!content) {
                console.log('[编纂者] 自我档案提取未获得有效结果');
                return;
            }
            const newInfo = this.parseJsonResponse(content);
            if (!newInfo || (!newInfo.mood && !newInfo.clothing && !newInfo.activity && !newInfo.needs)) {
                console.log('[编纂者] 未提取到有效的自我信息');
                return;
            }
            console.log('[编纂者] 处理自我档案');
            const existingRecords = this.queryArchiveByPrefix(SELF_PREFIX, this.ARCHIVE_QUERY_TOPK);
            if (existingRecords.length === 0) {
                const archiveText = this.formatSelfArchive(newInfo);
                this.writeArchive(archiveText);
                console.log('[编纂者] 新增自我档案');
                return;
            }
            console.log(`[编纂者] 发现自我旧档案 ${existingRecords.length} 条，执行合并`);
            for (const record of existingRecords) {
                const mergePrompt = this.buildSelfMergePrompt(record.content, newInfo);
                const merged = this.runMergeLLM(mergePrompt);
                if (merged && (merged.mood || merged.clothing || merged.activity || merged.needs)) {
                    this.deleteRecords([record.id]);
                    const archiveText = this.formatSelfArchive(merged);
                    this.writeArchive(archiveText);
                    console.log('[编纂者] 合并更新自我档案');
                }
                else {
                    const archiveText = this.formatSelfArchive(newInfo);
                    this.writeArchive(archiveText);
                    console.log('[编纂者] 合并失败，新信息作为补充写入');
                }
            }
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

    class ViewerRole extends ModelBuilder {
        BATCH_SIZE = 20;
        SECONDARY_SUMMARY_INTERVAL = 5;
        MAX_ROUNDS = 40;
        constructor() {
            super(fileView('prompts/viewerRole.md')[0]);
        }
        async watchVideo(keyframes) {
            const totalFrames = Math.min(keyframes.length, this.MAX_ROUNDS * this.BATCH_SIZE);
            const totalRounds = Math.ceil(totalFrames / this.BATCH_SIZE);
            console.log(`[观影者] 开始观看视频，共 ${totalFrames} 帧，${totalRounds} 轮`);
            const evaluations = [];
            const secondarySummaries = [];
            for (let round = 0; round < totalRounds; round++) {
                const start = round * this.BATCH_SIZE;
                const batch = keyframes.slice(start, start + this.BATCH_SIZE);
                if (batch.length === 0)
                    break;
                console.log(`[观影者] 第 ${round + 1}/${totalRounds} 轮，处理 ${batch.length} 帧`);
                const evaluation = await this.evaluateBatch(batch, round + 1);
                if (evaluation) {
                    evaluations.push(evaluation);
                    console.log(`[观影者] 第 ${round + 1} 轮评价完成`);
                }
                const isLastRound = round === totalRounds - 1;
                const shouldSummarize = (round + 1) % this.SECONDARY_SUMMARY_INTERVAL === 0 || isLastRound;
                if (shouldSummarize && evaluations.length > 0) {
                    const recentEvals = evaluations.slice(-this.SECONDARY_SUMMARY_INTERVAL);
                    const secondarySummary = await this.generateSecondarySummary(recentEvals);
                    if (secondarySummary) {
                        secondarySummaries.push(secondarySummary);
                        console.log(`[观影者] 二次摘要完成（第 ${secondarySummaries.length} 份）`);
                    }
                }
            }
            if (secondarySummaries.length === 0) {
                console.warn('[观影者] 未产生任何二次摘要');
                return '月华观看了这个视频，但没有获取到足够的信息。';
            }
            if (secondarySummaries.length === 1) {
                console.log('[观影者] 仅一份摘要，直接返回');
                return secondarySummaries[0];
            }
            const finalSummary = await this.generateTertiarySummary(secondarySummaries);
            console.log('[观影者] 三次摘要（最终观后感）完成');
            return finalSummary || secondarySummaries.join('\n\n');
        }
        async evaluateBatch(frames, round) {
            const imageContents = frames.map(frame => ({
                type: 'image_url',
                image_url: { url: `data:image/jpeg;base64,${frame.data}` }
            }));
            const prompt = `请观看以下视频的第 ${round} 批关键帧（共 ${frames.length} 帧），以月华的身份描述你的观影感受和发现的关键信息。
时间范围：${frames[0]?.timestamp || '?'} ~ ${frames[frames.length - 1]?.timestamp || '?'}

请按以下格式输出：
【感受】
（以月华的第一人称写2-4句话）

【关键信息】
- 人物：...
- 场景：...
- 事件：...
- 变化：...`;
            this.coverContext({
                role: 'user',
                content: [
                    { type: 'text', text: prompt },
                    ...imageContents
                ]
            });
            this.runtimeMessages = [];
            let response;
            try {
                response = this.run([], []);
            }
            catch (error) {
                console.error(`[观影者] 第 ${round} 轮推理失败:`, error);
                return '';
            }
            const content = response.body?.choices?.[0]?.message?.content || '';
            if (!content.trim()) {
                console.warn(`[观影者] 第 ${round} 轮返回空内容`);
            }
            return content;
        }
        async generateSecondarySummary(evaluations) {
            const prompt = `请将以下 ${evaluations.length} 段视频片段评价整合为一份连贯的摘要。

【评价内容】
${evaluations.map((e, i) => `--- 片段${i + 1} ---\n${e}`).join('\n\n')}

【整合要求】
1. 保持月华的第一人称视角
2. 使用活泼可爱的女孩语气
3. 突出最重要的感受和发现
4. 按时间线或逻辑线组织内容
5. 字数控制在200-400字

仅输出摘要内容，不要包含其他说明文字。`;
            this.coverContext({ role: 'user', content: prompt });
            this.runtimeMessages = [];
            let response;
            try {
                response = this.run([], []);
            }
            catch (error) {
                console.error('[观影者] 二次摘要推理失败:', error);
                return '';
            }
            return response.body?.choices?.[0]?.message?.content || '';
        }
        async generateTertiarySummary(secondarySummaries) {
            const prompt = `请将以下 ${secondarySummaries.length} 份视频片段摘要整合为一份完整的视频观后感。

【片段摘要】
${secondarySummaries.map((s, i) => `--- 摘要${i + 1} ---\n${s}`).join('\n\n')}

【整合要求】
1. 以月华的身份，用第一人称视角写一份完整的观后感
2. 使用活泼可爱的女孩语气
3. 描述月华对整个视频的整体感受和印象
4. 包含视频的主要内容概述、最打动月华的部分、月华的个人感受
5. 字数控制在300-500字
6. 结构清晰，有开头、主体和结尾

仅输出观后感内容，不要包含其他说明文字。`;
            this.coverContext({ role: 'user', content: prompt });
            this.runtimeMessages = [];
            let response;
            try {
                response = this.run([], []);
            }
            catch (error) {
                console.error('[观影者] 三次摘要推理失败:', error);
                return '';
            }
            return response.body?.choices?.[0]?.message?.content || '';
        }
    }

    class AgentDefine {
        static instance;
        summaryRole = new ModelBuilder(fileView('prompts/summaryRole.md')[0]);
        descriptionRole = new ModelBuilder(fileView('prompts/descriptionRole.md')[0]);
        dialogueRole = new DialogueRole();
        learnerRole = new LearnerRole();
        painterRole = new PainterRole();
        musicianRole = new MusicianRole();
        viewerRole = new ViewerRole();
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
                console.log('[观影者] 命中视频缓存，直接返回');
                return;
            }
            console.log('[观影者] 开始提取视频关键帧...');
            const [images, error] = keyframe(videoUrl, './cache');
            if (images.length === 0 || error) {
                console.error('[观影者] 关键帧提取失败:', error);
                throw new Error('提取关键帧失败');
            }
            console.log(`[观影者] 关键帧提取完成，共 ${images.length} 帧`);
            const keyframes = images.map((frame) => ({
                data: frame.data,
                timestamp: frame.timestamp || ''
            }));
            console.log('[观影者] 开始观看视频...');
            const videoSummary = await this.viewerRole.watchVideo(keyframes);
            console.log('[观影者] 视频观看完成');
            if (videoSummary && videoSummary.trim().length > 0) {
                this.unreadContext.push({ role: 'user', content: videoSummary });
            }
            else {
                this.unreadContext.push({
                    role: 'user',
                    content: this.defaultAnswers[RandomFloor(0, this.defaultAnswers.length - 1)]
                });
            }
            if (userNeeds.trim().length > 0) {
                this.unreadContext.push({ role: 'user', content: userNeeds });
            }
            if (videoSummary) {
                savePromptToKnowledge(videoUrl, videoSummary);
                console.log('[观影者] 观后感已缓存');
            }
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
        dumpAllContexts(outputDir) {
            if (!OnlyData.debugMode)
                return [];
            const dir = outputDir || 'd:\\Lunar_Astral_Agents\\local_data\\debug';
            const results = [];
            const dialoguePath = this.dialogueRole.dumpContext('对话者', `${dir}\\agent_debug_对话者.json`);
            if (dialoguePath)
                results.push(dialoguePath);
            const learnerPath = this.learnerRole.dumpContext(this.dialogueRole.messages, this.unreadContext, `${dir}\\agent_debug_学习者.json`);
            if (learnerPath)
                results.push(learnerPath);
            const painterPath = this.painterRole.dumpContext('画家', `${dir}\\agent_debug_画家.json`);
            if (painterPath)
                results.push(painterPath);
            const musicianPath = this.musicianRole.dumpContext('音乐家', `${dir}\\agent_debug_音乐家.json`);
            if (musicianPath)
                results.push(musicianPath);
            const viewerPath = this.viewerRole.dumpContext('观影者', `${dir}\\agent_debug_观影者.json`);
            if (viewerPath)
                results.push(viewerPath);
            const organizePath = this.organizeRole.dumpContext('编纂者', `${dir}\\agent_debug_编纂者.json`);
            if (organizePath)
                results.push(organizePath);
            const indexData = {
                timestamp: new Date().toLocaleString('zh-CN', {
                    year: 'numeric', month: '2-digit', day: '2-digit',
                    hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false
                }),
                unreadContextCount: this.unreadContext.length,
                unreadVideoUrlCount: this.unreadVideoUrl.length,
                unreadRecordsCount: OnlyData.unreadRecords.length,
                finalResponse: this.finalResponse,
                exportedFiles: results,
            };
            const indexPath = `${dir}\\agent_debug_index.json`;
            const [, indexError] = saveDebugFile(indexPath, JSON.stringify(indexData, null, 2));
            if (!indexError)
                results.push(indexPath);
            console.log(`[智能体] 已导出 ${results.length} 个上下文文件到 ${dir}`);
            return results;
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
                    this.learnerRole.executeLearner(this.unreadContext);
                    await this.createChatMessage();
                    if (!this.finalResponse.trim().length)
                        throw new Error('消息响应为空');
                    else
                        this.errorCount = 0;
                    if (OnlyData.unreadRecords.length > 30)
                        this.organizeRole.organizeHistoricalRecords();
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
                        const [audioData, err] = tts(chunk.tts);
                        if (!err && audioData)
                            audio = audioData;
                        pushContext(messageType, chunk.display, audio);
                    }
                    this.dumpAllContexts();
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
            this.summaryRole.coverContext([]);
            this.descriptionRole.coverContext([]);
            this.dialogueRole.coverContext([]);
            this.learnerRole.messages = [];
            this.painterRole.coverContext([]);
            this.musicianRole.coverContext([]);
            this.viewerRole.coverContext([]);
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
        constructor() {
            super();
            AgentDefine.instance = this;
            this.thinkingChainProcess();
        }
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
        console.log(`========== 开始处理截图工具调用 ==========`);
        console.log(`原始参数: ${JSON.stringify(args)}`);
        const parsed = typeof args === 'string' ? JSON.parse(args) : (args || {});
        const { display_index, region, scale, format } = parsed;
        console.log(`参数解析完成: display_index=${display_index}, region=${region}, scale=${scale}, format=${format}`);
        const displayIndex = display_index ?? 0;
        const captureFormat = format || 'png';
        console.log(`最终参数: display=${displayIndex}, region="${region || ''}", scale="${scale || ''}", format="${captureFormat}"`);
        console.log(`准备执行截图操作...`);
        const [result, captureErr] = screenshotCapture(displayIndex, region || '', scale || '', captureFormat, 0);
        if (captureErr) {
            console.error(`截图失败: ${captureErr.message || String(captureErr)}`);
            console.log(`========== 截图工具调用结束(失败) ==========`);
            return [`截图失败：${captureErr.message || String(captureErr)}`, ''];
        }
        console.log(`截图处理成功: ${result?.width}x${result?.height}, 格式=${result?.format}`);
        if (!result || !result.base64) {
            console.error(`截图失败: 未获取到截图数据`);
            console.log(`========== 截图工具调用结束(失败) ==========`);
            return ['截图失败：未获取到截图数据', ''];
        }
        pushImage([result.base64]);
        console.log(`图片已推送: ${result.width}x${result.height}, 格式=${result.format}, 数据长度=${result.base64.length} 字节`);
        const sizeInfo = `${result.width}x${result.height}`;
        const textResponse = `截图完成，已获取当前屏幕画面（${sizeInfo}），图片已展示给用户。`;
        console.log(`返回响应: ${sizeInfo}`);
        console.log(`========== 截图工具调用结束(成功) ==========`);
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
        },
        {
            type: "function",
            function: {
                name: "query_agent_position",
                description: "查询智能体当前在3D场景中的位置坐标。返回{x, y, z}坐标，可用于确定移动目标。",
                parameters: {
                    type: "object",
                    properties: {},
                    required: []
                }
            }
        },
        {
            type: "function",
            function: {
                name: "dispatch_painter",
                description: "向绘画师子智能体发布绘画创作任务。绘画师会完善需求并调用专业工具生成图像，完成后将作品直接推送至前端展示。",
                parameters: {
                    type: "object",
                    properties: {
                        description: {
                            type: "string",
                            description: "绘画需求描述，如'画一只在樱花树下的白猫'、'画一幅星空下的少女'。描述越详细，绘画效果越好。"
                        }
                    },
                    required: ["description"]
                }
            }
        },
        {
            type: "function",
            function: {
                name: "dispatch_musician",
                description: "向演奏家子智能体发布音乐创作任务。演奏家会完善需求并调用专业工具创作音乐，完成后将乐谱和音频直接推送至前端展示。",
                parameters: {
                    type: "object",
                    properties: {
                        description: {
                            type: "string",
                            description: "音乐需求描述，如'创作一首轻快的钢琴曲'、'写一首抒情的钢琴与大提琴二重奏'。描述越详细，音乐创作效果越好。"
                        }
                    },
                    required: ["description"]
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
    async function handleQueryAgentPosition(args) {
        const pos = getAgentPosition();
        const posStr = `当前智能体位置: x=${pos.x.toFixed(2)}, y=${pos.y.toFixed(2)}, z=${pos.z.toFixed(2)}`;
        console.log(`[智能体控制] ${posStr}`);
        return [posStr, ''];
    }
    async function handleDispatchPainter(args) {
        const { description } = parseArgs(args);
        if (!description || typeof description !== 'string' || description.trim().length === 0) {
            return ['绘画任务调度失败：创作描述不能为空，请提供具体的绘画需求', ''];
        }
        const instance = AgentDefine.instance;
        if (!instance || !instance.painterRole) {
            return ['绘画任务调度失败：绘画师子智能体未就绪，请稍后重试', ''];
        }
        console.log(`[智能体控制] 调度绘画师: ${description}`);
        const result = await instance.painterRole.createCreativeWork(description.trim());
        console.log(`[智能体控制] 绘画师完成: ${result}`);
        return [result, ''];
    }
    async function handleDispatchMusician(args) {
        const { description } = parseArgs(args);
        if (!description || typeof description !== 'string' || description.trim().length === 0) {
            return ['音乐任务调度失败：创作描述不能为空，请提供具体的音乐需求', ''];
        }
        const instance = AgentDefine.instance;
        if (!instance || !instance.musicianRole) {
            return ['音乐任务调度失败：演奏家子智能体未就绪，请稍后重试', ''];
        }
        console.log(`[智能体控制] 调度演奏家: ${description}`);
        const result = await instance.musicianRole.createCreativeWork(description.trim());
        console.log(`[智能体控制] 演奏家完成: ${result}`);
        return [result, ''];
    }
    OnlyData.LTPfunction.set('play_action', handlePlayAction);
    OnlyData.LTPfunction.set('agent_movement', handleAgentMovement);
    OnlyData.LTPfunction.set('query_agent_position', handleQueryAgentPosition);
    OnlyData.LTPfunction.set('dispatch_painter', handleDispatchPainter);
    OnlyData.LTPfunction.set('dispatch_musician', handleDispatchMusician);
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
    exports.LearnerRole = LearnerRole;
    exports.ModelBuilder = ModelBuilder;
    exports.MusicianRole = MusicianRole;
    exports.OnlyData = OnlyData;
    exports.OrganizeRole = OrganizeRole;
    exports.PainterRole = PainterRole;
    exports.RandomFloat = RandomFloat;
    exports.RandomFloor = RandomFloor;
    exports.ThinkType = ThinkType;
    exports.ViewerRole = ViewerRole;
    exports.agentControlTools = agentControlTools;
    exports.calculateFileHash = calculateFileHash;
    exports.checkDueItems = checkDueItems;
    exports.cleanTextForDisplay = cleanTextForDisplay;
    exports.cleanTextForTTS = cleanTextForTTS;
    exports.fetchDocumentCallback = fetchDocumentCallback;
    exports.getFileContent = getFileContent;
    exports.getPromptFromKnowledge = getPromptFromKnowledge;
    exports.initSchedules = initSchedules;
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
