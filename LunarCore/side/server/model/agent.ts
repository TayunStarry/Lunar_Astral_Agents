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
    /**
     * 创建消息
     *
     * @returns {Promise<string>} - 最终应答
     */
    public async createChatMessage(): Promise<string> {
        /** 初始化聊天缓存 */
        const cache: ChatCache = {
            currentToolCallIndex: -1,
            currentFunctionArgs: '',
            currentFunctionName: '',
            descriptionContent: '',
            thinkingContent: '',
            currentToolCall: null,
            toolCalls: [],
        };
        // 发送请求并获取响应
        await this.chatDialogueRole.callMultimediaAndToolParsing(cache, this);
        // 减少发言权重
        this.speakWeight--;
        // 返回最终应答
        return this.finalResponse;
    }
    /** 构建智能体 并 初始化各个子模型的系统提示词 */
    public constructor() { super(); this.thinkingChainProcess(); }
    /** 思考链处理 */
    protected async thinkingChainProcess() {
        while (true) {
            // 拉取外部消息
            this.pullExternalMessages();
            // 等待1秒
            await new Promise(resolve => setTimeout(resolve, 1000));
            /** 消息长度 */
            const messageLength = this.unreadContext.length + this.unreadVideoUrl.length;
            // 如果消息长度为0，且随机数大于发言权重，继续循环
            if (messageLength === 0 && RandomFloor(0, 100) > this.speakWeight) {
                // 等待5秒
                await new Promise(resolve => setTimeout(resolve, 5000));
                // 使得发言权重增加或减少（范围：-5到5）
                //this.speakWeight = Clamp({ min: 0, max: 100 }, this.speakWeight + RandomFloor(-5, 5));
                // 进入下一次循环
                continue;
            }
            // 等待1秒
            await new Promise(resolve => setTimeout(resolve, 1000));
            // 批量处理视频文件
            await this.batchProcessVideoFiles();
            // 等待1秒
            await new Promise(resolve => setTimeout(resolve, 1000));
            // 创建消息
            await this.createChatMessage();
            // 等待1秒
            await new Promise(resolve => setTimeout(resolve, 1000));
            console.log(this.finalResponse);
            // 使用ws发送消息
            // this.ws.send(JSON.stringify({ role: 'assistant', content: this.finalResponse }));
        }
    }
    /** 拉取外部消息 */
    protected pullExternalMessages() {
        // 合并消息
        pullContext().forEach(message => { this.writeMessage(message.role, message.content); })
        // 合并视频URL
        pullVideoUrl().forEach(videoUrl => { this.writeVideoUrl(videoUrl); })
    }
    /** 写入消息 */
    public writeMessage(role: PostMessageRole, messages: Array<ImageContent | TextContent>) {
        // 从外部写入消息
        this.unreadContext.push({ role, content: messages });
        // 增加随机的发言权重
        this.speakWeight += RandomFloor(1, 3);
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
}

const AgentExample = new LunarAgent();
// setTimeout(() => AgentExample.writeMessage('user', [{ type: 'text', text: '你好' }]), 5000);
// setTimeout(() => AgentExample.writeMessage('user', [{ type: 'text', text: '你叫什么名字' }]), 10000);
// setTimeout(() => AgentExample.writeMessage('user', [{ type: 'text', text: '你的哥哥叫什么名字' }]), 15000);
// setTimeout(() => AgentExample.writeMessage('user', [{ type: 'text', text: '你是一个智能体' }]), 20000);
// AgentExample.testMessageWrite('user', [{ type: 'image_url', image_url: { url: url()[0] + '/read/images/YmJjY2FhLm1wNA==.mp4' } }], 1000);
