package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"lunartick/api"
	"lunartick/engine"

	"logger"
)

var (
	apiPort  = flag.Int("api-port", 36800, "HTTP API 服务端口")
	tickMS   = flag.Int("tick-ms", 100, "Tick 间隔（毫秒）")
	devMode  = flag.Bool("developer", false, "启用开发者模式（详细日志）")
	loadFile = flag.String("load", "", "启动时加载的脚本文件路径")
)

func main() {
	flag.Parse()

	logger.SetDevMode(*devMode)

	logger.Info("LunarTick", "══════════════════════════════════")
	logger.Info("LunarTick", "  LunarTick 通用程序执行引擎 v5.0")
	logger.Info("LunarTick", "  Tick 间隔: %d ms", *tickMS)
	logger.Info("LunarTick", "  API 端口: %d", *apiPort)
	logger.Info("LunarTick", "══════════════════════════════════")

	eng := engine.NewEngine(time.Duration(*tickMS) * time.Millisecond)

	eng.SetLogFn(func(msg string) {
		logger.Info("LunarTick", "%s", msg)
	})

	if *loadFile != "" {
		loadScriptFile(eng, *loadFile)
	}

	srv := api.NewServer(eng, *apiPort)
	srv.Start()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("LunarTick", "正在关闭...")
	srv.Shutdown()
	logger.Info("LunarTick", "已安全关闭")
}

func loadScriptFile(eng *engine.Engine, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		logger.Error("LunarTick", "加载脚本文件失败: %v", err)
		return
	}

	content := string(data)
	eng.LoadMarkdown(content)
	logger.Info("LunarTick", "已加载脚本: %s (%d 字节)", path, len(content))
}

func init() {
	fmt.Println()
}
