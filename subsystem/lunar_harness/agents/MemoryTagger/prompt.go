package MemoryTagger

// prompt.go 图片识别取向的归一化与系统提示词构建。

import "strings"

// normalizeOrientation 归一化识别取向：非法或缺参考文本的自定义取向回退为自动处理。
func normalizeOrientation(orientation, custom string) string {
	if !validOrientation(orientation) {
		return recognitionAuto
	}
	if orientation == recognitionCustom && strings.TrimSpace(custom) == "" {
		return recognitionAuto
	}
	return orientation
}

// validOrientation 判断识别取向标识是否合法。
func validOrientation(o string) bool {
	switch o {
	case recognitionEmotion, recognitionText, recognitionColor, recognitionAppearance,
		recognitionSpecies, recognitionPosture, recognitionAuto, recognitionCustom:
		return true
	}
	return false
}

// imageSystemPrompt 依据识别取向构建图片标签生成的系统提示词。
func imageSystemPrompt(orientation, custom string) string {
	head := "你是一个视觉内容标签生成助手。请仔细观察图片，严格以JSON数组格式返回标签，不要包含任何其他内容。\n\n"

	var rules string
	var countRule string
	var example string

	switch orientation {
	case recognitionEmotion:
		rules = "专注识别并描述图片所表达的情绪（如：喜悦、悲伤、愤怒、平静、惊讶、紧张、害羞等）。\n" +
			"只输出与情绪、情感氛围相关的标签，不要输出人物外貌、衣着、发色、物种、场景等无关标签。\n"
		countRule = "若图片存在明显情绪，输出 1-6 个情绪相关标签；若没有明显情绪表达，仅返回 [\"无\"]。"
		example = "示例输出：[\"喜悦\",\"微笑\",\"温暖\",\"平静\"]"
	case recognitionText:
		rules = "专注识别图片中可能存在的文字信息（OCR），提取其中的文字内容、含义或排版特征。\n" +
			"只输出与文字内容相关的标签，不要输出画面中的物体、人物、场景等无关标签。\n"
		countRule = "若图片存在文字，输出 1-8 个文字相关标签；若没有任何文字内容，仅返回 [\"无\"]。"
		example = "示例输出：[\"欢迎光临\",\"限时优惠\",\"标题文字\",\"手写体\"]"
	case recognitionColor:
		rules = "专注分析并描述图片的色彩风格，包括主要配色、次要配色、点缀色、色调倾向、明暗对比与饱和度等。\n" +
			"只输出与色彩相关的标签。\n"
		countRule = "输出 3-10 个色彩相关标签。"
		example = "示例输出：[\"暖色调\",\"主色：橙色\",\"点缀：金色\",\"高饱和度\",\"柔和\"]"
	case recognitionAppearance:
		rules = "着重描述图片中人物的衣着款式、发型特征、身材特点、发色及瞳色等信息。\n" +
			"只输出与人物外观相关的标签。\n"
		countRule = "输出 3-10 个外观相关标签；若图片没有人物，仅返回 [\"无\"]。"
		example = "示例输出：[\"白色连衣裙\",\"黑色长发\",\"蓝色瞳孔\",\"苗条\",\"双马尾\"]"
	case recognitionSpecies:
		rules = "着重识别并描述图片中事物的种类与关键识别特征（如动物、植物、物品、建筑、场景等）。\n" +
			"只输出与物种、种类及其识别特征相关的标签。\n"
		countRule = "输出 3-10 个种类相关标签。"
		example = "示例输出：[\"黑猫\",\"猫科\",\"猫耳\",\"家猫\",\"短毛\"]"
	case recognitionPosture:
		rules = "重点表达图片中可能存在的肢体动作、人物表情与体态特征。\n" +
			"只输出与动作、表情、姿态相关的标签，不要输出外貌、衣着、发色、物种等无关标签。\n"
		countRule = "若图片存在相关动作或表情，输出 1-6 个标签；若没有相关内容，仅返回 [\"无\"]。"
		example = "示例输出：[\"站立\",\"挥手\",\"微笑\",\"奔跑\"]"
	case recognitionCustom:
		rules = "参考用户提供的自定义取向描述来确定图片描述角度与方式：\n" + strings.TrimSpace(custom) + "\n"
		countRule = "输出 3-10 个与自定义取向相关的标签。"
		example = "示例输出：[\"标签1\",\"标签2\",\"标签3\"]"
	default: // recognitionAuto 及未知取向统一走自动处理，单轮内同时覆盖多维度
		rules = "请在一轮回答中同时覆盖以下维度生成标签：\n" +
			"1. 整体内容主题与风格特点\n" +
			"2. 情绪表达（如：喜悦、悲伤、愤怒、平静、惊讶、温暖、宁静）\n" +
			"3. 文字信息（OCR）：如有可见文字，提取其内容或排版特征\n" +
			"4. 若图片包含人物，提取人物特征：\n" +
			"   - 面部表情（如：微笑、严肃、惊讶、悲伤、愤怒）\n" +
			"   - 肢体动作（如：站立、挥手、奔跑、坐着、跳舞）\n" +
			"   - 头发颜色（如：黑色头发、金色头发、棕色头发、红色头发）\n" +
			"   - 服饰风格与颜色（如：白色连衣裙、黑色西装、休闲T恤、校服）\n" +
			"5. 事物种类与关键识别特征（如：黑猫、猫科、建筑物、海边）\n" +
			"6. 色彩倾向与情感氛围\n"
		countRule = "输出 8-18 个标签。"
		example = "示例输出：[\"自然风景\",\"日落\",\"暖色调\",\"海边\",\"宁静\",\"白色连衣裙\",\"微笑\",\"黑色长发\",\"站立\",\"夕阳余晖\"]"
	}

	tail := "用中文输出。" + countRule + "\n\n" + example

	return head + rules + tail
}
