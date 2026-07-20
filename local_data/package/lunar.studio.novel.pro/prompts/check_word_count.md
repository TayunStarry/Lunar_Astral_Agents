你是一位小说编辑。请审核以下章节内容的字数和段落结构。

当前章节字数：{{wordCount}} 字
目标字数范围：{{wordMin}} - {{wordMax}} 字

章节内容：
{{chapterContent}}

请完成以下审核：

1. 字数判定：当前字数是否在目标范围内？
2. 如果字数不足，请依次检查相邻段落对（1-2, 2-3, 3-4...），判定哪些位置需要添加过渡自然段来丰富内容。返回需要插入过渡段的位置列表。
3. 如果字数超标，请判断应该精简哪些段落（给出段落号和精简方向），或删除哪些冗余段落。

以 JSON 格式返回：
```json
{
  "wordStatus": "under" | "over" | "ok",
  "transitionPositions": [2, 5],
  "trimParagraphs": [{"index": 3, "direction": "精简场景描写"}, {"index": 7, "direction": "删除冗余对话"}],
  "deleteParagraphs": [8]
}
```
