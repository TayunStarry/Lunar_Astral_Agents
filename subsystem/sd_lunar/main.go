package main

import (
	"flag"
	"logger"
	"math/rand"
)

func main() {
	flag.Parse()
	port := rand.Intn(30001) + 10000
	name := "< 星月智能 > SD_Lunar 图像生成测试系统 为您提供服务"
	if err := StartServer(port, Gethierarchy(), name); err != nil {
		logger.Error("sd_lunar", "%s 启动失败: %v", name, err)
	}
}
