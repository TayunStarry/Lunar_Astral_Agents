package lunar

// url_digest.go 统一 URL 速览：网页与 B 站视频走同一个入口，按类型分流取数。
//
//   - 网页：WebSearch.ReadURL（浏览器渲染 + 轻量模型摘要）；
//   - B 站：BilibiliViewer.QuickDigest（公开接口聚合元信息 + 轻量模型摘要）。
//
// 速览结果只写 URL 主表：普通网页可直接作为解析文本复用；
// B 站速览只补标题，不写解析文本——正式观看/直驱印象才是该视频唯一解析文本。

import (
	"LunarSubsystem/LunarHarness/engine/resource"
)

// quickDigestURL 按链接类型选择速览实现。
func quickDigestURL(rawURL, ask string) (title, summary string, err error) {
	if resource.BilibiliBvid(rawURL) != "" {
		return bilibiliDigestHook(rawURL, ask)
	}
	return readPageHook(rawURL, ask)
}

// storeURLDigest 把速览写进 URL 主表：普通网页写解析文本，B 站只补标题。
func storeURLDigest(rawURL, title, summary, origin string) {
	if resource.BilibiliBvid(rawURL) != "" {
		_, _ = resource.AdmitURL(resource.URLAdmitInput{
			URL: rawURL, Title: title, Origin: origin,
		})
		return
	}
	_, _ = resource.AdmitURL(resource.URLAdmitInput{
		URL: rawURL, Title: title, Summary: summary, Origin: origin,
	})
}
