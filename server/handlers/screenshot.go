package handlers

import (
	"Lunar-Astral-Agents/server/config" // 导入项目配置包，用于获取配置信息
	"bytes"                             // 字节操作包，用于处理字节数据
	"encoding/base64"
	"encoding/json" // JSON编码/解码包，用于处理JSON数据
	"fmt"           // 格式化输出包，用于字符串格式化
	"image"         // 图像操作包，用于处理图像数据
	"image/draw"    // 图像绘制包，用于绘制图像
	"image/jpeg"    // JPEG图像编码/解码包，用于处理JPEG图像
	"image/png"     // PNG图像编码/解码包，用于处理PNG图像
	"net/http"      // HTTP协议包，用于处理HTTP请求/响应
	"strconv"       // 字符串转换包，用于字符串到数字的转换
	"strings"       // 字符串操作包，用于字符串处理
	"sync"          // 同步包，用于并发编程
	"time"          // 时间包，用于处理时间

	"github.com/disintegration/imaging" // 图像处理库，用于图像缩放等操作
	"github.com/kbinani/screenshot"     // 屏幕截图库，用于截取屏幕图像
)

// 截图请求参数
type CaptureRequest struct {
	DisplayIndex int    `json:"display_index"` // -1表示所有显示器
	Region       string `json:"region"`        // "x,y,width,height"
	Scale        string `json:"scale"`         // "width,height" 或 "0.5"
	Format       string `json:"format"`        // png, jpg, jpeg
	Quality      int    `json:"quality"`       // JPEG质量 1-100
}

// 截图互斥锁
var screenshotMutex sync.RWMutex

// 最后截图时间和频率限制
var (
	lastCapture     time.Time               // 最后截图时间
	captureCooldown = 50 * time.Millisecond // 最小截图间隔
)

// 处理截图请求
func HandleCapture(w http.ResponseWriter, r *http.Request) {
	// 检查频率限制
	if err := checkCaptureRateLimit(); err != nil {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}

	var req CaptureRequest

	// 解析请求参数
	if r.Method == "POST" {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "无效的请求参数", http.StatusBadRequest)
			return
		}
	} else {
		// GET请求从查询参数获取
		req.DisplayIndex, _ = strconv.Atoi(r.URL.Query().Get("display"))
		req.Region = r.URL.Query().Get("region")
		req.Scale = r.URL.Query().Get("scale")
		req.Format = r.URL.Query().Get("format")
		req.Quality, _ = strconv.Atoi(r.URL.Query().Get("quality"))
	}

	// 使用默认值
	if req.Format == "" {
		req.Format = *config.Format
	}
	if req.Quality == 0 {
		req.Quality = *config.JPEGQuality
	}

	// 执行截图
	imgData, filename, err := captureScreenshot(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 设置响应头
	contentType := getContentType(req.Format)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filename))

	// 返回图片数据
	w.Write(imgData)
}

// 截图特定显示器
func HandleCaptureDisplay(w http.ResponseWriter, r *http.Request) {
	// 检查频率限制
	if err := checkCaptureRateLimit(); err != nil {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, "无效的URL", http.StatusBadRequest)
		return
	}

	displayIndex, err := strconv.Atoi(parts[3])
	if err != nil || displayIndex < 0 {
		http.Error(w, "无效的显示器索引", http.StatusBadRequest)
		return
	}

	req := CaptureRequest{
		DisplayIndex: displayIndex,
		Format:       r.URL.Query().Get("format"),
		Scale:        r.URL.Query().Get("scale"),
		Quality:      *config.JPEGQuality,
	}

	if req.Format == "" {
		req.Format = *config.Format
	}

	imgData, filename, err := captureScreenshot(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	contentType := getContentType(req.Format)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filename))
	w.Write(imgData)
}

// 处理区域截图
func HandleCaptureRegion(w http.ResponseWriter, r *http.Request) {
	// 检查频率限制
	if err := checkCaptureRateLimit(); err != nil {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}

	region := r.URL.Query().Get("region")
	scale := r.URL.Query().Get("scale")
	format := r.URL.Query().Get("format")
	quality, _ := strconv.Atoi(r.URL.Query().Get("quality"))

	if region == "" {
		http.Error(w, "需要region参数 (x,y,width,height)", http.StatusBadRequest)
		return
	}

	req := CaptureRequest{
		Region:  region,
		Scale:   scale,
		Format:  format,
		Quality: quality,
	}

	if req.Format == "" {
		req.Format = *config.Format
	}
	if req.Quality == 0 {
		req.Quality = *config.JPEGQuality
	}

	imgData, filename, err := captureScreenshot(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	contentType := getContentType(req.Format)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filename))
	w.Write(imgData)
}

