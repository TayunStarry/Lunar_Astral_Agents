package server

import (
	"LunarSubsystem/FileManager/module"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ArchiveHandler 处理 ZIP 压缩和解压请求
func ArchiveHandler(w http.ResponseWriter, r *http.Request) {
	// 根据请求方法进行分支处理
	switch r.Method {

	case "POST":
		// 处理创建 ZIP 文件的请求
		createZip(w, r)

	case "PUT":
		// 处理解压 ZIP 文件的请求
		extractZip(w, r)

	default:
		// 处理不允许的请求方法，返回错误响应
		http.Error(w, "Archive请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
	}
}

// createZip 创建 ZIP 压缩文件并直接返回给客户端
func createZip(w http.ResponseWriter, r *http.Request) {
	// 解析多部分表单数据，设置最大内存为 32MB
	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		// 若解析失败，返回错误响应给客户端
		http.Error(w, "Archive请求[ERROR] -> 解析表单失败: "+err.Error(), http.StatusBadRequest)
		return
	}
	// 从表单中获取名为 "files" 的文件列表
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		// 若未选择文件，返回错误响应给客户端
		http.Error(w, "Archive请求[ERROR] -> 未选择文件", http.StatusBadRequest)
		return
	}
	// 从表单中获取 ZIP 文件名，若未提供则使用默认名 "archive.zip"
	zipName := r.FormValue("zip_name")
	if zipName == "" {
		zipName = "archive.zip"
	}
	// 调用 execute 模块创建 ZIP 文件
	zipData, err := module.CreateZip(files, zipName)
	if err != nil {
		// 若创建失败，返回错误响应给客户端
		http.Error(w, "Archive请求[ERROR] -> "+err.Error(), http.StatusInternalServerError)
		return
	}
	// 确保 ZIP 文件名以 ".zip" 结尾
	if !strings.HasSuffix(strings.ToLower(zipName), ".zip") {
		zipName += ".zip"
	}
	// 若提供了 save_path（相对 LocalDir 的目录路径），则将 ZIP 保存到服务器并返回 JSON
	savePath := r.FormValue("save_path")
	if savePath != "" {
		saveDir := filepath.Clean(filepath.Join(*GeneralConfig.LocalDir, savePath))
		if !strings.HasPrefix(saveDir, filepath.Clean(*GeneralConfig.LocalDir)) {
			http.Error(w, "Archive请求[ERROR] -> 保存路径越界", http.StatusBadRequest)
			return
		}
		if err := os.MkdirAll(saveDir, 0755); err != nil {
			http.Error(w, "Archive请求[ERROR] -> 创建保存目录失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		fullSavePath := filepath.Join(saveDir, zipName)
		if err := os.WriteFile(fullSavePath, zipData, 0644); err != nil {
			http.Error(w, "Archive请求[ERROR] -> 保存ZIP文件失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		relPath, relErr := filepath.Rel(*GeneralConfig.LocalDir, fullSavePath)
		if relErr != nil {
			relPath = zipName
		}
		relPath = filepath.ToSlash(relPath)
		LoggerGeneral.SubInfo("FileManager", "Archive", "ZIP已保存到服务器: %s (%d 字节)", fullSavePath, len(zipData))
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"path":    relPath,
			"name":    zipName,
			"size":    len(zipData),
		})
		return
	}
	// 设置响应头，指定响应内容类型为 ZIP 文件
	w.Header().Set("Content-Type", "application/zip")
	// 设置响应头，指定文件下载时的文件名
	w.Header().Set("Content-Disposition", "attachment; filename="+zipName)
	// 设置响应头，指定响应内容的长度
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(zipData)))
	// 将 ZIP 文件数据写入响应
	_, err = w.Write(zipData)
	if err != nil {
		// 若发送文件失败，返回错误响应给客户端
		http.Error(w, "Archive请求[ERROR] -> 发送ZIP文件失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

// CreateZipHandler 服务端压缩：根据相对 LocalDir 的路径列表在服务器本地创建 ZIP 并保存
// 支持文件与文件夹（递归打包，保留目录层级），返回压缩包相对路径
func CreateZipHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持 POST 请求", http.StatusMethodNotAllowed)
		return
	}

	var req module.CreateZipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "请求体解析失败: " + err.Error()})
		return
	}
	if len(req.Paths) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "未选择文件"})
		return
	}

	zipName := req.ZipName
	if zipName == "" {
		zipName = "archive.zip"
	}
	if !strings.HasSuffix(strings.ToLower(zipName), ".zip") {
		zipName += ".zip"
	}

	localDir := filepath.Clean(*GeneralConfig.LocalDir)
	zipData, err := module.CreateZipFromPaths(localDir, req.Paths, zipName)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": err.Error()})
		return
	}

	// 保存目录（相对 LocalDir，空则保存到 LocalDir 根目录）
	saveDir := localDir
	if req.SavePath != "" {
		saveDir = filepath.Clean(filepath.Join(localDir, filepath.FromSlash(req.SavePath)))
		if saveDir != localDir && !strings.HasPrefix(saveDir, localDir+string(os.PathSeparator)) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "保存路径越界"})
			return
		}
	}
	if err := os.MkdirAll(saveDir, 0755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "error": "创建保存目录失败: " + err.Error()})
		return
	}
	fullSavePath := filepath.Join(saveDir, zipName)
	if err := os.WriteFile(fullSavePath, zipData, 0644); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "error": "保存ZIP文件失败: " + err.Error()})
		return
	}
	relPath, relErr := filepath.Rel(localDir, fullSavePath)
	if relErr != nil {
		relPath = zipName
	}
	relPath = filepath.ToSlash(relPath)
	LoggerGeneral.SubInfo("FileManager", "Archive", "服务端ZIP已保存: %s (%d 字节)", fullSavePath, len(zipData))
	writeJSON(w, http.StatusOK, module.ZipCreateResponse{
		Success: true,
		Path:    relPath,
		Name:    zipName,
		Size:    len(zipData),
	})
}

