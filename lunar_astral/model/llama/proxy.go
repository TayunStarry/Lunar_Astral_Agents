package llama

import (
	"LunarSubsystem/general_config"
	"LunarSubsystem/general_logger"
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Init 初始化并启动 llama.cpp 服务器
func Init() {
	args := []string{
		// 模型预设配置文件路径
		"--models-preset", "local_data/models/models.ini",
		// 启用Flash Attention注意力机制优化
		"--flash-attn", "on",
		// 温度参数：控制生成文本的随机性
		"--temp", "1.0",
		// 核采样阈值：保留累积概率95%的候选词
		"--top-p", "0.95",
		// 存在惩罚：不惩罚已出现的词
		"--presence-penalty", "0.0",
		// Top-K采样：只考虑概率最高的20个词
		"--top-k", "20",
		// 最小概率阈值：不设置下限
		"--min-p", "0.0",
		// 重复惩罚系数：1.0表示无惩罚
		"--repeat_penalty", "1.0",
		// 服务器监听端口
		"--port", strconv.Itoa(*config.ModelPort),
		// 并行请求处理数
		"--parallel", "1",
		// 批处理大小
		"--batch-size", "2048",
		// 微批处理大小
		"--ubatch-size", "1024",
		// 使用的CPU线程数
		"--threads", strconv.Itoa(runtime.NumCPU() / 2),
		// K缓存量化类型：8位量化
		"--cache-type-k", "q8_0",
		// V缓存量化类型：8位量化
		"--cache-type-v", "q8_0",
		// 不启动UI界面
		"--no-ui",
		// 空闲等待900秒后休眠服务器
		"--sleep-idle-seconds", "900",
	}

	logger.Info("LlamaProxy", "正在启动 llama-server 端口: %d", *config.ModelPort)

	cmd := exec.Command(*config.InferEngine, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logger.Error("LlamaProxy", "创建标准输出管道失败: %v", err)
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		logger.Error("LlamaProxy", "创建标准错误管道失败: %v", err)
		return
	}

	if err := cmd.Start(); err != nil {
		logger.Error("LlamaProxy", "启动 llama-server 失败: %v", err)
		return
	}

	serverProcess = cmd

	go monitorOutput(stdout, stderr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	select {
	case <-serverReady:
		logger.Info("LlamaProxy", "llama-server 已成功启动并准备就绪")
	case <-ctx.Done():
		logger.Error("LlamaProxy", "llama-server 启动超时")
		if err := cmd.Process.Kill(); err != nil {
			logger.Error("LlamaProxy", "终止 llama-server 进程失败: %v", err)
		}
		return
	}

	go waitForProcessExit(cmd)
}

// Close 优雅关闭 llama.cpp 服务器
func Close() {
	if serverProcess == nil {
		return
	}

	logger.Info("LlamaProxy", "正在关闭 llama-server...")

	// 直接 Kill 子进程（Signal(nil) 在 Windows 上不可用，跳过无效的空信号检查）
	if err := serverProcess.Process.Kill(); err != nil {
		logger.Error("LlamaProxy", "终止 llama-server 进程失败: %v", err)
	} else {
		logger.Info("LlamaProxy", "llama-server 已终止")
	}
}

// monitorOutput 监听 llama.cpp 服务器的标准输出和标准错误输出
func monitorOutput(stdout io.ReadCloser, stderr io.ReadCloser) {
	go readOutput(stdout, "STDOUT")
	go readOutput(stderr, "STDERR")
}

// readOutput 读取并处理 llama.cpp 服务器的标准输出和标准错误输出
func readOutput(reader io.ReadCloser, prefix string) {
	defer reader.Close()
	scanner := bufio.NewReader(reader)
	for {
		line, err := scanner.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				logger.Error("LlamaProxy", "[%s] 读取失败: %v", prefix, err)
			}
			return
		}
		line = strings.TrimSpace(line)
		if *config.Developer {
			logger.Info("LlamaProxy", "[%s] %s", prefix, line)
		}
		checkReadySignal(line)
	}
}

// checkReadySignal 检查 llama.cpp 服务器是否已启动并准备就绪
func checkReadySignal(line string) {
	readySignals := []string{
		"starting the main loop",
		"llama server listening",
		"HTTP server listening",
		"server is listening",
		"binding port",
		"srv",
	}
	for _, signal := range readySignals {
		if strings.Contains(line, signal) {
			readyOnce.Do(func() {
				serverReady <- struct{}{}
			})
			return
		}
	}
}

// waitForProcessExit 等待 llama.cpp 服务器进程退出
func waitForProcessExit(cmd *exec.Cmd) {
	if err := cmd.Wait(); err != nil {
		if !strings.Contains(err.Error(), "signal: killed") {
			logger.Error("LlamaProxy", "llama-server 进程异常退出: %v", err)
		}
	} else {
		logger.Info("LlamaProxy", "llama-server 进程正常退出")
	}
}

// ProxyHandler 将所有请求代理到 llama.cpp 服务器
func ProxyHandler(w http.ResponseWriter, r *http.Request) {
	if isEmbeddingRequest(r) {
		proxyToLocal(w, r)
		return
	}
	if *config.AgentMultimodalURL != "" && *config.AgentMultimodalURL != "http://127.0.0.1:36789/v1" {
		ProxyToCloud(w, r)
		return
	}
	proxyToLocal(w, r)
}

// isEmbeddingRequest 检测请求是否为 embedding 模型请求
func isEmbeddingRequest(r *http.Request) bool {
	path := strings.ToLower(r.URL.Path)
	return strings.Contains(path, "/embeddings") || strings.Contains(path, "/embedding")
}

// proxyToLocal 将请求反向代理到本地 llama.cpp 服务器
func proxyToLocal(w http.ResponseWriter, r *http.Request) {
	targetURL := fmt.Sprintf("http://localhost:%d", *config.ModelPort)
	target, err := url.Parse(targetURL)
	if err != nil {
		http.Error(w, "llama[ERROR] -> 解析目标 URL 失败", http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Error("LlamaProxy", "代理错误: %v", err)
		http.Error(w, "llama[ERROR] -> 代理失败", http.StatusBadGateway)
	}

	proxy.ServeHTTP(w, r)
}

// ProxyToCloud 将请求反向代理到云服务器
func ProxyToCloud(w http.ResponseWriter, r *http.Request) {
	target, err := url.Parse(*config.AgentMultimodalURL)
	if err != nil {
		http.Error(w, "GGUF模块[ERROR] -> 解析云服务器 URL 失败", http.StatusInternalServerError)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Error("LlamaProxy", "云代理错误: %v", err)
		http.Error(w, "GGUF模块[ERROR] -> 云代理失败", http.StatusBadGateway)
	}
	// 自定义 Director 函数，保留原始请求路径并注入认证头
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host
		// 注入云模型密钥认证头
		if *config.AgentMultimodalKey != "" {
			req.Header.Set("Authorization", "Bearer "+*config.AgentMultimodalKey)
		}
	}
	logger.Info("LlamaProxy", "云代理: %s %s -> %s%s", r.Method, r.URL.Path, *config.AgentMultimodalURL, r.URL.Path)
	proxy.ServeHTTP(w, r)
}
