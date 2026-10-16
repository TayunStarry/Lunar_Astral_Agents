import { ModelBuilder } from '../base/builder';
import { registerMediaRoles } from '../capabilities/media';
import { registerAgentTargets } from '../../tool/agent-control';
import { registerImageDescribeRole } from '../../tool/image-recall';
import { PainterRole } from './painter';
import { MusicianRole } from './musician';
import { DialogueRole } from './dialogue';
import { PerceiverRole } from './perceiver';
import { MemorizerRole } from './memorizer';
import { RandomFloor } from '../../tool/math';

/**
 * 追加给月华的系统提示词段落：强调调用聚合工具时传目标工具与自然语言指令。
 * 工具名与琉璃侧常量保持一致（crystal/ltpx_remote.go integratedToolsName = "use_the_program"），
 * 避免提示词里的工具名与真实注册名不符导致「未找到工具包」。
 */
const USE_THE_PROGRAM_PROMPT = '- 当被要求或需要(打开/启动/运行/使用)某个程序或操作时，必须调用工具"use_the_program"（tool 参数填目标工具名、instruction 参数填自然语言指令），不得凭空编造操作结果，必须等待工具真实返回。';

/** 描述者角色(视觉内容描述) */
export const descriptionRole: ModelBuilder = new ModelBuilder(fileView('prompts/descriptionRole.md')[0]);
/** 记忆者角色(长期记忆写入与检索摘要)；声明先于对话者（构造注入依赖） */
export const memorizerRole: MemorizerRole = new MemorizerRole();
/** 绘制者角色(图片生成) */
export const painterRole: PainterRole = new PainterRole();
/** 演奏者角色(演奏音乐) */
export const musicianRole: MusicianRole = new MusicianRole();
/** 对话者角色(与用户交互) */
export const dialogueRole: DialogueRole = new DialogueRole(descriptionRole);
/** 感知者角色(视频/动态图理解) */
export const perceiverRole: PerceiverRole = new PerceiverRole();
/** 注入媒体模块所需的角色单例（media → roles 静态边解除） */
registerMediaRoles({ perceiverRole, randomDefaultMessage });
/** 注入子智能体实例与描述角色（tool 模块处理函数所需，避免 tool → roles 静态循环引用） */
registerAgentTargets({ painterRole, musicianRole });
registerImageDescribeRole(descriptionRole);
/** 追加 LTPX 聚合工具的使用要求（静态段落，工具名与琉璃侧一致） */
dialogueRole.appendPrompt(USE_THE_PROGRAM_PROMPT);
/** 随机回答 */
export function randomDefaultMessage(): string {
    return ['月华在哦', '怎么了吗?', '详细说说?'][RandomFloor(0, 2)];
}
