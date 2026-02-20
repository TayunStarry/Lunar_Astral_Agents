package server

// 导入必要的包
import (
	"Lunar-Astral-Agents/server/config"   // 项目配置相关包
	"Lunar-Astral-Agents/server/handlers" // 处理API请求的包
	"Lunar-Astral-Agents/server/llama"    // GGUF相关包
	"Lunar-Astral-Agents/server/release"  // 端口释放相关包
	"context"                             // 用于处理请求上下文和超时
	"encoding/json"                       // 用于JSON编码/解码
	"flag"                                // 用于解析命令行参数
	"log"                                 // 用于日志记录
	"mime"                                // 用于处理MIME类型
	"net/http"                            // 用于构建HTTP服务器
	"os"                                  // 用于操作系统相关操作
	"os/signal"                           // 用于处理系统信号
	"path/filepath"                       // 用于处理文件路径
	"strings"                             // 用于字符串操作
	"syscall"                             // 用于系统调用
	"time"                                // 用于时间相关操作
)

// httpMux 是HTTP服务器的ServeMux实例
var httpMux *http.ServeMux

type ModelConfig struct {
	// 嵌入模型路径
	EmbeddingModelPath string `json:"embedding_model_path"`
	// 多模态模型路径
	MultimodalModelPath string `json:"multimodal_model_path"`
	// 多模态投影模型路径
	MmprojModelPath string `json:"mmproj_model_path"`
	// 扩散模型路径
	DiffusionModelPath string `json:"diffusion_model_path"`
	// 变分模型路径
	VariationalModelPath string `json:"variational_model_path"`
	// 提示精炼模型路径
	PromptRefineModelPath string `json:"prompt_refine_model_path"`
}

// InitializeServer 初始化服务器配置和组件
func InitializeServer() {
	// 解析命令行参数
	flag.Parse()
	// 加载配置文件
	loadConfigureFile()
	// 如果指定了端口释放选项，则执行端口释放
	if *config.ClearPort {
		release.ExecutePortRelease()
	}
	// 设置MIME类型映射
	for ext, mimeType := range config.MimeMap {
		mime.AddExtensionType(ext, mimeType)
	}
	// 创建本地目录
	if err := os.MkdirAll(config.LocalDir, 0755); err != nil {
		log.Fatalf("Lunar模块[ERROR] -> %v", err)
	}
	// 注册HTTP处理器
	registerHandlers()
	// 创建GGUF服务器
	llama.CreateServers()
}

// loadConfigureFile 加载配置文件
func loadConfigureFile() {
	// 获取当前可执行文件的路径
	exePath, err := os.Executable()
	// 若获取失败，打印错误日志并直接返回
	if err != nil {
		log.Printf("获取可执行文件路径失败: %v", err)
		return
	}
	// 提取可执行文件所在的目录
	exeDir := filepath.Dir(exePath)
	// 拼接配置文件 model_config.json 的完整路径
	configPath := filepath.Join(exeDir, config.LocalDir, "model_config.json")
	// 读取配置文件内容
	data, err := os.ReadFile(configPath)
	if err != nil {
		// 若读取失败，打印错误日志并直接返回
		log.Printf("读取配置文件失败 %s: %v", configPath, err)
		return
	}
	// 创建 ModelConfig 结构体实例用于接收解析结果
	parameter := &ModelConfig{}
	// 将 JSON 数据解析到结构体中
	if err := json.Unmarshal(data, parameter); err != nil {
		// 若解析失败，打印错误日志并直接返回
		log.Printf("解析配置文件失败: %v", err)
		return
	}
	// 如果配置文件中 EmbeddingModelPath 字段非空，则更新全局配置
	if parameter.EmbeddingModelPath != "" {
		*config.EmbeddingModel = parameter.EmbeddingModelPath
	}
	// 如果配置文件中 MultimodalModelPath 字段非空，则更新全局配置
	if parameter.MultimodalModelPath != "" {
		*config.MultimodalModel = parameter.MultimodalModelPath
	}
	// 如果配置文件中 MmprojModelPath 字段非空，则更新全局配置
	if parameter.MmprojModelPath != "" {
		*config.MmprojModel = parameter.MmprojModelPath
	}
	// 如果配置文件中 DiffusionModelPath 字段非空，则更新全局配置
	if parameter.DiffusionModelPath != "" {
		*config.DiffusionModel = parameter.DiffusionModelPath
	}
	// 如果配置文件中 VariationalModelPath 字段非空，则更新全局配置
	if parameter.VariationalModelPath != "" {
		*config.VariationalModel = parameter.VariationalModelPath
	}
	// 如果配置文件中 PromptRefineModelPath 字段非空，则更新全局配置
	if parameter.PromptRefineModelPath != "" {
		*config.PromptModel = parameter.PromptRefineModelPath
	}
}

// registerHandlers 注册所有HTTP请求处理器
func registerHandlers() {
	// 创建独立的ServeMux实例
	httpMux = http.NewServeMux()
	// 处理根路径请求，将请求路径中的前缀 "/" 去除后，使用文件服务器提供 Webpage 目录下的静态文件
	httpMux.Handle("/", http.StripPrefix("/", http.FileServer(http.Dir("webpage"))))
	// 检查显存是否足够，若不足则禁用灵绘坊功能
	if mem, err := llama.GetFreeMemory(); err == nil && mem < 8*1024*1024*1024 {
		log.Printf("Generate服务[WARN] -> 可用显存低于8GB, 请慎用[ 灵绘坊 ]功能")
	}
	// 启动灵绘坊任务协处理器
	handlers.StartTaskProcessor()
	// 注册所有系统端点路径的处理函数
	for _, endpoint := range SystemEndpoints {
		httpMux.HandleFunc(endpoint.Path, endpoint.Handler)
	}
}

// SetupSignalHandling 设置系统信号处理
func SetupSignalHandling() chan os.Signal {
	// 创建一个用于接收系统信号的通道，缓冲区大小为 1
	quit := make(chan os.Signal, 1)
	// 监听 SIGINT 和 SIGTERM 信号，当接收到这些信号时，将信号发送到 quit 通道
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	return quit
}

// WaitForShutdown 等待关闭信号并优雅关闭服务器
func WaitForShutdown(quit chan os.Signal, server *http.Server) {
	// 阻塞等待系统信号，当接收到信号时继续执行后续代码
	<-quit
	// 执行服务器关闭流程
	shutdownServer(server)
}

// shutdownServer 优雅关闭服务器
func shutdownServer(server *http.Server) {
	// 打印分隔符
	log.Printf("%s", strings.Repeat("-=", 28))
	// 打印服务器正在关闭的信息
	log.Println("Lunar模块 -> 正在关闭...")
	// 创建一个带有 5 秒超时的上下文，用于控制服务器关闭的时间
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	// 确保在函数结束时取消上下文，释放资源
	defer cancel()
	// 关闭WebSocket服务器
	CloseWebSocketServer()
	// 优雅地关闭服务器，等待所有活跃连接处理完成或超时
	if err := server.Shutdown(ctx); err != nil {
		// 如果关闭服务器时出错，打印错误信息并终止程序
		log.Fatalf("Lunar模块[ERROR] -> %v", err)
	}
	// 打印服务器已安全关闭的信息
	log.Println("Lunar模块 -> 已安全关闭")
}