// extractZip 解压 ZIP 文件并返回文件列表给客户端
func extractZip(w http.ResponseWriter, r *http.Request) {
	// 解析多部分表单，设置最大内存为 32MB
	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		// 若解析失败，返回错误响应给客户端
		http.Error(w, "Archive请求[ERROR] -> 解析表单失败: "+err.Error(), http.StatusBadRequest)
		return
	}
	// 从表单中获取名为 "zip_file" 的 ZIP 文件
	file, header, err := r.FormFile("zip_file")
	if err != nil {
		// 若获取文件失败，返回错误响应给客户端
		http.Error(w, "Archive请求[ERROR] -> 获取ZIP文件失败: "+err.Error(), http.StatusBadRequest)
		return
	}
	// 函数结束时关闭文件，防止资源泄漏
	defer file.Close()
	// 调用 execute 模块解压 ZIP 文件
	extractedFiles, _, err := module.ExtractZip(file)
	if err != nil {
		// 若解压失败，返回错误响应给客户端
		http.Error(w, "Archive请求[ERROR] -> "+err.Error(), http.StatusInternalServerError)
		return
	}
	// 设置响应头，指定响应内容类型为 JSON
	w.Header().Set("Content-Type", "application/json")
	// 设置响应状态码为 200 OK
	w.WriteHeader(http.StatusOK)
	// 构建响应数据
	response := map[string]any{
		"total_files":     len(extractedFiles), // 解压出的文件总数
		"extracted_files": extractedFiles,      // 解压出的文件信息列表
		"original_zip":    header.Filename,     // 原始 ZIP 文件名
	}
	// 将响应数据编码为 JSON 并写入响应
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Archive请求[ERROR] -> 生成响应失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

