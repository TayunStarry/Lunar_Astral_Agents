package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
)

// =============================================================================
// v2 LLM 标签生成 — 调用 /v1/chat/completions 自动生成中文标签
// =============================================================================

// generateTags 调用 LLM 为内容生成中文标签，返回标签数组
// isImage 为 true 时使用多模态 vision 格式请求
// orientation/custom 仅对图片生效，用于指定识别取向（文本内容固定使用自动处理）
// 图片默认(auto)模式使用单轮调用，prompt 内同时覆盖情绪/文本(OCR)/人物特征/种类/色彩/姿态等维度
// 最多重试 MaxTagRetries 次，全部失败则返回错误
func (d *Store) generateTags(ctx context.Context, content string, isImage bool, orientation string, custom string) ([]string, error) {
	if config.AgentMultimodalURL == "" {
		return nil, fmt.Errorf("LLM 服务 base_url 未配置")
	}

	var lastErr error
	for attempt := 0; attempt < MaxTagRetries; attempt++ {
		tags, err := d.generateTagsOnce(ctx, content, isImage, orientation, custom)
		if err == nil && len(tags) > 0 {
			return tags, nil
		}
		lastErr = err
		if attempt < MaxTagRetries-1 {
			// 短暂等待后重试
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Second):
			}
		}
	}
	return nil, fmt.Errorf("标签生成失败（已重试 %d 次）: %w", MaxTagRetries, lastErr)
}

// generateTagsOnce 单次 LLM 标签生成尝试
func (d *Store) generateTagsOnce(ctx context.Context, content string, isImage bool, orientation string, custom string) ([]string, error) {
	var messages []chatMessage

	if isImage {
		// 多模态图片标签生成，依据识别取向定制系统提示词
		orientation = normalizeImageOrientation(orientation, custom)
		systemMsg := chatMessage{
			Role:    "system",
			Content: imageTagSystemPrompt(orientation, custom),
		}
		userMsg := chatMessage{
			Role: "user",
			Content: []chatContentPart{
				{Type: "text", Text: "请为这张图片生成标签，以JSON数组格式返回"},
				{Type: "image_url", ImageURL: &imageURL{URL: content}},
			},
		}
		messages = []chatMessage{systemMsg, userMsg}
	} else {
		// 文本标签生成
		systemMsg := chatMessage{
			Role: "system",
			Content: "你是一个内容标签生成助手。请为以下文本生成标签，描述其核心主题和关键信息。用中文输出，严格以JSON数组格式返回，不要包含任何其他内容。\n\n" +
				"示例输出：[\"人工智能\",\"机器学习\",\"深度学习\",\"神经网络\"]",
		}
		userMsg := chatMessage{
			Role:    "user",
			Content: "请为以下文本生成标签，以JSON数组格式返回：\n\n" + content,
		}
		messages = []chatMessage{systemMsg, userMsg}
	}

	return d.postTagRequest(ctx, messages)
}

// generateTagsFromMedia 动态图标签生成：mediaRefs 为统一观看管线抽出的采样帧引用
// （整段动画等距抽取，与视频切片同一套规则），最多重试 MaxTagRetries 次
func (d *Store) generateTagsFromMedia(ctx context.Context, mediaRefs []string, orientation string, custom string) ([]string, error) {
	if config.AgentMultimodalURL == "" {
		return nil, fmt.Errorf("LLM 服务 base_url 未配置")
	}

	var lastErr error
	for attempt := 0; attempt < MaxTagRetries; attempt++ {
		tags, err := d.generateTagsFromMediaOnce(ctx, mediaRefs, orientation, custom)
		if err == nil && len(tags) > 0 {
			return tags, nil
		}
		lastErr = err
		if attempt < MaxTagRetries-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Second):
			}
		}
	}
	return nil, fmt.Errorf("动态图标签生成失败（已重试 %d 次）: %w", MaxTagRetries, lastErr)
}

