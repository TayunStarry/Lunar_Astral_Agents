/**
 * 绘制任务的第一人称消歧
 *
 * 对话者（月华）派发任务时常用第一人称书写（「画我的自画像」），而绘制者是独立
 * 子智能体，只拿到任务文本，无法知道「我」是谁。历史缺陷：模型因此把「我的自画像」
 * 画成了「通用-女」的陌生女孩。
 *
 * 这里在任务消息前补一条固定的上下文说明，和 painterRole.md 的「代词解析」规则
 * 形成双保险：只有任务里出现第一人称/自指表达时才追加，不影响纯风景/物品类任务。
 */

/** 需要消歧的第一人称/自指表达（我 / 咱 / 自己 / 自画像） */
const FIRST_PERSON = /我|咱|自己|自画像/u;

/** 追加给绘制任务的第一人称映射说明 */
const PRONOUN_NOTE = [
	'【任务上下文】本任务由月华本人派发，描述里的第一人称指代固定如下：',
	'「我」「我的」「自己」= 月华；「我的妹妹」「妹妹」= 琉璃；「我们」「我和妹妹」= 月华与琉璃。',
	'画到人物时必须用 portrait / group_photo 并填写对应角色名，不要因为没听到名字就改用「通用-女」或「通用-男」。'
].join('\n');

/**
 * 为含第一人称代词的绘制任务补充指代说明
 *
 * @param taskDescription 对话者传来的任务描述
 * @returns 带指代说明的任务描述；无第一人称代词时原样返回
 */
export function decoratePainterTask(taskDescription: string): string {
	if (!FIRST_PERSON.test(taskDescription)) return taskDescription;
	return `${PRONOUN_NOTE}\n\n${taskDescription}`;
}