// InstallPackageHandler 处理 .ltpx 包安装请求
// 将上传的归档文件解压并安装到 local_data/package/<包名>/ 目录下
func InstallPackageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持 POST 请求", http.StatusMethodNotAllowed)
		return
	}

	// 解析多部分表单，设置最大内存为 128MB
	err := r.ParseMultipartForm(128 << 20)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(PackageInstallResponse{
			Success: false,
			Message: "解析上传文件失败: " + err.Error(),
		})
		return
	}

	// 获取上传的包文件（支持 zip_file 和 package_file 两种表单字段名）
	file, header, err := r.FormFile("package_file")
	if err != nil {
		file, header, err = r.FormFile("zip_file")
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(PackageInstallResponse{
				Success: false,
				Message: "未找到上传的文件",
			})
			return
		}
	}
	defer file.Close()

	// 验证文件扩展名
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".ltpx" && ext != ".zip" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(PackageInstallResponse{
			Success: false,
			Message: "不支持的文件类型: " + ext + "，仅支持 .ltpx、.zip",
		})
		return
	}

	// 使用文件名（不含扩展名）作为包名
	packageName := strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
	if packageName == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(PackageInstallResponse{
			Success: false,
			Message: "无效的文件名",
		})
		return
	}

	// 解压归档文件
	extractedFiles, _, err := module.ExtractZip(file)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(PackageInstallResponse{
			Success: false,
			Message: "解压归档文件失败: " + err.Error(),
		})
		return
	}

	if len(extractedFiles) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(PackageInstallResponse{
			Success: false,
			Message: "归档文件中没有可提取的文件",
		})
		return
	}

	// 验证归档根目录下是否存在 metadata.json
	var metadataRaw []byte
	var hasMetadata bool
	metadataPrefix := packageName + "/metadata.json"
	altMetadataPrefix := "metadata.json"

	for _, ef := range extractedFiles {
		name, _ := ef["name"].(string)
		if name == metadataPrefix || name == altMetadataPrefix {
			if content, ok := ef["content"].([]byte); ok {
				metadataRaw = content
				hasMetadata = true
			}
			break
		}
	}

	if !hasMetadata {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(PackageInstallResponse{
			Success: false,
			Message: fmt.Sprintf("包 '%s' 缺少有效的 metadata.json 文件", packageName),
		})
		return
	}

	// 解析并验证 metadata.json 格式
	var metadata struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Icon        string   `json:"icon,omitempty"`
		URL         string   `json:"url,omitempty"`
		Path        string   `json:"path,omitempty"`
		Tags        []string `json:"tags,omitempty"`
	}
	if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(PackageInstallResponse{
			Success: false,
			Message: fmt.Sprintf("metadata.json 格式无效: %v", err),
		})
		return
	}

	if metadata.ID == "" || metadata.Title == "" || metadata.Description == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(PackageInstallResponse{
			Success: false,
			Message: "metadata.json 缺少必填字段（id、title、description）",
		})
		return
	}

	// 确定目标安装目录
	packageDir := filepath.Join(*GeneralConfig.LocalDir, "package", packageName)

	// 如果目标目录已存在，先删除
	if _, statErr := os.Stat(packageDir); statErr == nil {
		if removeErr := os.RemoveAll(packageDir); removeErr != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(PackageInstallResponse{
				Success: false,
				Message: "清理旧包目录失败: " + removeErr.Error(),
			})
			return
		}
	}

	// 创建目标目录
	if mkdirErr := os.MkdirAll(packageDir, 0755); mkdirErr != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(PackageInstallResponse{
			Success: false,
			Message: "创建包目录失败: " + mkdirErr.Error(),
		})
		return
	}

	// 写入所有解压的文件
	for _, ef := range extractedFiles {
		name, _ := ef["name"].(string)
		content, _ := ef["content"].([]byte)
		if name == "" {
			continue
		}

		// 处理文件名：去除可能的包名前缀
		// 如果文件名以 "<packageName>/" 开头，去掉这个前缀
		prefix := packageName + "/"
		relativeName := name
		if strings.HasPrefix(name, prefix) {
			relativeName = strings.TrimPrefix(name, prefix)
		}
		if relativeName == "" {
			continue
		}

		// 确定文件的完整路径
		filePath := filepath.Join(packageDir, relativeName)

		// 确保父目录存在
		parentDir := filepath.Dir(filePath)
		if mkdirErr := os.MkdirAll(parentDir, 0755); mkdirErr != nil {
			LoggerGeneral.Error("FileManager", "创建父目录失败 %s: %v", parentDir, mkdirErr)
			continue
		}

		// 写入文件
		if writeErr := os.WriteFile(filePath, content, 0644); writeErr != nil {
			LoggerGeneral.Error("FileManager", "写入文件失败 %s: %v", filePath, writeErr)
			continue
		}
	}

	LoggerGeneral.Info("FileManager", "包安装成功: %s (ID: %s, 标题: %s)", packageName, metadata.ID, metadata.Title)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(PackageInstallResponse{
		Success:      true,
		Message:      fmt.Sprintf("包 '%s' 安装成功", metadata.Title),
		PackageName:  packageName,
		PackageID:    metadata.ID,
		PackageTitle: metadata.Title,
	})
}

