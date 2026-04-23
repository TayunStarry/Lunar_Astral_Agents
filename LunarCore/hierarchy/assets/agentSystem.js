function GOsave(fileName, overwrite, fileData) {
    console.log('在磁盘中保存文件', fileName, overwrite, fileData);
    return shareFileSave(fileName, overwrite, fileData);
}
function GOread(filePath) {
    console.log('从磁盘中读取文件', filePath);
    return shareFileRead(filePath);
}
function GOlist(path) {
    console.log('获取目录下所有文件列表', path);
    return shareFileList(path);
}
function GOdatabase(request) {
    console.log('执行数据库请求', request);
    return shareDatabase(request);
}
function GOaddress() {
    console.log('获取当前地址信息');
    return shareAddress();
}
function GOcurrentUrl() {
    console.log('获取当前系统访问URL');
    return shareCurrentUrl();
}
function GOkeyframe(inputFile, cacheDir) {
    console.log('提取视频关键帧', inputFile, cacheDir);
    return shareVideoKeyframe(inputFile, cacheDir);
}
function GOfetch(config) {
    console.log('网络请求', config);
    return shareFetch(config);
}
function GOresize(imgData) {
    console.log('缩放图片');
    return shareResizeImage(imgData);
}

class OnlyData {
    static systemKey = 'key-520-1314-2000-02-18';
    static modelEmbedingName = "system-embedding";
    static modelMultimodalName = "system-multimodal";
    static customConfig = { cloud: {} };
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
        return GOcurrentUrl()[0] + '/v1';
    }
    ;
    static get fileServiceUrl() {
        return GOcurrentUrl()[0];
    }
    ;
    static get MultimodalUrl() {
        return OnlyData.customConfig.cloud.multimodalModelUrl || OnlyData.systemUrl;
    }
    ;
    static get MultimodalKey() {
        return OnlyData.customConfig.cloud.multimodalModelKey || OnlyData.systemKey;
    }
    ;
    static get MultimodalName() {
        return OnlyData.customConfig.cloud.multimodalModelName || OnlyData.modelMultimodalName;
    }
    ;
    static get EmbeddingUrl() {
        return OnlyData.customConfig.cloud.embeddingModelUrl || OnlyData.systemUrl;
    }
    ;
    static get EmbeddingKey() {
        return OnlyData.customConfig.cloud.embeddingModelKey || OnlyData.systemKey;
    }
    ;
    static get EmbeddingName() {
        return OnlyData.customConfig.cloud.embeddingModelName || OnlyData.modelEmbedingName;
    }
    ;
    static get userName() {
        return OnlyData.customConfig.cloud.userName || "你";
    }
    ;
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
        let address = "";
        if (currentAddress.length === 0)
            address = (GOaddress()[0]).join(' ');
        else
            address = currentAddress.join(' ');
        return prompt
            .replace(/{name}/g, OnlyData.userName)
            .replace(/{current-time}/g, new Date().toLocaleString())
            .replace(/{current-address}/g, address);
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
        if (this.messages.length > 30)
            this.messages.slice(-30).push(context);
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
    async runMultimodal() {
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
            Authorization: `Bearer ${encodeURIComponent(OnlyData.MultimodalKey)}`,
            "Content-Type": "application/json",
        };
        const modelRequest = {
            method: "POST",
            crossDomain: true,
            headers,
            body: JSON.stringify(requestBody)
        };
        const endpoint = "/chat/completions";
        const [result, error] = GOfetch({ url: OnlyData.MultimodalUrl + endpoint, execute: modelRequest });
        if (error)
            throw error;
        return result;
    }
    async runEmbedding() {
        const validMessages = this.extractTextFromMessages(this.messages);
        const requestBody = {
            model: OnlyData.EmbeddingName,
            input: validMessages,
            stream: this.stream,
        };
        const headers = {
            Authorization: `Bearer ${encodeURIComponent(OnlyData.EmbeddingKey)}`,
            "Content-Type": "application/json",
        };
        const modelRequest = {
            method: "POST",
            crossDomain: true,
            headers,
            body: JSON.stringify(requestBody)
        };
        const endpoint = "/embeddings";
        const [result, error] = GOfetch({ url: OnlyData.EmbeddingUrl + endpoint, execute: modelRequest });
        if (error)
            throw error;
        return result.data[0].embedding.slice(0, 256);
    }
    constructor() { super(); }
}

