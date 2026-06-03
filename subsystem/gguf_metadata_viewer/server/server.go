package server

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"logger"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"

	"gguf_metadata_viewer/gguf"
)

//go:embed static/*
var staticFiles embed.FS

type Server struct {
	httpServer *http.Server
}

func New(port int) *Server {
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		logger.Error("Server", "failed to load embedded static files: %v", err)
		staticFS = nil
	}

	mux := http.NewServeMux()

	if staticFS != nil {
		fileServer := http.FileServer(http.FS(staticFS))
		mux.Handle("/", fileServer)
	}

	mux.HandleFunc("/api/open-file-dialog", handleOpenFileDialog)
	mux.HandleFunc("/api/analyze-path", handleAnalyzePath)

	return &Server{
		httpServer: &http.Server{
			Addr:    fmt.Sprintf("127.0.0.1:%d", port),
			Handler: mux,
		},
	}
}

func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// handleOpenFileDialog opens a native file dialog and returns the selected path.
func handleOpenFileDialog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	logger.Info("Server", "Opening native file dialog...")

	filePath, err := openNativeFileDialog()
	if err != nil {
		logger.Error("Server", "File dialog failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"error":   "File dialog failed: " + err.Error(),
		})
		return
	}

	if filePath == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"success":   false,
			"cancelled": true,
		})
		return
	}

	logger.Info("Server", "User selected: %s", filePath)

	writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"filePath": filePath,
		"fileName": filepath.Base(filePath),
	})
}

// handleAnalyzePath reads a GGUF file at the given path and returns metadata.
func handleAnalyzePath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		FilePath string `json:"filePath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "Invalid request: " + err.Error(),
		})
		return
	}

	if req.FilePath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "File path is empty",
		})
		return
	}

	if !strings.HasSuffix(strings.ToLower(req.FilePath), ".gguf") {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "Only .gguf files are supported",
		})
		return
	}

	fileName := filepath.Base(req.FilePath)
	logger.Info("Server", "Analyzing GGUF file: %s", req.FilePath)

	metadata, err := gguf.ParseMetadata(req.FilePath)
	if err != nil {
		logger.Error("Server", "GGUF parse failed: %v", err)
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"success": false,
			"error":   "GGUF parse failed: " + err.Error(),
		})
		return
	}

	logger.Info("Server", "Header parsed (metadata only, size-independent)")

	logger.Info("Server", "========== GGUF Metadata: %s ==========", fileName)
	for key, value := range metadata {
		logger.SubInfo("Server", "Metadata", "  %-40s = %v", key, value)
	}
	logger.Info("Server", "========== Metadata complete (%d keys) ==========", len(metadata))

	jsonMetadata := convertToJSONMap(metadata)
	summary := extractSummary(metadata, fileName)

	writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"filename": fileName,
		"filePath": req.FilePath,
		"summary":  summary,
		"metadata": jsonMetadata,
		"count":    len(metadata),
	})
}

// openNativeFileDialog uses PowerShell to show a Windows file open dialog.
func openNativeFileDialog() (string, error) {
	script := `Add-Type -AssemblyName System.Windows.Forms
$d = New-Object System.Windows.Forms.OpenFileDialog
$d.Filter = 'GGUF Model Files (*.gguf)|*.gguf|All Files (*.*)|*.*'
$d.Title = 'Select GGUF Model File'
$d.RestoreDirectory = $true
if ($d.ShowDialog() -eq 'OK') { $d.FileName }`

	cmd := exec.Command("powershell", "-NoProfile", "-Command", script)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("PowerShell execution failed: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func convertToJSONMap(metadata map[string]any) map[string]string {
	result := make(map[string]string, len(metadata))
	for key, value := range metadata {
		result[key] = formatMetadataValue(value)
	}
	return result
}

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

func extractSummary(metadata map[string]any, filename string) map[string]string {
	summary := make(map[string]string)

	if name, ok := getString(metadata, "general.name"); ok {
		summary["Model Name"] = name
	} else {
		summary["Model Name"] = filename
	}

	if arch, ok := getString(metadata, "general.architecture"); ok {
		summary["Architecture"] = arch
	}

	if fileType, ok := getString(metadata, "general.file_type"); ok {
		summary["Quantization"] = fileType
	}

	if qv := getAny(metadata, "general.quantization_version"); qv != nil {
		summary["Quant Version"] = formatMetadataValue(qv)
	}

	if ctxLen := getAny(metadata, "llama.context_length"); ctxLen != nil {
		summary["Context Length"] = formatMetadataValue(ctxLen)
	} else if ctxLen = getAny(metadata, "qwen2.context_length"); ctxLen != nil {
		summary["Context Length"] = formatMetadataValue(ctxLen)
	}

	if embLen := getAny(metadata, "llama.embedding_length"); embLen != nil {
		summary["Embedding Dim"] = formatMetadataValue(embLen)
	} else if embLen = getAny(metadata, "qwen2.embedding_length"); embLen != nil {
		summary["Embedding Dim"] = formatMetadataValue(embLen)
	}

	if blockCount := getAny(metadata, "llama.block_count"); blockCount != nil {
		summary["Block Count"] = formatMetadataValue(blockCount)
	} else if blockCount = getAny(metadata, "qwen2.block_count"); blockCount != nil {
		summary["Block Count"] = formatMetadataValue(blockCount)
	}

	if headCount := getAny(metadata, "llama.attention.head_count"); headCount != nil {
		summary["Attention Heads"] = formatMetadataValue(headCount)
	} else if headCount = getAny(metadata, "qwen2.attention.head_count"); headCount != nil {
		summary["Attention Heads"] = formatMetadataValue(headCount)
	}

	if headCountKV := getAny(metadata, "llama.attention.head_count_kv"); headCountKV != nil {
		summary["KV Heads"] = formatMetadataValue(headCountKV)
	} else if headCountKV = getAny(metadata, "qwen2.attention.head_count_kv"); headCountKV != nil {
		summary["KV Heads"] = formatMetadataValue(headCountKV)
	}

	if ffnLen := getAny(metadata, "llama.feed_forward_length"); ffnLen != nil {
		summary["FFN Dim"] = formatMetadataValue(ffnLen)
	} else if ffnLen = getAny(metadata, "qwen2.feed_forward_length"); ffnLen != nil {
		summary["FFN Dim"] = formatMetadataValue(ffnLen)
	}

	if vocabSize := getAny(metadata, "tokenizer.ggml.token_count"); vocabSize != nil {
		summary["Vocab Size"] = formatMetadataValue(vocabSize)
	}

	return summary
}

func getString(metadata map[string]any, key string) (string, bool) {
	if val, ok := metadata[key]; ok {
		if s, ok := val.(string); ok {
			return s, ok
		}
	}
	return "", false
}

func getAny(metadata map[string]any, key string) any {
	if val, ok := metadata[key]; ok {
		return val
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, data map[string]any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		logger.Error("Server", "Failed to write JSON: %v", err)
	}
}