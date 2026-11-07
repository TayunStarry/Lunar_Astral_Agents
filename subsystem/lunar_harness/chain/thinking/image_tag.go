package thinking

import (
	"net/url"
	"regexp"
	"strings"

	"LunarSubsystem/LunarHarness/engine/mediaobject"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// 视觉读取 URL 的句柄参数
var tagParamRegex = regexp.MustCompile(`[?&]tag=([^&]+)`)

// imageNameFor 由统一分类入口给出图片文件名：类型与扩展名都不在消费侧按来源猜。
//
// 来源声明了 MIME 时交给分类层用 MIME 归一（data URI 头部或调用方传入）；
// 否则用调用方声明的图片类型配合来源扩展名。扩展名缺失时保持为空，
// 由入库阶段的分类器按文件头补齐。
func imageNameFor(source, kindHint, mimeType string) string {
	mimeType = strings.TrimSpace(mimeType)
	if mimeType == "" {
		mimeType = mediaobject.MIMETypeOfDataURI(source)
	}
	in := resource.ClassifyInput{
		SourceKind: classifySourceKind(source),
		Source:     source,
	}
	if mimeType != "" {
		in.MIME = mimeType
	} else {
		in.KindHint = kindHint
	}
	classified := resource.Classify(in)
	return "image" + classified.Ext
}

// handleFromVisionURL 从视觉读取 URL 的 tag 参数解析句柄
func handleFromVisionURL(source string) string {
	match := tagParamRegex.FindStringSubmatch(source)
	if match == nil {
		return ""
	}
	tag := match[1]
	if decoded, err := url.QueryUnescape(tag); err == nil {
		tag = decoded
	}
	if !visionTagPattern.MatchString(tag) {
		return ""
	}
	return tag
}
