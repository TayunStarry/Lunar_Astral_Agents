package llama_proxy

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
	serverProcess *exec.Cmd
	serverPort    int
	serverReady   = make(chan struct{}, 1)
	readyOnce     sync.Once
)

// Init 初始化并启动 llama.cpp 服务器
func Init() {
	port := *config.ModelPort
	serverPort = port

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
		"--port", strconv.Itoa(port),
		"--reasoning", "off",
	}

	log.Printf("llama_proxy -> 正在启动 llama-server 端口: %d", port)

	cmd := exec.Command(*config.InferEngine, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("llama_proxy[ERROR] -> 创建标准输出管道失败: %v", err)
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("llama_proxy[ERROR] -> 创建标准错误管道失败: %v", err)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("llama_proxy[ERROR] -> 启动 llama-server 失败: %v", err)
		return
	}

	serverProcess = cmd

	go monitorOutput(stdout, stderr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	select {
	case <-serverReady:
		log.Printf("llama_proxy -> llama-server 已成功启动并准备就绪")
	case <-ctx.Done():
		log.Printf("llama_proxy[ERROR] -> llama-server 启动超时")
		if err := cmd.Process.Kill(); err != nil {
			log.Printf("llama_proxy[ERROR] -> 终止 llama-server 进程失败: %v", err)
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

	log.Printf("llama_proxy -> 正在关闭 llama-server...")

	if err := serverProcess.Process.Signal(nil); err == nil {
		if err := serverProcess.Process.Kill(); err != nil {
			log.Printf("llama_proxy[ERROR] -> 终止 llama-server 进程失败: %v", err)
		} else {
			log.Printf("llama_proxy -> llama-server 已终止")
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
				log.Printf("llama_proxy[STDOUT] -> %s", line)
			}
			checkReadySignal(line)
		}
	}()

	go func() {
		for stderrScanner.Scan() {
			line := stderrScanner.Text()
			if *config.Developer {
				log.Printf("llama_proxy[STDERR] -> %s", line)
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
			log.Printf("llama_proxy[ERROR] -> llama-server 进程异常退出: %v", err)
		}
	} else {
		log.Printf("llama_proxy -> llama-server 进程正常退出")
	}
}

// ProxyHandler 将所有请求代理到 llama.cpp 服务器
func ProxyHandler(w http.ResponseWriter, r *http.Request) {
	targetURL := fmt.Sprintf("http://localhost:%d", serverPort)
	target, err := url.Parse(targetURL)
	if err != nil {
		http.Error(w, "llama_proxy[ERROR] -> 解析目标 URL 失败", http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("llama_proxy[ERROR] -> 代理错误: %v", err)
		http.Error(w, "llama_proxy[ERROR] -> 代理失败", http.StatusBadGateway)
	}

	proxy.ServeHTTP(w, r)
}
