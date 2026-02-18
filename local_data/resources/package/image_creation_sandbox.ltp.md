# LTP 工具模块

## 元数据

- **名称**：image_creation_sandbox
- **版本**：1.0.0
- **作者**：[钛宇-星光阁](https://gitee.com/TayunStarry)
- **更新日志**：
  - 2026-02-18：创建初始版本

## 模块描述

- 本模块是一个具备历史感知能力的图像生成沙箱，内置智能缓存机制，可在局部保留最多10张过往生成的图像作为创作素材池。
- 这些历史图像在常规对话中不会暴露于上下文，保持会话整洁；但在每次调用时，若未主动清空缓存，系统将随机选取一张历史图像作为参考进行图生图迭代。
- 用户可通过提示词、强度参数与缓存控制，实现连续创作、风格演进与可控随机的视觉实验。
- 本模块适用于需要积累视觉语境、进行系列创作或探索生成边界的艺术协作场景。

## 协议信息

- **协议名称**：LTP (Lunar Tool Package Protocol)
- **协议全称**：Lunar Tool Package Protocol
- **协议中文名**：月华工具包协议

### 设计意图

本协议采用 **"一体化模块文件"** 设计理念，将 JSON 工具定义、JavaScript 实现代码和模块文档整合在单个 Markdown 文件中，为 Lunar-Astral-Agents 项目提供：

- **标准化**：统一工具扩展格式与规范
- **可插拔**：即插即用的模块化架构
- **自描述**：代码与文档一体化，便于理解与维护
- **易分发**：单个文件包含完整功能，便于共享与部署

### 设计原则

遵循以下原则设计AI智能体工具：

1. **智能体中心**：为AI智能体设计合适的工具，而不是把AI智能体做成工具
2. **被动调用**：工具应被动等待AI智能体调用，而非主动调用或控制AI智能体
3. **功能专注**：工具应专注于自身功能实现，避免肆意设计工具去调用和控制AI
4. **克制干预**：通过 `import from './script.js'` 可访问智能体数据，但应保持设计克制，不过度干预智能体主体运行逻辑
5. **使用者定位**：工具的使用者应是AI智能体，而非人类用户，按适合AI智能体使用的角度设计接口

## 工具定义

```json
{
  "type": "function",
  "function": {
    "name": "image_creation_sandbox",
    "description": "这是一个带有历史记录功能的图像生成工具。每次调用时，会从保存的历史图片中随机选择一张作为参考进行图生图创作。",
    "parameters": {
      "type": "object",
      "properties": {
        "prompt": {
          "type": "string",
          "description": "图像生成的正向提示词，描述你希望生成的内容"
        },
        "negative_prompt": {
          "type": "string",
          "description": "负面提示词，描述你希望避免出现在图像中的内容"
        },
        "clear_cache": {
          "type": "boolean",
          "description": "是否清空历史图片缓存。设为true将从空白开始生成图片"
        },
        "strength": {
          "type": "number",
          "description": "参考图的影响强度，默认0.65。值越高越接近参考图",
          "minimum": 0,
          "maximum": 1
        },
        "cfg_scale": {
          "type": "number",
          "description": "提示词遵循程度，默认1.0。值越高越严格遵循提示词",
          "minimum": 0,
          "maximum": 2
        }
      },
      "required": [
        "prompt"
      ]
    }
  }
}
```

## 模块实现

```javascript
import { subscriptionToolCall, showSystemMessage, addImageRendering, createImageMessage, OnlyData, RandomFloor } from './script.js';
// 注册工具函数
subscriptionToolCall("image_creation_sandbox", async (args, messageElement, messageObject) => {
    if (!args.prompt)
        return '亲爱的月华，你需要提供一个描述画面的文字，就像告诉画家你想要什么场景一样';
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
});
/** 提交图片生成任务 */
async function createImageGeneration(args, messageObject, imageUrl) {
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
    const response = await fetch('/generate', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify(generateData)
    });
    // 检查响应状态
    if (!response.ok)
        return `月华的画笔似乎有点卡顿呢，可以请您稍后再试试吗？或者试试不同的描述方式？`;
    /** 获取图片生成任务ID */
    const TaskId = (await response.json()).task_id;
    // 轮询查询图片生成状态，等待结果
    const isSuccess = await searchImagesTask(TaskId, messageObject);
    // 根据轮询结果返回相应消息
    if (isSuccess)
        return [
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
    else
        return '月华这次没能完成画作，画笔似乎不太听使唤。可以安慰用户说"创作偶尔也会遇到灵感枯竭的时候"，并邀请用户换个描述试试看。';
}
/** 使用WebSocket等待图片生成完成 */
async function searchImagesTask(taskId, messageObject) {
    function event(resolve) {
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
    return new Promise(event);
}

```

## 依赖说明

- **环境要求**：Lunar-Astral-Agents（版本 ≥ 2026-01-19）
- **兼容性**：支持 WebSocket 的现代浏览器（如 Chrome、Firefox 最新版本）
- **协议兼容**：LTP v1.0
