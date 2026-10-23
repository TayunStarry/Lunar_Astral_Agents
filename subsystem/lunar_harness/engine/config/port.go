package config

import (
	"flag"
	"fmt"
)

var (
	// BasicPort 系统Web服务的监听端口，用户可通过此端口访问客户端界面
	BasicPort = flag.Int("basic-port", 36789, "系统Web服务的监听端口, 用户可通过此端口访问客户端界面")
	// ModelPort 模型服务的基础端口号，用于分配模型运行端口
	ModelPort = flag.Int("model-port", *BasicPort+1, "模型服务的基础端口号，用于分配模型运行端口")
	// ProxyPort 系统Web服务的代理监听口，界定了系统Web服务的端口范围
	ProxyPort = flag.Int("proxy-port", *BasicPort+5, "系统Web服务的代理监听口, 边定了系统Web服务的端口范围")
)

// LocalV1URL 本机 OpenAI 兼容接口基址（同源 /v1，端口随 BasicPort 配置变化）。
// 供内部调用方构造模型请求地址，避免在业务代码中硬编码端口。
func LocalV1URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/v1", *BasicPort)
}

// LocalBaseURL 本机 Web 服务根地址（端口随 BasicPort 配置变化）
func LocalBaseURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", *BasicPort)
}
