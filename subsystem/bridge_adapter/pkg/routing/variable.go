package routing

// routing 包全局变量

import (
	"net/http"
	"time"

	"bridge_adapter/pkg/config"
)

// AIHTTPClient 专用于AI路由调用的HTTP客户端，超时时间较长
var AIHTTPClient = &http.Client{Timeout: time.Duration(config.DefaultAIAPITimeout) * time.Second}
