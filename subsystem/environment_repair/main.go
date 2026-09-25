package main

import (
	"fmt"
	"strings"
)

func main() {
	printBanner()
	RunTUIApp()
}

func printBanner() {
	fmt.Println(strings.Repeat("═", 56))
	fmt.Println("  ✦  星月智能 · 环境修复工具  ✦")
	fmt.Println("  Environment Repair Tool")
	fmt.Println(strings.Repeat("═", 56))
	fmt.Println("正在启动 TUI 界面...")
}
