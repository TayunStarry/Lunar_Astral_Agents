# LTP 工具模块

## 元数据

- **名称**：diffusion_generation
- **版本**：1.0.0
- **作者**：[钛宇-星光阁](https://gitee.com/TayunStarry)
- **更新日志**：
  - 2026-01-24：创建初始版本

## 模块描述

- 本模块是一个基于扩散模型的可控图像生成引擎，提供文生图与图生图双模式创作支持。
- 用户可通过正面提示词描述预期画面，并利用负面提示词排除不希望出现的元素；系统还支持参考图强度调节、提示词权重配置等细粒度控制参数。
- 生成任务提交后，工具将自动轮询处理状态并在完成后进行视觉化呈现。
- 本模块还提供了API接口，方便开发者集成到自己的应用中。
- 适用于艺术创作、视觉构思、风格迁移等需要高质量图像生成的创意场景。

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
    "name": "diffusion_generation",
    "description": "根据文本描述生成图像。如需进行图像创作，请调用此函数。",
    "parameters": {
      "type": "object",
      "properties": {
        "prompt": {
          "type": "string",
          "description": "图像生成的正向描述文本"
        },
        "negative_prompt": {
          "type": "string",
          "description": "负面提示文本，用于排除图像中不希望出现的元素"
        },
        "use_reference": {
          "type": "boolean",
          "description": "是否使用上一次生成的图像作为参考，默认值为 false"
        },
        "strength": {
          "type": "number",
          "description": "参考图像的影响强度，取值范围为 0 到 1，默认值为 0.65"
        },
        "cfg_scale": {
          "type": "number",
          "description": "提示词权重调节参数，取值范围为 0 到 2，默认值为 1.0"
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
import { subscriptionToolCall, showSystemMessage, addImageRendering, createImageMessage } from './script.js';
// 注册工具函数
subscriptionToolCall("diffusion_generation", async (args, messageElement, messageObject) => {
    if (!args.prompt)
        return '生成图片需要提供正向提示文本';
    // 显示系统消息, 提示用户图片生成任务已提交
    messageElement.innerHTML = '<em><strong>月华正在努力绘制中...请稍等片刻</strong></em>';
    // 直接创建图片生成任务并等待结果
    const result = await createImageGeneration(args, messageObject);
    // 返回JSON响应
    return result;
});
/** 提交图片生成任务 */
async function createImageGeneration(args, messageObject) {
    /** 获取生成的图片列表 */
    const fileList = await fetch(`/file/list/generated`).then(res => res.json());
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
    const response = await fetch('/generate', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify(generateData)
    });
    // 检查响应状态
    if (!response.ok)
        return `尝试画图失败了, 失败原因是: ${response.statusText} 请向用户解释一下`;
    /** 获取图片生成任务ID */
    const TaskId = (await response.json()).task_id;
    // 轮询查询图片生成状态，等待结果
    const isSuccess = await searchImagesTask(TaskId, messageObject);
    // 根据轮询结果返回相应消息
    if (isSuccess)
        return `图片绘制完成！这是你的正面提示词: [ ${args.prompt} ] 负面提示词: [ ${args.negative_prompt} ] 请你简要描述一下画面内容，让用户更好地理解这幅画`;
    // 若生成失败, 则告知用户图片生成任务失败
    else
        return '图片生成失败，请向用户说明情况（例如：画笔暂时无法使用）';
}
/** 轮询查询图片生成状态 */
async function searchImagesTask(taskId, messageObject) {
    /**
     * 轮询查询图片生成状态
     *
     * @param {function} resolve 轮询成功回调函数
     *
     * @returns {Promise<void>} 图片生成状态
     */
    async function poll(resolve) {
        /** 查询图片生成状态 */
        const statusInquiry = await fetch(`/generate/status?task_id=${taskId}`).then(res => res.json());
        // 检查任务状态
        if (!statusInquiry) {
            showSystemMessage(`图片绘制状态查询失败`, 'error');
            resolve(false);
            return;
        }
        // 判断任务状态
        switch (statusInquiry.status) {
            case 'completed':
                /** 获取生成的图片列表 */
                const fileList = await fetch(`/file/list/generated`).then(res => res.json());
                /** 排序文件列表, 取最新生成的图片 */
                const imageUrl = '/file/read/' + fileList.sort((a, b) => new Date(b.lastModified).getTime() - new Date(a.lastModified).getTime())[0].path;
                /** 创建一个新的音频元素用于播放提示音 */
                const audio = new Audio('/file/read/resources/audios/prompt-tone.mp3');
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
                resolve(true);
                break;
            case 'failed':
                showSystemMessage(`图片绘制失败`, 'error');
                resolve(false);
                break;
            case 'running':
                // 继续轮询
                setTimeout(() => poll(resolve), 1000);
                break;
            default:
                // 继续轮询
                setTimeout(() => poll(resolve), 2000);
                break;
        }
    }
    // 使用Promise封装轮询过程
    return new Promise(resolve => poll(resolve));
}

```

## 依赖说明

- **环境要求**：Lunar-Astral-Agents（版本 ≥ 2026-01-19）
- **兼容性**：支持 WebSocket 的现代浏览器（如 Chrome、Firefox 最新版本）
- **协议兼容**：LTP v1.0
