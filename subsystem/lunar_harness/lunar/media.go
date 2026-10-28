package lunar

// media.go 媒体来源的通用小工具（仅保留命名推断，资源化统一走 ResourcesConversionHandle）

import "strings"

// mediaName 按来源推断文件名
func mediaName(source, fallbackExt string) string {
	lower := strings.ToLower(source)
	if strings.HasPrefix(lower, "data:image/") || strings.HasPrefix(lower, "data:video/") || strings.HasPrefix(lower, "data:audio/") {
		head := lower[len("data:"):]
		if cut := strings.IndexAny(head, ";,"); cut >= 0 {
			head = head[:cut]
		}
		if cut := strings.LastIndex(head, "/"); cut >= 0 {
			ext := head[cut+1:]
			if ext == "jpeg" {
				ext = "jpg"
			}
			return "media." + ext
		}
	}
	path := pathPartOf(lower)
	base := path[strings.LastIndex(path, "/")+1:]
	if base != "" && strings.LastIndex(base, ".") > 0 {
		return base
	}
	return "media" + fallbackExt
}
