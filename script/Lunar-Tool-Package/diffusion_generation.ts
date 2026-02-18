import { ToolCallParameters, subscriptionToolCall, HistoryMessage, FileListItem, TaskStatus, showSystemMessage, addImageRendering, createImageMessage } from '../EntryAPI/code';

// 注册工具函数
subscriptionToolCall("diffusion_generation",
    async (args: ToolCallParameters, messageElement: HTMLElement, messageObject: HistoryMessage) => {
        if (!args.prompt) return '生成图片需要提供正向提示文本';
        // 显示系统消息, 提示用户图片生成任务已提交
        messageElement.innerHTML = '<em><strong>月华正在努力绘制中...请稍等片刻</strong></em>';
        // 直接创建图片生成任务并等待结果
        const result = await createImageGeneration(args, messageObject);
        // 返回JSON响应
        return result;
    }
);

/** 提交图片生成任务 */
async function createImageGeneration(args: ToolCallParameters, messageObject: HistoryMessage): Promise<string> {
    /** 获取生成的图片列表 */
    const fileList = await fetch(`/file_list/generated`).then(res => res.json()) as FileListItem[];
    /** 排序文件列表, 取最新生成的图片 */
    const imageUrl = fileList.sort((a, b) => new Date(b.lastModified).getTime() - new Date(a.lastModified).getTime())[0]?.path;
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
        init_img: args.use_reference ? imageUrl : null,
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
    if (!response.ok) return `尝试画图失败了, 失败原因是: ${response.statusText} 请向用户解释一下`;
    /** 获取图片生成任务ID */
    const TaskId = (await response.json() as TaskStatus).task_id;
    // 轮询查询图片生成状态，等待结果
    const isSuccess = await searchImagesTask(TaskId, messageObject);
    // 根据轮询结果返回相应消息
    if (isSuccess) return `图片绘制完成！这是你的正面提示词: [ ${args.prompt} ] 负面提示词: [ ${args.negative_prompt} ] 请你简要描述一下画面内容，让用户更好地理解这幅画`;
    // 若生成失败, 则告知用户图片生成任务失败
    else return '图片生成失败，请向用户说明情况（例如：画笔暂时无法使用）';
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