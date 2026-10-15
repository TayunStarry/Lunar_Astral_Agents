package updatepack

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/TuiSystem/channel"
)

// ==== 发行说明（Release 描述） ====
//
// 推送更新包时的描述文本采用 RELEASE.md 的**精简版**：先说明「这是什么发行版」，
// 再给出本次发布的实际内容与安装方式。此前只有几行目录清单，用户点开 Release
// 看不出这是什么、能做什么、该怎么用。
//
// 维护约定：本文件是 RELEASE.md 的发行说明投影——RELEASE.md 的定位/平台/安装
// 方式如有调整，请同步这里的常量（保持精简，只留「是什么 / 装什么 / 怎么装」）。

const (
	// releaseProductLine 一句话定位（RELEASE.md「项目简介」精简）
	releaseProductLine = "星月智能（Lunar Astral Agents）是纯本地化的 AI 智能体平台：" +
		"多模态对话、图像生成、语音识别与合成全部在本机推理，无需 Python 环境与云端依赖。"
	// releasePlatform 适用平台
	releasePlatform = "Windows 10/11（64 位）"
)

// releaseIntro 发行说明开篇（说明「这是什么包」）
func releaseIntro() []string {
	return []string{
		"**这是什么**",
		releaseProductLine,
		"",
		"本附件是它的**资源更新包**（增量包，不是完整发行版）：内含模型资源目录、",
		"LTPX 工具包与配置文件等运行时资源，覆盖到程序根目录即可生效；",
		"包含主程序文件的更新会在拉取后自动关闭程序、备份覆盖并重启。",
		"",
		"**适用平台**：" + releasePlatform,
	}
}

// releaseUsage 安装方式（说明「怎么用」）
func releaseUsage() []string {
	return []string{
		"**安装方式**",
		"1. 推荐：控制台「资源更新 管理 → 拉取最新」，自动下载、备份并覆盖（含主程序时自动重启）。",
		"2. 手动：下载全部分卷后从 `.7z.001` 起解压，将内容覆盖到程序根目录。",
		"",
		"**注意**：全部分卷需齐备后再解压；覆盖前旧文件会备份到 `.backup/<时间戳>/`。",
	}
}

// buildReleaseBody 组装更新包 Release 描述。
//
// 结构：标记行 → 发行说明 → 本次发布 → 包内文件与校验。
// 标记行必须保留（[UPDATE]），它是与 LTPX 应用市场共用仓库时的通道判别依据。
func buildReleaseBody(cfg Config, version string, packFiles []string, shas map[string]string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s 星月智能 · 资源更新包 %s\n\n", channel.UpdateMarker, version)
	b.WriteString(strings.Join(releaseIntro(), "\n"))
	b.WriteString("\n\n")

	b.WriteString("**本次发布**\n")
	fmt.Fprintf(&b, "- 版本：%s\n", version)
	fmt.Fprintf(&b, "- 时间：%s\n", time.Now().Format("2006-01-02 15:04:05"))
	if len(cfg.Include) > 0 {
		fmt.Fprintf(&b, "- 资源目录：%s\n", strings.Join(cfg.Include, "、"))
	}
	if len(cfg.ProgramFiles) > 0 {
		fmt.Fprintf(&b, "- 主程序文件：%s\n", strings.Join(cfg.ProgramFiles, "、"))
	}
	b.WriteString("\n")

	b.WriteString(strings.Join(releaseUsage(), "\n"))
	b.WriteString("\n\n")

	b.WriteString("**包内文件与校验**\n")
	for _, f := range packFiles {
		name := filepath.Base(f)
		fmt.Fprintf(&b, "- %s（%.1f MB）\n", name, float64(fileSize(f))/(1024*1024))
		if sum := shas[name]; sum != "" {
			// 保持 "SHA-256: <hex>" 行格式：拉取端按该约定解析校验值
			fmt.Fprintf(&b, "  SHA-256: %s\n", sum)
		}
	}

	return b.String()
}
