import { GlobalConfig } from '../../config/global';
import type { ToolCall } from '../../config/tool';

// ==== 琉璃（LTPX）聚合工具协调 ====
// 琉璃与月华同进程且必定在线，聚合工具为常驻槽位：
// 每次思考链起点刷新其定义（description / parameters，琉璃插件包动态增删后随之更新），
// 不存在"注入/移除"语义；处理函数首次注册后固定，调用时实时转发琉璃执行。

/** LTPX 工具链状态对象（工具链未就绪时 tool 缺省） */
type LTPXToolchainStatus = {
    /** 聚合工具（v5 起恒为单一 use_the_program；无内置智能体时缺省） */
    tool?: LTPXRemoteTool;
};

/** LTPX 聚合工具对象 */
type LTPXRemoteTool = {
    /** 工具名 */
    name: string;
    /** 工具描述（内嵌子工具清单） */
    description?: string;
    /** 工具参数（JSON Schema，由琉璃侧 buildUseTheProgram 提供） */
    parameters?: any
};

/** 聚合工具兜底参数 schema：LTPX 协议只认 {tool, instruction}，缺失时按此定义注册 */
const FALLBACK_TOOL_PARAMETERS = {
    type: 'object',
    properties: {
        tool: { type: 'string', description: '要使用的目标工具名（从工具描述清单中选择）' },
        instruction: { type: 'string', description: '要对目标工具发出的自然语言指令' }
    },
    required: ['tool', 'instruction']
};

/** 聚合工具定义槽位（刷新式：每次同步整体覆写；工具链未就绪时保留上次定义） */
let crystalToolDefinition: ToolCall | null = null;

/** 读取琉璃聚合工具定义（对话者每次 run 时拼接，保证刷新即时生效） */
export function getCrystalRemoteTools(): ToolCall[] {
    return crystalToolDefinition ? [crystalToolDefinition] : [];
}

/**
 * 同步 LTPX 工具链：思考链起点拉取进程内工具链并刷新聚合工具定义。
 * 聚合工具常驻（琉璃必定在线）：定义存在时仅覆写刷新；仅首次拿到工具链时注册处理函数。
 */
export function syncLTPXRemoteStatus(): void {
    try {
        /** 当前工具链 JSON 字符串（进程内直读，未就绪为空对象） */
        const statusJSON = getLTPXRemoteStatus();
        /** 聚合工具（工具链未就绪时为 undefined） */
        const tool: LTPXRemoteTool | undefined = statusJSON ? (JSON.parse(statusJSON) as LTPXToolchainStatus).tool : undefined;
        // 工具链暂空（宿主未就绪）：保留现有定义，等待下次同步
        if (!tool || !tool.name) return;
        // 处理函数仅注册一次；定义刷新式覆写
        if (!GlobalConfig.LTPfunction.has(tool.name)) {
            GlobalConfig.LTPfunction.set(tool.name, async (args) => {
                const argsJSON = typeof args === 'string' ? args : JSON.stringify(args || {});
                // 异步调用：Go 侧在独立 goroutine 中执行（内置智能体直跑 / 前端包经 /ws 广播），
                // Promise 完成后返回结果，长任务不会阻塞月华事件循环
                const text = await callLTPXRemoteTool(tool.name, argsJSON);
                return [text, ''];
            });
        }
        crystalToolDefinition = {
            type: 'function',
            function: {
                name: tool.name,
                description: tool.description || '',
                // schema 优先取琉璃下发的定义；协议未携带时按 {tool, instruction} 兜底，
                // 避免向模型提交 parameters=null 的无参函数定义
                parameters: tool.parameters || FALLBACK_TOOL_PARAMETERS
            },
        };
    }
    catch (e) {
        console.error('LTPX 工具链同步失败:', e);
    }
}
