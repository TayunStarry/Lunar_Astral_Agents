package netsource

// variable.go 回源与本地资源定位的全局常量

const (
	// LocalFileReadPrefix 本机资源读取路由前缀（与 resource.FileReadPrefix 保持一致）
	LocalFileReadPrefix = "/file/read/"

	// userAgent 回源请求的默认 UA（部分图床 / CDN 会拒绝无 UA 的请求）
	userAgent = "LunarAstral/1.0"

	// maxRedirects 回源允许的最大重定向次数
	maxRedirects = 10
)
