package server

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"logger"
	"net/http"
	"strings"

	"gguf_metadata_viewer/gguf"
)

//go:embed static/*
var staticFiles embed.FS

// Server HTTP服务器结构体
type Server struct {
	httpServer *http.Server
}

// New 创建新的HTTP服务器实例
func New(port int) *Server {
	// 从embed中提取static子文件系统
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		logger.Error("Server", "无法加载嵌入式静态文件: %v", err)
		staticFS = nil
	}

	mux := http.NewServeMux()

	// 静态文件服务
	if staticFS != nil {
		fileServer := http.FileServer(http.FS(staticFS))
		mux.Handle("/", fileServer)
	}

	// API端点：文件上传并解析GGUF元数据
	mux.HandleFunc("/api/upload", handleUpload)

	return &Server{
		httpServer: &http.Server{
			Addr:    fmt.Sprintf("127.0.0.1:%d", port),
			Handler: mux,
		},
	}
}

// Start 启动HTTP服务器
func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

// Shutdown 安全关闭HTTP服务器
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// handleUpload 处理GGUF文件上传请求，解析元数据并返回JSON响应
func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持POST请求", http.StatusMethodNotAllowed)
		return
	}

	// 设置合理的上传大小限制（允许大文件，但仅读取 header）
	r.Body = http.MaxBytesReader(w, r.Body, 128<<30)

	// 解析multipart表单，内存缓冲32MB（超出部分由Go自动写入临时文件）
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		logger.Error("Server", "解析上传表单失败: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "文件上传解析失败: " + err.Error(),
		})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		logger.Error("Server", "获取上传文件失败: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "未找到上传文件",
		})
		return
	}
	defer file.Close()

	// 验证文件扩展名
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".gguf") {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "仅支持.gguf格式的模型文件",
		})
		return
	}

	logger.Info("Server", "收到GGUF文件上传: %s (大小: %d 字节)", header.Filename, header.Size)
	logger.Info("Server", "开始从上传流中直接解析GGUF header（无需将完整文件写入磁盘）...")

	// 直接从 multipart 文件流中解析 GGUF header（仅读取元数据部分，跳过 tensor 数据）
	metadata, err := gguf.ParseMetadataFromReader(file)
	if err != nil {
		logger.Error("Server", "解析GGUF元数据失败: %v", err)
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"success": false,
			"error":   "GGUF文件解析失败: " + err.Error(),
		})
		return
	}

	logger.Info("Server", "Header解析完成（未读取 tensor 数据，适用于任意大小文件）")

	// 打印元数据到终端日志（Developer模式）
	logger.Info("Server", "========== GGUF元数据: %s ==========", header.Filename)
	for key, value := range metadata {
		logger.SubInfo("Server", "Metadata", "  %-40s = %v", key, value)
	}
	logger.Info("Server", "========== 元数据解析完毕 (%d 项) ==========", len(metadata))

	// 转换为JSON友好格式，提取关键摘要
	jsonMetadata := convertToJSONMap(metadata)
	summary := extractSummary(metadata, header.Filename)

	writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"filename": header.Filename,
		"fileSize": header.Size,
		"summary":  summary,
		"metadata": jsonMetadata,
		"count":    len(metadata),
	})
}

// convertToJSONMap 将原始元数据转换为JSON友好的字符串映射
func convertToJSONMap(metadata map[string]any) map[string]string {
	result := make(map[string]string, len(metadata))
	for key, value := range metadata {
		result[key] = formatMetadataValue(value)
	}
	return result
}

// formatMetadataValue 将单个元数据值格式化为字符串
func formatMetadataValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	case uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v)
	case int8, int16, int32, int64:
		return fmt.Sprintf("%d", v)
	case float32, float64:
		return fmt.Sprintf("%.6f", v)
	case []any:
		parts := make([]string, 0, len(v))
		for i, elem := range v {
			if i > 10 {
				parts = append(parts, fmt.Sprintf("... (%d items)", len(v)))
				break
			}
			parts = append(parts, formatMetadataValue(elem))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// extractSummary 从元数据中提取关键摘要信息
func extractSummary(metadata map[string]any, filename string) map[string]string {
	summary := make(map[string]string)

	// 模型名称
	if name, ok := getString(metadata, "general.name"); ok {
		summary["模型名称"] = name
	} else {
		summary["模型名称"] = filename
	}

	// 架构
	if arch, ok := getString(metadata, "general.architecture"); ok {
		summary["架构"] = arch
	}

	// 文件类型/量化方式
	if fileType, ok := getString(metadata, "general.file_type"); ok {
		summary["量化方式"] = fileType
	}

	// 量化版本
	if qv := getAny(metadata, "general.quantization_version"); qv != nil {
		summary["量化版本"] = formatMetadataValue(qv)
	}

	// 上下文长度
	if ctxLen := getAny(metadata, "llama.context_length"); ctxLen != nil {
		summary["上下文长度"] = formatMetadataValue(ctxLen)
	} else if ctxLen = getAny(metadata, "qwen2.context_length"); ctxLen != nil {
		summary["上下文长度"] = formatMetadataValue(ctxLen)
	}

	// 嵌入维度
	if embLen := getAny(metadata, "llama.embedding_length"); embLen != nil {
		summary["嵌入维度"] = formatMetadataValue(embLen)
	} else if embLen = getAny(metadata, "qwen2.embedding_length"); embLen != nil {
		summary["嵌入维度"] = formatMetadataValue(embLen)
	}

	// 层数
	if blockCount := getAny(metadata, "llama.block_count"); blockCount != nil {
		summary["层数"] = formatMetadataValue(blockCount)
	} else if blockCount = getAny(metadata, "qwen2.block_count"); blockCount != nil {
		summary["层数"] = formatMetadataValue(blockCount)
	}

	// 注意力头数
	if headCount := getAny(metadata, "llama.attention.head_count"); headCount != nil {
		summary["注意力头数"] = formatMetadataValue(headCount)
	} else if headCount = getAny(metadata, "qwen2.attention.head_count"); headCount != nil {
		summary["注意力头数"] = formatMetadataValue(headCount)
	}

	// KV注意力头数
	if headCountKV := getAny(metadata, "llama.attention.head_count_kv"); headCountKV != nil {
		summary["KV头数"] = formatMetadataValue(headCountKV)
	} else if headCountKV = getAny(metadata, "qwen2.attention.head_count_kv"); headCountKV != nil {
		summary["KV头数"] = formatMetadataValue(headCountKV)
	}

	// FFN维度
	if ffnLen := getAny(metadata, "llama.feed_forward_length"); ffnLen != nil {
		summary["FFN维度"] = formatMetadataValue(ffnLen)
	} else if ffnLen = getAny(metadata, "qwen2.feed_forward_length"); ffnLen != nil {
		summary["FFN维度"] = formatMetadataValue(ffnLen)
	}

	// 词表大小
	if vocabSize := getAny(metadata, "tokenizer.ggml.token_count"); vocabSize != nil {
		summary["词表大小"] = formatMetadataValue(vocabSize)
	}

	return summary
}

// getString 从元数据中获取字符串值
func getString(metadata map[string]any, key string) (string, bool) {
	if val, ok := metadata[key]; ok {
		if s, ok := val.(string); ok {
			return s, ok
		}
	}
	return "", false
}

// getAny 从元数据中获取任意值
func getAny(metadata map[string]any, key string) any {
	if val, ok := metadata[key]; ok {
		return val
	}
	return nil
}

// writeJSON 写入JSON响应
func writeJSON(w http.ResponseWriter, status int, data map[string]any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		logger.Error("Server", "写入JSON响应失败: %v", err)
	}
}