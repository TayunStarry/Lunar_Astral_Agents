package LunarPersonality

// painterToolDefinitions 绘制者工具集
func painterToolDefinitions() []ToolCall {
	ratioParam := map[string]any{
		"type":        "string",
		"enum":        []string{"1:1", "4:3", "3:4", "16:9", "9:16", "3:2", "2:3"},
		"description": "画面比例预设。人物/物品用 2:3 或 3:4，风景/横构图用 3:2 或 16:9，方形构图用 1:1",
	}
	stepsParam := map[string]any{"type": "number", "description": "迭代步数，取值 8~60：quality 方案建议 30~60；turbo 方案建议 8~12。省略时使用当前模型方案的推荐步数"}
	negativeParam := map[string]any{"type": "string", "description": "负面提示文本，用于排除不希望出现的元素"}
	cfgParam := map[string]any{"type": "number", "description": "提示词引导系数（0~2），默认 1.0。当前为引导蒸馏模型，保持 1.0"}
	superParam := map[string]any{"type": "boolean", "description": "生成完成后追加 RealESRGAN 超分辨率重建（4x）"}

	return []ToolCall{
		functionTool("diffusion_generation",
			"根据文本描述进行文生图。仅适用于风景、物品、场景、氛围图等与人物形象无关的创作；严禁用于绘制任何人物形象（月华、琉璃或其他人物都必须调用 portrait）。",
			map[string]any{
				"prompt":           map[string]any{"type": "string", "description": "图像生成的正向描述文本"},
				"ratio":            mergeParam(ratioParam, "；缺省 1:1"),
				"steps":            stepsParam,
				"negative_prompt":  negativeParam,
				"cfg_scale":        cfgParam,
				"super_resolution": superParam,
			}, []string{"prompt"}),
		functionTool("image_to_image",
			"基于参考图进行图生图创作：以参考图为底图，按文本描述改变风格、场景、服饰或细节。无参考图时不可调用。",
			map[string]any{
				"prompt":           map[string]any{"type": "string", "description": "正向描述文本，描述希望参考图变成什么样子"},
				"strength":         map[string]any{"type": "number", "description": "图生图强度，0~1，越大越偏离参考图；缺省 0.5"},
				"steps":            stepsParam,
				"negative_prompt":  negativeParam,
				"cfg_scale":        cfgParam,
				"super_resolution": superParam,
			}, []string{"prompt"}),
		functionTool("portrait",
			"生成指定角色的立绘。凡是要求绘制月华、琉璃或其他人物的形象时，都必须且只能调用本函数。{characters}",
			map[string]any{
				"character":        map[string]any{"type": "string", "description": "目标角色：内置角色（月华/琉璃）或已创建的自定义角色名；通用-女 / 通用-男 或临时角色必须用 appearance 描述外观。{characters}"},
				"chibi":            map[string]any{"type": "boolean", "description": "是否Q版：true=二头身萌系比例"},
				"appearance":       map[string]any{"type": "string", "description": "通用角色或临时角色的外观描述"},
				"expression":       map[string]any{"type": "string", "description": "表情提示词"},
				"posture":          map[string]any{"type": "string", "description": "姿势或动作提示词"},
				"outfit":           map[string]any{"type": "string", "description": "服装提示词，不提供则用角色默认服装"},
				"environment":      map[string]any{"type": "string", "description": "环境提示词"},
				"ratio":            mergeParam(ratioParam, "；缺省：全尺寸 2:3、Q 版 1:1"),
				"steps":            stepsParam,
				"negative_prompt":  negativeParam,
				"cfg_scale":        cfgParam,
				"super_resolution": superParam,
			}, []string{"character", "expression", "posture", "environment"}),
		functionTool("create_character",
			"创建或更新一个可复用的自定义角色，写入角色库。适合会反复出场的原创角色。",
			map[string]any{
				"name":         map[string]any{"type": "string", "description": "角色名（唯一键）"},
				"gender":       map[string]any{"type": "string", "enum": []string{"女", "男"}, "description": "性别"},
				"appearance":   map[string]any{"type": "string", "description": "基础外观：年龄感、发型发色、瞳色、面部特征、配饰、体型"},
				"outfit_full":  map[string]any{"type": "string", "description": "默认全尺寸服装"},
				"outfit_chibi": map[string]any{"type": "string", "description": "默认Q版服装"},
			}, []string{"name", "gender", "appearance"}),
		functionTool("group_photo",
			"生成月华与一个或多个角色的合照。{characters}",
			map[string]any{
				"roles":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "合照参与者列表，至少一位。{characters}"},
				"chibi":            map[string]any{"type": "boolean", "description": "是否Q版"},
				"planning":         map[string]any{"type": "string", "description": "统一人物规划：逐人写清表情、动作与服饰"},
				"environment":      map[string]any{"type": "string", "description": "环境提示词，省略时随机使用预设背景"},
				"ratio":            mergeParam(ratioParam, "；缺省 1:1"),
				"strength":         map[string]any{"type": "number", "description": "已选定参考图时的图生图强度，0~1"},
				"steps":            stepsParam,
				"negative_prompt":  negativeParam,
				"cfg_scale":        cfgParam,
				"super_resolution": superParam,
			}, []string{"roles"}),
		functionTool("switch_diffusion_scheme",
			"切换后续绘图使用的模型方案。quality=质量优先；turbo=速度优先。",
			map[string]any{"scheme": map[string]any{"type": "string", "enum": []string{"turbo", "quality"}, "description": "目标方案"}},
			[]string{"scheme"}),
		functionTool("list_images",
			"查看资源目录中最近的图片句柄与内容描述。",
			map[string]any{}, []string{}),
		functionTool("upscale_image",
			"对已有图片执行 RealESRGAN 超分辨率放大（4x）。",
			map[string]any{"image_tag": map[string]any{"type": "string", "description": "图片句柄（如 @I.003）；不填时对最近作品超分"}},
			[]string{}),
	}
}

func mergeParam(base map[string]any, suffix string) map[string]any {
	result := make(map[string]any, len(base)+1)
	for key, value := range base {
		result[key] = value
	}
	result["description"] = base["description"].(string) + suffix
	return result
}
