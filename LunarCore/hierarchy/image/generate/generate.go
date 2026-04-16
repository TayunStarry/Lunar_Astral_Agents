package generate

import (
	"LunarCore/config"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// StartTaskProcessor 启动任务处理协程
func StartTaskProcessor() {
	go func() {
		for task := range TaskQueue {
			ProcessTask(task)
		}
	}()
}

// CreateGenerateTask 创建生成任务
func CreateGenerateTask(prompt, negativePrompt string, batchSize, width, height, steps int, strength, cfgScale float64, seed int64, initImg string) (*GenerateTask, int) {
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
		CreatedAt:      time.Now(),
		Status:         "queued",
	}

	// 存储任务状态
	TaskStatusMu.Lock()
	TaskStatus[taskID] = task
	TaskStatusMu.Unlock()

	// 将任务加入队列
	select {
	case TaskQueue <- *task:
		return task, len(TaskQueue)
	default:
		return nil, -1
	}
}

// ProcessTask 处理单个任务
func ProcessTask(task GenerateTask) {
	taskID := task.ID

	// 更新任务状态为运行中
	TaskStatusMu.Lock()
	task.Status = "running"
	TaskStatus[taskID] = &task
	TaskStatusMu.Unlock()

	// 打印任务开始分隔线
	log.Printf("%s", strings.Repeat("-=", 28))
	log.Printf("开始处理任务: %s", taskID)

	// 构建输出文件名
	timestamp := time.Now().Format("20060102_150405")
	outputFilename := fmt.Sprintf("generated_%s.png", timestamp)
	outputPath := filepath.Join(*config.LocalDir, "generated", outputFilename)

	// 确保输出目录存在
	os.MkdirAll(filepath.Join(*config.LocalDir, "generated"), 0755)

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
		initImgPath := filepath.Join(*config.LocalDir, task.InitImg)
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
		TaskStatusMu.Lock()
		task.Status = "failed"
		task.Error = err.Error()
		TaskStatus[taskID] = &task
		TaskStatusMu.Unlock()
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

	TaskStatusMu.Lock()
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
	TaskStatus[taskID] = completedTask
	TaskStatusMu.Unlock()

	// 通知等待的客户端
	NotifyWaitClients(taskID, completedTask)
}

// NotifyWaitClients 通知等待的客户端任务完成
func NotifyWaitClients(taskID string, task *GenerateTask) {
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

// GetTaskStatus 获取任务状态
func GetTaskStatus(taskID string) (*GenerateTask, bool) {
	TaskStatusMu.RLock()
	defer TaskStatusMu.RUnlock()
	task, exists := TaskStatus[taskID]
	return task, exists
}

// RegisterWaitClient 注册等待客户端
func RegisterWaitClient(taskID string) chan *GenerateTask {
	ch := make(chan *GenerateTask, 1)
	WaitClientsMu.Lock()
	WaitClients[taskID] = ch
	WaitClientsMu.Unlock()
	return ch
}

// RemoveWaitClient 移除等待客户端
func RemoveWaitClient(taskID string) {
	WaitClientsMu.Lock()
	delete(WaitClients, taskID)
	WaitClientsMu.Unlock()
}
