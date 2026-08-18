// GGUF 元数据解析 HTTP 处理器
package server

import (
	"LunarSubsystem/LoggerGeneral"
	"LunarSubsystem/MediaTools/module"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// GGUFMetadataHandler 处理 GGUF 模型元数据解析请求
// 请求体: {"filePath": "绝对路径或相对 LocalDir 的路径"}
func GGUFMetadataHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req module.GGUFMetadataRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, module.GGUFMetadataResponse{
			Success: false,
			Error:   "无效的请求体",
		})
		return
	}

	if req.FilePath == "" {
		writeJSON(w, http.StatusBadRequest, module.GGUFMetadataResponse{
			Success: false,
			Error:   "文件路径不能为空",
		})
		return
	}

	// 文件类型校验：仅支持 .gguf 扩展名（大小写不敏感）
	ext := strings.ToLower(filepath.Ext(req.FilePath))
	if ext != ".gguf" {
		writeJSON(w, http.StatusBadRequest, module.GGUFMetadataResponse{
			Success: false,
			Error:   "仅支持 .gguf 格式的模型文件，当前路径扩展名为: " + ext,
		})
		return
	}

	// 解析为本地绝对路径并校验范围
	fullPath := module.ResolvePath(req.FilePath)
	if !module.IsWithinLocalDir(fullPath) {
		writeJSON(w, http.StatusForbidden, module.GGUFMetadataResponse{
			Success: false,
			Error:   "访问被拒绝：路径超出本地目录范围",
		})
		return
	}

	// 检查文件是否存在
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		writeJSON(w, http.StatusNotFound, module.GGUFMetadataResponse{
			Success: false,
			Error:   fmt.Sprintf("文件不存在: %s", fullPath),
		})
		return
	}

	fileName := filepath.Base(fullPath)
	LoggerGeneral.Info("GGUF", "正在解析 GGUF 文件: %s", fullPath)

	metadata, err := module.ParseGGUFFile(fullPath)
	if err != nil {
		LoggerGeneral.Error("GGUF", "解析失败: %v", err)
		writeJSON(w, http.StatusUnprocessableEntity, module.GGUFMetadataResponse{
			Success: false,
			Error:   fmt.Sprintf("GGUF 解析失败: %v", err),
		})
		return
	}

	LoggerGeneral.Info("GGUF", "解析成功: %s (%d 项元数据)", fileName, len(metadata))

	// 转换为 JSON 友好的格式
	jsonMetadata := make(map[string]string, len(metadata))
	for key, value := range metadata {
		jsonMetadata[key] = module.FormatGGUFValue(value)
	}

	summary := module.ExtractGGUFSummary(metadata, fileName)

	writeJSON(w, http.StatusOK, module.GGUFMetadataResponse{
		Success:  true,
		FileName: fileName,
		FilePath: fullPath,
		Summary:  summary,
		Metadata: jsonMetadata,
		Count:    len(metadata),
	})
}
