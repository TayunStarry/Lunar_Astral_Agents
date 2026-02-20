package handlers

import (
	"bytes"         // 导入 bytes 包，用于处理字节切片
	"encoding/json" // 导入 encoding/json 包，用于 JSON 编码和解码
	"fmt"           // 导入 fmt 包，用于格式化输出
	"image"         // 导入 image 包，用于处理图像
	"image/jpeg"    // 导入 image/jpeg 包，用于处理 JPEG 图像
	"io"            // 导入 io 包，用于 IO 操作
	"net/http"      // 导入 net/http 包，用于 HTTP 服务器
	"os"            // 导入 os 包，用于操作系统交互
	"path/filepath" // 导入 path/filepath 包，用于文件路径操作
	"slices"        // 导入 slices 包，用于操作切片
	"strconv"       // 导入 strconv 包，用于字符串转换
	"strings"       // 导入 strings 包，用于字符串操作
	"sync"          // 导入 sync 包，用于并发控制
	"time"          // 导入 time 包，用于时间操作

	ffmpeg "github.com/u2takey/ffmpeg-go" // 导入 ffmpeg-go 包，用于处理视频
)

// 支持的视频格式列表
var supportedVideoFormats = []string{".mp4", ".avi", ".mov", ".wmv", ".flv", ".mkv", ".webm", ".m4v"}

// isSupportedVideoFormat 检查文件格式是否支持
func isSupportedVideoFormat(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return slices.Contains(supportedVideoFormats, ext)
}

// KeyFrame 关键帧结构
type KeyFrame struct {
	// 关键帧文件名
	FilePath string `json:"filePath"`
	// 关键帧时间戳
	Timestamp string `json:"timestamp"`
	// 关键帧编号
	FrameNum int `json:"frameNum"`
	// 关键帧图像数据
	Data []byte `json:"data,omitempty"`
}

// ExtractKeyFramesHandler 用于处理 HTTP POST 请求，提取上传视频的关键帧。
func ExtractKeyFramesHandler(w http.ResponseWriter, r *http.Request) {
	// 只允许 POST 方法，否则返回 405 Method Not Allowed
	if r.Method != "POST" {
		http.Error(w, "ExtractKeyFrames请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}
	// 解析 multipart 表单，限制最大内存 100 MiB（100 << 20 = 104,857,600 字节）
	err := r.ParseMultipartForm(100 << 20)
	if err != nil {
		http.Error(w, fmt.Sprintf("ExtractKeyFrames请求[ERROR] -> 解析表单失败: %v", err), http.StatusBadRequest)
		return
	}
	// 从表单获取文件字段 "video"，返回 multipart.File 及其文件头信息
	file, handler, err := r.FormFile("video")
	if err != nil {
		http.Error(w, fmt.Sprintf("ExtractKeyFrames请求[ERROR] -> 获取文件失败: %v", err), http.StatusBadRequest)
		return
	}
	// 确保文件句柄及时关闭，防止泄露
	defer file.Close()
	// 检查文件格式是否支持
	if !isSupportedVideoFormat(handler.Filename) {
		http.Error(w, "ExtractKeyFrames请求[ERROR] -> 输入文件必须是支持的视频格式", http.StatusBadRequest)
		return
	}
	// 在系统默认临时目录创建临时文件，模板为 "video_*.mp4"
	tempFile, err := os.CreateTemp("", "video_*.mp4")
	if err != nil {
		http.Error(w, fmt.Sprintf("ExtractKeyFrames请求[ERROR] -> 创建临时文件失败: %v", err), http.StatusInternalServerError)
		return
	}
	tempFileName := tempFile.Name()
	// 无论后续逻辑成败，均删除临时文件，防止磁盘堆积
	defer os.Remove(tempFileName)
	// 确保文件句柄关闭
	defer tempFile.Close()
	// 将上传文件流完整拷贝到临时文件
	if _, err = io.Copy(tempFile, file); err != nil {
		http.Error(w, fmt.Sprintf("ExtractKeyFrames请求[ERROR] -> 保存临时文件失败: %v", err), http.StatusInternalServerError)
		return
	}
	// 创建临时目录用于存储关键帧
	keyFrameDir, err := os.MkdirTemp("", "keyframes_*")
	if err != nil {
		http.Error(w, fmt.Sprintf("ExtractKeyFrames请求[ERROR] -> 创建关键帧目录失败: %v", err), http.StatusInternalServerError)
		return
	}
	// 无论后续逻辑成败，均删除关键帧目录，防止磁盘堆积
	defer os.RemoveAll(keyFrameDir)
	// 调用核心函数提取关键帧，返回 []KeyFrame
	keyFrames, err := ExtractKeyFramesWithLocalCache(tempFileName, keyFrameDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("ExtractKeyFrames请求[ERROR] -> 提取关键帧失败: %v", err), http.StatusInternalServerError)
		return
	}
	// 构造响应体：包含关键帧数组与总数
	response := map[string]any{
		"keyFrames": keyFrames,
		"count":     len(keyFrames),
	}
	// 设置响应头 Content-Type 为 application/json
	w.Header().Set("Content-Type", "application/json")
	// 写入 200 OK 状态码
	w.WriteHeader(http.StatusOK)
	// 将响应体编码为 JSON 并写入 ResponseWriter；若编码失败仅记录日志，不再抛错给客户端
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, fmt.Sprintf("ExtractKeyFrames请求[ERROR] -> 编码响应失败: %v", err), http.StatusInternalServerError)
		return
	}
}

