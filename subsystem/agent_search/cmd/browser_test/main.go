package main

import (
	"fmt"
	"os"

	AgentSearch "LunarSubsystem/AgentSearch"
)

func main() {
	fmt.Println("======== 浏览器启动最小化测试 ========")

	if err := AgentSearch.LaunchBrowser(); err != nil {
		fmt.Printf("[失败] 浏览器启动失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[成功] 浏览器启动成功")
	AgentSearch.CloseBrowser()
}
