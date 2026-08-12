package main

import (
	logger "LunarSubsystem/LoggerGeneral"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/chai2010/webp"
)

// convertImageHandler 处理单张图片格式转换
func convertImageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ConvertImageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ConvertImageResponse{
			Success: false,
			Error:   "无效的请求体",
		})
		return
	}

	if req.Path == "" || req.TargetFormat == "" {
		writeJSON(w, http.StatusBadRequest, ConvertImageResponse{
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
		writeJSON(w, http.StatusBadRequest, ConvertImageResponse{
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

	// 检查源文件是否存在
	if _, err := os.Stat(req.Path); os.IsNotExist(err) {
		writeJSON(w, http.StatusBadRequest, ConvertImageResponse{
			Success: false,
			Error:   fmt.Sprintf("文件不存在: %s", req.Path),
		})
		return
	}

	// 生成输出路径
	ext := strings.ToLower(filepath.Ext(req.Path))
	outputExt := "." + targetFormat
	if targetFormat == "jpeg" {
		outputExt = ".jpg"
	}
	outputPath := strings.TrimSuffix(req.Path, ext) + outputExt

	// 执行转换
	err := convertImage(req.Path, outputPath, targetFormat, quality)
	if err != nil {
		logger.Error("ConvertImage", "转换失败 %s: %v", filepath.Base(req.Path), err)
		writeJSON(w, http.StatusInternalServerError, ConvertImageResponse{
			Success: false,
			Error:   fmt.Sprintf("转换失败: %v", err),
		})
		return
	}

	// 删除源文件
	if req.DeleteSource {
		if err := os.Remove(req.Path); err != nil {
			logger.Warn("ConvertImage", "删除源文件失败 %s: %v", req.Path, err)
		}
	}

	logger.Info("ConvertImage", "转换成功: %s -> %s", filepath.Base(req.Path), filepath.Base(outputPath))
	writeJSON(w, http.StatusOK, ConvertImageResponse{
		Success:    true,
		OutputPath: outputPath,
	})
}

// batchConvertHandler 处理批量图片格式转换
func batchConvertHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BatchConvertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, BatchConvertResponse{
			Success: false,
			Error:   "无效的请求体",
		})
		return
	}

	if req.Folder == "" || req.TargetFormat == "" {
		writeJSON(w, http.StatusBadRequest, BatchConvertResponse{
			Success: false,
			Error:   "folder 和 target_format 为必填项",
		})
		return
	}

	// 检查文件夹是否存在
	info, err := os.Stat(req.Folder)
	if err != nil || !info.IsDir() {
		writeJSON(w, http.StatusBadRequest, BatchConvertResponse{
			Success: false,
			Error:   fmt.Sprintf("文件夹不存在: %s", req.Folder),
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
		writeJSON(w, http.StatusBadRequest, BatchConvertResponse{
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
		for ext := range supportedFormats {
			sourceExts = append(sourceExts, ext)
		}
	} else {
		ext := "." + sourceFormat
		if sourceFormat == "jpeg" {
			ext = ".jpg"
		}
		if !supportedFormats[ext] {
			writeJSON(w, http.StatusBadRequest, BatchConvertResponse{
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
	entries, err := os.ReadDir(req.Folder)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, BatchConvertResponse{
			Success: false,
			Error:   fmt.Sprintf("读取文件夹失败: %v", err),
		})
		return
	}

	var results []BatchConvertResult
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

		inputPath := filepath.Join(req.Folder, entry.Name())
		outputExt := "." + targetFormat
		if targetFormat == "jpeg" {
			outputExt = ".jpg"
		}
		outputPath := strings.TrimSuffix(inputPath, ext) + outputExt

		// 跳过源格式与目标格式相同的文件
		if ext == outputExt {
			continue
		}

		err := convertImage(inputPath, outputPath, targetFormat, quality)
		if err != nil {
			logger.Error("BatchConvert", "转换失败 %s: %v", entry.Name(), err)
			results = append(results, BatchConvertResult{
				Path:    inputPath,
				Success: false,
				Error:   err.Error(),
			})
			failCount++
		} else {
			if req.DeleteSource {
				os.Remove(inputPath)
			}
			results = append(results, BatchConvertResult{
				Path:       inputPath,
				Success:    true,
				OutputPath: outputPath,
			})
			successCount++
		}
	}

	logger.Info("BatchConvert", "批量转换完成: 成功 %d, 失败 %d", successCount, failCount)
	writeJSON(w, http.StatusOK, BatchConvertResponse{
		Success:      true,
		Results:      results,
		Total:        len(results),
		SuccessCount: successCount,
		FailCount:    failCount,
	})
}

// listImagesHandler 列出文件夹中所有支持的图片文件
func listImagesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Folder string `json:"folder"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ListImagesResponse{
			Success: false,
			Error:   "无效的请求体",
		})
		return
	}

	if req.Folder == "" {
		writeJSON(w, http.StatusBadRequest, ListImagesResponse{
			Success: false,
			Error:   "folder 为必填项",
		})
		return
	}

	info, err := os.Stat(req.Folder)
	if err != nil || !info.IsDir() {
		writeJSON(w, http.StatusBadRequest, ListImagesResponse{
			Success: false,
			Error:   fmt.Sprintf("文件夹不存在: %s", req.Folder),
		})
		return
	}

	entries, err := os.ReadDir(req.Folder)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ListImagesResponse{
			Success: false,
			Error:   fmt.Sprintf("读取文件夹失败: %v", err),
		})
		return
	}

	var files []ImageFileInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !supportedFormats[ext] {
			continue
		}
		format := ext[1:] // 去掉点号
		if format == "jpg" {
			format = "jpeg"
		}
		files = append(files, ImageFileInfo{
			Name:   entry.Name(),
			Path:   filepath.Join(req.Folder, entry.Name()),
			Format: format,
		})
	}

	writeJSON(w, http.StatusOK, ListImagesResponse{
		Success: true,
		Files:   files,
		Folder:  req.Folder,
	})
}

// convertImage 执行图片格式转换
func convertImage(inputPath, outputPath, targetFormat string, quality int) error {
	// 打开源文件
	inputFile, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("无法打开源文件: %v", err)
	}
	defer inputFile.Close()

	// 解码图片
	img, _, err := image.Decode(inputFile)
	if err != nil {
		return fmt.Errorf("无法解码图片: %v", err)
	}

	// 创建输出文件
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("无法创建输出文件: %v", err)
	}
	defer outputFile.Close()

	// 根据目标格式编码
	switch targetFormat {
	case "png":
		err = png.Encode(outputFile, img)
	case "jpeg":
		err = jpeg.Encode(outputFile, img, &jpeg.Options{Quality: quality})
	case "webp":
		err = webp.Encode(outputFile, img, &webp.Options{Quality: float32(quality)})
	default:
		return fmt.Errorf("不支持的目标格式: %s", targetFormat)
	}

	if err != nil {
		return fmt.Errorf("编码失败: %v", err)
	}

	return nil
}

// writeJSON 写入JSON响应
func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}
