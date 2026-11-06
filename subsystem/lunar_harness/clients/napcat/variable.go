package napcat

// variable.go NapCat 传输层的机制参数与默认客户端。

import (
	"regexp"
	"sync"
	"time"
)

const (
	// defaultHTTPTimeout 普通接口超时
	defaultHTTPTimeout = 10 * time.Second
	// uploadHTTPTimeout 文件上传专用超时：大文件上传可能远超普通接口的 10 秒
	uploadHTTPTimeout = 120 * time.Second
	// qqDirectoryTTL 群/好友目录缓存有效期，避免每轮都打两次接口
	qqDirectoryTTL = 30 * time.Second
	// maxMediaDownloadBytes 渠道媒体回源上限（256 MiB）：
	// QQ 图片 / 语音 / 短视频通常远小于此值；超大视频由 get_file 返回本地路径，
	// 不应整段读进内存。
	maxMediaDownloadBytes = 256 * 1024 * 1024
)

// mentionPattern 出站文本中的提及标记：[对 xxx 说]（与入站 @ 的渲染格式对称）
var mentionPattern = regexp.MustCompile(`\[对\s*(.+?)\s*说\]`)

// 默认客户端：由装配层注入真实端点，未注入时为空配置（调用会自然失败）。
var (
	defaultMu     sync.RWMutex
	defaultClient = New(Config{})
)
