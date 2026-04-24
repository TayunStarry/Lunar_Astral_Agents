import { ToolCallParameters, subscriptionToolCall, exportChatInteractionWithFetch, OnlyData, chatHistoryPanel } from '../EntryAPI/code';

// 注册工具函数
subscriptionToolCall("end_dialogue",
    async (args: ToolCallParameters) => {
        // 导出聊天交互数据
        exportChatInteractionWithFetch(args.reason);
        // 5 秒后清空会话历史和聊天历史面板
        setTimeout(() => { OnlyData.historyMessage = []; chatHistoryPanel.innerHTML = ''; }, 5000);
        // 现在返回模拟数据
        return '对话即将结束，请用户说明原因并告别';
    }
);