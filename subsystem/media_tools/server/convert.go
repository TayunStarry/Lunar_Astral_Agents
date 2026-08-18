// 图片格式转换 HTTP 处理器
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

// writeJSON 写入JSON响应
func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

// ConvertImageHandler 处理单张图片格式转换
// 请求体: {"path": "...", "target_format": "png|jpeg|webp", "delete_source": bool, "quality": int}
func ConvertImageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req module.ConvertImageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, module.ConvertImageResponse{
			Success: false,
			Error:   "无效的请求体",
		})
		return
	}

	if req.Path == "" || req.TargetFormat == "" {
		writeJSON(w, http.StatusBadRequest, module.ConvertImageResponse{
			Success: false,
			Error:   "path 和 target_format 为必填项",
		})
		return
	}

	// 规范化目标格式
	targetFormat := strings.ToLower(req.TargetFormat)
	if targetFormat == "jpg" {
		targetFormat = "jpeg"
	}
	if targetFormat != "png" && targetFormat != "jpeg" && targetFormat != "webp" {
		writeJSON(w, http.StatusBadRequest, module.ConvertImageResponse{
			Success: false,
			Error:   "不支持的目标格式，仅支持 png、jpeg、webp",
		})
		return
	}

	// 设置默认质量
	quality := req.Quality
	if quality <= 0 || quality > 100 {
		quality = 90
	}

	// 解析为本地绝对路径并校验范围
	fullPath := module.ResolvePath(req.Path)
	if !module.IsWithinLocalDir(fullPath) {
		writeJSON(w, http.StatusForbidden, module.ConvertImageResponse{
			Success: false,
			Error:   "访问被拒绝：路径超出本地目录范围",
		})
		return
	}

	// 检查源文件是否存在
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		writeJSON(w, http.StatusBadRequest, module.ConvertImageResponse{
			Success: false,
			Error:   fmt.Sprintf("文件不存在: %s", fullPath),
		})
		return
	}

	// 生成输出路径
	ext := strings.ToLower(filepath.Ext(fullPath))
	outputExt := "." + targetFormat
	if targetFormat == "jpeg" {
		outputExt = ".jpg"
	}
	outputPath := strings.TrimSuffix(fullPath, ext) + outputExt

	// 执行转换
	err := module.ConvertImage(fullPath, outputPath, targetFormat, quality)
	if err != nil {
		LoggerGeneral.Error("ConvertImage", "转换失败 %s: %v", filepath.Base(fullPath), err)
		writeJSON(w, http.StatusInternalServerError, module.ConvertImageResponse{
			Success: false,
			Error:   fmt.Sprintf("转换失败: %v", err),
		})
		return
	}

	// 删除源文件
	if req.DeleteSource {
		if err := os.Remove(fullPath); err != nil {
			LoggerGeneral.Warn("ConvertImage", "删除源文件失败 %s: %v", fullPath, err)
		}
	}

	LoggerGeneral.Info("ConvertImage", "转换成功: %s -> %s", filepath.Base(fullPath), filepath.Base(outputPath))
	writeJSON(w, http.StatusOK, module.ConvertImageResponse{
		Success:    true,
		OutputPath: outputPath,
	})
}

