package main

import (
	"flag"
	"fmt"
	"math/rand"
)

func main() {
	flag.Parse()
	// 生成10000~40000之间的随机端口
	port := rand.Intn(30001) + 10000
	name := "< 星月智能 > 星图.琉璃 在此为您提供服务支持"
	// 启动服务
	if err := StartServer(port, Gethierarchy(), name); err != nil {
		fmt.Printf("%s 启动失败: %v\n", name, err)
	}
}
