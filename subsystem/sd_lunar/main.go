package main

import (
	"config"
	"flag"
	"fmt"
	"logger"
	"os"
	"os/signal"
	"sd_lunar/engine"
	"sd_lunar/mempool"
	"sd_lunar/server"
	"syscall"
)

const defaultCLIPPath = "models/clip.safetensors"

func main() {
	flag.Parse()

	logger.SetDevMode(*config.Developer)
	mempool.InitImagePool()
	mempool.InitObjectPool()

	logger.Info("SD-LUNAR", "月华的文生图/图生图模块启动中...")
	logger.Info("SD-LUNAR", "数据流: 纯内存模式，全程无磁盘IO")

	engineParams := engine.EngineParams{
		DiffusionModelPath: *config.DiffusionModel,
		VaePath:            *config.VariationalModel,
		ClipLPath:          defaultCLIPPath,
		NThreads:           4,
		WType:              "f16",
		EnableMmap:         true,
		KeepClipOnCPU:      true,
		KeepVaeOnCPU:       true,
		FlashAttn:          true,
		DiffusionFlashAttn: true,
	}

	logger.Info("SD-LUNAR", "初始化SD引擎...")

	if err := engine.Init(engineParams); err != nil {
		logger.Error("SD-LUNAR", "SD引擎初始化失败: %v", err)
		logger.Info("SD-LUNAR", "将以降级模式运行（仅提供API接口，生成功能需正确配置模型文件）")
	}

	addr := ":36367"
	logger.Info("SD-LUNAR", "HTTP服务监听端口: %s", addr)

	srv := server.NewHTTPServer(addr)
	go func() {
		if err := srv.Start(); err != nil {
			logger.Fatal("SD-LUNAR", "HTTP服务启动失败: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("SD-LUNAR", "正在关闭服务...")
	engine.Release()
	mempool.Reset()
	logger.Info("SD-LUNAR", "已关闭，拜拜~")
	fmt.Println("")
}
