package server

// 导入必要的包
import (
	"Lunar-Astral-Agents/server/config"    // 项目配置相关包
	"Lunar-Astral-Agents/server/utils"     // 工具函数包
	"Lunar-Astral-Agents/server/websocket" // WebSocket相关包
	"fmt"                                  // 用于格式化输入输出
	"log"                                  // 用于日志记录
	"net/http"                             // 用于构建HTTP服务器
	"strings"                              // 用于字符串操作
)

// 全局变量
var websocketServer *http.Server

// StartServer 启动HTTP服务器
func StartServer() *http.Server {
	// 创建一个新的 HTTP 服务器实例
	server := &http.Server{}
	// 启动服务器监听
	go startServerListener(server)
	return server
}

// startServerListener 启动服务器监听循环
func startServerListener(server *http.Server) {
	// 打印启动服务器的日志信息
	log.Printf("%s", strings.Repeat("-=", 28))
	// 定义最大尝试次数
	const maxAttempts = 10
	// 尝试启动服务器
	if !attemptServerStart(server, maxAttempts) {
		log.Fatalf("Lunar模块[ERROR] -> 无可用端口")
	}
}

// attemptServerStart 尝试启动服务器，最多尝试指定次数
func attemptServerStart(server *http.Server, maxAttempts int) bool {
	for i := 0; i < maxAttempts; i++ {
		if tryStartServerOnPort(server) {
			return true
		}
		*config.BasicPort++
	}
	return false
}

// tryStartServerOnPort 尝试在指定端口上启动服务器
func tryStartServerOnPort(server *http.Server) bool {
	// 服务器成功启动后的初始化工作
	initializeServerComponents(server)
	// 配置服务器监听地址
	addr := fmt.Sprintf(":%d", *config.BasicPort)
	// 启动HTTPS服务器 - 使用TLS证书提供安全访问
	if err := http.ListenAndServeTLS(addr, *config.CertFile, *config.KeyFile, server.Handler); err != nil && err != http.ErrServerClosed {
		log.Printf("Lunar模块[ERROR] -> %v", err)
		return false
	}
	// 返回成功启动服务器
	return true
}

// initializeServerComponents 初始化服务器组件
func initializeServerComponents(server *http.Server) {
	// 为服务器添加 CORS 中间件
	server.Handler = utils.CORSMiddleware(httpMux)
	// 启动客户端加载任务
	go startClientLoading()
	// 构建模拟服务器并保存实例
	websocketServer = websocket.BuildSimulatedServer()
}

// startClientLoading 启动客户端加载任务
func startClientLoading() {
	// 获取本地 IP 地址
	ip, err := utils.GetLocalIP([]string{})
	// 处理获取 IP 地址失败的情况
	if err != nil {
		log.Printf("Lunar模块[ERROR] -> %v\n", err)
		return
	}
	// 构建客户端访问的 URL
	//clientUrl := fmt.Sprintf("https://localhost:%d", *config.BasicPort)
	// 构建内部接口的 URL
	internalURL := fmt.Sprintf("https://%s:%d", ip, *config.BasicPort)
	// 检查是否非开发模式，如果不是开发模式，则自动打开浏览器访问服务器
	if !*config.DevMode {
		utils.OpenBrowser(internalURL)
	}
	// 打印服务器端口
	PrintServerPort(internalURL)
}

// CloseWebSocketServer 关闭WebSocket服务器
func CloseWebSocketServer() {
	if websocketServer != nil {
		log.Printf("Lunar模块[WebSocket] -> 关闭服务器")
		websocketServer.Close()
		websocketServer = nil
	}
}

// PrintServerPort 打印服务器端口
func PrintServerPort(internalURL string) {
	log.Printf("%s", strings.Repeat("-=", 28))
	log.Printf("Lunar模块 : 前端文件 [POST] -> %v/", internalURL)
	log.Printf("Lunar模块 : 文件保存 [POST] -> %v/save", internalURL)
	log.Printf("Lunar模块 : 文件读取 [GET] -> %v/read", internalURL)
	log.Printf("Lunar模块 : 文件列表 [POST] -> %v/file_list", internalURL)
	log.Printf("Lunar模块 : 文件下载 [GET] -> %v/download", internalURL)
	log.Printf("Lunar模块 : 文件删除 [DELETE] -> %v/delete", internalURL)
	log.Printf("Lunar模块 : 文件归档 [POST] -> %v/archive", internalURL)
	log.Printf("Lunar模块 : 模型列表 [GET] -> %v/v1/models", internalURL)
	log.Printf("Lunar模块 : 模型交互 [POST] -> %v/v1/completions", internalURL)
	log.Printf("Lunar模块 : 绘图状态 [GET] -> %v/generate/status", internalURL)
	log.Printf("Lunar模块 : 图片生成 [POST] -> %v/generate", internalURL)
	log.Printf("Lunar模块 : 知识查询 [POST] -> %v/knowledge/query", internalURL)
	log.Printf("Lunar模块 : 知识写入 [POST] -> %v/knowledge/write", internalURL)
	log.Printf("Lunar模块 : 知识刷新 [POST] -> %v/knowledge/flush", internalURL)
	log.Printf("Lunar模块 : 知识删除 [POST] -> %v/knowledge/delete", internalURL)
	log.Printf("Lunar模块 : 知识列表 [GET] -> %v/knowledge/list", internalURL)
	log.Printf("Lunar模块 : 通用截图 [POST] -> %v/capture", internalURL)
	log.Printf("Lunar模块 : 屏幕截图 [GET] -> %v/capture/display", internalURL)
	log.Printf("Lunar模块 : 区域截图 [POST] -> %v/capture/region", internalURL)
	log.Printf("Lunar模块 : 屏幕列表 [GET] -> %v/capture/displays", internalURL)
	log.Printf("Lunar模块 : 视频切片 [POST] -> %v/extract/keyframes", internalURL)
	log.Printf("Lunar模块 : 视频首帧 [POST] -> %v/extract/firstframe", internalURL)
	log.Printf("Lunar模块 : 数据管理 [POST] -> %v/database", internalURL)
}
