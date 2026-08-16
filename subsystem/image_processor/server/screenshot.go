package server

import (
	"LunarSubsystem/ImageProcessor/module"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// HandleCapture 处理统一截图请求
// 支持 POST（JSON 请求体，对齐 module.CaptureRequest）与 GET（查询参数）
func HandleCapture(w http.ResponseWriter, r *http.Request) {
	req := module.CaptureRequest{}

	if r.Method == "POST" {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "无效的请求参数", http.StatusBadRequest)
			return
		}
	} else {
		// GET 从查询参数解析
		q := r.URL.Query()
		if m := q.Get("mode"); m != "" {
			req.Mode = module.CaptureMode(m)
		}
		req.DisplayIndex, _ = strconv.Atoi(q.Get("display_index"))
		req.OffsetX, _ = strconv.Atoi(q.Get("offset_x"))
		req.OffsetY, _ = strconv.Atoi(q.Get("offset_y"))
		req.Width, _ = strconv.Atoi(q.Get("width"))
		req.Height, _ = strconv.Atoi(q.Get("height"))
		req.RegionX, _ = strconv.Atoi(q.Get("region_x"))
		req.RegionY, _ = strconv.Atoi(q.Get("region_y"))
		req.RegionW, _ = strconv.Atoi(q.Get("region_w"))
		req.RegionH, _ = strconv.Atoi(q.Get("region_h"))
		req.Format = q.Get("format")
		req.Quality, _ = strconv.Atoi(q.Get("quality"))
		req.Scale = q.Get("scale")
	}

	result, err := module.Capture(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", result.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", captureFilename(result)))
	w.Write(result.Image)
}

// captureFilename 根据截图结果构造文件名
func captureFilename(result module.CaptureResult) string {
	switch result.Mode {
	case module.ModeWindow:
		return fmt.Sprintf("screenshot_window.%s", result.Format)
	case module.ModeDisplay:
		return fmt.Sprintf("screenshot_d%d.%s", result.DisplayIndex, result.Format)
	default:
		return fmt.Sprintf("screenshot.%s", result.Format)
	}
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