// generateTagsFromMediaOnce 单次动态图标签生成：
// user 消息含采样帧引用（image_url 形式的 data URI，帧数已由统一观看管线封顶）
// 与文本说明，综合动作过程与画面内容生成标签
func (d *Store) generateTagsFromMediaOnce(ctx context.Context, mediaRefs []string, orientation string, custom string) ([]string, error) {
	// 动态图按识别取向定制系统提示词，并追加帧序列说明
	orientation = normalizeImageOrientation(orientation, custom)
	systemMsg := chatMessage{
		Role: "system",
		Content: imageTagSystemPrompt(orientation, custom) +
			"\n\n本次输入不是单张静态图片，而是同一段动画（GIF/动图）按时间顺序抽取的采样帧序列。" +
			"请综合各帧的画面内容与帧间的动作/变化过程生成标签，标签需覆盖动作与过程维度。",
	}

	parts := make([]chatContentPart, 0, len(mediaRefs)+1)
	parts = append(parts, chatContentPart{Type: "text", Text: "以下是同一段动画按时间顺序抽取的采样帧，请综合动作过程与画面内容生成标签，以JSON数组格式返回"})
	for _, ref := range mediaRefs {
		parts = append(parts, chatContentPart{Type: "image_url", ImageURL: &imageURL{URL: ref}})
	}

	messages := []chatMessage{
		systemMsg,
		{Role: "user", Content: parts},
	}
	return d.postTagRequest(ctx, messages)
}

