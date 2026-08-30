package main

import (
	"LunarSubsystem/FileManager/module"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"archive/zip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ============================================================================
// 创建模块（/api/module/create）
// 在琉璃前端「创建模块」弹窗中，将用户输入的 URL/本地路径 或 上传的 ZIP
// 生成一个完整的扩展包（local_data/package/<包名>/）。
// 支持三种来源：
//   1. 外部链接（http/https）      → metadata.url
//   2. 本地 HTML 文件/目录         → 复制为 index.html（本地 HTML 项目）
//   3. 本地程序（exe/ps1/bat/lnk） → metadata.path
//   4. ZIP 上传                    → 解压到包目录（本地 HTML 项目）
// 开启 mini-LTP 时：
//   · 在 index.html 中内联注入通用页面操作智能体（mini_ltp_agent.js）+ AtoA 适配
//   · metadata.tags 追加 LTPX、mini-LTP
//   · metadata.tools 提供「自然语言指令」工具，供月华 AtoA 调用
// ============================================================================

// sanitizePackageName 规范化包目录名：仅保留 [a-zA-Z0-9._-]，防止路径穿越
func sanitizePackageName(name string) string {
	name = strings.TrimSpace(name)
	re := regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
	name = re.ReplaceAllString(name, "-")
	name = strings.Trim(name, ".-")
	return name
}

// sanitizeToolSlug 将包 ID 转为合法的工具名片段（小写、下划线）
func sanitizeToolSlug(id string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9_]+`)
	return strings.ToLower(re.ReplaceAllString(id, "_"))
}

// packageBaseDir 计算包目录根路径（可执行目录/local_data/package）
func packageBaseDir() string {
	execPath, err := os.Executable()
	if err != nil {
		return filepath.Join("local_data", "package")
	}
	execDir := filepath.Dir(execPath)
	return filepath.Join(execDir, *GeneralConfig.LocalDir, "package")
}

// parseModuleCreateRequest 解析创建请求：优先 multipart（ZIP 上传），否则 JSON
func parseModuleCreateRequest(r *http.Request) (ModuleCreateRequest, error) {
	var req ModuleCreateRequest
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(128 << 20); err != nil {
			return req, fmt.Errorf("解析表单失败: %v", err)
		}
		if dataStr := r.FormValue("data"); dataStr != "" {
			if err := json.Unmarshal([]byte(dataStr), &req); err != nil {
				return req, fmt.Errorf("解析 data 字段失败: %v", err)
			}
		}
		// ZIP 文件（临时文件由 moduleCreateHandler 解压后清理）
		if file, _, err := r.FormFile("zip_file"); err == nil {
			defer file.Close()
			tmp, err := os.CreateTemp("", "module_upload_*.zip")
			if err != nil {
				return req, fmt.Errorf("创建临时文件失败: %v", err)
			}
			tmpPath := tmp.Name()
			if _, err := io.Copy(tmp, file); err != nil {
				tmp.Close()
				os.Remove(tmpPath)
				return req, fmt.Errorf("读取上传文件失败: %v", err)
			}
			tmp.Close()
			req.ZipPath = tmpPath
		}
		return req, nil
	}
	// JSON 模式
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, fmt.Errorf("解析 JSON 请求失败: %v", err)
	}
	return req, nil
}

// moduleCreateHandler POST /api/module/create
func moduleCreateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	req, err := parseModuleCreateRequest(r)
	if err != nil {
		jsonOK(w, http.StatusBadRequest, ModuleCreateResponse{Success: false, Message: err.Error()})
		return
	}

	// ---- 1. 包目录名 ----
	packageName := sanitizePackageName(req.PackageName)
	if packageName == "" {
		packageName = sanitizePackageName(req.Title)
	}
	if packageName == "" {
		packageName = sanitizePackageName(req.ID)
	}
	if packageName == "" {
		jsonOK(w, http.StatusBadRequest, ModuleCreateResponse{Success: false, Message: "无法确定包名，请填写标题、ID 或包名"})
		return
	}

	// ---- 2. 默认字段 ----
	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = "module." + sanitizePackageName(packageName)
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = packageName
	}
	description := strings.TrimSpace(req.Description)
	if description == "" {
		description = title
	}

	// ---- 3. 校验来源 ----
	isExternalURL := strings.HasPrefix(req.URL, "http://") || strings.HasPrefix(req.URL, "https://")
	isLocalHTML := false
	if req.URL != "" && !isExternalURL {
		ext := strings.ToLower(filepath.Ext(req.URL))
		isLocalHTML = ext == ".html" || ext == ".htm"
	}
	isLocalDir := false
	if info, statErr := os.Stat(req.URL); statErr == nil && info.IsDir() {
		isLocalDir = true
	}
	isProgram := false
	if req.Path != "" {
		ext := strings.ToLower(filepath.Ext(req.Path))
		isProgram = ext == ".exe" || ext == ".ps1" || ext == ".bat" || ext == ".cmd" || ext == ".lnk"
	}
	if req.ZipPath == "" && req.URL == "" && req.Path == "" {
		jsonOK(w, http.StatusBadRequest, ModuleCreateResponse{Success: false, Message: "请提供 URL/路径 或 上传 ZIP 文件"})
		return
	}

	// mini-LTP 仅适用于本地 HTML 项目
	if req.MiniLTP && req.ZipPath == "" && !isLocalHTML && !isLocalDir {
		jsonOK(w, http.StatusBadRequest, ModuleCreateResponse{Success: false, Message: "mini-LTP 仅支持本地 HTML 项目（本地路径或 ZIP）"})
		return
	}

	// ---- 4. 创建包目录 ----
	baseDir := packageBaseDir()
	packageDir := filepath.Join(baseDir, packageName)
	if err := os.MkdirAll(packageDir, 0755); err != nil {
		jsonOK(w, http.StatusInternalServerError, ModuleCreateResponse{Success: false, Message: "创建包目录失败: " + err.Error()})
		return
	}

	// ---- 5. 写入项目文件 ----
	hasIndexHTML := false
	switch {
	case req.ZipPath != "":
		// ZIP 解压（含 zip-slip 防护）
		n, err := module.ExtractZipToDir(req.ZipPath, packageDir)
		os.Remove(req.ZipPath) // 清理临时上传文件
		if err != nil {
			jsonOK(w, http.StatusInternalServerError, ModuleCreateResponse{Success: false, Message: "解压 ZIP 失败: " + err.Error()})
			return
		}
		LoggerGeneral.Info("CrystalAstral", "创建模块[%s]：ZIP 解压 %d 个文件", packageName, n)
		hasIndexHTML = fileExists(filepath.Join(packageDir, "index.html"))
	case isLocalDir:
		// 本地目录：递归复制
		if err := copyDir(req.URL, packageDir); err != nil {
			jsonOK(w, http.StatusInternalServerError, ModuleCreateResponse{Success: false, Message: "复制目录失败: " + err.Error()})
			return
		}
		hasIndexHTML = fileExists(filepath.Join(packageDir, "index.html"))
	case isLocalHTML:
		// 本地 HTML 文件：复制为 index.html
		data, err := os.ReadFile(req.URL)
		if err != nil {
			jsonOK(w, http.StatusInternalServerError, ModuleCreateResponse{Success: false, Message: "读取 HTML 文件失败: " + err.Error()})
			return
		}
		if err := os.WriteFile(filepath.Join(packageDir, "index.html"), data, 0644); err != nil {
			jsonOK(w, http.StatusInternalServerError, ModuleCreateResponse{Success: false, Message: "写入 index.html 失败: " + err.Error()})
			return
		}
		hasIndexHTML = true
	}

	// ---- 6. mini-LTP：仅登记标签与工具；智能体由琉璃前端在 iframe 加载时动态注入（不改动包 HTML） ----
	var tags []string
	tags = append(tags, req.Tags...)
	var tools []map[string]any
	if req.MiniLTP {
		if !hasIndexHTML {
			jsonOK(w, http.StatusBadRequest, ModuleCreateResponse{Success: false, Message: "开启 mini-LTP 需要本地 HTML 项目（缺少 index.html）"})
			return
		}
		tags = append(tags, "LTPX", "mini-LTP")
		tools = append(tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        sanitizeToolSlug(id),
				"description": buildMiniLTPToolDesc(id, title, description),
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"instruction": map[string]any{
							"type":        "string",
							"description": "要驱动该模块执行的自然语言操作指令，例如：点击开始按钮；把城市切换到雨天；按下 W 键向前；滑动到页面底部",
						},
					},
					"required": []string{"instruction"},
				},
			},
		})
	}

	// ---- 7. 图标处理 ----
	icon := ""
	if req.Icon != "" {
		icon = saveModuleIcon(packageDir, req.Icon)
	}

	// ---- 8. 写 metadata.json ----
	meta := map[string]any{
		"id":          id,
		"title":       title,
		"description": description,
		"tags":        tags,
	}
	if icon != "" {
		meta["icon"] = icon
	}
	if req.URL != "" && isExternalURL {
		meta["url"] = req.URL
	}
	if isProgram && req.Path != "" {
		meta["path"] = req.Path
	}
	if len(tools) > 0 {
		meta["tools"] = tools
	}

	metaData, err := json.MarshalIndent(meta, "", "\t")
	if err != nil {
		jsonOK(w, http.StatusInternalServerError, ModuleCreateResponse{Success: false, Message: "序列化 metadata 失败: " + err.Error()})
		return
	}
	if err := os.WriteFile(filepath.Join(packageDir, "metadata.json"), metaData, 0644); err != nil {
		jsonOK(w, http.StatusInternalServerError, ModuleCreateResponse{Success: false, Message: "写入 metadata.json 失败: " + err.Error()})
		return
	}

	LoggerGeneral.Info("CrystalAstral", "创建模块成功: %s (ID: %s, 标题: %s, mini-LTP: %v)", packageName, id, title, req.MiniLTP)
	jsonOK(w, http.StatusOK, ModuleCreateResponse{
		Success:     true,
		Message:     fmt.Sprintf("模块「%s」创建成功", title),
		PackageName: packageName,
		PackageID:   id,
	})
}

// fileExists 判断文件是否存在
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// copyDir 递归复制目录内容（跳过元数据文件）
func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() == "metadata.json" {
			continue
		}
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := os.MkdirAll(dstPath, 0755); err != nil {
				return err
			}
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dstPath, data, 0644); err != nil {
			return err
		}
	}
	return nil
}

// saveModuleIcon 保存图标：
//   - data:image/...;base64,... → 解码写入包目录 icon.<ext>，返回相对文件名
//   - 其它字符串（相对路径/URL）→ 原样返回，作为 metadata.icon
func saveModuleIcon(packageDir, icon string) string {
	icon = strings.TrimSpace(icon)
	if icon == "" {
		return ""
	}
	if strings.HasPrefix(icon, "data:image/") {
		comma := strings.Index(icon, ",")
		if comma <= 0 {
			return ""
		}
		mime := strings.TrimSuffix(strings.TrimPrefix(icon[:comma], "data:"), ";base64")
		ext := ".png"
		switch strings.ToLower(mime) {
		case "image/webp":
			ext = ".webp"
		case "image/jpeg", "image/jpg":
			ext = ".jpg"
		case "image/gif":
			ext = ".gif"
		case "image/svg+xml":
			ext = ".svg"
		}
		raw, err := base64.StdEncoding.DecodeString(icon[comma+1:])
		if err != nil {
			return ""
		}
		name := "icon" + ext
		if err := os.WriteFile(filepath.Join(packageDir, name), raw, 0644); err != nil {
			return ""
		}
		return name
	}
	// 手动指定的相对路径 / URL
	return icon
}

// miniLTPAgentHandler GET /mini-ltp-agent.js
// 供琉璃前端在 iframe 加载 mini-LTP 包时动态注入通用页面操作智能体。
// 智能体源码从内嵌资源读取（go:embed assets/*），随二进制发布、版本一致；
// 包 index.html 保持原样，不改动原本项目代码。
func miniLTPAgentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	data, err := EmbeddedFiles.ReadFile("assets/mini_ltp_agent.js")
	if err != nil {
		http.Error(w, "读取智能体资源失败", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(data)
}

// reprTitle 从 HTML 中提取 <title> 内容
func reprTitle(html string) string {
	re := regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	m := re.FindStringSubmatch(html)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// buildMiniLTPToolDesc 生成 mini-LTP 页面操作工具的增强描述。
// 相比粗糙的"操作 XX 页面"模板，它把「这是驱动什么模块、该模块做什么、能对页面做什么」讲清楚，
// 让月华在工具链中一眼看懂该工具的用途，避免多工具间取舍困难。
func buildMiniLTPToolDesc(id, title, description string) string {
	title = strings.TrimSpace(title)
	desc := strings.TrimSpace(description)

	// 工具名人类可读：把 id/lower 的段用 . 还原为可读短语
	readable := title
	if readable == "" {
		readable = strings.ReplaceAll(strings.TrimSpace(id), "_", " ")
	}

	var b strings.Builder
	b.WriteString("驱动「" + readable + "」模块页面(DeepSeek mini-LTP 通用页面操作工具)。")
	if desc != "" {
		// 精炼补充该模块是什么：截断避免过长
		d := conciseText(desc, 90)
		b.WriteString("该模块：" + d + "。")
	}
	b.WriteString("接受自然语言指令，智能体自动识别意图并依次执行点击、输入文本、按键(键入/短按/长按)、滑动/滚动、元素捕获、下拉选择等页面操作，操作完成后返回执行结果。")
	return b.String()
}

// conciseText 截取文本前 maxRune 个字符（按 UTF-8 字节安全）
func conciseText(s string, maxBytes int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	// 按字节截断并去除可能截断的中文
	cut := s[:maxBytes]
	return cut + "\n…（已截断）"
}

// inspectZipProject 解析 ZIP 内容：提取 README.md / index.html 标题与文本、文件清单
func inspectZipProject(zipPath string) (name string, fields []ModuleInspectField) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", nil
	}
	defer zr.Close()

	// 优先 README.md / index.html，其次收集文件清单
	var readme, indexHTML string
	var filenames []string
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		lower := strings.ToLower(f.Name)
		filenames = append(filenames, f.Name)
		if readme == "" && (strings.HasSuffix(lower, "readme.md") || strings.HasSuffix(lower, "readme.txt")) {
			if rc, err := f.Open(); err == nil {
				b, _ := io.ReadAll(io.LimitReader(rc, 6000))
				rc.Close()
				readme = string(b)
			}
		}
		if indexHTML == "" && (strings.HasSuffix(lower, "index.html") || strings.HasSuffix(lower, "index.htm")) {
			if rc, err := f.Open(); err == nil {
				b, _ := io.ReadAll(io.LimitReader(rc, 6000))
				rc.Close()
				indexHTML = string(b)
			}
		}
	}
	// 项目名：取 ZIP 顶层目录名或第一个文件路径首段
	if len(filenames) > 0 {
		first := strings.SplitN(filenames[0], "/", 2)[0]
		if first != "" && first != "." {
			name = first
		}
	}
	if t := reprTitle(indexHTML); t != "" {
		fields = append(fields, ModuleInspectField{Key: "title", Text: t})
	}
	if readme != "" {
		fields = append(fields, ModuleInspectField{Key: "README", Text: conciseText(readme, 1500)})
	}
	// 文件清单（最多 40 项）
	var list []string
	for i, fn := range filenames {
		if i >= 40 {
			list = append(list, "…")
			break
		}
		list = append(list, fn)
	}
	fields = append(fields, ModuleInspectField{Key: "filenames", Text: strings.Join(list, "\n")})
	return name, fields
}

// inspectDirProject 检查本地目录/HTML 文件：提取标题与 README 摘要
func inspectDirProject(path string) ModuleInspectResponse {
	resp := ModuleInspectResponse{Success: true, Name: filepath.Base(path)}
	var indexHTML, readme string
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			if b, _ := os.ReadFile(filepath.Join(path, "index.html")); b != nil {
				indexHTML = string(b)
			}
			for _, cand := range []string{"README.md", "readme.md", "README.txt"} {
				if b, err := os.ReadFile(filepath.Join(path, cand)); err == nil {
					readme = conciseText(string(b), 1500)
					break
				}
			}
			if t := reprTitle(indexHTML); t != "" {
				resp.Fields = append(resp.Fields, ModuleInspectField{Key: "title", Text: t})
			}
			if readme != "" {
				resp.Fields = append(resp.Fields, ModuleInspectField{Key: "README", Text: readme})
			}
		} else {
			// 单个 HTML 文件
			if b, _ := os.ReadFile(path); b != nil {
				indexHTML = string(b)
				if t := reprTitle(indexHTML); t != "" {
					resp.Fields = append(resp.Fields, ModuleInspectField{Key: "title", Text: t})
				}
			}
		}
	}
	return resp
}

// moduleInspectHandler POST /api/module/inspect
// 提取 HTML 项目内容（README 标题、index.html <title>、文件清单），供前端 AI 生成模块元信息。
// 支持 ZIP（multipart zip_file）或 URL/本地路径（JSON url）。
func moduleInspectHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ModuleInspectRequest
	var zipTmp string
	ct := r.Header.Get("Content-Type")

	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(128 << 20); err != nil {
			jsonOK(w, http.StatusBadRequest, ModuleInspectResponse{Success: false, Message: "解析表单失败: " + err.Error()})
			return
		}
		req.URL = r.FormValue("url")
		if file, _, err := r.FormFile("zip_file"); err == nil {
			tmp, _ := os.CreateTemp("", "module_inspect_*.zip")
			io.Copy(tmp, file)
			tmp.Close()
			zipTmp = tmp.Name()
			defer os.Remove(zipTmp)
		} else if req.URL == "" {
			jsonOK(w, http.StatusBadRequest, ModuleInspectResponse{Success: false, Message: "请提供 URL/路径或 ZIP 文件"})
			return
		}
	} else {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonOK(w, http.StatusBadRequest, ModuleInspectResponse{Success: false, Message: "解析请求失败: " + err.Error()})
			return
		}
	}

	// ZIP 来源
	if zipTmp != "" {
		name, fields := inspectZipProject(zipTmp)
		if len(fields) == 0 {
			jsonOK(w, http.StatusOK, ModuleInspectResponse{Success: true, Name: name, Fields: fields})
			return
		}
		jsonOK(w, http.StatusOK, ModuleInspectResponse{Success: true, Name: name, Fields: fields})
		return
	}

	// URL/路径来源
	urlOrPath := strings.TrimSpace(req.URL)
	if urlOrPath == "" {
		jsonOK(w, http.StatusBadRequest, ModuleInspectResponse{Success: false, Message: "请提供 URL/路径"})
		return
	}

	if strings.HasPrefix(urlOrPath, "http://") || strings.HasPrefix(urlOrPath, "https://") {
		// 外部链接：尝试抓取 HTML 提取 title
		client := &http.Client{Timeout: 8 * time.Second}
		if resp, err := client.Get(urlOrPath); err == nil {
			defer resp.Body.Close()
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 8000))
			html := string(b)
			fields := []ModuleInspectField{}
			if t := reprTitle(html); t != "" {
				fields = append(fields, ModuleInspectField{Key: "title", Text: t})
			}
			jsonOK(w, http.StatusOK, ModuleInspectResponse{Success: true, Name: urlOrPath, Fields: fields})
			return
		}
		// 抓取失败：仅返回 URL 让 AI 基于链接推断
		jsonOK(w, http.StatusOK, ModuleInspectResponse{Success: true, Name: urlOrPath, Fields: []ModuleInspectField{{Key: "url", Text: urlOrPath}}})
		return
	}

	// 本地路径
	resp := inspectDirProject(urlOrPath)
	resp.Success = true
	jsonOK(w, http.StatusOK, resp)
}