// ExtractFirstFrameHandler 用于处理 HTTP POST 请求，提取上传视频的第一帧。
func ExtractFirstFrameHandler(w http.ResponseWriter, r *http.Request) {
	// 只允许 POST 方法，否则返回 405 Method Not Allowed
	if r.Method != "POST" {
		http.Error(w, "ExtractFirstFrame请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}
	// 解析 multipart 表单，限制最大内存 100 MiB（100 << 20 = 104,857,600 字节）
	err := r.ParseMultipartForm(100 << 20)
	if err != nil {
		http.Error(w, fmt.Sprintf("ExtractFirstFrame请求[ERROR] -> 解析表单失败: %v", err), http.StatusBadRequest)
		return
	}
	// 从表单获取文件字段 "video"，返回 multipart.File 及其文件头信息
	file, handler, err := r.FormFile("video")
	if err != nil {
		http.Error(w, fmt.Sprintf("ExtractFirstFrame请求[ERROR] -> 获取文件失败: %v", err), http.StatusBadRequest)
		return
	}
	// 确保文件句柄及时关闭，防止泄露
	defer file.Close()
	// 检查文件格式是否支持
	if !isSupportedVideoFormat(handler.Filename) {
		http.Error(w, "ExtractFirstFrame请求[ERROR] -> 输入文件必须是支持的视频格式", http.StatusBadRequest)
		return
	}
	// 在系统默认临时目录创建临时文件，模板为 "video_*.mp4"
	tempFile, err := os.CreateTemp("", "video_*.mp4")
	if err != nil {
		http.Error(w, fmt.Sprintf("ExtractFirstFrame请求[ERROR] -> 创建临时文件失败: %v", err), http.StatusInternalServerError)
		return
	}
	tempFileName := tempFile.Name()
	// 无论后续逻辑成败，均删除临时文件，防止磁盘堆积
	defer os.Remove(tempFileName)
	// 确保文件句柄关闭
	defer tempFile.Close()
	// 将上传文件流完整拷贝到临时文件
	if _, err = io.Copy(tempFile, file); err != nil {
		http.Error(w, fmt.Sprintf("ExtractFirstFrame请求[ERROR] -> 保存临时文件失败: %v", err), http.StatusInternalServerError)
		return
	}
	// 提取视频第一帧
	firstFrame, err := ExtractFirstFrame(tempFileName)
	if err != nil {
		http.Error(w, fmt.Sprintf("ExtractFirstFrame请求[ERROR] -> 提取第一帧失败: %v", err), http.StatusInternalServerError)
		return
	}
	// 构造响应体：包含第一帧数据
	response := map[string]any{
		"firstFrame": firstFrame,
	}
	// 设置响应头 Content-Type 为 application/json
	w.Header().Set("Content-Type", "application/json")
	// 写入 200 OK 状态码
	w.WriteHeader(http.StatusOK)
	// 将响应体编码为 JSON 并写入 ResponseWriter；若编码失败仅记录日志，不再抛错给客户端
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, fmt.Sprintf("ExtractFirstFrame请求[ERROR] -> 编码响应失败: %v", err), http.StatusInternalServerError)
		return
	}
}

