package server

import (
	"LunarSubsystem/FileManager/module"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// ReadHandler 处理文件读取请求，从本地目录读取文件并返回给客户端
func ReadHandler(w http.ResponseWriter, r *http.Request) {
	// 检查请求方法是否为 GET，如果不是则返回 405 Method Not Allowed 错误
	if r.Method != "GET" {
		http.Error(w, "Read请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}
	// 从请求 URL 路径中去除 "/file/read/" 前缀，获取实际的文件路径
	filePath := strings.TrimPrefix(r.URL.Path, "/file/read/")
	// 调用 execute 模块读取文件
	file, size, mimeType, err := module.ReadFile(filePath)
	if err != nil {
		// 根据错误信息返回相应的HTTP错误
		switch err.Error() {
		case "未指定文件路径":
			http.Error(w, "Read请求[ERROR] -> 未指定文件路径", http.StatusBadRequest)
		case "访问被拒绝":
			http.Error(w, "Read请求[ERROR] -> 访问被拒绝", http.StatusForbidden)
		case "文件未找到":
			http.Error(w, "Read请求[ERROR] -> 文件未找到", http.StatusNotFound)
		case "获取文件信息失败":
			http.Error(w, "Read请求[ERROR] -> 获取文件信息失败", http.StatusInternalServerError)
		case "不允许读取目录":
			http.Error(w, "Read请求[ERROR] -> 不允许读取目录", http.StatusBadRequest)
		case "打开文件失败":
			http.Error(w, "Read请求[ERROR] -> 打开文件失败", http.StatusInternalServerError)
		default:
			http.Error(w, "Read请求[ERROR] -> "+err.Error(), http.StatusInternalServerError)
		}
		return
	}
	// 确保文件在使用后关闭
	defer file.Close()
	// 设置 Content-Type 头信息
	w.Header().Set("Content-Type", mimeType)
	// 声明支持范围请求，供媒体播放器 seek 使用
	w.Header().Set("Accept-Ranges", "bytes")
	// 若携带 Range 头，则按范围返回 206 Partial Content
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		start, end, ok := parseRange(rangeHeader, size)
		if !ok {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", size))
			http.Error(w, "Read请求[ERROR] -> 范围请求无法满足", http.StatusRequestedRangeNotSatisfiable)
			return
		}
		// 计算返回长度
		length := end - start + 1
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, size))
		w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
		w.WriteHeader(http.StatusPartialContent)
		// 定位到起始偏移并复制指定长度
		if seeker, ok := file.(io.Seeker); ok {
			if _, err := seeker.Seek(start, io.SeekStart); err != nil {
				return
			}
		}
		io.CopyN(w, file, length)
		return
	}
	// 设置 Content-Length 头信息，值为文件大小
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	// 将文件内容复制到响应中返回给客户端
	io.Copy(w, file)
}

// parseRange 解析 HTTP Range 请求头（仅支持单范围），返回起止偏移
// 支持 bytes=start-end、bytes=start-、bytes=-suffix 三种格式
func parseRange(rangeHeader string, size int64) (start, end int64, ok bool) {
	// 仅支持 bytes 单位
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		return 0, 0, false
	}
	// 多范围请求不支持，仅取第一个范围
	spec := strings.TrimPrefix(rangeHeader, "bytes=")
	if idx := strings.Index(spec, ","); idx >= 0 {
		spec = spec[:idx]
	}
	dashIdx := strings.Index(spec, "-")
	if dashIdx < 0 {
		return 0, 0, false
	}
	startStr := strings.TrimSpace(spec[:dashIdx])
	endStr := strings.TrimSpace(spec[dashIdx+1:])
	if startStr == "" {
		// 后缀范围：bytes=-N 表示最后 N 字节
		suffix, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil || suffix <= 0 {
			return 0, 0, false
		}
		if suffix > size {
			suffix = size
		}
		return size - suffix, size - 1, true
	}
	start, err := strconv.ParseInt(startStr, 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false
	}
	if endStr == "" {
		// 开放范围：bytes=start- 表示从 start 到文件末尾
		return start, size - 1, true
	}
	end, err = strconv.ParseInt(endStr, 10, 64)
	if err != nil || end < start {
		return 0, 0, false
	}
	if end >= size {
		end = size - 1
	}
	return start, end, true
}
