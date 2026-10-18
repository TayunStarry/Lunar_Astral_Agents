import { ToolCall } from '../../../config/tool';
import { CFG_SCALE_PARAM, NEGATIVE_PROMPT_PARAM, RATIO_PARAM, STEPS_PARAM, SUPER_RESOLUTION_PARAM } from './common';

/** 立绘与自定义角色工具（character 描述中的 {characters} 由角色目录动态替换） */
export const portraitTools: ToolCall[] = [
	{
		type: "function",
		function: {
			name: "portrait",
			description: "生成指定角色的立绘。凡是要求绘制月华、琉璃或其他人物的形象时,都必须且只能调用本函数,禁止用 diffusion_generation 或 image_to_image 代替",
			parameters: {
				type: "object",
				properties: {
					"character": {
						type: "string",
						description: "目标角色:内置角色(月华/琉璃)或已创建的自定义角色名,已内置外观,只需给出表情/姿势/环境;通用-女 / 通用-男 或临时角色必须用 appearance 描述其外观。{characters}"
					},
					"chibi": {
						type: "boolean",
						description: "是否Q版:true=二头身萌系比例(贴纸/表情包/头像),false 或不填=标准头身比的全身立绘"
					},
					"appearance": {
						type: "string",
						description: "character=通用-女 / 通用-男 或临时角色时填写该人物的外观:性别、年龄感、发型发色、瞳色、服装、配饰、身份。内置/自定义角色不需要填写。"
					},
					"expression": {
						type: "string",
						description: "表情提示词,描述想要展现的表情"
					},
					"posture": {
						type: "string",
						description: "动作提示词,描述想要展现的姿势或动作"
					},
					"outfit": {
						type: "string",
						description: "服装提示词。不提供则用该角色的默认服装（通用角色仅在 appearance 未写服装时兜底）"
					},
					"environment": {
						type: "string",
						description: "环境提示词,描述背景或场景。月华偏暗色与红,琉璃偏淡蓝天空与星光"
					},
					"ratio": {
						...RATIO_PARAM,
						description: RATIO_PARAM.description + "；缺省：全尺寸 2:3、Q 版 1:1"
					},
					"steps": STEPS_PARAM,
					"negative_prompt": NEGATIVE_PROMPT_PARAM,
					"cfg_scale": CFG_SCALE_PARAM,
					"super_resolution": SUPER_RESOLUTION_PARAM
				},
				required: [
					"character",
					"expression",
					"posture",
					"environment"
				]
			}
		}
	},
	{
		type: "function",
		function: {
			name: "create_character",
			description: "创建或更新一个可复用的自定义角色,写入角色库(SQL数据库)。适合会反复出场的原创角色:创建后 portrait 的 character 与 group_photo 的 roles 都能直接用角色名引用,无需每次重写外观。一次性路人不建议创建。内置角色(月华/琉璃)不能重名",
			parameters: {
				type: "object",
				properties: {
					"name": {
						type: "string",
						description: "角色名(唯一键,后续绘制时用这个名字引用)"
					},
					"gender": {
						type: "string",
						enum: ["女", "男"],
						description: "性别,决定立绘模板中的性别表述"
					},
					"appearance": {
						type: "string",
						description: "基础外观:年龄感、发型发色、瞳色、面部特征、配饰、体型。这是角色的身份定义,不写表情/动作/场景"
					},
					"outfit_full": {
						type: "string",
						description: "默认全尺寸服装。省略则使用简洁得体的日常服装"
					},
					"outfit_chibi": {
						type: "string",
						description: "默认Q版服装。省略则复用全尺寸服装"
					}
				},
				required: [
					"name",
					"gender",
					"appearance"
				]
			}
		}
	}
];