// ExportPackageHandler 处理包导出（打包为 .ltpx）请求
func ExportPackageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持 POST 请求", http.StatusMethodNotAllowed)
		return
	}

	var req ExportPackageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "解析请求失败: " + err.Error(),
		})
		return
	}

	if req.PackageName == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "包名不能为空",
		})
		return
	}

	// 构建包目录路径
	packageDir := filepath.Join(*GeneralConfig.LocalDir, "package", req.PackageName)

	// 检查目录是否存在
	if _, statErr := os.Stat(packageDir); os.IsNotExist(statErr) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("包 '%s' 不存在", req.PackageName),
		})
		return
	}

	// 打包目录
	zipData, err := module.PackageDirZip(packageDir, req.PackageName)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "打包失败: " + err.Error(),
		})
		return
	}

	fileName := req.PackageName + ".ltpx"

	switch req.Action {
	case "save":
		// 保存到指定目录
		savePath := req.SavePath
		if savePath == "" {
			savePath = filepath.Join(*GeneralConfig.LocalDir, "package", "archive")
		}
		// 确保是相对路径
		savePath = strings.TrimPrefix(savePath, *GeneralConfig.LocalDir)
		savePath = strings.TrimPrefix(savePath, "/")
		savePath = strings.TrimPrefix(savePath, "\\")

		fullSavePath := filepath.Join(*GeneralConfig.LocalDir, savePath, fileName)
		if mkdirErr := os.MkdirAll(filepath.Dir(fullSavePath), 0755); mkdirErr != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"message": "创建保存目录失败: " + mkdirErr.Error(),
			})
			return
		}

		if writeErr := os.WriteFile(fullSavePath, zipData, 0644); writeErr != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"message": "保存文件失败: " + writeErr.Error(),
			})
			return
		}

		LoggerGeneral.Info("FileManager", "包导出成功: %s -> %s", req.PackageName, fullSavePath)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":   true,
			"message":   fmt.Sprintf("包 '%s' 已保存到 %s", req.PackageName, fullSavePath),
			"save_path": fullSavePath,
			"file_name": fileName,
			"file_size": len(zipData),
		})

	default:
		// 下载到本地（默认行为）
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileName))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(zipData)))
		w.WriteHeader(http.StatusOK)
		w.Write(zipData)
	}
}

// DeletePackageHandler 处理包删除请求
func DeletePackageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持 POST 请求", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		PackageName string `json:"package_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "解析请求失败: " + err.Error(),
		})
		return
	}

	if req.PackageName == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "包名不能为空",
		})
		return
	}

	// 防止路径穿越
	packageName := filepath.Clean(req.PackageName)
	if strings.Contains(packageName, "..") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "无效的包名",
		})
		return
	}

	packageDir := filepath.Join(*GeneralConfig.LocalDir, "package", packageName)

	// 安全检查：确保在 package 目录下
	if !strings.HasPrefix(filepath.Clean(packageDir), filepath.Clean(filepath.Join(*GeneralConfig.LocalDir, "package"))) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "访问被拒绝",
		})
		return
	}

	if _, statErr := os.Stat(packageDir); os.IsNotExist(statErr) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("包 '%s' 不存在", packageName),
		})
		return
	}

	if err := os.RemoveAll(packageDir); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "删除失败: " + err.Error(),
		})
		return
	}

	LoggerGeneral.Info("FileManager", "包删除成功: %s", packageName)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"message":      fmt.Sprintf("包 '%s' 已删除", packageName),
		"package_name": packageName,
	})
}

