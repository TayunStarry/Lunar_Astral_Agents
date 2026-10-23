package Descriptor

// media.go 影像描述入口：图片 / 动图 / 视频统一走这里
//
// 查询语义（与句柄系统约定一致）：
//   已有描述 → 直接返回，零模型调用；
//   没有描述 → 图片走视觉副本描述，动图/视频现场抽帧观看，返回后写回资源目录；
//   句柄失效 / 本体丢失 → 返回错误，由句柄层走「找不到」流程。
// 定向问题只回本次调用，不覆盖共用描述。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/multimodal"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

// DescribeMedia 取回图片 / 动图 / 视频的内容描述
//
// @param handle   资源句柄（@I.007 / @V.001）
// @param question 可选定向问题（非空时不读缓存、不写缓存）
// @return 描述文本；句柄失效或本体丢失时返回错误
func DescribeMedia(handle, question string) (string, error) {
	runMu.Lock()
	defer runMu.Unlock()

	question = strings.TrimSpace(question)
	res, err := persistence.ResourceResolve(handle)
	if err != nil {
		return "", err
	}
	if res.State != persistence.StateActive {
		return "", fmt.Errorf("%s 的本体已丢失", handle)
	}
	if res.Kind != persistence.KindVideo && res.Kind != persistence.KindImage {
		return "", fmt.Errorf("%s 不是图片、动图或视频", handle)
	}
	// 已有描述且无定向问题：直接命中
	if cached := strings.TrimSpace(res.Summary); cached != "" && question == "" {
		return cached, nil
	}

	abs := filepath.Join(filepath.Clean(*config.LocalDir), filepath.FromSlash(res.BlobPath))
	if res.Kind == persistence.KindVideo {
		groups, err := multimodal.VideoToFrames(abs)
		if err != nil {
			return "", err
		}
		return watchFrameGroups(res, groups, "video", question)
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return "", fmt.Errorf("读取图片失败: %w", err)
	}
	// 动图按序列帧理解；静态图走视觉读取副本
	if multimodal.IsAnimatedImage(data) {
		groups, err := multimodal.AnimatedImageToFrames(data)
		if err != nil {
			return "", err
		}
		return watchFrameGroups(res, groups, "animation", question)
	}
	return describeStaticImage(res, abs, question)
}

// storeResourceSummary 把通用描述写回资源目录（定向问题的答复不入库）
func storeResourceSummary(res persistence.ArchiveResource, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if err := persistence.ResourceSetSummary(res.ID, hardCutRunes(text, summaryStoreChars), nil); err != nil {
		logger.Warn("CrystalAstral", "描述者：写入 %s 描述失败: %v", res.Handle, err)
	}
}