// postTagRequest 发送标签生成请求并解析响应（消息构造与请求执行解耦，供静态/动态图分支复用）
func (d *Store) postTagRequest(ctx context.Context, messages []chatMessage) ([]string, error) {
	llmURL := config.AgentMultimodalURL
	apiURL := strings.TrimRight(llmURL, "/") + "/chat/completions"

	reqBody := chatRequest{
		Model:       config.AgentMultimodalModel,
		Messages:    messages,
		MaxTokens:   512,
		Temperature: 0.3,
		// 标签生成只需要一段 JSON，不需要思考过程。Gemma-4 等推理模型默认会先输出
		// 很长的 reasoning_content，max_tokens 被思考耗尽后 content 为空，表现为
		// 「LLM 返回空内容」并让动态图标签重试 3 次。显式关闭思考可稳定拿到标签。
		ChatTemplateKwargs: map[string]any{"enable_thinking": false},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化 LLM 请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("创建 LLM 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if config.AgentMultimodalKey != "" {
		req.Header.Set("Authorization", "Bearer "+config.AgentMultimodalKey)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 LLM 服务失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 LLM 响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp chatResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("LLM 错误 (HTTP %d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("LLM 返回异常状态码 %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("解析 LLM 响应失败: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("LLM 响应无 choices")
	}

	rawContent := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	if rawContent == "" {
		if reasoning := strings.TrimSpace(chatResp.Choices[0].Message.ReasoningContent); reasoning != "" {
			return nil, fmt.Errorf("LLM 返回空内容（仅推理内容: %s）", truncateForLog(reasoning, 120))
		}
		return nil, fmt.Errorf("LLM 返回空内容")
	}

	// 解析 JSON 数组：尝试从响应中提取 ["标签1","标签2",...]
	tags, err := parseTagsJSON(rawContent)
	if err != nil {
		return nil, fmt.Errorf("解析标签失败: %w (原始响应: %s)", err, truncateForLog(rawContent, 100))
	}

	return tags, nil
}

// parseTagsJSON 从 LLM 响应文本中提取 JSON 字符串数组
// 支持纯 JSON 数组、markdown 代码块包裹、以及带前后文字的混合格式
func parseTagsJSON(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)

	// 去除 markdown 代码块标记
	if strings.HasPrefix(raw, "```") {
		// 找到第一个换行后的内容（跳过语言标记行如 ```json）
		if idx := strings.Index(raw, "\n"); idx != -1 {
			raw = raw[idx+1:]
		}
		// 去除结尾的 ```（先 trim 尾部空白再 TrimSuffix，避免换行符干扰）
		raw = strings.TrimRight(raw, " \t\n\r")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
	}

	// 尝试找到 JSON 数组 [...]
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start == -1 || end == -1 || start >= end {
		return nil, fmt.Errorf("未找到 JSON 数组")
	}

	jsonStr := raw[start : end+1]

	var tags []string
	if err := json.Unmarshal([]byte(jsonStr), &tags); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}

	// 过滤空标签
	result := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t != "" {
			result = append(result, t)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("解析到的标签数组为空")
	}

	return result, nil
}

// isValidRecognitionOrientation 判断识别取向标识是否合法
func isValidRecognitionOrientation(o string) bool {
	switch o {
	case RecognitionEmotion, RecognitionText, RecognitionColor, RecognitionAppearance,
		RecognitionSpecies, RecognitionPosture, RecognitionAuto, RecognitionCustom:
		return true
	}
	return false
}

// normalizeImageOrientation 归一化识别取向：
// 未指定、非法标识、或自定义取向缺少参考文本时，统一回退为自动处理
func normalizeImageOrientation(orientation string, custom string) string {
	if !isValidRecognitionOrientation(orientation) {
		return RecognitionAuto
	}
	if orientation == RecognitionCustom && strings.TrimSpace(custom) == "" {
		return RecognitionAuto
	}
	return orientation
}

// imageTagSystemPrompt 依据识别取向构建图片标签生成的系统提示词
// orientation 必须是已归一化的合法取向，custom 为自定义取向参考文本（仅 custom 使用）
// 每个取向使用独立的标签数量规则与示例输出，避免「自动处理」的示例污染其他取向
func imageTagSystemPrompt(orientation string, custom string) string {
	head := "你是一个视觉内容标签生成助手。请仔细观察图片，严格以JSON数组格式返回标签，不要包含任何其他内容。\n\n"

	var rules string
	var countRule string
	var example string

	switch orientation {
	case RecognitionEmotion:
		rules = "专注识别并描述图片所表达的情绪（如：喜悦、悲伤、愤怒、平静、惊讶、紧张、害羞等）。\n" +
			"只输出与情绪、情感氛围相关的标签，不要输出人物外貌、衣着、发色、物种、场景等无关标签。\n"
		countRule = "若图片存在明显情绪，输出 1-6 个情绪相关标签；若没有明显情绪表达，仅返回 [\"无\"]。"
		example = "示例输出：[\"喜悦\",\"微笑\",\"温暖\",\"平静\"]"
	case RecognitionText:
		rules = "专注识别图片中可能存在的文字信息（OCR），提取其中的文字内容、含义或排版特征。\n" +
			"只输出与文字内容相关的标签，不要输出画面中的物体、人物、场景等无关标签。\n"
		countRule = "若图片存在文字，输出 1-8 个文字相关标签；若没有任何文字内容，仅返回 [\"无\"]。"
		example = "示例输出：[\"欢迎光临\",\"限时优惠\",\"标题文字\",\"手写体\"]"
	case RecognitionColor:
		rules = "专注分析并描述图片的色彩风格，包括主要配色、次要配色、点缀色、色调倾向、明暗对比与饱和度等。\n" +
			"只输出与色彩相关的标签。\n"
		countRule = "输出 3-10 个色彩相关标签。"
		example = "示例输出：[\"暖色调\",\"主色：橙色\",\"点缀：金色\",\"高饱和度\",\"柔和\"]"
	case RecognitionAppearance:
		rules = "着重描述图片中人物的衣着款式、发型特征、身材特点、发色及瞳色等信息。\n" +
			"只输出与人物外观相关的标签。\n"
		countRule = "输出 3-10 个外观相关标签；若图片没有人物，仅返回 [\"无\"]。"
		example = "示例输出：[\"白色连衣裙\",\"黑色长发\",\"蓝色瞳孔\",\"苗条\",\"双马尾\"]"
	case RecognitionSpecies:
		rules = "着重识别并描述图片中事物的种类与关键识别特征（如动物、植物、物品、建筑、场景等）。\n" +
			"只输出与物种、种类及其识别特征相关的标签。\n"
		countRule = "输出 3-10 个种类相关标签。"
		example = "示例输出：[\"黑猫\",\"猫科\",\"猫耳\",\"家猫\",\"短毛\"]"
	case RecognitionPosture:
		rules = "重点表达图片中可能存在的肢体动作、人物表情与体态特征。\n" +
			"只输出与动作、表情、姿态相关的标签，不要输出外貌、衣着、发色、物种等无关标签。\n"
		countRule = "若图片存在相关动作或表情，输出 1-6 个标签；若没有相关内容，仅返回 [\"无\"]。"
		example = "示例输出：[\"站立\",\"挥手\",\"微笑\",\"奔跑\"]"
	case RecognitionCustom:
		rules = "参考用户提供的自定义取向描述来确定图片描述角度与方式：\n" + strings.TrimSpace(custom) + "\n"
		countRule = "输出 3-10 个与自定义取向相关的标签。"
		example = "示例输出：[\"标签1\",\"标签2\",\"标签3\"]"
	default: // RecognitionAuto 及未知取向统一走自动处理，单轮内同时覆盖多维度
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

// truncateForLog 截断字符串用于日志输出
func truncateForLog(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
