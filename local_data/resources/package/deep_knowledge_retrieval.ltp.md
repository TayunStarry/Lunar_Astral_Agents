# LTP 工具模块

## 元数据

- **名称**：deep_knowledge_retrieval
- **版本**：1.0.0
- **作者**：[钛宇-星光阁](https://gitee.com/TayunStarry)
- **更新日志**：
  - 2026-04-19：创建初始版本

## 模块描述

- 本模块是一个面向历史语料的深度检索系统，支持基于语义向量匹配的知识召回。
- 用户通过自然语言描述发起查询后，系统将在全部历史记录中进行相关性排序，并以分页形式返回结果，便于逐段调阅。
- 该工具具备查询状态保持能力，可在同一会话中持续进行多轮翻阅与精筛。
- 适用于需要追溯对话历史、引用过往知识或进行多轮信息聚合的认知增强场景。

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
    "name": "deep_knowledge_retrieval",
    "description": "深度检索全部历史记录中的相关知识。首次查询需提供描述，系统将返回分页结果，默认返回第一页。如需获取后续页面，请在后续调用中提供索引（从0开始）。若需新建查询，请将索引设为0。",
    "parameters": {
      "type": "object",
      "properties": {
        "description": {
          "type": "string",
          "description": "检索相关知识的描述，用于首次查询"
        },
        "index": {
          "type": "integer",
          "description": "页码索引（从0开始）。若不提供则默认为0（第一页）。若要新建查询，请设置为0。"
        }
      }
    }
  }
}
```

## 模块实现

```javascript
import { subscriptionToolCall, EmbeddingRequest, knowledgeRanking, fetchDocumentCallback } from './script.js';
/** 知识文件列表 */
const knowledgeFileList = [];
/** 知识列表 */
const knowledgeList = [];
/** 知识切片列表 */
const knowledgeSlice = [];
// 注册工具函数
subscriptionToolCall("deep_knowledge_retrieval", async (args) => {
    // 如果提供了索引，直接返回对应知识
    if (args.index) {
        if (args.index < 0 || args.index >= knowledgeSlice.length)
            return '可爱的月华小姐, 您提供的索引超出了范围';
        return knowledgeSlice[args.index];
    }
    if (args.description) {
        /** 对输入描述进行向量化 */
        const embedVector = await new EmbeddingRequest(args.description, false, false).output();
        // 加载知识列表
        await loadKnowledgeList();
        // 检查知识列表是否为空
        if (!knowledgeList.length)
            return '可爱的月华小姐, 知识库文件列表为空, 请先添加知识文件';
        // 清空知识切片列表并添加排名后的知识文本内容
        knowledgeSlice.splice(0, knowledgeSlice.length, ...knowledgeRanking(knowledgeList, embedVector).map(item => item.content));
        // 检查知识切片列表是否为空
        if (!knowledgeSlice.length || !knowledgeSlice[0].length)
            return '可爱的月华小姐, 知识库内容为空, 请先添加知识';
        // 返回第一页内容
        return formatResponse(knowledgeSlice[0], 0, knowledgeSlice.length);
    }
    return '可爱的月华小姐, 您需要提供您想要检索的内容';
});
/** 格式化返回内容（Markdown + 分页提示） */
function formatResponse(content, currentIndex, totalSlices) {
    const prompts = [
        `### 📄 查询结果（第 ${currentIndex + 1} / ${totalSlices} 页）`,
        '',
        content,
        '',
        '---',
        '### 🔍 分页提示',
        `- 如需查看其他页面，请提供索引值（从 0 到 ${totalSlices - 1}）。`,
        '- 若要新建查询，请将索引设为 0 并提供新的描述。',
        '- 月华可以通过连续调用本工具来浏览全部相关结果。',
        ''
    ];
    return prompts.join('\n');
}
/** 加载知识文件列表 */
async function loadKnowledgeFileList() {
    // 检查知识文件列表是否为空
    if (knowledgeFileList.length)
        return;
    /** 获取知识库文件的文件索引列表 */
    const fileList = await fetch(`/file_list/knowledge`).then(res => res.json());
    // 遍历知识文件根目录索引列表
    for (const item of fileList) {
        // 过滤出 json 文件 并添加到知识文件列表
        if (item.name.endsWith('.json'))
            knowledgeFileList.push(item.name);
        // 递归遍历子目录
        if (item.isDir) {
            /** 获取子目录索引列表 */
            const subFileList = await fetch(`/file_list/${item.path}`).then(res => res.json()) || [];
            // 遍历子目录索引列表
            for (const subItem of subFileList) {
                // 过滤出 json 文件 并添加到知识文件列表
                if (subItem.name.endsWith('.json'))
                    knowledgeFileList.push(subItem.path.replace('knowledge\\', ''));
            }
        }
    }
}
/** 加载知识列表 */
async function loadKnowledgeList() {
    // 检查知识列表是否为空
    if (knowledgeList.length)
        return;
    // 加载知识文件列表
    await loadKnowledgeFileList();
    // 遍历知识文件列表
    for (const path of knowledgeFileList) {
        const knowledge = await fetchDocumentCallback('knowledge\\' + path);
        if (knowledge?.meta?.version)
            knowledgeList.push(...knowledge.history);
    }
}

```

## 依赖说明

- **环境要求**：Lunar-Astral-Agents（版本 ≥ 2026-01-19）
- **兼容性**：支持 WebSocket 的现代浏览器（如 Chrome、Firefox 最新版本）
- **协议兼容**：LTP v1.0
