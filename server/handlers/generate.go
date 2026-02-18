package handlers

import (
	"Lunar-Astral-Agents/server/config" // 导入配置包
	"encoding/json"                     // 导入JSON编码/解码包
	"fmt"                               // 导入格式化输出包
	"log"                               // 导入日志包
	"net/http"                          // 导入HTTP包
	"os"                                // 导入操作系统包
	"os/exec"                           // 导入执行外部命令包
	"path/filepath"                     // 导入文件路径操作包
	"strings"                           // 导入字符串操作包
	"sync"                              // 导入同步包
	"time"                              // 导入时间包
)

// 生成任务结构体
type GenerateTask struct {
	ID             string              `json:"id"`
	Prompt         string              `json:"prompt"`
	NegativePrompt string              `json:"negative_prompt"`
	BatchSize      int                 `json:"batch_size"`
	Width          int                 `json:"width"`
	Height         int                 `json:"height"`
	Strength       float64             `json:"strength"`
	Steps          int                 `json:"steps"`
	Seed           int64               `json:"seed"`
	CfgScale       float64             `json:"cfg_scale"`
	InitImg        string              `json:"init_img"`
	ResponseWriter http.ResponseWriter `json:"-"`
	Request        *http.Request       `json:"-"`
	CreatedAt      time.Time           `json:"created_at"`
	Status         string              `json:"status"`
	ResultPath     string              `json:"result_path"`
	Error          string              `json:"error"`
}

