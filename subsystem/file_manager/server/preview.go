package server

import (
	"LunarSubsystem/file_manager/module"
	"io"
	"net/http"
	"strconv"
)

// PreviewHandler 处理文件预览请求
// 支持 GET 方法（通过查询参数传递路径，用于 img 标签直接渲染）
// 和 POST 方法（通过 JSON body 传递路径，用于 API 调用）
//
// GET 请求示例:
//
//	GET /file/preview?path=C:/images/photo.png
//
// POST 请求示例:
//
//	POST /file/preview
//	Content-Type: application/json
//	{"path": "C:/images/photo.png"}
//
// 响应:
//
//	成功: 返回文件二进制内容，Content-Type 根据文件类型设置
//	失败: 返回 HTTP 错误状态码和错误信息
func PreviewHandler(w http.ResponseWriter, r *http.Request) {
	var filePath string

	switch r.Method {
	case "GET":
		// 从查询参数中获取路径
		filePath = r.URL.Query().Get("path")
	case "POST":
		// 从请求体中解析路径（兼容 JSON 格式）
		// 由于可能存在很大的 JSON body，仅读取路径字段
		// 这里简单处理：直接读取 query 参数优先，其次尝试读取 body
		filePath = r.URL.Query().Get("path")
		if filePath == "" {
			// 简单解析：读取 body 并提取 path 字段（不依赖 encoding/json 大库）
			body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
			if err == nil && len(body) > 0 {
				filePath = extractJSONPath(body)
			}
		}
	default:
		http.Error(w, "Preview请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	if filePath == "" {
		http.Error(w, "Preview请求[ERROR] -> 未指定文件路径", http.StatusBadRequest)
		return
	}

	// 调用 module 层执行预览读取
	file, size, mimeType, _, err := module.PreviewFile(filePath)
	if err != nil {
		switch err.Error() {
		case "未指定文件路径":
			http.Error(w, "Preview请求[ERROR] -> 未指定文件路径", http.StatusBadRequest)
		case "路径包含非法字符":
			http.Error(w, "Preview请求[ERROR] -> 路径包含非法字符", http.StatusForbidden)
		case "文件未找到":
			http.Error(w, "Preview请求[ERROR] -> 文件未找到", http.StatusNotFound)
		case "获取文件信息失败":
			http.Error(w, "Preview请求[ERROR] -> 获取文件信息失败", http.StatusInternalServerError)
		case "不允许读取目录":
			http.Error(w, "Preview请求[ERROR] -> 不允许读取目录", http.StatusBadRequest)
		case "文件大小超过限制 (最大 500MB)":
			http.Error(w, "Preview请求[ERROR] -> 文件大小超过限制 (最大 500MB)", http.StatusRequestEntityTooLarge)
		case "不允许的文件类型":
			http.Error(w, "Preview请求[ERROR] -> 不允许的文件类型", http.StatusForbidden)
		case "打开文件失败":
			http.Error(w, "Preview请求[ERROR] -> 打开文件失败", http.StatusInternalServerError)
		default:
			http.Error(w, "Preview请求[ERROR] -> "+err.Error(), http.StatusInternalServerError)
		}
		return
	}
	defer file.Close()

	// 设置响应头
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	// 允许跨域（用于前端 img 标签加载）
	w.Header().Set("Access-Control-Allow-Origin", "*")
	// 设置缓存控制（图片缓存 1 小时）
	w.Header().Set("Cache-Control", "public, max-age=3600")

	// 将文件内容写入响应
	io.Copy(w, file)
}

// extractJSONPath 从 JSON 字节中提取 "path" 字段的值
// 这是一个轻量级的解析，避免引入完整的 JSON 解析开销
func extractJSONPath(data []byte) string {
	// 查找 "path" 键
	content := string(data)
	// 简单的手动解析：查找 "path": 后的字符串值
	keyIdx := -1
	for i := 0; i < len(content)-6; i++ {
		if content[i] == '"' && content[i+1] == 'p' && content[i+2] == 'a' &&
			content[i+3] == 't' && content[i+4] == 'h' && content[i+5] == '"' {
			keyIdx = i + 6
			break
		}
	}
	if keyIdx == -1 {
		return ""
	}

	// 跳过冒号和空白
	for keyIdx < len(content) && (content[keyIdx] == ':' || content[keyIdx] == ' ' || content[keyIdx] == '\t' || content[keyIdx] == '\n' || content[keyIdx] == '\r') {
		keyIdx++
	}

	if keyIdx >= len(content) || content[keyIdx] != '"' {
		return ""
	}
	keyIdx++ // 跳过第一个引号

	// 提取字符串值
	start := keyIdx
	for keyIdx < len(content) && content[keyIdx] != '"' {
		if content[keyIdx] == '\\' && keyIdx+1 < len(content) {
			keyIdx++ // 跳过转义字符
		}
		keyIdx++
	}

	return content[start:keyIdx]
}
