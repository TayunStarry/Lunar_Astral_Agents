package config

import "flag"

var (
	// BasicPort 系统Web服务的监听端口，用户可通过此端口访问客户端界面
	BasicPort = flag.Int("basic-port", 36789, "系统Web服务的监听端口, 用户可通过此端口访问客户端界面")
	// MaxPort 系统Web服务的最大监听端口，界定了系统Web服务的端口范围
	MaxPort = flag.Int("max-port", *BasicPort+15, "系统Web服务的最大监听端口, 边定了系统Web服务的端口范围")
	// MinPort 系统Web服务的最小监听端口，界定了系统Web服务的端口范围
	MinPort = flag.Int("min-port", *BasicPort-5, "系统Web服务的最小监听端口, 边定了系统Web服务的端口范围")
	// ProxyPort 系统Web服务的代理监听口，界定了系统Web服务的端口范围
	ProxyPort = flag.Int("proxy-port", *BasicPort+5, "系统Web服务的代理监听口, 边定了系统Web服务的端口范围")
	// TTSUrl TTS语音服务的地址，用于语音生成等任务
	TTSUrl = flag.String("tts-url", "http://localhost:7860", "TTS语音服务的地址, 用于语音生成等任务")
	// ClearPort 启动时自动释放被占用的端口
	ClearPort = flag.Bool("clear-port", false, "启动时自动释放被占用的端口")
)
