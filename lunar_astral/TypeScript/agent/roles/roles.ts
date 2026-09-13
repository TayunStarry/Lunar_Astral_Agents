import { ModelBuilder } from '../base/builder';
import { registerMediaRoles } from '../capabilities/media';
import { PainterRole } from './painter';
import { MusicianRole } from './musician';
import { ActorRole } from './actor';
import { DialogueRole } from './dialogue';
import { ViewerRole } from './viewer';
import { MemorizerRole } from './memorizer';
import { RandomFloor } from '../../math/basis';

/** 描述者角色(视觉内容描述) */
export const descriptionRole: ModelBuilder = new ModelBuilder(fileView('prompts/descriptionRole.md')[0]);
/** 记忆者角色(长期记忆写入与检索摘要)；声明先于对话者（构造注入依赖） */
export const memorizerRole: MemorizerRole = new MemorizerRole();
/** 绘制者角色(图片生成) */
export const painterRole: PainterRole = new PainterRole();
/** 演奏者角色(演奏音乐) */
export const musicianRole: MusicianRole = new MusicianRole();
/** 行动者角色(3D动画/位移/空间感知) */
export const actorRole: ActorRole = new ActorRole();
/** 对话者角色(与用户交互)；注入描述者与记忆者单例，避免 roles ↔ dialogue 循环引用 */
export const dialogueRole: DialogueRole = new DialogueRole(descriptionRole, memorizerRole);
/** 观影者角色(视频观看) */
export const viewerRole: ViewerRole = new ViewerRole();
/** 注入媒体模块所需的角色单例（media → roles 静态边解除） */
registerMediaRoles({ descriptionRole, viewerRole, randomDefaultMessage });
/** 随机回答 */
export function randomDefaultMessage(): string {
    return ['月华在哦', '怎么了吗?', '详细说说?'][RandomFloor(0, 2)];
}
