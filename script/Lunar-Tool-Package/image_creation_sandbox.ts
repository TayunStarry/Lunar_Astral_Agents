import { ToolCallParameters, subscriptionToolCall, HistoryMessage, FileListItem, TaskStatus, showSystemMessage, addImageRendering, createImageMessage, OnlyData, RandomFloor } from '../EntryAPI/code';

// 注册工具函数
subscriptionToolCall("image_creation_sandbox",
    async (args: ToolCallParameters, messageElement: HTMLElement, messageObject: HistoryMessage) => {
        if (!args.prompt) return '亲爱的月华，你需要提供一个描述画面的文字，就像告诉画家你想要什么场景一样';
        // 处理清空缓存模式
        if (args.clear_cache) {
            // 清空附件数组
            OnlyData.toolAttachment.length = 0;
            // 返回工具消息
            return '已经清空了之前绘制的参考作品，准备好全新创作了';
        }
        // 显示系统消息, 提示用户图片生成任务已提交
        messageElement.innerHTML = '<em><strong>月华的画笔在画布上轻轻舞动，正在将想象变为现实...</strong></em>';
        /** 从沙箱中随机获取一张历史图片作为参考 */
        let referenceImageUrl = OnlyData.toolAttachment.length != 0 ? OnlyData.toolAttachment[RandomFloor(0, OnlyData.toolAttachment.length - 1)]?.image_url : undefined;
        // 直接创建图片生成任务并等待结果
        const result = await createImageGeneration(args, messageObject, referenceImageUrl);
        // 返回JSON响应
        return result;
    }
);

/** 提交图片生成任务 */
async function createImageGeneration(args: ToolCallParameters, messageObject: HistoryMessage, imageUrl: string | undefined): Promise<string> {
    /** 定义图片生成数据 */
    const generateData = {
        prompt: args.prompt?.trim(),
        negative_prompt: args.negative_prompt?.trim(),
        batch_size: 1,
        width: 512,
        height: 512,
        steps: 20,
        seed: Date.now() % 1000000000,
        cfg_scale: args.cfg_scale ?? 1.0,
        init_img: imageUrl,
        strength: args.strength ?? 0.65,
    };
    /** 发送POST请求 */
    const response = await fetch('/generate',
        {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify(generateData)
        }
    );
    // 检查响应状态
    if (!response.ok) return `月华的画笔似乎有点卡顿呢，可以请您稍后再试试吗？或者试试不同的描述方式？`;
    /** 获取图片生成任务ID */
    const TaskId = (await response.json() as TaskStatus).task_id;
    // 轮询查询图片生成状态，等待结果
    const isSuccess = await searchImagesTask(TaskId, messageObject);
    // 根据轮询结果返回相应消息
    if (isSuccess) return [
        '**新的画作完成了**',
        `- 正面提示词: ${args.prompt?.trim() ?? ''}`,
        `- 反面提示词：${args.negative_prompt?.trim() ?? ''}`,
        '画作已收入月华的画廊中，接下来月华可以:',
        '1. 评价这幅作品的风格和意境',
        '2. 提出优化建议（色彩、构图、细节等）',
        '3. 询问用户是否满意，或想继续调整什么',
        '> 月华，请用你专业的艺术眼光来分享见解吧！',
    ].join('\n');
    // 若生成失败, 则告知用户图片生成任务失败
    else return '月华这次没能完成画作，画笔似乎不太听使唤。可以安慰用户说"创作偶尔也会遇到灵感枯竭的时候"，并邀请用户换个描述试试看。';
}

/** 使用WebSocket等待图片生成完成 */
async function searchImagesTask(taskId: string, messageObject: HistoryMessage): Promise<boolean> {
    function event(resolve: (value: boolean | PromiseLike<boolean>) => void) {
        /** 创建EventSource连接到新的/generate/wait接口 */
        const eventSource = new EventSource(`/generate/wait?task_id=${taskId}`);
        // 处理接收到的消息
        eventSource.onmessage = function (event) {
            try {
                // 解析接收到的消息数据
                const data = JSON.parse(event.data);
                // 检查任务状态
                if (data.status === 'completed') {
                    // 任务完成，使用返回的read_path
                    const imageUrl = data.read_path;
                    /** 创建一个新的音频元素用于播放提示音 */
                    const audio = new Audio('/read/resources/audios/prompt-tone.mp3');
                    // 设置音量为最大
                    audio.volume = 1.0;
                    // 播放提示音, 失败时显示错误消息
                    audio.play().catch(() => showSystemMessage('播放提示音失败', 'error'));
                    /** 创建图片消息对象 */
                    const imageMessage = createImageMessage('assistant', '月华绘制的图片', imageUrl);
                    // 添加图片渲染到消息元素
                    addImageRendering(imageMessage);
                    // 存储图片URL到消息对象, 用于后续引用
                    messageObject.imageUrl = imageUrl;
                    // 关闭EventSource连接
                    eventSource.close();
                    resolve(true);
                }
                else if (data.status === 'failed') {
                    // 任务失败
                    showSystemMessage(`图片绘制失败`, 'error');
                    // 关闭EventSource连接
                    eventSource.close();
                    resolve(false);
                }
            }
            catch (error) {
                console.error('处理消息失败:', error);
                showSystemMessage(`处理消息失败`, 'error');
                eventSource.close();
                resolve(false);
            }
        };
        // 处理错误
        eventSource.onerror = function (error) {
            console.error('EventSource错误:', error);
            showSystemMessage(`图片绘制状态查询失败`, 'error');
            eventSource.close();
            resolve(false);
        };
    }
    return new Promise<boolean>(event);
}