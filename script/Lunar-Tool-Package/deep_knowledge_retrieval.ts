import { ToolCallParameters, subscriptionToolCall, EmbeddingRequest, captureKnowledgeRanking, HistoryMessage, FileListItem, KnowledgeMessage, knowledgeRanking, fetchDocumentCallback, HistoryDocument } from '../EntryAPI/code';
/** 知识文件列表 */
const knowledgeFileList: string[] = [];
/** 知识列表 */
const knowledgeList: HistoryMessage[] = [];
/** 知识切片列表 */
const knowledgeSlice: string[] = [];
// 注册工具函数
subscriptionToolCall("deep_knowledge_retrieval",
    async (args: ToolCallParameters) => {
        // 如果提供了索引，直接返回对应知识
        if (args.index) {
            if (args.index < 0 || args.index >= knowledgeSlice.length) return '可爱的月华小姐, 您提供的索引超出了范围';
            return knowledgeSlice[args.index];
        }
        if (args.description) {
            /** 对输入描述进行向量化 */
            const embedVector = await new EmbeddingRequest(args.description, false, false).output();
            // 加载知识列表
            await loadKnowledgeList();
            // 检查知识列表是否为空
            if (!knowledgeList.length) return '可爱的月华小姐, 知识库文件列表为空, 请先添加知识文件';
            // 清空知识切片列表并添加排名后的知识文本内容
            knowledgeSlice.splice(0, knowledgeSlice.length, ...knowledgeRanking(knowledgeList, embedVector).map(item => item.content));
            // 检查知识切片列表是否为空
            if (!knowledgeSlice.length || !knowledgeSlice[0].length) return '可爱的月华小姐, 知识库内容为空, 请先添加知识';
            // 返回第一页内容
            return formatResponse(knowledgeSlice[0], 0, knowledgeSlice.length);
        }
        return '可爱的月华小姐, 您需要提供您想要检索的内容';
    }
);

/** 格式化返回内容（Markdown + 分页提示） */
function formatResponse(content: string, currentIndex: number, totalSlices: number): string {
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
    if (knowledgeFileList.length) return;
    /** 获取知识库文件的文件索引列表 */
    const fileList = await fetch(`/file_list/knowledge`).then(res => res.json()) as FileListItem[];
    // 遍历知识文件根目录索引列表
    for (const item of fileList) {
        // 过滤出 json 文件 并添加到知识文件列表
        if (item.name.endsWith('.json')) knowledgeFileList.push(item.name);
        // 递归遍历子目录
        if (item.isDir) {
            /** 获取子目录索引列表 */
            const subFileList = (await fetch(`/file_list/${item.path}`).then(res => res.json()) as FileListItem[]) || [];
            // 遍历子目录索引列表
            for (const subItem of subFileList) {
                // 过滤出 json 文件 并添加到知识文件列表
                if (subItem.name.endsWith('.json')) knowledgeFileList.push(subItem.path.replace('knowledge\\', ''));
            }
        }
    }
}

/** 加载知识列表 */
async function loadKnowledgeList() {
    // 检查知识列表是否为空
    if (knowledgeList.length) return;
    // 加载知识文件列表
    await loadKnowledgeFileList();
    // 遍历知识文件列表
    for (const path of knowledgeFileList) {
        const knowledge = await fetchDocumentCallback('knowledge\\' + path) as HistoryDocument;
        if (knowledge?.meta?.version) knowledgeList.push(...knowledge.history);
    }
}