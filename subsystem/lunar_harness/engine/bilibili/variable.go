package bilibili

// variable.go B 站接口客户端的常量集中管理。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/netsource"
)

const (
	// UserAgent B 站接口要求的桌面端 UA（部分接口会拒绝无 UA / 非浏览器 UA 的请求）。
	UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"
	// Referer B 站接口的默认来源页。
	Referer = "https://www.bilibili.com/"
)

const (
	// viewURL 视频详情
	viewURL = "https://api.bilibili.com/x/web-interface/view?bvid="
	// tagsURL 视频标签
	tagsURL = "https://api.bilibili.com/x/tag/archive/tags?bvid="
	// playURL 播放地址
	playURL = "https://api.bilibili.com/x/player/playurl"
	// commentURL 评论列表
	commentURL = "https://api.bilibili.com/x/v2/reply"
	// replyAddURL 发布评论/回复
	replyAddURL = "https://api.bilibili.com/x/v2/reply/add"
	// replyListURL 根评论下的子评论
	replyListURL = "https://api.bilibili.com/x/v2/reply/reply"
	// dynamicCreateURL 发布动态
	dynamicCreateURL = "https://api.vc.bilibili.com/dynamic_svr/v1/dynamic_svr/create"
	// followTagURL 关注分组
	followTagURL = "https://api.bilibili.com/x/relation/tag"
	// inboxReplyURL 消息中心「回复我的」
	inboxReplyURL = "https://api.bilibili.com/x/msgfeed/reply?platform=web&build=0&mobi_app=web"
	// inboxAtURL 消息中心「@我的」
	inboxAtURL = "https://api.bilibili.com/x/msgfeed/at?platform=web&build=0&mobi_app=web"
	// danmakuURL 弹幕 XML
	danmakuURL = "https://comment.bilibili.com/%d.xml"
)

const (
	// apiTimeout 普通 JSON 接口超时
	apiTimeout = 25 * time.Second
	// danmakuTimeout 弹幕接口超时
	danmakuTimeout = 20 * time.Second
	// commentPageTimeout 页面上下文评论接口超时
	commentPageTimeout = 25 * time.Second
	// replyAddTimeout 发布评论超时
	replyAddTimeout = 25 * time.Second
	// replyVisibleTimeout 回读子评论超时
	replyVisibleTimeout = 20 * time.Second
	// dynamicTimeout 发布动态超时
	dynamicTimeout = 30 * time.Second
	// followTimeout 特别关注读取超时
	followTimeout = 20 * time.Second
	// inboxPageTimeout 消息中心单页超时
	inboxPageTimeout = 25 * time.Second
)

const (
	// maxJSONBytes JSON 响应体读取上限（8MB）
	maxJSONBytes = 8 << 20
	// maxDanmakuBytes 弹幕 XML 读取上限（4MB）
	maxDanmakuBytes = 4 << 20
	// maxCoverBytes 封面下载上限（4MB）
	maxCoverBytes = 4 << 20
)

const (
	// commentPageSize 评论接口默认单页条数
	commentPageSize = 20
	// quickCommentMaxPages 速览评论最多翻页数（高赞榜前三页足够覆盖）
	quickCommentMaxPages = 3
	// commentMessageMaxRunes 速览评论单条硬截断长度
	commentMessageMaxRunes = 512
)

const (
	// specialFollowTagID B 站「特别关注」分组的固定 tagid。
	specialFollowTagID = -10
	// specialFollowPageSize 关注分组单页人数，足够覆盖常见特别关注规模。
	specialFollowPageSize = 50
	// replyVisiblePageSize 子评论回读单页条数
	replyVisiblePageSize = 20
)

const (
	// coverMaxDim 封面送给多模态模型前的长边上限
	coverMaxDim = 480
	// coverJPEGQuality 封面统一转码为 JPEG 的质量
	coverJPEGQuality = 82
)

var (
	// httpTransport B 站直连接口共用的连接池：评论翻页等连续请求不必反复建连。
	// 第二个参数开启代理环境变量跟随，与原生 http.DefaultTransport 的行为一致。
	httpTransport = netsource.NewHTTPTransport(false, true)

	// apiClient 普通 JSON 接口与封面下载共用（25s）。
	apiClient = newBiliClient(httpTransport, apiTimeout)

	// danmakuClient 弹幕 XML 专用（20s），与普通接口共享同一个连接池。
	danmakuClient = newBiliClient(httpTransport, danmakuTimeout)
)
