package file

import (
	"encoding/json"
	"net/http"
	"open-lunar/file_system"
	"strconv"
)

// SaveHandler 处理文件保存的HTTP请求
func SaveHandler(w http.ResponseWriter, r *http.Request) {
	// 检查请求方法是否为POST，若不是则返回错误
	if r.Method != "POST" {
		http.Error(w, "Save请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}
	// 从请求头中获取编码后的文件名
	encodedName := r.Header.Get("X-File-Name")
	// 调用 execute 模块解码文件名
	fileName, err := file_system.DecodeFileName(encodedName)
	if err != nil {
		// 根据错误信息返回相应的HTTP错误
		switch err.Error() {
		case "缺少文件名":
			http.Error(w, "Save请求[ERROR] -> 缺少文件名", http.StatusBadRequest)
		case "文件名解码错误":
			http.Error(w, "Save请求[ERROR] -> 文件名解码错误", http.StatusBadRequest)
		case "文件名解码后为空":
			http.Error(w, "Save请求[ERROR] -> 文件名解码后为空", http.StatusBadRequest)
		default:
			http.Error(w, "Save请求[ERROR] -> "+err.Error(), http.StatusBadRequest)
		}
		return
	}
	// 检查文件名是否为 "." 或 ".."，防止目录遍历攻击
	if fileName == "." || fileName == ".." {
		http.Error(w, "Save请求[ERROR] -> 无效的文件名", http.StatusForbidden)
		return
	}
	// 初始化覆盖标志，默认为不覆盖
	overwrite := false
	// 从请求头中获取覆盖标志
	if ow := r.Header.Get("X-Overwrite"); ow != "" {
		// 尝试将覆盖标志转换为布尔值
		if val, parseErr := strconv.ParseBool(ow); parseErr == nil {
			overwrite = val
		}
	}
	// 调用 execute 模块保存文件
	savedFileName, fullPath, err := file_system.SaveFile(fileName, overwrite, r.Body)
	if err != nil {
		// 根据错误信息返回相应的HTTP错误
		switch err.Error() {
		case "无效的文件名":
			http.Error(w, "Save请求[ERROR] -> 无效的文件名", http.StatusForbidden)
		default:
			http.Error(w, "Save请求[ERROR] -> "+err.Error(), http.StatusInternalServerError)
		}
		return
	}
	// 设置响应状态码为200
	w.WriteHeader(http.StatusOK)
	// 构建响应数据
	response := map[string]string{
		"filename":  savedFileName,                 // 上传的文件名
		"path":      fullPath,                      // 文件的完整路径
		"overwrite": strconv.FormatBool(overwrite), // 是否覆盖 existing 文件
	}
	// 将响应数据编码为JSON并发送给客户端
	json.NewEncoder(w).Encode(response)
}
