import { ToolCall } from '../../../../config/tool';

/** 多人合照工具（roles 描述中的 {characters} 由角色目录动态替换） */
export const groupTools: ToolCall[] = [
	{
		type: "function",
		function: {
			name: "group_photo",
			description: "生成月华与一个或多个角色的合照。roles 按顺序列出参与者:内置角色与已创建的自定义角色直接写名字,自动继承外观与默认服装;临时角色写完整外观描述(性别、年龄感、发型发色、瞳色、服装、配饰、身份)。chibi=true 时所有参与者使用Q版二头身比例。planning 统一规划每人的表情/动作/服饰,例如\"月华:俏皮眨眼,比V手势,黑色哥特长裙;琉璃:安静微笑,双手交叠,白色星纹裙\";省略则每人使用随机默认表情/动作与默认服装。environment 省略时从预设背景中随机挑选。若本次任务附带参考图,将以该参考图为底图进行图生图,尽量保留参与者的容貌;未附带则按文本描述绘制",
			parameters: {
				type: "object",
				properties: {
					"roles": {
						type: "array",
						items: {
							type: "string"
						},
						description: "合照参与者列表,至少一位。内置角色与已创建的自定义角色直接写名字(自动继承外观与默认服装);临时角色写完整外观描述。任务里的「我们」「我和妹妹」对应 月华、琉璃。{characters}"
					},
					"chibi": {
						type: "boolean",
						description: "是否Q版:true=所有参与者使用Q版二头身比例,false 或不填=全尺寸合照"
					},
					"planning": {
						type: "string",
						description: "合照的统一人物规划:逐人写清表情、动作与服饰,用分号分隔。省略则全部使用默认表情/动作与默认服装"
					},
					"environment": {
						type: "string",
						description: "环境提示词,描述背景环境或场景。省略时随机使用预设背景"
					},
					"ratio": {
						type: "string",
						enum: ["1:1", "4:3", "3:4", "16:9", "9:16", "3:2", "2:3"],
						description: "画面比例预设,默认 1:1。仅在未选定参考图时生效;已选定参考图时输出尺寸自动随参考图比例"
					},
					"strength": {
						type: "number",
						description: "已选定参考图时的图生图强度,取值 0 到 1,越大越偏离参考图。保留参与者容貌建议 0.3~0.5;缺省按中性值 0.5。未选定参考图时忽略"
					},
					"steps": {
						type: "number",
						description: "迭代步数,取值 8~60:quality 方案建议 30~60(常规 40,精细 50~60);turbo 方案建议 8~12。省略时使用当前模型方案的推荐步数。越高细节越好但耗时线性增加,不影响构图"
					},
					"negative_prompt": {
						type: "string",
						description: "负面提示文本,用于排除图像中不希望出现的元素"
					},
					"cfg_scale": {
						type: "number",
						description: "提示词权重调节参数,取值范围为 0 到 2,默认值为 1.0"
					},
					"super_resolution": {
						type: "boolean",
						description: "生成完成后追加 RealESRGAN 超分辨率重建（4x）,提升清晰度与细节。用户要求高清、精细画质时开启,耗时增加"
					}
				},
				required: [
					"roles"
				]
			}
		}
	}
];
