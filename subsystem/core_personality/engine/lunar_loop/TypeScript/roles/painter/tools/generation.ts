import { ToolCall } from '../../../config/tool';
import { CFG_SCALE_PARAM, NEGATIVE_PROMPT_PARAM, RATIO_PARAM, STEPS_PARAM, SUPER_RESOLUTION_PARAM } from './common';

/** 通用文生图 / 图生图工具 */
export const generationTools: ToolCall[] = [
	{
		type: "function",
		function: {
			name: "diffusion_generation",
			description: "根据文本描述进行文生图。仅适用于风景、物品、场景、氛围图等与人物形象无关的创作；严禁用于绘制任何人物形象(月华、琉璃或其他人物都必须调用 portrait)。",
			parameters: {
				type: "object",
				properties: {
					"prompt": {
						type: "string",
						description: "图像生成的正向描述文本"
					},
					"ratio": {
						...RATIO_PARAM,
						description: RATIO_PARAM.description + "；缺省 1:1"
					},
					"steps": STEPS_PARAM,
					"negative_prompt": NEGATIVE_PROMPT_PARAM,
					"cfg_scale": CFG_SCALE_PARAM,
					"super_resolution": SUPER_RESOLUTION_PARAM
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
			name: "image_to_image",
			description: "基于参考图进行图生图创作:以参考图为底图,按文本描述改变风格、场景、服饰或细节,同时保留参考图的主体特征。无参考图时不可调用:人物形象改用 portrait,非人物改用 diffusion_generation,或说明需要用户提供参考图片。",
			parameters: {
				type: "object",
				properties: {
					"prompt": {
						type: "string",
						description: "图像生成的正向描述文本,描述希望参考图变成什么样子"
					},
					"strength": {
						type: "number",
						description: "图生图强度,取值范围 0 到 1,越大越偏离参考图。必须由你根据创作意图指定:只改画风/光照/质感用 0.2~0.4;换服饰/换配色/微调细节用 0.4~0.6;换背景/换地点/大幅改场景 用 0.8~0.9 推荐 0.85;替换画面里的一个对象(如把北极熊换成熊猫)同样用 0.8~0.9——实测 0.65/0.75 换不掉大物体。注意:强度≥0.8 时提示词主导画面,没写进提示词的东西会消失,因此提示词必须描述整个画面(含所有要保留的主体)。缺省按中性值 0.5"
					},
					"steps": STEPS_PARAM,
					"negative_prompt": {
						...NEGATIVE_PROMPT_PARAM,
						description: NEGATIVE_PROMPT_PARAM.description + "（换场景时点名去掉旧场景元素，如 beach, ocean, water, sea）"
					},
					"cfg_scale": {
						...CFG_SCALE_PARAM,
						description: CFG_SCALE_PARAM.description + "。需要改得更多时调 strength，不要调 cfg"
					},
					"super_resolution": SUPER_RESOLUTION_PARAM
				},
				required: [
					"prompt"
				]
			}
		}
	}
];
