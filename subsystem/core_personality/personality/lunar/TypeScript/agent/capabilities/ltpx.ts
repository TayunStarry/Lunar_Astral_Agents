import { GlobalConfig } from '../../config/global';
import { dialogueRole } from '../roles/roles';

/** 已注入的 LTPX 工具名集合，用于工具链变化时精确增删 */
const injectedLTPXRemoteTools: Set<string> = new Set();

/** 追加给月华的系统提示词段落：强调「打开/使用/启动」某应用时必须调用聚合工具 */
const USE_THE_PROGRAM_PROMPT = '- 当被要求或需要(打开/启动/运行/使用)某个程序或操作时，必须调用工具"use_the_program"，不得凭空编造操作结果，必须等待工具真实返回。';

/** LTPX 工具链状态对象（工具链为空时 tool 缺省） */
type LTPXToolchainStatus = {
    /** 聚合工具（v5 起恒为单一 use_program；无 AtoA 能力包时缺省） */
    tool?: LTPXRemoteTool;
};

/** LTPX 聚合工具对象 */
type LTPXRemoteTool = {
    /** 工具名 */
    name: string;
    /** 工具描述 */
    description?: string;
    /** 工具参数 */
    parameters?: any
};

/**
 * 同步 LTPX 工具链：思考链起点拉取进程内工具链并注入/更新工具与动态提示词。
 * 工具提供方与消费方同处一个进程，故不再有在线/离线判定：
 * 仅当工具链为空（无 AtoA 能力包）时移除工具与提示词。
 */
export function syncLTPXRemoteStatus(): void {
    try {
        /** 当前工具链 JSON 字符串（进程内直读，空工具链为 '{}'） */
        const statusJSON = getLTPXRemoteStatus();
        /** 聚合工具（工具链为空时为 undefined） */
        const tool: LTPXRemoteTool | undefined = statusJSON ? (JSON.parse(statusJSON) as LTPXToolchainStatus).tool : undefined;
        // 工具链为空：移除注入工具 + 动态提示词，并清空内部缓存
        if (!tool || !tool.name) {
            dialogueRole.removePrompt(USE_THE_PROGRAM_PROMPT);
            removeAllLTPXRemoteTools();
            clearLTPXRemoteTools();
            return;
        }
        /** 当前已注入的聚合工具名（至多一个） */
        const injected = [...injectedLTPXRemoteTools][0];
        // 聚合工具变化（包动态增删 LTPX 插件后随之更新）：先移除旧工具再注入新工具
        if (injected && injected !== tool.name) removeLTPXRemoteTool(injected);
        if (!injectedLTPXRemoteTools.has(tool.name)) injectLTPXRemoteTool(tool);
        // 加载动态提示词段落
        dialogueRole.appendPrompt(USE_THE_PROGRAM_PROMPT);
    }
    catch (e) {
        console.error('LTPX 工具链同步失败:', e);
    }
}

/** 注入单个 LTPX 工具到 LTPdefinition 与 LTPfunction */
function injectLTPXRemoteTool(tool: { name: string; description?: string; parameters?: any }): void {
    const name = tool.name;
    // 工具函数：将参数序列化为 JSON 字符串转发到工具提供方执行，返回 [文本结果, 图片]
    GlobalConfig.LTPfunction.set(name, async (args) => {
        const argsJSON = typeof args === 'string' ? args : JSON.stringify(args || {});
        // 异步调用：Go 侧在独立 goroutine 中执行（内置智能体直跑 / 前端包经 /ws 广播），
        // Promise 完成后返回结果，长任务不会阻塞月华事件循环
        const text = await callLTPXRemoteTool(name, argsJSON);
        return [text, ''];
    });
    // 工具定义（parameters 为 any，由工具链下发，直接透传）
    GlobalConfig.LTPdefinition.push({
        type: 'function',
        function: { name, description: tool.description || '', parameters: tool.parameters },
    });
    injectedLTPXRemoteTools.add(name);
}

/** 移除单个 LTPX 工具 */
function removeLTPXRemoteTool(name: string): void {
    // 从 LTPdefinition 移除同名定义
    for (let i = GlobalConfig.LTPdefinition.length - 1; i >= 0; i--) {
        const def = GlobalConfig.LTPdefinition[i];
        if (def.type === 'function' && def.function?.name === name) GlobalConfig.LTPdefinition.splice(i, 1);
    }
    // 从 LTPfunction 移除函数
    GlobalConfig.LTPfunction.delete(name);
    injectedLTPXRemoteTools.delete(name);
    console.log(`LTPX 已移除工具: ${name}`);
}

/** 移除全部 LTPX 工具（工具链为空时） */
function removeAllLTPXRemoteTools(): void {
    [...injectedLTPXRemoteTools].forEach(removeLTPXRemoteTool);
}
