package handlers

import (
	"Lunar-Astral-Agents/server/config"  // 导入配置包
	"Lunar-Astral-Agents/server/execute" // 导入执行模块
	"encoding/json"                      // 导入JSON编码/解码包
	"fmt"                                // 导入格式化输出包
	"net/http"                           // 导入HTTP包
	"strings"                            // 导入字符串操作包
	"time"                               // 导入时间包
)

// GenerateHandler 处理图像生成请求
func GenerateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Generate服务 → 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}
	if !*config.AllowDiffusion {
		http.Error(w, "Generate服务 → 灵绘坊功能未启用", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Prompt         string  `json:"prompt"`
		NegativePrompt string  `json:"negative_prompt"`
		BatchSize      int     `json:"batch_size"`
		Width          int     `json:"width"`
		Height         int     `json:"height"`
		Strength       float64 `json:"strength"`
		Steps          int     `json:"steps"`
		Seed           int64   `json:"seed"`
		CfgScale       float64 `json:"cfg_scale"`
		InitImg        string  `json:"init_img"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Generate服务 → 解析JSON失败: %v", err), http.StatusBadRequest)
		return
	}

	if req.Prompt == "" {
		http.Error(w, "Generate服务 → 提示词不能为空", http.StatusBadRequest)
		return
	}

	// 创建任务
	task, queuePos := execute.CreateGenerateTask(
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
	execute.StartTaskProcessor()
}

// GenerateWaitHandler 处理WebSocket连接，等待任务完成
func GenerateWaitHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}
	if !*config.AllowDiffusion {
		http.Error(w, "Generate服务 → 灵绘坊功能未启用", http.StatusServiceUnavailable)
		return
	}

	taskID := r.URL.Query().Get("task_id")
	if taskID == "" {
		http.Error(w, "需要task_id参数", http.StatusBadRequest)
		return
	}

	// 检查任务是否已存在
	existingTask, exists := execute.GetTaskStatus(taskID)
	if !exists {
		http.Error(w, "任务不存在", http.StatusNotFound)
		return
	}

	// 如果任务已完成，直接返回结果
	if existingTask.Status == "completed" || existingTask.Status == "failed" {
		response := map[string]any{
			"task_id": existingTask.ID,
			"status":  existingTask.Status,
			"result":  existingTask.ResultPath,
			"error":   existingTask.Error,
		}
		if existingTask.Status == "completed" {
			// 构建读取路径
			relativePath := strings.TrimPrefix(existingTask.ResultPath, config.LocalDir)
			relativePath = strings.TrimPrefix(relativePath, "\\")
			readPath := "/read/" + relativePath
			response["read_path"] = readPath
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	// 注册客户端
	ch := execute.RegisterWaitClient(taskID)

	// 设置响应头，使用服务器发送事件(SSE)模拟WebSocket
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// 发送一个空的事件来建立连接
	fmt.Fprintf(w, "\n")
	// 刷新响应
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// 等待任务完成，设置超时
	select {
	case completedTask := <-ch:
		// 构建响应
		response := map[string]any{
			"task_id": completedTask.ID,
			"status":  completedTask.Status,
			"result":  completedTask.ResultPath,
			"error":   completedTask.Error,
		}

		if completedTask.Status == "completed" {
			// 构建读取路径
			relativePath := strings.TrimPrefix(completedTask.ResultPath, config.LocalDir)
			relativePath = strings.TrimPrefix(relativePath, "\\")
			readPath := "/read/" + relativePath
			response["read_path"] = readPath
		}

		// 发送响应
		jsonData, _ := json.Marshal(response)
		fmt.Fprintf(w, "data: %s\n\n", jsonData)

		// 刷新响应
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	case <-time.After(5 * time.Minute):
		// 超时
		execute.RemoveWaitClient(taskID)
		http.Error(w, "任务处理超时", http.StatusRequestTimeout)
		return
	}
}