// 获取所有显示器信息
func HandleGetDisplays(w http.ResponseWriter, r *http.Request) {
	n := screenshot.NumActiveDisplays()
	displays := make([]map[string]int, n)

	for i := range n {
		bounds := screenshot.GetDisplayBounds(i)
		displays[i] = map[string]int{
			"index":  i,
			"x":      bounds.Min.X,
			"y":      bounds.Min.Y,
			"width":  bounds.Dx(),
			"height": bounds.Dy(),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(displays)
}

// 核心截图函数
func captureScreenshot(req CaptureRequest) ([]byte, string, error) {
	var img *image.RGBA
	var err error

	// 获取显示器数量
	displayCount := screenshot.NumActiveDisplays()

	// 根据参数决定截图方式
	if req.Region != "" {
		// 区域截图
		rect, err2 := parseRegion(req.Region)
		if err2 != nil {
			return nil, "", err2
		}
		img, err = screenshot.CaptureRect(rect)
	} else if req.DisplayIndex >= 0 && req.DisplayIndex < displayCount {
		// 指定显示器
		img, err = screenshot.CaptureDisplay(req.DisplayIndex)
	} else if req.DisplayIndex == -1 && displayCount > 1 {
		// 所有显示器拼接
		img, err = captureAllDisplaysOptimized()
	} else {
		// 默认第一个显示器
		img, err = screenshot.CaptureDisplay(0)
	}

	if err != nil {
		return nil, "", fmt.Errorf("截图失败: %v", err)
	}

	// 缩放处理
	img, err = applyScale(img, req.Scale)
	if err != nil {
		return nil, "", fmt.Errorf("缩放失败: %v", err)
	}

	// 编码图片
	buf := &bytes.Buffer{}
	var filename string

	if req.DisplayIndex >= 0 {
		filename = fmt.Sprintf("screenshot_d%d.%s", req.DisplayIndex, req.Format)
	} else {
		filename = fmt.Sprintf("screenshot.%s", req.Format)
	}

	if err := encodeImage(buf, img, req.Format, req.Quality); err != nil {
		return nil, "", err
	}

	// 更新最后截图时间
	screenshotMutex.Lock()
	lastCapture = time.Now()
	screenshotMutex.Unlock()

	return buf.Bytes(), filename, nil
}

// 解析区域字符串
func parseRegion(regionStr string) (image.Rectangle, error) {
	parts := strings.Split(regionStr, ",")
	if len(parts) != 4 {
		return image.Rectangle{}, fmt.Errorf("区域格式应为 'x,y,width,height'")
	}

	x, _ := strconv.Atoi(parts[0])
	y, _ := strconv.Atoi(parts[1])
	width, _ := strconv.Atoi(parts[2])
	height, _ := strconv.Atoi(parts[3])

	if width <= 0 || height <= 0 {
		return image.Rectangle{}, fmt.Errorf("宽高必须大于0")
	}

	return image.Rect(x, y, x+width, y+height), nil
}

// 截取所有显示器并拼接（优化版）
func captureAllDisplaysOptimized() (*image.RGBA, error) {
	n := screenshot.NumActiveDisplays()
	if n == 0 {
		return nil, fmt.Errorf("未找到显示器")
	}

	// 获取所有显示器的边界
	displays := make([]image.Rectangle, n)
	for i := range n {
		displays[i] = screenshot.GetDisplayBounds(i)
	}

	// 计算总边界
	minX, minY := displays[0].Min.X, displays[0].Min.Y
	maxX, maxY := displays[0].Max.X, displays[0].Max.Y

	for i := 1; i < n; i++ {
		bounds := displays[i]
		if bounds.Min.X < minX {
			minX = bounds.Min.X
		}
		if bounds.Min.Y < minY {
			minY = bounds.Min.Y
		}
		if bounds.Max.X > maxX {
			maxX = bounds.Max.X
		}
		if bounds.Max.Y > maxY {
			maxY = bounds.Max.Y
		}
	}

	totalWidth := maxX - minX
	totalHeight := maxY - minY

	// 创建大图
	img := image.NewRGBA(image.Rect(0, 0, totalWidth, totalHeight))

	// 使用 draw.Draw 合并每个显示器的截图
	for i, bounds := range displays {
		displayImg, err := screenshot.CaptureDisplay(i)
		if err != nil {
			// 记录错误但继续处理其他显示器
			fmt.Printf("截取显示器 %d 失败: %v\n", i, err)
			continue
		}

		// 计算在总图中的位置
		dx := bounds.Min.X - minX
		dy := bounds.Min.Y - minY

		// 使用 draw.Draw 复制图像，提高性能
		draw.Draw(
			img,
			image.Rect(dx, dy, dx+bounds.Dx(), dy+bounds.Dy()),
			displayImg,
			image.Point{0, 0},
			draw.Src,
		)
	}

	return img, nil
}

// 将 NRGBA 转换为 RGBA
func toRGBA(img image.Image) *image.RGBA {
	// 如果已经是 RGBA，直接返回
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}
	// 创建新的 RGBA 图像
	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)

	// 使用 draw.Draw 复制像素，提高性能
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)
	return rgba
}

