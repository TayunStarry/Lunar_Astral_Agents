package GeneralConfig

import "flag"

var (
	// BasicPort 系统Web服务的监听端口，用户可通过此端口访问客户端界面
	BasicPort = flag.Int("basic-port", 36789, "系统Web服务的监听端口, 用户可通过此端口访问客户端界面")
	// ModelPort 模型服务的基础端口号，用于分配模型运行端口
	ModelPort = flag.Int("model-port", *BasicPort+1, "模型服务的基础端口号，用于分配模型运行端口")
	// EnginePort 引擎服务的基础端口号，用于分配引擎运行端口
	EnginePort = flag.Int("engine-port", *BasicPort+3, "引擎服务的基础端口号，用于分配引擎运行端口")
	// ProxyPort 系统Web服务的代理监听口，界定了系统Web服务的端口范围
	ProxyPort = flag.Int("proxy-port", *BasicPort+5, "系统Web服务的代理监听口, 边定了系统Web服务的端口范围")
)
