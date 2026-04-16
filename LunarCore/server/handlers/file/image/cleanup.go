package image

import (
	"LunarCore/hierarchy/image"
	"encoding/json"
	"log"
	"net/http"
)

// CleanupUnreferencedImagesHandler 清理未被引用的图片
func CleanupUnreferencedImagesHandler(w http.ResponseWriter, r *http.Request) {
	// 检查请求方法是否为DELETE
	if r.Method != "DELETE" {
		http.Error(w, "Cleanup请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	// 执行清理操作
	result, err := image.CleanupUnreferencedImages()
	if err != nil {
		http.Error(w, "Cleanup请求[ERROR] -> 清理失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 返回清理结果
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Printf("Cleanup请求[ERROR] -> 编码响应失败: %v", err)
	}
}
