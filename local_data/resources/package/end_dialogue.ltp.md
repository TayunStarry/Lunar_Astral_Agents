# LTP 工具模块

## 元数据

- **名称**：end_dialogue
- **版本**：1.0.0
- **作者**：[钛宇-星光阁](https://gitee.com/TayunStarry)
- **更新日志**：
  - 2026-01-24：创建初始版本

## 模块描述

- 本模块是一个对话流程安全管控工具，赋予智能体在特定情境下主动终止会话的能力。
- 当对话触及不当内容、用户意图已完成或智能体自身状态不适时，可调用此工具并说明原因，系统将自动导出当前对话记录并清空会话上下文，实现可控、得体且留有记录的交互退出。
- 适用于人机对话中的边界守护、流程收尾与体验安全保障场景。

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
    "name": "end_dialogue",
    "description": "当你感到冒犯或不希望继续对话时，调用此函数结束当前对话",
    "parameters": {
      "type": "object",
      "properties": {
        "reason": {
          "type": "string",
          "description": "简要说明结束对话的原因或总结本次对话"
        }
      },
      "required": [
        "reason"
      ]
    }
  }
}
```

## 模块实现

```javascript
import { subscriptionToolCall, exportChatInteractionWithFetch, OnlyData, chatHistoryPanel } from './script.js';
// 注册工具函数
subscriptionToolCall("end_dialogue", async (args) => {
    // 导出聊天交互数据
    exportChatInteractionWithFetch(args.reason);
    // 5 秒后清空会话历史和聊天历史面板
    setTimeout(() => { OnlyData.historyMessage = []; chatHistoryPanel.innerHTML = ''; }, 5000);
    // 现在返回模拟数据
    return '对话即将结束，请用户说明原因并告别';
});

```

## 依赖说明

- **环境要求**：Lunar-Astral-Agents（版本 ≥ 2026-01-19）
- **兼容性**：支持 WebSocket 的现代浏览器（如 Chrome、Firefox 最新版本）
- **协议兼容**：LTP v1.0
