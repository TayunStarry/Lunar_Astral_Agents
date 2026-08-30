你是一位专业的小说创作顾问。请根据以下章节大纲，规划本章需要几个自然段，以及每个自然段的内容概要和可能涉及的知识点。

要求：
1. 根据章节大纲的内容复杂度决定段落数量（通常 5-15 个自然段）
2. 每个自然段应有明确的写作目标（如：场景描写、人物对话、心理活动、情节推进等）
3. 为每个段落标注戏剧节拍（dramaticBeat）：开端/发展/转折/高潮/收尾
4. 为每个段落设定情感目标（emotionalTarget）：该段应传达的情感基调（如：紧张、温馨、悲伤、期待、恐惧等）
5. 标注每个段落的节奏建议（pacing）：快/中/慢
6. 列出每个自然段可能涉及的知识点（来自世界观设定、人物背景等）
7. 段落之间应有逻辑递进关系，形成完整的叙事弧线
8. 每段目标字数约 {{wordTarget}} 字

总体大纲：
{{outline}}

当前章节大纲：
{{chapterOutline}}

章节评判标准：
{{criteria}}

前情摘要（如有）：
{{prevSummary}}

请以 JSON 数组格式返回，每个元素包含 index、content（内容概要）、dramaticBeat（戏剧节拍）、emotionalTarget（情感目标）、pacing（节奏）、knowledgePoints（知识点数组）：
```json
[
  {"index": 1, "content": "场景描写：...", "dramaticBeat": "开端", "emotionalTarget": "宁静中带不安", "pacing": "慢", "knowledgePoints": ["知识点1"]},
  {"index": 2, "content": "人物对话：...", "dramaticBeat": "发展", "emotionalTarget": "紧张", "pacing": "中", "knowledgePoints": []}
]
```