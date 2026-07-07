package main

import (
	"flag"
	"logger"
	"math/rand"
)

func main() {
	flag.Parse()
	// 备用机制：启动时检查 local_data 目录，从嵌入资源中补全缺失文件
	EnsureLocalData()
	// 生成10000~40000之间的随机端口
	port := rand.Intn(30001) + 10000
	// 服务名称
	name := "< 星月智能 > 星图.琉璃 在此为您提供服务支持"
	// 启动服务
	if err := StartServer(port, Gethierarchy(), name); err != nil {
		logger.Error("CrystalAstral", "%s 启动失败: %v", name, err)
	}
}
