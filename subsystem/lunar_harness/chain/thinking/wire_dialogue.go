package thinking

// wire_dialogue.go 把思维链侧可提供的宿主能力与生活 / 检索能力注入对话者工具执行器。
//
// 工具执行器归属 agents/Dialogue；这些能力只在链侧实现，因此由链在装配期注入，
// 智能体包本身不反向依赖 chain。创作链能力（RunFileOperations）由 app 装配层注入，
// 以保持「thinking 不直接 import creation」。

import (
	"LunarSubsystem/LunarHarness/agents/Dialogue"
	"LunarSubsystem/LunarHarness/chain/life"
	"LunarSubsystem/LunarHarness/chain/search"
)

func init() {
	Dialogue.PushImages = func(images []string) []string {
		return pushAssistantImage(images, false)
	}
	Dialogue.PushMusic = pushAssistantMusic
	Dialogue.VisionCopy = func(relPath, tag string) (int, int, error) {
		_, _, width, height, err := visionCopy(relPath, tag, 0)
		return width, height, err
	}
	Dialogue.VisionReference = visionReference
	Dialogue.ReadImageBytes = resolveImageBytes
	Dialogue.ResolveImageRef = resolveImageRef
	Dialogue.SaveImage = func(name string, data []byte) (string, error) {
		if _, err := persistenceSaveBytes(name, data); err != nil {
			return "", err
		}
		return localPathToReadURL(name), nil
	}

	Dialogue.WebSearch = search.Run
	Dialogue.RequestLifeEvent = life.RequestEvent
}