// ExtractFirstFrame 提取视频的第一帧
func ExtractFirstFrame(inputFile string) (KeyFrame, error) {
	// 创建内存缓冲区
	buf := new(bytes.Buffer)
	// 定义ffmpeg输入参数，从视频开始处提取
	kwargs1 := ffmpeg.KwArgs{"ss": "0"}
	// 定义ffmpeg输出参数，指定提取一帧并编码为mjpeg格式
	kwargs2 := ffmpeg.KwArgs{"vframes": 1, "an": "", "f": "image2pipe", "vcodec": "mjpeg"}
	// 执行ffmpeg命令提取第一帧
	err := ffmpeg.Input(inputFile, kwargs1).Output("pipe:1", kwargs2).WithOutput(buf, os.Stderr).Run()
	if err != nil {
		return KeyFrame{}, fmt.Errorf("提取第一帧失败: %v", err)
	}
	// 检查缓冲区是否为空
	if buf.Len() == 0 {
		return KeyFrame{}, fmt.Errorf("提取的帧数据为空")
	}
	// 解码提取的JPEG图像
	image, err := jpeg.Decode(buf)
	if err != nil {
		return KeyFrame{}, fmt.Errorf("解码图像失败: %v", err)
	}
	// 创建内存缓冲区用于编码
	encodeBuf := new(bytes.Buffer)
	// 优化JPEG编码参数，提高压缩质量和速度
	opt := &jpeg.Options{
		Quality: 85, // 设置适当的质量参数
	}
	// 将图像编码为JPEG格式并写入缓冲区
	if err := jpeg.Encode(encodeBuf, image, opt); err != nil {
		return KeyFrame{}, fmt.Errorf("编码图像失败: %v", err)
	}
	// 创建关键帧结构体
	firstFrame := KeyFrame{
		FilePath:  "first_frame.jpg",
		Timestamp: "00:00:00",
		FrameNum:  1,
		Data:      encodeBuf.Bytes(),
	}
	// 返回第一帧数据
	return firstFrame, nil
}

// FrameData 存储帧数据和时间戳
type FrameData struct {
	Image     image.Image
	Timestamp int
	Error     error
}

// ExtractKeyFramesWithLocalCache 提取视频关键帧并写入本地缓存
func ExtractKeyFramesWithLocalCache(inputFile string, cacheDir string) ([]KeyFrame, error) {
	// 初始化关键帧列表
	var keyFrames []KeyFrame
	// 存储前一帧图像，用于计算帧间差异
	var prevImage image.Image
	// 帧提取间隔，单位为秒
	frameInterval := 1
	// 获取视频时长
	duration, err := getVideoDuration(inputFile)
	// 处理获取时长失败的情况
	if err != nil {
		return nil, fmt.Errorf("获取视频时长失败: %v", err)
	}
	// 检查视频时长是否合理
	if duration > 3600 { // 限制最大处理时长为1小时
		return nil, fmt.Errorf("视频时长过长，最大支持1小时的视频")
	}

	// 计算需要处理的帧数
	frameCount := int(duration) / frameInterval
	if int(duration)%frameInterval > 0 {
		frameCount++
	}

	// 设置工作池大小，根据CPU核心数调整
	workerCount := 4
	if frameCount < workerCount {
		workerCount = frameCount
	}

	// 创建任务通道和结果通道
	taskChan := make(chan int, frameCount)
	resultChan := make(chan FrameData, frameCount)

	// 启动工作池
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for timestamp := range taskChan {
				// 创建缓冲区用于存储提取的帧数据
				buf := new(bytes.Buffer)
				// 使用ffmpeg提取指定时间点的一帧
				err := extractKeyFrames(inputFile, timestamp, buf)
				// 处理提取失败的情况
				if err != nil {
					// 继续处理下一帧，不返回错误
					fmt.Printf("警告: 提取帧失败 %d 秒: %v\n", timestamp, err)
					resultChan <- FrameData{Error: err}
					continue
				}
				// 检查缓冲区是否为空
				if buf.Len() == 0 {
					fmt.Printf("警告: 提取的帧数据为空 %d 秒\n", timestamp)
					resultChan <- FrameData{Error: fmt.Errorf("提取的帧数据为空")}
					continue
				}
				// 解码提取的JPEG图像
				currImage, err := jpeg.Decode(buf)
				// 处理解码失败的情况
				if err != nil {
					// 继续处理下一帧，不返回错误
					fmt.Printf("警告: 解码图像失败: %v\n", err)
					resultChan <- FrameData{Error: err}
					continue
				}
				// 发送帧数据到结果通道
				resultChan <- FrameData{Image: currImage, Timestamp: timestamp}
			}
		}()
	}

	// 发送任务到工作池
	for i := 0; float64(i) < duration; i += frameInterval {
		taskChan <- i
	}
	close(taskChan)

	// 等待所有任务处理完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 收集所有帧数据
	var allFrames []FrameData
	for frameData := range resultChan {
		// 跳过有错误的帧
		if frameData.Error != nil {
			continue
		}
		allFrames = append(allFrames, frameData)
	}

	// 按时间戳排序帧数据
	if len(allFrames) > 0 {
		// 对帧数据按照时间戳从小到大排序
		for i := 0; i < len(allFrames)-1; i++ {
			for j := i + 1; j < len(allFrames); j++ {
				if allFrames[i].Timestamp > allFrames[j].Timestamp {
					allFrames[i], allFrames[j] = allFrames[j], allFrames[i]
				}
			}
		}
	}

	// 处理排序后的帧数据
	for _, frameData := range allFrames {
		// 检查是否不是第一帧
		if prevImage != nil {
			// 计算当前帧与前一帧的差异
			diff := calculateImageDifference(prevImage, frameData.Image)
			// 如果差异小于等于阈值(0.45)，则跳过当前帧
			if diff <= 0.45 {
				continue
			}
			// 创建关键帧文件
			frameFileName, frameDataBytes, err := createKeyframeFile(frameData.Image, cacheDir, keyFrames)
			// 处理创建关键帧文件失败的情况
			if err != nil {
				fmt.Printf("警告: 创建关键帧文件失败: %v\n", err)
				continue
			}
			// 格式化时间戳
			timestampStr := formatTime(frameData.Timestamp)
			// 将当前帧添加到关键帧列表
			keyFrames = append(keyFrames, createKeyFrame(frameFileName, timestampStr, len(keyFrames), frameDataBytes))

		} else {
			// 创建关键帧文件
			frameFileName, frameDataBytes, err := createKeyframeFile(frameData.Image, cacheDir, keyFrames)
			// 处理创建关键帧文件失败的情况
			if err != nil {
				fmt.Printf("警告: 创建关键帧文件失败: %v\n", err)
				continue
			}
			// 格式化时间戳
			timestampStr := formatTime(frameData.Timestamp)
			// 将第一帧添加到关键帧列表
			keyFrames = append(keyFrames, createKeyFrame(frameFileName, timestampStr, len(keyFrames), frameDataBytes))
		}
		// 更新前一帧为当前帧
		prevImage = frameData.Image
	}

	// 检查是否提取到关键帧
	if len(keyFrames) == 0 {
		return nil, fmt.Errorf("未提取到关键帧，请检查视频文件是否正常")
	}
	// 返回提取的关键帧列表
	return keyFrames, nil
}

