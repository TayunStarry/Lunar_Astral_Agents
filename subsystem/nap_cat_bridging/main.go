package main

import (
	"log"
	"strings"

	"nap_cat_bridging/internal/core"
)

func main() {
	// 创建应用程序实例
	application, err := core.NewApplication()
	if err != nil {
		log.Fatalf("创建应用程序失败: %v", err)
	}

	// 执行初始化流程
	if err := application.InitProcess(); err != nil {
		log.Fatalf("初始化失败: %v", err)
	}

	log.Printf("%s", strings.Repeat("-=", 28))

	// 执行主循环
	if err := application.MainLoop(); err != nil {
		log.Fatalf("主循环失败: %v", err)
	}
}
