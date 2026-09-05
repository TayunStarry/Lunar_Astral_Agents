import { ModelBuilder, SearcherRole, PainterRole, MusicianRole, ActorRole, DialogueRole, ViewerRole, MemorizerRole, RandomFloor } from '../../index';

/** 描述者角色(视觉内容描述) */
export const descriptionRole: ModelBuilder = new ModelBuilder(fileView('prompts/descriptionRole.md')[0]);
/** 搜索者角色(深度调研与信息查证) */
export const searcherRole: SearcherRole = new SearcherRole();
/** 绘制者角色(图片生成) */
export const painterRole: PainterRole = new PainterRole();
/** 演奏者角色(演奏音乐) */
export const musicianRole: MusicianRole = new MusicianRole();
/** 行动者角色(3D动画/位移/空间感知) */
export const actorRole: ActorRole = new ActorRole();
/** 对话者角色(与用户交互) */
export const dialogueRole: DialogueRole = new DialogueRole();
/** 观影者角色(视频观看) */
export const viewerRole: ViewerRole = new ViewerRole();
/** 记忆者角色(长期记忆写入与检索摘要) */
export const memorizerRole: MemorizerRole = new MemorizerRole();
/** 随机回答 */
export function randomDefaultMessage(): string {
    return ['月华在哦', '怎么了吗?', '详细说说?'][RandomFloor(0, 2)];
}
