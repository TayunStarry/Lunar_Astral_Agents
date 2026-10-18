package module

import (
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"io"
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
//
// scheme 为请求指定的绘图模型方案；空串或未知值回落到 Go 层当前方案。
// 方案在入队时固化为任务字段，避免排队期间切换方案导致同一任务前后不一致。
func CreateGenerateTask(prompt, negativePrompt string, batchSize, width, height, steps int, strength, cfgScale float64, seed int64, initImg string, allowSuperResolution bool, scheme string) (*GenerateTask, int) {
	taskID := fmt.Sprintf("task_%d", time.Now().UnixNano())
	schemeName := GeneralConfig.NormalizeDiffusionSchemeName(scheme)
	if schemeName == "" {
		schemeName = GeneralConfig.CurrentDiffusionSchemeName()
	}
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
		Scheme:               schemeName,
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
	// 模型方案快照：任务创建时已固化方案名，这里按任务取成套模型路径，
	// 排队期间前端/智能体切换方案不会改变本任务实际使用的模型。
	scheme := GeneralConfig.ResolveDiffusionScheme(task.Scheme)
	schemeLabel := scheme.Name
	if schemeLabel == "" {
		schemeLabel = "默认（扁平配置）"
	}
	LoggerGeneral.Info("MultimodalAnalysis", "开始处理任务: %s（模型方案: %s）", taskID, schemeLabel)

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
		// 方案推荐步数优先（如 turbo 的少步数），否则回落到全局 sd_steps
		steps = scheme.Steps
	}
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

	// 构建命令参数（模型路径取自本任务的方案快照）
	args := []string{
		"--diffusion-model", scheme.DiffusionModel,
		"--vae", scheme.VariationalModel,
		"--llm", scheme.PromptAnalysisModel,
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
		// 多模态提示词模型（随方案成套切换）
		if scheme.PromptMmprojModel != "" {
			args = append(args, "--llm_vision", scheme.PromptMmprojModel)
		}
	}

	// 批处理数量
	if task.BatchSize > 1 {
		args = append(args, "-b", fmt.Sprintf("%d", task.BatchSize))
	}
	// 超分参数
	//
	// 说明：`--hires-denoising-strength` 单独传是**无效的** —— sd.cpp 的二次采样需要
	// `--hires` 才会启用（该 flag 本工程从未传递），因此此前那行参数只是让人误以为
	// 存在一次「高分辨率再扩散」。超分走的是 RealESRGAN 放大（--upscale-model），
	// 不做二次去噪，构图与细节不会再变。这里显式删掉误导性参数。
	if task.AllowSuperResolution && scheme.RealESRGANModel != "" {
		args = append(args, "--upscale-model", scheme.RealESRGANModel)
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

	// ==== VAE 解码路径（必须在启动引擎前定死）====
	//
	// 实测（RTX 4070 12GB + Qwen-Image 2.1，wan_vae bf16，1024×1536）：
	//   - 整帧解码（im2col 卷积）需要 12475–15689 MiB，守卫后可用约 10850–10990 MiB
	//     → 必然失败；sd.cpp 失败后**自行**改用空间分片（`backend_fit.cpp:506`），
	//     而 wan_vae 的分片接缝会留下硬边纯白方块（实测 17×16 px，右边界正落在
	//     512px = 32 latent 的分片边界上）；
	//   - 整帧解码改用 direct conv 后显存大降，实测可在 12GB 卡上直接成功，
	//     解码 3.81s；与 CPU 全帧解码像素比对：最大差 1/255、98.9% 像素完全相同；
	//   - 整段 CPU 解码同样无接缝，但 1024×1536 实测要 197.93s。
	// 因此按「整帧 GPU → GPU + direct conv → CPU」三档退让，绝不落到空间分片。
	plan := resolveVAEDecode(width, height, freeMiB, vaeBackendMode(), *GeneralConfig.SDVaeConvDirect)
	switch {
	case plan.onCPU:
		LoggerGeneral.Info("MultimodalAnalysis", "VAE 解码在 CPU 上进行: %s", plan.why)
	case plan.convDirect:
		LoggerGeneral.Info("MultimodalAnalysis", "VAE 解码在 GPU 上进行（direct conv）: %s", plan.why)
	default:
		LoggerGeneral.Info("MultimodalAnalysis", "VAE 解码在 GPU 上进行: %s", plan.why)
	}
	if plan.convDirect {
		args = append(args, "--vae-conv-direct")
	}

	// VAE 分片解码：默认 off。只有显式要求（always）或「显式要求 GPU 且显存放不下」时
	// 才传入分片几何——分片一定会在接缝处留白块，能不用就不用。
	needVAETiling := false
	switch strings.ToLower(strings.TrimSpace(*GeneralConfig.SDVaeTiling)) {
	case "always", "true", "on":
		needVAETiling = true
	case "off", "false", "no":
		needVAETiling = false
	default: // auto
		needVAETiling = !plan.onCPU && vaeBackendMode() == "gpu"
	}
	if needVAETiling {
		args = append(args, "--vae-tiling")
		if v := strings.TrimSpace(*GeneralConfig.SDVaeTileSize); v != "" {
			args = append(args, "--vae-tile-size", v)
		}
		if *GeneralConfig.SDVaeTileOverlap > 0 {
			args = append(args, "--vae-tile-overlap", fmt.Sprintf("%.2f", *GeneralConfig.SDVaeTileOverlap))
		}
		LoggerGeneral.Warn("MultimodalAnalysis",
			"sd.cpp 启用 VAE 分片解码 (%dx%d)：分片接缝可能出现硬边纯色方块，仅建议在无法整段解码时使用", width, height)
	}

	// 运行时后端分配：sd.cpp 的 --backend 接受**逗号列表**（如 clip=cpu,vae=cuda0）。
	// 必须合并成一条：分多次传 --backend 由解析器互相覆盖，依赖它等于赌实现细节。
	backendSpecs := make([]string, 0, 2)
	if *GeneralConfig.SDTextEncoderOnCPU {
		backendSpecs = append(backendSpecs, "te=cpu") // 提示词编码器（--llm）不占显存
	}
	if plan.onCPU {
		backendSpecs = append(backendSpecs, "vae=cpu")
	}
	if len(backendSpecs) > 0 {
		args = append(args, "--backend", strings.Join(backendSpecs, ","))
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

	// 引擎输出接入统一日志：已注入钩子（TUI）时按行切分后由 TUI 排版显示，
	// 未注入时（桌面构建）直写终端，行为与之前一致
	stdoutWriter := newEngineLogWriter("stdout", os.Stdout)
	stderrWriter := newEngineLogWriter("stderr", os.Stderr)

	go func() {
		_, _ = io.Copy(stdoutWriter, stdout)
		stdoutWriter.Close()
	}()

	go func() {
		_, _ = io.Copy(stderrWriter, stderr)
		stderrWriter.Close()
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

	// 创建任务（未指定方案时由 CreateGenerateTask 固化当前方案）
	task, _ := CreateGenerateTask(prompt, negativePrompt, batchSize, width, height, steps, strength, cfgScale, seed, initImg, allowSuperResolution, "")

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

// UpscaleImage 对已有图片执行 RealESRGAN 超分辨率放大（4x，sd-cli -M upscale 纯超分管线）
// 输入图片字节写入临时文件供引擎读取，放大结果写入多媒体生成目录并返回 base64 与尺寸
func UpscaleImage(imageData []byte) (map[string]any, error) {
	// 与扩散生成共用功能开关
	if !*GeneralConfig.AllowDiffusion {
		return nil, fmt.Errorf("未启用[扩散生成]功能")
	}
	if len(imageData) == 0 {
		return nil, fmt.Errorf("图片数据为空")
	}
	// 超分模型跟随当前绘图方案（未配置方案时回退扁平配置）
	upscaleModel := GeneralConfig.ResolveDiffusionScheme("").RealESRGANModel
	if strings.TrimSpace(upscaleModel) == "" {
		return nil, fmt.Errorf("未配置超分模型（real_esrgan_model）")
	}

	// 输入写入临时文件（sd-cli 只接受文件路径），结束后清理
	inputFile, err := os.CreateTemp("", "lunar_upscale_in_*.png")
	if err != nil {
		return nil, fmt.Errorf("创建超分输入临时文件失败: %v", err)
	}
	inputPath := inputFile.Name()
	defer os.Remove(inputPath)
	if _, err := inputFile.Write(imageData); err != nil {
		inputFile.Close()
		return nil, fmt.Errorf("写入超分输入临时文件失败: %v", err)
	}
	inputFile.Close()

	// 输出写入多媒体生成目录
	os.MkdirAll(filepath.Join(*GeneralConfig.LocalDir, "multimedia/generated"), 0755)
	outputPath := filepath.Join(*GeneralConfig.LocalDir, "multimedia/generated", fmt.Sprintf("upscale_%s.png", time.Now().Format("20060102_150405")))

	// 纯超分管线：不加载扩散模型，仅 RealESRGAN 放大
	args := []string{
		"-M", "upscale",
		"--upscale-model", upscaleModel,
		"-i", inputPath,
		"-o", outputPath,
	}
	LoggerGeneral.Info("MultimodalAnalysis", "图像超分: %s %s", *GeneralConfig.VisualEngine, strings.Join(args, " "))

	cmd := exec.Command(*GeneralConfig.VisualEngine, args...)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	stdoutWriter := newEngineLogWriter("stdout", os.Stdout)
	stderrWriter := newEngineLogWriter("stderr", os.Stderr)
	go func() { _, _ = io.Copy(stdoutWriter, stdout); stdoutWriter.Close() }()
	go func() { _, _ = io.Copy(stderrWriter, stderr); stderrWriter.Close() }()

	if err := cmd.Run(); err != nil {
		LoggerGeneral.Error("MultimodalAnalysis", "图像超分执行失败: %v", err)
		return nil, fmt.Errorf("超分执行失败: %v", err)
	}

	// 读取放大结果
	imageResult, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("读取超分结果失败: %v", err)
	}

	// 解码尺寸（仅读取头部信息，不整图解码）
	config, _, err := image.DecodeConfig(bytes.NewReader(imageResult))
	width, height := 0, 0
	if err != nil {
		LoggerGeneral.Warn("MultimodalAnalysis", "超分结果尺寸解码失败: %v", err)
	} else {
		width, height = config.Width, config.Height
	}

	LoggerGeneral.Info("MultimodalAnalysis", "图像超分完成: %s (%dx%d)", outputPath, width, height)
	return map[string]any{
		"path":   outputPath,
		"base64": base64.StdEncoding.EncodeToString(imageResult),
		"width":  width,
		"height": height,
	}, nil
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

// ==== VAE 解码路径决策 ====

// vaeDecodeMiBPerMegapixel 整帧解码（im2col 卷积）的显存系数（MiB / 兆像素）。
//
// 标定依据（实测，Qwen-Image 2.1 的 wan_vae bf16，sd.cpp 的 model_manager 报告值）：
// 1024×1536 = 1.5 MP 需要 12475 MiB / 15689 MiB 两次报告 → 约 8300–10400 MiB/MP。
// 取 9216（9 GiB/MP）落在区间中部：
//   - 不取低值，否则 12GB 卡上 1024×1536 会被判为「放得下」，然后 OOM 触发引擎自动分片；
//   - 不取高值，否则 12GB 卡上 1024×1024（实测可全帧解码）会被白白赶去 CPU/direct conv。
const vaeDecodeMiBPerMegapixel = 9216

// vaeDecodeDirectMiBPerMegapixel direct conv 的显存系数：按普通卷积的一半估算。
// 依据：direct conv 绕开了 im2col 的 K² 倍临时缓冲，实测 1024×1536（1.5MP）
// 在可用 10.4 GB 的卡上直接成功（解码 3.81s），据此反推其需求远低于整帧 im2col。
// 取一半既留出余量，又不至于把中尺寸图误判成放不下。
const vaeDecodeDirectMiBPerMegapixel = vaeDecodeMiBPerMegapixel / 2

// vaeDecodeFixedMiB 与分辨率无关的固定开销（VAE 权重约 644 MiB + 图切割调度缓冲）
const vaeDecodeFixedMiB = 1024

// estimateVAEDecodeMiB 估算 VAE 整帧解码（普通卷积）所需的设备显存（MiB）
func estimateVAEDecodeMiB(width, height int) int {
	return vaeDecodeFixedMiB + scaleByMegapixel(width, height, vaeDecodeMiBPerMegapixel)
}

// estimateVAEDecodeDirectMiB 估算 direct conv 整帧解码所需的设备显存（MiB）
func estimateVAEDecodeDirectMiB(width, height int) int {
	return vaeDecodeFixedMiB + scaleByMegapixel(width, height, vaeDecodeDirectMiBPerMegapixel)
}

// scaleByMegapixel 按面积（兆像素）缩放显存系数；尺寸缺失时返回 0（只剩固定开销）
func scaleByMegapixel(width, height, perMP int) int {
	if width <= 0 || height <= 0 {
		return 0
	}
	mp := float64(width) * float64(height) / (1024 * 1024)
	return int(mp * float64(perMP))
}

// vaeBackendMode 归一化 sd-vae-backend 配置（未知取值按 auto 处理）
func vaeBackendMode() string {
	switch strings.ToLower(strings.TrimSpace(*GeneralConfig.SDVaeBackend)) {
	case "cpu":
		return "cpu"
	case "gpu":
		return "gpu"
	}
	return "auto"
}

// vaeDecodePlan VAE 解码落点：三档里必选其一
type vaeDecodePlan struct {
	onCPU      bool   // 整段 CPU 解码
	convDirect bool   // GPU 解码但改用 direct conv（省显存）
	why        string // 判定依据（写进日志）
}

// resolveVAEDecode 决定 VAE 解码落点。
//
// 三档退让：整帧 GPU → GPU + direct conv → 整段 CPU。任何一档都**不**使用空间分片，
// 因为 wan_vae 的分片接缝会在画面上留下硬边纯色方块（实测 17×16 px 纯白块）。
// 实测代价（1024×1536 / RTX 4070）：GPU 整帧失败（需 12475+ MiB），
// GPU + direct conv 解码 3.81s 且与 CPU 全帧像素等价（最大差 1/255），CPU 全帧 197.93s。
func resolveVAEDecode(width, height, freeMiB int, mode string, allowConvDirect bool) vaeDecodePlan {
	switch mode {
	case "cpu":
		return vaeDecodePlan{onCPU: true, why: "配置 sd-vae-backend=cpu"}
	case "gpu":
		return vaeDecodePlan{
			convDirect: allowConvDirect,
			why:        "配置 sd-vae-backend=gpu（显存不足时引擎会自动降级分片，接缝可能留白块）",
		}
	}

	need := estimateVAEDecodeMiB(width, height)
	if freeMiB <= 0 {
		// 显存守卫没给出可用值（未启用或取不到）：不擅自改后端，但把估算写进日志，
		// 万一引擎随后降级分片，这条记录就是唯一能对照的线索。
		return vaeDecodePlan{why: fmt.Sprintf("可用显存未知（显存守卫未启用或取不到），交给引擎决策（整帧约需 %d MiB）", need)}
	}
	if need <= freeMiB {
		return vaeDecodePlan{why: fmt.Sprintf("%dx%d 整帧解码约需 %d MiB，可用 %d MiB", width, height, need, freeMiB)}
	}
	if allowConvDirect {
		needDirect := estimateVAEDecodeDirectMiB(width, height)
		if needDirect <= freeMiB {
			return vaeDecodePlan{convDirect: true, why: fmt.Sprintf(
				"%dx%d 整帧约需 %d MiB > 可用 %d MiB，改用 direct conv（估算 %d MiB；实测与 CPU 全帧像素等价）",
				width, height, need, freeMiB, needDirect)}
		}
	}
	return vaeDecodePlan{onCPU: true, why: fmt.Sprintf(
		"%dx%d 整帧解码约需 %d MiB，超过可用 %d MiB；改 CPU 解码以免引擎自动分片", width, height, need, freeMiB)}
}