// BatchConvertHandler 处理批量图片格式转换
// 请求体: {"folder": "...", "source_format": "all|png|jpeg|webp", "target_format": "png|jpeg|webp", "delete_source": bool, "quality": int}
func BatchConvertHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req module.BatchConvertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, module.BatchConvertResponse{
			Success: false,
			Error:   "无效的请求体",
		})
		return
	}

	if req.Folder == "" || req.TargetFormat == "" {
		writeJSON(w, http.StatusBadRequest, module.BatchConvertResponse{
			Success: false,
			Error:   "folder 和 target_format 为必填项",
		})
		return
	}

	// 解析为本地绝对路径并校验范围
	folderPath := module.ResolvePath(req.Folder)
	if !module.IsWithinLocalDir(folderPath) {
		writeJSON(w, http.StatusForbidden, module.BatchConvertResponse{
			Success: false,
			Error:   "访问被拒绝：路径超出本地目录范围",
		})
		return
	}

	// 检查文件夹是否存在
	info, err := os.Stat(folderPath)
	if err != nil || !info.IsDir() {
		writeJSON(w, http.StatusBadRequest, module.BatchConvertResponse{
			Success: false,
			Error:   fmt.Sprintf("文件夹不存在: %s", folderPath),
		})
		return
	}

	// 规范化格式
	sourceFormat := strings.ToLower(req.SourceFormat)
	targetFormat := strings.ToLower(req.TargetFormat)
	if targetFormat == "jpg" {
		targetFormat = "jpeg"
	}
	if targetFormat != "png" && targetFormat != "jpeg" && targetFormat != "webp" {
		writeJSON(w, http.StatusBadRequest, module.BatchConvertResponse{
			Success: false,
			Error:   "不支持的目标格式，仅支持 png、jpeg、webp",
		})
		return
	}

	quality := req.Quality
	if quality <= 0 || quality > 100 {
		quality = 90
	}

	// 确定源格式扩展名
	var sourceExts []string
	if sourceFormat == "" || sourceFormat == "all" {
		for ext := range module.SupportedFormats {
			sourceExts = append(sourceExts, ext)
		}
	} else {
		ext := "." + sourceFormat
		if sourceFormat == "jpeg" {
			ext = ".jpg"
		}
		if !module.SupportedFormats[ext] {
			writeJSON(w, http.StatusBadRequest, module.BatchConvertResponse{
				Success: false,
				Error:   "不支持的源格式，仅支持 png、jpg、jpeg、webp",
			})
			return
		}
		if sourceFormat == "jpeg" {
			sourceExts = []string{".jpg", ".jpeg"}
		} else {
			sourceExts = []string{ext}
		}
	}

	// 扫描文件夹
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, module.BatchConvertResponse{
			Success: false,
			Error:   fmt.Sprintf("读取文件夹失败: %v", err),
		})
		return
	}

	var results []module.BatchConvertResult
	successCount := 0
	failCount := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := strings.ToLower(filepath.Ext(entry.Name()))
		matched := false
		for _, se := range sourceExts {
			if ext == se {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}

		inputPath := filepath.Join(folderPath, entry.Name())
		outputExt := "." + targetFormat
		if targetFormat == "jpeg" {
			outputExt = ".jpg"
		}
		outputPath := strings.TrimSuffix(inputPath, ext) + outputExt

		// 跳过源格式与目标格式相同的文件
		if ext == outputExt {
			continue
		}

		err := module.ConvertImage(inputPath, outputPath, targetFormat, quality)
		if err != nil {
			LoggerGeneral.Error("BatchConvert", "转换失败 %s: %v", entry.Name(), err)
			results = append(results, module.BatchConvertResult{
				Path:    inputPath,
				Success: false,
				Error:   err.Error(),
			})
			failCount++
		} else {
			if req.DeleteSource {
				os.Remove(inputPath)
			}
			results = append(results, module.BatchConvertResult{
				Path:       inputPath,
				Success:    true,
				OutputPath: outputPath,
			})
			successCount++
		}
	}

	LoggerGeneral.Info("BatchConvert", "批量转换完成: 成功 %d, 失败 %d", successCount, failCount)
	writeJSON(w, http.StatusOK, module.BatchConvertResponse{
		Success:      true,
		Results:      results,
		Total:        len(results),
		SuccessCount: successCount,
		FailCount:    failCount,
	})
}

// ListImagesHandler 列出文件夹中所有支持的图片文件
// 请求体: {"folder": "..."}
func ListImagesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Folder string `json:"folder"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, module.ListImagesResponse{
			Success: false,
			Error:   "无效的请求体",
		})
		return
	}

	if req.Folder == "" {
		writeJSON(w, http.StatusBadRequest, module.ListImagesResponse{
			Success: false,
			Error:   "folder 为必填项",
		})
		return
	}

	folderPath := module.ResolvePath(req.Folder)
	if !module.IsWithinLocalDir(folderPath) {
		writeJSON(w, http.StatusForbidden, module.ListImagesResponse{
			Success: false,
			Error:   "访问被拒绝：路径超出本地目录范围",
		})
		return
	}

	info, err := os.Stat(folderPath)
	if err != nil || !info.IsDir() {
		writeJSON(w, http.StatusBadRequest, module.ListImagesResponse{
			Success: false,
			Error:   fmt.Sprintf("文件夹不存在: %s", folderPath),
		})
		return
	}

	entries, err := os.ReadDir(folderPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, module.ListImagesResponse{
			Success: false,
			Error:   fmt.Sprintf("读取文件夹失败: %v", err),
		})
		return
	}

	var files []module.ImageFileInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !module.SupportedFormats[ext] {
			continue
		}
		format := ext[1:] // 去掉点号
		if format == "jpg" {
			format = "jpeg"
		}
		files = append(files, module.ImageFileInfo{
			Name:   entry.Name(),
			Path:   filepath.Join(folderPath, entry.Name()),
			Format: format,
		})
	}

	writeJSON(w, http.StatusOK, module.ListImagesResponse{
		Success: true,
		Files:   files,
		Folder:  folderPath,
	})
}
