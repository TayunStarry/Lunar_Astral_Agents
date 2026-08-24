var agentSystem = (function (exports) {
    'use strict';

    class GlobalConfig {
        static customConfig = { agent: {}, server: {} };
        static imageFormatsExtensions = ['.jpg', '.jpeg', '.png', '.gif', '.bmp', '.webp'];
        static videoFormatsExtensions = ['.mp4', '.avi', '.mov', '.wmv', '.flv', '.mkv'];
        static LTPfunction = new Map();
        static LTPdefinition = [];
        static unreadRecords = [];
        static unreadContext = [];
        static unreadVideoUrl = [];
        static speakWeight = 1;
        static silenceCount = 0;
        static reasoningInProgress = false;
        static finalResponse = "";
        static memoryReady = false;
        static currentAddress = [];
        static get MultimodalUrl() {
            return GlobalConfig.customConfig?.agent?.multimodal_url || url()[0] + '/v1';
        }
        ;
        static get MultimodalKey() {
            return GlobalConfig.customConfig?.agent?.multimodal_key || 'key-520-1314-2000-02-18';
        }
        ;
        static get MultimodalName() {
            return GlobalConfig.customConfig?.agent?.multimodal_model || "system-multimodal";
        }
        ;
        static get EmbeddingName() {
            return GlobalConfig.customConfig?.agent?.embedding_model || "system-embedding";
        }
        ;
        static get userName() {
            return GlobalConfig.customConfig?.server?.user_name || "阁下";
        }
        ;
        static get debugMode() {
            return GlobalConfig.customConfig?.server?.developer ?? false;
        }
        ;
    }

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

    class ModelBuilder {
        stream = false;
        enableTools = true;
        messages = [];
        ragMessages = [];
        runtimeMessages = [];
        systemPrompt = "你的名字叫做月华, 是一个女孩子";
        promptCompletion(prompt) {
            let addressText = "";
            if (GlobalConfig.currentAddress.length === 0) {
                const addressResult = address();
                GlobalConfig.currentAddress = addressResult[0];
                addressText = GlobalConfig.currentAddress.join(' ');
            }
            else
                addressText = GlobalConfig.currentAddress.join(' ');
            return prompt
                .replace(/{name}/g, GlobalConfig.userName)
                .replace(/{current-address}/g, addressText);
        }
        writeContext(context) {
            const cleaned = this.stripReasoningContent(context);
            if (this.messages.length >= 48) {
                const discarded = this.messages.slice(0, this.messages.length - 47);
                this.messages = this.messages.slice(-47).concat(cleaned);
                GlobalConfig.unreadRecords.push(...discarded);
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
        run(appendContext, toolCall) {
            const rawMessages = [
                { role: 'system', content: this.systemPrompt },
                ...this.runtimeMessages,
                ...appendContext,
                ...this.messages
            ];
            const requestBody = {
                model: GlobalConfig.MultimodalName,
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
                Authorization: `Bearer ${encodeURIComponent(GlobalConfig.MultimodalKey)}`,
                "Content-Type": "application/json",
            };
            const modelRequest = {
                method: "POST",
                crossDomain: true,
                headers,
                body: JSON.stringify(requestBody)
            };
            const endpoint = "/chat/completions";
            const [result, error] = syncFetch({ url: GlobalConfig.MultimodalUrl + endpoint, execute: modelRequest });
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
        constructor(prompt) {
            this.systemPrompt = this.promptCompletion(prompt);
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
        async generateDialogue(cache) {
            try {
                await LiteImageFile();
                GlobalConfig.unreadContext.forEach(context => this.writeContext(context));
                GlobalConfig.unreadContext = [];
                this.formatHistoricalMessages();
                this.runtimeMessages = [{ role: 'user', content: `当前时间: ${new Date().toLocaleString()}` }];
                this.queryRagMessages();
                const response = this.run(this.ragMessages, GlobalConfig.LTPdefinition);
                this.analyzeMessageResponse(response.body, cache);
                if (cache.toolCalls.length > 0) {
                    this.writeContext(response.body.choices?.[0]?.message);
                    const hasProcessedToolCalls = await this.batchExecutionToolCall(cache);
                    if (hasProcessedToolCalls)
                        return await this.generateDialogue(cache);
                }
                this.writeContext(response.body.choices?.[0]?.message);
            }
            catch (error) {
                console.error('请求处理错误:', error);
            }
            this.updateMessageContent(cache);
        }
        formatHistoricalMessages() {
            if (this.messages.length === 0)
                return;
            const totalImages = this.countTotalImages(this.messages);
            if (totalImages >= 20) {
                const processedMessages = [];
                for (const message of this.messages) {
                    if (typeof message.content === 'string') {
                        processedMessages.push(message);
                        continue;
                    }
                    const textResult = this.summarizeMessageImages(message);
                    if (!textResult || textResult.trim() === '')
                        continue;
                    processedMessages.push({ role: message.role, content: textResult });
                }
                this.messages = processedMessages;
            }
            if (this.messages.length === 0)
                return;
            if (this.messages.slice(-1)[0].role === 'user')
                return;
            this.writeContext({ role: 'user', content: "继续" });
        }
        summarizeMessageImages(message) {
            if (typeof message.content === 'string')
                return message.content;
            const imageItems = message.content.filter((c) => c.type === 'image_url');
            const textItems = message.content.filter((c) => c.type === 'text');
            const textPart = textItems.map((c) => c.text).join('\n');
            if (imageItems.length === 0)
                return textPart || null;
            try {
                descriptionRole.coverContext({ role: 'user', content: imageItems });
                const summaryRequest = descriptionRole.run([], []);
                const summary = summaryRequest.body?.choices?.[0]?.message?.content;
                if (summary && summary.trim().length > 0) {
                    return textPart ? `${textPart}\n[图片描述：${summary}]` : `[图片描述：${summary}]`;
                }
                return textPart || null;
            }
            catch (error) {
                console.error('[对话者] 图片摘要异常:', error);
                return textPart || null;
            }
        }
        countImagesInMessage(message) {
            if (typeof message.content === 'string')
                return 0;
            return message.content.filter((c) => c.type === 'image_url').length;
        }
        countTotalImages(messages) {
            return messages.reduce((sum, m) => sum + this.countImagesInMessage(m), 0);
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
                const lunarToolPackage = GlobalConfig.LTPfunction.get(functionName);
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
        updateMessageContent(state) {
            if (state.thinkingContent.trim() !== "") {
                const newThinkTag = '<think>\n' + state.thinkingContent + '\n</think>\n';
                GlobalConfig.finalResponse = state.descriptionContent;
                console.log(newThinkTag);
            }
            else
                GlobalConfig.finalResponse = state.descriptionContent;
            return GlobalConfig.finalResponse;
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
        queryRagMessages() {
            const userMessages = this.getLatestUserMessages();
            const returnEvent = () => { this.ragMessages = []; return this; };
            if (userMessages.length === 0)
                returnEvent();
            if (!ensureMemoryReady())
                returnEvent();
            const allResults = [];
            for (const userMessage of userMessages) {
                const [results, error] = memoryQuery('lunar_messages', userMessage, 10);
                if (error) {
                    console.error('记忆库查询失败:', error);
                    continue;
                }
                if (results && results.length > 0) {
                    allResults.push(...results);
                }
            }
            if (allResults.length === 0)
                returnEvent();
            const seen = new Map();
            for (const r of allResults) {
                const content = r.content || '';
                const existing = seen.get(content);
                if (!existing || r.similarity > existing.similarity)
                    seen.set(content, r);
            }
            const uniqueResults = Array.from(seen.values()).sort((a, b) => b.similarity - a.similarity);
            console.log(`[RAG] 查询到 ${uniqueResults.length} 条相关消息，相似度范围: ${uniqueResults[0]?.similarity?.toFixed(4) ?? 'N/A'} ~ ${uniqueResults[uniqueResults.length - 1]?.similarity?.toFixed(4) ?? 'N/A'}`);
            this.ragMessages = uniqueResults.slice(0, 32).map(r => ({ role: r.role, content: r.content || '' }));
            return this;
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
        get roleName() { return '绘制者'; }
        getToolDefinitions() { return this.roleTool; }
        executeTool(toolCall) {
            const funcName = toolCall.function.name;
            let args = {};
            try {
                args = typeof toolCall.function.arguments === 'string' ? JSON.parse(toolCall.function.arguments) : toolCall.function.arguments;
            }
            catch (parseError) {
                console.error(`[绘制者] 工具调用参数解析失败:`, toolCall.function.arguments);
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
                console.log(`[绘制者] 扩散生成 - 正向提示词: ${prompt.slice(0, 100)}...`);
                const imageParams = {
                    prompt: prompt,
                    negativePrompt: args.negative_prompt || '',
                    cfgScale: args.cfg_scale ?? 1.0,
                };
                const [result, error] = generateImage(imageParams);
                if (error) {
                    console.error('[绘制者] 图像生成失败:', error);
                    return `扩散图像生成失败: ${error}`;
                }
                if (!result || !result.base64) {
                    return '扩散图像生成失败：引擎返回空结果';
                }
                console.log(`[绘制者] 扩散图像生成成功，尺寸: ${result.width}x${result.height}`);
                const pushSuccess = pushImage([result.base64]);
                if (!pushSuccess) {
                    console.warn('[绘制者] 推送图片到前端失败');
                }
                return `扩散图像生成成功。图片尺寸: ${result.width}x${result.height}，seed: ${result.seed}`;
            }
            catch (error) {
                console.error('[绘制者] 扩散生成处理异常:', error);
                return `扩散图像生成异常: ${error}`;
            }
        }
        handleSelfPortrait(args) {
            try {
                console.log(`[绘制者] -> 自画像生成`);
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
                    console.error('[绘制者] 自画像生成失败:', error);
                    return `自画像生成失败: ${error}`;
                }
                if (!result || !result.base64) {
                    return '自画像生成失败：引擎返回空结果';
                }
                console.log(`[绘制者] 自画像生成成功，尺寸: ${result.width}x${result.height}`);
                const pushSuccess = pushImage([result.base64]);
                if (!pushSuccess) {
                    console.warn('[绘制者] 推送自画像到前端失败');
                }
                return `自画像生成成功。图片尺寸: ${result.width}x${result.height}，seed: ${result.seed}`;
            }
            catch (error) {
                console.error('[绘制者] 自画像生成处理异常:', error);
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
                    description: "创作音乐作品并生成ABC记谱法格式的乐谱，前端音乐播放器将使用Tone.js合成真实乐器音色播放。必须生成完整、可直接播放的ABC乐谱，包含和弦伴奏与多声部编配。",
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
                                description: `ABC记谱法格式的完整乐谱。前端音乐播放器使用Tone.js合成真实乐器音色。

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

=== 完整示例：钢琴独奏（含和弦伴奏） ===
X:1
T:晨光曲
M:4/4
L:1/8
Q:1/4=90
K:C
!mp! [V:1] c2 e2 g2 e2 | f2 a2 g2 e2 | d2 f2 e2 d2 | c4 z4 |
!mf! [V:2] [C,,E,,G,,]4 | [F,,A,,C,]4 | [G,,B,,D,]4 | [C,,E,,G,,]4 |

=== 完整示例：钢琴+大提琴二重奏 ===
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
        get roleName() { return '演奏者'; }
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
                console.error(`[演奏者] 工具调用参数解析失败:`, toolCall.function.arguments);
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
                console.log(`[演奏者] 创作音乐: "${title}"`);
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
                    console.warn('[演奏者] ABC乐谱缺少必要字段 (X:/K:)，尝试自动补充');
                    if (!hasX)
                        enrichedAbc = 'X:1\n' + enrichedAbc;
                    if (!hasT)
                        enrichedAbc = enrichedAbc.replace(/^(X:\s*\d+\n)/m, `$1T:${title}\n`);
                    if (!hasK)
                        enrichedAbc = enrichedAbc.replace(/^(T:.*\n)/m, `$1K:C\n`);
                }
                const pushSuccess = pushContext('music', enrichedAbc, '');
                if (!pushSuccess) {
                    console.warn('[演奏者] 推送乐谱到前端失败');
                }
                console.log(`[演奏者] 乐谱推送成功，长度: ${enrichedAbc.length} 字符，乐器: ${instruments || '默认'}`);
                return `音乐作品"${title}"创作成功。乐谱已推送到前端展示，可通过音乐播放器查看和播放。`;
            }
            catch (error) {
                console.error('[演奏者] 音乐创作处理异常:', error);
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
            if (/^%%voice\s+/m.test(abcNotation))
                return abcNotation;
            const directives = [];
            for (let i = 0; i < list.length; i++) {
                const inst = list[i];
                const voiceNum = i + 1;
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

    let learnerInitialized = false;
    function ensureLearnerInitialized() {
        if (learnerInitialized)
            return true;
        if (!learnerIsReady()) {
            const [success, err] = learnerInit();
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
        async createCreativeWork(taskDescription) {
            if (!taskDescription || taskDescription.trim().length === 0) {
                return '研究任务调度失败：任务描述不能为空，请提供具体的学习研究需求';
            }
            if (!ensureLearnerInitialized()) {
                return '研究任务调度失败：学习者子智能体未就绪，请稍后重试';
            }
            console.log('[学习者] 开始执行研究:', taskDescription);
            const [report, error] = learnerExecute(taskDescription.trim());
            if (error) {
                console.error('[学习者] 执行失败:', error);
                return `研究任务执行失败：${error}`;
            }
            if (report && report.trim().length > 0) {
                this.messages.push({ role: 'assistant', content: report });
                console.log('[学习者] 研究完成，报告已生成');
                return report;
            }
            return '研究任务完成，但未找到相关信息。';
        }
        dumpContext(dialogueMessages, unreadContext, outputPath) {
            const path = outputPath || 'agent_debug_学习者.json';
            const timestamp = new Date().toLocaleString('zh-CN', {
                year: 'numeric', month: '2-digit', day: '2-digit',
                hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false
            });
            const snapshot = {
                timestamp,
                role: '学习者',
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
            const goPath = path.replace('.json', '_go.json');
            const [, goError] = learnerDumpContext('', goPath);
            if (goError) {
                console.error('[学习者] 导出 Go 层上下文失败:', goError);
            }
            console.log('[学习者] 上下文快照已导出:', path);
            return path;
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

    const FALLBACK_ACTIONS = ['荡秋千', '翻花绳'];
    class ActorRole extends CreativeRoleBase {
        MAX_ITERATIONS = 5;
        constructor() {
            super(fileView('prompts/actorRole.md')[0]);
        }
        get roleName() { return '行动者'; }
        getAvailableActionNames() {
            try {
                const raw = getAvailableActions();
                if (!raw || raw === '{}')
                    return FALLBACK_ACTIONS;
                const parsed = JSON.parse(raw);
                if (parsed.actions && Array.isArray(parsed.actions) && parsed.actions.length > 0) {
                    return parsed.actions.map(a => a.name);
                }
            }
            catch {
            }
            return FALLBACK_ACTIONS;
        }
        getToolDefinitions() {
            const actionNames = this.getAvailableActionNames();
            return [
                {
                    type: "function",
                    function: {
                        name: "play_action",
                        description: "让月华执行预设动作。可用动作：" + actionNames.join('、') + "。",
                        parameters: {
                            type: "object",
                            properties: {
                                action_name: {
                                    type: "string",
                                    description: "动作名称",
                                    enum: actionNames
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
                        description: "控制月华移动到指定3D坐标位置。移动期间会自动关闭鼠标追踪。",
                        parameters: {
                            type: "object",
                            properties: {
                                x: { type: "number", description: "目标X坐标" },
                                y: { type: "number", description: "目标Y坐标（地面为0）" },
                                z: { type: "number", description: "目标Z坐标" },
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
                        description: "查询月华当前在3D场景中的位置坐标。返回{x, y, z}格式坐标。",
                        parameters: {
                            type: "object",
                            properties: {},
                            required: []
                        }
                    }
                }
            ];
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
                console.error(`[行动者] 工具调用参数解析失败:`, toolCall.function.arguments);
                return `工具调用参数解析失败: ${parseError}`;
            }
            switch (funcName) {
                case 'play_action': return this.handlePlayAction(args);
                case 'agent_movement': return this.handleAgentMovement(args);
                case 'query_agent_position': return this.handleQueryAgentPosition();
                default: return `未知工具: ${funcName}`;
            }
        }
        collectDetail(toolCall, details) {
            try {
                const args = typeof toolCall.function.arguments === 'string'
                    ? JSON.parse(toolCall.function.arguments)
                    : toolCall.function.arguments;
                const detail = { toolName: toolCall.function.name };
                if (toolCall.function.name === 'play_action') {
                    detail.actionName = args.action_name || '';
                }
                else if (toolCall.function.name === 'agent_movement') {
                    detail.targetPos = `(${args.x}, ${args.y}, ${args.z})`;
                }
                details.push(detail);
            }
            catch {
            }
        }
        buildSummary(details) {
            if (details.length === 0)
                return '月华没有执行任何行动';
            const parts = [];
            for (const d of details) {
                if (d.toolName === 'query_agent_position')
                    continue;
                if (d.toolName === 'play_action' && d.actionName) {
                    parts.push(`月华${d.actionName}了`);
                }
                else if (d.toolName === 'agent_movement' && d.targetPos) {
                    parts.push(`月华移动到了${d.targetPos}`);
                }
            }
            if (parts.length === 0)
                return '月华完成了行动任务';
            const summary = parts.join('，') + '。';
            pushContext('action', summary, '');
            return summary;
        }
        handlePlayAction(args) {
            const actionName = args.action_name || '';
            if (!actionName)
                return '执行动作失败：动作名称不能为空';
            const allowed = this.getAvailableActionNames();
            if (!allowed.includes(actionName)) {
                return `执行动作失败：不支持的动作 "${actionName}"，可用动作为：${allowed.join('、')}`;
            }
            sendToEngine('action', JSON.stringify({ action: actionName }));
            console.log(`[行动者] 执行动作: ${actionName}`);
            return `已执行动作：${actionName}`;
        }
        handleAgentMovement(args) {
            const x = Number(args.x);
            const y = Number(args.y);
            const z = Number(args.z);
            const resumeTracking = args.resume_tracking !== false;
            if (isNaN(x) || isNaN(y) || isNaN(z)) {
                return '移动失败：坐标参数 x、y、z 必须为有效数字';
            }
            sendToEngine('movement', JSON.stringify({
                position: { x, y, z },
                resumeTracking
            }));
            console.log(`[行动者] 移动到 (${x}, ${y}, ${z})，恢复追踪: ${resumeTracking}`);
            return `已移动到 (${x}, ${y}, ${z})`;
        }
        handleQueryAgentPosition() {
            const pos = getAgentPosition();
            const result = `当前位置: x=${pos.x.toFixed(2)}, y=${pos.y.toFixed(2)}, z=${pos.z.toFixed(2)}`;
            console.log(`[行动者] ${result}`);
            return result;
        }
    }

    const descriptionRole = new ModelBuilder(fileView('prompts/descriptionRole.md')[0]);
    const learnerRole = new LearnerRole();
    const painterRole = new PainterRole();
    const musicianRole = new MusicianRole();
    const actorRole = new ActorRole();
    const dialogueRole = new DialogueRole();
    const viewerRole = new ViewerRole();
    function randomDefaultMessage() {
        return ['月华在哦', '怎么了吗?', '详细说说?'][RandomFloor(0, 2)];
    }
    async function analysisVideoFile(videoUrl, userNeeds) {
        const cachedPrompt = getPromptFromKnowledge(videoUrl);
        if (cachedPrompt) {
            GlobalConfig.unreadContext.push({ role: 'user', content: cachedPrompt });
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
        const keyframes = images.map(frame => ({ data: frame.data, timestamp: frame.timestamp || '' }));
        console.log('[观影者] 开始观看视频...');
        const videoSummary = await viewerRole.watchVideo(keyframes);
        console.log('[观影者] 视频观看完成');
        if (videoSummary && videoSummary.trim().length > 0) {
            GlobalConfig.unreadContext.push({ role: 'user', content: videoSummary });
        }
        else
            GlobalConfig.unreadContext.push({ role: 'user', content: randomDefaultMessage() });
        if (userNeeds.trim().length > 0) {
            GlobalConfig.unreadContext.push({ role: 'user', content: userNeeds });
        }
        if (videoSummary) {
            savePromptToKnowledge(videoUrl, videoSummary);
            console.log('[观影者] 观后感已缓存');
        }
    }
    async function summarizeDynamicImages(frames) {
        if (frames.length === 0)
            return '';
        const summaries = [];
        const BATCH_SIZE = 8;
        for (let i = 0; i < frames.length; i += BATCH_SIZE) {
            const batch = frames.slice(i, i + BATCH_SIZE);
            try {
                descriptionRole.coverContext({
                    role: 'user',
                    content: batch.map(frame => ({ type: 'image_url', image_url: { url: frame } }))
                });
                const summaryRequest = descriptionRole.run([], []);
                const summary = summaryRequest.body?.choices?.[0]?.message?.content;
                if (summary && summary.trim().length > 0)
                    summaries.push(summary.trim());
            }
            catch (error) {
                console.error('[动态图摘要] 批次摘要失败:', error);
            }
        }
        return summaries.join('\n');
    }
    async function LiteImageFile() {
        for (let message of GlobalConfig.unreadContext) {
            if (typeof message.content === 'string')
                continue;
            const newContent = [];
            for (let item of message.content) {
                if (item.type == 'text')
                    newContent.push(item);
                else if (item.image_url && GlobalConfig.videoFormatsExtensions.some(format => item.image_url.url.toLowerCase().endsWith(format))) {
                    await analysisVideoFile(item.image_url.url, '');
                }
                else if (item.image_url && !item.image_url.url.startsWith("data:image")) {
                    const [response, error] = syncFetch({ url: item.image_url.url, execute: { crossDomain: true } });
                    if (error) {
                        console.error('[获取图片文件失败]:', error.message, error.stack);
                        continue;
                    }
                    const [resizedImages, error1] = resizeImage(response.body);
                    if (error1) {
                        console.error('[缩放图片失败]:', error1.message, error1.stack);
                        continue;
                    }
                    if (resizedImages.length > 1) {
                        newContent.push({ type: 'text', text: await summarizeDynamicImages(resizedImages.map(image => image.base64)) || '' });
                        continue;
                    }
                    resizedImages.forEach(image => newContent.push({ type: 'image_url', image_url: { url: image.base64 } }));
                }
            }
            message.content = newContent;
        }
    }
    async function batchProcessVideoFiles(userNeeds) {
        if (GlobalConfig.unreadVideoUrl.length === 0)
            return;
        for (const videoUrl of GlobalConfig.unreadVideoUrl) {
            try {
                await analysisVideoFile(videoUrl, userNeeds || '');
                await new Promise(resolve => setTimeout(resolve, 1000));
            }
            catch (error) {
                continue;
            }
        }
        GlobalConfig.unreadVideoUrl = [];
    }
    async function createChatMessage() {
        const cache = { currentToolCallIndex: -1, currentFunctionArgs: '', currentFunctionName: '', descriptionContent: '', thinkingContent: '', currentToolCall: null, toolCalls: [], };
        await dialogueRole.generateDialogue(cache);
        GlobalConfig.speakWeight--;
        return GlobalConfig.finalResponse;
    }
    async function thoughtLoopTickEvent() {
        if (GlobalConfig.reasoningInProgress)
            return;
        try {
            GlobalConfig.reasoningInProgress = true;
            syncLTPXToolStatus();
            await pullExternalMessages();
            for (const item of checkDueItems()) {
                GlobalConfig.unreadContext.push({ role: 'user', content: `[计划提醒] 预约时间已到，请执行以下计划：${item.content}` });
            }
            const messageLength = GlobalConfig.unreadContext.length + GlobalConfig.unreadVideoUrl.length;
            const messageType = messageLength === 0 ? 'response' : 'active';
            const allowSpeak = RandomFloor(5, 100) < GlobalConfig.speakWeight;
            if (messageLength === 0 && !allowSpeak) {
                GlobalConfig.silenceCount = Math.min(GlobalConfig.silenceCount + 1, 100);
                GlobalConfig.reasoningInProgress = false;
                return;
            }
            if (messageLength === 0 && allowSpeak && GlobalConfig.silenceCount < 30) {
                GlobalConfig.silenceCount = Math.min(GlobalConfig.silenceCount + 1, 100);
                GlobalConfig.reasoningInProgress = false;
                return;
            }
            GlobalConfig.silenceCount = 0;
            if (messageLength === 0) {
                pullPoolContext().forEach(message => writeMessage(message.role, message.content));
                GlobalConfig.speakWeight = 0;
            }
            await batchProcessVideoFiles();
            await createChatMessage();
            if (!GlobalConfig.finalResponse.trim().length) {
                pushContext(messageType, randomDefaultMessage(), tts(randomDefaultMessage())[0]);
                return;
            }
            ;
            const { thinkingBlocks, codeBlocks, actionBlocks, textChunks } = parseContent(GlobalConfig.finalResponse);
            const validMessage = textChunks.map(chunk => chunk.display).join('').trim();
            if (!validMessage.length)
                throw new Error('清洗后的文本为空');
            if (actionBlocks.length) {
                await actorRole.createCreativeWork(actionBlocks.join('|'));
                pushImage([await queryEmotionSticker(actionBlocks.join('|'))], true);
            }
            else if (validMessage.length <= 35 && Math.random() < 0.55) {
                pushImage([await queryEmotionSticker(validMessage)], true);
            }
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
            if (GlobalConfig.unreadRecords.length >= 1)
                memorizeUnreadRecords();
        }
        catch (error) {
            const [promptSound, , , readErr] = readFile('audios/cartoon-fail.mp3');
            if (readErr)
                console.error('读取提示音失败:', readErr);
            console.error(error.message, ' || ', error.stack);
            pushContext('active', randomDefaultMessage(), promptSound);
            resetAgentState();
        }
        GlobalConfig.reasoningInProgress = false;
    }
    async function pullExternalMessages() {
        pullContext().forEach(message => writeMessage(message.role, message.content));
        pullVideoUrl().forEach(videoUrl => { writeVideoUrl(videoUrl); });
        await new Promise(resolve => setTimeout(resolve, 1000));
    }
    function writeMessage(role, messages) {
        GlobalConfig.unreadContext.push({ role, content: messages });
        GlobalConfig.speakWeight += RandomFloor(1, 3);
        if (typeof messages === 'string')
            messages = [{ type: 'text', text: messages }];
        messages.forEach(message => { if (message.type === 'text')
            console.log(message.text); });
    }
    function writeVideoUrl(videoUrl) {
        console.log('写入视频文件:' + videoUrl);
        GlobalConfig.unreadVideoUrl.push(videoUrl);
        GlobalConfig.speakWeight += RandomFloor(1, 3);
    }
    function resetAgentState() {
        descriptionRole.coverContext([]);
        dialogueRole.coverContext([]);
        learnerRole.messages = [];
        painterRole.coverContext([]);
        musicianRole.coverContext([]);
        viewerRole.coverContext([]);
        actorRole.coverContext([]);
        GlobalConfig.unreadContext = [];
        GlobalConfig.unreadVideoUrl = [];
    }
    function syncLTPXToolStatus() {
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
    const STICKER_COLLECTION = 'stickers';
    let stickerCollectionReady = false;
    async function queryEmotionSticker(query) {
        if (!query || !query.trim())
            return null;
        try {
            if (!stickerCollectionReady) {
                const [ready] = memoryInit(STICKER_COLLECTION, 'image');
                if (!ready)
                    return null;
                stickerCollectionReady = true;
            }
            const [results, error] = memoryQuery(STICKER_COLLECTION, query.trim(), 1);
            if (error || !results || results.length === 0)
                return null;
            const image = results[0].image;
            return image || null;
        }
        catch (error) {
            console.error('[表情包] 检索失败:', error);
            return null;
        }
    }
    function extractTextFromMessage(message) {
        if (typeof message.content === 'string')
            return message.content;
        if (Array.isArray(message.content)) {
            return message.content
                .filter(item => item.type === 'text')
                .map(item => item.text)
                .join(' ');
        }
        return '';
    }
    function initMemory() {
        if (GlobalConfig.memoryReady)
            return;
        const [_, err] = memoryInit('lunar_messages', 'text');
        if (err)
            console.error('记忆库初始化失败:', err);
        else
            GlobalConfig.memoryReady = true;
    }
    function ensureMemoryReady() {
        if (!GlobalConfig.memoryReady)
            initMemory();
        return GlobalConfig.memoryReady;
    }
    function memorizeUnreadRecords() {
        if (GlobalConfig.unreadRecords.length === 0)
            return;
        if (!ensureMemoryReady()) {
            console.warn('[记忆] 记忆库未就绪，保留缓冲消息待下次触发');
            return;
        }
        let written = 0;
        for (const message of GlobalConfig.unreadRecords) {
            const content = extractTextFromMessage(message).trim();
            if (!content)
                continue;
            const [, error] = memoryAdd('lunar_messages', message.role, content);
            if (error)
                console.error('[记忆] 写入记忆库失败:', error);
            else
                written++;
        }
        console.log(`[记忆] 已写入 ${written} 条消息到记忆库`);
        GlobalConfig.unreadRecords = [];
    }
    fetchDocumentCallback('lunar_config.json').then(content => GlobalConfig.customConfig = content);
    setInterval(() => thoughtLoopTickEvent(), 1000);

    const PRESET_DAILY_TASKS = [
        { id: 'daily_greeting_0800', type: 'daily', time: '08:00', content: '向用户发送早上好问候' },
        { id: 'daily_greeting_1000', type: 'daily', time: '10:00', content: '向用户发送"上午好，该喝水了"的问候' },
        { id: 'daily_greeting_1200', type: 'daily', time: '12:00', content: '向用户发送午安问候' },
        { id: 'daily_greeting_1500', type: 'daily', time: '15:00', content: '向用户发送"下午好，该喝水了"的问候' },
        { id: 'daily_greeting_1730', type: 'daily', time: '17:30', content: '向用户发送下午好问候' },
        { id: 'daily_greeting_2230', type: 'daily', time: '22:30', content: '向用户发送晚安问候' },
    ];
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
    function formatDate(date) {
        const y = date.getFullYear();
        const mo = String(date.getMonth() + 1).padStart(2, '0');
        const d = String(date.getDate()).padStart(2, '0');
        return `${y}-${mo}-${d}`;
    }
    function dailyTaskTime(item, now) {
        const match = item.time.trim().match(/^(\d{1,2}):(\d{2})$/);
        if (!match)
            return null;
        return new Date(now.getFullYear(), now.getMonth(), now.getDate(), Number(match[1]), Number(match[2]));
    }
    function initSchedules() {
        const raw = loadSchedulesFromDisk();
        const existingIds = new Set(raw.map(item => item.id));
        let added = 0;
        for (const preset of PRESET_DAILY_TASKS) {
            if (!existingIds.has(preset.id)) {
                raw.push({ ...preset });
                added++;
            }
        }
        if (raw.length === 0) {
            saveSchedulesToDisk([]);
            scheduleCache = [];
            console.log('[计划表] 初始化完成，计划表为空');
            return;
        }
        let needsRewrite = added > 0;
        for (const item of raw) {
            if (item.type === 'daily')
                continue;
            const normalized = normalizeTime(item.time);
            if (normalized && normalized !== item.time) {
                item.time = normalized;
                needsRewrite = true;
            }
        }
        scheduleCache = raw;
        if (needsRewrite) {
            saveSchedulesToDisk(scheduleCache);
            if (added > 0)
                console.log(`[计划表] 已补充 ${added} 个预设每日任务`);
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
        const todayStr = formatDate(now);
        const dueOnce = [];
        const dueDaily = [];
        const remaining = [];
        for (const item of scheduleCache) {
            if (item.type === 'daily') {
                const todayTime = dailyTaskTime(item, now);
                if (todayTime && now >= todayTime && item.completedDate !== todayStr) {
                    dueDaily.push(item);
                }
                remaining.push(item);
                continue;
            }
            const itemTime = new Date(item.time);
            if (isNaN(itemTime.getTime())) {
                console.warn(`[计划表] 无效的时间格式，跳过: [${item.id}] ${item.time}`);
                remaining.push(item);
                continue;
            }
            if (now >= itemTime) {
                dueOnce.push(item);
                console.log(`[计划表] 触发到期计划项: [${item.id}] ${item.time} - ${item.content}`);
            }
            else {
                remaining.push(item);
            }
        }
        const executed = [...dueOnce];
        if (dueDaily.length > 0) {
            dueDaily.sort((a, b) => {
                const ta = dailyTaskTime(a, now).getTime();
                const tb = dailyTaskTime(b, now).getTime();
                return Math.abs(now.getTime() - ta) - Math.abs(now.getTime() - tb);
            });
            const chosen = dueDaily.shift();
            console.log(`[计划表] 触发每日任务: [${chosen.id}] ${chosen.time} - ${chosen.content}`);
            executed.push(chosen);
            for (const d of dueDaily) {
                console.log(`[计划表] 每日任务冲突，标记今日已执行(不执行): [${d.id}] ${d.time} - ${d.content}`);
            }
            for (const d of [chosen, ...dueDaily]) {
                d.completedDate = todayStr;
            }
        }
        if (executed.length > 0) {
            scheduleCache = remaining;
            saveSchedulesToDisk(scheduleCache);
        }
        return executed;
    }
    initSchedules();
    GlobalConfig.LTPfunction.set('create_schedule', handleCreateSchedule);
    GlobalConfig.LTPfunction.set('edit_schedule', handleEditSchedule);
    GlobalConfig.LTPfunction.set('delete_schedule', handleDeleteSchedule);
    GlobalConfig.LTPfunction.set('query_schedule', handleQuerySchedule);
    GlobalConfig.LTPdefinition.push(...scheduleTools);

    const screenshotTools = [
        {
            type: "function",
            function: {
                name: "screenshot",
                description: "截取当前屏幕画面。默认优先截取当前焦点应用窗口，无法识别焦点窗口时自动降级为全屏截图。支持指定显示器、全屏、绝对坐标区域，以及窗口内精准子区域（偏移量+区域大小）。截取的图片会自动缩放处理并展示给用户。",
                parameters: {
                    type: "object",
                    properties: {
                        mode: {
                            type: "string",
                            enum: ["auto", "window", "fullscreen", "display", "region"],
                            description: "截图模式：auto=焦点窗口优先（默认，失败降级全屏）；window=强制焦点窗口；fullscreen=全屏；display=指定显示器；region=绝对坐标区域"
                        },
                        display_index: {
                            type: "number",
                            description: "显示器索引（mode=display 时生效，-1 表示全部）"
                        },
                        offset_x: {
                            type: "number",
                            description: "窗口相对 X 偏移（mode=auto/window，配合 width/height 使用，缺省为 0）"
                        },
                        offset_y: {
                            type: "number",
                            description: "窗口相对 Y 偏移（mode=auto/window，配合 width/height 使用，缺省为 0）"
                        },
                        width: {
                            type: "number",
                            description: "窗口相对区域宽度（>0 且 height>0 时启用精准区域覆盖）"
                        },
                        height: {
                            type: "number",
                            description: "窗口相对区域高度"
                        },
                        region_x: {
                            type: "number",
                            description: "绝对屏幕区域 X 坐标（mode=region）"
                        },
                        region_y: {
                            type: "number",
                            description: "绝对屏幕区域 Y 坐标（mode=region）"
                        },
                        region_w: {
                            type: "number",
                            description: "绝对屏幕区域宽度（mode=region）"
                        },
                        region_h: {
                            type: "number",
                            description: "绝对屏幕区域高度（mode=region）"
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
        console.log(`参数解析完成: ${JSON.stringify(parsed)}`);
        console.log(`准备执行截图操作...`);
        const [results, captureErr] = screenshotCapture(parsed);
        if (captureErr) {
            console.error(`截图失败: ${captureErr.message || String(captureErr)}`);
            console.log(`========== 截图工具调用结束(失败) ==========`);
            return [`截图失败：${captureErr.message || String(captureErr)}`, ''];
        }
        if (!results || results.length === 0) {
            console.error(`截图失败: 未获取到截图数据`);
            console.log(`========== 截图工具调用结束(失败) ==========`);
            return ['截图失败：未获取到截图数据', ''];
        }
        const firstFrame = results[0];
        console.log(`截图处理成功: ${firstFrame.width}x${firstFrame.height}, 格式=${firstFrame.format}, 帧数=${results.length}`);
        const base64List = results.map((r) => r.base64);
        pushImage(base64List);
        console.log(`图片已推送: ${firstFrame.width}x${firstFrame.height}, 格式=${firstFrame.format}, 帧数=${results.length}`);
        const sizeInfo = `${firstFrame.width}x${firstFrame.height}`;
        const frameInfo = results.length > 1 ? `（共${results.length}帧）` : '';
        const textResponse = `截图完成，已获取当前屏幕画面（${sizeInfo}）${frameInfo}，图片已展示给用户。`;
        console.log(`返回响应: ${sizeInfo}`);
        console.log(`========== 截图工具调用结束(成功) ==========`);
        return [textResponse, firstFrame.base64];
    }
    GlobalConfig.LTPfunction.set('screenshot', handleScreenshot);
    GlobalConfig.LTPdefinition.push(...screenshotTools);

    const agentControlTools = [
        {
            type: "function",
            function: {
                name: "dispatch_actor",
                description: "向行动者子智能体发布行动任务。行动者负责控制月华在3D场景中的动画、位移和空间感知。只需用一句话描述你想让月华做什么，行动者会自行规划并执行具体操作。",
                parameters: {
                    type: "object",
                    properties: {
                        description: {
                            type: "string",
                            description: "行动需求描述，如'让月华去荡秋千'、'移动到坐标(1, 2, 3)'、'开始翻花绳'。描述越清晰，行动者执行越准确。"
                        }
                    },
                    required: ["description"]
                }
            }
        },
        {
            type: "function",
            function: {
                name: "dispatch_painter",
                description: "向绘制者子智能体发布绘画创作任务。绘制者会完善需求并调用专业工具生成图像，完成后将作品直接推送至前端展示。",
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
                description: "向演奏者子智能体发布音乐创作任务。演奏者会完善需求并调用专业工具创作音乐，完成后将乐谱和音频直接推送至前端展示。",
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
        },
        {
            type: "function",
            function: {
                name: "dispatch_learner",
                description: "向学习者子智能体发布学习研究任务。学习者会执行网络搜索和记忆库查询，收集信息后生成结构化的研究报告。适用于需要查证事实、搜索资料、研究分析等场景。",
                parameters: {
                    type: "object",
                    properties: {
                        description: {
                            type: "string",
                            description: "学习研究需求描述，如'搜索2024年诺贝尔物理学奖得主'、'调查人工智能最新进展'、'查一下量子计算的基本原理'。描述越清晰，搜索结果越准确。"
                        }
                    },
                    required: ["description"]
                }
            }
        }
    ];
    function parseArgs(args) {
        return typeof args === 'string' ? JSON.parse(args) : (args || {});
    }
    async function handleDispatchActor(args) {
        const { description } = parseArgs(args);
        if (!description || typeof description !== 'string' || description.trim().length === 0) {
            return ['行动任务调度失败：任务描述不能为空，请提供具体的行动需求', ''];
        }
        if (!actorRole) {
            return ['行动任务调度失败：行动者子智能体未就绪，请稍后重试', ''];
        }
        console.log(`[智能体控制] 调度行动者: ${description}`);
        const result = await actorRole.createCreativeWork(description.trim());
        console.log(`[智能体控制] 行动者完成: ${result}`);
        return [result, ''];
    }
    async function handleDispatchPainter(args) {
        const { description } = parseArgs(args);
        if (!description || typeof description !== 'string' || description.trim().length === 0) {
            return ['绘画任务调度失败：创作描述不能为空，请提供具体的绘画需求', ''];
        }
        if (!painterRole) {
            return ['绘画任务调度失败：绘制者子智能体未就绪，请稍后重试', ''];
        }
        console.log(`[智能体控制] 调度绘制者: ${description}`);
        const result = await painterRole.createCreativeWork(description.trim());
        console.log(`[智能体控制] 绘制者完成: ${result}`);
        return [result, ''];
    }
    async function handleDispatchMusician(args) {
        const { description } = parseArgs(args);
        if (!description || typeof description !== 'string' || description.trim().length === 0) {
            return ['音乐任务调度失败：创作描述不能为空，请提供具体的音乐需求', ''];
        }
        if (!musicianRole) {
            return ['音乐任务调度失败：演奏者子智能体未就绪，请稍后重试', ''];
        }
        console.log(`[智能体控制] 调度演奏者: ${description}`);
        const result = await musicianRole.createCreativeWork(description.trim());
        console.log(`[智能体控制] 演奏者完成: ${result}`);
        return [result, ''];
    }
    async function handleDispatchLearner(args) {
        const { description } = parseArgs(args);
        if (!description || typeof description !== 'string' || description.trim().length === 0) {
            return ['学习研究任务调度失败：研究描述不能为空，请提供具体的学习研究需求', ''];
        }
        if (!learnerRole) {
            return ['学习研究任务调度失败：学习者子智能体未就绪，请稍后重试', ''];
        }
        console.log(`[智能体控制] 调度学习者: ${description}`);
        const result = await learnerRole.createCreativeWork(description.trim());
        console.log(`[智能体控制] 学习者完成，报告长度: ${result.length} 字符`);
        return [result, ''];
    }
    GlobalConfig.LTPfunction.set('dispatch_actor', handleDispatchActor);
    GlobalConfig.LTPfunction.set('dispatch_painter', handleDispatchPainter);
    GlobalConfig.LTPfunction.set('dispatch_musician', handleDispatchMusician);
    GlobalConfig.LTPfunction.set('dispatch_learner', handleDispatchLearner);
    GlobalConfig.LTPdefinition.push(...agentControlTools);

    const EMOJI_REGEX = /[\u{1F600}-\u{1F64F}\u{1F300}-\u{1F5FF}\u{1F680}-\u{1F6FF}\u{1F900}-\u{1F9FF}\u{1FA70}-\u{1FAFF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}\u{FE00}-\u{FE0F}\u{1F1E0}-\u{1F1FF}\u{200D}\u{20E3}\u{FE0F}]/gu;
    const KAOMOJI_MARKS = '_^･・。.、；;～~ノﾉゞヾω￣▽△□○●☆★°´｀♪♫＞＜><｡一≧≦∇∀ﾟ⌒⌣◕';
    const KAOMOJI_REGEX = new RegExp(`[<＜＼]?[（(](?:[^\\s（()）\u4e00-\u9fff]|\u4e00){0,5}[${KAOMOJI_MARKS}](?:[^\\s（()）\u4e00-\u9fff]|\u4e00){0,5}[）)](?:[<＜>＞／\u30ce\u309e\u266a\u266b]*)?` +
        `|[>＜^TtOo0vV][_\\-=^><。.・oO][<＞^TtOo0vV]`, 'gu');
    const EMOTION_REGEX = new RegExp(`${EMOJI_REGEX.source}|${KAOMOJI_REGEX.source}`, 'gu');
    function extractThinkingBlocks(text) {
        const blocks = [];
        const regex = /<think>([\s\S]*?)<\/think>/gi;
        let match;
        while ((match = regex.exec(text)) !== null) {
            const content = match[1].trim();
            if (content.length > 0)
                blocks.push(content);
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
            if (ch === '(' || ch === '\uFF08')
                stack.push(i);
            else if (ch === ')' || ch === '\uFF09') {
                if (stack.length === 0)
                    continue;
                const start = stack.pop();
                if (stack.length !== 0)
                    continue;
                const content = text.slice(start + 1, i).trim();
                if (content.length > 0)
                    blocks.push(content);
                ranges.push([start, i + 1]);
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
        const regex = new RegExp(EMOTION_REGEX.source, 'gu');
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
        const remaining = text.replace(regex, '');
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
        const allowed = '\\u4e00-\\u9fff' + 'a-zA-Z0-9' + '\\s_~\\-' + '\uFF0C\u3002\uFF1F\uFF1A\uFF01\uFF1B\u3001\u2014\u2026\u300A\u300B\u3008\u3009\u201C\u201D\u2018\u2019\uFF08\uFF09\u3010\u3011' + ',.\'\"?:!;()\\[\\]';
        const whitelist = new RegExp(`[^${allowed}]`, 'g');
        processed = processed.replace(whitelist, ',');
        processed = processed.replace(/,{2,}/g, ',');
        processed = processed.replace(/\s+/g, ' ');
        return processed.trim();
    }
    function removeEmojiSymbols(text) {
        return text.replace(/[\u{1F600}-\u{1F64F}\u{1F300}-\u{1F5FF}\u{1F680}-\u{1F6FF}\u{1F900}-\u{1F9FF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}\u{FE00}-\u{FE0F}\u{1F1E0}-\u{1F1FF}\u{200D}\u{20E3}\u{FE0F}]/gu, '');
    }
    function formatChunk(chunk) {
        const LEADING_PUNCT = /^[。，、：；:;,?!？！—～"'""''()（）\[\]【】{}<>…\s]+/;
        const TRAILING_COMMA = /[，,]+$/;
        const result = chunk.replace(LEADING_PUNCT, '').replace(TRAILING_COMMA, '');
        return result.trim();
    }
    function splitByPunct(source, punctRegex) {
        const result = [];
        let start = 0;
        for (let i = 0; i < source.length; i++) {
            if (!punctRegex.test(source[i]))
                continue;
            let end = i + 1;
            while (end < source.length && punctRegex.test(source[end]))
                end++;
            const fragment = source.slice(start, end).trim();
            if (fragment.length > 0)
                result.push(fragment);
            start = end;
            i = end - 1;
        }
        if (start < source.length) {
            const fragment = source.slice(start).trim();
            if (fragment.length > 0)
                result.push(fragment);
        }
        return result;
    }
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
    function splitSentences(text) {
        if (!text)
            return [];
        const LEVEL1_PUNCT = /[。？！—～?!]/;
        const LEVEL2_PUNCT = /[，,、：；:;]/;
        const IDEAL_MAXIMUM_LENGTH = 35;
        const level1 = splitByPunct(text, LEVEL1_PUNCT);
        const result = [];
        for (let content of level1) {
            if (content.length <= IDEAL_MAXIMUM_LENGTH) {
                const formatted = formatChunk(content);
                if (formatted.length > 0)
                    result.push(formatted);
                continue;
            }
            while (content.length > IDEAL_MAXIMUM_LENGTH) {
                let splitPos = -1;
                for (let i = Math.min(content.length - 1, IDEAL_MAXIMUM_LENGTH - 1); i >= 0; i--) {
                    if (!LEVEL2_PUNCT.test(content[i]) || isInsideBracket(content, i))
                        continue;
                    let end = i + 1;
                    while (end < content.length && LEVEL2_PUNCT.test(content[end]))
                        end++;
                    splitPos = end;
                    break;
                }
                if (splitPos === -1)
                    break;
                const slice = formatChunk(content.slice(0, splitPos));
                if (slice.length > 0)
                    result.push(slice);
                content = content.slice(splitPos);
            }
            const tail = formatChunk(content);
            if (tail.length > 0)
                result.push(tail);
        }
        return result;
    }
    function parseContent(rawText) {
        if (!rawText)
            return { thinkingBlocks: [], codeBlocks: [], actionBlocks: [], textChunks: [] };
        const [thinkingBlocks, textAfterThinking] = extractThinkingBlocks(rawText);
        const [codeBlocks, textAfterCode] = extractCodeBlocks(textAfterThinking);
        const [emotionBlocks, textAfterEmotion] = extractEmotionBlocks(textAfterCode);
        const [actionZoneBlocks, textAfterAction] = extractActionBlocks(textAfterEmotion);
        const actionBlocks = [...actionZoneBlocks, ...emotionBlocks];
        const displayText = removeEmojiSymbols(textAfterAction);
        const displayChunks = splitSentences(displayText);
        const textChunks = displayChunks.map(chunk => ({ display: chunk, tts: cleanTextForTTS(chunk), }));
        return { thinkingBlocks, codeBlocks, actionBlocks, textChunks };
    }

    exports.ActorRole = ActorRole;
    exports.CalculateMedian = CalculateMedian;
    exports.CalculateModes = CalculateModes;
    exports.Clamp = Clamp;
    exports.CreativeRoleBase = CreativeRoleBase;
    exports.DialogueRole = DialogueRole;
    exports.FileToBase64 = FileToBase64;
    exports.GlobalConfig = GlobalConfig;
    exports.LearnerRole = LearnerRole;
    exports.LiteImageFile = LiteImageFile;
    exports.ModelBuilder = ModelBuilder;
    exports.MusicianRole = MusicianRole;
    exports.PainterRole = PainterRole;
    exports.RandomFloat = RandomFloat;
    exports.RandomFloor = RandomFloor;
    exports.ViewerRole = ViewerRole;
    exports.actorRole = actorRole;
    exports.agentControlTools = agentControlTools;
    exports.calculateFileHash = calculateFileHash;
    exports.checkDueItems = checkDueItems;
    exports.cleanTextForTTS = cleanTextForTTS;
    exports.descriptionRole = descriptionRole;
    exports.ensureMemoryReady = ensureMemoryReady;
    exports.extractTextFromMessage = extractTextFromMessage;
    exports.fetchDocumentCallback = fetchDocumentCallback;
    exports.getFileContent = getFileContent;
    exports.getPromptFromKnowledge = getPromptFromKnowledge;
    exports.initSchedules = initSchedules;
    exports.learnerRole = learnerRole;
    exports.memorizeUnreadRecords = memorizeUnreadRecords;
    exports.musicianRole = musicianRole;
    exports.painterRole = painterRole;
    exports.parseContent = parseContent;
    exports.queryFromKnowledge = queryFromKnowledge;
    exports.randomDefaultMessage = randomDefaultMessage;
    exports.removeEmojiSymbols = removeEmojiSymbols;
    exports.saveImageToServer = saveImageToServer;
    exports.savePromptToKnowledge = savePromptToKnowledge;
    exports.scheduleTools = scheduleTools;
    exports.screenshotTools = screenshotTools;
    exports.splitSentences = splitSentences;
    exports.splitTextToStrings = splitTextToStrings;
    exports.toBtoaString = toBtoaString;

    return exports;

})({});
