import { registerAgentTargets } from '../tools/agent-control';
import { registerDumpRoles } from '../agent/data-dump';
import { registerRecoveryRoles, snapshotAgentContext, restoreAgentContext } from '../agent/recovery';
import { PainterRole } from './painter';
import { MusicianRole } from './musician';
import { ScreenshoterRole } from './screenshoter';
import { DialogueRole } from './dialogue';
import { MemorizerRole } from './memorizer';
import { RandomFloor } from '../tools/math';

/** 记忆者角色(长期记忆写入与检索摘要)；声明先于对话者（构造注入依赖） */
export const memorizerRole: MemorizerRole = new MemorizerRole();
/** 绘制者角色(图片生成) */
export const painterRole: PainterRole = new PainterRole();
/** 演奏者角色(演奏音乐) */
export const musicianRole: MusicianRole = new MusicianRole();
/** 截图者角色(自然语言驱动精确截图；承接对话者原有截图工具) */
export const screenshoterRole: ScreenshoterRole = new ScreenshoterRole();
/**
 * 姐妹基础外观：直接复用绘制者已解析的角色提示词
 *
 * 绘制者构造时已读取并解析 prompts/painterCharacters.md，
 * 这里取「基础外观」节填入对话者系统提示词，避免重复解析同一份 Markdown。
 */
const builtinAppearances = {
    lunar: painterRole.prompts.builtinCharacters.get('月华')?.appearance || '',
    crystal: painterRole.prompts.builtinCharacters.get('琉璃')?.appearance || '',
};
/** 对话者角色(与用户交互) */
export const dialogueRole: DialogueRole = new DialogueRole(builtinAppearances);
/** 注入子智能体实例（tool 模块处理函数所需，避免 tool → roles 静态循环引用） */
registerAgentTargets({ painterRole, musicianRole, screenshoterRole });
/** 注入可导出角色注册表（TUI 数据导出按名单导出单个子智能体上下文） */
registerDumpRoles({
    dialogue: { role: dialogueRole, label: '对话者' },
    painter: { role: painterRole, label: '绘制者' },
    musician: { role: musicianRole, label: '演奏者' },
    screenshoter: { role: screenshoterRole, label: '截图者' },
});
/** 向引擎重载的上下文抢救模块登记"月华主上下文"归属（重载后据此恢复对话历史） */
registerRecoveryRoles({ dialogue: dialogueRole });

/**
 * 把角色实例集合挂到宿主可见的全局（UMD 工厂的 root）。
 *
 * 用途：宿主（Go 侧检修工具 / 离线验证脚本）需要按名取用角色实例读取上下文；
 * 此前 UMD 工厂只返回 api 而从不写 root，宿主侧没有任何内省入口。
 * 挂载本身无副作用（仅一次属性赋值），不影响智能体运行。
 *
 * 其中 snapshotContext / restoreContext 是**引擎重载**的上下文抢救入口：
 * 宿主在关闭旧 goja 运行时之前取快照、启动新运行时之后回灌。
 */
export const roleInstances = {
    dialogue: dialogueRole,
    painter: painterRole,
    musician: musicianRole,
    screenshoter: screenshoterRole,
};
try {
    const host = (globalThis as unknown as { agentSystem?: Record<string, unknown> });
    host.agentSystem = Object.assign(host.agentSystem || {}, roleInstances, {
        randomDefaultMessage,
        snapshotContext: snapshotAgentContext,
        restoreContext: restoreAgentContext,
    });
}
catch (e) {
    console.warn('[角色] 宿主全局挂载失败（不影响运行）:', e);
}

/** 随机回答 */
export function randomDefaultMessage(): string {
    return ['月华在哦', '怎么了吗?', '详细说说?'][RandomFloor(0, 2)];
}
