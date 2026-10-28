package Descriptor

// image.go 静态图片描述：通过视觉读取副本交给模型，结论写回资源目录
//
// 图片进入模型的唯一形态是视觉读取管线产物（约束尺寸 + 句柄叠印副本），
// 原图字节不出 Go 侧（见 docs/adr/0008）。

import (
	"LunarSubsystem/LunarHarness/engine/resource"
)

// describeStaticImage 观看一张静态图片并返回描述
func describeStaticImage(res resource.Resource, abs, question string) (string, error) {
	uri, err := resourceVisionURI(res, abs)
	if err != nil {
		return "", err
	}
	system := promptText("descriptorImageRole.md")
	task := imageTask(question)
	messages := []chatMessage{
		{Role: "system", Content: system},
		{
			Role: "user",
			Content: []contentPart{
				{Type: "text", Text: task},
				{Type: "image_url", ImageURL: &imageURL{URL: uri}},
			},
		},
	}
	text, err := chatWithRetry(messages, nil, res.Handle)
	if err != nil {
		return "", err
	}
	if question == "" {
		storeResourceSummary(res, text)
	}
	return text, nil
}

// imageTask 构建图片描述任务
func imageTask(question string) string {
	if question != "" {
		return renderPrompt(promptText("descriptorImageQuestionTask.md"), "{question}", question)
	}
	return promptText("descriptorImageTask.md")
}
