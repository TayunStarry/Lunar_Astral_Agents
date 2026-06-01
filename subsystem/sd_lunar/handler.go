package main

import (
	"config"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"logger"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var proxyPrefixes = []string{}

func shouldProxy(path string) bool {
	for _, prefix := range proxyPrefixes {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func getProxyHandler() *httputil.ReverseProxy {
	proxyURL, err := url.Parse("http://localhost:36789")
	if err != nil {
		logger.Error("sd_lunar", "解析代理 URL 失败: %v", err)
		return nil
	}
	return httputil.NewSingleHostReverseProxy(proxyURL)
}

func copyBuffer(dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				return written, ew
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}
		}
		if er != nil {
			if er == io.EOF {
				er = nil
			}
			return written, er
		}
	}
}

func reloadPageParameters() {
	*config.WebViewTitle = "星月智能 -> SD图像生成能力测试系统"
	*config.WebViewWidth = 1320
	*config.WebViewHeight = 900
}

func getVisualEngine() string {
	return *config.VisualEngine
}

func getDiffusionModel() string {
	return *config.DiffusionModel
}

func getVAEModel() string {
	return *config.VariationalModel
}

func getRefineModel() string {
	return *config.PromptRefineModel
}

func saveInitImage(base64Data string) (string, error) {
	if base64Data == "" {
		return "", fmt.Errorf("初始图像数据为空")
	}
	decoded, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return "", fmt.Errorf("解码初始图像失败: %v", err)
	}
	initDir := filepath.Join(*config.LocalDir, "sd_init")
	os.MkdirAll(initDir, 0755)
	initPath := filepath.Join(initDir, fmt.Sprintf("init_%d.png", time.Now().UnixNano()))
	if err := os.WriteFile(initPath, decoded, 0644); err != nil {
		return "", fmt.Errorf("写入初始图像失败: %v", err)
	}
	return initPath, nil
}

func txt2imgHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	var req Txt2ImgRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: false,
			Error:   "无效的请求体: " + err.Error(),
		})
		return
	}

	if req.Prompt == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: false,
			Error:   "提示词不能为空",
		})
		return
	}

	if req.Width <= 0 {
		req.Width = 512
	}
	if req.Height <= 0 {
		req.Height = 512
	}
	if req.Steps <= 0 {
		req.Steps = 20
	}
	if req.CfgScale <= 0 {
		req.CfgScale = 7.0
	}
	if req.BatchSize <= 0 {
		req.BatchSize = 1
	}

	task, queueLen := createTask(
		req.Prompt,
		req.NegativePrompt,
		req.BatchSize,
		req.Width,
		req.Height,
		req.Steps,
		0,
		req.CfgScale,
		req.Seed,
		"",
		req.UseVulkan,
		req.DiffusionModel,
		req.VAEModel,
		req.RefineModel,
	)

	if task == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: false,
			Error:   "任务队列已满，请稍后重试",
		})
		return
	}

	logger.Info("sd_lunar", "文生图任务已创建: %s, 队列长度: %d", task.ID, queueLen)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(GenerateResponse{
		Success: true,
		TaskID:  task.ID,
		Message: fmt.Sprintf("任务已提交，当前队列长度: %d", queueLen),
	})
}

func img2imgHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	var req Img2ImgRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: false,
			Error:   "无效的请求体: " + err.Error(),
		})
		return
	}

	if req.Prompt == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: false,
			Error:   "提示词不能为空",
		})
		return
	}

	if req.InitImgBase64 == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: false,
			Error:   "初始图像不能为空",
		})
		return
	}

	initImgPath, err := saveInitImage(req.InitImgBase64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	if req.Width <= 0 {
		req.Width = 512
	}
	if req.Height <= 0 {
		req.Height = 512
	}
	if req.Steps <= 0 {
		req.Steps = 20
	}
	if req.CfgScale <= 0 {
		req.CfgScale = 7.0
	}
	if req.Strength <= 0 {
		req.Strength = 0.75
	}
	if req.BatchSize <= 0 {
		req.BatchSize = 1
	}

	task, queueLen := createTask(
		req.Prompt,
		req.NegativePrompt,
		req.BatchSize,
		req.Width,
		req.Height,
		req.Steps,
		req.Strength,
		req.CfgScale,
		req.Seed,
		initImgPath,
		req.UseVulkan,
		req.DiffusionModel,
		req.VAEModel,
		req.RefineModel,
	)

	if task == nil {
		os.Remove(initImgPath)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: false,
			Error:   "任务队列已满，请稍后重试",
		})
		return
	}

	logger.Info("sd_lunar", "图生图任务已创建: %s, 队列长度: %d", task.ID, queueLen)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(GenerateResponse{
		Success: true,
		TaskID:  task.ID,
		Message: fmt.Sprintf("任务已提交，当前队列长度: %d", queueLen),
	})
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	taskID := r.URL.Query().Get("task_id")
	if taskID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: false,
			Error:   "缺少 task_id 参数",
		})
		return
	}

	task, exists := getTaskStatus(taskID)
	if !exists {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: false,
			Error:   "未找到指定任务",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(GenerateResponse{
		Success: true,
		TaskID:  taskID,
		Data:    task,
	})
}

func pollHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/sd/poll/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: false,
			Error:   "缺少任务ID",
		})
		return
	}
	taskID := pathParts[0]

	task, exists := getTaskStatus(taskID)
	if !exists {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: false,
			Error:   "未找到指定任务",
		})
		return
	}

	if task.Status == "queued" || task.Status == "running" {
		ch := registerWaitClient(taskID)
		select {
		case <-time.After(300 * time.Second):
			removeWaitClient(taskID)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(GenerateResponse{
				Success: false,
				TaskID:  taskID,
				Data:    task,
				Error:   "任务等待超时",
			})
			return
		case completedTask := <-ch:
			task = completedTask
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if task.Status == "completed" {
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: true,
			TaskID:  taskID,
			Data:    task,
		})
	} else {
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: false,
			TaskID:  taskID,
			Data:    task,
			Error:   task.Error,
		})
	}
}

func resultHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/sd/result/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: false,
			Error:   "缺少任务ID",
		})
		return
	}
	taskID := pathParts[0]

	task, exists := getTaskStatus(taskID)
	if !exists {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: false,
			Error:   "未找到指定任务",
		})
		return
	}

	if task.Status != "completed" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: false,
			TaskID:  taskID,
			Error:   fmt.Sprintf("任务未完成，当前状态: %s", task.Status),
		})
		return
	}

	if task.ResultPath != "" {
		imageData, err := os.ReadFile(task.ResultPath)
		if err == nil {
			task.ResultBase64 = base64.StdEncoding.EncodeToString(imageData)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(GenerateResponse{
		Success: true,
		TaskID:  taskID,
		Data:    task,
	})
}

func configHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":          true,
			"visual_engine":    getVisualEngine(),
			"diffusion_model":  getDiffusionModel(),
			"vae_model":        getVAEModel(),
			"refine_model":     getRefineModel(),
			"allow_diffusion":  *config.AllowDiffusion,
			"allow_multimodal": *config.AllowMultimodal,
			"developer":        *config.Developer,
			"local_dir":        *config.LocalDir,
		})
		return
	}

	http.Error(w, "不允许的请求方法", http.StatusMethodNotAllowed)
}

func createTask(prompt, negativePrompt string, batchSize, width, height, steps int, strength, cfgScale float64, seed int64, initImg string, useVulkan bool, diffusionModel, vaeModel, refineModel string) (*GenerateTask, int) {
	taskID := fmt.Sprintf("task_%d", time.Now().UnixNano())
	task := &GenerateTask{
		ID:             taskID,
		Prompt:         prompt,
		NegativePrompt: negativePrompt,
		BatchSize:      batchSize,
		Width:          width,
		Height:         height,
		Strength:       strength,
		Steps:          steps,
		Seed:           seed,
		CfgScale:       cfgScale,
		InitImg:        initImg,
		UseVulkan:      useVulkan,
		CreatedAt:      time.Now(),
		Status:         "queued",
	}

	TaskStatusMu.Lock()
	TaskStatus[taskID] = task
	TaskStatusMu.Unlock()

	select {
	case TaskQueue <- *task:
		return task, len(TaskQueue)
	default:
		TaskStatusMu.Lock()
		delete(TaskStatus, taskID)
		TaskStatusMu.Unlock()
		return nil, -1
	}
}

func getTaskStatus(taskID string) (*GenerateTask, bool) {
	TaskStatusMu.RLock()
	defer TaskStatusMu.RUnlock()
	task, exists := TaskStatus[taskID]
	return task, exists
}

func registerWaitClient(taskID string) chan *GenerateTask {
	ch := make(chan *GenerateTask, 1)
	WaitClientsMu.Lock()
	WaitClients[taskID] = ch
	WaitClientsMu.Unlock()
	return ch
}

func removeWaitClient(taskID string) {
	WaitClientsMu.Lock()
	delete(WaitClients, taskID)
	WaitClientsMu.Unlock()
}

