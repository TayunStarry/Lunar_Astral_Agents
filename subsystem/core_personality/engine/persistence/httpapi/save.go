package httpapi

import (
	"CorePersonality/engine/persistence"
	"encoding/json"
	"net/http"
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
	fileName, err := persistence.DecodeFileName(encodedName)
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
	// 强制落盘开关：仅对「新内容」生效；内容哈希已存在时仍按唯一性复用/迁移，
	// 不会因为 X-Store-Exact 制造第二份同哈希实体
	mustStore := false
	if exact := r.Header.Get("X-Store-Exact"); exact != "" {
		if val, parseErr := strconv.ParseBool(exact); parseErr == nil {
			mustStore = val
		}
	}
	// 调用 persistence 保存文件（自动维护句柄映射）
	result, err := persistence.SaveFileEx(fileName, persistence.SaveOptions{Overwrite: overwrite, MustStore: mustStore}, r.Body)
	if err != nil {
		// 根据错误信息返回相应的HTTP错误
		switch err.Error() {
		case "无效的文件名", "访问被拒绝":
			http.Error(w, "Save请求[ERROR] -> "+err.Error(), http.StatusForbidden)
		default:
			http.Error(w, "Save请求[ERROR] -> "+err.Error(), http.StatusInternalServerError)
		}
		return
	}
	// 设置响应状态码为200
	w.WriteHeader(http.StatusOK)
	// 构建响应数据（含句柄，前端可直接引用）
	response := map[string]any{
		"filename":  result.Name,                   // 最终文件名
		"path":      result.Path,                   // 文件的完整路径
		"rel_path":  result.RelPath,                // 相对 LocalDir 的路径
		"handle":    result.Handle,                 // 句柄（内容仓库内文件）
		"deduped":   result.Deduped,                // 是否复用了既有内容实体
		"size":      result.Size,                   // 文件字节数
		"overwrite": strconv.FormatBool(overwrite), // 是否覆盖 existing 文件
	}
	// 将响应数据编码为JSON并发送给客户端
	json.NewEncoder(w).Encode(response)
}
