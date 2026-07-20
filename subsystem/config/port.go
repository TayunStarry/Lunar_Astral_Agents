package config

import "flag"

var (
	// BasicPort 系统Web服务的监听端口，用户可通过此端口访问客户端界面
	BasicPort = flag.Int("basic-port", 36789, "系统Web服务的监听端口, 用户可通过此端口访问客户端界面")
	// ModelPort 模型服务的基础端口号，用于分配模型运行端口
	ModelPort = flag.Int("model-port", *BasicPort+1, "模型服务的基础端口号，用于分配模型运行端口")
	// MaxPort 系统Web服务的最大监听端口，界定了系统Web服务的端口范围
	MaxPort = flag.Int("max-port", *BasicPort+15, "系统Web服务的最大监听端口, 边定了系统Web服务的端口范围")
	// MinPort 系统Web服务的最小监听端口，界定了系统Web服务的端口范围
	MinPort = flag.Int("min-port", *BasicPort-5, "系统Web服务的最小监听端口, 边定了系统Web服务的端口范围")
	// ProxyPort 系统Web服务的代理监听口，界定了系统Web服务的端口范围
	ProxyPort = flag.Int("proxy-port", *BasicPort+5, "系统Web服务的代理监听口, 边定了系统Web服务的端口范围")
	// CloudModelUrl 云模型服务的地址，用于云端模型调用等任务
	CloudModelUrl = flag.String("cloud-model-url", "", "云模型服务的地址, 用于云端模型调用等任务")
	// CloudModelKey 云模型服务的密钥，用于认证请求
	CloudModelKey = flag.String("cloud-model-key", "", "云模型服务的密钥, 用于认证请求")
)