func notifyWaitClients(taskID string, task *GenerateTask) {
	WaitClientsMu.RLock()
	ch, exists := WaitClients[taskID]
	WaitClientsMu.RUnlock()

	if exists {
		ch <- task
		close(ch)

		WaitClientsMu.Lock()
		delete(WaitClients, taskID)
		WaitClientsMu.Unlock()
	}
}

func startTaskProcessor() {
	go func() {
		for task := range TaskQueue {
			processTask(task)
		}
	}()
}

func processTask(task GenerateTask) {
	taskID := task.ID

	TaskStatusMu.Lock()
	task.Status = "running"
	TaskStatus[taskID] = &task
	TaskStatusMu.Unlock()
	logger.Info("sd_lunar", "开始处理任务: %s", taskID)

	timestamp := time.Now().Format("20060102_150405")
	outputFilename := fmt.Sprintf("%s_%s.png", taskID, timestamp)
	outputDir := filepath.Join(*config.LocalDir, "images/generated")
	os.MkdirAll(outputDir, 0755)
	outputPath := filepath.Join(outputDir, outputFilename)

	diffusionModel := getDiffusionModel()
	vaeModel := getVAEModel()
	refineModel := getRefineModel()

	args := []string{
		"--diffusion-model", diffusionModel,
		"--vae", vaeModel,
		"--llm", refineModel,
		"--diffusion-fa",
		"--vae-tiling",
		"--cfg-scale", fmt.Sprintf("%.2f", task.CfgScale),
		"--steps", fmt.Sprintf("%d", task.Steps),
		"-H", fmt.Sprintf("%d", task.Height),
		"-W", fmt.Sprintf("%d", task.Width),
		"-o", outputPath,
		"-p", task.Prompt,
	}

	if task.NegativePrompt != "" {
		args = append(args, "-n", task.NegativePrompt)
	}

	if task.InitImg != "" {
		if _, err := os.Stat(task.InitImg); err == nil {
			args = append(args, "--init-img", task.InitImg)
			args = append(args, "--strength", fmt.Sprintf("%.2f", task.Strength))
		}
	}

	if task.Seed != 0 {
		args = append(args, "--seed", fmt.Sprintf("%d", task.Seed))
	}

	if task.BatchSize > 1 {
		args = append(args, "-b", fmt.Sprintf("%d", task.BatchSize))
	}

	if *config.PromptMmprojModel != "" {
		args = append(args, "--llm_vision", *config.PromptMmprojModel)
	}

	logger.Info("sd_lunar", "执行命令参数:")
	logger.Info("sd_lunar", "  引擎: %s", getVisualEngine())

	for i := 0; i < len(args); i++ {
		current := args[i]
		isValueParam := false
		value := ""

		if strings.HasPrefix(current, "-") || strings.HasPrefix(current, "--") {
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") && !strings.HasPrefix(args[i+1], "--") {
				value = args[i+1]
				i++
				isValueParam = true
			}
		}

		if isValueParam {
			logger.Info("sd_lunar", "  参数: %s %s", current, value)
		} else {
			logger.Info("sd_lunar", "  参数: %s", current)
		}
	}

	cmd := exec.Command(getVisualEngine(), args...)

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		logger.Error("sd_lunar", "任务[%s]执行失败: %v", taskID, err)
		TaskStatusMu.Lock()
		task.Status = "failed"
		task.Error = err.Error()
		TaskStatus[taskID] = &task
		TaskStatusMu.Unlock()
		notifyWaitClients(taskID, &task)
		cleanupInitImage(task.InitImg)
		return
	}

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				fmt.Print(string(buf[:n]))
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
				fmt.Fprint(os.Stderr, string(buf[:n]))
			}
			if err != nil {
				break
			}
		}
	}()

	err := cmd.Wait()

	TaskStatusMu.Lock()
	if err != nil {
		logger.Error("sd_lunar", "任务[%s]执行失败: %v", taskID, err)
		task.Status = "failed"
		task.Error = err.Error()
	} else {
		logger.Info("sd_lunar", "任务[%s]已完成", taskID)
		logger.Info("sd_lunar", "生成结果: %s", outputPath)
		task.Status = "completed"
		task.ResultPath = outputPath
	}
	completedTask := &task
	TaskStatus[taskID] = completedTask
	TaskStatusMu.Unlock()

	notifyWaitClients(taskID, completedTask)
	cleanupInitImage(task.InitImg)
}

func cleanupInitImage(initImg string) {
	if initImg != "" {
		if strings.Contains(initImg, "sd_init") {
			os.Remove(initImg)
		}
	}
}
