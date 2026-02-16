# LTP 工具模块

## 元数据

- **名称**：query_intel
- **版本**：1.0.0
- **作者**：[钛宇-星光阁](https://gitee.com/TayunStarry)
- **更新日志**：
  - 2026-01-24：创建初始版本

## 模块描述

- 本模块是一个实时信息查询工具，提供精准的天气与新闻数据获取能力。
- 调用者可通过指定省份与城市查询当前天气状况，或直接获取由权威信源整理的当日新闻图文摘要。
- 系统内置多数据源容错机制，确保查询结果的稳定性与时效性。
- 适用于需要融合实时气象信息、社会动态或突发新闻的智能决策与交互场景。

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
    "name": "query_intel",
    "description": "查询天气信息或搜索最新新闻",
    "parameters": {
      "type": "object",
      "properties": {
        "query_type": {
          "type": "string",
          "enum": [
            "weather",
            "news"
          ],
          "description": "指定查询类型：'weather' 表示查询天气，'news' 表示搜索新闻"
        },
        "sheng": {
          "type": "string",
          "description": "省份名称，如“江苏省”、“北京市”。仅在 query_type 为 'weather' 时必填"
        },
        "place": {
          "type": "string",
          "description": "城市名称，如“南京市”、“上海市”。仅在 query_type 为 'weather' 时必填"
        }
      },
      "required": [
        "query_type"
      ]
    }
  }
}
```

## 模块实现

```javascript
import { subscriptionToolCall, addImageRendering, createImageMessage } from './script.js';
// 注册工具函数
subscriptionToolCall("query_intel", async (args, _, messageObject) => {
    // 根据查询类型执行不同的操作
    switch (args.query_type) {
        case "weather": return await getWeather(args.sheng, args.place);
        case "news": return await getNews(messageObject);
        default: return `不支持的查询类型: ${args.query_type}`;
    }
});
/** 获取天气信息 */
async function getWeather(sheng, place) {
    // 验证省和市参数是否存在
    if (!sheng || !place)
        return '天气查询需要提供省和市';
    /** 随机选择一个URL */
    const selectedUrl = `https://cn.apihz.cn/api/tianqi/tqyb.php?id=88888888&key=88888888&sheng=${sheng}&place=${place}`;
    /** 发送GET请求 */
    const response = await fetch(selectedUrl);
    // 检查响应状态
    if (!response.ok)
        return `天气查询API返回错误状态: ${response.status}`;
    // 返回JSON响应
    return await response.text();
}
/** 获取新闻信息并显示图片 */
async function getNews(messageObject) {
    /** 新闻查询API地址 */
    const url = "https://60s.7se.cn/v2/60s";
    /** 发送GET请求 */
    const response = await fetch(url);
    // 检查响应状态
    if (!response.ok)
        return `新闻查询API返回错误状态: ${response.status}`;
    /** 解析响应为JSON格式 */
    const decode = await response.json();
    /** 创建图片消息对象 */
    const imageMessage = createImageMessage('assistant', '包含新闻内容的图片', decode.data.image);
    // 添加图片渲染到消息元素
    addImageRendering(imageMessage);
    // 存储图片URL到消息对象, 用于后续引用
    messageObject.imageUrl = decode.data.image;
    // 返回新闻内容
    return decode.data.news.join('\n');
}

```

## 依赖说明

- **环境要求**：Lunar-Astral-Agents（版本 ≥ 2026-01-19）
- **兼容性**：支持 WebSocket 的现代浏览器（如 Chrome、Firefox 最新版本）
- **协议兼容**：LTP v1.0