var (
	taskQueue     = make(chan GenerateTask, 10)
	taskStatus    = make(map[string]*GenerateTask)
	taskStatusMu  sync.RWMutex
	waitClients   = make(map[string]chan *GenerateTask)
	waitClientsMu sync.RWMutex
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

	// 创建任务ID
	taskID := fmt.Sprintf("task_%d", time.Now().UnixNano())
	task := GenerateTask{
		ID:             taskID,
		Prompt:         req.Prompt,
		NegativePrompt: req.NegativePrompt,
		BatchSize:      req.BatchSize,
		Width:          req.Width,
		Height:         req.Height,
		Strength:       req.Strength,
		Steps:          req.Steps,
		Seed:           req.Seed,
		CfgScale:       req.CfgScale,
		InitImg:        req.InitImg,
		ResponseWriter: w,
		Request:        r,
		CreatedAt:      time.Now(),
		Status:         "queued",
	}

	// 存储任务状态
	taskStatusMu.Lock()
	taskStatus[taskID] = &task
	taskStatusMu.Unlock()

	// 将任务加入队列
	select {
	case taskQueue <- task:
		// 任务成功加入队列
		response := map[string]any{
			"status":    "queued",
			"message":   "任务已加入队列",
			"task_id":   taskID,
			"queue_pos": len(taskQueue),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	default:
		// 队列已满
		http.Error(w, "Generate服务 → 任务队列已满", http.StatusServiceUnavailable)
	}
}

// 任务处理协程
func StartTaskProcessor() {
	go func() {
		for task := range taskQueue {
			processTask(task)
		}
	}()
}

// 处理单个任务
func processTask(task GenerateTask) {
	taskID := task.ID

	// 更新任务状态为运行中
	taskStatusMu.Lock()
	task.Status = "running"
	taskStatus[taskID] = &task
	taskStatusMu.Unlock()

	// 打印任务开始分隔线
	log.Printf("%s", strings.Repeat("-=", 28))
	log.Printf("开始处理任务: %s", taskID)

	// 构建输出文件名
	timestamp := time.Now().Format("20060102_150405")
	outputFilename := fmt.Sprintf("generated_%s.png", timestamp)
	outputPath := filepath.Join(config.LocalDir, "generated", outputFilename)

	// 确保输出目录存在
	os.MkdirAll(filepath.Join(config.LocalDir, "generated"), 0755)

	// 构建命令参数
	args := []string{
		"--diffusion-model", *config.DiffusionModel,
		"--vae", *config.VariationalModel,
		"--llm", *config.PromptModel,
		"--diffusion-fa",
		"--vae-tiling",
		"--cfg-scale", fmt.Sprintf("%.2f", task.CfgScale),
		"--steps", fmt.Sprintf("%d", task.Steps),
		"-H", fmt.Sprintf("%d", task.Height),
		"-W", fmt.Sprintf("%d", task.Width),
		"-o", outputPath,
		"-p", task.Prompt,
	}

	// 添加负面提示词
	if task.NegativePrompt != "" {
		args = append(args, "-n", task.NegativePrompt)
	}

	// 图生图参数
	if task.InitImg != "" && task.InitImg != "null" {
		initImgPath := filepath.Join(config.LocalDir, task.InitImg)
		if _, err := os.Stat(initImgPath); err == nil {
			args = append(args, "--init-img", initImgPath)
			args = append(args, "--strength", fmt.Sprintf("%.2f", task.Strength))
		}
	}

	// 随机数种子
	if task.Seed != 0 {
		args = append(args, "--seed", fmt.Sprintf("%d", task.Seed))
	}
	// 批处理数量
	if task.BatchSize > 1 {
		args = append(args, "-b", fmt.Sprintf("%d", task.BatchSize))
	}
	// 多模态提示词模型
	if *config.PromptMmprojModel != "" {
		args = append(args, "--llm_vision", *config.PromptMmprojModel)
	}

	// 显示命令参数，正确分组
	log.Printf("执行命令参数:")
	log.Printf("  程序: %s", *config.VisualEngine)

	// 正确分组显示参数
	for i := 0; i < len(args); i++ {
		current := args[i]

		// 检查是否是带值的参数
		isValueParam := false
		value := ""

		if strings.HasPrefix(current, "-") || strings.HasPrefix(current, "--") {
			// 这是一个参数
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") && !strings.HasPrefix(args[i+1], "--") {
				// 下一个不是参数，是值
				value = args[i+1]
				i++ // 跳过值
				isValueParam = true
			}
		}

		if isValueParam {
			log.Printf("  参数: %s %s", current, value)
		} else {
			log.Printf("  参数: %s", current)
		}
	}

	log.Printf("%s", strings.Repeat("-=", 28))

	// 执行命令
	cmd := exec.Command(*config.VisualEngine, args...)

	// 捕获标准输出和错误输出
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		log.Printf("任务[%s]执行失败: %v", taskID, err)
		taskStatusMu.Lock()
		task.Status = "failed"
		task.Error = err.Error()
		taskStatus[taskID] = &task
		taskStatusMu.Unlock()
		return
	}

	// 创建一个更简单的输出处理器
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				output := string(buf[:n])
				// 直接输出，让终端处理格式化
				fmt.Print(output)
			}
			if err != nil {
				break
			}
		}
	}()

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				output := string(buf[:n])
				// 直接输出，让终端处理格式化
				fmt.Fprint(os.Stderr, output)
			}
			if err != nil {
				break
			}
		}
	}()

	// 等待命令完成
	err := cmd.Wait()

	taskStatusMu.Lock()
	if err != nil {
		log.Printf("\n")
		log.Printf("任务[ %s ]执行失败: %v", taskID, err)
		task.Status = "failed"
		task.Error = err.Error()
	} else {
		log.Printf("\n")
		log.Printf("任务[ %s ]已完成", taskID)
		log.Printf("生成结果: ./%s", outputPath)
		task.Status = "completed"
		task.ResultPath = outputPath
	}
	completedTask := &task
	taskStatus[taskID] = completedTask
	taskStatusMu.Unlock()

	// 通知等待的客户端
	notifyWaitClients(taskID, completedTask)
}

// notifyWaitClients 通知等待的客户端任务完成
func notifyWaitClients(taskID string, task *GenerateTask) {
	waitClientsMu.RLock()
	ch, exists := waitClients[taskID]
	waitClientsMu.RUnlock()

	if exists {
		ch <- task
		close(ch)

		waitClientsMu.Lock()
		delete(waitClients, taskID)
		waitClientsMu.Unlock()
	}
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
    taskStatusMu.RLock()
    existingTask, exists := taskStatus[taskID]
    taskStatusMu.RUnlock()

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

    // 创建通道用于等待任务完成
    ch := make(chan *GenerateTask, 1)

    // 注册客户端
    waitClientsMu.Lock()
    waitClients[taskID] = ch
    waitClientsMu.Unlock()

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
        http.Error(w, "任务处理超时", http.StatusRequestTimeout)
        return
    }
}
