package module

import (
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"encoding/base64"
	"fmt"
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
func CreateGenerateTask(prompt, negativePrompt string, batchSize, width, height, steps int, strength, cfgScale float64, seed int64, initImg string, allowSuperResolution bool) (*GenerateTask, int) {
	taskID := fmt.Sprintf("task_%d", time.Now().UnixNano())
	task := &GenerateTask{
		ID:                   taskID,
		Prompt:               prompt,
		NegativePrompt:       negativePrompt,
		BatchSize:            batchSize,
		Width:                width,
		Height:               height,
		Strength:             strength,
		Steps:                steps,
		Seed:                 seed,
		CfgScale:             cfgScale,
		InitImg:              initImg,
		AllowSuperResolution: allowSuperResolution,
		CreatedAt:            time.Now(),
		Status:               "queued",
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
	LoggerGeneral.Info("MultimodalAnalysis", "开始处理任务: %s", taskID)

	// 构建输出文件名
	timestamp := time.Now().Format("20060102_150405")
	outputFilename := fmt.Sprintf("%s.png", timestamp)
	outputPath := filepath.Join(*GeneralConfig.LocalDir, "multimedia/generated", outputFilename)

	// 确保输出目录存在
	os.MkdirAll(filepath.Join(*GeneralConfig.LocalDir, "multimedia/generated"), 0755)

	// 归一化请求参数：尺寸对齐到 32 的倍数（Qwen-Image 系列为 16× VAE 压缩 + 2×2 patch，官方要求
	// 宽高可被 32 整除；官方 2K 比例如 2528×1696 也是 32 的倍数而非 64），步数收进合理区间；
	// 调用方未指定时使用配置默认值（sd_steps / sd_image_size）。
	width, height := normalizeImageSide(task.Width), normalizeImageSide(task.Height)
	steps := task.Steps
	if steps <= 0 {
		steps = *GeneralConfig.SDSteps
	}
	if steps < 1 {
		steps = 1
	}
	if steps > 150 {
		steps = 150
	}
	if width != task.Width || height != task.Height {
		LoggerGeneral.Info("MultimodalAnalysis", "尺寸归一化: %dx%d -> %dx%d (32 的倍数)", task.Width, task.Height, width, height)
	}
	task.Width, task.Height, task.Steps = width, height, steps

	// 随机种子：请求未指定（<=0）时生成一个并回写任务。
	// 引擎默认种子固定为 42，不显式传参会导致每次构图完全一致，这里统一改成"随机且可复现"。
	if task.Seed <= 0 {
		task.Seed = time.Now().UnixNano() & 0x7FFFFFFF
		if task.Seed == 0 {
			task.Seed = 1
		}
	}

	// 构建命令参数
	args := []string{
		"--diffusion-model", *GeneralConfig.DiffusionModel,
		"--vae", *GeneralConfig.VariationalModel,
		"--llm", *GeneralConfig.PromptAnalysisModel,
		"--cfg-scale", fmt.Sprintf("%.2f", task.CfgScale),
		"--steps", fmt.Sprintf("%d", steps),
		"-H", fmt.Sprintf("%d", height),
		"-W", fmt.Sprintf("%d", width),
		"--seed", fmt.Sprintf("%d", task.Seed),
		"-o", outputPath,
		"-p", task.Prompt,
	}

	// 采样策略：auto 表示不传参，由 sd.cpp 按模型自动选择（本模型为 euler + flux 调度）
	if v := strings.TrimSpace(*GeneralConfig.SDSampler); v != "" && !strings.EqualFold(v, "auto") {
		args = append(args, "--sampling-method", v)
	}
	if v := strings.TrimSpace(*GeneralConfig.SDScheduler); v != "" && !strings.EqualFold(v, "auto") {
		args = append(args, "--scheduler", v)
	}
	if *GeneralConfig.SDFlowShift > 0 {
		args = append(args, "--flow-shift", fmt.Sprintf("%.3f", *GeneralConfig.SDFlowShift))
	}
	// 扩散模型 flash attention：速度换数值精度，排查方块伪影时可关闭后对比
	if *GeneralConfig.SDFlashAttention {
		args = append(args, "--diffusion-fa")
	}

	// 添加负面提示词
	if task.NegativePrompt != "" {
		args = append(args, "-n", task.NegativePrompt)
	}

	// 图生图参数
	if task.InitImg != "" && task.InitImg != "null" {
		initImgPath := filepath.Join(*GeneralConfig.LocalDir, task.InitImg)
		if _, err := os.Stat(initImgPath); err == nil {
			args = append(args, "--init-img", initImgPath)
			args = append(args, "--strength", fmt.Sprintf("%.2f", task.Strength))
		}
		// 多模态提示词模型
		if *GeneralConfig.PromptMmprojModel != "" {
			args = append(args, "--llm_vision", *GeneralConfig.PromptMmprojModel)
		}
	}

	// 批处理数量
	if task.BatchSize > 1 {
		args = append(args, "-b", fmt.Sprintf("%d", task.BatchSize))
	}
	// 超分参数
	if task.AllowSuperResolution {
		args = append(args, "--upscale-model", *GeneralConfig.RealESRGANModel)
		args = append(args, "--hires-denoising-strength", "0.55")
	}

	// 执行显存守卫：可用显存不足时先卸载月华模型, 并记录守卫后的可用显存
	freeMiB := 0
	if GeneralConfig.ImageVRAMGuardHook != nil {
		free, err := GeneralConfig.ImageVRAMGuardHook()
		freeMiB = free
		if err != nil {
			LoggerGeneral.Warn("MultimodalAnalysis", "显存守卫执行失败: %v", err)
		}
	}

	// 守卫后显存仍吃紧时, 让 sd.cpp 把权重驻留内存、按需载入显存（计算仍在 GPU）, 避免爆显存
	switch strings.ToLower(strings.TrimSpace(*GeneralConfig.SDOffloadToCPU)) {
	case "always", "true":
		args = append(args, "--offload-to-cpu")
		LoggerGeneral.Info("MultimodalAnalysis", "sd.cpp 权重内存卸载: always")
	case "off", "false", "no":
		// 显式禁用
	default: // auto
		if freeMiB > 0 && freeMiB < *GeneralConfig.ImageVRAMGuardMiB {
			args = append(args, "--offload-to-cpu")
			LoggerGeneral.Info("MultimodalAnalysis", "可用显存 %d MiB 不足, sd.cpp 启用权重内存卸载", freeMiB)
		}
	}

	// 显存预算：交给 sd.cpp 做图切割执行（GiB，支持 cuda0=9 形式）；未配置时由 --auto-fit 自动管理
	if v := strings.TrimSpace(*GeneralConfig.SDMaxVRAM); v != "" {
		args = append(args, "--max-vram", v)
		LoggerGeneral.Info("MultimodalAnalysis", "sd.cpp 显存预算: %s GiB", v)
	}

	// VAE 分片解码：默认关闭。sd.cpp 的分片默认几何为 tile 32 latent（VAE 16× 压缩 → 512px）、
	// overlap 0.5（步长 256px），会在分片角注入硬边纯白方块；仅在大图或显存吃紧时按需启用。
	needVAETiling := false
	switch strings.ToLower(strings.TrimSpace(*GeneralConfig.SDVaeTiling)) {
	case "always", "true", "on":
		needVAETiling = true
	case "off", "false", "no":
		needVAETiling = false
	default: // auto
		// 阈值取 2 MP（约 1536² 以上）：1024² 及以下的单次解码在 12GB 卡上无需分片，
		// 而分片会引入方块伪影，所以只有确实吃紧时才退让。
		needVAETiling = width*height > 2*1024*1024 || (freeMiB > 0 && freeMiB < *GeneralConfig.ImageVRAMGuardMiB)
	}
	if needVAETiling {
		args = append(args, "--vae-tiling")
		if v := strings.TrimSpace(*GeneralConfig.SDVaeTileSize); v != "" {
			args = append(args, "--vae-tile-size", v)
		}
		if *GeneralConfig.SDVaeTileOverlap > 0 {
			args = append(args, "--vae-tile-overlap", fmt.Sprintf("%.2f", *GeneralConfig.SDVaeTileOverlap))
		}
		LoggerGeneral.Info("MultimodalAnalysis", "sd.cpp 启用 VAE 分片解码 (%dx%d, 守卫后可用显存 %d MiB)", width, height, freeMiB)
	}

	// 提示词编码器（--llm）强制在 CPU 运行, 全程不占显存
	if *GeneralConfig.SDTextEncoderOnCPU {
		args = append(args, "--backend", "te=cpu")
	}

	// 显示命令参数，正确分组
	LoggerGeneral.Info("MultimodalAnalysis", "执行命令参数:")
	LoggerGeneral.Info("MultimodalAnalysis", "  程序: %s", *GeneralConfig.VisualEngine)

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
			LoggerGeneral.Info("MultimodalAnalysis", "  参数: %s %s", current, value)
		} else {
			LoggerGeneral.Info("MultimodalAnalysis", "  参数: %s", current)
		}
	}

	// 执行命令
	cmd := exec.Command(*GeneralConfig.VisualEngine, args...)

	// 捕获标准输出和错误输出
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		LoggerGeneral.Error("MultimodalAnalysis", "任务[%s]执行失败: %v", taskID, err)
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
		LoggerGeneral.Error("MultimodalAnalysis", "任务[%s]执行失败: %v", taskID, err)
		task.Status = "failed"
		task.Error = err.Error()
	} else {
		LoggerGeneral.Info("MultimodalAnalysis", "任务[%s]已完成", taskID)
		LoggerGeneral.Info("MultimodalAnalysis", "生成结果: ./%s", outputPath)
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

// GenerateImage 生成图片并等待完成，返回图片路径和base64编码
func GenerateImage(prompt, negativePrompt string, batchSize, width, height, steps int, strength, cfgScale float64, seed int64, initImg string, allowSuperResolution bool) (map[string]any, error) {
	// 检查是否允许使用扩散生成
	if !*GeneralConfig.AllowDiffusion {
		return nil, fmt.Errorf("未启用[扩散生成]功能")
	}

	// 验证参数
	if prompt == "" {
		return nil, fmt.Errorf("提示词不能为空")
	}

	// 创建任务
	task, _ := CreateGenerateTask(prompt, negativePrompt, batchSize, width, height, steps, strength, cfgScale, seed, initImg, allowSuperResolution)

	if task == nil {
		return nil, fmt.Errorf("任务队列已满")
	}

	// 注册等待客户端
	ch := RegisterWaitClient(task.ID)

	// 等待任务完成
	completedTask := <-ch

	// 检查任务状态
	if completedTask.Status == "failed" {
		return nil, fmt.Errorf("任务执行失败: %s", completedTask.Error)
	}

	// 读取生成的图片文件
	imageData, err := os.ReadFile(completedTask.ResultPath)
	if err != nil {
		return nil, fmt.Errorf("读取图片文件失败: %v", err)
	}

	// 生成base64编码
	base64Data := base64.StdEncoding.EncodeToString(imageData)

	// 构造返回结果：尺寸与种子回传任务归一化后的实际值（尺寸对齐后的边长、随机种子），
	// 便于调用方复现同一张图（原先回传的是请求原值，未指定时为 0）。
	result := map[string]any{
		"path":   completedTask.ResultPath,
		"base64": base64Data,
		"width":  completedTask.Width,
		"height": completedTask.Height,
		"seed":   completedTask.Seed,
	}

	return result, nil
}

// normalizeImageSide 把请求边长归一化为 32 的倍数。
// 依据：Qwen-Image 2.1 为 16× VAE 压缩 + 2×2 patch 的 DiT，官方要求宽高可被 32 整除；
// 官方 2K 比例（如 3:2 的 2528×1696）也是 32 的倍数而非 64，因此这里按 32 对齐而不是 64。
// 调用方未指定（<=0）时使用配置默认边长 sd_image_size。
func normalizeImageSide(side int) int {
	const minSide, maxSide, align = 256, 2816, 32
	if side <= 0 {
		side = *GeneralConfig.SDImageSize
	}
	if side <= 0 {
		side = 1024
	}
	if side < minSide {
		side = minSide
	}
	if side > maxSide {
		side = maxSide
	}
	side = (side + align/2) / align * align
	if side > maxSide {
		side = maxSide
	}
	return side
}
