import { ChatCache, RandomFloor, AgentDefine, ImageContent, TextContent, PostMessageRole, Clamp } from '../index';

/** 月华智能体 */
class LunarAgent extends AgentDefine {
    /** 发言权重 */
    protected speakWeight: number = 1;
    /**
     * 批量处理视频文件
     *
     * @param {string} [userNeeds] - 用户需求
     * 
     * @returns {Promise<void>} - 处理完成后的 Promise
     */
    public async batchProcessVideoFiles(userNeeds?: string): Promise<void> {
        // 如果未读视频文件数组为空，直接返回
        if (this.unreadVideoUrl.length === 0) return;
        //  遍历未读视频文件数组
        for (const videoUrl of this.unreadVideoUrl) {
            try {
                // 处理视频文件
                await this.analysisVideoFile(videoUrl, userNeeds || '');
                // 等待1秒，避免对服务器造成过大压力
                await new Promise(resolve => setTimeout(resolve, 1000));
            }
            catch (error) { continue; }
        }
        // 清空未读视频文件数组
        this.unreadVideoUrl = [];
    }
    /** 创建聊天消息 */
    public async createChatMessage(): Promise<string> {
        /** 初始化聊天缓存 */
        const cache: ChatCache = { currentToolCallIndex: -1, currentFunctionArgs: '', currentFunctionName: '', descriptionContent: '', thinkingContent: '', currentToolCall: null, toolCalls: [], };
        // 发送请求并获取响应
        await this.chatDialogueRole.callMultimediaAndToolParsing(cache, this);
        // 减少发言权重
        this.speakWeight--;
        // 返回最终应答
        return this.finalResponse;
    }
    /** 思考链处理 */
    protected async thinkingChainProcess() {
        let errorCount = 0;
        while (true) {
            try {
                // 拉取外部消息
                await this.pullExternalMessages();
                /** 消息长度 */
                const messageLength = this.unreadContext.length + this.unreadVideoUrl.length;
                /** 消息类型 */
                const messageType = messageLength === 0 ? 'response' : 'active';
                /** 是否允许发言 */
                const allowSpeak = RandomFloor(15, 100) < this.speakWeight;
                // 如果消息长度为0，且不允许发言，继续循环
                if (messageLength === 0 && !allowSpeak) {
                    // 等待1秒
                    await new Promise(resolve => setTimeout(resolve, 1000));
                    // 进入下一次循环
                    continue;
                }
                // 如果消息长度为0，且允许发言，重置发言权重
                else if (messageLength == 0 && allowSpeak) this.speakWeight = 0;
                // 批量处理视频文件
                await this.batchProcessVideoFiles();
                // 创建消息
                await this.createChatMessage();
                /** 消息响应 */
                const messageResponse = this.finalResponse.trim().length ? this.finalResponse : this.randomDefaultMessage;
                // 将消息推送至外部客户端
                pushContext(messageType, messageResponse);
            }
            catch (error) {
                // 推送错误消息
                if (this.pushErrorMessage(error as Error, errorCount)) break;
                // 错误次数增加
                errorCount++;
            }
        }
    }
    /** 推送错误消息 */
    protected pushErrorMessage(error: Error, errorCount: number): boolean {
        // 打印错误信息
        console.error(error.message, ' || ', error.stack);
        // 如果错误次数小于3次，则继续循环
        if (errorCount < 3) return false;
        // 随机选择一个错误消息
        pushContext('active', this.defaultAnswers[RandomFloor(0, this.defaultAnswers.length - 1)]);
        // 终止思考链循环
        return true;
    }
    /** 拉取外部消息 */
    protected async pullExternalMessages() {
        // 合并消息
        pullContext().forEach(message => this.writeMessage(message.role, message.content))
        // 合并视频URL
        pullVideoUrl().forEach(videoUrl => { this.writeVideoUrl(videoUrl); })
        // 等待1秒
        await new Promise(resolve => setTimeout(resolve, 1000));
    }
    /** 写入消息 */
    public writeMessage(role: PostMessageRole, messages: Array<ImageContent | TextContent>) {
        // 从外部写入消息
        this.unreadContext.push({ role, content: messages });
        // 增加随机的发言权重
        this.speakWeight += RandomFloor(1, 3);
        // 如果消息是字符串，将其转换为文本消息
        if (typeof messages === 'string') messages = [{ type: 'text', text: messages }];
        // 打印文本消息
        messages.forEach(message => { if (message.type === 'text') console.log(message.text); })
    }
    /** 写入视频文件 */
    public writeVideoUrl(videoUrl: string) {
        console.log('写入视频文件:' + videoUrl);
        // 从外部写入视频文件
        this.unreadVideoUrl.push(videoUrl);
        // 增加随机的发言权重
        this.speakWeight += RandomFloor(1, 3);
    }
    /** 测试消息写入 */
    public async testMessageWrite(role: PostMessageRole, messages: Array<ImageContent | TextContent>, timeout: number) {
        // 等待指定超时时间
        await new Promise(resolve => setTimeout(resolve, timeout));
        // 如果消息数组非空，写入消息
        if (messages.length > 0) this.writeMessage(role, messages);
    }
    /** 构建智能体 并 初始化各个子模型的系统提示词 */
    public constructor() { super(); this.thinkingChainProcess(); }
}

const AgentExample = new LunarAgent();
// setTimeout(() => AgentExample.writeMessage('user', [{ type: 'text', text: '你好' }]), 5000);
// setTimeout(() => AgentExample.writeMessage('user', [{ type: 'text', text: '你叫什么名字' }]), 10000);
// setTimeout(() => AgentExample.writeMessage('user', [{ type: 'text', text: '你的哥哥叫什么名字' }]), 15000);
// setTimeout(() => AgentExample.writeMessage('user', [{ type: 'text', text: '你是一个智能体' }]), 20000);
// const message: Array<ImageContent | TextContent> = [
//     {
//         type: 'image_url',
//         image_url: { url: url()[0] + '/read/images/6b4029976c90a71e.jpg' }
//     },
//     {
//         type: 'text',
//         text: '描述一下这张图片的内容'
//     }
// ];
// AgentExample.testMessageWrite('user', message, 5000);
const message: Array<ImageContent | TextContent> = [
    {
        type: 'text',
        text: '像与老朋友见面一样，打个招呼吧'
    }
];
AgentExample.testMessageWrite('user', message, 1500);