// getVideoDuration 获取视频时长
func getVideoDuration(inputFile string) (float64, error) {
	// 使用ffprobe获取视频信息
	data, err := ffmpeg.Probe(inputFile)
	// 处理ffprobe执行失败的情况
	if err != nil {
		return 0, err
	}
	// 定义JSON输出结构体
	var output map[string]any
	// 解析ffprobe输出为JSON格式
	if err := json.Unmarshal([]byte(data), &output); err != nil {
		return 0, fmt.Errorf("解析ffprobe输出失败: %v", err)
	}
	// 从format部分提取时长
	if format, ok := output["format"].(map[string]interface{}); ok {
		// 检查format中是否包含duration字段
		if durationStr, ok := format["duration"].(string); ok {
			// 解析时长字符串为浮点数
			duration, err := strconv.ParseFloat(durationStr, 64)
			// 处理解析时长失败的情况
			if err != nil {
				return 0, fmt.Errorf("解析时长失败: %v", err)
			}
			// 处理时长为0的情况
			if duration <= 0 {
				return 0, fmt.Errorf("视频时长为0秒")
			}
			// 返回视频时长
			return duration, nil
		}
	}
	// 处理format中未包含duration字段的情况
	return 0, fmt.Errorf("从ffprobe输出中提取时长失败")
}

