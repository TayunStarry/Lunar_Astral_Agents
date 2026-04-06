package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"open-lunar/image"
	"os"
)

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
	if !image.IsSupportedVideoFormat(handler.Filename) {
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
	keyFrames, err := image.ExtractKeyFramesWithLocalCache(tempFileName, keyFrameDir)
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
	if !image.IsSupportedVideoFormat(handler.Filename) {
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
	firstFrame, err := image.ExtractFirstFrame(tempFileName)
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
