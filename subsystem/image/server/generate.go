package server

import (
	"config"
	"encoding/json"
	"fmt"
	image "image/module"
	"net/http"
	"strings"
	"time"
)

// GenerateHandler 处理图像生成请求
func GenerateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Generate服务 → 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}
	if !*config.AllowDiffusion {
		http.Error(w, "Generate服务 → 未启用[扩散生成]功能", http.StatusServiceUnavailable)
		return
	}
	var req GenerateRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Generate服务 → 解析JSON失败: %v", err), http.StatusBadRequest)
		return
	}

	if req.Prompt == "" {
		http.Error(w, "Generate服务 → 提示词不能为空", http.StatusBadRequest)
		return
	}

	// 创建任务
	task, queuePos := image.CreateGenerateTask(
		req.Prompt,
		req.NegativePrompt,
		req.BatchSize,
		req.Width,
		req.Height,
		req.Steps,
		req.Strength,
		req.CfgScale,
		req.Seed,
		req.InitImg,
		req.AllowSuperResolution,
	)

	if task == nil {
		http.Error(w, "Generate服务 → 任务队列已满", http.StatusServiceUnavailable)
		return
	}

	// 任务成功加入队列
	response := map[string]any{
		"status":    "queued",
		"message":   "任务已加入队列",
		"task_id":   task.ID,
		"queue_pos": queuePos,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// StartTaskProcessor 启动任务处理协程
func StartTaskProcessor() {
	image.StartTaskProcessor()
}

// buildReadPath 构建文件读取路径
func buildReadPath(resultPath string) string {
	// 移除本地目录前缀，获取相对路径
	relativePath := strings.TrimPrefix(resultPath, *config.LocalDir)
	// 移除Windows路径开头的反斜杠，确保路径格式统一
	relativePath = strings.TrimPrefix(relativePath, "\\")
	return "/file/read/" + relativePath
}

// buildTaskResponse 构建任务响应
func buildTaskResponse(task *image.GenerateTask) map[string]any {
	response := map[string]any{
		"task_id": task.ID,
		"status":  task.Status,
		"result":  task.ResultPath,
		"error":   task.Error,
	}
	if task.Status == "completed" {
		response["read_path"] = buildReadPath(task.ResultPath)
	}
	return response
}

// setupSSEHeaders 设置SSE响应头
func setupSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
}

// sendSSEEvent 发送SSE事件
func sendSSEEvent(w http.ResponseWriter, data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

// GenerateWaitHandler 处理WebSocket连接，等待任务完成
func GenerateWaitHandler(w http.ResponseWriter, r *http.Request) {
	// 检查请求方法
	if r.Method != "GET" {
		http.Error(w, "不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	// 检查是否允许使用扩散生成
	if !*config.AllowDiffusion {
		http.Error(w, "Generate服务 → 未启用[扩散生成]功能", http.StatusServiceUnavailable)
		return
	}

	// 获取并验证task_id参数
	taskID := r.URL.Query().Get("task_id")
	if taskID == "" {
		http.Error(w, "需要task_id参数", http.StatusBadRequest)
		return
	}

	// 检查任务是否存在
	task, exists := image.GetTaskStatus(taskID)
	if !exists {
		http.Error(w, "任务不存在", http.StatusNotFound)
		return
	}

	// 如果任务已完成，直接返回结果
	if task.Status == "completed" || task.Status == "failed" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(buildTaskResponse(task))
		return
	}

	// 注册客户端等待任务完成
	ch := image.RegisterWaitClient(taskID)

	// 设置SSE响应头
	setupSSEHeaders(w)

	// 发送空事件建立连接
	fmt.Fprintf(w, "\n")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// 等待任务完成或超时
	select {
	case completedTask := <-ch:
		// 发送任务完成响应
		sendSSEEvent(w, buildTaskResponse(completedTask))
	case <-time.After(5 * time.Minute):
		// 超时处理
		image.RemoveWaitClient(taskID)
		http.Error(w, "任务处理超时", http.StatusRequestTimeout)
	}
}
