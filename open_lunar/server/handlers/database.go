package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"open-lunar/config"
	"open-lunar/library"
)

// DatabaseHandler 统一的数据库处理器
func DatabaseHandler(w http.ResponseWriter, r *http.Request) {
	// 检查请求方法
	if r.Method != "POST" {
		http.Error(w, "数据库请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	// 解析请求体
	var req library.DatabaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("数据库请求[ERROR] -> 解析请求失败: %v", err), http.StatusBadRequest)
		return
	}

	// 执行批量操作
	result := library.ExecuteDatabaseRequest(req)

	// 设置响应头
	w.Header().Set("Content-Type", "application/json")

	// 返回结果
	if err := json.NewEncoder(w).Encode(result); err != nil {
		http.Error(w, fmt.Sprintf("数据库请求[ERROR] -> 编码响应失败: %v", err), http.StatusInternalServerError)
		return
	}

	// 记录日志
	if *config.DevMode {
		log.Printf("数据库批量操作成功，执行 %d 个操作，耗时 %dms",
			result.Operations, result.TotalTime)
	}
}
