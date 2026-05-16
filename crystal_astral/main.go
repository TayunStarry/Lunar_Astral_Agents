package main

import (
	"config"
	"flag"
	"fmt"
	"math/rand"
)

func main() {
	flag.Parse()
	// 生成10000~40000之间的随机端口
	port := rand.Intn(30001) + 10000
	// 服务名称
	name := "< 星月智能 > 星图.琉璃 在此为您提供服务支持"
	// 引用模型目录
	modelDir := *config.LocalDir + "/models"
	// 引用模板音频
	refAudio := *config.LocalDir + "/audios/lunar-template.wav"
	// 初始化语音合成引擎
	initTTSEngine(modelDir, refAudio)
	// 启动服务
	if err := StartServer(port, Gethierarchy(), name); err != nil {
		fmt.Printf("%s 启动失败: %v\n", name, err)
	}
}
