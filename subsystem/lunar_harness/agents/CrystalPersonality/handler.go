package CrystalPersonality

import (
	"LunarSubsystem/LunarHarness/clients"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// LoadApplicationHandler 处理加载应用的 HTTP 请求
func LoadApplicationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoadApplicationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Path == "" {
		http.Error(w, "Path is required", http.StatusBadRequest)
		return
	}

	ext := strings.ToLower(filepath.Ext(req.Path))
	var cmd *exec.Cmd

	execPath, err := os.Executable()
	if err != nil {
		http.Error(w, "Failed to get executable path", http.StatusInternalServerError)
		return
	}
	execDir := filepath.Dir(execPath)

	if strings.HasPrefix(req.Path, "/") {
		req.Path = filepath.Join(execDir, req.Path[1:])
	}

	switch ext {
	case ".exe":
		cmd = exec.Command(req.Path)
	case ".ps1":
		cmd = exec.Command("powershell", "-NoExit", "-ExecutionPolicy", "Bypass", "-File", req.Path)
	case ".bat":
		cmd = exec.Command("cmd", "/c", "start", "", req.Path)
	default:
		http.Error(w, "Unsupported file type: "+ext, http.StatusBadRequest)
		return
	}

	if !filepath.IsAbs(req.Path) {
		absPath, err := filepath.Abs(req.Path)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(LoadApplicationResponse{
				Success: false,
				Message: fmt.Sprintf("Failed to resolve absolute path: %v", err),
			})
			return
		}
		req.Path = absPath
	}

	if _, err := os.Stat(req.Path); os.IsNotExist(err) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(LoadApplicationResponse{
			Success: false,
			Message: fmt.Sprintf("File not found: %s", req.Path),
		})
		return
	}

	cmd.Dir = filepath.Dir(req.Path)

	if err := cmd.Start(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(LoadApplicationResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to start application: %v", err),
		})
		return
	}

	logger.Info("CrystalAstral", "Application started: %s", req.Path)

	go func() {
		if err := cmd.Wait(); err != nil {
			logger.Error("CrystalAstral", "Application %s exited with error: %v", req.Path, err)
		} else {
			logger.Info("CrystalAstral", "Application %s exited successfully", req.Path)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(LoadApplicationResponse{
		Success: true,
		Message: fmt.Sprintf("Application started: %s", req.Path),
	})
}

// ScanPackagesHandler 扫描包目录，自动发现所有包含 metadata.json 的子文件夹
func ScanPackagesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	execPath, err := os.Executable()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to get executable path: %v", err),
		})
		return
	}
	execDir := filepath.Dir(execPath)
	packageDir := filepath.Join(execDir, config.LocalDir, "package")

	entries, err := os.ReadDir(packageDir)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to read package directory: %v", err),
		})
		return
	}

	var packages []PackageInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		configPath := filepath.Join(packageDir, entry.Name(), "metadata.json")
		data, err := os.ReadFile(configPath)
		if err != nil {
			// 没有 metadata.json 的子文件夹跳过（库/资源文件夹）
			continue
		}

		var pkg PackageInfo
		if err := json.Unmarshal(data, &pkg); err != nil {
			logger.Warn("CrystalAstral", "解析包配置失败 %s: %v", configPath, err)
			continue
		}

		// 设置包目录名
		pkg.PackageName = entry.Name()

		// 如果图标是相对路径，解析为完整 URL
		if pkg.Icon != "" && !strings.HasPrefix(pkg.Icon, "http") && !strings.HasPrefix(pkg.Icon, "/") {
			pkg.Icon = "/file/read/package/" + entry.Name() + "/" + pkg.Icon
		}

		// 如果未指定 url，自动生成默认入口：包内 index.html 优先，
		// index.html 不存在时回退 README.md（前端 openPage 识别 .md 后经 tool_viewer 渲染）
		if pkg.URL == "" && pkg.Path == "" {
			dir := filepath.Join(packageDir, entry.Name())
			suffix := "index.html"
			if !fileExists(filepath.Join(dir, "index.html")) {
				for _, cand := range []string{"README.md", "readme.md"} {
					if fileExists(filepath.Join(dir, cand)) {
						suffix = cand
						break
					}
				}
			}
			pkg.URL = "/file/read/package/" + entry.Name() + "/" + suffix
		}

		packages = append(packages, pkg)
	}

	// 内置包：随二进制发布在 clients/crystal/<name>/metadata.json，不出现在扩展包目录扫描中
	packages = append(packages, loadInternalPackages()...)

	// 按标题排序，保证输出稳定
	sort.Slice(packages, func(i, j int) bool {
		return packages[i].ID < packages[j].ID
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(packages)
}

// loadInternalPackages 读取内嵌在琉璃前端里的内置包元数据
//
// 内置包随二进制发布在 clients/crystal/<name>，URL 直接指向 /desktop/<name>/index.html；
// Internal=true 供前端过滤，不把它们放进桌面网格（由常驻侧边栏按钮进入）。
func loadInternalPackages() []PackageInfo {
	out := []PackageInfo{}
	entries, err := fs.ReadDir(clients.EmbeddedFiles, "crystal")
	if err != nil {
		return out
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := entry.Name()
		metaPath := path.Join("crystal", dir, "metadata.json")
		data, err := fs.ReadFile(clients.EmbeddedFiles, metaPath)
		if err != nil {
			continue
		}
		var pkg PackageInfo
		if err := json.Unmarshal(data, &pkg); err != nil {
			logger.Warn("CrystalAstral", "解析内置包配置失败 %s: %v", metaPath, err)
			continue
		}
		pkg.PackageName = dir
		pkg.Internal = true
		if pkg.Icon != "" && !strings.HasPrefix(pkg.Icon, "http") && !strings.HasPrefix(pkg.Icon, "/") {
			pkg.Icon = "/desktop/" + dir + "/" + pkg.Icon
		}
		if pkg.URL == "" && pkg.Path == "" {
			pkg.URL = "/desktop/" + dir + "/index.html"
		}
		out = append(out, pkg)
	}
	return out
}

// writeJSON 写入JSON响应
func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}
