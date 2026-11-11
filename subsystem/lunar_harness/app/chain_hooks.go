package app

// chain_hooks.go 装配层把链路实现接到引擎 / 智能体的能力钩子上。
//
// 链不再在 init() 里改全局状态：注入时机、注入目标与注入顺序都由 Assemble
// 显式决定，读一遍调用顺序即可知道「谁在什么之前拿到实现」。

import (
	"errors"

	"LunarSubsystem/LunarHarness/agents/Dialogue"
	"LunarSubsystem/LunarHarness/agents/Vision"
	"LunarSubsystem/LunarHarness/chain/life"
	"LunarSubsystem/LunarHarness/chain/search"
	"LunarSubsystem/LunarHarness/chain/thinking"
	"LunarSubsystem/LunarHarness/engine/resourcehub"
	"LunarSubsystem/LunarHarness/executors/ArchiveReader"
)

// wireChainHooks 注入对话者工具执行器与统一资源基座需要的能力。
// 必须在 server.Start 与思考链启动之前调用：资源取回与工具执行都依赖这些实现。
func wireChainHooks() {
	// 资源解读：engine/resourcehub 只声明钩子，视觉 / 文献 / 归档 / 网页 / B 站的
	// 具体实现由思维链提供，供对话者与生活-创造者共用同一套取回逻辑。
	resourcehub.SetHooks(resourcehub.Hooks{
		DescribeMedia:      thinking.DescribeMedia,
		DescribeVideo:      thinking.DescribeVideoURL,
		InspectText:        thinking.InspectTextResource,
		ReadPage:           thinking.ReadPage,
		BilibiliImpression: thinking.BilibiliImpression,
		IsArchiveExt: func(ext string) bool {
			return ArchiveReader.IsArchiveExt(ext) || ArchiveReader.IsGGUFExt(ext)
		},
		InspectArchive: ArchiveReader.Inspect,
		IsVideoExpired: func(err error) bool {
			return errors.Is(err, Vision.ErrVideoExpired)
		},
	})

	// 对话者工具执行器：宿主推送 / 检索 / 生活事件能力由链侧实现，工具声明留在智能体包。
	host := thinking.PainterHost{}
	Dialogue.PushImages = host.PushImages
	Dialogue.PushMusic = thinking.PushAssistantMusic
	Dialogue.VisionCopy = host.VisionCopy
	Dialogue.VisionReference = host.VisionReference
	Dialogue.ReadImageBytes = host.ReadImageBytes
	Dialogue.ResolveImageRef = thinking.ResolveImageRef
	Dialogue.SaveImage = thinking.SaveImage
	Dialogue.WebSearch = search.Run
	Dialogue.RequestLifeEvent = life.RequestEvent
}
