package main

import (
	"fmt"
	"lunar_window"
	"math/rand"
)

func main() {
	// 生成10000~40000之间的随机端口
	port := rand.Intn(30001) + 10000
	name := "< 星月智能 > 钛宇.琉璃 在此为您提供服务支持"
	// 启动服务
	if err := lunar_window.StartServer(port, Gethierarchy(), name); err != nil {
		fmt.Printf("%s 启动失败: %v\n", name, err)
	}
}