// calculateImageDifference 计算两张图像的差异
func calculateImageDifference(img1, img2 image.Image) float64 {
	// 获取图像边界
	bounds1 := img1.Bounds()
	bounds2 := img2.Bounds()
	// 使用较小的边界以避免索引越界
	minWidth := min(bounds1.Dx(), bounds2.Dx())
	minHeight := min(bounds1.Dy(), bounds2.Dy())
	// 检查任一图像是否为空
	if minWidth == 0 || minHeight == 0 {
		return 1.0
	}
	// 计算总像素数
	totalPixels := minWidth * minHeight
	// 初始化总差异
	totalDiff := 0.0
	// 遍历图像1的每个像素
	for y := 0; y < minHeight; y++ {
		// 遍历图像1的每个像素
		for x := 0; x < minWidth; x++ {
			// 获取像素颜色
			c1 := img1.At(x, y)
			c2 := img2.At(x, y)
			// 转换为RGBA
			r1, g1, b1, _ := c1.RGBA()
			r2, g2, b2, _ := c2.RGBA()
			// 计算RGB差异
			rDiff := float64(r1-r2) / 65535.0
			gDiff := float64(g1-g2) / 65535.0
			bDiff := float64(b1-b2) / 65535.0
			// 计算欧几里得距离
			pixelDiff := (rDiff*rDiff + gDiff*gDiff + bDiff*bDiff) / 3.0
			totalDiff += pixelDiff
		}
	}
	// 归一化差异到[0, 1]
	averageDiff := totalDiff / float64(totalPixels)
	// 返回归一化后的差异
	return averageDiff
}

// formatTime 格式化时间
func formatTime(seconds int) string {
	// 处理负数时间
	if seconds <= 0 {
		return "00:00:00"
	}
	// 处理正常时间
	t := time.Duration(seconds) * time.Second
	// 格式化时间为HH:MM:SS
	return fmt.Sprintf("%02d:%02d:%02d", int(t.Hours()), int(t.Minutes())%60, int(t.Seconds())%60)
}

// createKeyframeFile 创建关键帧文件并返回图像数据
func createKeyframeFile(currImage image.Image, cacheDir string, keyFrames []KeyFrame) (string, []byte, error) {
	// 生成关键帧文件名
	frameFileName := fmt.Sprintf("key_frame_%d.jpg", len(keyFrames)+1)
	// 构建完整的文件路径
	framePath := filepath.Join(cacheDir, frameFileName)
	// 将当前帧转换为RGBA格式，确保通道数为4
	rgbaImage := toRGBA(currImage)
	// 调整图像大小，确保宽高都不超过1024
	resizedImage := resizeToFit(rgbaImage, 1024, 1024)

	// 创建内存缓冲区
	buf := new(bytes.Buffer)
	// 优化JPEG编码参数，提高压缩质量和速度
	opt := &jpeg.Options{
		Quality: 85, // 设置适当的质量参数
	}
	// 将调整后的图像编码为JPEG格式并写入缓冲区
	if err := jpeg.Encode(buf, resizedImage, opt); err != nil {
		// 继续处理下一帧，不返回错误
		fmt.Printf("警告: 编码图像失败: %v\n", err)
		return frameFileName, nil, err
	}

	// 仅在需要时写入文件（可选）
	// 如果只是为了生成文件名而不需要实际文件，可以跳过这一步
	frameFile, err := os.Create(framePath)
	if err != nil {
		// 继续处理下一帧，不返回错误
		fmt.Printf("警告: 创建关键帧文件失败: %v\n", err)
		// 即使文件创建失败，也返回内存中的图像数据
		return frameFileName, buf.Bytes(), nil
	}
	defer frameFile.Close()

	// 将缓冲区内容写入文件
	if _, err := frameFile.Write(buf.Bytes()); err != nil {
		// 继续处理下一帧，不返回错误
		fmt.Printf("警告: 写入关键帧文件失败: %v\n", err)
		// 即使写入失败，也返回内存中的图像数据
		return frameFileName, buf.Bytes(), nil
	}

	// 返回关键帧数据和错误
	return frameFileName, buf.Bytes(), nil
}

// extractKeyFrames 提取视频关键帧
func extractKeyFrames(inputFile string, i int, buf *bytes.Buffer) error {
	// 定义ffmpeg输入参数，指定提取关键帧的时间点
	kwargs1 := ffmpeg.KwArgs{"ss": fmt.Sprintf("%d", i)}
	// 定义ffmpeg输出参数，指定提取一帧并编码为mjpeg格式
	kwargs2 := ffmpeg.KwArgs{"vframes": 1, "an": "", "f": "image2pipe", "vcodec": "mjpeg"}
	// 执行ffmpeg命令提取关键帧
	err := ffmpeg.Input(inputFile, kwargs1).Output("pipe:1", kwargs2).WithOutput(buf, os.Stderr).Run()
	// 返回提取关键帧的错误
	return err
}

// createKeyFrame 创建关键帧结构体
func createKeyFrame(frameFileName string, timestamp string, frameNum int, frameData []byte) KeyFrame {
	return KeyFrame{FilePath: frameFileName, Timestamp: timestamp, FrameNum: frameNum, Data: frameData}
}