// 缩放图片
func resizeImage(img *image.RGBA, scaleStr string) (*image.RGBA, error) {
	// 如果包含逗号，表示指定宽高
	if strings.Contains(scaleStr, ",") {
		parts := strings.Split(scaleStr, ",")
		if len(parts) != 2 {
			return nil, fmt.Errorf("缩放格式应为 'width,height' 或 '0.5'")
		}
		width, _ := strconv.Atoi(parts[0])
		height, _ := strconv.Atoi(parts[1])

		if width <= 0 || height <= 0 {
			return nil, fmt.Errorf("缩放宽高必须大于0")
		}

		resized := imaging.Resize(img, width, height, imaging.Lanczos)
		return toRGBA(resized), nil
	}

	// 否则是比例
	scale, err := strconv.ParseFloat(scaleStr, 64)
	if err != nil {
		return nil, fmt.Errorf("无效的缩放比例: %v", err)
	}

	if scale <= 0 {
		return nil, fmt.Errorf("缩放比例必须大于0")
	}

	newWidth := int(float64(img.Bounds().Dx()) * scale)
	newHeight := int(float64(img.Bounds().Dy()) * scale)

	// 限制最小尺寸
	if newWidth < 1 || newHeight < 1 {
		newWidth = 1
		newHeight = 1
	}

	resized := imaging.Resize(img, newWidth, newHeight, imaging.Lanczos)
	return toRGBA(resized), nil
}

// 缩放到合适大小
func resizeToFit(img *image.RGBA, maxWidth, maxHeight int) *image.RGBA {
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()

	if width <= maxWidth && height <= maxHeight {
		return img
	}

	ratio := float64(width) / float64(height)

	if width > maxWidth {
		width = maxWidth
		height = int(float64(width) / ratio)
	}

	if height > maxHeight {
		height = maxHeight
		width = int(float64(height) * ratio)
	}

	resized := imaging.Resize(img, width, height, imaging.Lanczos)
	return toRGBA(resized)
}

// 应用缩放处理
func applyScale(img *image.RGBA, scaleStr string) (*image.RGBA, error) {
	if scaleStr != "" {
		return resizeImage(img, scaleStr)
	}

	// 使用配置的最大尺寸限制
	return resizeToFit(img, *config.MaxWidth, *config.MaxHeight), nil
}

// 编码图片
func encodeImage(buf *bytes.Buffer, img *image.RGBA, format string, quality int) error {
	switch strings.ToLower(format) {
	case "jpg", "jpeg":
		if quality < 1 || quality > 100 {
			quality = *config.JPEGQuality // 默认质量
		}
		return jpeg.Encode(buf, img, &jpeg.Options{Quality: quality})
	case "png":
		return png.Encode(buf, img)
	default:
		return fmt.Errorf("不支持的图片格式: %s", format)
	}
}

// 获取内容类型
func getContentType(format string) string {
	switch strings.ToLower(format) {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	default:
		return "image/png"
	}
}

// 检查截图频率限制
func checkCaptureRateLimit() error {
	screenshotMutex.RLock()
	timeSinceLastCapture := time.Since(lastCapture)
	screenshotMutex.RUnlock()

	if timeSinceLastCapture < captureCooldown {
		return fmt.Errorf("截图过于频繁，请等待 %.1f 秒", float64(captureCooldown-timeSinceLastCapture)/float64(time.Second))
	}
	return nil
}

// 缩放图片到最大尺寸1080
func resizeImageTo1080(img *image.RGBA) *image.RGBA {
	return resizeToFit(img, 1080, 1080)
}

// 处理图片缩放请求
func HandleResizeImage(w http.ResponseWriter, r *http.Request) {
	// 只允许POST方法
	if r.Method != "POST" {
		http.Error(w, "只允许POST方法", http.StatusMethodNotAllowed)
		return
	}

	// 解析multipart表单
	file, _, err := r.FormFile("image")
	if err != nil {
		http.Error(w, fmt.Sprintf("获取文件失败: %v", err), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// 解码图片
	img, format, err := image.Decode(file)
	if err != nil {
		http.Error(w, fmt.Sprintf("解码图片失败: %v", err), http.StatusBadRequest)
		return
	}

	// 转换为RGBA格式
	rgbaImg := toRGBA(img)

	// 缩放图片
	resizedImg := resizeImageTo1080(rgbaImg)

	// 编码缩放后的图片
	buf := &bytes.Buffer{}
	var contentType string
	switch format {
	case "jpeg":
		contentType = "image/jpeg"
		jpeg.Encode(buf, resizedImg, &jpeg.Options{Quality: 90})
	case "png":
		contentType = "image/png"
		png.Encode(buf, resizedImg)
	default:
		contentType = "image/jpeg"
		jpeg.Encode(buf, resizedImg, &jpeg.Options{Quality: 90})
	}

	// 生成base64编码
	base64Data := base64.StdEncoding.EncodeToString(buf.Bytes())
	base64WithHeader := fmt.Sprintf("data:%s;base64,%s", contentType, base64Data)

	// 构造响应
	response := map[string]interface{}{
		"image":  buf.Bytes(),
		"base64": base64WithHeader,
		"format": format,
		"width":  resizedImg.Bounds().Dx(),
		"height": resizedImg.Bounds().Dy(),
	}

	// 设置响应头
	w.Header().Set("Content-Type", "application/json")

	// 编码并返回响应
	json.NewEncoder(w).Encode(response)
}
