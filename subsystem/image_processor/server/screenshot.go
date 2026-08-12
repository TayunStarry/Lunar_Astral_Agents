package server

import (
	"LunarSubsystem/ImageProcessor/module"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// HandleScreenshot 处理截图请求
func HandleScreenshot(w http.ResponseWriter, r *http.Request) {
	var req module.ScreenshotRequest

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

	// 执行截图
	imgData, filename, contentType, err := module.Screenshot(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 设置响应头
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filename))

	// 返回图片数据
	w.Write(imgData)
}

// HandleScreenshotDisplay 截图特定显示器
func HandleScreenshotDisplay(w http.ResponseWriter, r *http.Request) {
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

	req := module.ScreenshotRequest{
		DisplayIndex: displayIndex,
		Format:       r.URL.Query().Get("format"),
		Scale:        r.URL.Query().Get("scale"),
		Quality:      0, // 使用默认值
	}

	// 执行截图
	imgData, filename, contentType, err := module.Screenshot(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filename))
	w.Write(imgData)
}

// HandleScreenshotRegion 处理区域截图
func HandleScreenshotRegion(w http.ResponseWriter, r *http.Request) {
	region := r.URL.Query().Get("region")
	scale := r.URL.Query().Get("scale")
	format := r.URL.Query().Get("format")
	quality, _ := strconv.Atoi(r.URL.Query().Get("quality"))

	if region == "" {
		http.Error(w, "需要region参数 (x,y,width,height)", http.StatusBadRequest)
		return
	}

	req := module.ScreenshotRequest{
		Region:  region,
		Scale:   scale,
		Format:  format,
		Quality: quality,
	}

	// 执行截图
	imgData, filename, contentType, err := module.Screenshot(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filename))
	w.Write(imgData)
}

// HandleGetDisplays 获取所有显示器信息
func HandleGetDisplays(w http.ResponseWriter, r *http.Request) {
	displays := module.GetDisplays()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(displays)
}

// HandleResizeImage 处理图片缩放请求，返回图片数据数组（动态图多帧，静态图单帧）
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

	// 读取文件内容
	imgData := make([]byte, 1024*1024*10) // 10MB 缓冲区
	n, err := file.Read(imgData)
	if err != nil {
		http.Error(w, fmt.Sprintf("读取文件失败: %v", err), http.StatusBadRequest)
		return
	}
	imgData = imgData[:n]

	// 执行图片缩放
	response, err := module.ResizeImage(imgData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 设置响应头
	w.Header().Set("Content-Type", "application/json")

	// 编码并返回响应
	json.NewEncoder(w).Encode(response)
}
