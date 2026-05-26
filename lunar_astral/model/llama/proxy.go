package llama

import (
	"bufio"
	"config"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	// serverProcess 是 llama.cpp 服务器的进程 它负责处理模型推理和生成文本
	serverProcess *exec.Cmd
	// serverReady 是一个通道 用于通知调用方 llama.cpp 服务器已启动并准备就绪
	serverReady = make(chan struct{}, 1)
	// readyOnce 是一个 sync.Once 实例 用于确保 llama.cpp 服务器只启动一次
	readyOnce sync.Once
)

// Init 初始化并启动 llama.cpp 服务器
func Init() {
	// 判断是否在配置中允许加载多模态模型
	if *config.AllowMultimodal == false {
		return
	}

	args := []string{
		"--models-preset", "local_data/models/models.ini",
		"--spec-type", "draft-mtp",
		"--spec-draft-n-max", "5",
		"--n-gpu-layers", "999",
		"--flash-attn", "on",
		"--ctx-size", "16384",
		"--temp", "0.6",
		"--top-p", "0.95",
		"--presence-penalty", "0.0",
		"--top-k", "20",
		"--min-p", "0.0",
		"--repeat_penalty", "1.0",
		"--port", strconv.Itoa(*config.ModelPort),
		"--reasoning", "off",
	}

	log.Printf("llama -> 正在启动 llama-server 端口: %d", *config.ModelPort)

	cmd := exec.Command(*config.InferEngine, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("llama[ERROR] -> 创建标准输出管道失败: %v", err)
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("llama[ERROR] -> 创建标准错误管道失败: %v", err)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("llama[ERROR] -> 启动 llama-server 失败: %v", err)
		return
	}

	serverProcess = cmd

	go monitorOutput(stdout, stderr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	select {
	case <-serverReady:
		log.Printf("llama -> llama-server 已成功启动并准备就绪")
	case <-ctx.Done():
		log.Printf("llama[ERROR] -> llama-server 启动超时")
		if err := cmd.Process.Kill(); err != nil {
			log.Printf("llama[ERROR] -> 终止 llama-server 进程失败: %v", err)
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

	log.Printf("llama -> 正在关闭 llama-server...")

	if err := serverProcess.Process.Signal(nil); err == nil {
		if err := serverProcess.Process.Kill(); err != nil {
			log.Printf("llama[ERROR] -> 终止 llama-server 进程失败: %v", err)
		} else {
			log.Printf("llama -> llama-server 已终止")
		}
	}
}

func monitorOutput(stdout io.ReadCloser, stderr io.ReadCloser) {
	stdoutScanner := bufio.NewScanner(stdout)
	stderrScanner := bufio.NewScanner(stderr)

	go func() {
		for stdoutScanner.Scan() {
			line := stdoutScanner.Text()
			if *config.Developer {
				log.Printf("llama[STDOUT] -> %s", line)
			}
			checkReadySignal(line)
		}
	}()

	go func() {
		for stderrScanner.Scan() {
			line := stderrScanner.Text()
			if *config.Developer {
				log.Printf("llama[STDERR] -> %s", line)
			}
			checkReadySignal(line)
		}
	}()
}

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

func waitForProcessExit(cmd *exec.Cmd) {
	if err := cmd.Wait(); err != nil {
		if !strings.Contains(err.Error(), "signal: killed") {
			log.Printf("llama[ERROR] -> llama-server 进程异常退出: %v", err)
		}
	} else {
		log.Printf("llama -> llama-server 进程正常退出")
	}
}

// ProxyHandler 将所有请求代理到 llama.cpp 服务器
func ProxyHandler(w http.ResponseWriter, r *http.Request) {
	if *config.CloudModelUrl != "" {
		ProxyToCloud(w, r)
		return
	}
	targetURL := fmt.Sprintf("http://localhost:%d", *config.ModelPort)
	target, err := url.Parse(targetURL)
	if err != nil {
		http.Error(w, "llama[ERROR] -> 解析目标 URL 失败", http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("llama[ERROR] -> 代理错误: %v", err)
		http.Error(w, "llama[ERROR] -> 代理失败", http.StatusBadGateway)
	}

	proxy.ServeHTTP(w, r)
}

// ProxyToCloud 将请求反向代理到云服务器
func ProxyToCloud(w http.ResponseWriter, r *http.Request) {
	target, err := url.Parse(*config.CloudModelUrl + "/chat/completions")
	if err != nil {
		http.Error(w, "GGUF模块[ERROR] -> 解析云服务器 URL 失败", http.StatusInternalServerError)
		return
	}

	log.Printf("llama -> 云服务器 URL: %s", target.String())

	// 打印消息头中的密钥信息
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		log.Printf("llama -> Authorization 头: %s", authHeader)
	}
	if apiKey := r.Header.Get("x-api-key"); apiKey != "" {
		log.Printf("llama -> x-api-key 头: %s", apiKey)
	}
	if apiKey := r.Header.Get("api-key"); apiKey != "" {
		log.Printf("llama -> api-key 头: %s", apiKey)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	// 自定义 Director 函数，强制将所有请求重定向到 /chat/completions
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = target.Path
		req.URL.RawPath = ""
		req.URL.RawQuery = ""
		req.Host = target.Host
	}

	proxy.ServeHTTP(w, r)
}
