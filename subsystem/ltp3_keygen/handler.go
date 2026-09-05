package main

// ==== HTTP 处理器：前端页面 / 权限列表 / 密钥生成 ====

import (
	lunardecoder "LunarSubsystem/LunarDecoder"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

//go:embed assets/index.html
var indexHTML []byte

// indexHandler GET / 返回前端页面。
func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(indexHTML)
}

// permsHandler GET /api/perms 返回权限名全集（前端下拉框）。
func permsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"permissions": allPermissions})
}

// genHandler POST /api/gen 生成权限密钥。
// 支持两种脚本来源（二选一，dir 优先）：
//   - multipart 文件上传：字段名 files（可多个）+ 表单字段 permissions（JSON 数组）
//   - 表单字段 dir：插件目录绝对路径（后端自行扫描 .js，保证与引擎哈希规则一致）
func genHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp := genResponse{}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		jsonOK(w, http.StatusBadRequest, &genResponse{Success: false, Error: "解析表单失败: " + err.Error()})
		return
	}

	// 权限名
	var perms []string
	if raw := r.FormValue("permissions"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &perms); err != nil {
			jsonOK(w, http.StatusBadRequest, &genResponse{Success: false, Error: "permissions 需为 JSON 数组"})
			return
		}
	}
	if len(perms) == 0 {
		jsonOK(w, http.StatusBadRequest, &genResponse{Success: false, Error: "请至少选择 1 个权限"})
		return
	}
	if err := validatePerms(perms); err != nil {
		jsonOK(w, http.StatusBadRequest, &genResponse{Success: false, Error: err.Error()})
		return
	}
	resp.Permissions = perms

	// 脚本来源：目录优先
	var files []scriptFile
	if dir := strings.TrimSpace(r.FormValue("dir")); dir != "" {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			jsonOK(w, http.StatusBadRequest, &genResponse{Success: false, Error: "目录无效: " + dir})
			return
		}
		files, err = readScriptsFromDir(dir)
		if err != nil {
			jsonOK(w, http.StatusInternalServerError, &genResponse{Success: false, Error: "读取目录失败: " + err.Error()})
			return
		}
	} else {
		fh := r.MultipartForm.File["files"]
		if len(fh) == 0 {
			jsonOK(w, http.StatusBadRequest, &genResponse{Success: false, Error: "请上传脚本文件或填写插件目录"})
			return
		}
		for _, h := range fh {
			f, err := h.Open()
			if err != nil {
				jsonOK(w, http.StatusBadRequest, &genResponse{Success: false, Error: "读取文件失败: " + h.Filename})
				return
			}
			buf := make([]byte, 0, h.Size)
			tmp := make([]byte, 32*1024)
			for {
				n, rerr := f.Read(tmp)
				if n > 0 {
					buf = append(buf, tmp[:n]...)
				}
				if rerr != nil {
					break
				}
			}
			f.Close()
			files = append(files, scriptFile{Name: h.Filename, Data: buf})
		}
	}
	if len(files) == 0 {
		jsonOK(w, http.StatusBadRequest, &genResponse{Success: false, Error: "未找到任何 .js 脚本文件"})
		return
	}
	resp.FileCount = len(files)
	for _, f := range files {
		resp.Files = append(resp.Files, f.Name)
	}

	// 生成密钥：每权限一条 32 字符密钥字符串，整体加密为一段密文
	hashKey := scriptsHashFromFiles(files)
	resp.Hash = hashKey
	keys := buildPermissionKeys(perms)
	key, err := encryptPermissionKeys(keys, hashKey)
	if err != nil {
		jsonOK(w, http.StatusInternalServerError, &genResponse{Success: false, Error: "加密权限密钥失败: " + err.Error()})
		return
	}
	resp.Key = key
	resp.Length = len(key)
	resp.Permissions = perms
	resp.Success = true
	jsonOK(w, http.StatusOK, &resp)
}

// jsonOK 统一 JSON 输出。
func jsonOK(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// decodePermissionPayload 校验密文：整体解码 → 按 '+' 分割 → 去 '/' 填充 → 权限名。
// 规则与引擎 agent/YaraLTP/permission.go 完全一致。
func decodePermissionPayload(cipher, keyStr string) (plain string, blocks []verifyBlock, perms []string, err error) {
	decoded, derr := lunardecoder.DecodeFilesWithKeyString([]lunardecoder.FileData{{Name: "perm", Data: []byte(cipher)}}, keyStr)
	if derr != nil {
		return "", nil, nil, derr
	}
	if len(decoded) == 0 {
		return "", nil, nil, fmt.Errorf("解码结果为空")
	}
	plain = string(decoded[0].Data)
	parts := strings.Split(plain, permSeparator)
	blocks = make([]verifyBlock, 0, len(parts))
	perms = []string{}
	for i, block := range parts {
		block = strings.TrimSpace(block)
		vb := verifyBlock{Index: i + 1, Len: len(block), Text: block}
		name := strings.TrimRight(block, permPadding)
		name = strings.TrimSpace(name)
		if name != "" {
			perms = append(perms, name)
			vb.Perm = name
		}
		blocks = append(blocks, vb)
	}
	return plain, blocks, perms, nil
}

// verifyHandler POST /api/verify 校验权限密钥：输入脚本哈希密钥与 permissions.key 密文，
// 返回整体解码结果、各条密钥字符串与恢复出的权限名。
func verifyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req verifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonOK(w, http.StatusBadRequest, verifyResponse{Success: false, Error: "请求体需为 JSON: " + err.Error()})
		return
	}
	keyStr := strings.TrimSpace(req.Key)
	cipher := strings.TrimSpace(req.Cipher)
	if keyStr == "" {
		jsonOK(w, http.StatusBadRequest, verifyResponse{Success: false, Error: "请提供脚本哈希密钥 key"})
		return
	}
	if cipher == "" {
		jsonOK(w, http.StatusBadRequest, verifyResponse{Success: false, Error: "请提供密文 cipher"})
		return
	}
	plain, blocks, perms, err := decodePermissionPayload(cipher, keyStr)
	if err != nil {
		jsonOK(w, http.StatusOK, verifyResponse{Success: false, Error: "解码失败（密钥与密文不匹配，或密文已损坏/脚本哈希已变化）: " + err.Error()})
		return
	}
	jsonOK(w, http.StatusOK, verifyResponse{Success: true, Plain: plain, Blocks: blocks, Perms: perms})
}
