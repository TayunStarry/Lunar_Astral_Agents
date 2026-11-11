package thinking

// hooks_test.go 测试期显式装配链侧钩子。
//
// 本包不再有 init() 副作用：生产装配在 app.Assemble（wireChainHooks），
// 测试需要按同样方式显式接上默认实现，避免测试隐式依赖包初始化顺序。

import (
	"errors"
	"os"
	"testing"

	"LunarSubsystem/LunarHarness/agents/Dialogue"
	"LunarSubsystem/LunarHarness/agents/Vision"
	"LunarSubsystem/LunarHarness/chain/life"
	"LunarSubsystem/LunarHarness/chain/search"
	"LunarSubsystem/LunarHarness/engine/resourcehub"
	"LunarSubsystem/LunarHarness/executors/ArchiveReader"
)

func TestMain(m *testing.M) {
	resourcehub.SetHooks(resourcehub.Hooks{
		DescribeMedia:      DescribeMedia,
		DescribeVideo:      DescribeVideoURL,
		InspectText:        InspectTextResource,
		ReadPage:           ReadPage,
		BilibiliImpression: BilibiliImpression,
		IsArchiveExt: func(ext string) bool {
			return ArchiveReader.IsArchiveExt(ext) || ArchiveReader.IsGGUFExt(ext)
		},
		InspectArchive: ArchiveReader.Inspect,
		IsVideoExpired: func(err error) bool {
			return errors.Is(err, Vision.ErrVideoExpired)
		},
	})

	host := PainterHost{}
	Dialogue.PushImages = host.PushImages
	Dialogue.PushMusic = PushAssistantMusic
	Dialogue.VisionCopy = host.VisionCopy
	Dialogue.VisionReference = host.VisionReference
	Dialogue.ReadImageBytes = host.ReadImageBytes
	Dialogue.ResolveImageRef = ResolveImageRef
	Dialogue.SaveImage = SaveImage
	Dialogue.WebSearch = search.Run
	Dialogue.RequestLifeEvent = life.RequestEvent

	os.Exit(m.Run())
}