// ZipMetadataHandler 查询服务器上 ZIP 文件的元数据
func ZipMetadataHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持 POST 请求", http.StatusMethodNotAllowed)
		return
	}

	var req module.ZipMetadataRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "请求体解析失败: " + err.Error()})
		return
	}
	if req.Path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "路径不能为空"})
		return
	}

	// 校验路径在 LocalDir 内且为 .zip 文件
	fullPath := filepath.Clean(filepath.Join(*GeneralConfig.LocalDir, filepath.FromSlash(req.Path)))
	localDir := filepath.Clean(*GeneralConfig.LocalDir)
	if !strings.HasPrefix(fullPath, localDir) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "路径越界"})
		return
	}
	if !strings.EqualFold(filepath.Ext(fullPath), ".zip") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "仅支持 .zip 压缩包"})
		return
	}
	stat, err := os.Stat(fullPath)
	if err != nil || stat.IsDir() {
		writeJSON(w, http.StatusNotFound, map[string]any{"success": false, "error": "文件不存在"})
		return
	}

	entries, err := module.ReadZipFileList(fullPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}

	var totalSize int64
	for _, entry := range entries {
		if !entry.IsDir {
			totalSize += entry.Size
		}
	}
	relPath, _ := filepath.Rel(localDir, fullPath)
	writeJSON(w, http.StatusOK, module.ZipMetadataResponse{
		Success:   true,
		Path:      filepath.ToSlash(relPath),
		FileCount: len(entries),
		TotalSize: totalSize,
		ZipSize:   stat.Size(),
		Entries:   entries,
	})
}

// ExtractZipHandler 将服务器上的 ZIP 文件解压到指定目录
func ExtractZipHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持 POST 请求", http.StatusMethodNotAllowed)
		return
	}

	var req module.ExtractZipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "请求体解析失败: " + err.Error()})
		return
	}
	if req.Path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "路径不能为空"})
		return
	}

	localDir := filepath.Clean(*GeneralConfig.LocalDir)
	// 校验 ZIP 路径在 LocalDir 内且为 .zip 文件
	zipPath := filepath.Clean(filepath.Join(localDir, filepath.FromSlash(req.Path)))
	if !strings.HasPrefix(zipPath, localDir) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "ZIP路径越界"})
		return
	}
	if strings.ToLower(filepath.Ext(zipPath)) != ".zip" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "仅支持 .zip 压缩包"})
		return
	}
	if stat, err := os.Stat(zipPath); err != nil || stat.IsDir() {
		writeJSON(w, http.StatusNotFound, map[string]any{"success": false, "error": "文件不存在"})
		return
	}

	// 校验目标目录在 LocalDir 内
	targetDir := filepath.Clean(filepath.Join(localDir, filepath.FromSlash(req.TargetDir)))
	if !strings.HasPrefix(targetDir, localDir) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "解压路径越界"})
		return
	}

	fileCount, err := module.ExtractZipToDir(zipPath, targetDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}

	relTarget, _ := filepath.Rel(localDir, targetDir)
	LoggerGeneral.SubInfo("FileManager", "Archive", "ZIP解压完成: %s -> %s (%d 个文件)", zipPath, targetDir, fileCount)
	writeJSON(w, http.StatusOK, module.ExtractZipResponse{
		Success:   true,
		TargetDir: filepath.ToSlash(relTarget),
		FileCount: fileCount,
	})
}
