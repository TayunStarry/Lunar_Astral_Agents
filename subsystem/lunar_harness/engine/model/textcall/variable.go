package textcall

import (
	"net/http"
	"time"
)

const (
	// defaultTimeout 单次文本调用超时；调用方带更短 deadline 时以调用方为准。
	defaultTimeout = 5 * time.Minute
	// maxErrorBody 错误响应最多保留的字符数
	maxErrorBody = 300
)

var httpClient = &http.Client{Timeout: defaultTimeout}
