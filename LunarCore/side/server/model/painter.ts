import { ToolCall,  RandomFloor, ModelBuilder } from '../index';

export class PainterRole extends ModelBuilder {
	/** 默认表情提示 */
	protected defaultExpressionPrompt = [
		'温柔的表情,开心的笑容,脸颊泛红',
		'害羞的表情,抿嘴微笑,眼神躲闪',
		'俏皮的表情,单眼眨眼,嘴角微微上扬,略带调皮的笑容',
		'平静的表情,眼神略微向下看向一侧,嘴唇轻抿,无笑容,若有所思',
		'惊讶的表情,双眼睁大,眉毛抬高,嘴巴微张成O形,脸颊泛红',
		'非常开心的表情,双眼弯成月牙形,张大嘴巴欢笑,脸颊泛红明显',
		'害羞的表情,眼神向下看,眉毛呈八字形,抿嘴微笑,脸颊大面积泛红',
		'自信的表情,眼神直视前方,眉毛微微上扬,嘴角带有一抹浅笑,眼神明亮'
	]
	/** 默认姿势提示 */
	protected defaultPosturePrompt = [
		'一条腿轻轻抬起,俏皮姿势',
		'双手背在身后,身体微微前倾,双脚并拢',
		'一手插在外套口袋里,另一只手轻抬至脸颊旁比“V”字手势,身体略侧,双脚前后交叉站立',
		'双手自然垂放于身前,手指轻轻交握,双肩微微内收,双脚并拢,站姿端正',
		'手抬起至嘴前,十指轻轻触碰（做捂嘴状）,身体微微后仰,一条腿向后小半步,重心落在后脚',
		'双手举过头顶比心或张开五指,一条腿向后踢起,身体微微前倾,脚尖离地呈跳跃瞬间',
		'双手食指在胸前互点,头部微微低下,双膝内扣,两脚脚尖向内呈内八站姿',
		'双手叉腰,挺胸收腹,一条腿向侧方伸出,脚尖点地,身体笔直有力',
	]
	/** 自我外观提示 */
	protected selfAppearancePrompt = fileView('prompts/selfAppearance.md')[0]
	/** 绘画角色工具 */
	protected roleTool: ToolCall[] = [
		{
			type: "function",
			function: {
				name: "diffusion_generation",
				description: "根据文本描述生成图像。如需进行图像创作,请调用此函数",
				parameters: {
					type: "object",
					properties: {
						"prompt": {
							type: "string",
							description: "图像生成的正向描述文本"
						},
						"negative_prompt": {
							type: "string",
							description: "负面提示文本,用于排除图像中不希望出现的元素"
						},
						"use_reference": {
							type: "boolean",
							description: "是否使用上一次生成的图像作为参考,默认值为 false"
						},
						"strength": {
							type: "number",
							description: "参考图像的影响强度,取值范围为 0 到 1,默认值为 0.65"
						},
						"cfg_scale": {
							type: "number",
							description: "提示词权重调节参数,取值范围为 0 到 2,默认值为 1.0"
						}
					},
					required: [
						"prompt"
					]
				}
			}
		},
		{
			type: "function",
			function: {
				name: "self_portrait",
				description: "生成自画像。调用此函数来创建自己的形象",
				parameters: {
					type: "object",
					properties: {
						"expression": {
							type: "string",
							description: "表情提示词,描述想要展现的表情"
						},
						"posture": {
							type: "string",
							description: "动作提示词,描述想要展现的姿势或动作"
						},
						"environment": {
							type: "string",
							description: "环境提示词,描述背景环境或场景"
						}
					},
					required: [
						"expression",
						"posture",
						"environment"
					]
				}
			}
		}
	]
	/** 构造函数 */
	public constructor() {
		super();
		this.useMultimodal(fileView('prompts/painterRole.md')[0]);
	}
	/** 获得写入了动作与表情的自我外观提示词 */
	protected writeAppearancePrompt(expression?: string, posture?: string): string {
		/** 当前表情提示词, 默认使用随机表情提示 */
		const currentExpression = expression || this.defaultExpressionPrompt[RandomFloor(0, this.defaultExpressionPrompt.length - 1)];
		/** 当前姿势提示词, 默认使用随机姿势提示 */
		const currentPosture = posture || this.defaultPosturePrompt[RandomFloor(0, this.defaultPosturePrompt.length - 1)];
		// 替换表情提示词与姿势提示词
		return this.selfAppearancePrompt.replace('{expression}', currentExpression).replace('{posture}', currentPosture);
	}
}