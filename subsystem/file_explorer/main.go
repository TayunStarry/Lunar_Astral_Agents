package main

import (
	"fmt"
	"math/rand"
	lunar_window "lunar_window"
	"subsystem/component"
)

func main() {
	// 生成10000~40000之间的随机端口	
	port := rand.Intn(30001) + 10000
	name := "< 星月智能-蔷薇 > (文件管理)"
	// 启动服务
	if err := lunar_window.StartServer(port, component.Gethierarchy(), name); err != nil {
		fmt.Printf("%s 启动失败: %v\n", name, err)
	}
}