function getFileContent(path, removeNewLines = false) {
    let [content, size, mimeType, err] = GOread(path);
    if (err)
        throw err;
    if (removeNewLines)
        return String(content).replace(/[\r\n]+/g, '');
    return String(content).replace(/[ \t]+/g, ' ');
}
async function fetchDocumentCallback(url, initializeContent = '{}', callback) {
    const defaultCallback = (content) => JSON.parse(content);
    const applyCallback = defaultCallback;
    const fallback = async () => {
        GOsave(url.toString(), true, initializeContent);
        return applyCallback(initializeContent);
    };
    try {
        const filePath = url.toString().split(/[\/\\]/);
        const [fileList, err1] = GOlist(filePath.slice(0, -1).join('/'));
        if (!err1)
            return await fallback();
        const exists = fileList.some(item => item.name === filePath[filePath.length - 1] && !item.isDir);
        if (!exists)
            return await fallback();
        const [content, size, mimeType, err2] = GOread(url.toString());
        if (!err2)
            return await fallback();
        const text = String(content);
        if (!text)
            return await fallback();
        return applyCallback(text);
    }
    catch (error) {
        if (error instanceof Error)
            return await fallback();
    }
}

function queryFromDatabase(operations, createTableOperation) {
    const requestBody = { operations, transaction: false };
    let [result, error] = GOdatabase(requestBody);
    if (!error)
        throw new Error('数据库查询失败');
    if (!result.success || !result.results[0].success) {
        const errorMessage = result.error || result.results[0].error || '';
        if (errorMessage.includes('no such table') && createTableOperation) {
            const createTableRequest = { operations: [createTableOperation], transaction: false };
            let [createTableResult, tableError] = GOdatabase(createTableRequest);
            if (!tableError)
                throw new Error('创建表失败');
            if (!createTableResult.success)
                throw new Error('创建表失败');
            [result, error] = GOdatabase(requestBody);
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

class ChatDialogueRole extends ModelBuilder {
    async callMultimediaAndToolParsing(cache, source) {
        try {
            await source.LiteImageFile();
            source.unreadContext.forEach(context => this.writeContext(context));
            source.unreadContext = [];
            const response = await this.run;
            if (!response.ok) {
                source.finalResponse = `月华发现了一个错误: ${response.status} ${response.statusText}`;
                return;
            }
            const responseText = await response.text();
            this.analyzeMessageResponse(responseText, cache, source);
            if (cache.toolCalls.length > 0) {
                const hasProcessedToolCalls = await this.batchExecutionToolCall(cache, source);
                if (hasProcessedToolCalls)
                    return await this.callMultimediaAndToolParsing(cache, source);
            }
        }
        catch (error) {
            console.error('请求处理错误:', error);
        }
        this.updateMessageContent(cache, source);
    }
    analyzeMessageResponse(message, cache, source) {
        try {
            const jsonData = JSON.parse(message);
            if (jsonData.choices?.[0]?.message?.reasoning_content) {
                cache.thinkingContent = jsonData.choices[0].message.reasoning_content;
            }
            if (jsonData.timings?.predicted_per_second) {
                source.responseSpeed = jsonData.timings.predicted_per_second;
            }
            if (jsonData.choices?.[0]?.message?.tool_calls) {
                for (const toolCall of jsonData.choices[0].message.tool_calls) {
                    try {
                        toolCall.function.arguments = JSON.parse(toolCall.function.arguments);
                        cache.toolCalls.push(toolCall);
                    }
                    catch (parseError) {
                        console.error('工具调用参数解析错误:', parseError);
                    }
                }
            }
            if (jsonData.choices?.[0]?.message?.content) {
                cache.descriptionContent = jsonData.choices[0].message.content;
            }
        }
        catch (error) {
            console.error('聊天消息响应处理错误:', error);
        }
    }
    async batchExecutionToolCall(state, source) {
        let hasToolCalls = false;
        for (const toolCall of state.toolCalls) {
            if (toolCall.type !== "function")
                continue;
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
            const newThinkTag = '<think>\n' + state.thinkingContent + '\n</think>';
            source.finalResponse = newThinkTag + state.descriptionContent;
        }
        else
            source.finalResponse = state.descriptionContent;
        if (source.finalResponse.trim() === "")
            return source.defaultAnswer;
        return source.finalResponse;
    }
    constructor() {
        super();
        this.useMultimodal(getFileContent('resources/prompts/chatRole.md'));
    }
}

function RandomFloor(min, max) {
    return Math.floor(Math.random() * (max - min + 1) + min);
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
    selfAppearancePrompt = getFileContent('resources/prompts/selfAppearance.md');
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
        this.useMultimodal(getFileContent('resources/prompts/painterRole.md'));
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
    defaultAnswer = "月华不知道哦";
    constructor() {
        this.compilePlan.useMultimodal(getFileContent('resources/prompts/compilePlan.md'));
        this.queryKeywords.useMultimodal(getFileContent('resources/prompts/queryKeywords.md'));
        this.emotionManager.useMultimodal(getFileContent('resources/prompts/emotionManager.md'));
        this.recorderRole.useMultimodal(getFileContent('resources/prompts/recorderRole.md'));
        this.summaryRole.useMultimodal(getFileContent('resources/prompts/summaryRole.md'));
        this.descriptionRole.useMultimodal(getFileContent('resources/prompts/descriptionRole.md'));
        fetchDocumentCallback('lunar_config.json').then(content => OnlyData.customConfig = JSON.parse(content));
    }
    async analysisVideoFile(videoUrl, userNeeds) {
        const cachedPrompt = getPromptFromDatabase(videoUrl);
        if (cachedPrompt) {
            this.unreadContext.push({ role: 'user', content: cachedPrompt });
            return;
        }
        const [keyFrames, error] = GOkeyframe(videoUrl, './cache');
        if (!keyFrames || keyFrames.length === 0 || error)
            throw new Error('提取关键帧失败');
        const sandboxMessages = [];
        let videoSummary = '';
        const frameMessages = keyFrames.map(frame => ({ type: "image_url", image_url: { url: `data:image/jpeg;base64,${frame.data}` } }));
        for (let i = 0; i < frameMessages.length; i += 20) {
            const batchFrames = frameMessages.slice(i, i + 20);
            this.descriptionRole.coverContext({ role: 'user', content: batchFrames });
            const summaryRequest = await (await this.descriptionRole.run).json();
            const summary = summaryRequest?.choices?.[0]?.message?.content;
            if (summary && summary.trim().length > 0)
                sandboxMessages.push(summary);
        }
        if (sandboxMessages.length > 1) {
            this.summaryRole.coverContext({ role: 'user', content: sandboxMessages });
            const summaryRequest = await (await this.summaryRole.run).json();
            videoSummary = summaryRequest?.choices?.[0]?.message?.content;
        }
        else if (sandboxMessages.length === 1)
            videoSummary = sandboxMessages[0].text;
        else
            videoSummary = this.defaultAnswer;
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
            for (let item of message.content) {
                if (item.type == 'text')
                    continue;
                if (OnlyData.videoFormatsExtensions.some(format => item.image_url.url.toLowerCase().endsWith(format))) {
                    await this.analysisVideoFile(item.image_url.url, '');
                }
                else if (!item.image_url.url.startsWith("data:image")) {
                    const [response, error] = GOfetch({ url: item.image_url.url, execute: { crossDomain: true } });
                    if (error)
                        throw new Error('获取图片文件失败');
                    if (!response.ok)
                        throw new Error(`获取图片文件失败: ${response.status} ${response.statusText}`);
                    const blob = await response.blob();
                    const [resizedBlob, error1] = GOresize(blob);
                    if (error1)
                        throw new Error('缩放图片失败');
                    item.image_url.url = resizedBlob.base64;
                }
            }
        }
    }
}

class LunarAgent extends AgentDefine {
    messageWeight = 1;
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
        const cache = {
            currentToolCallIndex: -1,
            currentFunctionArgs: '',
            currentFunctionName: '',
            descriptionContent: '',
            thinkingContent: '',
            currentToolCall: null,
            toolCalls: [],
        };
        await this.chatDialogueRole.callMultimediaAndToolParsing(cache, this);
        return this.finalResponse;
    }
    constructor() { super(); }
    async thinkingChainProcess() {
        while (true) {
            console.log('思考链处理');
            await new Promise(resolve => setTimeout(resolve, 1000));
            const messageLength = this.unreadContext.length + this.unreadVideoUrl.length;
            if (messageLength === 0 && RandomFloor(0, 100) <= this.messageWeight)
                continue;
            await new Promise(resolve => setTimeout(resolve, 1000));
            await this.batchProcessVideoFiles();
            await new Promise(resolve => setTimeout(resolve, 1000));
            await this.createChatMessage();
            await new Promise(resolve => setTimeout(resolve, 1000));
            console.log(this.finalResponse);
        }
    }
}
setTimeout(awakenAgent, 1000);
function awakenAgent() {
    console.log('智能体系统已唤醒');
    const agent = new LunarAgent();
    console.log('思考链处理0');
    setTimeout(() => agent.unreadContext.push({ role: 'user', content: '你好' }), 5000);
    setTimeout(() => console.log(agent.unreadContext), 5000);
    setTimeout(() => agent.unreadContext.push({ role: 'user', content: '你叫什么名字' }), 10000);
    setTimeout(() => console.log(agent.unreadContext), 10000);
    setTimeout(() => agent.unreadContext.push({ role: 'user', content: '你的哥哥叫什么名字' }), 15000);
    setTimeout(() => console.log(agent.unreadContext), 15000);
}

