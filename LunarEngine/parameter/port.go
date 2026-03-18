package parameter

import "flag"

var (
	// BasicPort 系统Web服务的监听端口，用户可通过此端口访问客户端界面
	BasicPort = flag.Int("basic-port", 36789, "系统Web服务的监听端口，用户可通过此端口访问客户端界面")
	// MaxPort 系统Web服务的最大监听端口，界定了系统Web服务的端口范围
	MaxPort = flag.Int("max-port", *BasicPort+15, "系统Web服务的最大监听端口，界定了系统Web服务的端口范围")
	// MinPort 系统Web服务的最小监听端口，界定了系统Web服务的端口范围
	MinPort = flag.Int("min-port", *BasicPort-5, "系统Web服务的最小监听端口，界定了系统Web服务的端口范围")
	// ClearPort 启动时自动释放被占用的端口
	ClearPort = flag.Bool("clear-port", false, "启动时自动释放被占用的端口")
)
