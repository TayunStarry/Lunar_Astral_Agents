package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// 命令行参数
	mode := flag.String("mode", "standalone", "运行模式: standalone 或 server")
	addr := flag.String("addr", ":8080", "WebSocket 服务器地址 (仅 server 模式)")
	file := flag.String("file", "", "要执行的 LunarTick 脚本文件")
	tickMs := flag.Int("tick", 100, "Tick 间隔 (毫秒)")
	flag.Parse()

	// 创建解释器
	interp := NewInterpreter()
	interp.SetTickInterval(*tickMs)

	// 根据模式运行
	switch *mode {
	case "server":
		runServerMode(interp, *addr)
	case "standalone":
		runStandaloneMode(interp, *file)
	default:
		fmt.Printf("未知模式: %s\n", *mode)
		flag.Usage()
		os.Exit(1)
	}
}

// runServerMode 服务器模式 - 启动 WebSocket 服务器
func runServerMode(interp *Interpreter, addr string) {
	log.Println("LunarTick WebSocket Server Mode")
	log.Printf("Tick interval: %dms", 100)
	log.Printf("WebSocket address: %s", addr)

	// 创建并启动 WebSocket 服务器
	wsServer := NewWebSocketServer(interp, addr)
	if err := wsServer.Start(); err != nil {
		log.Fatalf("Failed to start WebSocket server: %v", err)
	}
	defer wsServer.Stop()

	// 等待信号
	waitForShutdown(interp, wsServer)
}

// runStandaloneMode 独立模式 - 执行脚本文件
func runStandaloneMode(interp *Interpreter, filename string) {
	log.Println("LunarTick Standalone Mode")

	// 如果提供了文件，加载并执行
	if filename != "" {
		content, err := os.ReadFile(filename)
		if err != nil {
			log.Fatalf("Failed to read file: %v", err)
		}
		
		interp.LoadMarkdown(string(content))
		log.Printf("Loaded script from: %s", filename)
	} else {
		// 默认示例
		log.Println("No script file provided. Running example...")
		exampleCode := `
` + "```LunarTick" + `
@log "Hello, LunarTick!"
SET counter "0"
@lazy *increment
@math counter #counter + 1
@log "Counter: #counter"
*increment
@sleep 1000
@stop
` + "```" + `
`
		interp.LoadMarkdown(exampleCode)
	}

	// 启动解释器
	interp.Start()
	defer interp.Stop()

	// 等待信号
	waitForShutdown(interp, nil)
}

// waitForShutdown 等待关闭信号
func waitForShutdown(interp *Interpreter, wsServer *WebSocketServer) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	log.Println("\nShutting down...")

	if wsServer != nil {
		wsServer.Stop()
	}
	interp.Stop()

	log.Println("Goodbye!")
}
