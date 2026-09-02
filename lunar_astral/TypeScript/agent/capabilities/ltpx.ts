import { GlobalConfig } from '../../index';
import { dialogueRole } from '../roles/roles';

/** 已注入的琉璃（远程）工具名集合，用于在线状态变化时精确增删 */
const injectedLTPXRemoteTools: Set<string> = new Set();

/** 追加给月华的系统提示词段落：强调「打开/使用/启动」某应用时必须调用聚合工具 */
const USE_THE_PROGRAM_PROMPT = '- 当被要求或需要(打开/启动/运行/使用)某个程序或操作时，必须调用工具"use_the_program"，不得凭空编造操作结果，必须等待工具真实返回。';

/** 琉璃（远程 LTPX）状态对象 */
type LTPXRemoteStatus = {
    /** 琉璃当前在线状态 */
    online: boolean;
    /** 琉璃当前 URL */
    url: string;
    /** 琉璃当前工具列表 */
    tools: Array<LTPXRemoteTool>;
};

/** 琉璃（远程 LTPX）工具对象 */
type LTPXRemoteTool = {
    /** 琉璃当前工具名 */
    name: string;
    /** 琉璃当前工具描述 */
    description?: string;
    /** 琉璃当前工具应用 ID */
    app_id?: string;
    /** 琉璃当前工具参数 */
    parameters?: any
};

/** 同步琉璃（远程 LTPX）状态：思考链起点检查在线状态并注入/移除工具链 */
export function syncLTPXRemoteStatus(): void {
    try {
        /** 琉璃当前状态 JSON 字符串 */
        const statusJSON = getLTPXRemoteStatus();
        // 无联络状态或琉璃离线：移除全部注入工具 + 动态提示词，并清空内部缓存
        if (!statusJSON || statusJSON === '{}' || !JSON.parse(statusJSON).online) {
            dialogueRole.removePrompt(USE_THE_PROGRAM_PROMPT);
            removeAllLTPXRemoteTools();
            clearLTPXRemoteTools();
            return;
        }
        /** 琉璃当前状态对象 */
        const status = JSON.parse(statusJSON) as LTPXRemoteStatus;
        /** 琉璃当前工具名集合 */
        const names = new Set<string>();
        /** 琉璃当前工具名→定义映射 */
        const known = new Map<string, { name: string; description?: string; parameters?: any }>();
        // 琉璃当前工具列表：提取工具名、描述、参数、应用 ID
        (status.tools || []).forEach(t => { if (t && t.name) { names.add(t.name); known.set(t.name, t); } });
        // 移除已不存在的琉璃工具（琉璃可能动态卸载插件）
        injectedLTPXRemoteTools.forEach(name => { if (!names.has(name)) removeLTPXRemoteTool(name); });
        // 注入新增的琉璃工具
        names.forEach(name => { if (!injectedLTPXRemoteTools.has(name)) injectLTPXRemoteTool(known.get(name)!); });
        // 加载动态提示词段落
        dialogueRole.appendPrompt(USE_THE_PROGRAM_PROMPT);
    }
    catch (e) {
        console.error('LTPX 远程（琉璃）工具状态同步失败:', e);
    }
}

/** 注入单个琉璃工具到 LTPdefinition 与 LTPfunction */
function injectLTPXRemoteTool(tool: { name: string; description?: string; parameters?: any }): void {
    const name = tool.name;
    // 工具函数：将参数序列化为 JSON 字符串转发到琉璃执行，返回 [文本结果, 图片]
    GlobalConfig.LTPfunction.set(name, async (args) => {
        const argsJSON = typeof args === 'string' ? args : JSON.stringify(args || {});
        // 异步转发：Go 侧在独立 goroutine 中请求琉璃，Promise 完成后返回结果，
        // 琉璃掉线/挂起时不会阻塞月华事件循环
        const text = await callLTPXRemoteTool(name, argsJSON);
        // 琉璃掉线/无响应（Go 侧已清空联络 URL）：立即移除该工具，
        // 避免模型继续调用已离线的琉璃工具，琉璃重启注册后由 sync 重新注入
        if (typeof text === 'string' && text.startsWith('【琉璃工具调用失败】'))
            removeLTPXRemoteTool(name);
        return [text, ''];
    });
    // 工具定义（parameters 为 any，由琉璃下发，直接透传）
    GlobalConfig.LTPdefinition.push({
        type: 'function',
        function: { name, description: tool.description || '', parameters: tool.parameters },
    });
    injectedLTPXRemoteTools.add(name);
    console.log(`LTPX 已注入琉璃远程工具: ${name}`);
}

/** 移除单个琉璃远程工具 */
function removeLTPXRemoteTool(name: string): void {
    // 从 LTPdefinition 移除同名定义
    for (let i = GlobalConfig.LTPdefinition.length - 1; i >= 0; i--) {
        const def = GlobalConfig.LTPdefinition[i];
        if (def.type === 'function' && def.function?.name === name) GlobalConfig.LTPdefinition.splice(i, 1);
    }
    // 从 LTPfunction 移除函数
    GlobalConfig.LTPfunction.delete(name);
    injectedLTPXRemoteTools.delete(name);
    console.log(`LTPX 已移除琉璃远程工具: ${name}`);
}

/** 移除全部琉璃远程工具（离线时） */
function removeAllLTPXRemoteTools(): void {
    [...injectedLTPXRemoteTools].forEach(removeLTPXRemoteTool);
}
